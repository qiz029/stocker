// Package httpapi is the REST layer. Handlers stay thin: decode, call
// store, map errors. Blind-box rule: no response may carry news track,
// shock vectors, or real instrument identity except the reveal endpoint.
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/store"
)

type Server struct {
	DB *pgxpool.Pool
	// Now is the wall clock; tests override it to steer the deterministic
	// timeline. Everything time-dependent must go through s.Now().
	Now func() time.Time
}

func NewServer(db *pgxpool.Pool) *Server {
	return &Server{DB: db, Now: time.Now}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Post("/api/register", s.handleRegister)
	r.Post("/api/login", s.handleLogin)
	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Post("/api/logout", s.handleLogout)
		r.Get("/api/me", s.handleMe)
		r.Get("/api/scenarios", s.handleScenarios)
		r.Post("/api/rooms", s.handleCreateRoom)
		r.Post("/api/rooms/join", s.handleJoinRoom)
		r.Get("/api/rooms", s.handleMyRooms)
		r.Post("/api/rooms/{roomID}/start", s.handleStartRoom)
		r.Get("/api/rooms/{roomID}", s.handleRoomState)
		r.Get("/api/rooms/{roomID}/prices/{instrumentID}", s.handlePrices)
		r.Get("/api/rooms/{roomID}/news", s.handleNews)
		r.Get("/api/rooms/{roomID}/events", s.handleEvents)
		r.Post("/api/rooms/{roomID}/orders", s.handlePlaceOrder)
		r.Delete("/api/rooms/{roomID}/orders/{orderID}", s.handleCancelOrder)
		r.Get("/api/rooms/{roomID}/portfolio", s.handlePortfolio)
		r.Get("/api/rooms/{roomID}/reveal", s.handleReveal)
		r.Post("/api/rooms/{roomID}/chat", s.handlePostChat)
		r.Get("/api/rooms/{roomID}/chat", s.handleGetChat)
		r.Get("/api/rooms/{roomID}/trades", s.handleMyTrades)
	})
	return r
}

type ctxKey int

const userKey ctxKey = 0

func userFrom(r *http.Request) *store.User {
	u, _ := r.Context().Value(userKey).(*store.User)
	return u
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ck, err := r.Cookie(sessionCookie)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "not logged in")
			return
		}
		u, err := store.UserBySession(r.Context(), s.DB, ck.Value, s.Now())
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "not logged in")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}
