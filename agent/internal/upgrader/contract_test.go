package upgrader

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentversion "github.com/user/server-ops-agent/pkg/version"
)

func TestUpgradeInstructionRequiresCompleteStrictContract(t *testing.T) {
	assetName := "better-monitor-agent-1.4.0-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	valid := UpgradeInstruction{
		RequestID:       "job-request-id",
		TargetVersion:   "1.4.0",
		TargetAgentType: "full",
		AssetName:       assetName,
		AssetSize:       1024,
		DownloadURL:     "https://downloads.example/" + assetName,
		SHA256:          strings.Repeat("a", 64),
	}
	require.NoError(t, ValidateUpgradeInstruction(valid))

	tests := map[string]func(*UpgradeInstruction){
		"missing request id":     func(v *UpgradeInstruction) { v.RequestID = "" },
		"invalid semver":         func(v *UpgradeInstruction) { v.TargetVersion = "latest" },
		"invalid agent type":     func(v *UpgradeInstruction) { v.TargetAgentType = "lite" },
		"wrong asset name":       func(v *UpgradeInstruction) { v.AssetName = "agent.bin" },
		"zero asset size":        func(v *UpgradeInstruction) { v.AssetSize = 0 },
		"oversized asset":        func(v *UpgradeInstruction) { v.AssetSize = DefaultMaxDownloadBytes + 1 },
		"non https url":          func(v *UpgradeInstruction) { v.DownloadURL = "http://downloads.example/agent" },
		"url with credentials":   func(v *UpgradeInstruction) { v.DownloadURL = "https://user:pass@downloads.example/agent" },
		"invalid sha":            func(v *UpgradeInstruction) { v.SHA256 = "abc" },
		"version leading zero":   func(v *UpgradeInstruction) { v.TargetVersion = "01.4.0" },
		"version with leading v": func(v *UpgradeInstruction) { v.TargetVersion = "v1.4.0" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)
			assert.Error(t, ValidateUpgradeInstruction(input))
		})
	}
}

func TestUpgradeNoopRequiresMatchingCompiledVersionAndType(t *testing.T) {
	originalVersion := agentversion.Version
	originalType := agentversion.AgentType
	t.Cleanup(func() {
		agentversion.Version = originalVersion
		agentversion.AgentType = originalType
	})
	agentversion.Version = "1.4.0"
	agentversion.AgentType = "full"

	assert.True(t, IsUpgradeNoop(UpgradeInstruction{TargetVersion: "1.4.0", TargetAgentType: "full"}))
	assert.False(t, IsUpgradeNoop(UpgradeInstruction{TargetVersion: "1.4.0", TargetAgentType: "monitor"}))
	assert.False(t, IsUpgradeNoop(UpgradeInstruction{TargetVersion: "1.4.1", TargetAgentType: "full"}))
}
