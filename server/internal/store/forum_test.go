package store

import (
	"context"
	"testing"

	"github.com/toddzheng/stocker/server/internal/engine"
)

func TestCreateRoomPersistsForumPosts(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)

	room, err := CreateRoom(ctx, pool, sc, host.ID, 3600, nil)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	world, err := engine.GenerateWorld(sc, room.Seed)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if len(world.Forum) == 0 {
		t.Fatal("world has no forum posts")
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM room_forum_posts WHERE room_id = $1`, room.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(world.Forum) {
		t.Fatalf("room_forum_posts rows = %d, want %d", n, len(world.Forum))
	}

	// Ids follow day order (pagination cursor), fields fully populated.
	rows, err := pool.Query(ctx, `
		SELECT day, npc_name, body FROM room_forum_posts
		WHERE room_id = $1 ORDER BY id`, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	prevDay, checked := -1, 0
	for rows.Next() {
		var day int
		var npc, body string
		if err := rows.Scan(&day, &npc, &body); err != nil {
			t.Fatal(err)
		}
		if day < prevDay {
			t.Fatalf("day order broke at day %d after %d", day, prevDay)
		}
		if npc == "" || body == "" {
			t.Fatalf("empty forum row: day=%d npc=%q body=%q", day, npc, body)
		}
		prevDay, checked = day, checked+1
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if checked != len(world.Forum) {
		t.Fatalf("scanned %d posts, want %d", checked, len(world.Forum))
	}
}
