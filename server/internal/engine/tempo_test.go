package engine

import (
	"reflect"
	"testing"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

func TestFastestDayDurationTargetsShortRoom(t *testing.T) {
	for _, tc := range []struct {
		days int
		want int
	}{{300, 6}, {750, 2}, {881, 2}, {10, 180}} {
		if got := FastestDayDuration(tc.days); got != tc.want {
			t.Errorf("FastestDayDuration(%d) = %d, want %d", tc.days, got, tc.want)
		}
	}
}

func TestCompressedTempoTrimsNarrativeWithoutChangingPrices(t *testing.T) {
	sc := scenario.Synthetic()
	world, err := GenerateWorld(sc, 42)
	if err != nil {
		t.Fatal(err)
	}
	tempo := NewTempo(FastestDayDuration(sc.Days))
	compressed := tempo.CompressWorld(sc, world)
	if !tempo.Compressed() || tempo.DaysPerBeat() != 4 {
		t.Fatalf("tempo = compressed:%v daysPerBeat:%d", tempo.Compressed(), tempo.DaysPerBeat())
	}
	if !reflect.DeepEqual(compressed.Prices, world.Prices) {
		t.Fatal("compressed tempo changed the market price path")
	}
	if len(compressed.News) >= len(world.News)/2 {
		t.Fatalf("news barely compressed: %d from %d", len(compressed.News), len(world.News))
	}
	if len(compressed.Forum) >= len(world.Forum)/4 {
		t.Fatalf("forum barely compressed: %d from %d", len(compressed.Forum), len(world.Forum))
	}
	newsPerBeat := map[int]int{}
	for _, event := range compressed.News {
		newsPerBeat[event.Day/tempo.DaysPerBeat()]++
	}
	for beat, count := range newsPerBeat {
		if count > compressedNewsPerBeat {
			t.Fatalf("beat %d has %d news items", beat, count)
		}
	}
	forumPerBeat := map[int]int{}
	for _, post := range compressed.Forum {
		forumPerBeat[post.Day/tempo.DaysPerBeat()]++
	}
	for beat, count := range forumPerBeat {
		if count > compressedForumPerBeat {
			t.Fatalf("beat %d has %d forum posts", beat, count)
		}
	}
}

func TestCompressedAgentCadenceRotatesTwoSeats(t *testing.T) {
	tempo := NewTempo(2)
	if tempo.DaysPerBeat() != 12 {
		t.Fatalf("days per beat = %d, want 12", tempo.DaysPerBeat())
	}
	for beat := 0; beat < 5; beat++ {
		active := 0
		for slot := 1; slot <= 5; slot++ {
			if tempo.AgentActs(beat*tempo.DaysPerBeat(), slot, 5) {
				active++
			}
		}
		if active != 2 {
			t.Fatalf("beat %d active agents = %d, want 2", beat, active)
		}
	}
	if tempo.AgentActs(1, 1, 5) {
		t.Fatal("agent acted between compressed beats")
	}
}
