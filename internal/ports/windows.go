//go:build windows

package ports

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func list() ([]PortInfo, error) {
	cmd := exec.Command("netstat", "-ano")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running netstat: %w", err)
	}

	pidNames := getPidNames()

	var results []PortInfo
	scanner := bufio.NewScanner(strings.NewReader(string(out)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		proto := strings.ToUpper(fields[0])
		if proto != "TCP" && proto != "UDP" {
			continue
		}

		localAddr := fields[1]
		lastColon := strings.LastIndex(localAddr, ":")
		if lastColon == -1 {
			continue
		}
		addr := localAddr[:lastColon]
		port, err := strconv.ParseUint(localAddr[lastColon+1:], 10, 16)
		if err != nil {
			continue
		}

		state := ""
		pidStr := ""
		if proto == "TCP" && len(fields) >= 5 {
			state = fields[3]
			pidStr = fields[4]
		} else if proto == "UDP" && len(fields) >= 4 {
			state = "UNCONN"
			pidStr = fields[3]
		}

		pid, _ := strconv.Atoi(pidStr)
		process := pidNames[pid]

		results = append(results, PortInfo{
			Protocol: proto,
			Port:     uint16(port),
			PID:      pid,
			Process:  process,
			State:    state,
			Address:  addr,
		})
	}

	return results, nil
}

func getPidNames() map[int]string {
	names := make(map[int]string)
	cmd := exec.Command("tasklist", "/fo", "csv", "/nh")
	out, err := cmd.Output()
	if err != nil {
		return names
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ",", 3)
		if len(parts) < 2 {
			continue
		}
		name := strings.Trim(parts[0], "\"")
		pid, err := strconv.Atoi(strings.Trim(parts[1], "\""))
		if err != nil {
			continue
		}
		names[pid] = name
	}
	return names
}
