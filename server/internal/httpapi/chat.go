package httpapi

import (
	"net/http"

	"github.com/toddzheng/stocker/server/internal/store"
)

func (s *Server) handlePostChat(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	day := 0
	if room.StartedAt != nil {
		if d, _, err := room.CurrentDay(s.Now()); err == nil {
			day = d
		}
	}
	id, err := store.PostChat(r.Context(), s.DB, room, userFrom(r).ID, day, req.Text)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) handleGetChat(w http.ResponseWriter, r *http.Request) {
	room, _, ok := s.roomForViewer(w, r)
	if !ok {
		return
	}
	msgs, err := store.ChatSince(r.Context(), s.DB, room.ID, afterParam(r), newsPageLimit)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	items := []map[string]any{}
	for _, m := range msgs {
		items = append(items, map[string]any{
			"id": m.ID, "username": m.Username, "is_agent": m.IsAgent,
			"avatar_id": m.AvatarID, "is_me": m.UserID == userFrom(r).ID,
			"day": m.Day, "text": m.Text, "text_en": m.TextEn,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
