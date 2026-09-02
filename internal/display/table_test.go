package display

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Viljoen13/port-explorer/internal/ports"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func fixtures() []ports.PortInfo {
	return []ports.PortInfo{
		{Protocol: "TCP", Port: 80, Address: "0.0.0.0", State: "LISTEN", PID: 100, Process: "nginx", User: "root", Service: "http", Exposed: true, IPv4: true, IPv6: true},
		{Protocol: "TCP", Port: 5432, Address: "127.0.0.1", State: "LISTEN", PID: 200, Process: "postgres", User: "postgres", Service: "postgres", IPv4: true, Container: "db-1"},
		{Protocol: "UDP", Port: 53, Address: "127.0.0.53", State: "UNCONN", IPv4: true, User: "systemd-resolve"},
	}
}

func init() { lipgloss.SetColorProfile(termenv.Ascii) }

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, fixtures(), Options{})
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("want header + 3 rows, got %d lines:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "PORT") || !strings.Contains(lines[0], "BIND") {
		t.Errorf("header: %q", lines[0])
	}
	if !strings.Contains(lines[1], "tcp46") || !strings.Contains(lines[1], "⚠ exposed") {
		t.Errorf("row 1 should show dual-stack and exposure: %q", lines[1])
	}
	if !strings.Contains(lines[2], "◧ db-1") {
		t.Errorf("row 2 should show container: %q", lines[2])
	}
	if !strings.Contains(lines[3], " - ") {
		t.Errorf("row 3 should show dash for missing pid/process: %q", lines[3])
	}
	for _, l := range lines {
		if strings.HasSuffix(l, " ") {
			t.Errorf("trailing whitespace: %q", l)
		}
	}
}

func TestPrintTableWide(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, fixtures(), Options{Wide: true, ShowRemote: true})
	head := strings.SplitN(buf.String(), "\n", 2)[0]
	for _, col := range []string{"USER", "UPTIME", "REMOTE", "COMMAND"} {
		if !strings.Contains(head, col) {
			t.Errorf("wide header missing %s: %q", col, head)
		}
	}
}

func TestPrintTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, nil, Options{})
	if buf.Len() != 0 {
		t.Errorf("empty table should print nothing, got %q", buf.String())
	}
}

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintJSON(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "[]" {
		t.Errorf("nil entries should encode as [], got %q", buf.String())
	}
	buf.Reset()
	if err := PrintJSON(&buf, fixtures()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"service": "http"`) || strings.Contains(buf.String(), "started_at") {
		t.Errorf("unexpected json: %s", buf.String())
	}
}

func TestPrintCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintCSV(&buf, fixtures()); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "port,protocol") {
		t.Errorf("csv: %s", buf.String())
	}
}

func TestPrintPlain(t *testing.T) {
	var buf bytes.Buffer
	PrintPlain(&buf, fixtures())
	if !strings.HasPrefix(buf.String(), "80\tTCP\t100\tnginx\tLISTEN\t0.0.0.0\n") {
		t.Errorf("plain: %q", buf.String())
	}
}

func TestSummary(t *testing.T) {
	s := Summary(fixtures(), false)
	for _, want := range []string{"3 listening", "1 exposed", "1 in containers", "sudo"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary missing %q: %s", want, s)
		}
	}
	if strings.Contains(Summary(fixtures(), true), "sudo") {
		t.Error("privileged summary should not suggest sudo")
	}
}
