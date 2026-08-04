package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	DisplayName  string
	AvatarID     string
}

func (u *User) ProfileComplete() bool { return u.DisplayName != "" && u.AvatarID != "" }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func CreateUser(ctx context.Context, q Querier, username, passwordHash string) (*User, error) {
	u := &User{Username: username, PasswordHash: passwordHash}
	err := q.QueryRow(ctx,
		`INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id`,
		username, passwordHash).Scan(&u.ID)
	if isUniqueViolation(err) {
		return nil, ErrUsernameTaken
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func GetUserByUsername(ctx context.Context, q Querier, username string) (*User, error) {
	u := &User{}
	err := q.QueryRow(ctx,
		`SELECT id, username, password_hash, display_name, avatar_id FROM users WHERE username = $1`,
		username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.AvatarID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func CreateSession(ctx context.Context, q Querier, userID int64, token string, expiresAt time.Time) error {
	_, err := q.Exec(ctx,
		`INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3)`,
		token, userID, expiresAt)
	return err
}

func UserBySession(ctx context.Context, q Querier, token string, now time.Time) (*User, error) {
	u := &User{}
	err := q.QueryRow(ctx, `
		SELECT u.id, u.username, u.password_hash, u.display_name, u.avatar_id
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token = $1 AND s.expires_at > $2`,
		token, now).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.AvatarID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func UpdateUserProfile(ctx context.Context, q Querier, userID int64, displayName, avatarID string) (*User, error) {
	u := &User{}
	err := q.QueryRow(ctx, `
		UPDATE users SET display_name = $2, avatar_id = $3 WHERE id = $1
		RETURNING id, username, password_hash, display_name, avatar_id`,
		userID, displayName, avatarID).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.AvatarID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func DeleteSession(ctx context.Context, q Querier, token string) error {
	_, err := q.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}
