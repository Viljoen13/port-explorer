//go:build darwin

package ports

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func list() ([]PortInfo, error) {
	cmd := exec.Command("lsof", "-iTCP", "-iUDP", "-P", "-n", "-sTCP:LISTEN,ESTABLISHED,CLOSE_WAIT,TIME_WAIT")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running lsof: %w", err)
	}

	var results []PortInfo
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Scan() // skip header

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}

		process := fields[0]
		pid, _ := strconv.Atoi(fields[1])
		proto := strings.ToUpper(fields[7])
		if strings.HasPrefix(proto, "TCP") {
			proto = "TCP"
		} else if strings.HasPrefix(proto, "UDP") {
			proto = "UDP"
		}

		nameField := fields[8]
		state := ""
		if len(fields) >= 10 {
			state = strings.Trim(fields[9], "()")
		}

		// Parse address:port from the name field
		lastColon := strings.LastIndex(nameField, ":")
		if lastColon == -1 {
			continue
		}
		addr := nameField[:lastColon]
		port, err := strconv.ParseUint(nameField[lastColon+1:], 10, 16)
		if err != nil {
			continue
		}

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
