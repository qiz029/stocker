package store

import (
	"context"
	"time"
)

// LobbyRoomTTL is how long a room may wait for its host to start it.
const LobbyRoomTTL = 10 * time.Minute

// DeleteExpiredLobbyRooms removes rooms that never left the lobby before the
// TTL. Room-owned rows are deleted by their foreign-key cascades. Expiry uses
// the same database clock that supplies rooms.created_at, avoiding premature
// deletion when application replicas have clock skew. The status predicate
// also makes this safe against a concurrent start: PostgreSQL rechecks it
// after waiting for the row lock.
func DeleteExpiredLobbyRooms(ctx context.Context, q Querier) (int64, error) {
	tag, err := q.Exec(ctx, `
		DELETE FROM rooms
		WHERE status = 'lobby'
			AND created_at <= CURRENT_TIMESTAMP - $1::double precision * INTERVAL '1 second'`,
		LobbyRoomTTL.Seconds())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
