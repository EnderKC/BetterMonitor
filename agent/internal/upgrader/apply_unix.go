//go:build !windows

package upgrader

import (
	"context"
	"io"
	"os"
	"syscall"
)

type unixApplyOps struct{}

func (unixApplyOps) Backup(src, dst string) error {
	_ = os.Remove(dst)
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func (unixApplyOps) Replace(src, dst string) error {
	return os.Rename(src, dst)
}

func (unixApplyOps) Restore(backup, dst string) error {
	return os.Rename(backup, dst)
}

func (unixApplyOps) Exec(path string, args, env []string) error {
	return syscall.Exec(path, args, env)
}

func applyAndRestart(
	ctx context.Context,
	req UpgradeRequest,
	exePath, newBinaryPath string,
	report ProgressFunc,
) error {
	ops := req.ApplyOps
	if ops == nil {
		ops = unixApplyOps{}
	}
	return applyAndRollback(ctx, req, exePath, newBinaryPath, ops, report)
}
