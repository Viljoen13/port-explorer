//go:build windows

package ports

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func list() ([]PortInfo, error) {
	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return nil, fmt.Errorf("running netstat: %w", err)
	}

	names := taskNames()
	details := map[int]*procDetails{}

	var results []PortInfo
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		proto := strings.ToUpper(fields[0])
		if proto != "TCP" && proto != "UDP" {
			continue
		}
		addr, port := splitHostPort(fields[1])
		if port == 0 && addr == "" {
			continue
		}
		e := PortInfo{
			Protocol: proto,
			Port:     port,
			Address:  addr,
			IPv6:     strings.Contains(addr, ":"),
		}
		e.IPv4 = !e.IPv6

		var pidStr string
		if proto == "TCP" && len(fields) >= 5 {
			e.RemoteAddress, e.RemotePort = splitHostPort(fields[2])
			e.State = strings.ToUpper(fields[3])
			if e.State == "LISTENING" {
				e.State = "LISTEN"
			}
			pidStr = fields[4]
		} else {
			e.State = "UNCONN"
			pidStr = fields[3]
		}
		e.PID, _ = strconv.Atoi(pidStr)
		e.Process = names[e.PID]
		if e.PID > 0 {
			d, ok := details[e.PID]
			if !ok {
				d = loadProcDetails(e.PID)
				details[e.PID] = d
			}
			d.apply(&e)
		}
		results = append(results, e)
	}
	return results, nil
}

func splitHostPort(s string) (string, uint16) {
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return s, 0
	}
	host := strings.Trim(s[:i], "[]")
	p, _ := strconv.ParseUint(s[i+1:], 10, 16)
	return host, uint16(p)
}

func taskNames() map[int]string {
	names := map[int]string{}
	out, err := exec.Command("tasklist", "/fo", "csv", "/nh").Output()
	if err != nil {
		return names
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), ",", 3)
		if len(parts) < 2 {
			continue
		}
		pid, err := strconv.Atoi(strings.Trim(parts[1], "\""))
		if err != nil {
			continue
		}
		names[pid] = strings.TrimSuffix(strings.Trim(parts[0], "\""), ".exe")
	}
	return names
}

type procDetails struct {
	cmdline   string
	exe       string
	ppid      int
	startedAt *time.Time
	user      string
}

func (d *procDetails) apply(e *PortInfo) {
	if d == nil {
		return
	}
	e.Cmdline = d.cmdline
	e.Exe = d.exe
	e.PPID = d.ppid
	e.StartedAt = d.startedAt
	if e.User == "" {
		e.User = d.user
	}
}

// loadProcDetails shells out to PowerShell once per PID. It is best-effort:
// any failure simply leaves the details empty.
func loadProcDetails(pid int) *procDetails {
	d := &procDetails{}
	script := fmt.Sprintf(`$p = Get-CimInstance Win32_Process -Filter "ProcessId=%d"; if ($p) { $o = $p | Invoke-CimMethod -MethodName GetOwner; "$($p.ParentProcessId)`+"`t"+`$($p.CreationDate.ToString('o'))`+"`t"+`$($o.User)`+"`t"+`$($p.ExecutablePath)`+"`t"+`$($p.CommandLine)" }`, pid)
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return d
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "\t", 5)
	if len(parts) < 5 {
		return d
	}
	d.ppid, _ = strconv.Atoi(parts[0])
	if t, err := time.Parse(time.RFC3339Nano, parts[1]); err == nil {
		d.startedAt = &t
	}
	d.user = parts[2]
	d.exe = parts[3]
	d.cmdline = parts[4]
	return d
}

// IsPrivileged is a best-effort check for an elevated shell.
func IsPrivileged() bool {
	return exec.Command("net", "session").Run() == nil
}
