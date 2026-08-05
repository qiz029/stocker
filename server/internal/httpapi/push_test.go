package httpapi

import (
	"context"
	"net/http"
	"testing"
)

func TestPushTokenEndpoints(t *testing.T) {
	s := newServer(t)
	c := registerClient(t, s, "pusher")

	c.mustJSON("POST", "/api/me/push-token",
		map[string]any{"token": "ExponentPushToken[aaa]"}, http.StatusOK)
	// Idempotent re-register.
	c.mustJSON("POST", "/api/me/push-token",
		map[string]any{"token": "ExponentPushToken[aaa]"}, http.StatusOK)

	var n int
	if err := s.DB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM push_tokens WHERE user_id =
			(SELECT id FROM users WHERE username = 'pusher')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("tokens = %d, want 1", n)
	}

	// Bad tokens rejected.
	c.mustJSON("POST", "/api/me/push-token", map[string]any{"token": "  "}, http.StatusBadRequest)

	// Delete via query param.
	c.mustJSON("DELETE", "/api/me/push-token?token=ExponentPushToken[aaa]", nil, http.StatusOK)
	if err := s.DB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM push_tokens WHERE user_id =
			(SELECT id FROM users WHERE username = 'pusher')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("tokens after delete = %d, want 0", n)
	}
}

func TestPushTokenRequiresAuth(t *testing.T) {
	s := newServer(t)
	c := newClient(t, s)
	c.mustJSON("POST", "/api/me/push-token",
		map[string]any{"token": "ExponentPushToken[aaa]"}, http.StatusUnauthorized)
}
