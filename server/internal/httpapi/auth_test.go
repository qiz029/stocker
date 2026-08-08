package httpapi

import (
	"net/http"
	"strings"
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

func TestExtendedProfileAndPasswordChange(t *testing.T) {
	s := newServer(t)
	c := newClient(t, s)
	c.mustJSON("POST", "/api/register",
		map[string]any{"username": "profile_user", "password": "password123"}, http.StatusOK)

	profile := map[string]any{
		"display_name": "Market Owl",
		"avatar_id":    "owl",
		"email":        "owl@example.com",
		"description":  "Long-term investor. Short-term skeptic.",
		"social_links": map[string]string{
			"website":  "https://market-owl.example",
			"x":        "https://x.com/market_owl",
			"github":   "https://github.com/market-owl",
			"linkedin": "https://www.linkedin.com/in/market-owl",
		},
	}
	got := c.mustJSON("PUT", "/api/me/profile", profile, http.StatusOK)
	if got["display_name"] != "Market Owl" || got["email"] != "owl@example.com" || got["description"] != profile["description"] {
		t.Fatalf("extended profile response: %v", got)
	}
	links, ok := got["social_links"].(map[string]any)
	if !ok || links["github"] != "https://github.com/market-owl" {
		t.Fatalf("social links response: %v", got["social_links"])
	}
	got = c.mustJSON("GET", "/api/me", nil, http.StatusOK)
	if got["email"] != "owl@example.com" {
		t.Fatalf("persisted profile: %v", got)
	}

	resp, _ := c.do("PUT", "/api/me/password", map[string]any{
		"current_password": "wrong-password", "new_password": "new-password-456",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong current password: status %d, want 400", resp.StatusCode)
	}
	c.mustJSON("PUT", "/api/me/password", map[string]any{
		"current_password": "password123", "new_password": "new-password-456",
	}, http.StatusOK)
	c.mustJSON("POST", "/api/logout", nil, http.StatusOK)

	resp, _ = c.do("POST", "/api/login", map[string]any{"username": "profile_user", "password": "password123"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password after change: status %d, want 401", resp.StatusCode)
	}
	c.mustJSON("POST", "/api/login",
		map[string]any{"username": "profile_user", "password": "new-password-456"}, http.StatusOK)
}

func TestAliasMustBeUniqueIgnoringCase(t *testing.T) {
	s := newServer(t)
	first := registerIncompleteClient(t, s, "alias_owner")
	second := registerIncompleteClient(t, s, "alias_challenger")

	first.mustJSON("PUT", "/api/me/profile", map[string]any{
		"display_name": "Market Owl", "avatar_id": "owl",
	}, http.StatusOK)
	resp, body := second.do("PUT", "/api/me/profile", map[string]any{
		"display_name": "market owl", "avatar_id": "fox",
	})
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(body), "alias already in use") {
		t.Fatalf("duplicate alias: status %d body %s", resp.StatusCode, body)
	}
	resp, body = second.do("PUT", "/api/me/profile", map[string]any{
		"display_name": "Seattle Value Sage", "avatar_id": "fox",
	})
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(body), "alias already in use") {
		t.Fatalf("agent alias: status %d body %s", resp.StatusCode, body)
	}
}

func TestExtendedProfileValidation(t *testing.T) {
	tests := []struct {
		name string
		req  profileRequest
	}{
		{name: "bad email", req: profileRequest{DisplayName: "Market Owl", AvatarID: "owl", Email: ptr("not-an-email")}},
		{name: "long description", req: profileRequest{DisplayName: "Market Owl", AvatarID: "owl", Description: ptr(strings.Repeat("x", 501))}},
		{name: "unknown social network", req: profileRequest{DisplayName: "Market Owl", AvatarID: "owl", SocialLinks: &map[string]string{"mastodon": "https://example.social/@owl"}}},
		{name: "non https social link", req: profileRequest{DisplayName: "Market Owl", AvatarID: "owl", SocialLinks: &map[string]string{"website": "javascript:alert(1)"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateProfileRequest(&tt.req); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func ptr(value string) *string { return &value }
