package store

import (
	"context"
	"testing"
	"time"
)

func TestPushTokenRoundtrip(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	u := mkUser(t, pool, "pusher")

	if err := AddPushToken(ctx, pool, u.ID, "ExponentPushToken[aaa]", "en"); err != nil {
		t.Fatalf("AddPushToken: %v", err)
	}
	// Idempotent re-register.
	if err := AddPushToken(ctx, pool, u.ID, "ExponentPushToken[aaa]", "zh"); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if err := AddPushToken(ctx, pool, u.ID, "ExponentPushToken[bbb]", "en"); err != nil {
		t.Fatalf("second device: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM push_tokens WHERE user_id = $1`, u.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("tokens = %d, want 2", n)
	}
	var lang string
	if err := pool.QueryRow(ctx, `SELECT lang FROM push_tokens WHERE user_id = $1 AND token = $2`, u.ID, "ExponentPushToken[aaa]").Scan(&lang); err != nil || lang != "zh" {
		t.Fatalf("updated language = %q, err=%v", lang, err)
	}

	if err := RemovePushToken(ctx, pool, u.ID, "ExponentPushToken[aaa]"); err != nil {
		t.Fatalf("RemovePushToken: %v", err)
	}
	// Removing an unknown token is a no-op.
	if err := RemovePushToken(ctx, pool, u.ID, "ExponentPushToken[nope]"); err != nil {
		t.Fatalf("remove unknown: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM push_tokens WHERE user_id = $1`, u.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("tokens after remove = %d, want 1", n)
	}
}

func TestPushTokensForRoom(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, _ := mkRunningRoom(t, pool)

	// A second member with two devices; the guest with one; an outsider with one.
	other := mkUser(t, pool, "other")
	if _, err := JoinRoom(ctx, pool, room, other.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	outsider := mkUser(t, pool, "outsider")

	for _, tok := range []string{"t-other-a", "t-other-b"} {
		if err := AddPushToken(ctx, pool, other.ID, tok, "zh"); err != nil {
			t.Fatal(err)
		}
	}
	if err := AddPushToken(ctx, pool, guest.ID, "t-guest", "en"); err != nil {
		t.Fatal(err)
	}
	if err := AddPushToken(ctx, pool, outsider.ID, "t-outsider", "en"); err != nil {
		t.Fatal(err)
	}

	toks, err := PushTokensForRoom(ctx, pool, room.ID, guest.ID)
	if err != nil {
		t.Fatalf("PushTokensForRoom: %v", err)
	}
	if len(toks) != 2 { // other's two devices; guest excluded; outsider not a member
		t.Fatalf("tokens = %v, want other's two", toks)
	}
	for _, tok := range toks {
		if tok.Lang != "zh" {
			t.Fatalf("other token language = %q", tok.Lang)
		}
	}

	toks, err = PushTokensForRoom(ctx, pool, room.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(toks) != 3 {
		t.Fatalf("tokens = %v, want 3", toks)
	}
}
