package gcp

import (
	"context"
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/protobuf/proto"
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

func TestParseInstanceStatus(t *testing.T) {
	cases := []struct {
		in   string
		want State
	}{
		{"PROVISIONING", StatePending},
		{"STAGING", StatePending},
		{"RUNNING", StateRunning},
		{"STOPPING", StateStopped},
		{"STOPPED", StateStopped},
		{"SUSPENDED", StateStopped},
		{"TERMINATED", StateTerminated},
		{"", StateUnknown},
		{"weird-new-state", StateUnknown},
	}
	for _, c := range cases {
		if got := parseInstanceStatus(c.in); got != c.want {
			t.Errorf("parseInstanceStatus(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestLastPathSegment(t *testing.T) {
	cases := []struct{ in, want string }{
		{"projects/p/zones/us-west1-a", "us-west1-a"},
		{"plain", "plain"},
		{"trailing/", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := lastPathSegment(c.in); got != c.want {
			t.Errorf("lastPathSegment(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInstanceToExitNode_PublicIPFromFirstAccessConfig(t *testing.T) {
	inst := &computepb.Instance{
		Name:        proto.String("vpn-1"),
		Zone:        proto.String("https://www.googleapis.com/.../zones/us-west1-a"),
		MachineType: proto.String("https://www.googleapis.com/.../zones/us-west1-a/machineTypes/e2-micro"),
		Status:      proto.String("RUNNING"),
		Labels:      map[string]string{"region": "us-west1"},
		NetworkInterfaces: []*computepb.NetworkInterface{
			{AccessConfigs: []*computepb.AccessConfig{{NatIP: proto.String("1.2.3.4")}}},
			{AccessConfigs: []*computepb.AccessConfig{{NatIP: proto.String("5.6.7.8")}}},
		},
	}
	got := instanceToExitNode(inst)
	if got.PublicIP != "1.2.3.4" {
		t.Errorf("PublicIP = %q, want 1.2.3.4 (first NIC's first AccessConfig)", got.PublicIP)
	}
	if got.Zone != "us-west1-a" {
		t.Errorf("Zone = %q", got.Zone)
	}
	if got.MachineType != "e2-micro" {
		t.Errorf("MachineType = %q", got.MachineType)
	}
	if got.State != StateRunning {
		t.Errorf("State = %v, want StateRunning", got.State)
	}
	if got.Region != "us-west1" {
		t.Errorf("Region = %q", got.Region)
	}
}
