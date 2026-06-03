package eips

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/panda/internal/githubapi"
	"github.com/ethpandaops/panda/pkg/types"
)

type cacheData struct {
	CommitSHA string      `json:"commit_sha"`
	FetchedAt time.Time   `json:"fetched_at"`
	EIPs      []types.EIP `json:"eips"`
}

// Registry manages a collection of parsed EIPs with disk caching.
type Registry struct {
	eips []types.EIP
	mu   sync.RWMutex
}

// NewRegistry creates an EIP registry, fetching from GitHub if the
// cache is stale.
func NewRegistry(
	ctx context.Context,
	log logrus.FieldLogger,
	cacheDir string,
) (*Registry, error) {
	log = log.WithField("component", "eip_registry")

	if cacheDir == "" {
		userCache, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("determining cache directory: %w", err)
		}

		cacheDir = filepath.Join(userCache, "ethpandaops-panda", "eips")
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	f := newFetcher()

	latestSHA, err := f.latestCommitSHA(ctx)
	if err != nil {
		log.WithError(err).
			Warn("Failed to check latest EIP commit — trying cache")

		return loadFromCache(log, cacheDir)
	}

	cached, cacheErr := readCache(cacheDir)
	if cacheErr == nil && cached.CommitSHA == latestSHA {
		log.WithField("commit", latestSHA[:8]).
			Info("EIP cache is current")

		return buildRegistry(cached), nil
	}

	log.WithField("commit", latestSHA[:8]).
		Info("Fetching EIPs from GitHub")

	eipList, err := f.fetchAll(ctx)
	if err != nil {
		log.WithError(err).
			Warn("Failed to fetch EIPs — trying cache")

		return loadFromCache(log, cacheDir)
	}

	sort.Slice(eipList, func(i, j int) bool {
		return eipList[i].Number < eipList[j].Number
	})

	newCache := &cacheData{
		CommitSHA: latestSHA,
		FetchedAt: time.Now(),
		EIPs:      eipList,
	}

	if err := writeCache(cacheDir, newCache); err != nil {
		log.WithError(err).Warn("Failed to write EIP cache")
	}

	log.WithField("eip_count", len(eipList)).
		Info("EIP registry initialized")

	return buildRegistry(newCache), nil
}

// All returns a copy of all EIPs.
func (r *Registry) All() []types.EIP {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]types.EIP, len(r.eips))
	copy(out, r.eips)

	return out
}

// Count returns the number of EIPs in the registry.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.eips)
}

// Statuses returns sorted unique status values across all EIPs.
func (r *Registry) Statuses() []string {
	return r.uniqueField(func(e types.EIP) string { return e.Status })
}

// Categories returns sorted unique category values across all EIPs.
func (r *Registry) Categories() []string {
	return r.uniqueField(func(e types.EIP) string { return e.Category })
}

// Types returns sorted unique type values across all EIPs.
func (r *Registry) Types() []string {
	return r.uniqueField(func(e types.EIP) string { return e.Type })
}

func (r *Registry) uniqueField(
	extract func(types.EIP) string,
) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]struct{}, 16)

	for _, e := range r.eips {
		if v := extract(e); v != "" {
			seen[v] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}

	sort.Strings(out)

	return out
}

func buildRegistry(cache *cacheData) *Registry {
	eips := make([]types.EIP, len(cache.EIPs))
	copy(eips, cache.EIPs)

	return &Registry{
		eips: eips,
	}
}

func loadFromCache(
	log logrus.FieldLogger,
	cacheDir string,
) (*Registry, error) {
	cached, err := readCache(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("no cached EIPs available: %w", err)
	}

	log.WithField("eip_count", len(cached.EIPs)).
		Info("Loaded EIPs from cache")

	return buildRegistry(cached), nil
}

func cachePath(cacheDir string) string {
	return filepath.Join(cacheDir, "eips.json")
}

func readCache(cacheDir string) (*cacheData, error) {
	return githubapi.ReadCache[cacheData](cachePath(cacheDir))
}

func writeCache(cacheDir string, cache *cacheData) error {
	return githubapi.WriteCache(cachePath(cacheDir), cache)
}
