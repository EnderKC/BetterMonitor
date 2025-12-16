//go:build !windows

package server

import (
	"os"
	"syscall"
)

func execSelf(path string, args []string) error {
	return syscall.Exec(path, args, os.Environ())
}

