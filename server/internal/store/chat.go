package store

import (
	"context"
	"strings"
	"unicode/utf8"
)

type ChatMessage struct {
	ID       int64
	Username string
	IsAgent  bool
	Day      int
	Text     string
	TextEn   string
}

const maxChatRunes = 500

// PostChat records a message stamped with the room's current day (the
// caller computes it — lobby rooms use day 0).
func PostChat(ctx context.Context, q Querier, room *Room, userID int64, day int, text string) (int64, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxChatRunes {
		return 0, ErrBadChatMessage
	}
	var id int64
	err := q.QueryRow(ctx, `
		INSERT INTO room_chat (room_id, user_id, day, text)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		room.ID, userID, day, text).Scan(&id)
	return id, err
}

// ChatSince returns messages with id > afterID in ascending id order.
func ChatSince(ctx context.Context, q Querier, roomID, afterID int64, limit int) ([]ChatMessage, error) {
	rows, err := q.Query(ctx, `
		SELECT c.id, COALESCE(u.agent_name, u.username), u.is_agent,
		       c.day, c.text, c.text_en
		FROM room_chat c JOIN users u ON u.id = c.user_id
		WHERE c.room_id = $1 AND c.id > $2
		ORDER BY c.id LIMIT $3`, roomID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.Username, &m.IsAgent, &m.Day, &m.Text, &m.TextEn); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
