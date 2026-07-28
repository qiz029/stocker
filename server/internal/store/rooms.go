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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

const InitialCashCents int64 = 10_000_000 // $100,000 (spec §2.2)

// CopyFillBudget bounds how long room creation waits for the news copy
// filler. Providers with tight per-key concurrency limits (observed:
// DeepSeek queues past ~5 in-flight requests) may need several minutes to
// fill a 1500+-item world; unfilled items keep template copy either way.
// cmd/server overrides this from LLM_ROOM_BUDGET_SECS.
var CopyFillBudget = 120 * time.Second

// NewsCopyFiller rewrites news headlines/bodies before a room's world is
// persisted (the LLM generator in internal/llm; nil keeps template copy).
type NewsCopyFiller interface {
	FillCopy(ctx context.Context, sc *scenario.Scenario, evs []engine.NewsEvent)
}

type Room struct {
	ID              int64
	InviteCode      string
	ScenarioID      string
	Days            int
	Seed            uint64
	Status          string // "lobby" | "running"
	DayDurationSecs int
	StartedAt       *time.Time
	HostUserID      int64
}

const roomCols = `id, invite_code, scenario_id, days, seed, status, day_duration_secs, started_at, host_user_id`

func scanRoom(row pgx.Row) (*Room, error) {
	r := &Room{}
	var seed int64
	err := row.Scan(&r.ID, &r.InviteCode, &r.ScenarioID, &r.Days, &seed,
		&r.Status, &r.DayDurationSecs, &r.StartedAt, &r.HostUserID)
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
func CreateRoom(ctx context.Context, db *pgxpool.Pool, sc *scenario.Scenario, hostID int64, dayDurationSecs int, filler NewsCopyFiller) (*Room, error) {
	if dayDurationSecs < 60 || dayDurationSecs > 86400 {
		return nil, ErrBadDayDuration
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
	invite, err := newInviteCode()
	if err != nil {
		return nil, err
	}

	if filler != nil {
		fctx, cancel := context.WithTimeout(ctx, CopyFillBudget)
		filler.FillCopy(fctx, sc, world.News)
		cancel()
	}

	var room *Room
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		r, err := scanRoom(tx.QueryRow(ctx, `
			INSERT INTO rooms (invite_code, scenario_id, days, seed, day_duration_secs, host_user_id)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+roomCols,
			invite, sc.ID, sc.Days, int64(seed), dayDurationSecs, hostID))
		if err != nil {
			return err
		}
		room = r
		if _, err := tx.Exec(ctx, `
			INSERT INTO room_players (room_id, user_id, cash_cents, joined_day)
			VALUES ($1, $2, $3, 0)`, room.ID, hostID, InitialCashCents); err != nil {
			return err
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
		for _, ev := range world.News {
			news = append(news, []any{room.ID, ev.Day, ev.MediaID, ev.Headline,
				string(ev.Track), shockJSON(ev.TrueShock), shockJSON(ev.ReportShock),
				ev.Body, ev.ClusterID})
		}
		_, err = tx.CopyFrom(ctx, pgx.Identifier{"room_news"},
			[]string{"room_id", "day", "media_id", "headline", "track", "true_shock", "report_shock", "body", "cluster_id"},
			pgx.CopyFromRows(news))
		return err
	})
	if err != nil {
		return nil, err
	}
	return room, nil
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
