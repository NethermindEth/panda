package consensusspecs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/panda/internal/githubapi"
	"github.com/ethpandaops/panda/pkg/config"
	"github.com/ethpandaops/panda/pkg/types"
)

type cacheData struct {
	CommitSHA string                `json:"commit_sha"`
	Ref       string                `json:"ref"`
	FetchedAt time.Time             `json:"fetched_at"`
	Specs     []types.ConsensusSpec `json:"specs"`
	Constants []types.SpecConstant  `json:"constants"`
}

// Registry manages a collection of parsed consensus specs with disk caching.
type Registry struct {
	specs     []types.ConsensusSpec
	constants []types.SpecConstant
	mu        sync.RWMutex
}

// NewRegistry creates a consensus specs registry, fetching from GitHub if the
// cache is stale.
func NewRegistry(
	ctx context.Context,
	log logrus.FieldLogger,
	cfg config.ConsensusSpecsConfig,
	cacheDir string,
) (*Registry, error) {
	log = log.WithField("component", "consensus_specs_registry")

	if cacheDir == "" {
		userCache, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("determining cache directory: %w", err)
		}

		cacheDir = filepath.Join(userCache, "ethpandaops-panda", "consensus-specs")
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	f := newFetcher(cfg)

	ref, err := f.resolveRef(ctx)
	if err != nil {
		log.WithError(err).
			Warn("Failed to resolve consensus-specs ref — trying cache")

		return loadFromCache(log, cacheDir)
	}

	log.WithField("ref", ref).Info("Resolved consensus-specs ref")

	latestSHA, err := f.latestCommitSHA(ctx, ref)
	if err != nil {
		log.WithError(err).
			Warn("Failed to check latest commit — trying cache")

		return loadFromCache(log, cacheDir)
	}

	cached, cacheErr := readCache(cacheDir)
	if cacheErr == nil && cached.CommitSHA == latestSHA {
		log.WithField("commit", latestSHA[:8]).
			Info("Consensus specs cache is current")

		return buildRegistry(cached), nil
	}

	log.WithFields(logrus.Fields{
		"commit": latestSHA[:8],
		"ref":    ref,
	}).Info("Fetching consensus specs from GitHub")

	result, err := f.fetchAll(ctx, ref)
	if err != nil {
		log.WithError(err).
			Warn("Failed to fetch consensus specs — trying cache")

		return loadFromCache(log, cacheDir)
	}

	sortSpecs(result.Specs)
	sortConstants(result.Constants)

	newCache := &cacheData{
		CommitSHA: latestSHA,
		Ref:       ref,
		FetchedAt: time.Now(),
		Specs:     result.Specs,
		Constants: result.Constants,
	}

	if err := writeCache(cacheDir, newCache); err != nil {
		log.WithError(err).Warn("Failed to write consensus specs cache")
	}

	log.WithFields(logrus.Fields{
		"spec_count":     len(result.Specs),
		"constant_count": len(result.Constants),
	}).Info("Consensus specs registry initialized")

	return buildRegistry(newCache), nil
}

// AllSpecs returns a copy of all specs.
func (r *Registry) AllSpecs() []types.ConsensusSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]types.ConsensusSpec, len(r.specs))
	copy(out, r.specs)

	return out
}

// AllConstants returns a copy of all constants.
func (r *Registry) AllConstants() []types.SpecConstant {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]types.SpecConstant, len(r.constants))
	copy(out, r.constants)

	return out
}

// SpecCount returns the number of spec documents.
func (r *Registry) SpecCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.specs)
}

// ConstantCount returns the number of constants.
func (r *Registry) ConstantCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.constants)
}

// Forks returns sorted unique fork names across all specs.
func (r *Registry) Forks() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]struct{}, 8)
	for _, s := range r.specs {
		seen[s.Fork] = struct{}{}
	}

	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}

	sort.Strings(out)

	return out
}

// GetConstant finds a constant by name, optionally filtered to a specific fork.
// When fork is empty, returns the constant from the latest fork that defines it.
func (r *Registry) GetConstant(name string, fork string) (types.SpecConstant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	upperName := strings.ToUpper(name)

	var best types.SpecConstant

	found := false

	for _, c := range r.constants {
		if strings.ToUpper(c.Name) != upperName {
			continue
		}

		if fork != "" && c.Fork != fork {
			continue
		}

		// Take the last match (constants are sorted; later forks override).
		best = c
		found = true
	}

	return best, found
}

// GetSpec finds a spec by fork and topic.
func (r *Registry) GetSpec(fork, topic string) (types.ConsensusSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, s := range r.specs {
		if s.Fork == fork && s.Topic == topic {
			return s, true
		}
	}

	return types.ConsensusSpec{}, false
}

func buildRegistry(cache *cacheData) *Registry {
	specs := make([]types.ConsensusSpec, len(cache.Specs))
	copy(specs, cache.Specs)

	constants := make([]types.SpecConstant, len(cache.Constants))
	copy(constants, cache.Constants)

	return &Registry{
		specs:     specs,
		constants: constants,
	}
}

func loadFromCache(
	log logrus.FieldLogger,
	cacheDir string,
) (*Registry, error) {
	cached, err := readCache(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("no cached consensus specs available: %w", err)
	}

	log.WithFields(logrus.Fields{
		"spec_count":     len(cached.Specs),
		"constant_count": len(cached.Constants),
	}).Info("Loaded consensus specs from cache")

	return buildRegistry(cached), nil
}

func cachePath(cacheDir string) string {
	return filepath.Join(cacheDir, "consensus-specs.json")
}

func readCache(cacheDir string) (*cacheData, error) {
	return githubapi.ReadCache[cacheData](cachePath(cacheDir))
}

func writeCache(cacheDir string, cache *cacheData) error {
	return githubapi.WriteCache(cachePath(cacheDir), cache)
}

// sortSpecs sorts specs by fork then topic.
func sortSpecs(specs []types.ConsensusSpec) {
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].Fork != specs[j].Fork {
			return forkOrder(specs[i].Fork) < forkOrder(specs[j].Fork)
		}

		return specs[i].Topic < specs[j].Topic
	})
}

// sortConstants sorts constants by fork order then name.
func sortConstants(constants []types.SpecConstant) {
	sort.Slice(constants, func(i, j int) bool {
		if constants[i].Fork != constants[j].Fork {
			return forkOrder(constants[i].Fork) < forkOrder(constants[j].Fork)
		}

		return constants[i].Name < constants[j].Name
	})
}

// forkOrderMap is a static lookup table for consensus fork ordering.
// Declared at package level to avoid allocating a new map on every sort
// comparison.
var forkOrderMap = map[string]int{
	"_config":   0,
	"phase0":    1,
	"altair":    2,
	"bellatrix": 3,
	"capella":   4,
	"deneb":     5,
	"electra":   6,
	"fulu":      7,
}

// forkOrder returns a numeric sort key for consensus layer fork names.
func forkOrder(fork string) int {
	if v, ok := forkOrderMap[fork]; ok {
		return v
	}

	return 99
}
