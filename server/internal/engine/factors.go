package engine

import "github.com/toddzheng/stocker/server/internal/scenario"

// EvolveFactorStates advances the dual-timescale factor state day by day.
// An event published on day d first affects states on day d+1.
func EvolveFactorStates(sc *scenario.Scenario, events []NewsEvent) [][]float64 {
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
	for d := 0; d < sc.Days; d++ {
		for i := range fast {
			fast[i] *= lamFast
			slow[i] *= lamSlow
		}
		for _, ev := range byDay[d-1] { // published after close of d-1
			for f, v := range ev.TrueShock {
				fast[idx[f]] += v * fracFast
				slow[idx[f]] += v * (1 - fracFast)
			}
		}
		row := make([]float64, len(ids))
		for i := range row {
			row[i] = fast[i] + slow[i]
		}
		states[d] = row
	}
	return states
}
