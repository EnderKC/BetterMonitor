//go:build !windows

package upgrader

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func applyAndRestart(_ context.Context, req UpgradeRequest, exePath, newBinaryPath string, report ProgressFunc) error {
	if report == nil {
		report = func(Progress) {}
	}

	// 备份旧二进制（best-effort，不影响主流程）
	backupPath := exePath + ".old"
	_ = os.Remove(backupPath)
	_ = tryHardlinkOrCopy(exePath, backupPath)

	// 原子替换：同目录 rename 覆盖旧文件（Unix 下是原子操作）
	if err := os.Rename(newBinaryPath, exePath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	report(Progress{
		RequestID:     req.RequestID,
		Status:        "restarting",
		Message:       "重启 Agent 进程",
		TargetVersion: req.TargetVersion,
		DownloadURL:   req.DownloadURL,
		SHA256:        req.SHA256,
		Time:          time.Now().UTC(),
	})

	argv := req.Args
	if len(argv) == 0 {
		argv = []string{filepath.Base(exePath)}
	}
	env := req.Env
	if env == nil {
		env = os.Environ()
	}

	// 使用 syscall.Exec 替换当前进程
	return syscall.Exec(exePath, argv, env)
}

func tryHardlinkOrCopy(src, dst string) error {
	// 优先 hardlink，失败则 copy（两者都 best-effort）
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
