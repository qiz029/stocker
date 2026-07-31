package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/store"
)

const newsPageLimit = 200

func roomJSON(room *store.Room, curDay int, ended, started bool, userID int64) map[string]any {
	m := map[string]any{
		"id":                room.ID,
		"invite_code":       room.InviteCode,
		"scenario_id":       room.ScenarioID,
		"days":              room.Days,
		"status":            room.Status,
		"day_duration_secs": room.DayDurationSecs,
		"is_host":           room.HostUserID == userID,
	}
	if room.StartedAt != nil {
		m["started_at"] = room.StartedAt.UTC().Format(time.RFC3339)
	}
	if started {
		m["current_day"] = curDay
		m["ended"] = ended
	}
	return m
}

// roomForMember loads the {roomID} route param and enforces membership.
func (s *Server) roomForMember(w http.ResponseWriter, r *http.Request) (*store.Room, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "roomID"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no such room")
		return nil, false
	}
	room, err := store.GetRoom(r.Context(), s.DB, id)
	if err != nil {
		s.storeErr(w, err)
		return nil, false
	}
	member, err := store.IsMember(r.Context(), s.DB, room.ID, userFrom(r).ID)
	if err != nil {
		s.storeErr(w, err)
		return nil, false
	}
	if !member {
		writeErr(w, http.StatusForbidden, "not a member of this room")
		return nil, false
	}
	return room, true
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScenarioID      string `json:"scenario_id"`
		DayDurationSecs int    `json:"day_duration_secs"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sc, err := store.LoadScenario(r.Context(), s.DB, req.ScenarioID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	room, err := store.CreateRoom(r.Context(), s.DB, sc, userFrom(r).ID, req.DayDurationSecs, s.CopyFiller)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roomJSON(room, 0, false, false, userFrom(r).ID))
}

func (s *Server) handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InviteCode string `json:"invite_code"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	room, err := store.GetRoomByInvite(r.Context(), s.DB, req.InviteCode)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if _, err := store.JoinRoom(r.Context(), s.DB, room, userFrom(r).ID, s.Now()); err != nil {
		s.storeErr(w, err)
		return
	}
	day, ended, started := s.roomProgress(room)
	writeJSON(w, http.StatusOK, roomJSON(room, day, ended, started, userFrom(r).ID))
}

// roomProgress computes clock state without settling (cheap display path).
func (s *Server) roomProgress(room *store.Room) (day int, ended, started bool) {
	if room.StartedAt == nil {
		return 0, false, false
	}
	day, ended, err := room.CurrentDay(s.Now())
	if err != nil {
		return 0, false, false
	}
	return day, ended, true
}

func (s *Server) handleStartRoom(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	room, err := store.StartRoom(r.Context(), s.DB, room.ID, userFrom(r).ID, s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roomJSON(room, 0, false, true, userFrom(r).ID))
}

func (s *Server) handleMyRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := store.ListRoomsForUser(r.Context(), s.DB, userFrom(r).ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rooms))
	for i := range rooms {
		day, ended, started := s.roomProgress(&rooms[i])
		out = append(out, roomJSON(&rooms[i], day, ended, started, userFrom(r).ID))
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": out})
}

func (s *Server) handleRoomState(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	curDay, ended, err := store.SettleRoom(r.Context(), s.DB, room, s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	started := room.StartedAt != nil

	// Blind-box instrument cards: alias + description only. The alias is
	// resolved per room from the room's seed (candidate set in i.aliases;
	// NULL there falls back to the plain alias column).
	instRows, err := s.DB.Query(r.Context(), `
		SELECT i.id, i.alias, i.descr, i.profile, i.aliases, i.descr_en, i.profile_en
		FROM instruments i JOIN rooms rm ON rm.scenario_id = i.scenario_id
		WHERE rm.id = $1 ORDER BY i.ord`, room.ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer instRows.Close()
	instruments := []map[string]any{}
	for instRows.Next() {
		var id, alias, desc, descEn string
		var profile map[string]any   // nil when column is NULL
		var profileEn map[string]any // nil when column is NULL
		var aliases []string         // nil when column is NULL
		if err := instRows.Scan(&id, &alias, &desc, &profile, &aliases, &descEn, &profileEn); err != nil {
			s.storeErr(w, err)
			return
		}
		alias = engine.ResolveAlias(room.Seed, id, alias, aliases)
		instruments = append(instruments, map[string]any{
			"id": id, "alias": alias, "desc": desc, "profile": profile,
			"desc_en": descEn, "profile_en": profileEn,
		})
	}
	if err := instRows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}

	quotes := []map[string]any{}
	if started {
		prevDay := curDay - 1
		if prevDay < 0 {
			prevDay = 0
		}
		quoteRows, err := s.DB.Query(r.Context(), `
			SELECT cur.instrument_id, cur.close, prev.close
			FROM room_prices cur
			JOIN room_prices prev ON prev.room_id = cur.room_id
				AND prev.instrument_id = cur.instrument_id AND prev.day = $3
			WHERE cur.room_id = $1 AND cur.day = $2
			ORDER BY cur.instrument_id`, room.ID, curDay, prevDay)
		if err != nil {
			s.storeErr(w, err)
			return
		}
		defer quoteRows.Close()
		for quoteRows.Next() {
			var id string
			var cl, prev float64
			if err := quoteRows.Scan(&id, &cl, &prev); err != nil {
				s.storeErr(w, err)
				return
			}
			quotes = append(quotes, map[string]any{"instrument_id": id, "close": cl, "prev_close": prev})
		}
		if err := quoteRows.Err(); err != nil {
			s.storeErr(w, err)
			return
		}
	}

	leaderboard := []map[string]any{}
	if started {
		rows, err := store.Leaderboard(r.Context(), s.DB, room, curDay)
		if err != nil {
			s.storeErr(w, err)
			return
		}
		for _, lr := range rows {
			leaderboard = append(leaderboard, map[string]any{
				"username":    lr.Username,
				"total_cents": lr.TotalCents,
				"return_pct":  float64(lr.TotalCents-store.InitialCashCents) / float64(store.InitialCashCents),
				"late_join":   lr.JoinedDay > 0,
				"bankrupt":    lr.Bankrupt,
				"curve":       lr.Curve,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"room":        roomJSON(room, curDay, ended, started, userFrom(r).ID),
		"instruments": instruments,
		"quotes":      quotes,
		"leaderboard": leaderboard,
	})
}

func (s *Server) handlePrices(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if room.StartedAt == nil {
		writeErr(w, http.StatusBadRequest, store.ErrNotStarted.Error())
		return
	}
	curDay, _, err := room.CurrentDay(s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	instrumentID := chi.URLParam(r, "instrumentID")
	rows, err := s.DB.Query(r.Context(), `
		SELECT open, high, low, close FROM room_prices
		WHERE room_id = $1 AND instrument_id = $2 AND day <= $3
		ORDER BY day`, room.ID, instrumentID, curDay)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer rows.Close()
	days := []map[string]float64{}
	for rows.Next() {
		var o, h, l, c float64
		if err := rows.Scan(&o, &h, &l, &c); err != nil {
			s.storeErr(w, err)
			return
		}
		days = append(days, map[string]float64{"open": o, "high": h, "low": l, "close": c})
	}
	if err := rows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}
	if len(days) == 0 {
		writeErr(w, http.StatusNotFound, "no such instrument")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days})
}

func afterParam(r *http.Request) int64 {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	return after
}

// handleNews serves the player-visible feed. Blind box: id, day, media,
// headline, body, headline_en, body_en, cluster_id, disputed, exposed —
// nothing else. Track,
// shock vectors and driven_by_user_id never leave the server; disputed and
// exposed are public action flags (a debunked or regulator-busted item is
// public knowledge); cluster_id only groups already-published items
// (传闻→主事件→追踪). media_accuracy carries per-outlet 应验率 as aggregate
// counts (see store.MediaAccuracy); the underlying report shocks are never
// exposed.
func (s *Server) handleNews(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if room.StartedAt == nil {
		writeErr(w, http.StatusBadRequest, store.ErrNotStarted.Error())
		return
	}
	curDay, _, err := room.CurrentDay(s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	rows, err := s.DB.Query(r.Context(), `
		SELECT id, day, media_id, headline, body, cluster_id, disputed, exposed, headline_en, body_en FROM room_news
		WHERE room_id = $1 AND day <= $2 AND id > $3
		ORDER BY id LIMIT $4`, room.ID, curDay, afterParam(r), newsPageLimit)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, clusterID int64
		var day int
		var mediaID, headline, body, headlineEn, bodyEn string
		var disputed, exposed bool
		if err := rows.Scan(&id, &day, &mediaID, &headline, &body, &clusterID, &disputed, &exposed, &headlineEn, &bodyEn); err != nil {
			s.storeErr(w, err)
			return
		}
		item := map[string]any{
			"id": id, "day": day, "media_id": mediaID, "headline": headline, "body": body,
			"headline_en": headlineEn, "body_en": bodyEn,
			"disputed": disputed, "exposed": exposed,
		}
		if clusterID > 0 {
			item["cluster_id"] = clusterID
		} else {
			item["cluster_id"] = nil
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}
	accuracy, err := store.MediaAccuracy(r.Context(), s.DB, room.ID, curDay)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "media_accuracy": accuracy})
}

// handleForum serves the NPC forum feed. Blind box: id, day, npc_name,
// body, npc_name_en, body_en — nothing else (persona hints never leave the
// server).
func (s *Server) handleForum(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if room.StartedAt == nil {
		writeErr(w, http.StatusBadRequest, store.ErrNotStarted.Error())
		return
	}
	curDay, _, err := room.CurrentDay(s.Now())
	if err != nil {
		s.storeErr(w, err)
		return
	}
	rows, err := s.DB.Query(r.Context(), `
		SELECT id, day, npc_name, body, npc_name_en, body_en FROM room_forum_posts
		WHERE room_id = $1 AND day <= $2 AND id > $3
		ORDER BY id LIMIT $4`, room.ID, curDay, afterParam(r), newsPageLimit)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var day int
		var npcName, body, npcNameEn, bodyEn string
		if err := rows.Scan(&id, &day, &npcName, &body, &npcNameEn, &bodyEn); err != nil {
			s.storeErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"id": id, "day": day, "npc_name": npcName, "body": body,
			"npc_name_en": npcNameEn, "body_en": bodyEn,
		})
	}
	if err := rows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	room, ok := s.roomForMember(w, r)
	if !ok {
		return
	}
	if _, _, err := store.SettleRoom(r.Context(), s.DB, room, s.Now()); err != nil {
		s.storeErr(w, err)
		return
	}
	rows, err := s.DB.Query(r.Context(), `
		SELECT id, day, kind, payload FROM room_events
		WHERE room_id = $1 AND id > $2
		ORDER BY id LIMIT $3`, room.ID, afterParam(r), newsPageLimit)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var day int
		var kind string
		var payload map[string]any
		if err := rows.Scan(&id, &day, &kind, &payload); err != nil {
			s.storeErr(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "day": day, "kind": kind, "payload": payload})
	}
	if err := rows.Err(); err != nil {
		s.storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleScenarios(w http.ResponseWriter, r *http.Request) {
	infos, err := store.ScenarioInfos(r.Context(), s.DB)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	items := []map[string]any{}
	for _, info := range infos {
		name := info.Name
		if name == "" {
			name = info.ID
		}
		// English display names ride along; empty means "fall back to name".
		items = append(items, map[string]any{
			"id": info.ID, "name": name, "name_en": info.NameEn, "days": info.Days,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
