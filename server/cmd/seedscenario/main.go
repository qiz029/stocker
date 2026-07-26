// Command seedscenario writes the built-in synthetic scenario into
// Postgres so rooms can be created against it during development.
// Plan 4's data pipeline replaces this with real historical scenarios.
//
//	DATABASE_URL=postgres://localhost/stocker?sslmode=disable go run ./cmd/seedscenario
package main

import (
	"context"
	"log"
	"os"

	"github.com/toddzheng/stocker/server/internal/scenario"
	"github.com/toddzheng/stocker/server/internal/store"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := store.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	if err := store.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	sc := scenario.Synthetic()
	if err := store.SaveScenario(ctx, pool, sc); err != nil {
		log.Fatalf("save scenario: %v", err)
	}
	log.Printf("seeded scenario %q (%d instruments, %d days)", sc.ID, len(sc.Instruments), sc.Days)
}
