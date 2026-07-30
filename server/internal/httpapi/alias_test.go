package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/store"
)

// TestSeedResolvedAlias: room state and reveal both serve the alias
// resolved deterministically from the room's seed, stably across repeated
// requests. (A second room may pick the same name by chance, so the test
// asserts determinism and membership, never difference.)
func TestSeedResolvedAlias(t *testing.T) {
	s := newServer(t)
	sc := seedScenario(t, s)
	candidates := []string{"Ridgeline Networks", "Vantor Networks", "Copperline Communications"}
	if err := store.SetInstrumentDisplay(context.Background(), s.DB, sc.ID, map[string]store.InstrumentDisplay{
		"S1": {Alias: candidates[0], Aliases: candidates, Desc: "网络设备巨头"},
	}); err != nil {
		t.Fatalf("set display: %v", err)
	}
	t0 := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	advance := fakeClock(s, t0)

	host := registerClient(t, s, "host")
	created := host.mustJSON("POST", "/api/rooms",
		map[string]any{"scenario_id": "synthetic-v1", "day_duration_secs": 60}, http.StatusOK)
	roomID := int64(created["id"].(float64))

	// The expected pick is derived from the room's persisted seed.
	var seed int64
	if err := s.DB.QueryRow(context.Background(),
		`SELECT seed FROM rooms WHERE id = $1`, roomID).Scan(&seed); err != nil {
		t.Fatal(err)
	}
	want := engine.ResolveAlias(uint64(seed), "S1", candidates[0], candidates)

	statePath := fmt.Sprintf("/api/rooms/%d", roomID)
	aliasFromState := func() string {
		state := host.mustJSON("GET", statePath, nil, http.StatusOK)
		s1 := state["instruments"].([]any)[0].(map[string]any)
		return s1["alias"].(string)
	}
	if got := aliasFromState(); got != want {
		t.Fatalf("room state alias %q, want seed-resolved %q", got, want)
	}
	if got := aliasFromState(); got != want {
		t.Fatalf("repeated room state alias %q, want stable %q", got, want)
	}

	// Reveal shows the same per-room name once the game ends.
	host.mustJSON("POST", fmt.Sprintf("/api/rooms/%d/start", roomID), nil, http.StatusOK)
	advance(301 * 60 * time.Second)
	reveal := host.mustJSON("GET", fmt.Sprintf("/api/rooms/%d/reveal", roomID), nil, http.StatusOK)
	r1 := reveal["instruments"].([]any)[0].(map[string]any)
	if r1["alias"] != want {
		t.Fatalf("reveal alias %q, want %q", r1["alias"], want)
	}
}
