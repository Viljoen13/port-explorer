//go:build linux

package ports

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var tcpStates = map[string]string{
	"01": "ESTABLISHED",
	"02": "SYN_SENT",
	"03": "SYN_RECV",
	"04": "FIN_WAIT1",
	"05": "FIN_WAIT2",
	"06": "TIME_WAIT",
	"07": "CLOSE",
	"08": "CLOSE_WAIT",
	"09": "LAST_ACK",
	"0A": "LISTEN",
	"0B": "CLOSING",
}

func list() ([]PortInfo, error) {
	inodeMap, err := buildInodeMap()
	if err != nil {
		return nil, fmt.Errorf("building inode map: %w", err)
	}

	var results []PortInfo

	tcpEntries, err := parseProcNet("/proc/net/tcp", "TCP", inodeMap)
	if err != nil {
		return nil, err
	}
	results = append(results, tcpEntries...)

	tcp6Entries, err := parseProcNet("/proc/net/tcp6", "TCP", inodeMap)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	results = append(results, tcp6Entries...)

	udpEntries, err := parseProcNet("/proc/net/udp", "UDP", inodeMap)
	if err != nil {
		return nil, err
	}
	results = append(results, udpEntries...)

	udp6Entries, err := parseProcNet("/proc/net/udp6", "UDP", inodeMap)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	results = append(results, udp6Entries...)

	return results, nil
}

type procInfo struct {
	pid  int
	comm string
}

func buildInodeMap() (map[string]procInfo, error) {
	inodeMap := make(map[string]procInfo)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		comm := readComm(pid)
		fdPath := filepath.Join("/proc", entry.Name(), "fd")
		fdEntries, err := os.ReadDir(fdPath)
		if err != nil {
			continue // permission denied is common
		}

		for _, fd := range fdEntries {
			link, err := os.Readlink(filepath.Join(fdPath, fd.Name()))
			if err != nil {
				continue
			}
			if strings.HasPrefix(link, "socket:[") {
				inode := link[8 : len(link)-1]
				inodeMap[inode] = procInfo{pid: pid, comm: comm}
			}
		}
	}

	return inodeMap, nil
}

func readComm(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func parseProcNet(path string, proto string, inodeMap map[string]procInfo) ([]PortInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var results []PortInfo
	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header line

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		localAddr := fields[1]
		stateHex := fields[3]
		inode := fields[9]

		addr, port, err := parseHexAddr(localAddr)
		if err != nil {
			continue
		}

		state := "UNCONN"
		if proto == "TCP" {
			if s, ok := tcpStates[stateHex]; ok {
				state = s
			}
		}

		info := PortInfo{
			Protocol: proto,
			Port:     port,
			State:    state,
			Address:  addr,
		}

		if pi, ok := inodeMap[inode]; ok {
			info.PID = pi.pid
			info.Process = pi.comm
		}

		results = append(results, info)
	}

	return results, scanner.Err()
}

func parseHexAddr(s string) (string, uint16, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address: %s", s)
	}

	portNum, err := strconv.ParseUint(parts[1], 16, 16)
	if err != nil {
		return "", 0, err
	}

	addr := parseIP(parts[0])
	return addr, uint16(portNum), nil
}

func parseIP(hexStr string) string {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return hexStr
	}

	switch len(b) {
	case 4:
		// IPv4: stored in little-endian
		return fmt.Sprintf("%d.%d.%d.%d", b[3], b[2], b[1], b[0])
	case 16:
		// IPv6: stored as 4 groups of 4 bytes, each group little-endian
		for i := 0; i < 16; i += 4 {
			b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
		}
		return fmt.Sprintf("%x:%x:%x:%x:%x:%x:%x:%x",
			uint16(b[0])<<8|uint16(b[1]),
			uint16(b[2])<<8|uint16(b[3]),
			uint16(b[4])<<8|uint16(b[5]),
			uint16(b[6])<<8|uint16(b[7]),
			uint16(b[8])<<8|uint16(b[9]),
			uint16(b[10])<<8|uint16(b[11]),
			uint16(b[12])<<8|uint16(b[13]),
			uint16(b[14])<<8|uint16(b[15]),
		)
	default:
		return hexStr
	}
}
