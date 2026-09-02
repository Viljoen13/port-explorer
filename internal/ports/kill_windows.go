//go:build windows

package ports

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Kill asks a process to exit via taskkill. Without force Windows sends a
// close request the application may honour; with force it is terminated.
func Kill(pid int, force bool) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	args := []string{"/PID", strconv.Itoa(pid)}
	if force {
		args = append(args, "/F")
	}
	out, err := exec.Command("taskkill", args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("killing PID %d: %s", pid, msg)
	}
	return nil
}

// SignalName describes the action Kill takes.
func SignalName(force bool) string {
	if force {
		return "terminate"
	}
	return "close request"
}
