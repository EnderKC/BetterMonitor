//go:build windows

package upgrader

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func applyAndRestart(
	_ context.Context,
	req UpgradeRequest,
	exePath, newBinaryPath string,
	report ProgressFunc,
) error {
	if report != nil {
		report(Progress{
			RequestID:     req.RequestID,
			Status:        "restarting",
			Message:       "starting supervised Windows updater",
			TargetVersion: req.TargetVersion,
			Time:          time.Now().UTC(),
		})
	}

	args := req.Args
	if len(args) > 0 {
		args = args[1:]
	}
	argsJSON, _ := json.Marshal(args)
	directory := filepath.Dir(exePath)
	scriptPath := filepath.Join(directory, "bm-agent-upgrade-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".ps1")
	if err := os.WriteFile(scriptPath, []byte(BuildWindowsUpdaterScript()), 0o600); err != nil {
		return failWithUpgradeOutcome(req, "failed", "windows_updater_write_failed")
	}
	taskName := strings.TrimSpace(req.ScheduledTaskName)
	if taskName == "" {
		taskName = "BetterMonitorAgent"
	}
	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
		"-Pid", strconv.Itoa(os.Getpid()),
		"-OldExe", exePath,
		"-NewExe", newBinaryPath,
		"-BackupExe", exePath+".old",
		"-ScheduledTaskName", taskName,
		"-OutcomePath", req.MarkerPath,
		"-ArgsJson", string(argsJSON),
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return failWithUpgradeOutcome(req, "failed", "windows_updater_start_failed")
	}
	os.Exit(0)
	return nil
}
