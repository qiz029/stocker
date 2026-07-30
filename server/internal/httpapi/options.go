package httpapi

import (
	"net/http"

	"github.com/toddzheng/stocker/server/internal/store"
)

// handleListOptions serves the tradeable option chain for one instrument,
// priced at the current day's Black-Scholes mark. Response: a bare array
// [{option_id, kind, strike, expiry_day, price}] sorted by expiry then
// strike; an unknown instrument yields an empty array.
func (s *Server) handleListOptions(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if room.StartedAt == nil {
		writeErr(w, http.StatusBadRequest, store.ErrNotStarted.Error())
		return
	}
	instrumentID := r.URL.Query().Get("instrument_id")
	if instrumentID == "" {
		writeErr(w, http.StatusBadRequest, "instrument_id is required")
		return
	}
	curDay, _, err := store.SettleRoom(r.Context(), s.DB, room, s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	opts, prices, err := store.ListOptions(r.Context(), s.DB, room.ID, instrumentID, curDay)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	items := []map[string]any{}
	for i, o := range opts {
		items = append(items, map[string]any{
			"option_id": o.ID, "kind": o.Kind, "strike": o.Strike,
			"expiry_day": o.ExpiryDay, "price": prices[i],
		})
	}
	writeJSON(w, http.StatusOK, items)
}

// handleOptionOrder buys or sells-to-close option contracts at the current
// BS mark, effective immediately. Body: {"option_id": n, "action":
// "buy"|"sell", "contracts": x}. Response: the fill and the cash after it.
func (s *Server) handleOptionOrder(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	var req struct {
		OptionID  int64   `json:"option_id"`
		Action    string  `json:"action"`
		Contracts float64 `json:"contracts"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var fill *store.OptionFill
	var err error
	switch req.Action {
	case "buy":
		fill, err = store.BuyOption(r.Context(), s.DB, room, userFrom(r).ID, req.OptionID, req.Contracts, s.Now())
	case "sell":
		fill, err = store.SellOption(r.Context(), s.DB, room, userFrom(r).ID, req.OptionID, req.Contracts, s.Now())
	default:
		writeErr(w, http.StatusBadRequest, "action must be buy or sell")
		return
	}
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"action": fill.Action, "contracts": fill.Contracts, "price": fill.Price,
		"amount_cents": fill.AmountCents, "cash_cents": fill.CashCents,
	})
}
