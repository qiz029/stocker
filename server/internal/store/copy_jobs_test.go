package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/toddzheng/stocker/server/internal/engine"
	"github.com/toddzheng/stocker/server/internal/scenario"
)

type completeCopyFiller struct{}

func (completeCopyFiller) FillCopy(_ context.Context, _ *scenario.Scenario, events []engine.NewsEvent) {
	for i := range events {
		events[i].Headline = "生成标题"
		events[i].Body = "生成正文。"
		events[i].HeadlineEn = "Generated headline"
		events[i].BodyEn = "Generated body."
	}
}

type emptyCopyFiller struct{}

func (emptyCopyFiller) FillCopy(_ context.Context, _ *scenario.Scenario, _ []engine.NewsEvent) {}

func TestCopyJobsUseFiveDayRollingWindow(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "copyhost")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 60, completeCopyFiller{})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	for i := 0; i < CopyLookaheadDays; i++ {
		worked, err := RunNextCopyJob(ctx, pool, completeCopyFiller{}, now)
		if err != nil || !worked {
			t.Fatalf("lobby job %d: worked=%v err=%v", i, worked, err)
		}
	}
	if worked, err := RunNextCopyJob(ctx, pool, completeCopyFiller{}, now); err != nil || worked {
		t.Fatalf("job beyond lobby window: worked=%v err=%v", worked, err)
	}
	assertDoneCopyRange(t, pool, room.ID, CopyLookaheadDays, 0, CopyLookaheadDays-1)

	started, err := StartRoom(ctx, pool, room.ID, host.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	room = started
	// Current day 3 exposes a five-day window through day 7. Days 0..4 are
	// already warm, so exactly three additional jobs become eligible.
	atDay3 := now.Add(3 * time.Minute)
	for i := 0; i < 3; i++ {
		worked, err := RunNextCopyJob(ctx, pool, completeCopyFiller{}, atDay3)
		if err != nil || !worked {
			t.Fatalf("rolling job %d: worked=%v err=%v", i, worked, err)
		}
	}
	if worked, err := RunNextCopyJob(ctx, pool, completeCopyFiller{}, atDay3); err != nil || worked {
		t.Fatalf("job beyond rolling window: worked=%v err=%v", worked, err)
	}
	assertDoneCopyRange(t, pool, room.ID, 8, 0, 7)
}

func TestRunningCopyJobsNeverRewritePastDays(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "historyhost")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 60, completeCopyFiller{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := StartRoom(ctx, pool, room.ID, host.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE room_copy_jobs SET status = CASE WHEN day = 1 THEN 'pending' ELSE 'done' END
		WHERE room_id = $1`, room.ID); err != nil {
		t.Fatal(err)
	}
	if worked, err := RunNextCopyJob(ctx, pool, completeCopyFiller{}, now.Add(3*time.Minute)); err != nil || worked {
		t.Fatalf("past-day job: worked=%v err=%v", worked, err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM room_copy_jobs WHERE room_id = $1 AND day = 1`, room.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("past-day status = %s, want untouched pending", status)
	}
}

func TestIncompleteCopyJobRetriesWithoutFailingRoom(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "retryhost")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 60, emptyCopyFiller{})
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic day 0 may contain no news and is therefore trivially ready;
	// select day 1, which always has the daily recap and exercises retry.
	if _, err := pool.Exec(ctx, `UPDATE room_copy_jobs SET status = 'done' WHERE room_id = $1 AND day = 0`, room.ID); err != nil {
		t.Fatal(err)
	}
	claimAt := time.Now().UTC().Add(-time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE room_copy_jobs SET available_at = $2 WHERE room_id = $1 AND day = 1`, room.ID, claimAt.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	finishedAfter := time.Now().UTC()
	worked, err := RunNextCopyJob(ctx, pool, emptyCopyFiller{}, claimAt)
	if err != nil || !worked {
		t.Fatalf("RunNextCopyJob: worked=%v err=%v", worked, err)
	}
	var status string
	var attempts int
	var available time.Time
	if err := pool.QueryRow(ctx, `
		SELECT status, attempts, available_at FROM room_copy_jobs
		WHERE room_id = $1 AND day = 1`, room.ID).Scan(&status, &attempts, &available); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempts != 1 || available.Before(finishedAfter.Add(14*time.Second)) {
		t.Fatalf("retry state = %s attempts=%d available=%v", status, attempts, available)
	}
	if _, err := GetRoom(ctx, pool, room.ID); err != nil {
		t.Fatalf("copy failure removed room: %v", err)
	}
}

func TestCopyJobsWarmLobbyRoomsFairly(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	sc := mkScenario(t, pool)
	room1, err := CreateRoom(ctx, pool, sc, mkUser(t, pool, "fairhost1").ID, 60, completeCopyFiller{})
	if err != nil {
		t.Fatal(err)
	}
	room2, err := CreateRoom(ctx, pool, sc, mkUser(t, pool, "fairhost2").ID, 60, completeCopyFiller{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		if worked, err := RunNextCopyJob(ctx, pool, completeCopyFiller{}, now); err != nil || !worked {
			t.Fatalf("warm job %d: worked=%v err=%v", i, worked, err)
		}
	}
	for _, room := range []*Room{room1, room2} {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM room_copy_jobs WHERE room_id = $1 AND day = 0`, room.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "done" {
			t.Fatalf("room %d day 0 status = %s, want done", room.ID, status)
		}
	}
}

func TestStaleCopyWorkerCannotOverwriteReclaimedJob(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "stalehost")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 60, completeCopyFiller{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE room_copy_jobs SET status = 'done' WHERE room_id = $1 AND day = 0`, room.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	first, err := claimCopyJob(ctx, pool, now)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := loadNewsCopyRows(ctx, pool, first)
	if err != nil || len(rows) == 0 {
		t.Fatalf("load day copy: rows=%d err=%v", len(rows), err)
	}
	second, err := claimCopyJob(ctx, pool, now.Add(CopyFillBudget+2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if second.roomID != first.roomID || second.day != first.day || second.attempt != first.attempt+1 {
		t.Fatalf("reclaimed job = %+v, first = %+v", second, first)
	}
	rows[0].event.HeadlineEn = "stale worker"
	rows[0].event.BodyEn = "stale worker body"
	applied, _, _, err := saveCopyJob(ctx, pool, time.Now().UTC(), first, rows, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("stale attempt unexpectedly applied")
	}
	var status, headlineEn string
	var attempts int
	if err := pool.QueryRow(ctx, `
		SELECT j.status, j.attempts, n.headline_en
		FROM room_copy_jobs j
		JOIN room_news n ON n.room_id = j.room_id AND n.day = j.day
		WHERE j.room_id = $1 AND j.day = $2
		ORDER BY n.id LIMIT 1`, first.roomID, first.day).Scan(&status, &attempts, &headlineEn); err != nil {
		t.Fatal(err)
	}
	if status != "running" || attempts != second.attempt || headlineEn == "stale worker" {
		t.Fatalf("post-stale state = status %s attempts %d headline %q", status, attempts, headlineEn)
	}
}

func TestClaimedCopyJobCannotRewriteDayAfterItAdvances(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "rolloverhost")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 60, completeCopyFiller{})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().UTC()
	if _, err := StartRoom(ctx, pool, room.ID, host.ID, startedAt); err != nil {
		t.Fatal(err)
	}
	job, err := claimCopyJob(ctx, pool, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := loadNewsCopyRows(ctx, pool, job)
	if err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		rows[i].event.HeadlineEn = "late rewrite"
		rows[i].event.BodyEn = "late rewrite body"
	}
	applied, _, obsolete, err := saveCopyJob(ctx, pool, startedAt.Add(2*time.Minute), job, rows, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied || !obsolete {
		t.Fatalf("rollover save: applied=%v obsolete=%v", applied, obsolete)
	}
	var status string
	var lateCount int
	if err := pool.QueryRow(ctx, `
		SELECT j.status, COUNT(*) FILTER (WHERE n.headline_en = 'late rewrite')
		FROM room_copy_jobs j
		LEFT JOIN room_news n ON n.room_id = j.room_id AND n.day = j.day
		WHERE j.room_id = $1 AND j.day = $2
		GROUP BY j.status`, job.roomID, job.day).Scan(&status, &lateCount); err != nil {
		t.Fatal(err)
	}
	if status != "skipped" || lateCount != 0 {
		t.Fatalf("rollover state = status %s late rows %d", status, lateCount)
	}
}

func TestCopyJobStopsRetryingAfterMaxAttempts(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	host := mkUser(t, pool, "maxretryhost")
	sc := mkScenario(t, pool)
	room, err := CreateRoom(ctx, pool, sc, host.ID, 60, emptyCopyFiller{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE room_copy_jobs SET status = 'done' WHERE room_id = $1 AND day = 0`, room.ID); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= CopyMaxAttempts; attempt++ {
		if _, err := pool.Exec(ctx, `UPDATE room_copy_jobs SET available_at = now() - interval '1 second' WHERE room_id = $1 AND day = 1`, room.ID); err != nil {
			t.Fatal(err)
		}
		if worked, err := RunNextCopyJob(ctx, pool, emptyCopyFiller{}, time.Now().UTC()); err != nil || !worked {
			t.Fatalf("attempt %d: worked=%v err=%v", attempt, worked, err)
		}
	}
	var status string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT status, attempts FROM room_copy_jobs WHERE room_id = $1 AND day = 1`, room.ID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || attempts != CopyMaxAttempts {
		t.Fatalf("terminal job = status %s attempts %d", status, attempts)
	}
}

func assertDoneCopyRange(t *testing.T, pool *pgxpool.Pool, roomID int64, wantCount, wantMin, wantMax int) {
	t.Helper()
	var count, minDay, maxDay int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*), MIN(day), MAX(day) FROM room_copy_jobs
		WHERE room_id = $1 AND status = 'done'`, roomID).Scan(&count, &minDay, &maxDay); err != nil {
		t.Fatal(err)
	}
	if count != wantCount || minDay != wantMin || maxDay != wantMax {
		t.Fatalf("done jobs = count %d range %d..%d, want %d range %d..%d",
			count, minDay, maxDay, wantCount, wantMin, wantMax)
	}
}
