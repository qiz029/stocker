package httpapi

import (
	"net/http"
	"testing"
)

func TestRegisterLoginMeLogout(t *testing.T) {
	s := newServer(t)
	c := newClient(t, s)

	// Unauthenticated /me is rejected.
	resp, _ := c.do("GET", "/api/me", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me before register: %d", resp.StatusCode)
	}

	got := c.mustJSON("POST", "/api/register",
		map[string]any{"username": "alice", "password": "password123"}, http.StatusOK)
	if got["username"] != "alice" {
		t.Fatalf("register response: %v", got)
	}

	// Register logs you in (cookie captured by client).
	got = c.mustJSON("GET", "/api/me", nil, http.StatusOK)
	if got["username"] != "alice" {
		t.Fatalf("me: %v", got)
	}

	// Duplicate username.
	c2 := newClient(t, s)
	resp, _ = c2.do("POST", "/api/register",
		map[string]any{"username": "alice", "password": "password456"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate register: %d", resp.StatusCode)
	}

	// Bad credentials.
	resp, _ = c2.do("POST", "/api/login",
		map[string]any{"username": "alice", "password": "wrong-password"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", resp.StatusCode)
	}

	// Good login on a fresh client.
	c2.mustJSON("POST", "/api/login",
		map[string]any{"username": "alice", "password": "password123"}, http.StatusOK)
	c2.mustJSON("GET", "/api/me", nil, http.StatusOK)

	// Logout invalidates the session server-side.
	c2.mustJSON("POST", "/api/logout", nil, http.StatusOK)
	resp, _ = c2.do("GET", "/api/me", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout: %d", resp.StatusCode)
	}
}

func TestRegisterValidation(t *testing.T) {
	s := newServer(t)
	c := newClient(t, s)
	for _, bad := range []map[string]any{
		{"username": "ab", "password": "password123"},        // too short
		{"username": "has space", "password": "password123"}, // bad charset
		{"username": "alice", "password": "short"},           // weak password
	} {
		resp, _ := c.do("POST", "/api/register", bad)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("register %v: status %d, want 400", bad, resp.StatusCode)
		}
	}
}
