package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

// Hype (造势) fee tiers in cents, and the base idiosyncratic shock each tier
// plants. Catch probabilities feed the regulatory roll.
const (
	HypeTier1FeeCents int64 = 500_000   // $5,000
	HypeTier2FeeCents int64 = 1_500_000 // $15,000
	HypeTier3FeeCents int64 = 4_000_000 // $40,000

	HypeTier1Shock = 0.015
	HypeTier2Shock = 0.03
	HypeTier3Shock = 0.05

	// HypeDiminishFactor discounts repeat manipulation of the same
	// instrument (any player) within a room: effective shock = tier shock ×
	// HypeDiminishFactor^n after n prior hypes.
	HypeDiminishFactor = 0.6

	// HypeDailyLimit caps hype actions per player per sim day.
	HypeDailyLimit = 1

	// DebunkFeeCents (辟谣/调查) is a flat fee.
	DebunkFeeCents int64 = 200_000 // $2,000
	// IntelFeeCents (内幕消息) per lookup, 1 per player per instrument per day.
	IntelFeeCents int64 = 300_000 // $3,000

	// debunkFidelity is the share of debunk verdicts that report the truth;
	// the rest are flipped (the investigation itself is fallible).
	debunkFidelity = 0.85
	// intelNoiseProb is the share of intel lookups whose answer is corrupted
	// (direction flipped, event silenced, or quiet fabricated into a tip).
	intelNoiseProb = 0.25
)

var (
	hypeFeeCents  = map[int]int64{1: HypeTier1FeeCents, 2: HypeTier2FeeCents, 3: HypeTier3FeeCents}
	hypeShock     = map[int]float64{1: HypeTier1Shock, 2: HypeTier2Shock, 3: HypeTier3Shock}
	hypeCatchProb = map[int]float64{1: 0.10, 2: 0.20, 3: 0.30}
)

// Intel strength buckets on the absolute shock magnitude.
const (
	intelStrongShock = 0.025
	intelMediumShock = 0.01
)

// HypeResult reports a settled hype action.
type HypeResult struct {
	FeeCents  int64
	Caught    bool
	FineCents int64
	CashCents int64
}

// DebunkResult reports a settled debunk action. Verdict is one of
// "likely_true" | "likely_false" | "no_substance".
type DebunkResult struct {
	Verdict   string
	FeeCents  int64
	CashCents int64
}

// IntelResult reports a settled intel action. Outlook is "up" | "down" |
// "quiet"; Strength is "strong" | "medium" | "weak", empty when quiet.
type IntelResult struct {
	Outlook   string
	Strength  string
	FeeCents  int64
	CashCents int64
}

// actionGuards runs the shared pre-transaction checks and returns curDay.
func actionGuards(room *Room, now time.Time) (int, error) {
	if room.Status != "running" {
		return 0, ErrRoomNotRunning
	}
	curDay, ended, err := room.CurrentDay(now)
	if err != nil {
		return 0, err
	}
	if ended {
		return 0, ErrRoomEnded
	}
	return curDay, nil
}

// lockRoomTx serializes all price-manipulating actions on the room row.
func lockRoomTx(ctx context.Context, tx pgx.Tx, roomID int64) error {
	var id int64
	return tx.QueryRow(ctx, `SELECT id FROM rooms WHERE id = $1 FOR UPDATE`, roomID).Scan(&id)
}

// instrumentFactor validates the action target against the room's scenario:
// the instrument must exist and carry an "IDIO:<id>" factor (otherwise the
// price re-synthesis would reject the planted shock).
func instrumentFactor(sc *scenario.Scenario, instrumentID string) (string, error) {
	fid := "IDIO:" + instrumentID
	found := false
	for _, inst := range sc.Instruments {
		if inst.ID == instrumentID {
			found = true
			break
		}
	}
	if found {
		for _, f := range sc.FactorIDs() {
			if f == fid {
				return fid, nil
			}
		}
	}
	return "", ErrUnknownInstrument
}

// lockPlayerTx reads and locks the acting player's row.
func lockPlayerTx(ctx context.Context, tx pgx.Tx, roomID, userID int64) (cash, debt int64, err error) {
	var bankruptDay *int
	err = tx.QueryRow(ctx, `
		SELECT cash_cents, debt_cents, bankrupt_day FROM room_players
		WHERE room_id = $1 AND user_id = $2 FOR UPDATE`,
		roomID, userID).Scan(&cash, &debt, &bankruptDay)
	if err != nil {
		return 0, 0, err
	}
	if bankruptDay != nil {
		return 0, 0, ErrPlayerBankrupt
	}
	return cash, debt, nil
}

// resynthPricesTx recomputes the room's future prices from the full impact
// shock timeline persisted in room_news (world-generation events plus every
// planted hype): factor states evolve from day 0, then prices are
// re-synthesized with the room seed — the ε stream is seed-derived, so days
// untouched by new shocks recompute byte-identically. Only rows for day ≥
// fromDay are written; published history is never rewritten. Callers must
// hold the room lock (lockRoomTx).
func resynthPricesTx(ctx context.Context, tx pgx.Tx, room *Room, sc *scenario.Scenario, fromDay int) error {
	rows, err := tx.Query(ctx, `
		SELECT day, true_shock FROM room_news
		WHERE room_id = $1 AND track = 'impact' AND true_shock IS NOT NULL`,
		room.ID)
	if err != nil {
		return err
	}
	var evs []engine.NewsEvent
	for rows.Next() {
		var ev engine.NewsEvent
		if err := rows.Scan(&ev.Day, &ev.TrueShock); err != nil {
			rows.Close()
			return err
		}
		ev.Track = engine.TrackImpact
		evs = append(evs, ev)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	states, err := engine.EvolveFactorStates(sc, evs)
	if err != nil {
		return err
	}
	prices := engine.SynthesizePrices(sc, states, room.Seed)

	batch := &pgx.Batch{}
	queued := 0
	for _, inst := range sc.Instruments {
		for d := fromDay; d < sc.Days; d++ {
			p := prices[inst.ID][d]
			batch.Queue(`
				UPDATE room_prices SET open = $1, high = $2, low = $3, close = $4
				WHERE room_id = $5 AND instrument_id = $6 AND day = $7`,
				p.Open, p.High, p.Low, p.Close, room.ID, inst.ID, d)
			queued++
		}
	}
	if queued == 0 {
		return nil
	}
	br := tx.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < queued; i++ {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ResynthesizePrices recomputes a room's prices for day >= fromDay from the
// persisted impact shock timeline. With no new shocks injected the rewrite
// is a no-op byte-for-byte; it exists to prove that invariant and to repair.
func ResynthesizePrices(ctx context.Context, db *pgxpool.Pool, room *Room, sc *scenario.Scenario, fromDay int) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := lockRoomTx(ctx, tx, room.ID); err != nil {
			return err
		}
		return resynthPricesTx(ctx, tx, room, sc, fromDay)
	})
}

// Hype plants a paid, true idiosyncratic shock on an instrument: the fee
// buys a real price move (the story is literally true), applied from the
// next sim day via full price re-synthesis. One transaction, serialized per
// room: settle → lock player → guards → fee → news row → re-synthesis →
// NPC forum follow-ups → regulatory roll → player_actions record.
func Hype(ctx context.Context, db *pgxpool.Pool, room *Room, sc *scenario.Scenario, userID int64, now time.Time, instrumentID, direction string, tier int) (*HypeResult, error) {
	curDay, err := actionGuards(room, now)
	if err != nil {
		return nil, err
	}
	fee, ok := hypeFeeCents[tier]
	if !ok || (direction != "up" && direction != "down") {
		return nil, ErrBadAction
	}
	fid, err := instrumentFactor(sc, instrumentID)
	if err != nil {
		return nil, err
	}

	var out *HypeResult
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := lockRoomTx(ctx, tx, room.ID); err != nil {
			return err
		}
		if err := SettleTx(ctx, tx, room, curDay, false); err != nil {
			return err
		}
		cash, debt, err := lockPlayerTx(ctx, tx, room.ID, userID)
		if err != nil {
			return err
		}
		var todayCount int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM player_actions
			WHERE room_id = $1 AND user_id = $2 AND day = $3 AND kind = 'hype'`,
			room.ID, userID, curDay).Scan(&todayCount); err != nil {
			return err
		}
		if todayCount >= HypeDailyLimit {
			return ErrActionLimit
		}
		if cash < fee {
			return ErrInsufficientCash
		}

		// Diminishing returns: n prior hypes (any player) on this instrument.
		var priorHypes, userHypes int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM player_actions
			WHERE room_id = $1 AND kind = 'hype' AND payload->>'instrument_id' = $2`,
			room.ID, instrumentID).Scan(&priorHypes); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM player_actions
			WHERE room_id = $1 AND user_id = $2 AND kind = 'hype'`,
			room.ID, userID).Scan(&userHypes); err != nil {
			return err
		}
		shock := hypeShock[tier] * math.Pow(HypeDiminishFactor, float64(priorHypes))
		if direction == "down" {
			shock = -shock
		}

		if _, err := tx.Exec(ctx, `
			UPDATE room_players SET cash_cents = cash_cents - $1
			WHERE room_id = $2 AND user_id = $3`,
			fee, room.ID, userID); err != nil {
			return err
		}
		cash -= fee

		// The planted story: rumor style, directional, alias-only, no
		// numbers — and TRUE (true_shock == report_shock), because the
		// manipulation really moves the price.
		var alias string
		for _, inst := range sc.Instruments {
			if inst.ID == instrumentID {
				alias = engine.ResolveAlias(room.Seed, inst.ID, inst.Alias, inst.Aliases)
				break
			}
		}
		headline, body, headlineEn, bodyEn := hypeCopy(direction, alias)
		shockMap := map[string]float64{fid: shock}
		var newsID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO room_news (room_id, day, media_id, headline, track, true_shock, report_shock, body, cluster_id, driven_by_user_id, headline_en, body_en)
			VALUES ($1, $2, 'tabloid', $3, 'impact', $4, $5, $6, 0, $7, $8, $9) RETURNING id`,
			room.ID, curDay, headline, shockJSON(shockMap), shockJSON(shockMap), body, userID, headlineEn, bodyEn).Scan(&newsID); err != nil {
			return err
		}

		if err := resynthPricesTx(ctx, tx, room, sc, curDay+1); err != nil {
			return err
		}

		// NPC forum reacts to the fresh rumor (1-3 posts, forum families).
		for _, p := range engine.ManipulationFollowUps(room.Seed, curDay, alias,
			fmt.Sprint(userID), fmt.Sprint(userHypes)) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO room_forum_posts (room_id, day, npc_name, body, npc_name_en, body_en, is_agent)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				room.ID, p.Day, p.NPCName, p.Body, p.NPCNameEn, p.BodyEn, p.IsAgent); err != nil {
				return err
			}
		}

		// Regulatory roll, deterministic in (seed, user, action count).
		caught := engine.Stream(room.Seed, "manipulation",
			fmt.Sprint(userID), fmt.Sprint(userHypes)).Float64() < hypeCatchProb[tier]
		var fine int64
		if caught {
			// Fine rule: 3× the fee, capped at what the player can carry —
			// cash on hand plus the remaining debt headroom under
			// MaxDebtCents. Cash is taken first (down to zero), any
			// shortfall becomes debt; the cap keeps debt ≤ MaxDebtCents so
			// the fine alone never bankrupts. Fine-created debt mirrors
			// Borrow: a fresh debt starts accruing the NEXT day.
			headroom := MaxDebtCents - debt
			if headroom < 0 {
				headroom = 0
			}
			fine = min(3*fee, cash+headroom)
			fromCash := min(fine, cash)
			if _, err := tx.Exec(ctx, `
				UPDATE room_players SET
					cash_cents = cash_cents - $1,
					debt_cents = debt_cents + $2,
					interest_through_day = CASE
						WHEN debt_cents = 0 THEN $5
						ELSE interest_through_day END
				WHERE room_id = $3 AND user_id = $4`,
				fromCash, fine-fromCash, room.ID, userID, curDay); err != nil {
				return err
			}
			cash -= fromCash
			var alias string
			if err := tx.QueryRow(ctx,
				`SELECT COALESCE(NULLIF(display_name, ''), 'Player') FROM users WHERE id = $1`, userID).Scan(&alias); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO room_events (room_id, day, kind, payload)
				VALUES ($1, $2, 'manipulation_bust', jsonb_build_object(
					'username', $3::text, 'fine_cents', $4::bigint,
					'instrument_id', $5::text, 'day', $2::int))`,
				room.ID, curDay, alias, fine, instrumentID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx,
				`UPDATE room_news SET exposed = TRUE WHERE id = $1`, newsID); err != nil {
				return err
			}
		}

		payload, err := json.Marshal(map[string]any{
			"instrument_id": instrumentID, "direction": direction, "tier": tier,
			"shock": shock, "caught": caught, "fine_cents": fine,
		})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO player_actions (room_id, user_id, day, kind, payload, fee_cents)
			VALUES ($1, $2, $3, 'hype', $4, $5)`,
			room.ID, userID, curDay, string(payload), fee); err != nil {
			return err
		}
		out = &HypeResult{FeeCents: fee, Caught: caught, FineCents: fine, CashCents: cash}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// hypeCopy renders the planted headline/body in both languages: rumor
// tone, directional, no numbers, alias only.
func hypeCopy(direction, alias string) (headline, body, headlineEn, bodyEn string) {
	if direction == "up" {
		return fmt.Sprintf("据传%s有重磅利好正在酝酿，资金闻风而动", alias),
			fmt.Sprintf("市场传闻称，%s近期或有重大利好消息公布。多位市场人士暗示已提前布局，具体细节尚待证实。", alias),
			fmt.Sprintf("Word is %s has blockbuster news brewing, and the smart money is already circling", alias),
			fmt.Sprintf("Market whispers say %s could unveil major good news any day now. Several insiders hint they positioned early; details remain unconfirmed.", alias)
	}
	return fmt.Sprintf("据传%s暗藏隐忧，知情人士悄然离场", alias),
		fmt.Sprintf("市场传闻称，%s近期或面临重大不利变化。有消息称部分资金已提前撤离，具体细节尚待证实。", alias),
		fmt.Sprintf("Whispers say %s is hiding serious trouble, and those in the know are quietly slipping out", alias),
		fmt.Sprintf("Market whispers say %s could be facing a major setback. Word is some funds have already pulled out; details remain unconfirmed.", alias)
}

// Debunk investigates one published news item: the player pays a flat fee
// and gets a private verdict (85% fidelity); the item is publicly flagged
// disputed. One-shot per item.
func Debunk(ctx context.Context, db *pgxpool.Pool, room *Room, userID int64, now time.Time, newsID int64) (*DebunkResult, error) {
	curDay, err := actionGuards(room, now)
	if err != nil {
		return nil, err
	}

	var out *DebunkResult
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := lockRoomTx(ctx, tx, room.ID); err != nil {
			return err
		}
		if err := SettleTx(ctx, tx, room, curDay, false); err != nil {
			return err
		}
		cash, _, err := lockPlayerTx(ctx, tx, room.ID, userID)
		if err != nil {
			return err
		}
		if cash < DebunkFeeCents {
			return ErrInsufficientCash
		}

		var day int
		var trueShock, reportShock map[string]float64
		var disputed bool
		err = tx.QueryRow(ctx, `
			SELECT day, true_shock, report_shock, disputed FROM room_news
			WHERE id = $1 AND room_id = $2 FOR UPDATE`,
			newsID, room.ID).Scan(&day, &trueShock, &reportShock, &disputed)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if day > curDay {
			return ErrNotFound // unpublished items don't exist for players
		}
		if disputed {
			return ErrAlreadyDisputed
		}

		verdict := "no_substance"
		if len(trueShock) > 0 {
			// Dominant true direction vs dominant reported direction.
			trueDir := sign(dominant(trueShock))
			repDir := trueDir
			if len(reportShock) > 0 {
				repDir = sign(dominant(reportShock))
			}
			verdict = "likely_true"
			if trueDir != repDir {
				verdict = "likely_false"
			}
			if engine.Stream(room.Seed, "debunk", fmt.Sprint(newsID)).Float64() >= debunkFidelity {
				if verdict == "likely_true" {
					verdict = "likely_false"
				} else {
					verdict = "likely_true"
				}
			}
		}

		if _, err := tx.Exec(ctx,
			`UPDATE room_news SET disputed = TRUE WHERE id = $1`, newsID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE room_players SET cash_cents = cash_cents - $1
			WHERE room_id = $2 AND user_id = $3`,
			DebunkFeeCents, room.ID, userID); err != nil {
			return err
		}
		cash -= DebunkFeeCents

		payload, err := json.Marshal(map[string]any{"news_id": newsID, "verdict": verdict})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO player_actions (room_id, user_id, day, kind, payload, fee_cents)
			VALUES ($1, $2, $3, 'debunk', $4, $5)`,
			room.ID, userID, curDay, string(payload), DebunkFeeCents); err != nil {
			return err
		}
		out = &DebunkResult{Verdict: verdict, FeeCents: DebunkFeeCents, CashCents: cash}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// dominant returns the value of the largest-|v| entry of a shock map
// (deterministic: ties break on the lexicographically smaller factor id).
func dominant(shock map[string]float64) float64 {
	best := ""
	var val float64
	for f, v := range shock {
		if best == "" || math.Abs(v) > math.Abs(val) || (math.Abs(v) == math.Abs(val) && f < best) {
			best, val = f, v
		}
	}
	return val
}

func sign(v float64) int {
	if v < 0 {
		return -1
	}
	return 1
}

// Intel sells a noisy peek at tomorrow: the strongest true idiosyncratic
// shock on the instrument published today (which takes effect tomorrow),
// bucketed, with a 25% chance the tip itself is corrupted. Quiet days
// answer "quiet" (modulo the same noise). Limit 1 per player per
// instrument per day.
func Intel(ctx context.Context, db *pgxpool.Pool, room *Room, sc *scenario.Scenario, userID int64, now time.Time, instrumentID string) (*IntelResult, error) {
	curDay, err := actionGuards(room, now)
	if err != nil {
		return nil, err
	}
	fid, err := instrumentFactor(sc, instrumentID)
	if err != nil {
		return nil, err
	}

	var out *IntelResult
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := lockRoomTx(ctx, tx, room.ID); err != nil {
			return err
		}
		if err := SettleTx(ctx, tx, room, curDay, false); err != nil {
			return err
		}
		cash, _, err := lockPlayerTx(ctx, tx, room.ID, userID)
		if err != nil {
			return err
		}
		var todayCount int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM player_actions
			WHERE room_id = $1 AND user_id = $2 AND day = $3 AND kind = 'intel'
				AND payload->>'instrument_id' = $4`,
			room.ID, userID, curDay, instrumentID).Scan(&todayCount); err != nil {
			return err
		}
		if todayCount >= 1 {
			return ErrActionLimit
		}
		if cash < IntelFeeCents {
			return ErrInsufficientCash
		}

		// True shocks targeting this instrument, published today, effective
		// tomorrow (a publish-day-d shock lands on d+1).
		rows, err := tx.Query(ctx, `
			SELECT (true_shock ->> $3)::float8 FROM room_news
			WHERE room_id = $1 AND day = $2 AND track = 'impact' AND true_shock ? $3`,
			room.ID, curDay, fid)
		if err != nil {
			return err
		}
		var strongest float64
		found := false
		for rows.Next() {
			var v float64
			if err := rows.Scan(&v); err != nil {
				rows.Close()
				return err
			}
			if !found || math.Abs(v) > math.Abs(strongest) {
				strongest, found = v, true
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		outlook, strength := "quiet", ""
		if found {
			outlook, strength = "up", bucketStrength(math.Abs(strongest))
			if strongest < 0 {
				outlook = "down"
			}
		}

		// Noise: 25% of tips are corrupted — flip the direction, silence a
		// real event, or fabricate a tip out of a quiet day. Deterministic
		// in (seed, user, instrument, day).
		rng := engine.Stream(room.Seed, "intel",
			fmt.Sprint(userID), instrumentID, fmt.Sprint(curDay))
		if rng.Float64() < intelNoiseProb {
			if found {
				if rng.Float64() < 0.5 {
					outlook, strength = "quiet", ""
				} else if outlook == "up" {
					outlook = "down"
				} else {
					outlook = "up"
				}
			} else {
				outlook = "up"
				if rng.Float64() < 0.5 {
					outlook = "down"
				}
				strength = []string{"strong", "medium", "weak"}[rng.IntN(3)]
			}
		}

		if _, err := tx.Exec(ctx, `
			UPDATE room_players SET cash_cents = cash_cents - $1
			WHERE room_id = $2 AND user_id = $3`,
			IntelFeeCents, room.ID, userID); err != nil {
			return err
		}
		cash -= IntelFeeCents

		payload, err := json.Marshal(map[string]any{
			"instrument_id": instrumentID, "outlook": outlook, "strength": strength,
		})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO player_actions (room_id, user_id, day, kind, payload, fee_cents)
			VALUES ($1, $2, $3, 'intel', $4, $5)`,
			room.ID, userID, curDay, string(payload), IntelFeeCents); err != nil {
			return err
		}
		out = &IntelResult{Outlook: outlook, Strength: strength, FeeCents: IntelFeeCents, CashCents: cash}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func bucketStrength(mag float64) string {
	switch {
	case mag >= intelStrongShock:
		return "strong"
	case mag >= intelMediumShock:
		return "medium"
	default:
		return "weak"
	}
}
