//go:build !windows

package ports

import (
	"fmt"
	"syscall"
)

// Kill asks a process to exit. With force it is killed immediately (SIGKILL);
// otherwise it receives SIGTERM and may shut down cleanly.
func Kill(pid int, force bool) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	if err := syscall.Kill(pid, sig); err != nil {
		if err == syscall.EPERM {
			return fmt.Errorf("permission denied killing PID %d (try sudo)", pid)
		}
		return fmt.Errorf("killing PID %d: %w", pid, err)
	}
	return nil
}

// SignalName describes the signal Kill sends.
func SignalName(force bool) string {
	if force {
		return "SIGKILL"
	}
	return "SIGTERM"
}
