package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
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
	links := u.SocialLinks
	if links == nil {
		links = map[string]string{}
	}
	return map[string]any{
		"id": u.ID, "username": u.Username, "display_name": u.DisplayName,
		"avatar_id": u.AvatarID, "email": u.Email, "description": u.Description,
		"social_links": links, "profile_complete": u.ProfileComplete(),
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

type profileRequest struct {
	DisplayName string             `json:"display_name"`
	AvatarID    string             `json:"avatar_id"`
	Email       *string            `json:"email"`
	Description *string            `json:"description"`
	SocialLinks *map[string]string `json:"social_links"`
}

var allowedSocialLinks = map[string]bool{
	"website": true, "x": true, "github": true, "linkedin": true,
}

func validateProfileRequest(req *profileRequest) error {
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if utf8.RuneCountInString(req.DisplayName) < 2 || utf8.RuneCountInString(req.DisplayName) > 24 || !allowedAvatars[req.AvatarID] {
		return fmt.Errorf("alias must be 2-24 characters and a valid avatar is required")
	}
	if req.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		if email != "" {
			parsed, err := mail.ParseAddress(email)
			if err != nil || parsed.Address != email || len(email) > 254 {
				return fmt.Errorf("invalid email address")
			}
		}
		req.Email = &email
	}
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		if utf8.RuneCountInString(description) > 500 {
			return fmt.Errorf("description must be at most 500 characters")
		}
		req.Description = &description
	}
	if req.SocialLinks != nil {
		normalized := make(map[string]string, len(*req.SocialLinks))
		for network, raw := range *req.SocialLinks {
			if !allowedSocialLinks[network] {
				return fmt.Errorf("unsupported social link: %s", network)
			}
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			parsed, err := url.ParseRequestURI(value)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || len(value) > 200 {
				return fmt.Errorf("social links must be valid http or https URLs")
			}
			normalized[network] = value
		}
		req.SocialLinks = &normalized
	}
	return nil
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	var req profileRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	current := userFrom(r)
	if req.Email == nil {
		req.Email = &current.Email
	}
	if req.Description == nil {
		req.Description = &current.Description
	}
	if req.SocialLinks == nil {
		req.SocialLinks = &current.SocialLinks
	}
	if err := validateProfileRequest(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := store.UpdateUserProfile(r.Context(), s.DB, current.ID, req.DisplayName, req.AvatarID, *req.Email, *req.Description, *req.SocialLinks)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userJSON(u))
}

func (s *Server) handleUpdatePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.NewPassword) < 8 || len(req.NewPassword) > 72 {
		writeErr(w, http.StatusBadRequest, "new password must be 8-72 characters")
		return
	}
	current := userFrom(r)
	if bcrypt.CompareHashAndPassword([]byte(current.PasswordHash), []byte(req.CurrentPassword)) != nil {
		writeErr(w, http.StatusBadRequest, "current password is incorrect")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if err := store.UpdateUserPassword(r.Context(), s.DB, current.ID, string(hash)); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password updated"})
}
