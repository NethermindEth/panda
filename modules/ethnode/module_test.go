package ethnode

import (
	"errors"
	"testing"

	"github.com/ethpandaops/panda/pkg/module"
	"github.com/ethpandaops/panda/pkg/types"
)

// TestModule_InitFromDiscovery_PopulatesDatasourceInfo verifies that a
// discovered ethnode datasource is surfaced through DatasourceInfo and that
// non-ethnode entries are filtered out.
func TestModule_InitFromDiscovery_PopulatesDatasourceInfo(t *testing.T) {
	t.Parallel()

	mod := New()

	if err := mod.InitFromDiscovery([]types.DatasourceInfo{
		{Type: "clickhouse", Name: "xatu"},
		{Type: "ethnode", Name: "ethnode"},
	}); err != nil {
		t.Fatalf("InitFromDiscovery error = %v", err)
	}

	infos := mod.DatasourceInfo()
	if len(infos) != 1 {
		t.Fatalf("DatasourceInfo() = %#v, want a single ethnode entry", infos)
	}

	if infos[0].Type != "ethnode" || infos[0].Name != "ethnode" {
		t.Fatalf("DatasourceInfo()[0] = %#v, want {Type: ethnode, Name: ethnode}", infos[0])
	}
}

// TestModule_InitFromDiscovery_AbsentReturnsNoValidConfig verifies the module
// stays disabled and reports no datasources when the proxy exposes no ethnode
// access, and that a later disappearance clears a previously discovered entry.
func TestModule_InitFromDiscovery_AbsentReturnsNoValidConfig(t *testing.T) {
	t.Parallel()

	mod := New()

	if err := mod.InitFromDiscovery([]types.DatasourceInfo{
		{Type: "ethnode", Name: "ethnode"},
	}); err != nil {
		t.Fatalf("initial InitFromDiscovery error = %v", err)
	}

	if got := len(mod.DatasourceInfo()); got != 1 {
		t.Fatalf("after initial init: DatasourceInfo() has %d entries, want 1", got)
	}

	err := mod.InitFromDiscovery([]types.DatasourceInfo{
		{Type: "clickhouse", Name: "xatu"},
	})
	if !errors.Is(err, module.ErrNoValidConfig) {
		t.Fatalf("after ethnode disappears: err = %v, want ErrNoValidConfig", err)
	}

	if got := len(mod.DatasourceInfo()); got != 0 {
		t.Fatalf("after ethnode disappears: DatasourceInfo() has %d entries, want 0", got)
	}
}
