package config

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ethpandaops/panda/pkg/configpath"
)

// ClientConfig is the subset of configuration needed by the CLI when operating as a server client.
type ClientConfig struct {
	Server  ServerConfig  `yaml:"server"`
	Proxy   ProxyConfig   `yaml:"proxy"`
	Proxies []ProxyConfig `yaml:"proxies,omitempty"`

	path string `yaml:"-"`
}

// LoadClient loads client configuration from the standard config locations.
func LoadClient(path string) (*ClientConfig, error) {
	resolvedPath, err := configpath.ResolveAppConfigPath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", resolvedPath, err)
	}

	substituted, err := substituteEnvVars(string(data))
	if err != nil {
		return nil, fmt.Errorf("substituting env vars: %w", err)
	}

	// Decode strictly into the full Config so typos in any section are caught,
	// then project the Server and Proxy subsets the client needs.
	var full Config
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(substituted)))
	decoder.KnownFields(true)

	if err := decoder.Decode(&full); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := rejectSingularAndPluralProxies(full.Proxy, full.Proxies); err != nil {
		return nil, err
	}

	cfg := ClientConfig{
		Server:  full.Server,
		Proxy:   full.Proxy,
		Proxies: full.Proxies,
	}

	cfg.applyDefaults()
	if cfg.ServerURL() == "" {
		return nil, fmt.Errorf(
			"no server URL configured. set server.url or server.base_url in %s",
			resolvedPath,
		)
	}

	cfg.path = resolvedPath

	return &cfg, nil
}

// ServerURL returns the resolved server base URL for client use.
func (c *ClientConfig) ServerURL() string {
	if c == nil {
		return ""
	}

	if c.Server.URL != "" {
		return strings.TrimRight(c.Server.URL, "/")
	}

	if c.Server.BaseURL != "" {
		return strings.TrimRight(c.Server.BaseURL, "/")
	}

	host := strings.TrimSpace(c.Server.Host)
	if host == "" || host == "0.0.0.0" || host == "::" || host == "::0" {
		host = "localhost"
	}

	port := c.Server.Port
	if port == 0 {
		port = defaultServerPort
	}

	return fmt.Sprintf("http://%s", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
}

// Path returns the resolved path this client config was loaded from.
func (c *ClientConfig) Path() string {
	return c.path
}

func (c *ClientConfig) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = defaultServerPort
	}

	normalizeProxyConfigs(&c.Proxy, &c.Proxies, "")
}
