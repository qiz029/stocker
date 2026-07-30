package httpapi

import (
	"net/http"

	"github.com/toddzheng/stocker/server/internal/store"
)

// handleHype (造势): paid manipulation that plants a real price shock.
// Response carries the fee, the regulatory outcome, and the resulting cash.
func (s *Server) handleHype(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	var req struct {
		InstrumentID string `json:"instrument_id"`
		Direction    string `json:"direction"`
		Tier         int    `json:"tier"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sc, err := store.LoadScenario(r.Context(), s.DB, room.ScenarioID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	res, err := store.Hype(r.Context(), s.DB, room, sc, userFrom(r).ID, s.Now(),
		req.InstrumentID, req.Direction, req.Tier)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fee_cents": res.FeeCents, "caught": res.Caught,
		"fine_cents": res.FineCents, "cash_cents": res.CashCents,
	})
}

// handleDebunk (辟谣/调查): paid investigation of one published news item.
// The verdict is private to this response (no numbers, direction only).
func (s *Server) handleDebunk(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	var req struct {
		NewsID int64 `json:"news_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	res, err := store.Debunk(r.Context(), s.DB, room, userFrom(r).ID, s.Now(), req.NewsID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"verdict": res.Verdict, "fee_cents": res.FeeCents, "cash_cents": res.CashCents,
	})
}

// handleIntel (内幕消息): a noisy peek at tomorrow's true shock on one
// instrument. The response never reveals whether the tip was corrupted.
func (s *Server) handleIntel(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	var req struct {
		InstrumentID string `json:"instrument_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sc, err := store.LoadScenario(r.Context(), s.DB, room.ScenarioID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	res, err := store.Intel(r.Context(), s.DB, room, sc, userFrom(r).ID, s.Now(), req.InstrumentID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	var strength any
	if res.Strength != "" {
		strength = res.Strength
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"outlook": res.Outlook, "strength": strength,
		"fee_cents": res.FeeCents, "cash_cents": res.CashCents,
	})
}
