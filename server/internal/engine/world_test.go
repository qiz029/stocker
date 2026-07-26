package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestGenerateWorldAssembles(t *testing.T) {
	sc := scenario.Synthetic()
	w, err := GenerateWorld(sc, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Prices) != len(sc.Instruments) {
		t.Fatalf("prices for %d instruments", len(w.Prices))
	}
	tracks := map[Track]int{}
	for i, ev := range w.News {
		if ev.Headline == "" {
			t.Fatalf("news %d missing headline", i)
		}
		tracks[ev.Track]++
	}
	for _, tr := range []Track{TrackHistorical, TrackImpact, TrackNoise} {
		if tracks[tr] == 0 {
			t.Fatalf("no %s-track news", tr)
		}
	}
	if !sort.SliceIsSorted(w.News, func(i, j int) bool { return w.News[i].Day < w.News[j].Day }) {
		t.Fatal("news not sorted by day")
	}
}

// Golden: 固定 (scenario, seed) 的世界哈希不随重构漂移。
// 首跑写入 testdata/world-42.sha256, 之后比对。
func TestGenerateWorldGolden(t *testing.T) {
	sc := scenario.Synthetic()
	w, err := GenerateWorld(sc, 42)
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(w)
	sum := sha256.Sum256(blob)
	got := hex.EncodeToString(sum[:])
	path := filepath.Join("testdata", "world-42.sha256")
	want, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(path, []byte(got), 0o644)
		t.Logf("golden recorded: %s", got)
		return
	}
	if got != string(want) {
		t.Fatalf("world changed for fixed seed:\n got %s\nwant %s", got, want)
	}
}
