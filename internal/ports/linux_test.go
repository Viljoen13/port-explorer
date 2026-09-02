//go:build linux

package ports

import "testing"

func TestParseHexAddr(t *testing.T) {
	cases := []struct {
		in   string
		addr string
		port uint16
	}{
		{"0100007F:1F90", "127.0.0.1", 8080},
		{"00000000:0050", "0.0.0.0", 80},
		{"0101A8C0:01BB", "192.168.1.1", 443},
		{"00000000000000000000000001000000:0BB8", "::1", 3000},
		{"00000000000000000000000000000000:1538", "::", 5432},
		{"0000000000000000FFFF00000100007F:0035", "127.0.0.1", 53}, // v4-mapped
		{"000080FE000000007A463F13A274619F:0222", "fe80::133f:467a:9f61:74a2", 546},
	}
	for _, tc := range cases {
		addr, port, err := parseHexAddr(tc.in)
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if addr != tc.addr || port != tc.port {
			t.Errorf("%s: got %s:%d want %s:%d", tc.in, addr, port, tc.addr, tc.port)
		}
	}
	if _, _, err := parseHexAddr("garbage"); err == nil {
		t.Error("expected error for malformed address")
	}
}

func TestTCPStates(t *testing.T) {
	if tcpStates["0A"] != "LISTEN" || tcpStates["01"] != "ESTABLISHED" {
		t.Error("state table broken")
	}
}

func TestListSmoke(t *testing.T) {
	entries, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Every machine has at least one socket; and every entry must be well-formed.
	for _, e := range entries {
		if e.Protocol != "TCP" && e.Protocol != "UDP" {
			t.Errorf("bad protocol %q", e.Protocol)
		}
		if e.State == "" {
			t.Errorf("empty state for %+v", e)
		}
		if !e.IPv4 && !e.IPv6 {
			t.Errorf("no family for %+v", e)
		}
	}
}
