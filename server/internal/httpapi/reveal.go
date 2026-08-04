package httpapi

import (
	"net/http"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/store"
)

// handleReveal is the blind box's opening ceremony (spec §2.4): only after
// the scenario's final day has passed may real identities and everyone's
// trades be shown. Until then it answers 409.
func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if room.StartedAt == nil {
		writeErr(w, http.StatusConflict, "game not finished")
		return
	}
	curDay, ended, err := store.SettleRoom(r.Context(), s.DB, room, s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if !ended {
		writeErr(w, http.StatusConflict, "game not finished")
		return
	}

	instRows, err := s.DB.Query(r.Context(), `
		SELECT i.id, i.alias, i.real_name, i.aliases
		FROM instruments i JOIN rooms rm ON rm.scenario_id = i.scenario_id
		WHERE rm.id = $1 ORDER BY i.ord`, room.ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer instRows.Close()
	instruments := []map[string]any{}
	for instRows.Next() {
		var id, alias, realName string
		var aliases []string // nil when column is NULL
		if err := instRows.Scan(&id, &alias, &realName, &aliases); err != nil {
			s.storeErr(w, err)
			return
		}
		// Same per-room name the player saw all game.
		alias = engine.ResolveAlias(room.Seed, id, alias, aliases)
		instruments = append(instruments, map[string]any{
			"id": id, "alias": alias, "real_name": realName,
		})
	}
	if err := instRows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}

	tradeRows, err := s.DB.Query(r.Context(), `
		SELECT COALESCE(u.agent_name, u.username), u.is_agent,
			t.instrument_id, t.side, t.day, t.price, t.shares, t.amount_cents
		FROM trades t JOIN users u ON u.id = t.user_id
		WHERE t.room_id = $1 ORDER BY t.day, t.id`, room.ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer tradeRows.Close()
	trades := []map[string]any{}
	for tradeRows.Next() {
		var username, instrumentID, side string
		var isAgent bool
		var day int
		var price, shares float64
		var amountCents int64
		if err := tradeRows.Scan(&username, &isAgent, &instrumentID, &side, &day, &price, &shares, &amountCents); err != nil {
			s.storeErr(w, err)
			return
		}
		trades = append(trades, map[string]any{
			"username": username, "is_agent": isAgent, "instrument_id": instrumentID, "side": side,
			"day": day, "price": price, "shares": shares, "amount_cents": amountCents,
		})
	}
	if err := tradeRows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}

	lbRows, err := store.Leaderboard(r.Context(), s.DB, room, curDay)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	leaderboard := []map[string]any{}
	for _, lr := range lbRows {
		leaderboard = append(leaderboard, map[string]any{
			"username":    lr.Username,
			"is_agent":    lr.IsAgent,
			"total_cents": lr.TotalCents,
			"return_pct":  float64(lr.TotalCents-store.InitialCashCents) / float64(store.InitialCashCents),
			"late_join":   lr.JoinedDay > 0,
			"bankrupt":    lr.Bankrupt,
		})
	}

	var realPeriod string
	if err := s.DB.QueryRow(r.Context(), `
		SELECT s.real_period FROM scenarios s JOIN rooms rm ON rm.scenario_id = s.id
		WHERE rm.id = $1`, room.ID).Scan(&realPeriod); err != nil {
		s.storeErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"instruments": instruments,
		"trades":      trades,
		"leaderboard": leaderboard,
		"real_period": realPeriod,
	})
}
