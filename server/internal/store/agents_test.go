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

	// If the loop misses several sim days, it catches up chronologically:
	// day 0-2 orders fill and every agent has turns recorded through day 3.
	if err := RunAgentTurns(ctx, pool, t0.Add(3*61*time.Second)); err != nil {
		t.Fatalf("RunAgentTurns day 3: %v", err)
	}
	var trades, turns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM trades t JOIN users u ON u.id = t.user_id
		WHERE t.room_id = $1 AND u.is_agent`, room.ID).Scan(&trades); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM agent_turns WHERE room_id = $1`, room.ID).Scan(&turns); err != nil {
		t.Fatal(err)
	}
	if trades != 3*AgentPlayerCount || turns != 4*AgentPlayerCount {
		t.Fatalf("catch-up activity: trades=%d turns=%d, want %d/%d",
			trades, turns, 3*AgentPlayerCount, 4*AgentPlayerCount)
	}
}

func TestAgentTurnsIsolateBrokenRooms(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	broken, _, t0 := mkRunningRoom(t, pool)

	host := mkUser(t, pool, "second_host")
	sc := scenarioMustLoad(t, pool)
	healthy, err := CreateRoom(ctx, pool, sc, host.ID, 60, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := StartRoom(ctx, pool, healthy.ID, host.ID, t0); err != nil {
		t.Fatal(err)
	}

	// A persistent invariant violation in the lower-id room must be reported
	// without starving later healthy rooms.
	if _, err := pool.Exec(ctx, `
		DELETE FROM room_players WHERE room_id = $1 AND user_id = (
			SELECT user_id FROM room_players rp JOIN users u ON u.id = rp.user_id
			WHERE rp.room_id = $1 AND u.is_agent ORDER BY u.agent_slot LIMIT 1)`,
		broken.ID); err != nil {
		t.Fatal(err)
	}
	if err := RunAgentTurns(ctx, pool, t0); err == nil {
		t.Fatal("RunAgentTurns returned nil for broken room")
	}
	var healthyEvents int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM room_events WHERE room_id = $1 AND kind = 'agent_order'`,
		healthy.ID).Scan(&healthyEvents); err != nil {
		t.Fatal(err)
	}
	if healthyEvents != AgentPlayerCount {
		t.Fatalf("healthy room events = %d, want %d", healthyEvents, AgentPlayerCount)
	}
}

func TestAgentTurnsCatchUpAfterRoomEnds(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, _, t0 := mkRunningRoom(t, pool)

	// Keep this integration case small while exercising the same final-day
	// boundary as production scenarios.
	room.Days = 5
	if _, err := pool.Exec(ctx, `UPDATE rooms SET days = 5 WHERE id = $1`, room.ID); err != nil {
		t.Fatal(err)
	}
	if err := RunAgentTurns(ctx, pool, t0); err != nil {
		t.Fatal(err)
	}
	if err := RunAgentTurns(ctx, pool, t0.Add(6*60*time.Second)); err != nil {
		t.Fatalf("ended catch-up: %v", err)
	}

	var turns, trades, pending int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM agent_turns WHERE room_id = $1`, room.ID).Scan(&turns); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM trades t JOIN users u ON u.id = t.user_id
		WHERE t.room_id = $1 AND u.is_agent`, room.ID).Scan(&trades); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM orders o JOIN users u ON u.id = o.user_id
		WHERE o.room_id = $1 AND u.is_agent AND o.status = 'pending'`, room.ID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	want := 4 * AgentPlayerCount // decisions on days 0..3, fills on days 1..4
	if turns != want || trades != want || pending != 0 {
		t.Fatalf("ended activity: turns=%d trades=%d pending=%d, want %d/%d/0",
			turns, trades, pending, want, want)
	}
}

func TestAgentCatchUpDoesNotRewindHumanLoanSettlement(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, t0 := mkRunningRoom(t, pool)

	if _, err := Borrow(ctx, pool, room, guest.ID, 1_000_000, t0); err != nil {
		t.Fatal(err)
	}
	at3 := t0.Add(3*61*time.Second + time.Second)
	if _, _, err := SettleRoom(ctx, pool, room, at3); err != nil {
		t.Fatal(err)
	}
	debtBefore, throughBefore, _ := debtOf(t, pool, room.ID, guest.ID)
	// Simulate an Agent batch whose clock observation is stale (day 2) after
	// an HTTP path has already settled through day 3.
	if err := RunAgentTurns(ctx, pool, t0.Add(2*61*time.Second)); err != nil {
		t.Fatal(err)
	}
	debtAfter, throughAfter, _ := debtOf(t, pool, room.ID, guest.ID)
	if debtAfter != debtBefore || throughAfter == nil || throughBefore == nil || *throughAfter != *throughBefore {
		t.Fatalf("human loan settlement changed during agent catch-up: debt %d→%d, through %v→%v",
			debtBefore, debtAfter, throughBefore, throughAfter)
	}
}
