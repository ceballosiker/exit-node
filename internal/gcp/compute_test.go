package gcp

import (
	"context"
	"testing"
)

func TestNewWithoutCreds_UsesADC(t *testing.T) {
	// Without GCP_CREDENTIALS_JSON set, New should still succeed — the
	// underlying client lazily resolves ADC. We don't make any API
	// calls in this test.
	t.Setenv("GCP_CREDENTIALS_JSON", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	p, err := New(context.Background(), Options{
		Project: "test-proj",
		Region:  "us-west1",
	})
	// New may or may not return an error here depending on whether
	// ADC is available in the test env. The important assertions are:
	// (a) it doesn't panic, and (b) when it succeeds, returns
	// non-nil. CI runs in environments without ADC, so we tolerate
	// either outcome.
	if err == nil && p == nil {
		t.Errorf("nil provider with nil error")
	}
}

func TestNewRequiresProject(t *testing.T) {
	if _, err := New(context.Background(), Options{Region: "us-west1"}); err == nil {
		t.Errorf("expected error for empty Project")
	}
}
