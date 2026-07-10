package upgrader

import "strings"

type WindowsUpdaterInput struct {
	PID               int
	OldExe            string
	NewExe            string
	BackupExe         string
	ScheduledTaskName string
	OutcomePath       string
	ArgsJSON          string
}

func BuildWindowsUpdaterScript() string {
	return strings.TrimSpace(`
param(
  [Parameter(Mandatory=$true)][int]$Pid,
  [Parameter(Mandatory=$true)][string]$OldExe,
  [Parameter(Mandatory=$true)][string]$NewExe,
  [Parameter(Mandatory=$true)][string]$BackupExe,
  [Parameter(Mandatory=$true)][string]$ScheduledTaskName,
  [Parameter(Mandatory=$true)][string]$OutcomePath,
  [Parameter(Mandatory=$false)][string]$ArgsJson
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Write-UpgradeOutcome([string]$Outcome, [string]$ErrorCode) {
  $marker = Get-Content -Raw -Path $OutcomePath -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
  $marker.outcome = $Outcome
  $marker.error_code = $ErrorCode
  $temporary = "$OutcomePath.tmp"
  [System.IO.File]::WriteAllText($temporary, ($marker | ConvertTo-Json -Compress), [System.Text.UTF8Encoding]::new($false))
  Move-Item -Force -Path $temporary -Destination $OutcomePath -ErrorAction Stop
}

function Wait-ForProcessExit([int]$ProcessId) {
  for ($attempt = 0; $attempt -lt 120; $attempt++) {
    try {
      Get-Process -Id $ProcessId -ErrorAction Stop | Out-Null
      Start-Sleep -Seconds 1
    } catch [Microsoft.PowerShell.Commands.ProcessCommandException] {
      return $true
    }
  }
  return $false
}

function Start-AgentScheduledTask {
  Start-ScheduledTask -TaskName $ScheduledTaskName -ErrorAction Stop
  for ($attempt = 0; $attempt -lt 30; $attempt++) {
    $task = Get-ScheduledTask -TaskName $ScheduledTaskName -ErrorAction Stop
    $taskInfo = Get-ScheduledTaskInfo -TaskName $ScheduledTaskName -ErrorAction Stop
    if ($task.State -eq 'Running') {
      return
    }
    if ($taskInfo.LastTaskResult -ne 0 -and $taskInfo.LastTaskResult -ne 267009) {
      throw "scheduled task exited with result $($taskInfo.LastTaskResult)"
    }
    Start-Sleep -Seconds 1
  }
  throw "scheduled task did not enter Running state"
}

function Invoke-Rollback([string]$OriginalErrorCode) {
  try {
    Copy-Item -Force -Path $BackupExe -Destination $OldExe -ErrorAction Stop
  } catch {
    Write-UpgradeOutcome -Outcome "failed" -ErrorCode "rollback_restore_failed"
    exit 1
  }
  try {
    Start-AgentScheduledTask
  } catch {
    Write-UpgradeOutcome -Outcome "failed" -ErrorCode "rollback_restart_failed"
    exit 1
  }
  Write-UpgradeOutcome -Outcome "rolled_back" -ErrorCode $OriginalErrorCode
  exit 1
}

if (-not (Wait-ForProcessExit -ProcessId $Pid)) {
  Write-UpgradeOutcome -Outcome "failed" -ErrorCode "process_stop_failed"
  exit 1
}

try {
  Copy-Item -Force -Path $OldExe -Destination $BackupExe -ErrorAction Stop
} catch {
  Write-UpgradeOutcome -Outcome "failed" -ErrorCode "backup_failed"
  exit 1
}

try {
  Move-Item -Force -Path $NewExe -Destination $OldExe -ErrorAction Stop
} catch {
  Invoke-Rollback -OriginalErrorCode "replace_failed"
}

try {
  Start-AgentScheduledTask
} catch {
  Invoke-Rollback -OriginalErrorCode "restart_failed"
}

exit 0
`) + "\r\n"
}
