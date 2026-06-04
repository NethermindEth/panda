package githubapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToken(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "no token",
			env:  map[string]string{},
			want: "",
		},
		{
			name: "github token only",
			env:  map[string]string{"GITHUB_TOKEN": "ghtok"},
			want: "ghtok",
		},
		{
			name: "gh token only",
			env:  map[string]string{"GH_TOKEN": "ghtok2"},
			want: "ghtok2",
		},
		{
			name: "github token takes precedence over gh token",
			env:  map[string]string{"GITHUB_TOKEN": "primary", "GH_TOKEN": "secondary"},
			want: "primary",
		},
		{
			name: "whitespace is trimmed",
			env:  map[string]string{"GITHUB_TOKEN": "  spaced  "},
			want: "spaced",
		},
		{
			name: "blank github token falls through to gh token",
			env:  map[string]string{"GITHUB_TOKEN": "   ", "GH_TOKEN": "fallback"},
			want: "fallback",
		},
	}

	original := getenv
	t.Cleanup(func() { getenv = original })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv = func(key string) string { return tt.env[key] }

			assert.Equal(t, tt.want, Token())
		})
	}
}
