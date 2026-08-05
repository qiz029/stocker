package store

import "context"

// Expo push tokens: one user may carry several devices, so tokens are keyed
// by (user_id, token). The tokens themselves are opaque Expo push tokens.

// AddPushToken registers a token for a user; re-registering is a no-op.
func AddPushToken(ctx context.Context, q Querier, userID int64, token string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO push_tokens (user_id, token) VALUES ($1, $2)
		ON CONFLICT (user_id, token) DO NOTHING`, userID, token)
	return err
}

// RemovePushToken unregisters one token (e.g. on logout). Unknown tokens are
// a no-op.
func RemovePushToken(ctx context.Context, q Querier, userID int64, token string) error {
	_, err := q.Exec(ctx, `
		DELETE FROM push_tokens WHERE user_id = $1 AND token = $2`, userID, token)
	return err
}

// PushTokensForRoom lists the push tokens of every human room member except
// excludeUserID (pass 0 to exclude nobody).
func PushTokensForRoom(ctx context.Context, q Querier, roomID, excludeUserID int64) ([]string, error) {
	rows, err := q.Query(ctx, `
		SELECT pt.token FROM push_tokens pt
		JOIN room_players rp ON rp.user_id = pt.user_id
		WHERE rp.room_id = $1 AND pt.user_id <> $2`, roomID, excludeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var tok string
		if err := rows.Scan(&tok); err != nil {
			return nil, err
		}
		out = append(out, tok)
	}
	return out, rows.Err()
}
