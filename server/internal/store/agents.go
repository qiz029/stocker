package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const minimumAgentOrderCents int64 = 10_000 // do not emit dust orders below $100

type agentPlayer struct {
	userID    int64
	slot      int
	name      string
	joinedDay int
}

// RunAgentTurns advances every running room's built-in competitors. The
// room/day uniqueness constraint makes calls safe to repeat. Decisions use
// only persisted room state and deterministic room/day/slot selection, so
// restarts do not change an agent's choice.
func RunAgentTurns(ctx context.Context, db *pgxpool.Pool, now time.Time) error {
	rows, err := db.Query(ctx, `SELECT id FROM rooms WHERE status = 'running' ORDER BY id`)
	if err != nil {
		return err
	}
	var roomIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		roomIDs = append(roomIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	var roomErrs []error
	for _, roomID := range roomIDs {
		room, err := GetRoom(ctx, db, roomID)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			roomErrs = append(roomErrs, fmt.Errorf("agent room %d: %w", roomID, err))
			continue
		}
		if err := runAgentRoom(ctx, db, room, now); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			roomErrs = append(roomErrs, fmt.Errorf("agent room %d: %w", roomID, err))
		}
	}
	return errors.Join(roomErrs...)
}

func runAgentRoom(ctx context.Context, db *pgxpool.Pool, room *Room, now time.Time) error {
	curDay, ended, err := room.CurrentDay(now)
	if err != nil {
		return err
	}
	lastDecisionDay := min(curDay, room.Days-2)

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := lockRoomTx(ctx, tx, room.ID); err != nil {
			return err
		}
		instruments, err := agentInstruments(ctx, tx, room.ScenarioID)
		if err != nil {
			return err
		}
		if len(instruments) == 0 {
			return nil
		}
		agents, err := roomAgents(ctx, tx, room.ID)
		if err != nil {
			return err
		}
		// Migrations intentionally leave already-completed historical rooms
		// untouched. Rooms that ended after agents joined still catch up below.
		if ended && len(agents) == 0 {
			return nil
		}
		if len(agents) != AgentPlayerCount {
			return fmt.Errorf("found %d agents, want %d", len(agents), AgentPlayerCount)
		}
		joinedDay := agents[0].joinedDay
		for _, agent := range agents[1:] {
			if agent.joinedDay != joinedDay {
				return fmt.Errorf("agents have inconsistent joined days")
			}
		}
		var lastTurnDay int
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(max(day), $2 - 1) FROM agent_turns WHERE room_id = $1`,
			room.ID, joinedDay).Scan(&lastTurnDay); err != nil {
			return err
		}
		// Settle the room monotonically at today's watermark before reading
		// agent resources. Never call SettleTx with a historical day here: an
		// HTTP request may already have accrued human loans through curDay.
		if err := SettleTx(ctx, tx, room, curDay, ended); err != nil {
			return err
		}
		// Catch up decisions chronologically when the process was stopped for
		// multiple sim days. An overdue order still carries its historical
		// exec_day, so settling at curDay fills it at the correct historical
		// open without rewinding any room-level settlement watermarks.
		for day := max(joinedDay, lastTurnDay+1); day <= lastDecisionDay; day++ {
			for _, agent := range agents {
				if err := takeAgentTurn(ctx, tx, room, day, instruments, agent); err != nil {
					return err
				}
			}
			if day < curDay {
				if err := SettleTx(ctx, tx, room, curDay, ended); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func agentInstruments(ctx context.Context, q Querier, scenarioID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id FROM instruments WHERE scenario_id = $1 ORDER BY ord`, scenarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func roomAgents(ctx context.Context, q Querier, roomID int64) ([]agentPlayer, error) {
	rows, err := q.Query(ctx, `
		SELECT u.id, u.agent_slot, u.agent_name, rp.joined_day
		FROM room_players rp JOIN users u ON u.id = rp.user_id
		WHERE rp.room_id = $1 AND u.is_agent
		ORDER BY u.agent_slot`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []agentPlayer
	for rows.Next() {
		var a agentPlayer
		if err := rows.Scan(&a.userID, &a.slot, &a.name, &a.joinedDay); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func takeAgentTurn(ctx context.Context, tx pgx.Tx, room *Room, day int, instruments []string, agent agentPlayer) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO agent_turns (room_id, user_id, day)
		VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, room.ID, agent.userID, day)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}

	var cash int64
	var bankrupt bool
	if err := tx.QueryRow(ctx, `
		SELECT cash_cents, bankrupt_day IS NOT NULL FROM room_players
		WHERE room_id = $1 AND user_id = $2 FOR UPDATE`, room.ID, agent.userID).
		Scan(&cash, &bankrupt); err != nil {
		return err
	}
	if bankrupt {
		return nil
	}

	instrumentID := instruments[(int(room.Seed%uint64(len(instruments)))+agent.slot*7+day*3)%len(instruments)]
	side := "buy"
	var shares float64
	// Every fourth turn realizes part of an existing position. If the agent
	// has nothing to sell yet it falls back to its deterministic buy.
	if (day+agent.slot)%4 == 0 {
		err := tx.QueryRow(ctx, `
			SELECT instrument_id, shares FROM positions
			WHERE room_id = $1 AND user_id = $2 AND shares > 0
			ORDER BY shares DESC, instrument_id LIMIT 1`, room.ID, agent.userID).
			Scan(&instrumentID, &shares)
		if err == nil {
			side = "sell"
			shares /= 3
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}

	var orderID int64
	if side == "buy" {
		amount := cash / 6 // ~16.7% of available cash, below the whale threshold
		if amount < minimumAgentOrderCents {
			return nil
		}
		if _, err := tx.Exec(ctx, `
			UPDATE room_players SET cash_cents = cash_cents - $1
			WHERE room_id = $2 AND user_id = $3`, amount, room.ID, agent.userID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO orders (room_id, user_id, instrument_id, side, amount_cents, exec_day)
			VALUES ($1, $2, $3, 'buy', $4, $5) RETURNING id`,
			room.ID, agent.userID, instrumentID, amount, day+1).Scan(&orderID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE positions SET shares = shares - $1
			WHERE room_id = $2 AND user_id = $3 AND instrument_id = $4`,
			shares, room.ID, agent.userID, instrumentID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO orders (room_id, user_id, instrument_id, side, shares, exec_day)
			VALUES ($1, $2, $3, 'sell', $4, $5) RETURNING id`,
			room.ID, agent.userID, instrumentID, shares, day+1).Scan(&orderID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE agent_turns SET order_id = $1, instrument_id = $2, side = $3
		WHERE room_id = $4 AND user_id = $5 AND day = $6`,
		orderID, instrumentID, side, room.ID, agent.userID, day); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO room_events (room_id, day, kind, payload)
		VALUES ($1, $2, 'agent_order', jsonb_build_object(
			'username', $3::text, 'is_agent', true,
			'instrument_id', $4::text, 'side', $5::text, 'order_id', $6::bigint))`,
		room.ID, day, agent.name, instrumentID, side, orderID)
	return err
}
