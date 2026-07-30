package store

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/toddzheng/stocker/server/internal/scenario"
)

// Accuracy aggregates one media outlet's track record: Reports is the
// number of its observable impact-track reports, Hits how often the
// reported direction matched the realized price move.
type Accuracy struct {
	Reports int `json:"reports"`
	Hits    int `json:"hits"`
}

// accuracyObserveDays is how many days after publication a report's
// outcome is measured over (close-to-close, clipped at the current day).
const accuracyObserveDays = 3

// MediaAccuracy computes per-media 应验率 for a room as of curDay. Only
// impact-track reports published on day <= curDay-2 are scored, so the
// following-days outcome window is (at least partially) observable from
// already-public room_prices. The report_shock vectors themselves never
// leave this function — only their dominant factor's sign is compared
// against realized moves, and only aggregate counts are returned.
//
// Target mapping: "IDIO:<instID>" → that instrument; "MKT" → the
// scenario's market_proxy (equal-weighted basket fallback, mirroring the
// loan rate); a sector factor → equal-weighted mean of the instruments
// carrying that sector in their beta map (sector membership is not
// persisted anywhere else; rows targeting a member-less factor are
// skipped).
func MediaAccuracy(ctx context.Context, q Querier, roomID int64, curDay int) (map[string]Accuracy, error) {
	out := map[string]Accuracy{}
	if curDay < 2 {
		return out, nil
	}
	var scenarioID string
	if err := q.QueryRow(ctx,
		`SELECT scenario_id FROM rooms WHERE id = $1`, roomID).Scan(&scenarioID); err != nil {
		return nil, err
	}
	sc, err := LoadScenario(ctx, q, scenarioID)
	if err != nil {
		return nil, err
	}
	sectorKind := map[string]bool{}
	for _, f := range sc.Factors {
		if f.Kind == scenario.KindSector {
			sectorKind[f.ID] = true
		}
	}
	sectorMembers := map[string][]string{}
	for _, inst := range sc.Instruments {
		for fid, b := range inst.Beta {
			if sectorKind[fid] && b != 0 {
				sectorMembers[fid] = append(sectorMembers[fid], inst.ID)
			}
		}
	}

	// Public prices up to the current day, closes indexed by day.
	lastDay := curDay
	priceRows, err := q.Query(ctx, `
		SELECT instrument_id, day, close FROM room_prices
		WHERE room_id = $1 AND day <= $2`, roomID, lastDay)
	if err != nil {
		return nil, err
	}
	closes := map[string][]float64{}
	for priceRows.Next() {
		var inst string
		var d int
		var c float64
		if err := priceRows.Scan(&inst, &d, &c); err != nil {
			priceRows.Close()
			return nil, err
		}
		s := closes[inst]
		if s == nil {
			s = make([]float64, lastDay+1)
			closes[inst] = s
		}
		s[d] = c
	}
	priceRows.Close()
	if err := priceRows.Err(); err != nil {
		return nil, err
	}

	// move is the realized close-to-close log return of one instrument
	// from day d to day e (false when either close is missing/nonpositive).
	move := func(inst string, d, e int) (float64, bool) {
		s, ok := closes[inst]
		if !ok || d >= len(s) || e >= len(s) || s[d] <= 0 || s[e] <= 0 {
			return 0, false
		}
		return math.Log(s[e] / s[d]), true
	}
	basketMove := func(members []string, d, e int) (float64, bool) {
		var sum float64
		n := 0
		for _, inst := range members {
			if r, ok := move(inst, d, e); ok {
				sum += r
				n++
			}
		}
		if n == 0 {
			return 0, false
		}
		return sum / float64(n), true
	}
	allIDs := make([]string, 0, len(sc.Instruments))
	for _, inst := range sc.Instruments {
		allIDs = append(allIDs, inst.ID)
	}
	targetMove := func(factor string, d, e int) (float64, bool) {
		switch {
		case strings.HasPrefix(factor, "IDIO:"):
			return move(factor[len("IDIO:"):], d, e)
		case factor == "MKT":
			if sc.MarketProxy != "" {
				if r, ok := move(sc.MarketProxy, d, e); ok {
					return r, true
				}
			}
			return basketMove(allIDs, d, e)
		case sectorKind[factor]:
			members := sectorMembers[factor]
			if len(members) == 0 {
				return 0, false // membership not derivable — skip
			}
			return basketMove(members, d, e)
		default:
			return 0, false
		}
	}

	newsRows, err := q.Query(ctx, `
		SELECT day, media_id, report_shock FROM room_news
		WHERE room_id = $1 AND track = 'impact' AND day <= $2 AND report_shock IS NOT NULL`,
		roomID, curDay-2)
	if err != nil {
		return nil, err
	}
	defer newsRows.Close()
	for newsRows.Next() {
		var day int
		var mediaID string
		var reportJSON []byte
		if err := newsRows.Scan(&day, &mediaID, &reportJSON); err != nil {
			return nil, err
		}
		var report map[string]float64
		if err := json.Unmarshal(reportJSON, &report); err != nil || len(report) == 0 {
			continue
		}
		// Dominant factor only; its sign is the reported direction.
		var top string
		var mag float64
		for f, v := range report {
			if math.Abs(v) > math.Abs(mag) {
				top, mag = f, v
			}
		}
		end := day + accuracyObserveDays
		if end > curDay {
			end = curDay
		}
		ret, ok := targetMove(top, day, end)
		if !ok {
			continue
		}
		acc := out[mediaID]
		acc.Reports++
		if ret > 0 == (mag > 0) && ret != 0 {
			acc.Hits++
		}
		out[mediaID] = acc
	}
	return out, newsRows.Err()
}
