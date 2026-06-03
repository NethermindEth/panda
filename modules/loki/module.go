package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sync"

	"github.com/ethpandaops/panda/pkg/module"
	"github.com/ethpandaops/panda/pkg/types"
)

// Compile-time interface checks.
var (
	_ module.Module                 = (*Module)(nil)
	_ module.ProxyDiscoverable      = (*Module)(nil)
	_ module.SandboxEnvProvider     = (*Module)(nil)
	_ module.DatasourceInfoProvider = (*Module)(nil)
	_ module.ExamplesProvider       = (*Module)(nil)
	_ module.PythonAPIDocsProvider  = (*Module)(nil)
)

// Module implements the module.Module interface for Loki.
type Module struct {
	dsMu        sync.RWMutex
	datasources []types.DatasourceInfo
}

// New creates a new Loki module.
func New() *Module { return &Module{} }

func (m *Module) Name() string { return "loki" }

// InitFromDiscovery initializes the module from discovered datasources.
// Safe to call repeatedly: subsequent calls replace the datasource list in
// place so the proxy client's periodic refresh propagates without a restart.
//
// Always writes the filtered list, including when empty — see the comment on
// the clickhouse module's InitFromDiscovery for the rationale.
func (m *Module) InitFromDiscovery(datasources []types.DatasourceInfo) error {
	filtered := make([]types.DatasourceInfo, 0, len(datasources))

	for _, ds := range datasources {
		if ds.Type != "loki" {
			continue
		}

		filtered = append(filtered, ds)
	}

	m.dsMu.Lock()
	m.datasources = filtered
	m.dsMu.Unlock()

	if len(filtered) == 0 {
		return module.ErrNoValidConfig
	}

	return nil
}

// Init is a no-op: the module's datasources come from proxy discovery via
// InitFromDiscovery.
func (m *Module) Init(_ []byte) error { return nil }

// ApplyDefaults sets default values before validation.
func (m *Module) ApplyDefaults() {}

// Validate checks that the parsed config is valid.
func (m *Module) Validate() error {
	m.dsMu.RLock()
	defer m.dsMu.RUnlock()

	names := make(map[string]struct{}, len(m.datasources))
	for i, ds := range m.datasources {
		if ds.Name == "" {
			return fmt.Errorf("datasource[%d].name is required", i)
		}

		if _, exists := names[ds.Name]; exists {
			return fmt.Errorf("datasource[%d].name %q is duplicated", i, ds.Name)
		}

		names[ds.Name] = struct{}{}
	}

	return nil
}

// SandboxEnv returns environment variables for the sandbox.
func (m *Module) SandboxEnv() (map[string]string, error) {
	m.dsMu.RLock()
	defer m.dsMu.RUnlock()

	if len(m.datasources) == 0 {
		return nil, nil
	}

	type datasourceInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	infos := make([]datasourceInfo, 0, len(m.datasources))
	for _, ds := range m.datasources {
		infos = append(infos, datasourceInfo{
			Name:        ds.Name,
			Description: ds.Description,
		})
	}

	infosJSON, err := json.Marshal(infos)
	if err != nil {
		return nil, fmt.Errorf("marshaling Loki datasource info: %w", err)
	}

	return map[string]string{
		"ETHPANDAOPS_LOKI_DATASOURCES": string(infosJSON),
	}, nil
}

// DatasourceInfo returns datasource metadata for datasources:// resources.
func (m *Module) DatasourceInfo() []types.DatasourceInfo {
	m.dsMu.RLock()
	defer m.dsMu.RUnlock()

	result := make([]types.DatasourceInfo, len(m.datasources))
	copy(result, m.datasources)

	return result
}

// Examples returns query examples for the Loki module.
func (m *Module) Examples() map[string]types.ExampleCategory {
	result := make(map[string]types.ExampleCategory, len(queryExamples))
	maps.Copy(result, queryExamples)

	return result
}

// PythonAPIDocs returns the Loki module documentation.
func (m *Module) PythonAPIDocs() map[string]types.ModuleDoc {
	return map[string]types.ModuleDoc{
		"loki": {
			Description: "Query Loki for log data",
			Functions: map[string]types.FunctionDoc{
				"list_datasources": {
					Signature:   "loki.list_datasources() -> list[dict]",
					Description: "List available Loki datasources. Prefer datasources://loki resource.",
					Returns:     "List of dicts with 'name', 'description', 'url' keys",
				},
				"query": {
					Signature:   "loki.query(instance_name: str, logql: str, limit: int = 100, start: str = None, end: str = None, direction: str = 'backward') -> dict",
					Description: "Execute LogQL range query and return the raw Loki data payload",
					Parameters: map[string]string{
						"instance_name": "Datasource name from datasources://loki",
						"logql":         "LogQL query string",
						"limit":         "Max entries to return (default: 100)",
						"start":         "Start time (default: now-1h)",
						"end":           "End time (default: now)",
						"direction":     "'forward' or 'backward' (default)",
					},
					Returns: "Dict with Loki stream/vector data under 'resultType' and 'result'",
				},
				"query_instant": {
					Signature:   "loki.query_instant(instance_name: str, logql: str, time: str = None, limit: int = 100, direction: str = 'backward') -> dict",
					Description: "Execute instant LogQL query and return the raw Loki data payload",
					Parameters: map[string]string{
						"instance_name": "Datasource name",
						"logql":         "LogQL query string",
						"time":          "Evaluation timestamp (default: now)",
						"limit":         "Max entries (default: 100)",
						"direction":     "'forward' or 'backward'",
					},
					Returns: "Dict with Loki stream/vector data under 'resultType' and 'result'",
				},
				"get_labels": {
					Signature:   "loki.get_labels(instance_name: str, start: str = None, end: str = None) -> list[str]",
					Description: "Get all label names",
					Parameters: map[string]string{
						"instance_name": "Datasource name",
						"start":         "Optional start time",
						"end":           "Optional end time",
					},
					Returns: "List of label names",
				},
				"get_label_values": {
					Signature:   "loki.get_label_values(instance_name: str, label: str, start: str = None, end: str = None) -> list[str]",
					Description: "Get all values for a label",
					Parameters: map[string]string{
						"instance_name": "Datasource name",
						"label":         "Label name",
						"start":         "Optional start time",
						"end":           "Optional end time",
					},
					Returns: "List of label values",
				},
			},
		},
	}
}

// Start performs async initialization.
func (m *Module) Start(_ context.Context) error { return nil }

// Stop cleans up resources.
func (m *Module) Stop(_ context.Context) error { return nil }
