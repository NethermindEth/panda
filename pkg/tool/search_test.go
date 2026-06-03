package tool

import (
	"context"
	"io"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/panda/pkg/searchsvc"
)

func newTestSearchHandler() *searchHandler {
	log := logrus.New()
	log.SetOutput(io.Discard)

	return &searchHandler{
		log:     log.WithField("tool", SearchToolName),
		service: nil,
	}
}

func searchRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      SearchToolName,
			Arguments: args,
		},
	}
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()

	require.NotNil(t, res)
	require.Len(t, res.Content, 1)

	text, ok := res.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected text content")

	return text.Text
}

func TestSearchHandlerValidation(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		wantErr  bool
		contains string
	}{
		{
			name:     "empty query is rejected",
			args:     map[string]any{"query": ""},
			wantErr:  true,
			contains: "query is required",
		},
		{
			name:     "missing query is rejected",
			args:     map[string]any{},
			wantErr:  true,
			contains: "query is required",
		},
		{
			name:     "unsupported type is rejected",
			args:     map[string]any{"query": "x", "type": "bogus"},
			wantErr:  true,
			contains: "unsupported search type",
		},
		{
			name:     "tag rejected for examples",
			args:     map[string]any{"query": "x", "type": "examples", "tag": "finality"},
			wantErr:  true,
			contains: "tag is only supported",
		},
		{
			name:     "tag rejected for eips",
			args:     map[string]any{"query": "x", "type": "eips", "tag": "finality"},
			wantErr:  true,
			contains: "tag is only supported",
		},
		{
			name:     "tag rejected for consensus-specs",
			args:     map[string]any{"query": "x", "type": "consensus-specs", "tag": "finality"},
			wantErr:  true,
			contains: "tag is only supported",
		},
		{
			name:     "category rejected for runbooks",
			args:     map[string]any{"query": "x", "type": "runbooks", "category": "validators"},
			wantErr:  true,
			contains: "category is only supported",
		},
		{
			name:     "category rejected for eips",
			args:     map[string]any{"query": "x", "type": "eips", "category": "validators"},
			wantErr:  true,
			contains: "category is only supported",
		},
		{
			name:     "category rejected for consensus-specs",
			args:     map[string]any{"query": "x", "type": "consensus-specs", "category": "validators"},
			wantErr:  true,
			contains: "category is only supported",
		},
	}

	h := newTestSearchHandler()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := h.handle(context.Background(), searchRequest(tt.args))
			require.NoError(t, err)
			require.NotNil(t, res)

			assert.Equal(t, tt.wantErr, res.IsError)

			if tt.contains != "" {
				assert.Contains(t, resultText(t, res), tt.contains)
			}
		})
	}
}

func TestSearchHandlerAliasesRouteToCanonicalType(t *testing.T) {
	// Aliases fold into their canonical type before reaching the service.
	// A nil-index service surfaces the canonical "not available" message,
	// which proves the alias routed to the same handler as its canonical name.
	tests := []struct {
		alias     string
		canonical string
		unavail   string
	}{
		{alias: "notebooks", canonical: searchsvc.SearchTypeRunbooks, unavail: "runbook search index not available"},
		{alias: "specs", canonical: searchsvc.SearchTypeConsensusSpecs, unavail: "consensus specs search index not available"},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			normalized, err := searchsvc.NormalizeSearchType(tt.alias)
			require.NoError(t, err)
			assert.Equal(t, tt.canonical, normalized)

			h := &searchHandler{
				log:     logrus.New().WithField("tool", SearchToolName),
				service: searchsvc.New(nil, nil, nil, nil, nil, nil, nil, nil),
			}

			res, err := h.handle(context.Background(), searchRequest(map[string]any{
				"query": "x",
				"type":  tt.alias,
			}))
			require.NoError(t, err)
			require.True(t, res.IsError)
			assert.Contains(t, resultText(t, res), tt.unavail)
		})
	}
}
