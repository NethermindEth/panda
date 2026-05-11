package proxy

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	simpleauth "github.com/ethpandaops/panda/pkg/auth"
	"github.com/ethpandaops/panda/pkg/configpath"
	"github.com/ethpandaops/panda/pkg/proxy/handlers"
)

// customEthNodeSegmentPattern matches the same lowercase alphanumeric + hyphen
// segments that the ethnode handler accepts for network/instance identifiers.
var customEthNodeSegmentPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// ServerConfig is the configuration for the proxy server.
// This is the single configuration schema used for both local and K8s deployments.
type ServerConfig struct {
	// Server holds HTTP server configuration.
	Server HTTPServerConfig `yaml:"server"`

	// Auth holds authentication configuration.
	Auth AuthConfig `yaml:"auth"`

	// ClickHouse holds ClickHouse cluster configurations.
	ClickHouse []ClickHouseClusterConfig `yaml:"clickhouse,omitempty"`

	// Prometheus holds Prometheus instance configurations.
	Prometheus []PrometheusInstanceConfig `yaml:"prometheus,omitempty"`

	// Loki holds Loki instance configurations.
	Loki []LokiInstanceConfig `yaml:"loki,omitempty"`

	// EthNode holds Ethereum node API access configuration.
	EthNode *EthNodeInstanceConfig `yaml:"ethnode,omitempty"`

	// CustomEthNode holds user-defined Ethereum node endpoints, addressed at
	// /custom/beacon|execution/{network}/{instance}/{path} and bypassing the
	// ethpandaops.io DNS naming convention used by EthNode.
	CustomEthNode *CustomEthNodeConfig `yaml:"custom_ethnode,omitempty"`

	// RateLimiting holds rate limiting configuration.
	RateLimiting RateLimitConfig `yaml:"rate_limiting"`

	// Audit holds audit logging configuration.
	Audit AuditConfig `yaml:"audit"`

	// Metrics holds Prometheus metrics configuration.
	Metrics MetricsConfig `yaml:"metrics"`

	// Embedding holds optional embedding API configuration.
	Embedding *EmbeddingConfig `yaml:"embedding,omitempty"`

	// GitHub holds optional GitHub API configuration for triggering workflows.
	GitHub *GitHubAPIConfig `yaml:"github,omitempty"`
}

// GitHubAPIConfig holds GitHub API configuration for the proxy.
type GitHubAPIConfig struct {
	// Token is a GitHub personal access token or app token with actions:write permission.
	Token string `yaml:"token"`
}

// HTTPServerConfig holds HTTP server configuration.
type HTTPServerConfig struct {
	// ListenAddr is the address to listen on (default: ":18081").
	ListenAddr string `yaml:"listen_addr,omitempty"`

	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration `yaml:"read_timeout,omitempty"`

	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration `yaml:"write_timeout,omitempty"`

	// IdleTimeout is the maximum amount of time to wait for the next request.
	IdleTimeout time.Duration `yaml:"idle_timeout,omitempty"`
}

// AuthConfig holds authentication configuration for the proxy.
type AuthConfig struct {
	// Mode is the authentication mode.
	Mode AuthMode `yaml:"mode"`

	// IssuerURL is the external URL clients should use for auth and token validation.
	IssuerURL string `yaml:"issuer_url,omitempty"`

	// ClientID is the OIDC client identifier expected in bearer token audiences.
	ClientID string `yaml:"client_id,omitempty"`

	// GitHub configures the GitHub OAuth app used for user authentication.
	GitHub *simpleauth.GitHubConfig `yaml:"github,omitempty"`

	// AllowedOrgs restricts access to members of these GitHub orgs.
	AllowedOrgs []string `yaml:"allowed_orgs,omitempty"`

	// Tokens configures proxy-issued bearer tokens.
	Tokens simpleauth.TokensConfig `yaml:"tokens"`

	// AccessTokenTTL is the lifetime of proxy-issued access tokens.
	AccessTokenTTL time.Duration `yaml:"access_token_ttl,omitempty"`

	// RefreshTokenTTL is the lifetime of proxy-issued refresh tokens.
	RefreshTokenTTL time.Duration `yaml:"refresh_token_ttl,omitempty"`

	// SuccessPage customizes the OAuth callback success page shown in the browser.
	SuccessPage *simpleauth.SuccessPageConfig `yaml:"success_page,omitempty"`
}

// DatasourceConfig is the interface every datasource config must satisfy.
// The Authorizer uses this to build access rules generically, ensuring that
// any new datasource type added to the proxy must include authorization support.
type DatasourceConfig interface {
	DatasourceName() string
	DatasourceDescription() string
	DatasourceAllowedOrgs() []string
}

// BaseDatasourceConfig holds fields common to all datasource configurations.
// Embed this in every datasource config struct to get compile-time enforcement
// of authorization support via the DatasourceConfig interface.
type BaseDatasourceConfig struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	AllowedOrgs []string `yaml:"allowed_orgs,omitempty"`
}

// DatasourceName returns the datasource name.
func (b BaseDatasourceConfig) DatasourceName() string { return b.Name }

// DatasourceDescription returns the datasource description.
func (b BaseDatasourceConfig) DatasourceDescription() string { return b.Description }

// DatasourceAllowedOrgs returns the list of GitHub orgs allowed to access this datasource.
func (b BaseDatasourceConfig) DatasourceAllowedOrgs() []string { return b.AllowedOrgs }

// Compile-time interface checks for datasource configs.
var (
	_ DatasourceConfig = ClickHouseClusterConfig{}
	_ DatasourceConfig = PrometheusInstanceConfig{}
	_ DatasourceConfig = LokiInstanceConfig{}
	_ DatasourceConfig = EthNodeInstanceConfig{}
)

// ClickHouseClusterConfig holds ClickHouse cluster configuration.
type ClickHouseClusterConfig struct {
	BaseDatasourceConfig `yaml:",inline"`
	Host                 string `yaml:"host"`
	Port                 int    `yaml:"port"`
	Database             string `yaml:"database,omitempty"`
	Username             string `yaml:"username"`
	Password             string `yaml:"password"`
	Secure               bool   `yaml:"secure"`
	SkipVerify           bool   `yaml:"skip_verify,omitempty"`
	Timeout              int    `yaml:"timeout,omitempty"`
}

// PrometheusInstanceConfig holds Prometheus instance configuration.
type PrometheusInstanceConfig struct {
	BaseDatasourceConfig `yaml:",inline"`
	URL                  string `yaml:"url"`
	Username             string `yaml:"username,omitempty"`
	Password             string `yaml:"password,omitempty"`
}

// LokiInstanceConfig holds Loki instance configuration.
type LokiInstanceConfig struct {
	BaseDatasourceConfig `yaml:",inline"`
	URL                  string `yaml:"url"`
	Username             string `yaml:"username,omitempty"`
	Password             string `yaml:"password,omitempty"`
}

// EthNodeInstanceConfig holds Ethereum node API access configuration.
// A single credential pair is used for all beacon and execution node endpoints.
type EthNodeInstanceConfig struct {
	BaseDatasourceConfig `yaml:",inline"`
	Username             string `yaml:"username"`
	Password             string `yaml:"password"`
}

// CustomEthNodeConfig configures user-defined Ethereum node endpoints, keyed by
// network. Each network entry lists nodes with explicit beacon/execution URLs
// and optional per-node basic-auth credentials.
type CustomEthNodeConfig struct {
	// AllowedOrgs restricts access to members of these GitHub orgs (type-level,
	// same scoping as EthNode).
	AllowedOrgs []string `yaml:"allowed_orgs,omitempty"`

	// Networks maps a network name to a list of nodes belonging to that network.
	Networks map[string][]CustomEthNodeNodeConfig `yaml:"networks"`
}

// CustomEthNodeNodeConfig holds a single user-defined node's endpoints and
// shared basic-auth credentials.
type CustomEthNodeNodeConfig struct {
	// Instance is the node identifier within its network.
	Instance string `yaml:"instance"`

	// BeaconURL is the upstream consensus-layer (beacon) HTTP(S) endpoint.
	// Optional: if empty, /custom/beacon/... requests for this node return 404.
	BeaconURL string `yaml:"beacon_url,omitempty"`

	// ExecutionURL is the upstream execution-layer HTTP(S) endpoint.
	// Optional: if empty, /custom/execution/... requests for this node return 404.
	ExecutionURL string `yaml:"execution_url,omitempty"`

	// Username and Password are basic-auth credentials applied to both EL and CL.
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// RateLimitConfig holds rate limiting configuration.
type RateLimitConfig struct {
	// Enabled controls whether rate limiting is active.
	Enabled bool `yaml:"enabled"`

	// RequestsPerMinute is the maximum requests per minute per user.
	RequestsPerMinute int `yaml:"requests_per_minute,omitempty"`

	// BurstSize is the maximum burst size.
	BurstSize int `yaml:"burst_size,omitempty"`
}

// AuditConfig holds audit logging configuration.
type AuditConfig struct {
	// Enabled controls whether audit logging is active.
	Enabled bool `yaml:"enabled"`
}

// EmbeddingConfig holds configuration for the remote embedding API.
type EmbeddingConfig struct {
	// APIKey is the API key for the embedding provider (e.g., OpenRouter).
	APIKey string `yaml:"api_key"`

	// Model is the embedding model name (default: "openai/text-embedding-3-small").
	Model string `yaml:"model,omitempty"`

	// APIURL is the base URL of the embedding API (default: "https://openrouter.ai/api/v1").
	APIURL string `yaml:"api_url,omitempty"`

	// Cache holds embedding cache configuration.
	Cache EmbeddingCacheConfig `yaml:"cache"`
}

// EmbeddingCacheConfig holds cache configuration for embeddings.
type EmbeddingCacheConfig struct {
	// Backend is the cache backend: "memory" (default) or "redis".
	Backend string `yaml:"backend,omitempty"`

	// RedisURL is the Redis connection URL (required when backend is "redis").
	RedisURL string `yaml:"redis_url,omitempty"`
}

// MetricsConfig holds Prometheus metrics configuration for the proxy.
type MetricsConfig struct {
	// Enabled controls whether the Prometheus metrics server is active.
	Enabled bool `yaml:"enabled"`

	// ListenAddr is the address to serve the /metrics endpoint on.
	ListenAddr string `yaml:"listen_addr,omitempty"`

	// Port is the port to serve the /metrics endpoint on (default: 9090).
	Port int `yaml:"port,omitempty"`
}

// ApplyDefaults sets default values for the server config.
func (c *ServerConfig) ApplyDefaults() {
	// Server defaults.
	if c.Server.ListenAddr == "" {
		c.Server.ListenAddr = ":18081"
	}

	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 30 * time.Second
	}

	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 5 * time.Minute
	}

	if c.Server.IdleTimeout == 0 {
		c.Server.IdleTimeout = 60 * time.Second
	}

	// Auth defaults.
	// Default to no auth for local development. Hosted deployments should explicitly set oauth mode.
	if c.Auth.Mode == "" {
		c.Auth.Mode = AuthModeNone
	}

	if c.Auth.AccessTokenTTL == 0 {
		c.Auth.AccessTokenTTL = 1 * time.Hour
	}

	if c.Auth.RefreshTokenTTL == 0 {
		c.Auth.RefreshTokenTTL = 30 * 24 * time.Hour
	}

	// Rate limiting defaults.
	if c.RateLimiting.RequestsPerMinute == 0 {
		c.RateLimiting.RequestsPerMinute = 60
	}

	if c.RateLimiting.BurstSize == 0 {
		c.RateLimiting.BurstSize = 10
	}

	// Metrics defaults.
	if c.Metrics.Port == 0 {
		c.Metrics.Port = 9090
	}

	if c.Metrics.ListenAddr == "" {
		c.Metrics.ListenAddr = fmt.Sprintf("127.0.0.1:%d", c.Metrics.Port)
	}

	// Embedding defaults.
	if c.Embedding != nil {
		if c.Embedding.Model == "" {
			c.Embedding.Model = "openai/text-embedding-3-small"
		}

		if c.Embedding.APIURL == "" {
			c.Embedding.APIURL = "https://openrouter.ai/api/v1"
		}

		if c.Embedding.Cache.Backend == "" {
			c.Embedding.Cache.Backend = "memory"
		}
	}

	// ClickHouse defaults.
	for i := range c.ClickHouse {
		if c.ClickHouse[i].Port == 0 {
			if c.ClickHouse[i].Secure {
				c.ClickHouse[i].Port = 8443
			} else {
				c.ClickHouse[i].Port = 8123
			}
		}
	}
}

// Validate validates the server config.
func (c *ServerConfig) Validate() error {
	if c.Auth.Mode == AuthModeOAuth {
		if c.Auth.GitHub == nil {
			return fmt.Errorf("auth.github is required when auth.mode is 'oauth'")
		}

		if c.Auth.GitHub.ClientID == "" {
			return fmt.Errorf("auth.github.client_id is required")
		}

		if c.Auth.GitHub.ClientSecret == "" {
			return fmt.Errorf("auth.github.client_secret is required")
		}

		if c.Auth.Tokens.SecretKey == "" {
			return fmt.Errorf("auth.tokens.secret_key is required")
		}

		if strings.TrimSpace(c.Auth.IssuerURL) == "" {
			return fmt.Errorf("auth.issuer_url is required")
		}
	}

	if c.Auth.Mode == AuthModeOIDC {
		if strings.TrimSpace(c.Auth.IssuerURL) == "" {
			return fmt.Errorf("auth.issuer_url is required when auth.mode is 'oidc'")
		}

		if strings.TrimSpace(c.Auth.ClientID) == "" {
			return fmt.Errorf("auth.client_id is required when auth.mode is 'oidc'")
		}
	}

	// Validate embedding config.
	if c.Embedding != nil {
		if c.Embedding.APIKey == "" {
			return fmt.Errorf("embedding.api_key is required when embedding is configured")
		}

		if c.Embedding.Cache.Backend == "redis" && c.Embedding.Cache.RedisURL == "" {
			return fmt.Errorf("embedding.cache.redis_url is required when cache backend is 'redis'")
		}
	}

	// Validate at least one datasource is configured.
	if len(c.ClickHouse) == 0 && len(c.Prometheus) == 0 && len(c.Loki) == 0 && c.EthNode == nil && c.CustomEthNode == nil {
		return fmt.Errorf("at least one datasource (clickhouse, prometheus, loki, ethnode, or custom_ethnode) must be configured")
	}

	// Validate ClickHouse configs.
	for i, ch := range c.ClickHouse {
		if ch.Name == "" {
			return fmt.Errorf("clickhouse[%d].name is required", i)
		}

		if ch.Host == "" {
			return fmt.Errorf("clickhouse[%d].host is required", i)
		}
	}

	// Validate Prometheus configs.
	for i, prom := range c.Prometheus {
		if prom.Name == "" {
			return fmt.Errorf("prometheus[%d].name is required", i)
		}

		if prom.URL == "" {
			return fmt.Errorf("prometheus[%d].url is required", i)
		}
	}

	// Validate Loki configs.
	for i, loki := range c.Loki {
		if loki.Name == "" {
			return fmt.Errorf("loki[%d].name is required", i)
		}

		if loki.URL == "" {
			return fmt.Errorf("loki[%d].url is required", i)
		}
	}

	// Validate CustomEthNode config.
	if c.CustomEthNode != nil {
		if err := validateCustomEthNode(c.CustomEthNode); err != nil {
			return err
		}
	}

	return nil
}

func validateCustomEthNode(cfg *CustomEthNodeConfig) error {
	if len(cfg.Networks) == 0 {
		return fmt.Errorf("custom_ethnode.networks must contain at least one network")
	}

	for network, nodes := range cfg.Networks {
		if !customEthNodeSegmentPattern.MatchString(network) {
			return fmt.Errorf("custom_ethnode.networks: invalid network name %q (must match [a-z0-9-])", network)
		}

		seen := make(map[string]struct{}, len(nodes))
		for i, node := range nodes {
			if node.Instance == "" {
				return fmt.Errorf("custom_ethnode.networks[%s][%d].instance is required", network, i)
			}

			if !customEthNodeSegmentPattern.MatchString(node.Instance) {
				return fmt.Errorf("custom_ethnode.networks[%s][%d].instance %q is invalid (must match [a-z0-9-])", network, i, node.Instance)
			}

			if _, dup := seen[node.Instance]; dup {
				return fmt.Errorf("custom_ethnode.networks[%s]: duplicate instance %q", network, node.Instance)
			}
			seen[node.Instance] = struct{}{}

			if node.BeaconURL == "" && node.ExecutionURL == "" {
				return fmt.Errorf("custom_ethnode.networks[%s][%s]: at least one of beacon_url or execution_url is required", network, node.Instance)
			}

			if err := validateCustomEthNodeURL("beacon_url", network, node.Instance, node.BeaconURL); err != nil {
				return err
			}

			if err := validateCustomEthNodeURL("execution_url", network, node.Instance, node.ExecutionURL); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateCustomEthNodeURL(field, network, instance, raw string) error {
	if raw == "" {
		return nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("custom_ethnode.networks[%s][%s].%s is invalid: %w", network, instance, field, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("custom_ethnode.networks[%s][%s].%s must use http or https scheme (got %q)", network, instance, field, parsed.Scheme)
	}

	if parsed.Host == "" {
		return fmt.Errorf("custom_ethnode.networks[%s][%s].%s must include a host", network, instance, field)
	}

	return nil
}

// ToHandlerConfigs converts the server config to handler configs.
func (c *ServerConfig) ToHandlerConfigs() ([]handlers.ClickHouseConfig, []handlers.PrometheusConfig, []handlers.LokiConfig, *handlers.EthNodeConfig, *handlers.CustomEthNodeConfig) {
	// Convert ClickHouse configs.
	chConfigs := make([]handlers.ClickHouseConfig, len(c.ClickHouse))
	for i, ch := range c.ClickHouse {
		chConfigs[i] = handlers.ClickHouseConfig{
			Name:        ch.Name,
			Description: ch.Description,
			Host:        ch.Host,
			Port:        ch.Port,
			Database:    ch.Database,
			Username:    ch.Username,
			Password:    ch.Password,
			Secure:      ch.Secure,
			SkipVerify:  ch.SkipVerify,
			Timeout:     ch.Timeout,
		}
	}

	// Convert Prometheus configs.
	promConfigs := make([]handlers.PrometheusConfig, len(c.Prometheus))
	for i, prom := range c.Prometheus {
		promConfigs[i] = handlers.PrometheusConfig{
			Name:        prom.Name,
			Description: prom.Description,
			URL:         prom.URL,
			Username:    prom.Username,
			Password:    prom.Password,
		}
	}

	// Convert Loki configs.
	lokiConfigs := make([]handlers.LokiConfig, len(c.Loki))
	for i, loki := range c.Loki {
		lokiConfigs[i] = handlers.LokiConfig{
			Name:        loki.Name,
			Description: loki.Description,
			URL:         loki.URL,
			Username:    loki.Username,
			Password:    loki.Password,
		}
	}

	// Convert EthNode config.
	var ethNodeConfig *handlers.EthNodeConfig
	if c.EthNode != nil && c.EthNode.Username != "" {
		ethNodeConfig = &handlers.EthNodeConfig{
			Username: c.EthNode.Username,
			Password: c.EthNode.Password,
		}
	}

	// Convert CustomEthNode config.
	var customEthNodeConfig *handlers.CustomEthNodeConfig
	if c.CustomEthNode != nil && len(c.CustomEthNode.Networks) > 0 {
		nodes := make(map[handlers.CustomEthNodeKey]handlers.CustomEthNodeNode)
		for network, networkNodes := range c.CustomEthNode.Networks {
			for _, node := range networkNodes {
				nodes[handlers.CustomEthNodeKey{Network: network, Instance: node.Instance}] = handlers.CustomEthNodeNode{
					BeaconURL:    node.BeaconURL,
					ExecutionURL: node.ExecutionURL,
					Username:     node.Username,
					Password:     node.Password,
				}
			}
		}

		if len(nodes) > 0 {
			customEthNodeConfig = &handlers.CustomEthNodeConfig{Nodes: nodes}
		}
	}

	return chConfigs, promConfigs, lokiConfigs, ethNodeConfig, customEthNodeConfig
}

// envVarWithDefaultPattern matches ${VAR_NAME:-default} patterns.
var envVarWithDefaultPattern = regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

// LoadServerConfig loads a proxy server config from a YAML file.
func LoadServerConfig(path string) (*ServerConfig, error) {
	resolvedPath, err := configpath.ResolveProxyConfigPath(path, "")
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", resolvedPath, err)
	}

	// Substitute environment variables.
	substituted, err := substituteEnvVars(string(data))
	if err != nil {
		return nil, fmt.Errorf("substituting env vars: %w", err)
	}

	var cfg ServerConfig
	if err := yaml.Unmarshal([]byte(substituted), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// substituteEnvVars replaces ${VAR_NAME} and ${VAR_NAME:-default} patterns with environment variable values.
// Lines that are comments (starting with #) are skipped.
// Missing environment variables without defaults are replaced with empty strings (lenient mode).
func substituteEnvVars(content string) (string, error) {
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		// Skip lines that are YAML comments.
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		lines[i] = envVarWithDefaultPattern.ReplaceAllStringFunc(line, func(match string) string {
			parts := envVarWithDefaultPattern.FindStringSubmatch(match)
			varName := parts[1]
			defaultVal := ""
			if len(parts) > 2 {
				defaultVal = parts[2]
			}

			value := os.Getenv(varName)
			if value == "" {
				return defaultVal // Use default or empty string
			}

			return value
		})
	}

	return strings.Join(lines, "\n"), nil
}
