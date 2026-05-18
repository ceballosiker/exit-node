// Package gcp's concrete Provider impl using cloud.google.com/go/compute/apiv1.
// The interface is declared in provider.go (Plan 1); this file implements
// it against real Compute Engine.
package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
)

// errNotImplemented is returned by stub methods that will be filled in by
// later tasks in Plan 2 (Tasks 18-20).
var errNotImplemented = errors.New("gcp: not implemented")

// Options configures the GCP Provider.
type Options struct {
	// Project is the GCP project ID. Required.
	Project string
	// Region is the default region used when ProvisionOpts.Region is
	// empty. Optional but typically set from config.
	Region string
	// CredentialsJSON, if non-empty, is parsed as a service-account
	// JSON key. Otherwise the underlying clients fall back to ADC
	// (GOOGLE_APPLICATION_CREDENTIALS or the metadata server).
	CredentialsJSON []byte
	// Network is the VPC network name to attach instances to (default
	// "default" if empty).
	Network string
	// InstallScriptURL is the URL pushed to VM metadata as
	// startup-script-url.
	InstallScriptURL string
	// DiskSizeGB is the boot disk size in GB; default 10.
	DiskSizeGB int64
}

// gcpProvider implements Provider.
type gcpProvider struct {
	instances *compute.InstancesClient
	zones     *compute.ZonesClient
	project   string
	region    string
	network   string
	scriptURL string
	diskGB    int64
}

// Compile-time assertion.
var _ Provider = (*gcpProvider)(nil)

// New constructs a Provider. With CredentialsJSON empty, falls back to
// Application Default Credentials.
func New(ctx context.Context, opts Options) (Provider, error) {
	if strings.TrimSpace(opts.Project) == "" {
		return nil, errors.New("gcp: Project required")
	}
	if opts.DiskSizeGB == 0 {
		opts.DiskSizeGB = 10
	}
	if strings.TrimSpace(opts.Network) == "" {
		opts.Network = "default"
	}

	var clientOpts []option.ClientOption
	if len(opts.CredentialsJSON) > 0 {
		clientOpts = append(clientOpts, option.WithCredentialsJSON(opts.CredentialsJSON))
	}

	inst, err := compute.NewInstancesRESTClient(ctx, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("gcp: new instances client: %w", err)
	}
	zones, err := compute.NewZonesRESTClient(ctx, clientOpts...)
	if err != nil {
		_ = inst.Close()
		return nil, fmt.Errorf("gcp: new zones client: %w", err)
	}
	return &gcpProvider{
		instances: inst,
		zones:     zones,
		project:   opts.Project,
		region:    opts.Region,
		network:   opts.Network,
		scriptURL: opts.InstallScriptURL,
		diskGB:    opts.DiskSizeGB,
	}, nil
}

// Close releases the underlying gRPC connections. Exposed for tests +
// graceful shutdown.
func (p *gcpProvider) Close() error {
	var err error
	if e := p.instances.Close(); e != nil {
		err = e
	}
	if e := p.zones.Close(); e != nil && err == nil {
		err = e
	}
	return err
}

// Provision creates a new VM with the configured labels + metadata,
// waits for it to reach RUNNING, and returns the populated ExitNode
// record (including the assigned public IP).
//
// Zone selection: if opts.Zone is empty, the caller is expected to
// have resolved a zone first (e.g., via PickZoneInRegion). v0.1 does
// not implement auto-pick inside Provision because zone-picking
// touches a separate API surface (ZonesClient).
func (p *gcpProvider) Provision(ctx context.Context, opts ProvisionOpts) (*ExitNode, error) {
	if opts.Zone == "" {
		return nil, fmt.Errorf("gcp: Provision requires opts.Zone (auto-pick TODO)")
	}
	inst := p.buildInstanceResource(opts)

	op, err := p.instances.Insert(ctx, &computepb.InsertInstanceRequest{
		Project:          p.project,
		Zone:             opts.Zone,
		InstanceResource: inst,
	})
	if err != nil {
		return nil, fmt.Errorf("gcp: instances.Insert: %w", err)
	}
	if err := op.Wait(ctx); err != nil {
		return nil, fmt.Errorf("gcp: wait for insert: %w", err)
	}

	got, err := p.instances.Get(ctx, &computepb.GetInstanceRequest{
		Project: p.project, Zone: opts.Zone, Instance: opts.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("gcp: instances.Get after insert: %w", err)
	}
	return instanceToExitNode(got), nil
}

// instanceToExitNode converts a Compute API Instance to our ExitNode.
func instanceToExitNode(inst *computepb.Instance) *ExitNode {
	out := &ExitNode{
		Name:        inst.GetName(),
		Zone:        lastPathSegment(inst.GetZone()),
		MachineType: lastPathSegment(inst.GetMachineType()),
		State:       parseInstanceStatus(inst.GetStatus()),
	}
	out.Region = inst.GetLabels()["region"]
	if t := inst.GetCreationTimestamp(); t != "" {
		// RFC3339 from compute API.
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			out.CreatedAt = parsed
		}
	}
outer:
	for _, nic := range inst.GetNetworkInterfaces() {
		for _, ac := range nic.GetAccessConfigs() {
			if ip := ac.GetNatIP(); ip != "" {
				out.PublicIP = ip
				break outer
			}
		}
	}
	return out
}

// parseInstanceStatus maps Compute's status strings to our State enum.
func parseInstanceStatus(s string) State {
	switch s {
	case "PROVISIONING", "STAGING":
		return StatePending
	case "RUNNING":
		return StateRunning
	case "STOPPING", "STOPPED", "SUSPENDED":
		return StateStopped
	case "TERMINATED":
		return StateTerminated
	default:
		return StateUnknown
	}
}

// lastPathSegment returns the substring after the final "/" — used to
// trim resource URLs like ".../zones/us-west1-a" down to "us-west1-a".
func lastPathSegment(s string) string {
	i := strings.LastIndex(s, "/")
	if i < 0 {
		return s
	}
	return s[i+1:]
}

// Start brings a stopped VM back online.
func (p *gcpProvider) Start(ctx context.Context, name string) error {
	zone, err := p.findZone(ctx, name)
	if err != nil {
		return err
	}
	op, err := p.instances.Start(ctx, &computepb.StartInstanceRequest{
		Project: p.project, Zone: zone, Instance: name,
	})
	if err != nil {
		return fmt.Errorf("gcp: instances.Start: %w", err)
	}
	return op.Wait(ctx)
}

// Stop shuts down a VM (preserves disk).
func (p *gcpProvider) Stop(ctx context.Context, name string) error {
	zone, err := p.findZone(ctx, name)
	if err != nil {
		return err
	}
	op, err := p.instances.Stop(ctx, &computepb.StopInstanceRequest{
		Project: p.project, Zone: zone, Instance: name,
	})
	if err != nil {
		return fmt.Errorf("gcp: instances.Stop: %w", err)
	}
	return op.Wait(ctx)
}

// Destroy deletes a VM permanently.
func (p *gcpProvider) Destroy(ctx context.Context, name string) error {
	zone, err := p.findZone(ctx, name)
	if err != nil {
		// If it's already gone, that's fine for rotate cleanup.
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return err
	}
	op, err := p.instances.Delete(ctx, &computepb.DeleteInstanceRequest{
		Project: p.project, Zone: zone, Instance: name,
	})
	if err != nil {
		return fmt.Errorf("gcp: instances.Delete: %w", err)
	}
	return op.Wait(ctx)
}

// findZone walks AggregatedList to locate the zone of a managed
// instance by name. Returns the zone (e.g., "us-west1-a") or an error
// if no matching instance exists.
func (p *gcpProvider) findZone(ctx context.Context, name string) (string, error) {
	it := p.instances.AggregatedList(ctx, &computepb.AggregatedListInstancesRequest{
		Project: p.project,
		Filter:  proto.String(`labels.managed-by=exitnode`),
	})
	for {
		pair, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("gcp: aggregated list: %w", err)
		}
		// Pair is (zone-key, *InstancesScopedList). Zone key looks like "zones/us-west1-a".
		for _, inst := range pair.Value.GetInstances() {
			if inst.GetName() == name {
				return lastPathSegment(pair.Key), nil
			}
		}
	}
	return "", fmt.Errorf("gcp: instance %q not found among managed nodes", name)
}

// List — Task 20 will implement.
func (p *gcpProvider) List(ctx context.Context) ([]*ExitNode, error) {
	return nil, errNotImplemented
}

// Get — Task 20 will implement.
func (p *gcpProvider) Get(ctx context.Context, name string) (*ExitNode, error) {
	return nil, errNotImplemented
}

// buildInstanceResource constructs the *computepb.Instance that
// Provision passes to Insert. Separated so we can shape-test it
// without hitting the API.
func (p *gcpProvider) buildInstanceResource(opts ProvisionOpts) *computepb.Instance {
	labels := map[string]string{
		"managed-by": "exitnode",
		"region":     opts.Region,
	}

	scriptURL := opts.InstallScriptURL
	if scriptURL == "" {
		scriptURL = p.scriptURL
	}
	diskGB := int64(opts.DiskSizeGB)
	if diskGB == 0 {
		diskGB = p.diskGB
	}
	network := opts.Network
	if network == "" {
		network = p.network
	}

	return &computepb.Instance{
		Name:        proto.String(opts.Name),
		MachineType: proto.String(fmt.Sprintf("zones/%s/machineTypes/%s", opts.Zone, opts.MachineType)),
		Labels:      labels,
		Disks: []*computepb.AttachedDisk{{
			Boot:       proto.Bool(true),
			AutoDelete: proto.Bool(true),
			Type:       proto.String("PERSISTENT"),
			InitializeParams: &computepb.AttachedDiskInitializeParams{
				DiskSizeGb:  proto.Int64(diskGB),
				SourceImage: proto.String("projects/debian-cloud/global/images/family/debian-12"),
			},
		}},
		NetworkInterfaces: []*computepb.NetworkInterface{{
			Network: proto.String("global/networks/" + network),
			AccessConfigs: []*computepb.AccessConfig{{
				Type: proto.String("ONE_TO_ONE_NAT"),
				Name: proto.String("External NAT"),
			}},
		}},
		Metadata: &computepb.Metadata{
			Items: []*computepb.Items{
				{Key: proto.String("startup-script-url"), Value: proto.String(scriptURL)},
				{Key: proto.String("tailscale-auth-key"), Value: proto.String(opts.TailscaleAuthKey)},
				{Key: proto.String("tailscale-hostname"), Value: proto.String(opts.Hostname)},
				{Key: proto.String("tailscale-tags"), Value: proto.String(strings.Join(opts.Tags, ","))},
			},
		},
	}
}
