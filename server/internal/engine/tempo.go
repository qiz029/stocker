package engine

import (
	"math"
	"sort"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

const (
	// FastestTargetDurationSecs is the player-facing target for the shortest
	// room preset. Integer per-day durations make the actual total approximate.
	FastestTargetDurationSecs = 30 * 60
	FastestMinDayDurationSecs = 2
	compressedBeatWallSecs    = 24
	compressedNewsPerBeat     = 2
	compressedForumPerBeat    = 1
)

// Tempo is the single seam for pace-dependent simulation cadence. Standard
// rooms retain daily narrative and Agent turns. Rooms below one minute per
// trading day use beats so faster clocks do not flood the player with content.
type Tempo struct {
	dayDurationSecs int
	daysPerBeat     int
}

func NewTempo(dayDurationSecs int) Tempo {
	daysPerBeat := 1
	if dayDurationSecs < 60 {
		daysPerBeat = max(1, int(math.Round(float64(compressedBeatWallSecs)/float64(dayDurationSecs))))
	}
	return Tempo{dayDurationSecs: dayDurationSecs, daysPerBeat: daysPerBeat}
}

// FastestDayDuration returns an integer clock rate targeting an approximately
// thirty-minute room while retaining a two-second lower bound for polling and
// settlement to observe progress.
func FastestDayDuration(totalDays int) int {
	if totalDays <= 0 {
		return 60
	}
	return max(FastestMinDayDurationSecs,
		int(math.Round(float64(FastestTargetDurationSecs)/float64(totalDays))))
}

func (t Tempo) Compressed() bool { return t.dayDurationSecs < 60 }

func (t Tempo) DaysPerBeat() int { return t.daysPerBeat }

// AgentActs schedules two of the five Agent seats per compressed beat. The
// rotating pair gives every personality equal long-run participation without
// emitting five simultaneous orders every few seconds.
func (t Tempo) AgentActs(day, slot, agentCount int) bool {
	if !t.Compressed() {
		return true
	}
	if day%t.daysPerBeat != 0 || agentCount <= 0 {
		return false
	}
	beat := day / t.daysPerBeat
	first := (beat * 2) % agentCount
	normalizedSlot := (slot - 1 + agentCount) % agentCount
	return normalizedSlot == first || normalizedSlot == (first+1)%agentCount
}

// CompressWorld keeps the complete price path untouched and trims only the
// player-facing narrative. This is intentionally applied after factor states
// and prices are generated: omitted headlines never alter market outcomes.
func (t Tempo) CompressWorld(sc *scenario.Scenario, world *World) *World {
	if !t.Compressed() {
		return world
	}
	news := t.compressNews(world.News)
	// Rebuild draft forum copy from the retained headlines so a post cannot
	// react to a story that the compressed feed intentionally omitted.
	forum := t.compressForum(ForumPosts(sc, world.Prices, news, world.Seed), news)
	return &World{
		ScenarioID: world.ScenarioID,
		Seed:       world.Seed,
		Prices:     world.Prices,
		News:       news,
		Forum:      forum,
	}
}

type rankedNews struct {
	event NewsEvent
	index int
	score float64
}

func (t Tempo) compressNews(events []NewsEvent) []NewsEvent {
	byBeat := map[int][]rankedNews{}
	for i, event := range events {
		beat := event.Day / t.daysPerBeat
		if event.Track == TrackNoise && beat%5 != 0 {
			continue
		}
		byBeat[beat] = append(byBeat[beat], rankedNews{
			event: event,
			index: i,
			score: narrativeScore(event),
		})
	}

	beats := make([]int, 0, len(byBeat))
	for beat := range byBeat {
		beats = append(beats, beat)
	}
	sort.Ints(beats)

	out := make([]NewsEvent, 0, len(beats)*compressedNewsPerBeat)
	for _, beat := range beats {
		candidates := byBeat[beat]
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].score != candidates[j].score {
				return candidates[i].score > candidates[j].score
			}
			// Prefer the latest recap within a beat, otherwise preserve source order.
			if candidates[i].event.Recap && candidates[j].event.Recap {
				return candidates[i].event.Day > candidates[j].event.Day
			}
			return candidates[i].index < candidates[j].index
		})
		if len(candidates) > compressedNewsPerBeat {
			candidates = candidates[:compressedNewsPerBeat]
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].event.Day != candidates[j].event.Day {
				return candidates[i].event.Day < candidates[j].event.Day
			}
			return candidates[i].index < candidates[j].index
		})
		for _, candidate := range candidates {
			out = append(out, candidate.event)
		}
	}
	return out
}

func narrativeScore(event NewsEvent) float64 {
	switch {
	case event.Track == TrackImpact && event.TrueShock != nil:
		return 500 + shockMagnitude(event.TrueShock)
	case event.Track == TrackHistorical && !event.Recap:
		return 400
	case event.Track == TrackImpact:
		return 300 + shockMagnitude(event.ReportShock)
	case event.Recap:
		return 200
	default:
		return 100
	}
}

func shockMagnitude(shock map[string]float64) float64 {
	var strongest float64
	for _, value := range shock {
		strongest = math.Max(strongest, math.Abs(value))
	}
	return strongest
}

func (t Tempo) compressForum(posts []ForumPost, news []NewsEvent) []ForumPost {
	newsDays := map[int]bool{}
	for _, event := range news {
		newsDays[event.Day] = true
	}
	byBeat := map[int][]ForumPost{}
	for _, post := range posts {
		beat := post.Day / t.daysPerBeat
		byBeat[beat] = append(byBeat[beat], post)
	}

	beats := make([]int, 0, len(byBeat))
	for beat := range byBeat {
		beats = append(beats, beat)
	}
	sort.Ints(beats)

	out := make([]ForumPost, 0, len(beats)*compressedForumPerBeat)
	for _, beat := range beats {
		candidates := byBeat[beat]
		matching := make([]ForumPost, 0, len(candidates))
		for _, post := range candidates {
			if newsDays[post.Day] {
				matching = append(matching, post)
			}
		}
		if len(matching) > 0 {
			candidates = matching
		}
		// Rotate the chosen voice instead of always retaining the first post.
		chosen := candidates[beat%len(candidates)]
		out = append(out, chosen)
	}
	return out
}
