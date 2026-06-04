package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompleteEthNodeArgsSuppressesUnavailableListError(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/operations/ethnode.list_networks" {
			http.NotFound(w, r)
			return
		}

		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "Ethnode is not enabled or no node access is available.",
		})
	}))
	defer server.Close()

	setClientConfig(t, server.URL)

	var (
		names     []string
		directive cobra.ShellCompDirective
		stdout    string
	)

	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			names, directive = completeEthNodeArgs(&cobra.Command{}, nil, "")
		})
	})

	require.Equal(t, int32(1), requests.Load())
	assert.Empty(t, names)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stderr
	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	output := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		output <- buf.String()
	}()

	os.Stderr = writer
	defer func() { os.Stderr = original }()

	fn()

	require.NoError(t, writer.Close())

	return <-output
}
