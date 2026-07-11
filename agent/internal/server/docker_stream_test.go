//go:build !monitor_only

package server

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/server-ops-agent/pkg/logger"
)

func TestCloseOperationResourcesStopsAllDockerLogStreams(t *testing.T) {
	canceled := false
	ctx, cancel := context.WithCancel(context.Background())
	trackedCancel := func() {
		canceled = true
		cancel()
	}
	log, err := logger.New("", "error")
	require.NoError(t, err)
	c := &Client{log: log}
	c.logStreams = map[string]*logStreamSession{
		"stream-a": {
			reader: io.NopCloser(strings.NewReader("logs")),
			cancel: trackedCancel,
			stopCh: make(chan struct{}),
		},
	}

	c.closeOperationResources()

	assert.True(t, canceled)
	assert.Error(t, ctx.Err())
	assert.Empty(t, c.logStreams)
}
