package consensusspecs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		fork          string
		topic         string
		data          string
		repository    string
		ref           string
		expectedTitle string
		expectedURL   string
	}{
		{
			name:          "title from first heading",
			fork:          "deneb",
			topic:         "beacon-chain",
			data:          "intro line\n# The Beacon Chain\nmore content",
			repository:    "ethereum/consensus-specs",
			ref:           "v1.4.0",
			expectedTitle: "The Beacon Chain",
			expectedURL:   "https://github.com/ethereum/consensus-specs/blob/v1.4.0/specs/deneb/beacon-chain.md",
		},
		{
			name:          "falls back to fork/topic when no heading",
			fork:          "capella",
			topic:         "validator",
			data:          "no heading here\njust text",
			repository:    "ethereum/consensus-specs",
			ref:           "main",
			expectedTitle: "capella/validator",
			expectedURL:   "https://github.com/ethereum/consensus-specs/blob/main/specs/capella/validator.md",
		},
		{
			name:          "ignores deeper headings",
			fork:          "altair",
			topic:         "sync-protocol",
			data:          "## Subsection\n# Real Title\n### Another",
			repository:    "ethereum/consensus-specs",
			ref:           "v1.3.0",
			expectedTitle: "Real Title",
			expectedURL:   "https://github.com/ethereum/consensus-specs/blob/v1.3.0/specs/altair/sync-protocol.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := ParseSpec(tt.fork, tt.topic, []byte(tt.data), tt.repository, tt.ref)

			assert.Equal(t, tt.fork, spec.Fork)
			assert.Equal(t, tt.topic, spec.Topic)
			assert.Equal(t, tt.expectedTitle, spec.Title)
			assert.Equal(t, tt.expectedURL, spec.URL)
			assert.NotEmpty(t, spec.Content)
		})
	}
}

func TestParseSpecTrimsContent(t *testing.T) {
	t.Parallel()

	spec := ParseSpec("phase0", "beacon-chain", []byte("\n\n  # Title  \nbody\n\n"), "ethereum/consensus-specs", "main")

	assert.Equal(t, "# Title  \nbody", spec.Content)
	assert.Equal(t, "Title", spec.Title)
}

func TestParsePreset(t *testing.T) {
	t.Parallel()

	data := []byte("# comment line is yaml-stripped\nMAX_COMMITTEES_PER_SLOT: 64\nSLOTS_PER_EPOCH: 32\nGENESIS_FORK_VERSION: '0x00000000'\n")

	constants, err := ParsePreset("phase0", data)
	require.NoError(t, err)

	byName := make(map[string]string, len(constants))
	for _, c := range constants {
		assert.Equal(t, "phase0", c.Fork)
		byName[c.Name] = c.Value
	}

	assert.Equal(t, "64", byName["MAX_COMMITTEES_PER_SLOT"])
	assert.Equal(t, "32", byName["SLOTS_PER_EPOCH"])
	assert.Equal(t, "0x00000000", byName["GENESIS_FORK_VERSION"])
}

func TestParsePresetInvalidYAML(t *testing.T) {
	t.Parallel()

	_, err := ParsePreset("phase0", []byte("not: valid: yaml: here:"))
	require.Error(t, err)
}

func TestFormatValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"int64", int64(64), "64"},
		{"float whole", float64(32), "32"},
		{"float fractional", 3.5, "3.5"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"fallback", []int{1, 2}, "[1 2]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, formatValue(tt.input))
		})
	}
}
