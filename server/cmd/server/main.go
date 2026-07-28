// Command server runs the stocker HTTP API.
//
//	DATABASE_URL=postgres://localhost/stocker?sslmode=disable ADDR=:8080 go run ./cmd/server
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/toddzheng/stocker/server/internal/httpapi"
	"github.com/toddzheng/stocker/server/internal/llm"
	"github.com/toddzheng/stocker/server/internal/store"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
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

	api := httpapi.NewServer(pool)
	if cfg := llm.FromEnv(); cfg != nil {
		api.CopyFiller = llm.New(*cfg)
		if v, err := strconv.Atoi(os.Getenv("LLM_ROOM_BUDGET_SECS")); err == nil && v > 0 {
			store.CopyFillBudget = time.Duration(v) * time.Second
		}
		log.Printf("llm news copy enabled: model=%s concurrency=%d", cfg.Model, cfg.Concurrency)
	} else {
		log.Printf("llm news copy disabled (LLM_BASE_URL unset) — template copy")
	}
	srv := &http.Server{Addr: addr, Handler: api.Router()}
	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
