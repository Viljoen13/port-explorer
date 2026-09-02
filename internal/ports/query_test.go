package ports

import "testing"

func sample() []PortInfo {
	entries := []PortInfo{
		{Protocol: "TCP", Port: 80, Address: "0.0.0.0", State: "LISTEN", PID: 100, Process: "nginx", User: "root", Cmdline: "nginx -g daemon off;"},
		{Protocol: "TCP", Port: 443, Address: "0.0.0.0", State: "LISTEN", PID: 100, Process: "nginx", User: "root"},
		{Protocol: "TCP", Port: 5432, Address: "127.0.0.1", State: "LISTEN", PID: 200, Process: "postgres", User: "postgres"},
		{Protocol: "TCP", Port: 8080, Address: "0.0.0.0", State: "LISTEN", PID: 300, Process: "docker-proxy", User: "root",
			Cmdline: "/usr/bin/docker-proxy -proto tcp -host-ip 0.0.0.0 -host-port 8080 -container-ip 172.17.0.2 -container-port 80"},
		{Protocol: "UDP", Port: 53, Address: "127.0.0.53", State: "UNCONN", User: "systemd-resolve"},
		{Protocol: "TCP", Port: 45000, Address: "192.168.1.5", RemoteAddress: "1.2.3.4", RemotePort: 443, State: "ESTABLISHED", PID: 400, Process: "firefox", User: "me"},
		{Protocol: "TCP", Port: 3000, Address: "::1", State: "LISTEN", PID: 500, Process: "node", User: "me", Container: "web-1"},
	}
	for i := range entries {
		finalize(&entries[i])
	}
	return entries
}

func portsOf(entries []PortInfo) []uint16 {
	out := make([]uint16, len(entries))
	for i, e := range entries {
		out[i] = e.Port
	}
	return out
}

func TestQuery(t *testing.T) {
	entries := sample()
	cases := []struct {
		query string
		want  []uint16
	}{
		{"", []uint16{80, 443, 5432, 8080, 53, 45000, 3000}},
		{"80", []uint16{80, 8080}},          // port prefix
		{":80", []uint16{80}},               // exact port
		{"port:443", []uint16{443}},         // exact port, long form
		{"100", []uint16{80, 443}},          // PID match (no port starts with 100)
		{"3000-6000", []uint16{5432, 3000}}, // range
		{">8000", []uint16{8080, 45000}},
		{"<=443", []uint16{80, 443, 53}},
		{"udp", []uint16{53}},
		{"tcp listen", []uint16{80, 443, 5432, 8080, 3000}},
		{"estab", []uint16{45000}},
		{"exposed", []uint16{80, 443, 8080}},
		{"local", []uint16{5432, 53, 3000}},
		{"docker", []uint16{8080, 3000}},
		{"unknown", []uint16{53}},
		{"nginx", []uint16{80, 443}},
		{"NGINX", []uint16{80, 443}}, // case-insensitive
		{"proc:node", []uint16{3000}},
		{"user:root", []uint16{80, 443, 8080}},
		{"pid:200", []uint16{5432}},
		{"svc:postgres", []uint16{5432}},
		{"remote:443", []uint16{45000}},
		{"addr:192.168", []uint16{45000}},
		{"!nginx listen", []uint16{5432, 8080, 53, 3000}},
		{"-exposed tcp", []uint16{5432, 45000, 3000}},
		{"web-1", []uint16{3000}},    // container name as free text
		{"daemon", []uint16{80}},     // cmdline as free text
		{"time_wait", []uint16{}},    // state name with no matches
		{"nothing-here", []uint16{}}, // free text with no matches
	}
	for _, tc := range cases {
		got := portsOf(ParseQuery(tc.query).Filter(entries))
		if len(got) != len(tc.want) {
			t.Errorf("%q: got %v, want %v", tc.query, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%q: got %v, want %v", tc.query, got, tc.want)
				break
			}
		}
	}
}

func TestQueryExactPort(t *testing.T) {
	for _, tc := range []struct {
		q    string
		port uint16
		ok   bool
	}{
		{"8080", 8080, true}, {":8080", 8080, true}, {"nginx", 0, false}, {"80 nginx", 0, false}, {"", 0, false}, {"99999", 0, false},
	} {
		port, ok := ParseQuery(tc.q).ExactPort()
		if port != tc.port || ok != tc.ok {
			t.Errorf("%q: got (%d,%v) want (%d,%v)", tc.q, port, ok, tc.port, tc.ok)
		}
	}
}

func TestQueryEmpty(t *testing.T) {
	if !ParseQuery("   ").Empty() {
		t.Fatal("whitespace query should be empty")
	}
	if ParseQuery("x").Empty() {
		t.Fatal("non-empty query reported empty")
	}
}
