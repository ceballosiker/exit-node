// Package gcp's concrete Provider impl using cloud.google.com/go/compute/apiv1.
// The interface is declared in provider.go (Plan 1); this file implements
// it against real Compute Engine.
package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
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

// Provision — Task 18 will implement.
func (p *gcpProvider) Provision(ctx context.Context, opts ProvisionOpts) (*ExitNode, error) {
	return nil, errNotImplemented
}

// Start — Task 19 will implement.
func (p *gcpProvider) Start(ctx context.Context, name string) error {
	return errNotImplemented
}

// Stop — Task 19 will implement.
func (p *gcpProvider) Stop(ctx context.Context, name string) error {
	return errNotImplemented
}

// Destroy — Task 19 will implement.
func (p *gcpProvider) Destroy(ctx context.Context, name string) error {
	return errNotImplemented
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
