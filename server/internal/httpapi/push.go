package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/toddzheng/stocker/server/internal/store"
)

const expoPushURL = "https://exp.host/--/api/v2/push/send"

// pushMessage is one Expo push notification payload.
type pushMessage struct {
	To    string `json:"to"`
	Title string `json:"title,omitempty"`
	Body  string `json:"body"`
}

// sendPush posts messages to the Expo push service. Failures are logged and
// never affect the calling request.
func sendPush(ctx context.Context, msgs []pushMessage) {
	if len(msgs) == 0 {
		return
	}
	payload, err := json.Marshal(msgs)
	if err != nil {
		log.Printf("push: marshal: %v", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, expoPushURL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("push: request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("push: send: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("push: expo push service returned %s", resp.Status)
	}
}

// notifyRoomMembers pushes to every human member of the room except
// excludeUserID. Callers invoke it in a goroutine — it does its own
// background DB read and never blocks the request.
func (s *Server) notifyRoomMembers(roomID, excludeUserID int64, title, bodyEn, bodyZh string) {
	tokens, err := store.PushTokensForRoom(context.Background(), s.DB, roomID, excludeUserID)
	if err != nil {
		log.Printf("push: tokens for room %d: %v", roomID, err)
		return
	}
	msgs := make([]pushMessage, 0, len(tokens))
	for _, tok := range tokens {
		body := bodyEn
		if tok.Lang == "zh" {
			body = bodyZh
		}
		msgs = append(msgs, pushMessage{To: tok.Token, Title: title, Body: body})
	}
	sendPush(context.Background(), msgs)
}

func (s *Server) handleAddPushToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
		Lang  string `json:"lang"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" || len(req.Token) > 256 {
		writeErr(w, http.StatusBadRequest, "invalid push token")
		return
	}
	if req.Lang != "zh" {
		req.Lang = "en"
	}
	if err := store.AddPushToken(r.Context(), s.DB, userFrom(r).ID, req.Token, req.Lang); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRemovePushToken(w http.ResponseWriter, r *http.Request) {
	// Accept the token as a query param (RN fetch DELETE bodies are awkward)
	// or as a JSON body.
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		var req struct {
			Token string `json:"token"`
		}
		if err := readJSON(r, &req); err == nil {
			token = strings.TrimSpace(req.Token)
		}
	}
	if token == "" {
		writeErr(w, http.StatusBadRequest, "invalid push token")
		return
	}
	if err := store.RemovePushToken(r.Context(), s.DB, userFrom(r).ID, token); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
