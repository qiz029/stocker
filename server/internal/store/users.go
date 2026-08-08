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
	Email        string
	Description  string
	SocialLinks  map[string]string
}

func (u *User) ProfileComplete() bool { return u.DisplayName != "" && u.AvatarID != "" }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isConstraintViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
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
		`SELECT id, username, password_hash, display_name, avatar_id, email, description, social_links FROM users WHERE username = $1`,
		username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.AvatarID, &u.Email, &u.Description, &u.SocialLinks)
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
		SELECT u.id, u.username, u.password_hash, u.display_name, u.avatar_id, u.email, u.description, u.social_links
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token = $1 AND s.expires_at > $2`,
		token, now).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.AvatarID, &u.Email, &u.Description, &u.SocialLinks)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func UpdateUserProfile(ctx context.Context, q Querier, userID int64, displayName, avatarID, email, description string, socialLinks map[string]string) (*User, error) {
	var agentConflict bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users
			WHERE is_agent AND (
				lower(agent_name) = lower($1)
				OR lower(COALESCE(agent_name_en, '')) = lower($1)
			)
		)`, displayName).Scan(&agentConflict); err != nil {
		return nil, err
	}
	if agentConflict {
		return nil, ErrAliasTaken
	}
	u := &User{}
	err := q.QueryRow(ctx, `
		UPDATE users SET display_name = $2, avatar_id = $3, email = $4, description = $5, social_links = $6 WHERE id = $1
		RETURNING id, username, password_hash, display_name, avatar_id, email, description, social_links`,
		userID, displayName, avatarID, email, description, socialLinks).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.AvatarID, &u.Email, &u.Description, &u.SocialLinks)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if isConstraintViolation(err, "users_email_unique") {
		return nil, ErrEmailTaken
	}
	if isConstraintViolation(err, "users_display_name_unique") {
		return nil, ErrAliasTaken
	}
	return u, err
}

func UpdateUserPassword(ctx context.Context, q Querier, userID int64, passwordHash string) error {
	tag, err := q.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func DeleteSession(ctx context.Context, q Querier, token string) error {
	_, err := q.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}
