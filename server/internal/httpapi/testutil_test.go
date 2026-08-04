package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/toddzheng/stocker/server/internal/store"
)

func newServer(t *testing.T) *Server {
	t.Helper()
	return NewServer(store.TestDB(t, "httpapi"))
}

// client is a minimal cookie-carrying test client against Server.Router().
type client struct {
	t       *testing.T
	h       http.Handler
	cookies map[string]*http.Cookie
}

func newClient(t *testing.T, s *Server) *client {
	return &client{t: t, h: s.Router(), cookies: map[string]*http.Cookie{}}
}

func (c *client) do(method, path string, body any) (*http.Response, []byte) {
	c.t.Helper()
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal body: %v", err)
		}
		buf = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	resp := rec.Result()
	for _, ck := range resp.Cookies() {
		if ck.MaxAge < 0 {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck
		}
	}
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func (c *client) mustJSON(method, path string, body any, wantStatus int) map[string]any {
	c.t.Helper()
	resp, data := c.do(method, path, body)
	if resp.StatusCode != wantStatus {
		c.t.Fatalf("%s %s: status %d, want %d; body: %s", method, path, resp.StatusCode, wantStatus, data)
	}
	out := map[string]any{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &out); err != nil {
			c.t.Fatalf("%s %s: bad JSON %q: %v", method, path, data, err)
		}
	}
	return out
}

func registerClient(t *testing.T, s *Server, username string) *client {
	t.Helper()
	c := registerIncompleteClient(t, s, username)
	c.mustJSON("PUT", "/api/me/profile",
		map[string]any{"display_name": username, "avatar_id": "bull"}, http.StatusOK)
	return c
}

func registerIncompleteClient(t *testing.T, s *Server, username string) *client {
	t.Helper()
	c := newClient(t, s)
	c.mustJSON("POST", "/api/register",
		map[string]any{"username": username, "password": "password123"}, http.StatusOK)
	return c
}

func storeSetDisplayForTest(s *Server, scenarioID string) error {
	return store.SetInstrumentDisplay(context.Background(), s.DB, scenarioID, map[string]store.InstrumentDisplay{
		"S1": {Alias: "Ridgeline Networks", Desc: "网络设备巨头", Business: "路由器", Bull: "卖铲人", Bear: "客户烧钱"},
	})
}
