package store

import (
	"context"
	"testing"
	"time"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
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
		SELECT day, npc_name, body, npc_name_en, body_en FROM room_forum_posts
		WHERE room_id = $1 ORDER BY id`, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	prevDay, checked := -1, 0
	for rows.Next() {
		var day int
		var npc, body, npcEn, bodyEn string
		if err := rows.Scan(&day, &npc, &body, &npcEn, &bodyEn); err != nil {
			t.Fatal(err)
		}
		if day < prevDay {
			t.Fatalf("day order broke at day %d after %d", day, prevDay)
		}
		if npc == "" || body == "" {
			t.Fatalf("empty forum row: day=%d npc=%q body=%q", day, npc, body)
		}
		// Row order matches world.Forum (CopyFrom preserves insertion
		// order); en copies round-trip (empty until templates fill them).
		if want := world.Forum[checked]; npcEn != want.NPCNameEn || bodyEn != want.BodyEn {
			t.Fatalf("post %d en copy = %q/%q, want %q/%q", checked, npcEn, bodyEn, want.NPCNameEn, want.BodyEn)
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

// forumEnOnlyFiller changes only the English bodies: the rows must still
// be upgraded (the LLM can succeed in one language and fail the other).
type forumEnOnlyFiller struct{}

func (forumEnOnlyFiller) FillCopy(_ context.Context, _ *scenario.Scenario, _ []engine.NewsEvent) {}

func (forumEnOnlyFiller) FillForumCopy(_ context.Context, _ *scenario.Scenario, posts []engine.ForumPost) {
	for i := range posts {
		posts[i].BodyEn = "EN-only polish."
	}
}

func TestFillForumCopyEnOnlyStillUpgrades(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "host")
	sc := mkScenario(t, pool)

	room, err := CreateRoom(ctx, pool, sc, host.ID, 3600, forumEnOnlyFiller{})
	if err != nil {
		t.Fatal(err)
	}
	// The fill runs asynchronously after CreateRoom returns — poll for it.
	// (Template en copy may already be present, so wait for OUR value.)
	var body, bodyEn string
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := pool.QueryRow(ctx, `
			SELECT body, body_en FROM room_forum_posts WHERE room_id = $1 ORDER BY id LIMIT 1`,
			room.ID).Scan(&body, &bodyEn); err != nil {
			t.Fatal(err)
		}
		if bodyEn == "EN-only polish." || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if body == "" || bodyEn != "EN-only polish." {
		t.Fatalf("en-only forum fill: body=%q body_en=%q, want zh template kept and en persisted", body, bodyEn)
	}
}
