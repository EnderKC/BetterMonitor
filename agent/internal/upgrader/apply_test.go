package upgrader

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeApplyOps struct {
	calls       []string
	backupErr   error
	replaceErr  error
	restoreErr  error
	execResults []error
}

func (ops *fakeApplyOps) Backup(string, string) error {
	ops.calls = append(ops.calls, "backup")
	return ops.backupErr
}
func (ops *fakeApplyOps) Replace(string, string) error {
	ops.calls = append(ops.calls, "replace")
	return ops.replaceErr
}
func (ops *fakeApplyOps) Restore(string, string) error {
	ops.calls = append(ops.calls, "restore")
	return ops.restoreErr
}
func (ops *fakeApplyOps) Exec(string, []string, []string) error {
	ops.calls = append(ops.calls, "exec")
	if len(ops.execResults) == 0 {
		return nil
	}
	result := ops.execResults[0]
	ops.execResults = ops.execResults[1:]
	return result
}

func TestApplyAndRollbackEnforcesBackupReplaceExecAndRecoveryOrder(t *testing.T) {
	tests := []struct {
		name        string
		ops         *fakeApplyOps
		wantCalls   []string
		wantCode    string
		wantOutcome string
	}{
		{
			name:        "backup failure stops before replace",
			ops:         &fakeApplyOps{backupErr: errors.New("backup failed")},
			wantCalls:   []string{"backup"},
			wantCode:    "backup_failed",
			wantOutcome: "failed",
		},
		{
			name:        "replace failure restores backup",
			ops:         &fakeApplyOps{replaceErr: errors.New("replace failed")},
			wantCalls:   []string{"backup", "replace", "restore"},
			wantCode:    "replace_failed",
			wantOutcome: "rolled_back",
		},
		{
			name:        "new exec failure restores and restarts old binary",
			ops:         &fakeApplyOps{execResults: []error{errors.New("new exec failed"), nil}},
			wantCalls:   []string{"backup", "replace", "exec", "restore", "exec"},
			wantCode:    "upgrade_rolled_back",
			wantOutcome: "rolled_back",
		},
		{
			name:        "restore failure is terminal",
			ops:         &fakeApplyOps{execResults: []error{errors.New("new exec failed")}, restoreErr: errors.New("restore failed")},
			wantCalls:   []string{"backup", "replace", "exec", "restore"},
			wantCode:    "rollback_restore_failed",
			wantOutcome: "failed",
		},
		{
			name:        "old exec failure is terminal",
			ops:         &fakeApplyOps{execResults: []error{errors.New("new exec failed"), errors.New("old exec failed")}},
			wantCalls:   []string{"backup", "replace", "exec", "restore", "exec"},
			wantCode:    "rollback_restart_failed",
			wantOutcome: "failed",
		},
		{
			name:        "successful exec follows mandatory backup and replace",
			ops:         &fakeApplyOps{},
			wantCalls:   []string{"backup", "replace", "exec"},
			wantCode:    "",
			wantOutcome: "applied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markerPath := filepath.Join(t.TempDir(), "upgrade-marker.json")
			req := UpgradeRequest{
				RequestID:       "job-request-id",
				TargetVersion:   "1.4.0",
				TargetAgentType: "monitor",
				MarkerPath:      markerPath,
				Args:            []string{"agent", "--config", "agent.yaml"},
				Env:             []string{"A=B"},
			}
			err := applyAndRollback(context.Background(), req, "/opt/agent", "/opt/agent.new", tt.ops, nil)
			assert.Equal(t, tt.wantCode, UpgradeErrorCode(err))
			assert.Equal(t, tt.wantCalls, tt.ops.calls)

			marker, loadErr := LoadUpgradeMarker(markerPath)
			require.NoError(t, loadErr)
			require.NotNil(t, marker)
			assert.Equal(t, tt.wantOutcome, marker.Outcome)
			if tt.wantCode != "" {
				assert.NotEmpty(t, marker.ErrorCode)
			}
		})
	}
}
