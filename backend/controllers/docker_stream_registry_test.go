package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerStreamRegistryOwnershipAndCleanup(t *testing.T) {
	registry := newDockerStreamRegistry()
	owner := &SafeConn{}
	other := &SafeConn{}

	require.NoError(t, registry.start("stream-a", 1, owner))
	assert.ErrorIs(t, registry.start("stream-a", 1, other), errDockerStreamExists)
	assert.ErrorIs(t, registry.stop("stream-a", 1, other), errDockerStreamOwnerMismatch)
	assert.ErrorIs(t, registry.stop("stream-a", 2, owner), errDockerStreamServerMismatch)

	entry, ok := registry.current("stream-a", 1)
	require.True(t, ok)
	assert.Same(t, owner, entry.owner)
	assert.Equal(t, []dockerStreamEntry{{streamID: "stream-a", serverID: 1, owner: owner}}, registry.cleanupOwner(owner))
	assert.Zero(t, registry.count())
}

func TestDockerStreamRegistryRejectsCrossServerEnd(t *testing.T) {
	registry := newDockerStreamRegistry()
	owner := &SafeConn{}
	require.NoError(t, registry.start("stream-a", 1, owner))

	_, ok := registry.end("stream-a", 2)
	assert.False(t, ok)
	_, ok = registry.end("stream-a", 1)
	assert.True(t, ok)
	assert.Zero(t, registry.count())
}
