package consensusspecs

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ethpandaops/panda/pkg/types"
)

func newTestRegistry(specs []types.ConsensusSpec, constants []types.SpecConstant) *Registry {
	return buildRegistry(&cacheData{Specs: specs, Constants: constants})
}

func TestForkOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fork     string
		expected int
	}{
		{"_config", 0},
		{"phase0", 1},
		{"altair", 2},
		{"bellatrix", 3},
		{"capella", 4},
		{"deneb", 5},
		{"electra", 6},
		{"fulu", 7},
		{"unknown-fork", 99},
		{"", 99},
	}

	for _, tt := range tests {
		t.Run(tt.fork, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, forkOrder(tt.fork))
		})
	}
}

func TestSortSpecs(t *testing.T) {
	t.Parallel()

	specs := []types.ConsensusSpec{
		{Fork: "deneb", Topic: "validator"},
		{Fork: "phase0", Topic: "fork-choice"},
		{Fork: "phase0", Topic: "beacon-chain"},
		{Fork: "altair", Topic: "beacon-chain"},
	}

	sortSpecs(specs)

	got := make([][2]string, len(specs))
	for i, s := range specs {
		got[i] = [2]string{s.Fork, s.Topic}
	}

	want := [][2]string{
		{"phase0", "beacon-chain"},
		{"phase0", "fork-choice"},
		{"altair", "beacon-chain"},
		{"deneb", "validator"},
	}

	assert.Equal(t, want, got)
}

func TestSortConstants(t *testing.T) {
	t.Parallel()

	constants := []types.SpecConstant{
		{Fork: "deneb", Name: "MAX_BLOBS"},
		{Fork: "phase0", Name: "SLOTS_PER_EPOCH"},
		{Fork: "phase0", Name: "MAX_COMMITTEES_PER_SLOT"},
	}

	sortConstants(constants)

	got := make([][2]string, len(constants))
	for i, c := range constants {
		got[i] = [2]string{c.Fork, c.Name}
	}

	want := [][2]string{
		{"phase0", "MAX_COMMITTEES_PER_SLOT"},
		{"phase0", "SLOTS_PER_EPOCH"},
		{"deneb", "MAX_BLOBS"},
	}

	assert.Equal(t, want, got)
}

func TestRegistryForks(t *testing.T) {
	t.Parallel()

	reg := newTestRegistry([]types.ConsensusSpec{
		{Fork: "deneb", Topic: "beacon-chain"},
		{Fork: "phase0", Topic: "beacon-chain"},
		{Fork: "phase0", Topic: "fork-choice"},
		{Fork: "deneb", Topic: "validator"},
	}, nil)

	assert.Equal(t, []string{"deneb", "phase0"}, reg.Forks())
}

func TestRegistryGetSpec(t *testing.T) {
	t.Parallel()

	reg := newTestRegistry([]types.ConsensusSpec{
		{Fork: "phase0", Topic: "beacon-chain", Title: "Beacon"},
		{Fork: "deneb", Topic: "validator", Title: "Validator"},
	}, nil)

	spec, ok := reg.GetSpec("deneb", "validator")
	assert.True(t, ok)
	assert.Equal(t, "Validator", spec.Title)

	_, ok = reg.GetSpec("deneb", "beacon-chain")
	assert.False(t, ok)
}

func TestRegistryGetConstant(t *testing.T) {
	t.Parallel()

	constants := []types.SpecConstant{
		{Name: "MAX_BLOBS", Value: "4", Fork: "deneb"},
		{Name: "MAX_BLOBS", Value: "6", Fork: "electra"},
		{Name: "SLOTS_PER_EPOCH", Value: "32", Fork: "phase0"},
	}

	reg := newTestRegistry(nil, constants)

	tests := []struct {
		name        string
		lookup      string
		fork        string
		expectFound bool
		expectValue string
		expectFork  string
	}{
		{
			name:        "case-insensitive name match",
			lookup:      "slots_per_epoch",
			fork:        "",
			expectFound: true,
			expectValue: "32",
			expectFork:  "phase0",
		},
		{
			name:        "no fork takes last (latest) match",
			lookup:      "MAX_BLOBS",
			fork:        "",
			expectFound: true,
			expectValue: "6",
			expectFork:  "electra",
		},
		{
			name:        "fork filter selects specific fork",
			lookup:      "MAX_BLOBS",
			fork:        "deneb",
			expectFound: true,
			expectValue: "4",
			expectFork:  "deneb",
		},
		{
			name:        "fork filter with no match",
			lookup:      "MAX_BLOBS",
			fork:        "phase0",
			expectFound: false,
		},
		{
			name:        "unknown name",
			lookup:      "DOES_NOT_EXIST",
			fork:        "",
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, ok := reg.GetConstant(tt.lookup, tt.fork)
			assert.Equal(t, tt.expectFound, ok)

			if tt.expectFound {
				assert.Equal(t, tt.expectValue, c.Value)
				assert.Equal(t, tt.expectFork, c.Fork)
			}
		})
	}
}

func TestRegistryCountsAndCopies(t *testing.T) {
	t.Parallel()

	specs := []types.ConsensusSpec{{Fork: "phase0", Topic: "beacon-chain"}}
	constants := []types.SpecConstant{{Name: "A", Fork: "phase0"}, {Name: "B", Fork: "phase0"}}

	reg := newTestRegistry(specs, constants)

	assert.Equal(t, 1, reg.SpecCount())
	assert.Equal(t, 2, reg.ConstantCount())

	gotSpecs := reg.AllSpecs()
	gotSpecs[0].Fork = "mutated"
	assert.Equal(t, "phase0", reg.AllSpecs()[0].Fork, "AllSpecs must return a copy")

	gotConstants := reg.AllConstants()
	gotConstants[0].Name = "mutated"
	assert.Equal(t, "A", reg.AllConstants()[0].Name, "AllConstants must return a copy")
}
