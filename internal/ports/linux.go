//go:build linux

package ports

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
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
	inodes, err := buildInodeMap()
	if err != nil {
		return nil, fmt.Errorf("scanning /proc: %w", err)
	}

	details := map[int]*procDetails{}
	users := map[string]string{}

	var results []PortInfo
	for _, src := range []struct{ path, proto string }{
		{"/proc/net/tcp", "TCP"}, {"/proc/net/tcp6", "TCP"},
		{"/proc/net/udp", "UDP"}, {"/proc/net/udp6", "UDP"},
	} {
		entries, err := parseProcNet(src.path, src.proto, inodes, details, users)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		results = append(results, entries...)
	}

	resolveContainerNames(results)
	return results, nil
}

type socketOwner struct {
	pid  int
	comm string
}

// buildInodeMap maps socket inode numbers to the process holding them.
func buildInodeMap() (map[string]socketOwner, error) {
	inodes := make(map[string]socketOwner)
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil || !p.IsDir() {
			continue
		}
		fdDir := filepath.Join("/proc", p.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // not our process and not root: expected
		}
		var comm string
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			if comm == "" {
				comm = readTrimmed(pid, "comm")
			}
			inodes[link[8:len(link)-1]] = socketOwner{pid: pid, comm: comm}
		}
	}
	return inodes, nil
}

func parseProcNet(path, proto string, inodes map[string]socketOwner, details map[int]*procDetails, users map[string]string) ([]PortInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var results []PortInfo
	sc := bufio.NewScanner(f)
	sc.Scan() // header
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 {
			continue
		}
		localAddr, localPort, err := parseHexAddr(fields[1])
		if err != nil {
			continue
		}
		remoteAddr, remotePort, _ := parseHexAddr(fields[2])

		state := "UNCONN"
		if proto == "TCP" {
			if s, ok := tcpStates[strings.ToUpper(fields[3])]; ok {
				state = s
			}
		} else if remotePort != 0 {
			state = "ESTABLISHED" // connected UDP socket
		}

		e := PortInfo{
			Protocol:      proto,
			Port:          localPort,
			Address:       localAddr,
			RemoteAddress: remoteAddr,
			RemotePort:    remotePort,
			State:         state,
			User:          lookupUser(fields[7], users),
			IPv6:          strings.HasSuffix(path, "6"),
			IPv4:          !strings.HasSuffix(path, "6"),
		}

		if owner, ok := inodes[fields[9]]; ok {
			e.PID = owner.pid
			e.Process = owner.comm
			d, cached := details[owner.pid]
			if !cached {
				d = loadProcDetails(owner.pid)
				details[owner.pid] = d
			}
			d.apply(&e)
		}
		results = append(results, e)
	}
	return results, sc.Err()
}

func lookupUser(uidStr string, cache map[string]string) string {
	if name, ok := cache[uidStr]; ok {
		return name
	}
	name := uidStr
	if u, err := user.LookupId(uidStr); err == nil {
		name = u.Username
	}
	cache[uidStr] = name
	return name
}

// procDetails holds per-process facts that are expensive to read.
type procDetails struct {
	cmdline   string
	exe       string
	cwd       string
	ppid      int
	startedAt *time.Time
	container string
}

func (d *procDetails) apply(e *PortInfo) {
	if d == nil {
		return
	}
	e.Cmdline = d.cmdline
	e.Exe = d.exe
	e.Cwd = d.cwd
	e.PPID = d.ppid
	e.StartedAt = d.startedAt
	e.Container = d.container
}

func loadProcDetails(pid int) *procDetails {
	d := &procDetails{}
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
		d.cmdline = strings.TrimSpace(strings.ReplaceAll(strings.TrimRight(string(data), "\x00"), "\x00", " "))
	}
	if link, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		d.exe = strings.TrimSuffix(link, " (deleted)")
	}
	if link, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid)); err == nil {
		d.cwd = link
	}
	d.ppid, d.startedAt = parseProcStat(pid)
	d.container = containerIDFromCgroup(pid)
	return d
}

// parseProcStat extracts the parent PID and start time from /proc/<pid>/stat.
func parseProcStat(pid int) (int, *time.Time) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, nil
	}
	// comm can contain spaces and parentheses; everything after the last ')' is safe.
	idx := strings.LastIndexByte(string(data), ')')
	if idx < 0 {
		return 0, nil
	}
	fields := strings.Fields(string(data[idx+1:]))
	// After ')' fields are: state(0) ppid(1) ... starttime(19)
	if len(fields) < 20 {
		return 0, nil
	}
	ppid, _ := strconv.Atoi(fields[1])
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return ppid, nil
	}
	boot := bootTime()
	if boot.IsZero() {
		return ppid, nil
	}
	started := boot.Add(time.Duration(ticks) * time.Second / clockTicks)
	return ppid, &started
}

// clockTicks is CLK_TCK; every mainstream Linux build uses 100 and sysconf is
// unavailable without cgo.
const clockTicks = 100

var (
	bootOnce sync.Once
	bootAt   time.Time
)

func bootTime() time.Time {
	bootOnce.Do(func() {
		f, err := os.Open("/proc/stat")
		if err != nil {
			return
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if fields := strings.Fields(sc.Text()); len(fields) == 2 && fields[0] == "btime" {
				if secs, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					bootAt = time.Unix(secs, 0)
				}
				return
			}
		}
	})
	return bootAt
}

func readTrimmed(pid int, name string) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/%s", pid, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func parseHexAddr(s string) (string, uint16, error) {
	addrHex, portHex, ok := strings.Cut(s, ":")
	if !ok {
		return "", 0, fmt.Errorf("invalid address: %s", s)
	}
	port, err := strconv.ParseUint(portHex, 16, 16)
	if err != nil {
		return "", 0, err
	}
	return parseIP(addrHex), uint16(port), nil
}

// parseIP decodes the kernel's hex address encoding: each 32-bit word is
// stored in host (little-endian) byte order.
func parseIP(hexStr string) string {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return hexStr
	}
	switch len(b) {
	case 4:
		return fmt.Sprintf("%d.%d.%d.%d", b[3], b[2], b[1], b[0])
	case 16:
		for i := 0; i < 16; i += 4 {
			b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
		}
		// IPv4-mapped IPv6 (::ffff:a.b.c.d) is really an IPv4 peer.
		if isV4Mapped(b) {
			return fmt.Sprintf("%d.%d.%d.%d", b[12], b[13], b[14], b[15])
		}
		return net.IP(b).String()
	}
	return hexStr
}

func isV4Mapped(b []byte) bool {
	for i := 0; i < 10; i++ {
		if b[i] != 0 {
			return false
		}
	}
	return b[10] == 0xff && b[11] == 0xff
}

// IsPrivileged reports whether we can see every process's sockets.
func IsPrivileged() bool { return os.Geteuid() == 0 }
