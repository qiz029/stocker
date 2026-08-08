package store

import (
	"context"
	"io/fs"
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
		"room_forum_posts", "player_actions", "agent_turns",
		"room_copy_jobs",
	} {
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Errorf("table %s: %v", table, err)
		}
	}

	// Every embedded migration is recorded exactly once.
	var applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	if applied != len(names) {
		t.Fatalf("applied migrations = %d, want %d", applied, len(names))
	}
}

func TestUniqueAliasMigrationRepairsLegacyRows(t *testing.T) {
	pool := TestDB(t, "alias_migration")
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DROP INDEX users_display_name_unique`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (username, password_hash, display_name) VALUES
			('legacy_alias_a', '!', 'Market Owl'),
			('legacy_alias_b', '!', 'market owl'),
			('legacy_alias_blank', '!', ''),
			('legacy_alias_agent', '!', '西雅图价值客')`); err != nil {
		t.Fatal(err)
	}
	migration, err := migrationsFS.ReadFile("migrations/0021_unique_aliases.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("reapply unique alias migration: %v", err)
	}

	var total, distinct, blank, agentCollisions int
	if err := pool.QueryRow(ctx, `
		SELECT count(*), count(DISTINCT lower(display_name)),
			count(*) FILTER (WHERE display_name = ''),
			count(*) FILTER (WHERE lower(display_name) IN (
				SELECT lower(agent_name) FROM users WHERE is_agent
				UNION SELECT lower(agent_name_en) FROM users WHERE is_agent
			))
		FROM users WHERE NOT is_agent`).Scan(&total, &distinct, &blank, &agentCollisions); err != nil {
		t.Fatal(err)
	}
	if total != 4 || distinct != total || blank != 0 || agentCollisions != 0 {
		t.Fatalf("repaired aliases: total=%d distinct=%d blank=%d agent_collisions=%d",
			total, distinct, blank, agentCollisions)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE users SET display_name = 'MARKET OWL'
		WHERE username = 'legacy_alias_blank'`); !isConstraintViolation(err, "users_display_name_unique") {
		t.Fatalf("case-insensitive duplicate alias: %v", err)
	}
}
