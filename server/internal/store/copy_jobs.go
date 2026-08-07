package store

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/engine"
)

const CopyLookaheadDays = 5

const CopyMaxAttempts = 5

// CopyFillBudget bounds one simulation day's background LLM work. Unlike the
// former room-creation budget, expiration never fails or removes a room: the
// job is released with backoff and its bilingual template copy stays usable.
var CopyFillBudget = 2 * time.Minute

type copyJob struct {
	roomID  int64
	day     int
	attempt int
}

type newsCopyRow struct {
	id    int64
	event engine.NewsEvent
}

type forumCopyRow struct {
	id   int64
	post engine.ForumPost
}

// RunNextCopyJob claims and processes at most one simulation day. Waiting
// rooms expose days 0..4 to the queue; running rooms keep the same five-day
// rolling window from their current day. The bool reports whether a job was
// claimed, allowing callers to drain or pace the queue deliberately.
func RunNextCopyJob(ctx context.Context, db *pgxpool.Pool, filler NewsCopyFiller, now time.Time) (bool, error) {
	if filler == nil {
		return false, nil
	}
	job, err := claimCopyJob(ctx, db, now)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := runCopyJob(ctx, db, filler, now, job); err != nil {
		failedAt := time.Now().UTC()
		if releaseErr := releaseCopyJob(context.WithoutCancel(ctx), db, failedAt, job); releaseErr != nil {
			return true, fmt.Errorf("copy job %d/%d: %w (release: %v)", job.roomID, job.day, err, releaseErr)
		}
		return true, fmt.Errorf("copy job %d/%d: %w", job.roomID, job.day, err)
	}
	return true, nil
}

func claimCopyJob(ctx context.Context, db *pgxpool.Pool, now time.Time) (copyJob, error) {
	var job copyJob
	staleBefore := now.Add(-CopyFillBudget - time.Minute)
	err := db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT j.room_id, j.day
			FROM room_copy_jobs j
			JOIN rooms r ON r.id = j.room_id
			WHERE (
				(j.status = 'pending' AND j.available_at <= $1)
				OR (j.status = 'running' AND j.claimed_at < $2)
			)
			AND (
				(r.status = 'lobby' AND j.day < $3)
				OR (r.status = 'running'
					AND j.day >= FLOOR(GREATEST(EXTRACT(EPOCH FROM ($1 - r.started_at)), 0) / r.day_duration_secs)::INT
					AND j.day <= LEAST(
						r.days - 1,
						FLOOR(GREATEST(EXTRACT(EPOCH FROM ($1 - r.started_at)), 0) / r.day_duration_secs)::INT + $3 - 1
					)
				)
			)
			ORDER BY
				CASE
					WHEN r.status = 'lobby' THEN j.day
					ELSE j.day - FLOOR(GREATEST(EXTRACT(EPOCH FROM ($1 - r.started_at)), 0) / r.day_duration_secs)::INT
				END,
				j.attempts, j.available_at, j.room_id, j.day
			FOR UPDATE OF j SKIP LOCKED
			LIMIT 1
		)
		UPDATE room_copy_jobs j
		SET status = 'running', claimed_at = $1, attempts = j.attempts + 1
		FROM candidate c
		WHERE j.room_id = c.room_id AND j.day = c.day
		RETURNING j.room_id, j.day, j.attempts`, now, staleBefore, CopyLookaheadDays).
		Scan(&job.roomID, &job.day, &job.attempt)
	return job, err
}

func runCopyJob(ctx context.Context, db *pgxpool.Pool, filler NewsCopyFiller, now time.Time, job copyJob) error {
	room, err := GetRoom(ctx, db, job.roomID)
	if err != nil {
		return err
	}
	sc, err := LoadScenario(ctx, db, room.ScenarioID)
	if err != nil {
		return err
	}
	for i := range sc.Instruments {
		inst := &sc.Instruments[i]
		inst.Alias = engine.ResolveAlias(room.Seed, inst.ID, inst.Alias, inst.Aliases)
	}

	news, err := loadNewsCopyRows(ctx, db, job)
	if err != nil {
		return err
	}
	forum, err := loadForumCopyRows(ctx, db, job)
	if err != nil {
		return err
	}

	fctx, cancel := context.WithTimeout(ctx, CopyFillBudget)
	defer cancel()
	if len(news) > 0 {
		events := make([]engine.NewsEvent, len(news))
		for i := range news {
			events[i] = news[i].event
		}
		filler.FillCopy(fctx, sc, events)
		for i := range news {
			news[i].event = events[i]
		}
	}
	if ff, ok := filler.(ForumCopyFiller); ok && len(forum) > 0 {
		posts := make([]engine.ForumPost, len(forum))
		for i := range forum {
			posts[i] = forum[i].post
		}
		ff.FillForumCopy(fctx, sc, posts)
		for i := range forum {
			forum[i].post = posts[i]
		}
	}
	if err := fctx.Err(); err != nil {
		return err
	}

	complete := true
	for i := range news {
		if news[i].event.Body == "" || news[i].event.BodyEn == "" {
			complete = false
		}
	}
	applied, retrying, obsolete, err := saveCopyJob(ctx, db, time.Now().UTC(), job, news, forum, complete)
	if err != nil {
		return err
	}
	if obsolete {
		log.Printf("copy: room=%d day=%d skipped after day advanced (attempt=%d)", job.roomID, job.day, job.attempt)
		return nil
	}
	if !applied {
		log.Printf("copy: room=%d day=%d discarded stale attempt=%d", job.roomID, job.day, job.attempt)
		return nil
	}
	if complete {
		log.Printf("copy: room=%d day=%d ready (news=%d forum=%d)", job.roomID, job.day, len(news), len(forum))
	} else if retrying {
		log.Printf("copy: room=%d day=%d incomplete; retry scheduled (attempt=%d)", job.roomID, job.day, job.attempt)
	} else {
		log.Printf("copy: room=%d day=%d incomplete after %d attempts; keeping template copy", job.roomID, job.day, job.attempt)
	}
	return nil
}

func loadNewsCopyRows(ctx context.Context, db *pgxpool.Pool, job copyJob) ([]newsCopyRow, error) {
	rows, err := db.Query(ctx, `
		SELECT id, day, media_id, track, true_shock, report_shock,
			headline, body, headline_en, body_en, cluster_id, is_recap, copy_role
		FROM room_news WHERE room_id = $1 AND day = $2 ORDER BY id`, job.roomID, job.day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []newsCopyRow{}
	for rows.Next() {
		var row newsCopyRow
		var track string
		if err := rows.Scan(&row.id, &row.event.Day, &row.event.MediaID, &track,
			&row.event.TrueShock, &row.event.ReportShock, &row.event.Headline,
			&row.event.Body, &row.event.HeadlineEn, &row.event.BodyEn,
			&row.event.ClusterID, &row.event.Recap, &row.event.CopyRole); err != nil {
			return nil, err
		}
		row.event.Track = engine.Track(track)
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadForumCopyRows(ctx context.Context, db *pgxpool.Pool, job copyJob) ([]forumCopyRow, error) {
	rows, err := db.Query(ctx, `
		SELECT id, day, npc_name, body, npc_name_en, body_en, is_agent, persona
		FROM room_forum_posts WHERE room_id = $1 AND day = $2 ORDER BY id`, job.roomID, job.day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []forumCopyRow{}
	for rows.Next() {
		var row forumCopyRow
		if err := rows.Scan(&row.id, &row.post.Day, &row.post.NPCName, &row.post.Body,
			&row.post.NPCNameEn, &row.post.BodyEn, &row.post.IsAgent, &row.post.Persona); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func saveCopyJob(ctx context.Context, db *pgxpool.Pool, now time.Time, job copyJob, news []newsCopyRow, forum []forumCopyRow, complete bool) (applied, retrying, obsolete bool, err error) {
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		// Fence the lease before touching copy. If another worker reclaimed this
		// job, its incremented attempt owns the row and this result is discarded.
		var status, roomStatus string
		var attempt, currentDay int
		if err := tx.QueryRow(ctx, `
			SELECT j.status, j.attempts, r.status,
				CASE WHEN r.status = 'running' THEN
					FLOOR(GREATEST(EXTRACT(EPOCH FROM ($3 - r.started_at)), 0) / r.day_duration_secs)::INT
				ELSE 0 END
			FROM room_copy_jobs j
			JOIN rooms r ON r.id = j.room_id
			WHERE j.room_id = $1 AND j.day = $2
			FOR UPDATE OF j`, job.roomID, job.day, now).Scan(&status, &attempt, &roomStatus, &currentDay); err != nil {
			return err
		}
		if status != "running" || attempt != job.attempt {
			return nil
		}
		if roomStatus == "running" && job.day < currentDay {
			_, err := tx.Exec(ctx, `
				UPDATE room_copy_jobs SET status = 'skipped', claimed_at = NULL
				WHERE room_id = $1 AND day = $2 AND status = 'running' AND attempts = $3`,
				job.roomID, job.day, job.attempt)
			obsolete = err == nil
			return err
		}
		applied = true
		for _, row := range news {
			if _, err := tx.Exec(ctx, `
				UPDATE room_news SET headline = $2, body = $3, headline_en = $4, body_en = $5
				WHERE id = $1 AND room_id = $6`, row.id, row.event.Headline, row.event.Body,
				row.event.HeadlineEn, row.event.BodyEn, job.roomID); err != nil {
				return err
			}
		}
		for _, row := range forum {
			if _, err := tx.Exec(ctx, `
				UPDATE room_forum_posts SET body = $2, body_en = $3
				WHERE id = $1 AND room_id = $4`, row.id, row.post.Body, row.post.BodyEn, job.roomID); err != nil {
				return err
			}
		}
		if complete {
			_, err := tx.Exec(ctx, `
				UPDATE room_copy_jobs SET status = 'done', completed_at = $3
				WHERE room_id = $1 AND day = $2 AND status = 'running' AND attempts = $4`,
				job.roomID, job.day, now, job.attempt)
			return err
		}
		var scheduleErr error
		retrying, scheduleErr = scheduleCopyRetry(ctx, tx, now, job)
		return scheduleErr
	})
	return applied, retrying, obsolete, err
}

func releaseCopyJob(ctx context.Context, db *pgxpool.Pool, now time.Time, job copyJob) error {
	_, err := scheduleCopyRetry(ctx, db, now, job)
	return err
}

func scheduleCopyRetry(ctx context.Context, q Querier, now time.Time, job copyJob) (bool, error) {
	if job.attempt >= CopyMaxAttempts {
		_, err := q.Exec(ctx, `
			UPDATE room_copy_jobs
			SET status = 'failed', claimed_at = NULL
			WHERE room_id = $1 AND day = $2 AND status = 'running' AND attempts = $3`,
			job.roomID, job.day, job.attempt)
		return false, err
	}
	backoff := 15 * time.Second
	for i := 1; i < job.attempt && backoff < 10*time.Minute; i++ {
		backoff *= 2
	}
	if backoff > 10*time.Minute {
		backoff = 10 * time.Minute
	}
	_, err := q.Exec(ctx, `
		UPDATE room_copy_jobs
		SET status = 'pending', available_at = $3, claimed_at = NULL
		WHERE room_id = $1 AND day = $2 AND status = 'running' AND attempts = $4`,
		job.roomID, job.day, now.Add(backoff), job.attempt)
	return true, err
}
