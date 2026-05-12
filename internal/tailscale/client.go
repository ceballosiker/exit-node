package tailscale

import (
	"context"
	"time"
)

// TailscaleClient abstracts the Tailscale management API.
type TailscaleClient interface {
	// MintEphemeralAuthKey returns a single-use, pre-authorized auth key
	// tagged for the exit-node role. The key has a short TTL (~5 minutes).
	MintEphemeralAuthKey(ctx context.Context, tags []string) (key string, err error)

	// WaitForDevice polls until a device with the given hostname registers.
	// Returns the registered device or an error on timeout.
	WaitForDevice(ctx context.Context, hostname string, timeout time.Duration) (*Device, error)

	// AuthorizeExitNode approves the device's exit-node advertisement.
	AuthorizeExitNode(ctx context.Context, deviceID string) error

	// SetTags applies the tag set to the device.
	SetTags(ctx context.Context, deviceID string, tags []string) error

	// DeleteDevice removes the device from the tailnet.
	DeleteDevice(ctx context.Context, deviceID string) error
}
