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
	_ = time.Now
)

func TestMintEphemeralAuthKey_Success(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/tailnet/test.example.com/keys",
		func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":  "key-id-1",
				"key": "tskey-auth-xxxxxx",
			})
		})

	_, c := newTestClient(t, mux)
	got, err := c.MintEphemeralAuthKey(context.Background(), []string{"tag:exit-node"})
	if err != nil {
		t.Fatalf("MintEphemeralAuthKey: %v", err)
	}
	if got != "tskey-auth-xxxxxx" {
		t.Errorf("key = %q", got)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %s", gotMethod)
	}
	if gotPath != "/api/v2/tailnet/test.example.com/keys" {
		t.Errorf("path = %s", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("auth header = %q, want Bearer prefix", gotAuth)
	}

	// Body shape — drill into capabilities.devices.create:
	caps, _ := gotBody["capabilities"].(map[string]any)
	devs, _ := caps["devices"].(map[string]any)
	create, _ := devs["create"].(map[string]any)
	if v, _ := create["ephemeral"].(bool); !v {
		t.Errorf("ephemeral != true (body=%v)", gotBody)
	}
	if v, _ := create["preauthorized"].(bool); !v {
		t.Errorf("preauthorized != true")
	}
	if v, _ := create["reusable"].(bool); v {
		t.Errorf("reusable should be false")
	}
	tags, _ := create["tags"].([]any)
	if len(tags) != 1 || tags[0] != "tag:exit-node" {
		t.Errorf("tags = %v", tags)
	}
	if exp, _ := gotBody["expirySeconds"].(float64); exp != 300 {
		t.Errorf("expirySeconds = %v, want 300", exp)
	}
}

func TestMintEphemeralAuthKey_APIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/tailnet/test.example.com/keys",
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"bad scope"}`))
		})
	_, c := newTestClient(t, mux)
	if _, err := c.MintEphemeralAuthKey(context.Background(), nil); err == nil {
		t.Errorf("expected error")
	}
}
