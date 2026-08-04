package store

import (
	"context"
	"testing"
	"time"
)

func TestAgentTurnsPlaceLabeledOrdersOncePerDay(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, _, t0 := mkRunningRoom(t, pool)

	if err := RunAgentTurns(ctx, pool, t0); err != nil {
		t.Fatalf("RunAgentTurns day 0: %v", err)
	}

	var pending, events, labeled int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM orders o JOIN users u ON u.id = o.user_id
		WHERE o.room_id = $1 AND u.is_agent AND o.status = 'pending'`, room.ID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE (payload->>'is_agent')::boolean)
		FROM room_events WHERE room_id = $1 AND kind = 'agent_order'`, room.ID).Scan(&events, &labeled); err != nil {
		t.Fatal(err)
	}
	if pending != AgentPlayerCount || events != AgentPlayerCount || labeled != AgentPlayerCount {
		t.Fatalf("day-0 agent activity: pending=%d events=%d labeled=%d; want %d each",
			pending, events, labeled, AgentPlayerCount)
	}

	// Same room/day is idempotent.
	if err := RunAgentTurns(ctx, pool, t0); err != nil {
		t.Fatalf("RunAgentTurns repeat: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM room_events WHERE room_id = $1 AND kind = 'agent_order'`,
		room.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != AgentPlayerCount {
		t.Fatalf("events after repeat = %d, want %d", events, AgentPlayerCount)
	}

	// At the next day the first orders fill, and each agent takes one new turn.
	if err := RunAgentTurns(ctx, pool, t0.Add(61*time.Second)); err != nil {
		t.Fatalf("RunAgentTurns day 1: %v", err)
	}
	var trades int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM trades t JOIN users u ON u.id = t.user_id
		WHERE t.room_id = $1 AND u.is_agent`, room.ID).Scan(&trades); err != nil {
		t.Fatal(err)
	}
	if trades != AgentPlayerCount {
		t.Fatalf("filled agent trades = %d, want %d", trades, AgentPlayerCount)
	}
}
