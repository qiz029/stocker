package store

import "context"

// Expo push tokens: one user may carry several devices, so tokens are keyed
// by (user_id, token). The tokens themselves are opaque Expo push tokens.

// AddPushToken registers a token for a user. Re-registering updates the
// device's language so push banners follow the current UI preference.
func AddPushToken(ctx context.Context, q Querier, userID int64, token, lang string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO push_tokens (user_id, token, lang) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, token) DO UPDATE SET lang = EXCLUDED.lang`, userID, token, lang)
	return err
}

// RemovePushToken unregisters one token (e.g. on logout). Unknown tokens are
// a no-op.
func RemovePushToken(ctx context.Context, q Querier, userID int64, token string) error {
	_, err := q.Exec(ctx, `
		DELETE FROM push_tokens WHERE user_id = $1 AND token = $2`, userID, token)
	return err
}

// PushToken carries the token and the language selected on that device.
type PushToken struct {
	Token string
	Lang  string
}

// PushTokensForRoom lists the push tokens of every human room member except
// excludeUserID (pass 0 to exclude nobody).
func PushTokensForRoom(ctx context.Context, q Querier, roomID, excludeUserID int64) ([]PushToken, error) {
	rows, err := q.Query(ctx, `
		SELECT pt.token, pt.lang FROM push_tokens pt
		JOIN room_players rp ON rp.user_id = pt.user_id
		WHERE rp.room_id = $1 AND pt.user_id <> $2`, roomID, excludeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushToken
	for rows.Next() {
		var tok PushToken
		if err := rows.Scan(&tok.Token, &tok.Lang); err != nil {
			return nil, err
		}
		out = append(out, tok)
	}
	return out, rows.Err()
}
