// Package pfsense holds the pfSense REST client. The interface is
// declared in client.go (Plan 1); this file is the concrete impl that
// talks to the community pfsense-api plugin (v2 endpoints).
package pfsense

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Options configures the pfSense client.
type Options struct {
	// BaseURL is the pfSense webConfigurator origin (e.g.,
	// "https://pfsense.lan"). No trailing slash required.
	BaseURL string
	// APIKey is the raw API key minted by the pfsense-api plugin.
	// Sent as the value of the configured AuthHeader (default
	// "Authorization", no "Bearer " prefix — that's what the plugin
	// expects on v2).
	APIKey string
	// AuthHeader is the header name carrying the API key. Defaults to
	// "Authorization" if empty. Exposed because some plugin builds
	// accept "X-API-Key" instead.
	AuthHeader string
	// VerifyTLS controls server cert verification. Set false for
	// pfSense's default self-signed cert; provide a real cert and set
	// true for production.
	VerifyTLS bool
	// CACertPEM optionally pins a CA used to verify the server cert.
	// Ignored if VerifyTLS is false.
	CACertPEM []byte
	// HTTPTimeout caps each request. Defaults to 30s if zero.
	HTTPTimeout time.Duration
}

// APIError is returned when the pfSense API responds with a non-2xx
// envelope code.
type APIError struct {
	Code       int    // envelope.code (e.g., 404)
	Status     string // envelope.status (e.g., "error")
	ResponseID string // envelope.response_id (e.g., "NOT_FOUND")
	Message    string // envelope.message
}

func (e *APIError) Error() string {
	return fmt.Sprintf("pfsense api: code=%d status=%s response_id=%s: %s",
		e.Code, e.Status, e.ResponseID, e.Message)
}

// pfClient is the concrete PFSenseClient. Unexported; New returns the
// interface.
type pfClient struct {
	baseURL    string
	apiKey     string
	authHeader string
	http       *http.Client
}

// New constructs a PFSenseClient. Returns the interface so the caller
// can't reach into the struct.
func New(opts Options) (PFSenseClient, error) {
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("pfsense: APIKey required")
	}
	if strings.TrimSpace(opts.BaseURL) == "" {
		return nil, errors.New("pfsense: BaseURL required")
	}
	if opts.HTTPTimeout == 0 {
		opts.HTTPTimeout = 30 * time.Second
	}
	if opts.AuthHeader == "" {
		opts.AuthHeader = "Authorization"
	}

	tlsCfg := &tls.Config{InsecureSkipVerify: !opts.VerifyTLS} //nolint:gosec
	// CA-cert pinning is a future enhancement; opts.CACertPEM ignored
	// for v0.1.

	return &pfClient{
		baseURL:    strings.TrimRight(opts.BaseURL, "/"),
		apiKey:     opts.APIKey,
		authHeader: opts.AuthHeader,
		http: &http.Client{
			Timeout:   opts.HTTPTimeout,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

// envelope is the wire shape of every pfsense-api v2 response.
type envelope struct {
	Code       int             `json:"code"`
	Status     string          `json:"status"`
	ResponseID string          `json:"response_id"`
	Message    string          `json:"message"`
	Data       json.RawMessage `json:"data"`
}

// decodeEnvelope reads a pfsense-api response body, returns *APIError
// for non-2xx envelope codes, and otherwise unmarshals envelope.data
// into `into`. Pass `new(any)` (or any other discardable) when there's
// no useful payload (e.g., POST /apply).
func decodeEnvelope(body []byte, into any) error {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("pfsense: parse envelope: %w (body=%q)", err, string(body))
	}
	if env.Code < 200 || env.Code >= 300 {
		return &APIError{
			Code: env.Code, Status: env.Status,
			ResponseID: env.ResponseID, Message: env.Message,
		}
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, into); err != nil {
		return fmt.Errorf("pfsense: parse envelope.data: %w", err)
	}
	return nil
}

// errNotImplemented is returned by the interface stubs below until
// Tasks 8/9/10 replace them with real implementations. Stubs exist so
// *pfClient satisfies PFSenseClient and New() can return the
// interface as specified.
var errNotImplemented = errors.New("pfsense: not implemented")

// GetGateway returns the named gateway. Translates 404 envelopes to
// *APIError so callers can distinguish "not found" from "transport
// error".
func (c *pfClient) GetGateway(ctx context.Context, name string) (*Gateway, error) {
	body, _, err := c.do(ctx, http.MethodGet,
		"/api/v2/routing/gateway?id="+url.QueryEscape(name), nil)
	if err != nil {
		return nil, fmt.Errorf("pfsense GET gateway: %w", err)
	}
	var raw struct {
		Name    string `json:"name"`
		Gateway string `json:"gateway"`
	}
	if err := decodeEnvelope(body, &raw); err != nil {
		return nil, err
	}
	return &Gateway{Name: raw.Name, IP: raw.Gateway}, nil
}

// UpdateGatewayIP is a stub; Task 9 lands the real impl.
func (c *pfClient) UpdateGatewayIP(ctx context.Context, name, ip string) error {
	return errNotImplemented
}

// Apply is a stub; Task 10 lands the real impl.
func (c *pfClient) Apply(ctx context.Context) error {
	return errNotImplemented
}

// do issues an HTTP request, applies auth + JSON headers, returns the
// raw body and HTTP status.
func (c *pfClient) do(ctx context.Context, method, path string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set(c.authHeader, c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return b, resp.StatusCode, nil
}
