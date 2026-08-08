package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/toddzheng/stocker/server/internal/store"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(dst)
}

// storeErr maps store sentinel errors onto HTTP statuses.
func (s *Server) storeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrUsernameTaken),
		errors.Is(err, store.ErrAliasTaken),
		errors.Is(err, store.ErrEmailTaken),
		errors.Is(err, store.ErrAlreadyJoined),
		errors.Is(err, store.ErrRoomFull),
		errors.Is(err, store.ErrPublicJoinClosed),
		errors.Is(err, store.ErrRoomEnded),
		errors.Is(err, store.ErrNotCancellable):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrCannotStart):
		writeErr(w, http.StatusForbidden, err.Error())
	case errors.Is(err, store.ErrBadProfile):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, store.ErrBadDayDuration),
		errors.Is(err, store.ErrBadRoomName),
		errors.Is(err, store.ErrBadVisibility),
		errors.Is(err, store.ErrBadOrder),
		errors.Is(err, store.ErrUnknownInstrument),
		errors.Is(err, store.ErrInsufficientCash),
		errors.Is(err, store.ErrInsufficientShares),
		errors.Is(err, store.ErrRoomNotRunning),
		errors.Is(err, store.ErrNotStarted),
		errors.Is(err, store.ErrBadChatMessage),
		errors.Is(err, store.ErrPlayerBankrupt),
		errors.Is(err, store.ErrBadLoanAmount),
		errors.Is(err, store.ErrDebtCapExceeded),
		errors.Is(err, store.ErrBadOptionOrder),
		errors.Is(err, store.ErrInsufficientContracts),
		errors.Is(err, store.ErrBadAction),
		errors.Is(err, store.ErrActionLimit),
		errors.Is(err, store.ErrAlreadyDisputed):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("internal error: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
}
