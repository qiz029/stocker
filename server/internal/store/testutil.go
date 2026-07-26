package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDB returns a pool whose search_path points at a freshly dropped and
// recreated schema, then runs migrations into it — every test starts from
// zero state. Skips when STOCKER_TEST_DB is unset. Use a distinct schema
// name per test package ("store", "httpapi") so `go test ./...` running
// packages in parallel cannot collide.
func TestDB(t *testing.T, schema string) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("STOCKER_TEST_DB")
	if url == "" {
		t.Skip("STOCKER_TEST_DB not set; skipping Postgres-backed test")
	}
	ctx := context.Background()
	ident := pgx.Identifier{schema}.Sanitize()

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse STOCKER_TEST_DB: %v", err)
	}
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET search_path TO "+ident)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+ident+" CASCADE"); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+ident); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}
