package ports

import (
	"fmt"
	"net"
	"strconv"
)

// IsFree reports whether a TCP port can be bound on all interfaces right now.
func IsFree(port uint16) bool {
	l, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(int(port))))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// FindFree returns the first count free TCP ports at or above start, skipping
// anything that is currently bound in any state.
func FindFree(start uint16, count int, inUse map[uint16]bool) ([]uint16, error) {
	var out []uint16
	for p := uint32(start); p <= 65535 && len(out) < count; p++ {
		port := uint16(p)
		if inUse[port] || !IsFree(port) {
			continue
		}
		out = append(out, port)
	}
	if len(out) < count {
		return out, fmt.Errorf("only %d free port(s) found from %d upwards", len(out), start)
	}
	return out, nil
}
