package httpapi

import (
	"net/http"

	"github.com/toddzheng/stocker/server/internal/store"
)

// handleLoan borrows against (or repays) the room's credit line. Body:
// {"action": "borrow"|"repay", "amount_cents": n}. Interest accrues
// lazily inside settlement, so the debt this reports is current.
func (s *Server) handleLoan(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	var req struct {
		Action      string `json:"action"`
		AmountCents int64  `json:"amount_cents"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var state *store.LoanState
	var err error
	switch req.Action {
	case "borrow":
		state, err = store.Borrow(r.Context(), s.DB, room, userFrom(r).ID, req.AmountCents, s.Now())
	case "repay":
		state, err = store.Repay(r.Context(), s.DB, room, userFrom(r).ID, req.AmountCents, s.Now())
	default:
		writeErr(w, http.StatusBadRequest, "action must be borrow or repay")
		return
	}
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"action": req.Action, "amount_cents": req.AmountCents,
		"cash_cents": state.CashCents, "debt_cents": state.DebtCents,
		"max_debt_cents": store.MaxDebtCents,
	})
}
