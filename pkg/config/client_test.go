package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSubstituteEnvVars(t *testing.T) {
	tests := []struct {
		name    string
		content string
		env     map[string]string
		want    string
	}{
		{
			name:    "simple variable",
			content: "url: ${HOST}",
			env:     map[string]string{"HOST": "example.com"},
			want:    "url: example.com",
		},
		{
			name:    "missing variable becomes empty",
			content: "url: ${HOST}",
			want:    "url: ",
		},
		{
			name:    "default used when variable unset",
			content: "url: ${HOST:-localhost}",
			want:    "url: localhost",
		},
		{
			name:    "default ignored when variable set",
			content: "url: ${HOST:-localhost}",
			env:     map[string]string{"HOST": "example.com"},
			want:    "url: example.com",
		},
		{
			name:    "comment lines are skipped",
			content: "# url: ${HOST}",
			env:     map[string]string{"HOST": "example.com"},
			want:    "# url: ${HOST}",
		},
		{
			name:    "multiple variables on one line",
			content: "addr: ${HOST}:${PORT}",
			env:     map[string]string{"HOST": "example.com", "PORT": "8080"},
			want:    "addr: example.com:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := substituteEnvVars(tt.content)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClientConfigServerURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ClientConfig
		want string
	}{
		{
			name: "nil receiver",
			cfg:  nil,
			want: "",
		},
		{
			name: "explicit url wins and trailing slash trimmed",
			cfg: &ClientConfig{Server: ServerConfig{
				URL:     "http://server:9000/",
				BaseURL: "http://other:1234",
			}},
			want: "http://server:9000",
		},
		{
			name: "base_url used when url empty",
			cfg:  &ClientConfig{Server: ServerConfig{BaseURL: "http://server:1234/"}},
			want: "http://server:1234",
		},
		{
			name: "host and port composed",
			cfg:  &ClientConfig{Server: ServerConfig{Host: "myhost", Port: 1234}},
			want: "http://myhost:1234",
		},
		{
			name: "wildcard host normalizes to localhost",
			cfg:  &ClientConfig{Server: ServerConfig{Host: "0.0.0.0", Port: 1234}},
			want: "http://localhost:1234",
		},
		{
			name: "empty host normalizes to localhost with default port",
			cfg:  &ClientConfig{Server: ServerConfig{}},
			want: "http://localhost:2480",
		},
		{
			name: "ipv6 host is bracketed",
			cfg:  &ClientConfig{Server: ServerConfig{Host: "::1", Port: 1234}},
			want: "http://[::1]:1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.ServerURL())
		})
	}
}
