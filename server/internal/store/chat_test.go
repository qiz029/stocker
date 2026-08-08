package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestChatPostAndSince(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()
	room, guest, _ := mkRunningRoom(t, pool)
	setTestAlias(t, pool, guest, "Guest Bear")

	id1, err := PostChat(ctx, pool, room, guest.ID, 0, "大家好")
	if err != nil {
		t.Fatalf("PostChat: %v", err)
	}
	id2, err := PostChat(ctx, pool, room, guest.ID, 3, "  科技股要起飞了  ")
	if err != nil || id2 <= id1 {
		t.Fatalf("second PostChat: id=%d err=%v", id2, err)
	}

	msgs, err := ChatSince(ctx, pool, room.ID, 0, 100)
	if err != nil {
		t.Fatalf("ChatSince: %v", err)
	}
	if len(msgs) != 2 || msgs[0].ID != id1 || msgs[1].ID != id2 {
		t.Fatalf("messages: %+v", msgs)
	}
	if msgs[1].Text != "科技股要起飞了" { // trimmed
		t.Fatalf("text not trimmed: %q", msgs[1].Text)
	}
	if msgs[0].Username != "Guest Bear" || msgs[0].Day != 0 || msgs[1].Day != 3 {
		t.Fatalf("metadata: %+v", msgs)
	}

	// Incremental fetch.
	tail, err := ChatSince(ctx, pool, room.ID, id1, 100)
	if err != nil || len(tail) != 1 || tail[0].ID != id2 {
		t.Fatalf("incremental: %+v err=%v", tail, err)
	}

	// Validation.
	if _, err := PostChat(ctx, pool, room, guest.ID, 0, "   "); !errors.Is(err, ErrBadChatMessage) {
		t.Fatalf("blank message: %v", err)
	}
	if _, err := PostChat(ctx, pool, room, guest.ID, 0, strings.Repeat("啊", 501)); !errors.Is(err, ErrBadChatMessage) {
		t.Fatalf("oversized message: %v", err)
	}
	if _, err := PostChat(ctx, pool, room, guest.ID, 0, strings.Repeat("a", 500)); err != nil {
		t.Fatalf("500 runes should be allowed: %v", err)
	}
}
