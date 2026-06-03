package execsvc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/panda/pkg/config"
)

func TestSandboxAPIURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: "",
		},
		{
			name: "sandbox url wins and trailing slash trimmed",
			cfg: &config.Config{Server: config.ServerConfig{
				SandboxURL: "http://sandbox:1234/",
				BaseURL:    "http://base:5678",
			}},
			want: "http://sandbox:1234",
		},
		{
			name: "base url used when sandbox url empty",
			cfg:  &config.Config{Server: config.ServerConfig{BaseURL: "http://base:5678"}},
			want: "http://base:5678",
		},
		{
			name: "host.docker.internal default uses configured port",
			cfg:  &config.Config{Server: config.ServerConfig{Port: 9999}},
			want: "http://host.docker.internal:9999",
		},
		{
			name: "host.docker.internal default falls back to 2480",
			cfg:  &config.Config{Server: config.ServerConfig{}},
			want: "http://host.docker.internal:2480",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, sandboxAPIURL(tt.cfg))
		})
	}
}
