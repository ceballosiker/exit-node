// Package tailscale's concrete TailscaleClient impl wraps
// tailscale.com/client/tailscale/v2. The wrapper is thin: it owns
// option validation, the AuthorizeExitNode composite call (which
// requires two underlying v2 calls), and WaitForDevice's polling loop.
package tailscale

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	tsv2 "tailscale.com/client/tailscale/v2"
)

// errNotImplemented is returned by stub methods on tsClient until the
// corresponding task (12-15) supplies the real implementation.
var errNotImplemented = errors.New("tailscale: not implemented")

// Options configures the Tailscale client.
type Options struct {
	// Tailnet is the organization name (the part before .ts.net or
	// the configured custom domain). Required.
	Tailnet string
	// ClientID + ClientSecret are the OAuth client-credentials. The
	// recommended scopes are "auth_keys" and "devices:core" (write).
	// Fall back to "all:write" if scope naming has shifted on the
	// admin console.
	ClientID     string
	ClientSecret string
	// BaseURL overrides the Tailscale API origin. Empty → use the
	// library default (https://api.tailscale.com). Tests set this to
	// an httptest server URL.
	BaseURL string
	// PollInterval is the WaitForDevice poll cadence. Defaults to 2s.
	PollInterval time.Duration
}

// tsClient implements TailscaleClient.
type tsClient struct {
	inner        *tsv2.Client
	tailnet      string
	pollInterval time.Duration
}

// New constructs a TailscaleClient.
func New(opts Options) (TailscaleClient, error) {
	if strings.TrimSpace(opts.Tailnet) == "" {
		return nil, errors.New("tailscale: Tailnet required")
	}
	if strings.TrimSpace(opts.ClientID) == "" {
		return nil, errors.New("tailscale: ClientID required")
	}
	if strings.TrimSpace(opts.ClientSecret) == "" {
		return nil, errors.New("tailscale: ClientSecret required")
	}
	c := &tsv2.Client{
		Tailnet: opts.Tailnet,
		Auth: &tsv2.OAuth{
			ClientID:     opts.ClientID,
			ClientSecret: opts.ClientSecret,
			Scopes:       []string{"auth_keys", "devices:core"},
		},
	}
	if opts.BaseURL != "" {
		u, err := url.Parse(opts.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("tailscale: parse BaseURL: %w", err)
		}
		c.BaseURL = u
	}
	poll := opts.PollInterval
	if poll == 0 {
		poll = 2 * time.Second
	}
	return &tsClient{inner: c, tailnet: opts.Tailnet, pollInterval: poll}, nil
}

// MintEphemeralAuthKey mints a single-use, ephemeral, preauthorized
// auth key tagged for the given tags. The key has a 5-minute TTL.
func (c *tsClient) MintEphemeralAuthKey(ctx context.Context, tags []string) (string, error) {
	req := tsv2.CreateKeyRequest{
		Description:   "exit-node bootstrap",
		ExpirySeconds: 300,
	}
	// The Capabilities shape uses anonymous nested structs in the v2
	// library. The construction below mirrors the library's struct
	// literal pattern; field names match those in CreateKeyRequest.
	req.Capabilities.Devices.Create.Reusable = false
	req.Capabilities.Devices.Create.Ephemeral = true
	req.Capabilities.Devices.Create.Preauthorized = true
	req.Capabilities.Devices.Create.Tags = tags

	key, err := c.inner.Keys().CreateAuthKey(ctx, req)
	if err != nil {
		return "", fmt.Errorf("tailscale create auth key: %w", err)
	}
	return key.Key, nil
}

// WaitForDevice — Task 13 will implement.
func (c *tsClient) WaitForDevice(ctx context.Context, hostname string, timeout time.Duration) (*Device, error) {
	return nil, errNotImplemented
}

// AuthorizeExitNode — Task 14 will implement.
func (c *tsClient) AuthorizeExitNode(ctx context.Context, deviceID string) error {
	return errNotImplemented
}

// SetTags — Task 15 will implement.
func (c *tsClient) SetTags(ctx context.Context, deviceID string, tags []string) error {
	return errNotImplemented
}

// DeleteDevice — Task 15 will implement.
func (c *tsClient) DeleteDevice(ctx context.Context, deviceID string) error {
	return errNotImplemented
}

// Compile-time assertion.
var _ TailscaleClient = (*tsClient)(nil)
