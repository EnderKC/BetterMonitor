//go:build windows

package server

import "fmt"

func execSelf(path string, args []string) error {
	return fmt.Errorf("exec not supported on windows")
}

