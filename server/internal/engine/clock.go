package engine

import "time"

// CurrentDay maps wall-clock time to the historical trading-day index of a
// room. Callers guarantee now >= startedAt. When the scenario is exhausted
// the last day is returned with ended=true.
func CurrentDay(startedAt time.Time, dayDuration time.Duration, totalDays int, now time.Time) (int, bool) {
	day := int(now.Sub(startedAt) / dayDuration)
	if day >= totalDays {
		return totalDays - 1, true
	}
	return day, false
}
