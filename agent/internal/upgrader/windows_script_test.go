package upgrader

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWindowsUpdaterScriptChecksEveryStepAndUsesScheduledTaskRollback(t *testing.T) {
	script := BuildWindowsUpdaterScript()

	for _, required := range []string{
		"Wait-ForProcessExit",
		"BackupExe",
		"Copy-Item",
		"Move-Item",
		"Start-ScheduledTask",
		"Get-ScheduledTaskInfo",
		"State -eq 'Running'",
		"ScheduledTaskName",
		"Write-UpgradeOutcome",
		"rolled_back",
		"rollback_restore_failed",
		"rollback_restart_failed",
		"exit 1",
	} {
		assert.Contains(t, script, required)
	}
	assert.NotContains(t, script, "Start-Process -FilePath $OldExe")
	assert.NotContains(t, strings.ToLower(script), "silentlycontinue")
}
