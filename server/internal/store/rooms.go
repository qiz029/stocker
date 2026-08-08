package store

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

const (
	InitialCashCents  int64 = 10_000_000 // $100,000 (spec §2.2)
	AgentPlayerCount        = 5
	MaxHumanPlayers         = 12
	RoomNameMaxLength       = 40
)

// NewsCopyFiller rewrites news headlines/bodies. Room creation persists the
// engine's bilingual templates immediately; copy_jobs.go invokes the filler
// later for a small rolling window of simulation days.
type NewsCopyFiller interface {
	FillCopy(ctx context.Context, sc *scenario.Scenario, evs []engine.NewsEvent)
}

// ForumCopyFiller is an optional NewsCopyFiller extension that polishes NPC
// forum-post bodies in place (the LLM generator implements it). Posts whose
// bodies it leaves unchanged keep their template text.
type ForumCopyFiller interface {
	FillForumCopy(ctx context.Context, sc *scenario.Scenario, posts []engine.ForumPost)
}

type Room struct {
	ID              int64
	Name            string
	InviteCode      string
	ScenarioID      string
	Days            int
	Seed            uint64
	Status          string // "lobby" | "running"
	DayDurationSecs int
	StartedAt       *time.Time
	HostUserID      int64
	Visibility      string // "public" | "private"
}

const roomCols = `id, name, invite_code, scenario_id, days, seed, status, day_duration_secs, started_at, host_user_id, visibility`

func scanRoom(row pgx.Row) (*Room, error) {
	r := &Room{}
	var seed int64
	err := row.Scan(&r.ID, &r.Name, &r.InviteCode, &r.ScenarioID, &r.Days, &seed,
		&r.Status, &r.DayDurationSecs, &r.StartedAt, &r.HostUserID, &r.Visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.Seed = uint64(seed) // seeds are stored bit-cast into BIGINT
	return r, nil
}

// CurrentDay maps wall clock to the historical trading day.
// engine.CurrentDay's contract requires now >= startedAt and a positive
// duration; the duration is validated at creation and clock skew is
// clamped here, so the contract always holds.
func (r *Room) CurrentDay(now time.Time) (int, bool, error) {
	if r.StartedAt == nil {
		return 0, false, ErrNotStarted
	}
	if now.Before(*r.StartedAt) {
		now = *r.StartedAt
	}
	day, ended := engine.CurrentDay(*r.StartedAt,
		time.Duration(r.DayDurationSecs)*time.Second, r.Days, now)
	return day, ended, nil
}

func newInviteCode() (string, error) {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]), nil
}

func shockJSON(m map[string]float64) any {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return string(b)
}

// CreateRoom generates this room's parallel world and persists it whole.
// The fidelity gate (engine.VerifyFidelity) can reject a seed; we retry
// derived seeds a bounded number of times (spec §4.6: "不达标的参数组合拒绝").
func CreateRoom(ctx context.Context, db *pgxpool.Pool, sc *scenario.Scenario, hostID int64, dayDurationSecs int, _ NewsCopyFiller, visibility ...string) (*Room, error) {
	return CreateNamedRoom(ctx, db, sc, hostID, dayDurationSecs, "Market Room", nil, visibility...)
}

// CreateNamedRoom persists a room with the player-facing name chosen by its
// host. CreateRoom remains as a compatibility wrapper for internal callers.
func CreateNamedRoom(ctx context.Context, db *pgxpool.Pool, sc *scenario.Scenario, hostID int64, dayDurationSecs int, roomName string, _ NewsCopyFiller, visibility ...string) (*Room, error) {
	roomName = strings.TrimSpace(roomName)
	if n := utf8.RuneCountInString(roomName); n < 2 || n > RoomNameMaxLength || strings.IndexFunc(roomName, unicode.IsControl) >= 0 {
		return nil, ErrBadRoomName
	}
	if dayDurationSecs < 60 || dayDurationSecs > 86400 {
		return nil, ErrBadDayDuration
	}
	roomVisibility := "private"
	if len(visibility) > 0 && visibility[0] != "" {
		roomVisibility = visibility[0]
	}
	if roomVisibility != "public" && roomVisibility != "private" {
		return nil, ErrBadVisibility
	}
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return nil, err
	}
	base := binary.LittleEndian.Uint64(b[:])
	var world *engine.World
	var seed uint64
	var lastErr error
	for attempt := uint64(0); attempt < 10; attempt++ {
		w, err := engine.GenerateWorld(sc, base+attempt)
		if err == nil {
			world, seed = w, base+attempt
			break
		}
		lastErr = err
	}
	if world == nil {
		return nil, fmt.Errorf("world generation failed after 10 seeds: %w", lastErr)
	}
	if seed != base {
		log.Printf("room: scenario %s took %d world-gen attempts (fidelity rejections are by design; see spec §4.6)", sc.ID, seed-base+1)
	}
	// Per-room blind-box names: resolve each instrument's display alias
	// from the FINAL seed (after fidelity-gate retries), so the news-copy
	// filler below writes the same name players see on their cards.
	// Display-only: the engine ignores Alias, and the world is already
	// generated by this point.
	for i := range sc.Instruments {
		inst := &sc.Instruments[i]
		inst.Alias = engine.ResolveAlias(seed, inst.ID, inst.Alias, inst.Aliases)
	}
	invite, err := newInviteCode()
	if err != nil {
		return nil, err
	}

	var room *Room
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		r, err := scanRoom(tx.QueryRow(ctx, `
			INSERT INTO rooms (name, invite_code, scenario_id, days, seed, day_duration_secs, host_user_id, visibility)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+roomCols,
			roomName, invite, sc.ID, sc.Days, int64(seed), dayDurationSecs, hostID, roomVisibility))
		if err != nil {
			return err
		}
		room = r
		if _, err := tx.Exec(ctx, `
			INSERT INTO room_players (room_id, user_id, cash_cents, joined_day)
			VALUES ($1, $2, $3, 0)`, room.ID, hostID, InitialCashCents); err != nil {
			return err
		}
		if tag, err := tx.Exec(ctx, `
			INSERT INTO room_players (room_id, user_id, cash_cents, joined_day)
			SELECT $1, id, $2, 0 FROM users WHERE is_agent ORDER BY agent_slot
			ON CONFLICT (room_id, user_id) DO NOTHING`, room.ID, InitialCashCents); err != nil {
			return err
		} else if tag.RowsAffected() != AgentPlayerCount {
			return fmt.Errorf("seat agents: inserted %d, want %d", tag.RowsAffected(), AgentPlayerCount)
		}

		prices := make([][]any, 0, len(sc.Instruments)*sc.Days)
		for _, inst := range sc.Instruments {
			for d, p := range world.Prices[inst.ID] {
				prices = append(prices, []any{room.ID, inst.ID, d, p.Open, p.High, p.Low, p.Close})
			}
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"room_prices"},
			[]string{"room_id", "instrument_id", "day", "open", "high", "low", "close"},
			pgx.CopyFromRows(prices)); err != nil {
			return err
		}

		news := make([][]any, 0, len(world.News))
		for i, ev := range world.News {
			news = append(news, []any{room.ID, ev.Day, ev.MediaID, ev.Headline,
				string(ev.Track), shockJSON(ev.TrueShock), shockJSON(ev.ReportShock),
				ev.Body, ev.ClusterID, ev.HeadlineEn, ev.BodyEn, ev.Recap,
				copyRole(world.News, i)})
		}
		_, err = tx.CopyFrom(ctx, pgx.Identifier{"room_news"},
			[]string{"room_id", "day", "media_id", "headline", "track", "true_shock", "report_shock", "body", "cluster_id", "headline_en", "body_en", "is_recap", "copy_role"},
			pgx.CopyFromRows(news))
		if err != nil {
			return err
		}

		posts := make([][]any, 0, len(world.Forum))
		for _, p := range world.Forum {
			posts = append(posts, []any{room.ID, p.Day, p.NPCName, p.Body, p.NPCNameEn, p.BodyEn, p.IsAgent, p.Persona})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"room_forum_posts"},
			[]string{"room_id", "day", "npc_name", "body", "npc_name_en", "body_en", "is_agent", "persona"},
			pgx.CopyFromRows(posts)); err != nil {
			return err
		}
		jobs := make([][]any, sc.Days)
		for day := 0; day < sc.Days; day++ {
			jobs[day] = []any{room.ID, day}
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"room_copy_jobs"},
			[]string{"room_id", "day"}, pgx.CopyFromRows(jobs)); err != nil {
			return err
		}
		// Initial option chain: the first 2 expiries, strikes anchored to
		// the day-0 close. Settlement keeps the chain rolling from there.
		return listOptionsTx(ctx, tx, room, 0)
	})
	if err != nil {
		return nil, err
	}
	return room, nil
}

// copyRole preserves a clustered story's position for day-sized LLM prompts.
// This is server-only metadata and never enters the blind-box API projection.
func copyRole(events []engine.NewsEvent, i int) string {
	ev := events[i]
	if ev.ClusterID == 0 {
		return ""
	}
	if ev.TrueShock != nil {
		return "report"
	}
	for j := range events {
		if events[j].ClusterID == ev.ClusterID && events[j].TrueShock != nil {
			if ev.Day < events[j].Day {
				return "rumor"
			}
			return "followup"
		}
	}
	return ""
}

func GetRoom(ctx context.Context, q Querier, id int64) (*Room, error) {
	return scanRoom(q.QueryRow(ctx, `SELECT `+roomCols+` FROM rooms WHERE id = $1`, id))
}

func GetRoomByInvite(ctx context.Context, q Querier, code string) (*Room, error) {
	return scanRoom(q.QueryRow(ctx, `SELECT `+roomCols+` FROM rooms WHERE invite_code = $1`, code))
}

// JoinRoom seats a user. Mid-game joiners start on the current day with
// full initial cash and a "late join" marker (spec §2.1 中途加入).
func JoinRoom(ctx context.Context, db Querier, room *Room, userID int64, now time.Time) (int, error) {
	joinedDay := 0
	if room.Status == "running" {
		day, ended, err := room.CurrentDay(now)
		if err != nil {
			return 0, err
		}
		if ended {
			return 0, ErrRoomEnded
		}
		joinedDay = day
	}
	_, err := db.Exec(ctx, `
		INSERT INTO room_players (room_id, user_id, cash_cents, joined_day)
		VALUES ($1, $2, $3, $4)`, room.ID, userID, InitialCashCents, joinedDay)
	if isUniqueViolation(err) {
		return 0, ErrAlreadyJoined
	}
	if err != nil {
		return 0, err
	}
	return joinedDay, nil
}

func StartRoom(ctx context.Context, q Querier, roomID, hostID int64, now time.Time) (*Room, error) {
	r, err := scanRoom(q.QueryRow(ctx, `
		UPDATE rooms SET status = 'running', started_at = $3
		WHERE id = $1 AND host_user_id = $2 AND status = 'lobby'
		RETURNING `+roomCols, roomID, hostID, now))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrCannotStart
	}
	return r, err
}

func ListRoomsForUser(ctx context.Context, q Querier, userID int64) ([]Room, error) {
	rows, err := q.Query(ctx, `
		SELECT `+roomCols+` FROM rooms
		WHERE id IN (SELECT room_id FROM room_players WHERE user_id = $1)
		ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Room
	for rows.Next() {
		r, err := scanRoom(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func IsMember(ctx context.Context, q Querier, roomID, userID int64) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM room_players WHERE room_id = $1 AND user_id = $2)`,
		roomID, userID).Scan(&ok)
	return ok, err
}

func HumanPlayerCount(ctx context.Context, q Querier, roomID int64) (int, error) {
	var count int
	err := q.QueryRow(ctx, `
		SELECT COUNT(*) FROM room_players rp JOIN users u ON u.id = rp.user_id
		WHERE rp.room_id = $1 AND NOT u.is_agent`, roomID).Scan(&count)
	return count, err
}

// JoinPublicRoom atomically verifies that the table is still public, waiting,
// and below its human-player limit before seating the user. Locking the room
// prevents a concurrent start or burst of joins from crossing that boundary.
func JoinPublicRoom(ctx context.Context, db *pgxpool.Pool, roomID, userID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		room, err := scanRoom(tx.QueryRow(ctx, `SELECT `+roomCols+` FROM rooms WHERE id = $1 FOR UPDATE`, roomID))
		if err != nil {
			return err
		}
		if room.Visibility != "public" || room.Status != "lobby" {
			return ErrPublicJoinClosed
		}
		humans, err := HumanPlayerCount(ctx, tx, room.ID)
		if err != nil {
			return err
		}
		if humans >= MaxHumanPlayers {
			return ErrRoomFull
		}
		_, err = JoinRoom(ctx, tx, room, userID, time.Time{})
		return err
	})
}

type PublicRoomSummary struct {
	Room
	HumanPlayers int
	LeaderName   string
	LeaderAvatar string
	LeaderReturn float64
}

// ListPublicRooms returns lobby and running rooms without any invite codes or
// private player state. Agents are counted separately by the API contract and
// never consume the human capacity shown in the hall.
func ListPublicRooms(ctx context.Context, q Querier, limit int) ([]PublicRoomSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := q.Query(ctx, `
		SELECT r.id, r.name, r.invite_code, r.scenario_id, r.days, r.seed, r.status,
			r.day_duration_secs, r.started_at, r.host_user_id, r.visibility,
			(SELECT COUNT(*)::int FROM room_players rp JOIN users u ON u.id = rp.user_id
			 WHERE rp.room_id = r.id AND NOT u.is_agent) AS human_players,
			COALESCE(leader.username, ''), COALESCE(leader.avatar_id, ''),
			COALESCE((leader.total_cents - $2)::double precision / $2, 0)
		FROM rooms r
		LEFT JOIN LATERAL (
			SELECT COALESCE(NULLIF(u.display_name, ''), 'Player') username,
				u.avatar_id, latest.total_cents
			FROM (
				SELECT DISTINCT ON (d.user_id) d.user_id, d.total_cents
				FROM room_player_daily d
				WHERE d.room_id = r.id
				ORDER BY d.user_id, d.day DESC
			) latest
			JOIN users u ON u.id = latest.user_id AND NOT u.is_agent
			ORDER BY latest.total_cents DESC, u.id
			LIMIT 1
		) leader ON true
		WHERE r.visibility = 'public'
		ORDER BY CASE r.status WHEN 'running' THEN 0 ELSE 1 END, r.id DESC
		LIMIT $1`, limit, InitialCashCents)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PublicRoomSummary{}
	for rows.Next() {
		r := PublicRoomSummary{}
		var seed int64
		if err := rows.Scan(&r.ID, &r.Name, &r.InviteCode, &r.ScenarioID, &r.Days, &seed,
			&r.Status, &r.DayDurationSecs, &r.StartedAt, &r.HostUserID, &r.Visibility,
			&r.HumanPlayers, &r.LeaderName, &r.LeaderAvatar, &r.LeaderReturn); err != nil {
			return nil, err
		}
		r.Seed = uint64(seed)
		out = append(out, r)
	}
	return out, rows.Err()
}

type EraLeaderboardRow struct {
	ScenarioID string
	Username   string
	AvatarID   string
	ReturnPct  float64
	Wins       int
}

// EraLeaderboard ranks humans by their best settled total in completed public
// rooms. A player's best run represents them once per era; agents are excluded.
func EraLeaderboard(ctx context.Context, q Querier, now time.Time, limit int) ([]EraLeaderboardRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := q.Query(ctx, `
		WITH completed AS (
			SELECT r.id, r.scenario_id
			FROM rooms r
			WHERE r.visibility = 'public' AND r.started_at IS NOT NULL
			  AND r.started_at + make_interval(secs => r.day_duration_secs * r.days) <= $2
		), final_totals AS (
			SELECT d.room_id, d.user_id, d.total_cents
			FROM room_player_daily d
			JOIN completed c ON c.id = d.room_id
			JOIN rooms r ON r.id = d.room_id
			WHERE d.day = r.days - 1
		), ranked AS (
			SELECT c.scenario_id, u.id, COALESCE(NULLIF(u.display_name, ''), 'Player') username,
				u.avatar_id, (f.total_cents - $1)::double precision / $1 AS return_pct,
				ROW_NUMBER() OVER (PARTITION BY c.id ORDER BY f.total_cents DESC, u.id) room_rank
			FROM final_totals f
			JOIN completed c ON c.id = f.room_id
			JOIN users u ON u.id = f.user_id
			WHERE NOT u.is_agent
		), best AS (
			SELECT scenario_id, id, username, avatar_id, MAX(return_pct) return_pct,
				COUNT(*) FILTER (WHERE room_rank = 1)::int wins
			FROM ranked GROUP BY scenario_id, id, username, avatar_id
		), era_ranked AS (
			SELECT *, ROW_NUMBER() OVER (
				PARTITION BY scenario_id ORDER BY return_pct DESC, username
			) era_rank
			FROM best
		)
		SELECT scenario_id, username, avatar_id, return_pct, wins
		FROM era_ranked WHERE era_rank <= $3
		ORDER BY scenario_id, era_rank`, InitialCashCents, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EraLeaderboardRow{}
	for rows.Next() {
		var r EraLeaderboardRow
		if err := rows.Scan(&r.ScenarioID, &r.Username, &r.AvatarID, &r.ReturnPct, &r.Wins); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
