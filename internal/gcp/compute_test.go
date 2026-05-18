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

func TestBuildInstanceResource_ShapesAllFields(t *testing.T) {
	p := &gcpProvider{
		project:   "test-proj",
		region:    "us-west1",
		network:   "default",
		scriptURL: "https://example.com/install.sh",
		diskGB:    20,
	}
	inst := p.buildInstanceResource(ProvisionOpts{
		Name:             "vpn-test-1",
		Region:           "us-west1",
		Zone:             "us-west1-a",
		MachineType:      "e2-micro",
		Hostname:         "vpn-test-1",
		TailscaleAuthKey: "tskey-secret",
		Tags:             []string{"tag:exit-node", "tag:home"},
		InstallScriptURL: "https://example.com/install.sh",
		DiskSizeGB:       20,
		Network:          "default",
	})

	if got := inst.GetName(); got != "vpn-test-1" {
		t.Errorf("Name = %q", got)
	}
	if got := inst.GetMachineType(); got != "zones/us-west1-a/machineTypes/e2-micro" {
		t.Errorf("MachineType = %q", got)
	}
	labels := inst.GetLabels()
	if labels["managed-by"] != "exitnode" {
		t.Errorf("missing managed-by label: %v", labels)
	}
	if labels["region"] != "us-west1" {
		t.Errorf("missing region label: %v", labels)
	}

	// Boot disk: SourceImage points at Debian 12 family.
	if len(inst.Disks) != 1 {
		t.Fatalf("disks = %d, want 1", len(inst.Disks))
	}
	disk := inst.Disks[0]
	if !disk.GetBoot() {
		t.Errorf("boot disk not flagged Boot=true")
	}
	if got := disk.InitializeParams.GetSourceImage(); got != "projects/debian-cloud/global/images/family/debian-12" {
		t.Errorf("SourceImage = %q", got)
	}
	if got := disk.InitializeParams.GetDiskSizeGb(); got != 20 {
		t.Errorf("DiskSizeGb = %d", got)
	}

	// Public-IP NetworkInterface with one AccessConfig.
	if len(inst.NetworkInterfaces) != 1 {
		t.Fatalf("nics = %d, want 1", len(inst.NetworkInterfaces))
	}
	nic := inst.NetworkInterfaces[0]
	if nic.GetNetwork() != "global/networks/default" {
		t.Errorf("Network = %q", nic.GetNetwork())
	}
	if len(nic.AccessConfigs) != 1 || nic.AccessConfigs[0].GetType() != "ONE_TO_ONE_NAT" {
		t.Errorf("AccessConfigs = %v", nic.AccessConfigs)
	}

	// Metadata: startup-script-url + tailscale-auth-key + ...-hostname + ...-tags
	got := map[string]string{}
	for _, it := range inst.Metadata.Items {
		got[it.GetKey()] = it.GetValue()
	}
	if got["startup-script-url"] != "https://example.com/install.sh" {
		t.Errorf("startup-script-url = %q", got["startup-script-url"])
	}
	if got["tailscale-auth-key"] != "tskey-secret" {
		t.Errorf("tailscale-auth-key missing")
	}
	if got["tailscale-hostname"] != "vpn-test-1" {
		t.Errorf("tailscale-hostname missing")
	}
	if got["tailscale-tags"] != "tag:exit-node,tag:home" {
		t.Errorf("tailscale-tags = %q", got["tailscale-tags"])
	}
}
