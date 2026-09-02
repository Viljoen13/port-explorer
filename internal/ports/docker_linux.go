//go:build linux

package ports

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// cgroupIDPattern matches a 64-hex container ID in a cgroup path such as
// /docker/<id>, /system.slice/docker-<id>.scope, /kubepods/.../<id> or
// /machine.slice/libpod-<id>.scope.
var cgroupIDPattern = regexp.MustCompile(`(?:docker[-/]|libpod-|cri-containerd-|containerd[-/]|/)([0-9a-f]{64})`)

// containerIDFromCgroup returns the full container ID a process runs in, or "".
func containerIDFromCgroup(pid int) string {
	f, err := os.Open(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := cgroupIDPattern.FindStringSubmatch(sc.Text()); m != nil {
			return m[1]
		}
	}
	return ""
}

// resolveContainerNames replaces full container IDs with names when a container
// runtime CLI is available, falling back to the short 12-character ID.
func resolveContainerNames(entries []PortInfo) {
	ids := map[string]bool{}
	for _, e := range entries {
		if len(e.Container) == 64 {
			ids[e.Container] = true
		}
	}
	if len(ids) == 0 {
		return
	}
	names := containerNames()
	for i := range entries {
		id := entries[i].Container
		if len(id) != 64 {
			continue
		}
		if name, ok := names[id]; ok {
			entries[i].Container = name
		} else {
			entries[i].Container = id[:12]
		}
	}
}

// containerNames asks docker (then podman) for running containers. It is
// best-effort and time-boxed so a hung daemon can never stall the UI.
func containerNames() map[string]string {
	names := map[string]string{}
	for _, tool := range []string{"docker", "podman"} {
		if _, err := exec.LookPath(tool); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		out, err := exec.CommandContext(ctx, tool, "ps", "--no-trunc", "--format", "{{.ID}}\t{{.Names}}").Output()
		cancel()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			id, name, ok := strings.Cut(strings.TrimSpace(line), "\t")
			if ok && len(id) == 64 {
				names[id] = strings.Split(name, ",")[0]
			}
		}
		if len(names) > 0 {
			return names
		}
	}
	return names
}
