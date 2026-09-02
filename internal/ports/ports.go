// Package ports discovers open network ports and the processes behind them.
//
// It provides a single cross-platform data model (PortInfo) that is populated by
// platform-specific collectors (Linux /proc, macOS lsof, Windows netstat), then
// enriched with service names, exposure analysis and container detection.
package ports

import (
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PortInfo describes one socket on the local machine.
type PortInfo struct {
	Protocol string `json:"protocol"` // "TCP" or "UDP"
	Port     uint16 `json:"port"`
	Address  string `json:"address"` // local bind address

	RemoteAddress string `json:"remote_address,omitempty"`
	RemotePort    uint16 `json:"remote_port,omitempty"`

	State string `json:"state"` // LISTEN, ESTABLISHED, UNCONN, ...

	PID     int    `json:"pid"`
	Process string `json:"process"`
	User    string `json:"user,omitempty"`

	Service string `json:"service,omitempty"` // well-known service name (http, postgres, ...)

	Cmdline   string     `json:"cmdline,omitempty"`
	Exe       string     `json:"exe,omitempty"`
	Cwd       string     `json:"cwd,omitempty"`
	PPID      int        `json:"ppid,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`

	Container string `json:"container,omitempty"` // container name or short ID
	Forward   string `json:"forward,omitempty"`   // docker-proxy target, e.g. 172.17.0.2:80

	Exposed bool `json:"exposed"` // listening on a non-loopback interface
	IPv4    bool `json:"ipv4"`
	IPv6    bool `json:"ipv6"`
}

// List returns every socket on the system, enriched and ready for display.
// Entries are not deduplicated; call Merge to collapse IPv4/IPv6 twins.
func List() ([]PortInfo, error) {
	entries, err := list()
	if err != nil {
		return nil, err
	}
	for i := range entries {
		finalize(&entries[i])
	}
	return entries, nil
}

// finalize derives computed fields that do not depend on the platform.
func finalize(e *PortInfo) {
	if e.Service == "" {
		e.Service = ServiceName(e.Port, e.Protocol)
	}
	if e.Forward == "" && e.Cmdline != "" {
		if target, ok := ParseDockerProxy(e.Cmdline); ok {
			e.Forward = target
		}
	}
	if !e.IPv4 && !e.IPv6 {
		if strings.Contains(e.Address, ":") {
			e.IPv6 = true
		} else {
			e.IPv4 = true
		}
	}
	if e.RemotePort == 0 && IsWildcard(e.RemoteAddress) {
		e.RemoteAddress = ""
	}
	e.Exposed = e.IsListening() && IsReachable(e.Address)
}

// IsReachable reports whether a bind address makes a listener reachable from
// other machines: the wildcard, or a routable unicast address. Loopback,
// link-local and multicast binds do not count.
func IsReachable(addr string) bool {
	if IsWildcard(addr) {
		return true
	}
	ip := net.ParseIP(strings.Trim(addr, "[]"))
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsMulticast() && !ip.IsLinkLocalUnicast() && !ip.IsInterfaceLocalMulticast() && !ip.IsUnspecified()
}

// IsListening reports whether the socket accepts traffic: a TCP LISTEN socket
// or a bound, unconnected UDP socket.
func (e *PortInfo) IsListening() bool {
	switch e.Protocol {
	case "TCP":
		return e.State == "LISTEN"
	case "UDP":
		return e.State == "UNCONN"
	}
	return false
}

// IsWildcard reports whether the address binds every interface.
func IsWildcard(addr string) bool {
	switch addr {
	case "", "*", "0.0.0.0", "::", "[::]":
		return true
	}
	return false
}

// IsLoopback reports whether the address is a loopback address.
func IsLoopback(addr string) bool {
	if addr == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(addr, "[]"))
	return ip != nil && ip.IsLoopback()
}

// BindLabel renders the local address in a compact human form.
func (e *PortInfo) BindLabel() string {
	switch {
	case IsWildcard(e.Address):
		return "*"
	case IsLoopback(e.Address):
		return "localhost"
	default:
		return e.Address
	}
}

// Remote renders the remote endpoint, or "" when there is none.
func (e *PortInfo) Remote() string {
	if e.RemotePort == 0 && (e.RemoteAddress == "" || IsWildcard(e.RemoteAddress)) {
		return ""
	}
	return net.JoinHostPort(e.RemoteAddress, strconv.Itoa(int(e.RemotePort)))
}

// Uptime returns how long the owning process has been running.
func (e *PortInfo) Uptime() time.Duration {
	if e.StartedAt == nil {
		return 0
	}
	return time.Since(*e.StartedAt).Truncate(time.Second)
}

// Key uniquely identifies a socket across refreshes.
func (e *PortInfo) Key() string {
	return e.Protocol + "|" + strconv.Itoa(int(e.Port)) + "|" + strconv.Itoa(e.PID) + "|" +
		e.State + "|" + e.Address + "|" + e.Remote()
}

// mergeKey groups IPv4/IPv6 twins of the same logical socket.
func (e *PortInfo) mergeKey() string {
	addr := e.Address
	switch {
	case IsWildcard(addr):
		addr = "*"
	case IsLoopback(addr):
		addr = "lo"
	}
	return e.Protocol + "|" + strconv.Itoa(int(e.Port)) + "|" + strconv.Itoa(e.PID) + "|" +
		e.State + "|" + addr + "|" + e.Remote()
}

// Merge collapses entries that describe the same logical socket over both IPv4
// and IPv6 (very common for servers bound to * or localhost) into one entry
// flagged with both families. Order is preserved.
func Merge(entries []PortInfo) []PortInfo {
	index := make(map[string]int, len(entries))
	out := make([]PortInfo, 0, len(entries))
	for _, e := range entries {
		k := e.mergeKey()
		if i, ok := index[k]; ok {
			out[i].IPv4 = out[i].IPv4 || e.IPv4
			out[i].IPv6 = out[i].IPv6 || e.IPv6
			// Prefer showing the IPv4 address when both exist.
			if strings.Contains(out[i].Address, ":") && !strings.Contains(e.Address, ":") {
				out[i].Address = e.Address
			}
			continue
		}
		index[k] = len(out)
		out = append(out, e)
	}
	return out
}

// Listening filters to sockets that accept traffic.
func Listening(entries []PortInfo) []PortInfo {
	out := make([]PortInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsListening() {
			out = append(out, e)
		}
	}
	return out
}

// Hidden counts sockets whose owning process could not be determined,
// usually because they belong to another user and we lack privileges.
func Hidden(entries []PortInfo) int {
	n := 0
	for _, e := range entries {
		if e.PID == 0 {
			n++
		}
	}
	return n
}

// Stats summarises a set of entries.
type Stats struct {
	Total       int
	Listening   int
	Established int
	Exposed     int
	Hidden      int
	Processes   int
	Containers  int
}

// Summarize computes Stats for a set of entries.
func Summarize(entries []PortInfo) Stats {
	var s Stats
	pids := map[int]bool{}
	containers := map[string]bool{}
	s.Total = len(entries)
	for _, e := range entries {
		if e.IsListening() {
			s.Listening++
		}
		if e.State == "ESTABLISHED" {
			s.Established++
		}
		if e.Exposed {
			s.Exposed++
		}
		if e.PID == 0 {
			s.Hidden++
		} else {
			pids[e.PID] = true
		}
		if e.Container != "" {
			containers[e.Container] = true
		}
	}
	s.Processes = len(pids)
	s.Containers = len(containers)
	return s
}

// SortField names a sortable column.
type SortField string

const (
	SortPort    SortField = "port"
	SortProcess SortField = "process"
	SortPID     SortField = "pid"
	SortState   SortField = "state"
	SortUser    SortField = "user"
)

// SortFields lists the supported sort fields in cycle order.
var SortFields = []SortField{SortPort, SortProcess, SortPID, SortState, SortUser}

// Sort orders entries by the given field. Ties fall back to port, then protocol.
func Sort(entries []PortInfo, field SortField, ascending bool) {
	less := func(a, b *PortInfo) bool {
		switch field {
		case SortProcess:
			pa, pb := strings.ToLower(a.Process), strings.ToLower(b.Process)
			if pa != pb {
				// Unknown processes sink to the bottom regardless of direction.
				if pa == "" || pb == "" {
					return pb == "" == ascending
				}
				return pa < pb
			}
		case SortPID:
			if a.PID != b.PID {
				return a.PID < b.PID
			}
		case SortState:
			if a.State != b.State {
				return stateRank(a.State) < stateRank(b.State)
			}
		case SortUser:
			if a.User != b.User {
				return a.User < b.User
			}
		}
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		return a.Remote() < b.Remote()
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if ascending {
			return less(&entries[i], &entries[j])
		}
		return less(&entries[j], &entries[i])
	})
}

func stateRank(s string) int {
	switch s {
	case "LISTEN":
		return 0
	case "UNCONN":
		return 1
	case "ESTABLISHED":
		return 2
	case "SYN_SENT", "SYN_RECV":
		return 3
	case "CLOSE_WAIT", "TIME_WAIT", "FIN_WAIT1", "FIN_WAIT2", "CLOSING", "LAST_ACK":
		return 4
	}
	return 9
}

// FormatDuration renders a duration compactly: 3d4h, 2h15m, 45s.
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	switch {
	case days > 0:
		return strconv.Itoa(days) + "d" + strconv.Itoa(hours) + "h"
	case hours > 0:
		return strconv.Itoa(hours) + "h" + strconv.Itoa(mins) + "m"
	case mins > 0:
		return strconv.Itoa(mins) + "m" + strconv.Itoa(secs) + "s"
	default:
		return strconv.Itoa(secs) + "s"
	}
}
