//go:build darwin

package ports

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// list uses lsof's machine-readable -F output, which is immune to the column
// drift that breaks naive parsing when command names contain spaces.
func list() ([]PortInfo, error) {
	cmd := exec.Command("lsof", "-nP", "-w", "-iTCP", "-iUDP", "-FpcuLPnT")
	out, err := cmd.Output()
	if err != nil {
		// lsof exits 1 when it finds nothing at all; that's not an error for us.
		if len(out) == 0 {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				return nil, nil
			}
			return nil, fmt.Errorf("running lsof: %w", err)
		}
	}

	var (
		results []PortInfo
		pid     int
		comm    string
		user    string
		cur     *PortInfo
		details = map[int]*procDetails{}
	)
	flush := func() {
		if cur != nil && cur.Protocol != "" {
			results = append(results, *cur)
		}
		cur = nil
	}

	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		code, val := line[0], line[1:]
		switch code {
		case 'p':
			flush()
			pid, _ = strconv.Atoi(val)
		case 'c':
			comm = val
		case 'L':
			user = val
		case 'u':
			if user == "" {
				user = val
			}
		case 'f':
			flush()
			cur = &PortInfo{PID: pid, Process: comm, User: user}
			d, ok := details[pid]
			if !ok {
				d = loadProcDetails(pid)
				details[pid] = d
			}
			d.apply(cur)
		case 'P':
			if cur != nil {
				cur.Protocol = strings.ToUpper(val)
			}
		case 'n':
			if cur != nil {
				parseLsofName(cur, val)
			}
		case 'T':
			if cur != nil && strings.HasPrefix(val, "ST=") {
				cur.State = strings.TrimPrefix(val, "ST=")
			}
		}
	}
	flush()

	for i := range results {
		e := &results[i]
		if e.Protocol == "UDP" && e.State == "" {
			if e.RemotePort != 0 {
				e.State = "ESTABLISHED"
			} else {
				e.State = "UNCONN"
			}
		}
		if e.State == "" {
			e.State = "UNKNOWN"
		}
	}
	return results, nil
}

// parseLsofName handles "*:8080", "127.0.0.1:5432", "[::1]:3000" and
// "192.168.1.5:52000->1.2.3.4:443".
func parseLsofName(e *PortInfo, name string) {
	local, remote, hasRemote := strings.Cut(name, "->")
	addr, port := splitHostPort(local)
	e.Address = addr
	e.Port = port
	if hasRemote {
		e.RemoteAddress, e.RemotePort = splitHostPort(remote)
	}
	e.IPv6 = strings.Contains(addr, ":")
	e.IPv4 = !e.IPv6
}

func splitHostPort(s string) (string, uint16) {
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return s, 0
	}
	host := strings.Trim(s[:i], "[]")
	if host == "*" {
		host = "0.0.0.0"
	}
	p, _ := strconv.ParseUint(s[i+1:], 10, 16)
	return host, uint16(p)
}

type procDetails struct {
	cmdline   string
	exe       string
	cwd       string
	ppid      int
	startedAt *time.Time
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
}

func loadProcDetails(pid int) *procDetails {
	d := &procDetails{}
	if pid <= 0 {
		return d
	}
	// ps prints: PPID  LSTART(5 words)  COMM  ARGS...
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "ppid=,lstart=,comm=,args=").Output()
	if err == nil {
		fields := strings.Fields(string(out))
		if len(fields) >= 7 {
			d.ppid, _ = strconv.Atoi(fields[0])
			if t, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(fields[1:6], " "), time.Local); err == nil {
				d.startedAt = &t
			}
			d.exe = fields[6]
			d.cmdline = strings.Join(fields[7:], " ")
		}
	}
	if out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "n") {
				d.cwd = line[1:]
			}
		}
	}
	return d
}

// IsPrivileged reports whether we can see every process's sockets.
func IsPrivileged() bool { return os.Geteuid() == 0 }
