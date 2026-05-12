package pfsense

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestServer + newTestClient give every test a fresh httptest
// server and a pf REST client wired to it. The handler is what the
// test customizes.
func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *pfClient) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)

	// httptest.NewTLSServer uses a self-signed cert; the production
	// client supports InsecureSkipVerify via Options.VerifyTLS=false.
	c, err := New(Options{
		BaseURL:   srv.URL,
		APIKey:    "test-key",
		VerifyTLS: false,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv, c.(*pfClient)
}

func TestDecodeEnvelope_Success(t *testing.T) {
	body := `{"code":200,"status":"ok","response_id":"SUCCESS","message":"","data":{"name":"GW","gateway":"100.64.0.1"}}`
	var into struct {
		Name    string `json:"name"`
		Gateway string `json:"gateway"`
	}
	if err := decodeEnvelope([]byte(body), &into); err != nil {
		t.Fatalf("decodeEnvelope: %v", err)
	}
	if into.Name != "GW" || into.Gateway != "100.64.0.1" {
		t.Errorf("got %+v", into)
	}
}

func TestDecodeEnvelope_ErrorCode(t *testing.T) {
	body := `{"code":404,"status":"error","response_id":"NOT_FOUND","message":"gateway 'X' not found","data":null}`
	err := decodeEnvelope([]byte(body), new(any))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != 404 || apiErr.ResponseID != "NOT_FOUND" {
		t.Errorf("got %+v", apiErr)
	}
}

func TestNewRequiresAPIKey(t *testing.T) {
	_, err := New(Options{BaseURL: "https://example.com", APIKey: ""})
	if err == nil {
		t.Errorf("expected error for empty APIKey")
	}
}

// Silence unused-import warnings in early tasks; they're used by later tests.
var (
	_ = json.Marshal
	_ = tls.Config{}
	_ = strings.NewReader
	_ = context.Background
)
