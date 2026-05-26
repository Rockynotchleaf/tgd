// Package ipc handles inter-process communication between tgd and tgd-hook.
package ipc

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// WritePID writes the current process PID to pidPath.
func WritePID(pidPath string) error {
	return os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0600)
}

// ReadPID reads the PID stored in pidPath. Returns 0 if the file doesn't exist.
func ReadPID(pidPath string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID file: %w", err)
	}
	return pid, nil
}

// IsAlive returns true if the process whose PID is in pidPath is still running
// and the socket at sockPath is accessible.
func IsAlive(pidPath, sockPath string) bool {
	pid, err := ReadPID(pidPath)
	if err != nil || pid == 0 {
		return false
	}
	// Try sending signal 0 — confirms process exists without signaling it
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	// Check socket file exists
	_, err = os.Stat(sockPath)
	return err == nil
}
