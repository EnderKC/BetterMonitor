package main

import (
	"bytes"
	"encoding/json"
	"io"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentversion "github.com/user/server-ops-agent/pkg/version"
)

func TestSelfTestReturnsCompiledIdentityBeforeNormalStartup(t *testing.T) {
	originalVersion := agentversion.Version
	originalType := agentversion.AgentType
	t.Cleanup(func() {
		agentversion.Version = originalVersion
		agentversion.AgentType = originalType
	})
	agentversion.Version = "1.4.0"
	agentversion.AgentType = "monitor"

	var output bytes.Buffer
	handled, err := maybeRunSelfTest([]string{"--self-test", "--config", "/definitely/missing.yaml"}, &output)
	require.NoError(t, err)
	assert.True(t, handled)

	decoder := json.NewDecoder(&output)
	var result selfTestResult
	require.NoError(t, decoder.Decode(&result))
	assert.Equal(t, "1.4.0", result.Version)
	assert.Equal(t, "monitor", result.AgentType)
	assert.Equal(t, runtime.GOOS, result.OS)
	assert.Equal(t, runtime.GOARCH, result.Arch)
	var extra interface{}
	assert.ErrorIs(t, decoder.Decode(&extra), io.EOF)

	handled, err = maybeRunSelfTest([]string{"--version"}, io.Discard)
	require.NoError(t, err)
	assert.False(t, handled)
}
