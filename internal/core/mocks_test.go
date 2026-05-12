package core

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/iker/exit-node/internal/gcp"
	"github.com/iker/exit-node/internal/pfsense"
	"github.com/iker/exit-node/internal/tailscale"
)

// clock issues monotonically-increasing sequence numbers shared across all
// mocks in a fixture, so the test can reconstruct the chronological order
// of cross-mock calls (e.g. ts.Mint → prov.Provision → ts.WaitForDevice).
type clock struct {
	mu  sync.Mutex
	seq int
}

func (c *clock) tick() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	return c.seq
}

// callRecord captures the name, args, and chronological sequence of a mock
// invocation.
type callRecord struct {
	Seq  int
	Name string
	Args []any
}

type recorder struct {
	mu    sync.Mutex
	clock *clock // shared across mocks in the same fixture
	calls []callRecord
}

func (r *recorder) record(name string, args ...any) {
	seq := 0
	if r.clock != nil {
		seq = r.clock.tick()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, callRecord{Seq: seq, Name: name, Args: args})
}

func (r *recorder) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	for i, c := range r.calls {
		out[i] = c.Name
	}
	return out
}

// --- Provider mock ----------------------------------------------------------

type mockProvider struct {
	recorder
	ProvisionResult *gcp.ExitNode
	ProvisionErr    error
	StartErr        error
	StopErr         error
	DestroyErr      map[string]error // keyed by name
	ListResult      []*gcp.ExitNode
	ListErr         error
	GetResult       map[string]*gcp.ExitNode
	GetErr          error
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		DestroyErr: map[string]error{},
		GetResult:  map[string]*gcp.ExitNode{},
	}
}

// attachClock wires a shared sequence clock into the mock's recorder so its
// calls can be merged chronologically with other mocks in the fixture.
func (m *mockProvider) attachClock(c *clock) { m.recorder.clock = c }

func (m *mockProvider) Provision(ctx context.Context, opts gcp.ProvisionOpts) (*gcp.ExitNode, error) {
	m.record("Provision", opts.Name, opts.Region, opts.MachineType)
	if m.ProvisionErr != nil {
		return nil, m.ProvisionErr
	}
	return m.ProvisionResult, nil
}
func (m *mockProvider) Start(ctx context.Context, name string) error {
	m.record("Start", name)
	return m.StartErr
}
func (m *mockProvider) Stop(ctx context.Context, name string) error {
	m.record("Stop", name)
	return m.StopErr
}
func (m *mockProvider) Destroy(ctx context.Context, name string) error {
	m.record("Destroy", name)
	return m.DestroyErr[name]
}
func (m *mockProvider) List(ctx context.Context) ([]*gcp.ExitNode, error) {
	m.record("List")
	return m.ListResult, m.ListErr
}
func (m *mockProvider) Get(ctx context.Context, name string) (*gcp.ExitNode, error) {
	m.record("Get", name)
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	return m.GetResult[name], nil
}

// --- TailscaleClient mock ---------------------------------------------------

type mockTS struct {
	recorder
	MintKey            string
	MintErr            error
	WaitDevice         *tailscale.Device
	WaitErr            error
	AuthorizeErr       error
	SetTagsErr         error
	DeleteDeviceErr    map[string]error // keyed by device id
	BlockMintUntilSeen bool             // if true, returns context.Canceled if ctx cancels
}

func newMockTS() *mockTS {
	return &mockTS{
		DeleteDeviceErr: map[string]error{},
		MintKey:         "tskey-ephemeral",
	}
}

func (m *mockTS) attachClock(c *clock) { m.recorder.clock = c }

func (m *mockTS) MintEphemeralAuthKey(ctx context.Context, tags []string) (string, error) {
	m.record("MintEphemeralAuthKey", tags)
	if m.MintErr != nil {
		return "", m.MintErr
	}
	return m.MintKey, nil
}
func (m *mockTS) WaitForDevice(ctx context.Context, hostname string, timeout time.Duration) (*tailscale.Device, error) {
	m.record("WaitForDevice", hostname, timeout)
	if m.WaitErr != nil {
		return nil, m.WaitErr
	}
	return m.WaitDevice, nil
}
func (m *mockTS) AuthorizeExitNode(ctx context.Context, deviceID string) error {
	m.record("AuthorizeExitNode", deviceID)
	return m.AuthorizeErr
}
func (m *mockTS) SetTags(ctx context.Context, deviceID string, tags []string) error {
	m.record("SetTags", deviceID, tags)
	return m.SetTagsErr
}
func (m *mockTS) DeleteDevice(ctx context.Context, deviceID string) error {
	m.record("DeleteDevice", deviceID)
	return m.DeleteDeviceErr[deviceID]
}

// --- PFSenseClient mock -----------------------------------------------------

type mockPF struct {
	recorder
	Gateways           map[string]string // name → IP
	GetErr             error
	UpdateErr          map[string]error // keyed by IP being set; matched on the new IP arg
	UpdateErrFirstCall error            // if non-nil, fails on first call only (for revert tests)
	ApplyErr           []error          // pop per call (1st call uses [0], 2nd [1])
	applyIdx           int
}

func newMockPF() *mockPF {
	return &mockPF{
		Gateways:  map[string]string{},
		UpdateErr: map[string]error{},
	}
}

func (m *mockPF) attachClock(c *clock) { m.recorder.clock = c }

func (m *mockPF) GetGateway(ctx context.Context, name string) (*pfsense.Gateway, error) {
	m.record("GetGateway", name)
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	ip, ok := m.Gateways[name]
	if !ok {
		return nil, errors.New("gateway not found")
	}
	return &pfsense.Gateway{Name: name, IP: ip}, nil
}
func (m *mockPF) UpdateGatewayIP(ctx context.Context, name, ip string) error {
	m.record("UpdateGatewayIP", name, ip)
	if err, ok := m.UpdateErr[ip]; ok {
		return err
	}
	m.Gateways[name] = ip
	return nil
}
func (m *mockPF) Apply(ctx context.Context) error {
	m.record("Apply")
	i := m.applyIdx
	m.applyIdx++
	if i < len(m.ApplyErr) {
		return m.ApplyErr[i]
	}
	return nil
}

// --- Probe mock -------------------------------------------------------------

type mockProbe struct {
	recorder
	EgressViaResult    string
	EgressViaErr       error
	EgressDirectResult string
	EgressDirectErr    error
}

func (m *mockProbe) EgressVia(ctx context.Context, tailscaleIP string) (string, error) {
	m.record("EgressVia", tailscaleIP)
	return m.EgressViaResult, m.EgressViaErr
}
func (m *mockProbe) EgressDirect(ctx context.Context) (string, error) {
	m.record("EgressDirect")
	return m.EgressDirectResult, m.EgressDirectErr
}

func (m *mockProbe) attachClock(c *clock) { m.recorder.clock = c }
