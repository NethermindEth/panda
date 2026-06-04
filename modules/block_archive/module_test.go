package blockarchive

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "default url passes",
			cfg:  Config{},
		},
		{
			name: "explicit https url passes",
			cfg:  Config{URL: "https://archive.example.com"},
		},
		{
			name: "explicit http url passes",
			cfg:  Config{URL: "http://archive.example.com"},
		},
		{
			name:    "non-http scheme rejected",
			cfg:     Config{URL: "ftp://archive.example.com"},
			wantErr: true,
		},
		{
			name:    "missing host rejected",
			cfg:     Config{URL: "https://"},
			wantErr: true,
		},
		{
			name:    "userinfo rejected",
			cfg:     Config{URL: "https://user:pass@archive.example.com"},
			wantErr: true,
		},
		{
			name:    "query rejected",
			cfg:     Config{URL: "https://archive.example.com?token=abc"},
			wantErr: true,
		},
		{
			name:    "fragment rejected",
			cfg:     Config{URL: "https://archive.example.com#section"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Module{cfg: tt.cfg}

			err := m.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestURLDefaultsToProductionEndpoint(t *testing.T) {
	m := &Module{}
	require.Equal(t, DefaultURL, m.URL())

	m = &Module{cfg: Config{URL: "https://custom.example.com"}}
	require.Equal(t, "https://custom.example.com", m.URL())
}
