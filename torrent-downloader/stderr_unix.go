//go:build linux || freebsd || darwin

package main

import (
	"os"
	"strings"
	"syscall"
)

// configOpenLogDup opens the internal-log target and dups it onto file
// descriptor 2, returning the fd number that fd 2 pointed at before (0 on
// failure). See main.configRedirect.
func configOpenLogDup(logPath string) int {
	var logFile *os.File
	var err error
	if strings.EqualFold(logPath, "none") {
		logFile, err = os.OpenFile(os.DevNull, os.O_WRONLY, 0644)
	} else {
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	}
	if err != nil {
		return 0 // keep fd 2 as-is; too bad if it's a TTY
	}
	defer func() { _ = logFile.Close() }()

	saved, err := syscall.Dup(2)
	if err != nil {
		return 0
	}
	if err := syscall.Dup3(int(logFile.Fd()), 2, 0); err != nil {
		_ = syscall.Close(saved)
		return 0
	}
	return saved
}
