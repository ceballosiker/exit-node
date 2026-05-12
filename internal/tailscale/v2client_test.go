package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
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

func TestWaitForDevice_FindsAfterDelay(t *testing.T) {
	var listCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/tailnet/test.example.com/devices",
		func(w http.ResponseWriter, r *http.Request) {
			n := atomic.AddInt32(&listCalls, 1)
			devices := []map[string]any{}
			if n >= 2 { // appear on second poll
				devices = []map[string]any{{
					"nodeId":    "node-1",
					"id":        "id-1",
					"name":      "vpn-us-west1-abc.test.example.com",
					"hostname":  "vpn-us-west1-abc",
					"addresses": []string{"100.64.0.9"},
				}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"devices": devices})
		})
	_, c := newTestClient(t, mux)
	c.pollInterval = 10 * time.Millisecond // fast poll for tests

	dev, err := c.WaitForDevice(context.Background(), "vpn-us-west1-abc", 2*time.Second)
	if err != nil {
		t.Fatalf("WaitForDevice: %v", err)
	}
	if dev == nil || dev.Hostname != "vpn-us-west1-abc" {
		t.Errorf("dev = %+v", dev)
	}
	if dev.TailscaleIP != "100.64.0.9" {
		t.Errorf("TailscaleIP = %q", dev.TailscaleIP)
	}
	if atomic.LoadInt32(&listCalls) < 2 {
		t.Errorf("expected >=2 list calls, got %d", listCalls)
	}
}

func TestWaitForDevice_Timeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/tailnet/test.example.com/devices",
		func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"devices": []any{}})
		})
	_, c := newTestClient(t, mux)
	c.pollInterval = 5 * time.Millisecond

	_, err := c.WaitForDevice(context.Background(), "missing", 50*time.Millisecond)
	if err == nil {
		t.Errorf("expected timeout error")
	}
}

// atomic import marker.
var _ = atomic.LoadInt32

func TestAuthorizeExitNode_CallsBothEndpoints(t *testing.T) {
	var setAuthorizedCalled, setRoutesCalled bool
	var routesBody map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/device/node-1/authorized",
		func(w http.ResponseWriter, r *http.Request) {
			setAuthorizedCalled = true
			w.WriteHeader(http.StatusOK)
		})
	mux.HandleFunc("/api/v2/device/node-1/routes",
		func(w http.ResponseWriter, r *http.Request) {
			setRoutesCalled = true
			_ = json.NewDecoder(r.Body).Decode(&routesBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabledRoutes":    []string{"0.0.0.0/0", "::/0"},
				"advertisedRoutes": []string{"0.0.0.0/0", "::/0"},
			})
		})

	_, c := newTestClient(t, mux)
	if err := c.AuthorizeExitNode(context.Background(), "node-1"); err != nil {
		t.Fatalf("AuthorizeExitNode: %v", err)
	}
	if !setAuthorizedCalled {
		t.Errorf("expected /authorized to be called")
	}
	if !setRoutesCalled {
		t.Errorf("expected /routes to be called")
	}
	routes, _ := routesBody["routes"].([]any)
	if len(routes) != 2 {
		t.Errorf("routes = %v", routes)
	}
}
