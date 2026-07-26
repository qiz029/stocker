package engine

import (
	"fmt"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

// EvolveFactorStates advances the dual-timescale factor state day by day.
// An event published on day d first affects states on day d+1.
//
// Returns an error if any TrackImpact event's TrueShock names a factor ID
// that isn't in sc.FactorIDs() — an unrecognized ID must never be allowed
// to silently alias factor index 0 (see idx below).
func EvolveFactorStates(sc *scenario.Scenario, events []NewsEvent) ([][]float64, error) {
	ids := sc.FactorIDs()
	idx := make(map[string]int, len(ids))
	for i, id := range ids {
		idx[id] = i
	}
	byDay := map[int][]NewsEvent{}
	for _, ev := range events {
		if ev.Track == TrackImpact {
			byDay[ev.Day] = append(byDay[ev.Day], ev)
		}
	}
	fast := make([]float64, len(ids))
	slow := make([]float64, len(ids))
	states := make([][]float64, sc.Days)
	// Note: an impact event published on the last scenario day (sc.Days-1)
	// would take effect on day sc.Days, which is past the end of the loop
	// below — see the comment on GenerateShockTimeline's final-day shocks.
	for d := 0; d < sc.Days; d++ {
		for i := range fast {
			fast[i] *= lamFast
			slow[i] *= lamSlow
		}
		for _, ev := range byDay[d-1] { // published after close of d-1
			for f, v := range ev.TrueShock {
				i, ok := idx[f]
				if !ok {
					return nil, fmt.Errorf("evolve factor states: event on day %d references unknown factor id %q (not in scenario %q)", ev.Day, f, sc.ID)
				}
				fast[i] += v * fracFast
				slow[i] += v * (1 - fracFast)
			}
		}
		row := make([]float64, len(ids))
		for i := range row {
			row[i] = fast[i] + slow[i]
		}
		states[d] = row
	}
	return states, nil
}
