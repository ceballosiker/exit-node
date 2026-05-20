//go:build integration

package gcp

import (
	"context"
	"os"
	"testing"
	"time"
)

func gateIntegration(t *testing.T) Options {
	t.Helper()
	if os.Getenv("EXITNODE_INTEGRATION") != "1" {
		t.Skip("EXITNODE_INTEGRATION!=1; skipping real-GCP test")
	}
	proj := os.Getenv("EXITNODE_TEST_PROJECT")
	if proj == "" {
		t.Skip("EXITNODE_TEST_PROJECT unset; skipping")
	}
	opts := Options{
		Project:          proj,
		Region:           envOr("EXITNODE_TEST_REGION", "us-central1"),
		InstallScriptURL: "https://example.com/install.sh", // not actually run
	}
	if jsonKey := os.Getenv("GCP_CREDENTIALS_JSON"); jsonKey != "" {
		opts.CredentialsJSON = []byte(jsonKey)
	}
	return opts
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestIntegration_ListIsCallable(t *testing.T) {
	opts := gateIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, err := New(ctx, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		if err := p.(*gcpProvider).Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	// We don't assert content (the test project may have no managed
	// VMs); just that the call returns without error.
	if _, err := p.List(ctx); err != nil {
		t.Fatalf("List: %v", err)
	}
}
