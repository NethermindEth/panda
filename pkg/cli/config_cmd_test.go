package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/panda/pkg/config"
)

func TestConfigTUIProxyURLOverrideWithBaseProxiesListReloads(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	basePath := filepath.Join(dir, "config.yaml")
	userPath := config.UserConfigPath(basePath)

	baseConfig := `
server:
  base_url: "http://localhost:2480"
sandbox:
  image: "test-image:latest"
proxies:
  - name: hosted
    url: "https://proxy.example/"
    auth:
      mode: "oidc"
      issuer_url: "https://issuer.example"
      client_id: "panda"
  - name: lab
    url: "http://lab.example:18081"
`
	require.NoError(t, os.WriteFile(basePath, []byte(baseConfig), 0o644))

	cfg, err := config.LoadWithUserOverrides(basePath)
	require.NoError(t, err)

	baseUsesProxyList, err := config.BaseUsesProxyList(basePath)
	require.NoError(t, err)
	require.True(t, baseUsesProxyList)

	proxyURLPath := "proxy.url"
	if baseUsesProxyList {
		proxyURLPath = "proxies.0.url"
	}

	categories := buildCategories(cfg, proxyURLPath)
	snapshotOriginals(categories)

	proxyParam := findConfigParam(t, categories, "Proxy URL")
	require.Equal(t, "proxies.0.url", proxyParam.Path)
	proxyParam.Value = "https://new-proxy.example"

	overrides, err := buildOverrideMap(categories, map[string]any{})
	require.NoError(t, err)
	require.NoError(t, config.ValidateMergedConfig(basePath, overrides))
	require.NoError(t, config.SaveUserConfig(userPath, overrides))

	reloaded, err := config.LoadWithUserOverrides(basePath)
	require.NoError(t, err)

	require.Len(t, reloaded.Proxies, 2)
	assert.Equal(t, "hosted", reloaded.Proxies[0].Name)
	assert.Equal(t, "https://new-proxy.example", reloaded.Proxies[0].URL)
	require.NotNil(t, reloaded.Proxies[0].Auth)
	assert.Equal(t, "https://issuer.example", reloaded.Proxies[0].Auth.IssuerURL)
	assert.Equal(t, "lab", reloaded.Proxies[1].Name)
	assert.Equal(t, "http://lab.example:18081", reloaded.Proxies[1].URL)

	data, err := os.ReadFile(userPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "proxies:")
	assert.NotContains(t, string(data), "\nproxy:")
}

func findConfigParam(t *testing.T, categories []configCategory, name string) *configParam {
	t.Helper()

	for i := range categories {
		for _, param := range categories[i].Params {
			if param.Name == name {
				return param
			}
		}
	}

	require.FailNowf(t, "config param not found", "name=%s", name)

	return nil
}
