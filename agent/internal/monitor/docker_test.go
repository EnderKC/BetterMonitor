//go:build !monitor_only

package monitor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/server-ops-agent/pkg/logger"
)

func TestDockerImageRefValidation(t *testing.T) {
	for _, image := range []string{"alpine:3.20", "registry.example.test/team/image@sha256:abc123"} {
		assert.NoError(t, validateDockerImageRef(image), image)
	}
	for _, image := range []string{"", " alpine", "alpine latest", "alpine\nlatest", "-alpine"} {
		assert.Error(t, validateDockerImageRef(image), image)
	}
}

func TestDockerPullUsesContextRunnerAndHidesCommandOutput(t *testing.T) {
	log, err := logger.New("", "error")
	require.NoError(t, err)
	runnerErr := errors.New("exit status 1")
	var capturedContext context.Context
	var capturedName string
	var capturedArgs []string
	dm := &DockerManager{
		ctx: context.WithValue(context.Background(), struct{}{}, "request-context"),
		log: log,
		runCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			capturedContext = ctx
			capturedName = name
			capturedArgs = append([]string(nil), args...)
			return []byte("registry token and private command output"), runnerErr
		},
	}

	err = dm.PullImage("alpine:3.20")
	assert.ErrorIs(t, err, ErrDockerImagePullFailed)
	assert.NotContains(t, err.Error(), "registry token")
	assert.NotContains(t, err.Error(), "private command output")
	assert.Equal(t, "request-context", capturedContext.Value(struct{}{}))
	assert.Equal(t, "docker", capturedName)
	assert.Equal(t, []string{"pull", "alpine:3.20"}, capturedArgs)
}

func TestDockerPullRejectsLeadingDashBeforeRunner(t *testing.T) {
	called := false
	dm := &DockerManager{
		ctx: context.Background(),
		runCommand: func(context.Context, string, ...string) ([]byte, error) {
			called = true
			return nil, nil
		},
	}

	err := dm.PullImage("-alpine")
	assert.Error(t, err)
	assert.False(t, called)
}
