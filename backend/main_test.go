package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAgentUpgradeTimeoutReconcilerProcessesTicksAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	processed := make(chan time.Time, 1)
	done := make(chan struct{})

	go func() {
		runAgentUpgradeTimeoutReconciler(ctx, ticks, func(now time.Time) error {
			processed <- now
			return nil
		})
		close(done)
	}()

	want := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	ticks <- want
	require.Equal(t, want, <-processed)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconciler did not stop after context cancellation")
	}
}

func TestAgentUpgradeReconcileIntervalIsThirtySeconds(t *testing.T) {
	assert.Equal(t, 30*time.Second, agentUpgradeReconcileInterval)
}
