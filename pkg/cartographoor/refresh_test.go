package cartographoor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ethpandaops/cartographoor/pkg/client"
	"github.com/ethpandaops/cartographoor/pkg/discovery"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// fakeProvider is a controllable client.Provider for testing the refresh path.
// Only GetNetworks and NotifyChannel carry behaviour; the rest are stubs.
type fakeProvider struct {
	mu       sync.RWMutex
	networks map[string]discovery.Network
	notifyCh chan struct{}
}

var _ client.Provider = (*fakeProvider)(nil)

func newFakeProvider() *fakeProvider {
	return &fakeProvider{
		networks: make(map[string]discovery.Network),
		notifyCh: make(chan struct{}, 1),
	}
}

// setNetworks replaces the data the provider will return.
func (f *fakeProvider) setNetworks(networks map[string]discovery.Network) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.networks = networks
}

// notify signals a refresh, mirroring the upstream provider's non-blocking send.
func (f *fakeProvider) notify() {
	select {
	case f.notifyCh <- struct{}{}:
	default:
	}
}

func (f *fakeProvider) GetNetworks(_ context.Context) (map[string]discovery.Network, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := make(map[string]discovery.Network, len(f.networks))
	for k, v := range f.networks {
		out[k] = v
	}

	return out, nil
}

func (f *fakeProvider) NotifyChannel() <-chan struct{} { return f.notifyCh }

func (f *fakeProvider) Start(_ context.Context) error { return nil }
func (f *fakeProvider) Stop() error                   { return nil }
func (f *fakeProvider) Ready() bool                   { return true }

func (f *fakeProvider) GetNetwork(_ context.Context, name string) (discovery.Network, bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	n, ok := f.networks[name]

	return n, ok, nil
}

func (f *fakeProvider) GetActiveNetworks(_ context.Context) (map[string]discovery.Network, error) {
	return nil, nil
}

func (f *fakeProvider) GetInactiveNetworks(_ context.Context) (map[string]discovery.Network, error) {
	return nil, nil
}

func (f *fakeProvider) GetNetworksByStatus(_ context.Context, _ string) (map[string]discovery.Network, error) {
	return nil, nil
}

func (f *fakeProvider) GetClients(_ context.Context) (map[string]discovery.ClientInfo, error) {
	return nil, nil
}

func (f *fakeProvider) GetClient(_ context.Context, _ string) (discovery.ClientInfo, bool, error) {
	return discovery.ClientInfo{}, false, nil
}

func (f *fakeProvider) GetClientsByType(_ context.Context, _ string) (map[string]discovery.ClientInfo, error) {
	return nil, nil
}

// TestClientRefresh verifies that when the underlying provider reports new data
// (via NotifyChannel), the client's watch loop rebuilds its network cache and
// devnet group index.
func TestClientRefresh(t *testing.T) {
	log := logrus.New()

	fake := newFakeProvider()
	fake.setNetworks(map[string]discovery.Network{
		"fusaka-devnet-1": {Name: "fusaka-devnet-1", Status: "active", Repository: "ethpandaops/fusaka-devnets"},
	})

	c := newClient(log, CartographoorConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, c.startWithProvider(ctx, fake))
	defer func() { require.NoError(t, c.Stop()) }()

	// Initial state reflects the first dataset.
	require.Len(t, c.GetAllNetworks(), 1)
	require.ElementsMatch(t, []string{"fusaka"}, c.GetGroups())

	// Swap the provider's data and signal a refresh.
	fake.setNetworks(map[string]discovery.Network{
		"fusaka-devnet-1": {Name: "fusaka-devnet-1", Status: "active", Repository: "ethpandaops/fusaka-devnets"},
		"pectra-devnet-2": {Name: "pectra-devnet-2", Status: "active", Repository: "ethpandaops/pectra-devnets"},
		"mainnet":         {Name: "mainnet", Status: "active"},
	})
	fake.notify()

	// The watch goroutine should pick up the new data and rebuild.
	require.Eventually(t, func() bool {
		return len(c.GetAllNetworks()) == 3
	}, 2*time.Second, 5*time.Millisecond, "client did not rebuild after refresh notification")

	require.ElementsMatch(t, []string{"fusaka", "pectra"}, c.GetGroups())

	_, ok := c.GetNetwork("pectra-devnet-2")
	require.True(t, ok, "newly added network should be queryable after refresh")
}
