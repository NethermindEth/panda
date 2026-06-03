package sandbox

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/panda/pkg/config"
)

func TestSessionManagerStopIsIdempotent(t *testing.T) {
	enabled := true
	cfg := config.SessionConfig{
		Enabled:     &enabled,
		MaxSessions: 1,
	}

	m := NewSessionManager(
		cfg,
		logrus.New(),
		func(context.Context, string) (*SessionContainer, error) { return nil, nil },
		func(context.Context) ([]*SessionContainer, error) { return nil, nil },
		func(context.Context, string) error { return nil },
	)

	ctx := context.Background()
	require.NoError(t, m.Start(ctx))

	require.NoError(t, m.Stop(ctx))
	// A second Stop must not panic on close of an already-closed channel.
	require.NotPanics(t, func() {
		_ = m.Stop(ctx)
	})
}

func TestSessionManagerRemoveSessionThenUnmarkDoesNotRecreateState(t *testing.T) {
	m := NewSessionManager(
		config.SessionConfig{MaxSessions: 1},
		logrus.New(),
		func(context.Context, string) (*SessionContainer, error) { return nil, nil },
		func(context.Context) ([]*SessionContainer, error) { return nil, nil },
		func(context.Context, string) error { return nil },
	)

	const sessionID = "session-1"

	m.RecordAccess(sessionID)
	m.markExecuting(sessionID)
	m.removeSession(sessionID)
	m.unmarkExecuting(sessionID)

	m.mu.Lock()
	defer m.mu.Unlock()

	require.NotContains(t, m.lastUsed, sessionID)
	require.NotContains(t, m.activeExecs, sessionID)
}
