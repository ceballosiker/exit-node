package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const minimalToml = `
[gcp]
project              = "test-proj"
default_region       = "us-west1"
default_machine_type = "e2-micro"
network              = "default"
disk_size_gb         = 10

[tailscale]
tailnet                 = "example.com"
oauth_client_id_env     = "TS_ID"
oauth_client_secret_env = "TS_SECRET"
tags                    = ["tag:exit-node"]
ephemeral_key_ttl       = "5m"

[pfsense]
host         = "10.0.0.1"
api_key_env  = "PF_KEY"
gateway_name = "GW"
verify_tls   = true

[behavior]
auto_sync_pfsense    = true
verify_pre_cutover   = true
verify_post_cutover  = false
probe_url            = "https://api.ipify.org"
registration_timeout = "90s"
install_script_url   = "https://example.com/install.sh"
`

func writeToml(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestLoadMinimal(t *testing.T) {
	p := writeToml(t, minimalToml)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GCP.Project != "test-proj" {
		t.Errorf("GCP.Project = %q, want %q", cfg.GCP.Project, "test-proj")
	}
	if cfg.GCP.DefaultMachineType != "e2-micro" {
		t.Errorf("GCP.DefaultMachineType = %q", cfg.GCP.DefaultMachineType)
	}
	if cfg.Tailscale.Tailnet != "example.com" {
		t.Errorf("Tailscale.Tailnet = %q", cfg.Tailscale.Tailnet)
	}
	if cfg.Behavior.RegistrationTimeout != 90*time.Second {
		t.Errorf("RegistrationTimeout = %v, want 90s", cfg.Behavior.RegistrationTimeout)
	}
	if !cfg.Behavior.VerifyPreCutover {
		t.Errorf("VerifyPreCutover should be true")
	}
	if cfg.Behavior.VerifyPostCutover {
		t.Errorf("VerifyPostCutover should be false")
	}
}

func TestResolveTailscaleSecrets(t *testing.T) {
	p := writeToml(t, minimalToml)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Setenv("TS_ID", "client-id-1")
	t.Setenv("TS_SECRET", "client-secret-1")

	id, sec, err := cfg.ResolveTailscaleSecrets()
	if err != nil {
		t.Fatalf("ResolveTailscaleSecrets: %v", err)
	}
	if id != "client-id-1" || sec != "client-secret-1" {
		t.Errorf("got (%q, %q), want (client-id-1, client-secret-1)", id, sec)
	}
}

func TestResolveTailscaleSecretsMissing(t *testing.T) {
	p := writeToml(t, minimalToml)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Setenv("TS_ID", "")
	t.Setenv("TS_SECRET", "")

	if _, _, err := cfg.ResolveTailscaleSecrets(); err == nil {
		t.Errorf("expected error when env vars unset")
	}
}

func TestResolvePFSenseAPIKey(t *testing.T) {
	p := writeToml(t, minimalToml)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Setenv("PF_KEY", "pfkey-1")
	key, err := cfg.ResolvePFSenseAPIKey()
	if err != nil {
		t.Fatalf("ResolvePFSenseAPIKey: %v", err)
	}
	if key != "pfkey-1" {
		t.Errorf("key = %q, want pfkey-1", key)
	}
}

func TestResolveGCPCredentialsEnvJSONOverridesADC(t *testing.T) {
	// When GCP_CREDENTIALS_JSON is set, it wins.
	p := writeToml(t, minimalToml)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Setenv("GCP_CREDENTIALS_JSON", `{"type":"service_account","project_id":"x"}`)
	src, payload, err := cfg.ResolveGCPCredentials()
	if err != nil {
		t.Fatalf("ResolveGCPCredentials: %v", err)
	}
	if src != GCPSourceEnvJSON {
		t.Errorf("source = %v, want GCPSourceEnvJSON", src)
	}
	if string(payload) == "" {
		t.Errorf("payload should not be empty when env JSON is set")
	}
}

func TestResolveGCPCredentialsFallsBackToADC(t *testing.T) {
	p := writeToml(t, minimalToml)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Setenv("GCP_CREDENTIALS_JSON", "") // explicitly clear
	src, payload, err := cfg.ResolveGCPCredentials()
	if err != nil {
		t.Fatalf("ResolveGCPCredentials: %v", err)
	}
	if src != GCPSourceADC {
		t.Errorf("source = %v, want GCPSourceADC", src)
	}
	if len(payload) != 0 {
		t.Errorf("ADC payload should be empty (resolved later by google sdk)")
	}
}
