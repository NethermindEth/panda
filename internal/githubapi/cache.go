package githubapi

import (
	"encoding/json"
	"fmt"
	"os"
)

// ReadCache reads and JSON-decodes a cache file into a value of type T.
func ReadCache[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cache T
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("decoding cache: %w", err)
	}

	return &cache, nil
}

// WriteCache JSON-encodes a cache value and writes it to a file.
func WriteCache[T any](path string, cache *T) error {
	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("encoding cache: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}
