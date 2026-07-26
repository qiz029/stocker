package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUsersAndSessions(t *testing.T) {
	pool := TestDB(t, "store")
	ctx := context.Background()

	u, err := CreateUser(ctx, pool, "alice", "hash1")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 || u.Username != "alice" {
		t.Fatalf("bad user: %+v", u)
	}

	if _, err := CreateUser(ctx, pool, "alice", "hash2"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate username: got %v, want ErrUsernameTaken", err)
	}

	got, err := GetUserByUsername(ctx, pool, "alice")
	if err != nil || got.ID != u.ID || got.PasswordHash != "hash1" {
		t.Fatalf("GetUserByUsername: %+v, %v", got, err)
	}
	if _, err := GetUserByUsername(ctx, pool, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user: got %v, want ErrNotFound", err)
	}

	now := time.Now()
	if err := CreateSession(ctx, pool, u.ID, "tok1", now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	su, err := UserBySession(ctx, pool, "tok1", now)
	if err != nil || su.ID != u.ID {
		t.Fatalf("UserBySession: %+v, %v", su, err)
	}
	// Expired session is invisible.
	if _, err := UserBySession(ctx, pool, "tok1", now.Add(2*time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session: got %v, want ErrNotFound", err)
	}
	// Deleted session is invisible.
	if err := DeleteSession(ctx, pool, "tok1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := UserBySession(ctx, pool, "tok1", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session: got %v, want ErrNotFound", err)
	}
}
