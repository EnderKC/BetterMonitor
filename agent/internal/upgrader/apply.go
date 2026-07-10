package upgrader

import (
	"context"
	"errors"
	"strings"
	"time"
)

type ApplyOps interface {
	Backup(src, dst string) error
	Replace(src, dst string) error
	Restore(backup, dst string) error
	Exec(path string, args, env []string) error
}

func applyAndRollback(
	ctx context.Context,
	req UpgradeRequest,
	exePath, newBinaryPath string,
	ops ApplyOps,
	report ProgressFunc,
) error {
	if ops == nil {
		return newUpgradeError("apply_ops_missing")
	}
	if err := ctx.Err(); err != nil {
		return newUpgradeError("upgrade_cancelled")
	}
	if err := persistUpgradeOutcome(req, "applied", ""); err != nil {
		return err
	}

	backupPath := exePath + ".old"
	if err := ops.Backup(exePath, backupPath); err != nil {
		return failWithUpgradeOutcome(req, "failed", "backup_failed")
	}
	if err := ops.Replace(newBinaryPath, exePath); err != nil {
		if restoreErr := ops.Restore(backupPath, exePath); restoreErr != nil {
			return failWithUpgradeOutcome(req, "failed", "rollback_restore_failed")
		}
		return failWithUpgradeOutcome(req, "rolled_back", "replace_failed")
	}

	if report != nil {
		report(Progress{
			RequestID:     req.RequestID,
			Status:        "restarting",
			Message:       "restarting agent",
			TargetVersion: req.TargetVersion,
			Time:          time.Now().UTC(),
		})
	}
	args := req.Args
	if len(args) == 0 {
		args = []string{exePath}
	}
	if err := ops.Exec(exePath, args, req.Env); err == nil {
		return nil
	}
	if err := ops.Restore(backupPath, exePath); err != nil {
		return failWithUpgradeOutcome(req, "failed", "rollback_restore_failed")
	}
	if err := ops.Exec(exePath, args, req.Env); err != nil {
		return failWithUpgradeOutcome(req, "failed", "rollback_restart_failed")
	}
	return failWithUpgradeOutcome(req, "rolled_back", "upgrade_rolled_back")
}

func persistUpgradeOutcome(req UpgradeRequest, outcome, errorCode string) error {
	if strings.TrimSpace(req.MarkerPath) == "" {
		return newUpgradeError("upgrade_marker_path_missing")
	}
	marker := UpgradeMarker{
		RequestID:       strings.TrimSpace(req.RequestID),
		TargetVersion:   strings.TrimSpace(req.TargetVersion),
		TargetAgentType: strings.ToLower(strings.TrimSpace(req.TargetAgentType)),
		Outcome:         outcome,
		ErrorCode:       errorCode,
	}
	if err := WriteUpgradeMarker(req.MarkerPath, marker); err != nil {
		return newUpgradeError("upgrade_marker_write_failed")
	}
	return nil
}

func failWithUpgradeOutcome(req UpgradeRequest, outcome, errorCode string) error {
	if err := persistUpgradeOutcome(req, outcome, errorCode); err != nil {
		return err
	}
	return newUpgradeError(errorCode)
}

type UpgradeError struct {
	Code string
}

func (err *UpgradeError) Error() string {
	if err == nil {
		return ""
	}
	return err.Code
}

func UpgradeErrorCode(err error) string {
	var upgradeErr *UpgradeError
	if errors.As(err, &upgradeErr) {
		return upgradeErr.Code
	}
	return ""
}

func newUpgradeError(code string) error {
	return &UpgradeError{Code: code}
}
