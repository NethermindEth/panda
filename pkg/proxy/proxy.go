// Package proxy provides the credential proxy for server-side upstream access.
// The proxy holds datasource credentials and serves raw credentialed routes.
package proxy

import (
	"context"

	"github.com/ethpandaops/panda/pkg/types"
)

// NoAuthToken is the sentinel RegisterToken returns when the proxy needs no
// bearer token (e.g. auth.mode=none). Callers must not send it as a credential.
const NoAuthToken = "none"

// Service is the credential proxy service interface.
// This is implemented by both Client (for connecting to a proxy)
// and directly by the proxy Server.
type Service interface {
	// Start starts the service.
	Start(ctx context.Context) error

	// Stop stops the service.
	Stop(ctx context.Context) error

	// URL returns the proxy URL.
	URL() string

	// RegisterToken returns the current access token for server-to-proxy
	// requests, or NoAuthToken when no bearer token is required.
	RegisterToken() string

	// RevokeToken is a no-op for client-managed bearer tokens.
	RevokeToken()

	// ClickHouseDatasources returns the list of ClickHouse datasource names.
	ClickHouseDatasources() []string
	// ClickHouseDatasourceInfo returns detailed ClickHouse datasource info.
	ClickHouseDatasourceInfo() []types.DatasourceInfo

	// PrometheusDatasourceInfo returns detailed Prometheus datasource info.
	PrometheusDatasourceInfo() []types.DatasourceInfo

	// LokiDatasourceInfo returns detailed Loki datasource info.
	LokiDatasourceInfo() []types.DatasourceInfo

	// EthNodeAvailable returns true if ethnode proxy access is configured.
	EthNodeAvailable() bool
	// EthNodeDatasourceInfo returns the ethnode datasource info when ethnode
	// access is configured, or nil otherwise. Ethnode is exposed as a single
	// type-level datasource rather than a discoverable list.
	EthNodeDatasourceInfo() []types.DatasourceInfo

	// EmbeddingAvailable returns true if the proxy has embedding configured.
	EmbeddingAvailable() bool
	// EmbeddingModel returns the configured embedding model name.
	EmbeddingModel() string
}

// ethNodeDatasourceInfo returns the ethnode datasource identity when available,
// or nil. Ethnode is a single type-level datasource, not a discoverable list.
func ethNodeDatasourceInfo(available bool) []types.DatasourceInfo {
	if !available {
		return nil
	}

	return []types.DatasourceInfo{{Type: "ethnode", Name: "ethnode"}}
}
