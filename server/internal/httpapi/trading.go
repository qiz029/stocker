package httpapi

import (
	"math"
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
			"debt_cents": 0, "max_debt_cents": store.MaxDebtCents,
			"interest_rate_annual_bp": 300, "bankrupt": false,
			"positions": []any{}, "options": []any{}, "pending": []any{},
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
	rate, err := store.CurrentAnnualRate(r.Context(), s.DB, room.ID, curDay)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	positions := []map[string]any{}
	for _, pos := range p.Positions {
		positions = append(positions, map[string]any{
			"instrument_id": pos.InstrumentID, "shares": pos.Shares,
			"close": pos.Close, "value_cents": pos.ValueCents,
			"avg_cost": pos.AvgCost, "pnl_cents": pos.PnLCents, "pnl_pct": pos.PnLPct,
		})
	}
	pending := []map[string]any{}
	for _, o := range p.Pending {
		pending = append(pending, map[string]any{
			"id": o.ID, "instrument_id": o.InstrumentID, "side": o.Side,
			"amount_cents": o.AmountCents, "shares": o.Shares, "exec_day": o.ExecDay,
		})
	}
	options := []map[string]any{}
	for _, o := range p.Options {
		options = append(options, map[string]any{
			"option_id": o.OptionID, "instrument_id": o.InstrumentID,
			"kind": o.Kind, "strike": o.Strike, "expiry_day": o.ExpiryDay,
			"contracts": o.Contracts, "price": o.Price,
			"value_cents": o.ValueCents, "avg_cost": o.AvgCost,
			"pnl_cents": o.PnLCents, "pnl_pct": o.PnLPct,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cash_cents": p.CashCents, "total_cents": p.TotalCents,
		"debt_cents": p.DebtCents, "max_debt_cents": store.MaxDebtCents,
		"interest_rate_annual_bp": int(math.Round(rate * 10000)),
		"bankrupt":  p.Bankrupt,
		"positions": positions, "options": options, "pending": pending,
	})
}

func (s *Server) handleMyTrades(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if room.StartedAt != nil {
		if _, _, err := store.SettleRoom(r.Context(), s.DB, room, s.Now()); err != nil {
			s.storeErr(w, err)
			return
		}
	}
	trades, err := store.TradesForUser(r.Context(), s.DB, room.ID, userFrom(r).ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	items := []map[string]any{}
	for _, t := range trades {
		items = append(items, map[string]any{
			"instrument_id": t.InstrumentID, "side": t.Side, "day": t.Day,
			"price": t.Price, "shares": t.Shares, "amount_cents": t.AmountCents,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
