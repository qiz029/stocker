package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/toddzheng/stocker/server/internal/store"
)

const (
	sessionCookie = "stocker_session"
	sessionTTL    = 30 * 24 * 3600 // seconds
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)

var allowedAvatars = map[string]bool{
	"bull": true, "bear": true, "fox": true, "owl": true,
	"shark": true, "tiger": true, "rocket": true, "diamond": true,
}

func userJSON(u *store.User) map[string]any {
	return map[string]any{
		"id": u.ID, "username": u.Username, "display_name": u.DisplayName,
		"avatar_id": u.AvatarID, "profile_complete": u.ProfileComplete(),
	}
}

// dummyHash burns the same bcrypt cost on unknown usernames as on wrong
// passwords, so login timing can't be used to enumerate accounts.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("stocker-dummy-password"), bcrypt.DefaultCost)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *store.User) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	expires := s.Now().Add(time.Duration(sessionTTL) * time.Second)
	if err := store.CreateSession(r.Context(), s.DB, u.ID, token, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: sessionTTL,
	})
	return nil
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !usernameRe.MatchString(req.Username) {
		writeErr(w, http.StatusBadRequest, "username must be 3-32 chars of letters, digits, underscore")
		return
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	u, err := store.CreateUser(r.Context(), s.DB, req.Username, string(hash))
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if err := s.startSession(w, r, u); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	u, err := store.GetUserByUsername(r.Context(), s.DB, req.Username)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
		writeErr(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		writeErr(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err := s.startSession(w, r, u); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie(sessionCookie); err == nil {
		_ = store.DeleteSession(r.Context(), s.DB, ck.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	writeJSON(w, http.StatusOK, userJSON(u))
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName string `json:"display_name"`
		AvatarID    string `json:"avatar_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if utf8.RuneCountInString(req.DisplayName) < 2 || utf8.RuneCountInString(req.DisplayName) > 24 || !allowedAvatars[req.AvatarID] {
		writeErr(w, http.StatusBadRequest, "display name must be 2-24 characters and a valid avatar is required")
		return
	}
	u, err := store.UpdateUserProfile(r.Context(), s.DB, userFrom(r).ID, req.DisplayName, req.AvatarID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}
