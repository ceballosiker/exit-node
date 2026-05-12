package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// tokenIssuer is the standard OAuth client-credentials endpoint stub.
// Every test server mux-routes /api/v2/oauth/token to this handler.
func tokenIssuer(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	// Don't bother validating client_id/secret here — that's the
	// Tailscale library's job. Return a valid token response.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "fake-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
}

// newTestClient wires a tsClient to an httptest server. Tests provide
// the mux; tokenIssuer is registered for /api/v2/oauth/token here so
// every test gets OAuth for free.
func newTestClient(t *testing.T, mux *http.ServeMux) (*httptest.Server, *tsClient) {
	t.Helper()
	mux.HandleFunc("/api/v2/oauth/token", tokenIssuer)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := New(Options{
		Tailnet:      "test.example.com",
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		BaseURL:      srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv, c.(*tsClient)
}

func TestNewRequiresCredentials(t *testing.T) {
	cases := []struct {
		name string
		opts Options
	}{
		{"missing tailnet", Options{ClientID: "x", ClientSecret: "y"}},
		{"missing client id", Options{Tailnet: "x", ClientSecret: "y"}},
		{"missing client secret", Options{Tailnet: "x", ClientID: "y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.opts); err == nil {
				t.Errorf("expected error")
			}
		})
	}
}

// Silence "imported and not used" for early tasks; later tasks use these.
var (
	_ = errors.New
	_ = url.Parse
	_ = strings.TrimSpace
	_ = time.Now
	_ = context.Background
)
