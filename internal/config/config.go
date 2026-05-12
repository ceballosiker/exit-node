// Package config loads exitnode's TOML config and resolves secrets from
// the environment.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the root config struct, populated from TOML.
type Config struct {
	GCP       GCPConfig       `toml:"gcp"`
	Tailscale TailscaleConfig `toml:"tailscale"`
	PFSense   PFSenseConfig   `toml:"pfsense"`
	Behavior  BehaviorConfig  `toml:"behavior"`
}

type GCPConfig struct {
	Project            string `toml:"project"`
	DefaultRegion      string `toml:"default_region"`
	DefaultMachineType string `toml:"default_machine_type"`
	DefaultZone        string `toml:"default_zone"`
	Network            string `toml:"network"`
	DiskSizeGB         int    `toml:"disk_size_gb"`
}

type TailscaleConfig struct {
	Tailnet              string        `toml:"tailnet"`
	OAuthClientIDEnv     string        `toml:"oauth_client_id_env"`
	OAuthClientSecretEnv string        `toml:"oauth_client_secret_env"`
	Tags                 []string      `toml:"tags"`
	EphemeralKeyTTL      time.Duration `toml:"ephemeral_key_ttl"`
}

type PFSenseConfig struct {
	Host        string `toml:"host"`
	APIKeyEnv   string `toml:"api_key_env"`
	GatewayName string `toml:"gateway_name"`
	VerifyTLS   bool   `toml:"verify_tls"`
}

type BehaviorConfig struct {
	AutoSyncPFSense     bool          `toml:"auto_sync_pfsense"`
	VerifyPreCutover    bool          `toml:"verify_pre_cutover"`
	VerifyPostCutover   bool          `toml:"verify_post_cutover"`
	ProbeURL            string        `toml:"probe_url"`
	RegistrationTimeout time.Duration `toml:"registration_timeout"`
	InstallScriptURL    string        `toml:"install_script_url"`
}

// Load reads and parses the TOML config at path.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}

// ResolveTailscaleSecrets reads the OAuth client id and secret from the env
// var names declared in the TOML config. Both must be set.
func (c *Config) ResolveTailscaleSecrets() (clientID, clientSecret string, err error) {
	if c.Tailscale.OAuthClientIDEnv == "" || c.Tailscale.OAuthClientSecretEnv == "" {
		return "", "", fmt.Errorf("config tailscale.oauth_client_id_env / oauth_client_secret_env must be set")
	}
	clientID = os.Getenv(c.Tailscale.OAuthClientIDEnv)
	clientSecret = os.Getenv(c.Tailscale.OAuthClientSecretEnv)
	if clientID == "" || clientSecret == "" {
		return "", "", fmt.Errorf("tailscale OAuth env vars %s / %s are unset",
			c.Tailscale.OAuthClientIDEnv, c.Tailscale.OAuthClientSecretEnv)
	}
	return clientID, clientSecret, nil
}

// ResolvePFSenseAPIKey reads the pfSense API key from the configured env var.
func (c *Config) ResolvePFSenseAPIKey() (string, error) {
	if c.PFSense.APIKeyEnv == "" {
		return "", fmt.Errorf("config pfsense.api_key_env must be set")
	}
	key := os.Getenv(c.PFSense.APIKeyEnv)
	if key == "" {
		return "", fmt.Errorf("pfsense API key env var %s is unset", c.PFSense.APIKeyEnv)
	}
	return key, nil
}

// GCPCredSource identifies where the GCP credentials came from.
type GCPCredSource int

const (
	GCPSourceADC GCPCredSource = iota
	GCPSourceEnvJSON
)

// ResolveGCPCredentials returns the GCP credentials source and, when the
// source is GCPSourceEnvJSON, the JSON payload bytes. For GCPSourceADC the
// caller is expected to use google.FindDefaultCredentials and the payload
// will be empty.
func (c *Config) ResolveGCPCredentials() (GCPCredSource, []byte, error) {
	if envJSON := os.Getenv("GCP_CREDENTIALS_JSON"); envJSON != "" {
		return GCPSourceEnvJSON, []byte(envJSON), nil
	}
	return GCPSourceADC, nil, nil
}
