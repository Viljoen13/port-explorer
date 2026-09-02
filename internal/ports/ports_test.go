package ports

import (
	"testing"
	"time"
)

func TestFinalizeExposure(t *testing.T) {
	cases := []struct {
		proto, addr, state string
		exposed            bool
	}{
		{"TCP", "0.0.0.0", "LISTEN", true},
		{"TCP", "::", "LISTEN", true},
		{"TCP", "192.168.1.5", "LISTEN", true},
		{"TCP", "172.17.0.1", "LISTEN", true},
		{"TCP", "127.0.0.1", "LISTEN", false},
		{"TCP", "::1", "LISTEN", false},
		{"TCP", "127.0.0.53", "LISTEN", false},
		{"TCP", "0.0.0.0", "ESTABLISHED", false},
		{"UDP", "0.0.0.0", "UNCONN", true},
		{"UDP", "224.0.0.251", "UNCONN", false},               // multicast bind
		{"UDP", "fe80::133f:467a:9f61:74a2", "UNCONN", false}, // link-local
		{"UDP", "192.168.1.5", "ESTABLISHED", false},
	}
	for _, tc := range cases {
		e := PortInfo{Protocol: tc.proto, Address: tc.addr, State: tc.state, Port: 1234}
		finalize(&e)
		if e.Exposed != tc.exposed {
			t.Errorf("%s %s %s: exposed=%v want %v", tc.proto, tc.addr, tc.state, e.Exposed, tc.exposed)
		}
	}
}

func TestFinalizeDerivedFields(t *testing.T) {
	e := PortInfo{Protocol: "TCP", Port: 5432, Address: "::", State: "LISTEN", RemoteAddress: "::", RemotePort: 0,
		Cmdline: "/usr/bin/docker-proxy -container-ip 172.17.0.2 -container-port 5432"}
	finalize(&e)
	if e.Service != "postgres" {
		t.Errorf("service = %q", e.Service)
	}
	if !e.IPv6 || e.IPv4 {
		t.Errorf("family: v4=%v v6=%v", e.IPv4, e.IPv6)
	}
	if e.RemoteAddress != "" {
		t.Errorf("wildcard remote address should be cleared, got %q", e.RemoteAddress)
	}
	if e.Forward != "172.17.0.2:5432" {
		t.Errorf("forward = %q", e.Forward)
	}
	if e.BindLabel() != "*" {
		t.Errorf("bind label = %q", e.BindLabel())
	}
}

func TestMerge(t *testing.T) {
	entries := []PortInfo{
		{Protocol: "TCP", Port: 80, Address: "0.0.0.0", State: "LISTEN", PID: 1, IPv4: true},
		{Protocol: "TCP", Port: 80, Address: "::", State: "LISTEN", PID: 1, IPv6: true},
		{Protocol: "TCP", Port: 5432, Address: "::1", State: "LISTEN", PID: 2, IPv6: true},
		{Protocol: "TCP", Port: 5432, Address: "127.0.0.1", State: "LISTEN", PID: 2, IPv4: true},
		{Protocol: "TCP", Port: 8080, Address: "192.168.1.5", State: "LISTEN", PID: 3, IPv4: true},
		{Protocol: "TCP", Port: 8080, Address: "10.0.0.5", State: "LISTEN", PID: 3, IPv4: true}, // distinct addresses: keep both
		{Protocol: "TCP", Port: 80, Address: "0.0.0.0", State: "LISTEN", PID: 9, IPv4: true},    // different pid: keep
	}
	got := Merge(entries)
	if len(got) != 5 {
		t.Fatalf("merged to %d entries, want 5: %+v", len(got), got)
	}
	if !got[0].IPv4 || !got[0].IPv6 || got[0].Address != "0.0.0.0" {
		t.Errorf("wildcard merge: %+v", got[0])
	}
	if !got[1].IPv4 || !got[1].IPv6 || got[1].Address != "127.0.0.1" {
		t.Errorf("loopback merge should prefer the IPv4 address: %+v", got[1])
	}
}

func TestSort(t *testing.T) {
	entries := []PortInfo{
		{Port: 443, Process: "nginx", PID: 10, State: "ESTABLISHED"},
		{Port: 22, Process: "", PID: 0, State: "LISTEN"},
		{Port: 80, Process: "nginx", PID: 10, State: "LISTEN"},
		{Port: 5432, Process: "postgres", PID: 5, State: "LISTEN"},
	}
	Sort(entries, SortPort, true)
	if p := portsOf(entries); p[0] != 22 || p[3] != 5432 {
		t.Errorf("sort port asc: %v", p)
	}
	Sort(entries, SortPort, false)
	if p := portsOf(entries); p[0] != 5432 || p[3] != 22 {
		t.Errorf("sort port desc: %v", p)
	}
	Sort(entries, SortProcess, true)
	if p := portsOf(entries); p[0] != 80 || p[1] != 443 || p[2] != 5432 || p[3] != 22 {
		t.Errorf("sort process asc (unknown last, ties by port): %v", p)
	}
	Sort(entries, SortProcess, false)
	if p := portsOf(entries); p[0] != 5432 || p[3] != 22 {
		t.Errorf("sort process desc should still sink unknown: %v", p)
	}
	Sort(entries, SortState, true)
	if entries[0].State != "LISTEN" || entries[3].State != "ESTABLISHED" {
		t.Errorf("sort state: %+v", entries)
	}
	Sort(entries, SortPID, true)
	if entries[0].PID != 0 || entries[3].PID != 10 {
		t.Errorf("sort pid: %+v", entries)
	}
}

func TestSummarize(t *testing.T) {
	s := Summarize(sample())
	if s.Total != 7 || s.Listening != 6 || s.Established != 1 || s.Exposed != 3 || s.Hidden != 1 || s.Processes != 5 || s.Containers != 1 {
		t.Errorf("unexpected stats: %+v", s)
	}
}

func TestListeningAndHidden(t *testing.T) {
	entries := sample()
	if n := len(Listening(entries)); n != 6 {
		t.Errorf("listening = %d", n)
	}
	if n := Hidden(entries); n != 1 {
		t.Errorf("hidden = %d", n)
	}
}

func TestRemoteAndKey(t *testing.T) {
	e := PortInfo{Protocol: "TCP", Port: 1, Address: "::1", RemoteAddress: "2001:db8::1", RemotePort: 443, State: "ESTABLISHED"}
	if got := e.Remote(); got != "[2001:db8::1]:443" {
		t.Errorf("remote = %q", got)
	}
	listener := PortInfo{Protocol: "TCP", Port: 80, Address: "0.0.0.0", State: "LISTEN"}
	if listener.Remote() != "" {
		t.Errorf("listener remote should be empty")
	}
	a, b := e, e
	b.RemotePort = 444
	if a.Key() == b.Key() {
		t.Error("keys should differ when remote port differs")
	}
}

func TestFormatDuration(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{0, ""}, {45 * time.Second, "45s"}, {3*time.Minute + 5*time.Second, "3m5s"},
		{2*time.Hour + 15*time.Minute, "2h15m"}, {49 * time.Hour, "2d1h"},
	} {
		if got := FormatDuration(tc.d); got != tc.want {
			t.Errorf("%v: got %q want %q", tc.d, got, tc.want)
		}
	}
}

func TestParseDockerProxy(t *testing.T) {
	target, ok := ParseDockerProxy("/usr/bin/docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 8080 -container-ip 172.17.0.2 -container-port 80")
	if !ok || target != "172.17.0.2:80" {
		t.Errorf("got %q %v", target, ok)
	}
	target, ok = ParseDockerProxy("/usr/bin/docker-proxy -proto tcp -host-ip :: -host-port 8080 -container-ip fd00::2 -container-port 80")
	if !ok || target != "[fd00::2]:80" {
		t.Errorf("ipv6: got %q %v", target, ok)
	}
	if _, ok := ParseDockerProxy("nginx -g daemon off;"); ok {
		t.Error("nginx should not parse as docker-proxy")
	}
	if _, ok := ParseDockerProxy("/usr/bin/docker-proxy -proto tcp"); ok {
		t.Error("incomplete docker-proxy args should not parse")
	}
}

func TestServiceName(t *testing.T) {
	for _, tc := range []struct {
		port uint16
		want string
	}{
		{80, "http"}, {443, "https"}, {5432, "postgres"}, {6379, "redis"}, {3000, "dev-server"}, {27017, "mongodb"}, {49152, ""},
	} {
		if got := ServiceName(tc.port, "TCP"); got != tc.want {
			t.Errorf("port %d: got %q want %q", tc.port, got, tc.want)
		}
	}
}

func TestIsFreeAndFindFree(t *testing.T) {
	free, err := FindFree(40000, 2, map[uint16]bool{40000: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(free) != 2 || free[0] == 40000 {
		t.Errorf("FindFree should skip in-use ports: %v", free)
	}
	if !IsFree(free[0]) {
		t.Errorf("port %d reported free but cannot be bound", free[0])
	}
}
