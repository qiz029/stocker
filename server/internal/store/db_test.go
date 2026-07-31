package store

import (
	"context"
	"testing"
)

// Requires STOCKER_TEST_DB (e.g. postgres://localhost:5432/stocker_test?sslmode=disable);
// skips otherwise. Create the db once with: createdb stocker_test
func TestMigrateCreatesSchemaAndIsIdempotent(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()

	// TestDB already ran Migrate once; running again must be a no-op.
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	// Every table exists and is queryable.
	for _, table := range []string{
		"users", "sessions", "scenarios", "instruments", "scenario_prices",
		"rooms", "room_players", "room_prices", "room_news", "room_chat",
		"orders", "trades", "positions", "room_events",
		"loan_txns", "room_player_daily",
		"room_options", "option_positions", "option_trades",
		"room_forum_posts", "player_actions",
	} {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Errorf("table %s: %v", table, err)
		}
	}

	// Exactly eleven migrations recorded, exactly once.
	var applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	if applied != 11 {
		t.Fatalf("applied migrations = %d, want 11", applied)
	}
}
