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
		if v, err := strconv.Atoi(os.Getenv("LLM_COPY_JOB_BUDGET_SECS")); err == nil && v > 0 {
			store.CopyFillBudget = time.Duration(v) * time.Second
		}
		log.Printf("llm rolling copy enabled: model=%s concurrency=%d lookahead_days=%d", cfg.Model, cfg.Concurrency, store.CopyLookaheadDays)
	} else {
		log.Printf("llm news copy disabled (LLM_BASE_URL unset) — template copy")
	}
	srv := &http.Server{Addr: addr, Handler: api.Router()}
	workerCtx, stopWorkers := context.WithCancel(ctx)
	defer stopWorkers()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			cleanupCtx, cancel := context.WithTimeout(workerCtx, 5*time.Second)
			deleted, err := store.DeleteExpiredLobbyRooms(cleanupCtx, pool)
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("room cleanup: %v", err)
			} else if deleted > 0 {
				log.Printf("room cleanup: reclaimed %d expired lobby rooms", deleted)
			}
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			if err := store.RunAgentTurns(workerCtx, pool, time.Now()); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("agent turns: %v", err)
			}
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	if api.CopyFiller != nil {
		go func() {
			for {
				worked, err := store.RunNextCopyJob(workerCtx, pool, api.CopyFiller, time.Now())
				if err != nil && !errors.Is(err, context.Canceled) {
					log.Printf("copy worker: %v", err)
				}
				delay := 15 * time.Second
				if worked {
					// Drain an eligible five-day window one day at a time without
					// making room creation wait for any provider request.
					delay = time.Second
				}
				timer := time.NewTimer(delay)
				select {
				case <-workerCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
	}
	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	stopWorkers()
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
