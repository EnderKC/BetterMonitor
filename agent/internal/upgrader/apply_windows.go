//go:build windows

package upgrader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func applyAndRestart(_ context.Context, req UpgradeRequest, exePath, newBinaryPath string, report ProgressFunc) error {
	if report == nil {
		report = func(Progress) {}
	}

	report(Progress{
		RequestID:     req.RequestID,
		Status:        "restarting",
		Message:       "Windows 采用外部 updater 完成替换并重启",
		TargetVersion: req.TargetVersion,
		DownloadURL:   req.DownloadURL,
		SHA256:        req.SHA256,
		Time:          time.Now().UTC(),
	})

	args := req.Args
	if len(args) > 0 {
		args = args[1:] // 去掉 argv[0]
	} else {
		args = []string{}
	}
	argsJSON, _ := json.Marshal(args)

	dir := filepath.Dir(exePath)
	scriptPath := filepath.Join(dir, fmt.Sprintf("bm-agent-upgrade-%d.ps1", time.Now().UnixNano()))
	script := buildPowerShellUpdaterScript()
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return fmt.Errorf("write updater script: %w", err)
	}

	pid := os.Getpid()
	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		scriptPath,
		"-Pid",
		strconv.Itoa(pid),
		"-OldExe",
		exePath,
		"-NewExe",
		newBinaryPath,
		"-ArgsJson",
		string(argsJSON),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("start updater script: %w", err)
	}

	// 当前进程退出，由 updater 负责替换/启动新进程
	os.Exit(0)
	return nil
}

func buildPowerShellUpdaterScript() string {
	// 说明：
	// - 等待旧进程退出（避免文件被锁）
	// - Move-Item 将旧 exe 备份为 .old
	// - Move-Item 将 new exe 覆盖到旧路径
	// - Start-Process 以相同参数启动新版本
	// - 尝试清理 new 文件与脚本自身
	return strings.TrimSpace(`
param(
  [Parameter(Mandatory=$true)][int]$Pid,
  [Parameter(Mandatory=$true)][string]$OldExe,
  [Parameter(Mandatory=$true)][string]$NewExe,
  [Parameter(Mandatory=$false)][string]$ArgsJson
)

function Try-Remove([string]$Path) {
  try { if (Test-Path $Path) { Remove-Item -Force -ErrorAction SilentlyContinue $Path } } catch {}
}

function Try-Move([string]$From, [string]$To) {
  try { Move-Item -Force -ErrorAction Stop $From $To; return $true } catch { return $false }
}

$args = @()
try {
  if ($ArgsJson -and $ArgsJson.Trim().Length -gt 0) {
    $args = ConvertFrom-Json -InputObject $ArgsJson
  }
} catch {
  $args = @()
}

# wait for old process to exit (max ~120s)
for ($i = 0; $i -lt 120; $i++) {
  try {
    $p = Get-Process -Id $Pid -ErrorAction Stop
    Start-Sleep -Seconds 1
  } catch {
    break
  }
}

$backup = "$OldExe.old"
Try-Remove $backup

# replace: OldExe -> backup, NewExe -> OldExe (retry a few times)
for ($i = 0; $i -lt 30; $i++) {
  try {
    if (Test-Path $OldExe) { Try-Move $OldExe $backup | Out-Null }
    if (Try-Move $NewExe $OldExe) { break }
  } catch {}
  Start-Sleep -Milliseconds 500
}

try {
  Start-Process -FilePath $OldExe -ArgumentList $args -WindowStyle Hidden
} catch {
  # best-effort rollback
  try {
    if (Test-Path $backup) { Try-Move $backup $OldExe | Out-Null }
  } catch {}
}

Try-Remove $NewExe
Try-Remove $MyInvocation.MyCommand.Path
`) + "\r\n"
}
