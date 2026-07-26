package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/toddzheng/stocker/server/internal/store"
)

func (s *Server) handlePlaceOrder(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	var req struct {
		InstrumentID string  `json:"instrument_id"`
		Side         string  `json:"side"`
		AmountCents  int64   `json:"amount_cents"`
		Shares       float64 `json:"shares"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	order, err := store.PlaceOrder(r.Context(), s.DB, room, userFrom(r).ID, s.Now(), store.OrderReq{
		InstrumentID: req.InstrumentID, Side: req.Side,
		AmountCents: req.AmountCents, Shares: req.Shares,
	})
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": order.ID, "instrument_id": order.InstrumentID, "side": order.Side,
		"amount_cents": order.AmountCents, "shares": order.Shares,
		"exec_day": order.ExecDay, "status": order.Status,
	})
}

func (s *Server) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	orderID, err := strconv.ParseInt(chi.URLParam(r, "orderID"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no such order")
		return
	}
	if err := store.CancelOrder(r.Context(), s.DB, room, userFrom(r).ID, orderID, s.Now()); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Server) handlePortfolio(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	// Lobby: nothing to value yet — just the untouched cash.
	if room.StartedAt == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"cash_cents": store.InitialCashCents, "total_cents": store.InitialCashCents,
			"positions": []any{}, "pending": []any{},
		})
		return
	}
	curDay, _, err := store.SettleRoom(r.Context(), s.DB, room, s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	p, err := store.GetPortfolio(r.Context(), s.DB, room, userFrom(r).ID, curDay)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	positions := []map[string]any{}
	for _, pos := range p.Positions {
		positions = append(positions, map[string]any{
			"instrument_id": pos.InstrumentID, "shares": pos.Shares,
			"close": pos.Close, "value_cents": pos.ValueCents,
		})
	}
	pending := []map[string]any{}
	for _, o := range p.Pending {
		pending = append(pending, map[string]any{
			"id": o.ID, "instrument_id": o.InstrumentID, "side": o.Side,
			"amount_cents": o.AmountCents, "shares": o.Shares, "exec_day": o.ExecDay,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cash_cents": p.CashCents, "total_cents": p.TotalCents,
		"positions": positions, "pending": pending,
	})
}
