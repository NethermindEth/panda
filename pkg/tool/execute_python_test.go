package tool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestResourceTipCacheMarkShown(t *testing.T) {
	c := &resourceTipCache{entries: make(map[string]time.Time, 8)}

	assert.True(t, c.markShown("session-a"), "first mark should report newly shown")
	assert.False(t, c.markShown("session-a"), "second mark for same key should report already shown")
	assert.True(t, c.markShown("session-b"), "a different key should report newly shown")
}

func TestResourceTipCacheCleanupLockedEvictsStaleEntries(t *testing.T) {
	c := &resourceTipCache{entries: make(map[string]time.Time, 8)}

	c.entries["stale"] = time.Now().Add(-resourceTipCacheMaxAge - time.Hour)
	c.entries["fresh"] = time.Now()

	c.cleanupLocked()

	_, staleExists := c.entries["stale"]
	_, freshExists := c.entries["fresh"]

	assert.False(t, staleExists, "entries older than max age should be evicted")
	assert.True(t, freshExists, "recent entries should be retained")
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "bytes", bytes: 512, want: "512 B"},
		{name: "kilobytes", bytes: 2048, want: "2.0 KB"},
		{name: "megabytes", bytes: 5 * 1024 * 1024, want: "5.0 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatSize(tt.bytes))
		})
	}
}
