package store

import (
	"context"
	"testing"
	"time"
)

func TestDeleteExpiredLobbyRooms(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	sc := mkScenario(t, pool)
	var dbNow time.Time
	if err := pool.QueryRow(ctx, `SELECT CURRENT_TIMESTAMP`).Scan(&dbNow); err != nil {
		t.Fatal(err)
	}

	expired, err := CreateRoom(ctx, pool, sc, mkUser(t, pool, "expired-host").ID, 60, nil)
	if err != nil {
		t.Fatal(err)
	}
	running, err := CreateRoom(ctx, pool, sc, mkUser(t, pool, "running-host").ID, 60, nil)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := CreateRoom(ctx, pool, sc, mkUser(t, pool, "fresh-host").ID, 60, nil)
	if err != nil {
		t.Fatal(err)
	}

	cutoff := dbNow.Add(-LobbyRoomTTL)
	if _, err := pool.Exec(ctx, `UPDATE rooms SET created_at = $2 WHERE id IN ($1, $3)`, expired.ID, cutoff, running.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := StartRoom(ctx, pool, running.ID, running.HostUserID, dbNow); err != nil {
		t.Fatal(err)
	}

	deleted, err := DeleteExpiredLobbyRooms(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted rooms = %d, want 1", deleted)
	}
	if deleted, err := DeleteExpiredLobbyRooms(ctx, pool); err != nil || deleted != 0 {
		t.Fatalf("repeat cleanup: deleted=%d err=%v", deleted, err)
	}
	if _, err := GetRoom(ctx, pool, expired.ID); err != ErrNotFound {
		t.Fatalf("expired room lookup err = %v, want ErrNotFound", err)
	}
	if _, err := GetRoom(ctx, pool, running.ID); err != nil {
		t.Fatalf("running room was deleted: %v", err)
	}
	if _, err := GetRoom(ctx, pool, fresh.ID); err != nil {
		t.Fatalf("fresh lobby was deleted: %v", err)
	}

	for _, table := range []string{"room_players", "room_prices", "room_news", "room_forum_posts", "room_copy_jobs"} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE room_id = $1", expired.ID).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d expired-room rows", table, count)
		}
	}
}
