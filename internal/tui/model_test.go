package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Viljoen13/port-explorer/internal/ports"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() { lipgloss.SetColorProfile(termenv.Ascii) }

func fixtures() []ports.PortInfo {
	return []ports.PortInfo{
		{Protocol: "TCP", Port: 80, Address: "0.0.0.0", State: "LISTEN", PID: 100, Process: "nginx", User: "root", Service: "http", Exposed: true, IPv4: true},
		{Protocol: "TCP", Port: 443, Address: "0.0.0.0", State: "LISTEN", PID: 100, Process: "nginx", User: "root", Service: "https", Exposed: true, IPv4: true},
		{Protocol: "TCP", Port: 5432, Address: "127.0.0.1", State: "LISTEN", PID: 200, Process: "postgres", User: "postgres", Service: "postgres", IPv4: true},
		{Protocol: "TCP", Port: 3000, Address: "127.0.0.1", State: "LISTEN", PID: os.Getpid(), Process: "port-explorer", IPv4: true},
		{Protocol: "TCP", Port: 50000, Address: "192.168.1.5", RemoteAddress: "1.2.3.4", RemotePort: 443, State: "ESTABLISHED", PID: 300, Process: "curl", IPv4: true},
		{Protocol: "TCP", Port: 22, Address: "0.0.0.0", State: "LISTEN", PID: 0, User: "root", Service: "ssh", Exposed: true, IPv4: true},
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func loaded(t *testing.T, opts Options) Model {
	t.Helper()
	base := New(opts)
	// New() probes the real shell, so the sudo hints these tests assert on
	// disappear when the suite runs elevated (root, or a Windows CI runner).
	base.privileged = false
	var m tea.Model = base
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(refreshMsg{entries: fixtures(), at: time.Now()})
	return m.(Model)
}

func press(m Model, keys ...string) (Model, tea.Cmd) {
	var cmd tea.Cmd
	var mm tea.Model = m
	for _, k := range keys {
		mm, cmd = mm.Update(key(k))
	}
	return mm.(Model), cmd
}

func TestInitialStateShowsListeningOnly(t *testing.T) {
	m := loaded(t, Options{})
	if len(m.rows) != 5 {
		t.Fatalf("want 5 listening rows, got %d", len(m.rows))
	}
	if m.rows[0].entry.Port != 22 {
		t.Errorf("rows should be sorted by port, first is %d", m.rows[0].entry.Port)
	}
	view := m.View()
	for _, want := range []string{"5 listening", "3 exposed", "1 established", "1 hidden", "nginx", "postgres", "⚠ exposed"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}
	if strings.Contains(view, "curl") {
		t.Error("established connections should be hidden until 'a' is pressed")
	}
}

func TestToggleAllAndSort(t *testing.T) {
	m := loaded(t, Options{})
	m, _ = press(m, "a")
	if len(m.rows) != 6 || !strings.Contains(m.View(), "curl") {
		t.Errorf("after 'a' want 6 rows incl. curl, got %d", len(m.rows))
	}
	m, _ = press(m, "s") // sort by process
	if m.rows[0].entry.Process != "curl" {
		t.Errorf("sort by process: first row is %q", m.rows[0].entry.Process)
	}
	m, _ = press(m, "S") // flip direction
	if m.rows[0].entry.Process != "postgres" {
		t.Errorf("sort by process desc: first row is %q", m.rows[0].entry.Process)
	}
}

func TestFilterTyping(t *testing.T) {
	m := loaded(t, Options{})
	m, _ = press(m, "/", "n", "g", "i")
	if !m.filtering || m.query != "ngi" {
		t.Fatalf("filtering=%v query=%q", m.filtering, m.query)
	}
	if len(m.rows) != 2 {
		t.Errorf("live filter should show 2 nginx rows, got %d", len(m.rows))
	}
	m, _ = press(m, "backspace", "backspace", "backspace")
	if len(m.rows) != 5 {
		t.Errorf("clearing filter should restore rows, got %d", len(m.rows))
	}
	m, _ = press(m, "e", "x", "p", "o", "s", "e", "d", " ", "!", "2", "2", "enter")
	if m.filtering {
		t.Error("enter should leave filter mode")
	}
	if len(m.rows) != 2 {
		t.Errorf("'exposed !22' should match 80 and 443, got %d rows", len(m.rows))
	}
	m, _ = press(m, "esc")
	if m.query != "" || len(m.rows) != 5 {
		t.Errorf("esc should clear the filter: query=%q rows=%d", m.query, len(m.rows))
	}
}

func TestInitialQueryOption(t *testing.T) {
	m := loaded(t, Options{Query: "postgres"})
	if len(m.rows) != 1 || m.rows[0].entry.Port != 5432 {
		t.Errorf("initial query not applied: %d rows", len(m.rows))
	}
}

func TestGrouping(t *testing.T) {
	m := loaded(t, Options{})
	m, _ = press(m, "g")
	if !m.grouped || len(m.groups) != 4 {
		t.Fatalf("grouped=%v groups=%d", m.grouped, len(m.groups))
	}
	if len(m.rows) != 4 {
		t.Fatalf("collapsed groups should give 4 rows, got %d", len(m.rows))
	}
	// First group is the hidden sshd (port 22 sorts first); second is nginx.
	m, _ = press(m, "down", "enter")
	if len(m.rows) != 6 {
		t.Fatalf("expanding nginx should add 2 rows, got %d", len(m.rows))
	}
	if m.rows[2].kind != rowEntry || m.rows[2].entry.Port != 80 {
		t.Errorf("row 2 should be nginx :80, got %+v", m.rows[2])
	}
	// Esc from a child collapses and returns to the header.
	m, _ = press(m, "down", "esc")
	if len(m.rows) != 4 || m.cursor != 1 {
		t.Errorf("esc should collapse: rows=%d cursor=%d", len(m.rows), m.cursor)
	}
	// Expansion state survives a refresh.
	m, _ = press(m, "enter")
	var mm tea.Model = m
	mm, _ = mm.Update(refreshMsg{entries: fixtures(), at: time.Now()})
	m = mm.(Model)
	if len(m.rows) != 6 {
		t.Errorf("expansion should survive refresh, rows=%d", len(m.rows))
	}
	view := m.View()
	if !strings.Contains(view, "nginx") || !strings.Contains(view, "2 listening") {
		t.Errorf("group header missing: %s", view)
	}
}

func TestCursorStaysOnSocketAcrossRefresh(t *testing.T) {
	m := loaded(t, Options{})
	m, _ = press(m, "down", "down") // on :443
	if m.rows[m.cursor].entry.Port != 443 {
		t.Fatalf("setup: cursor on %d", m.rows[m.cursor].entry.Port)
	}
	// A new lower port appears; the cursor should follow :443.
	entries := append(fixtures(), ports.PortInfo{Protocol: "TCP", Port: 25, Address: "127.0.0.1", State: "LISTEN", PID: 900, Process: "smtpd", IPv4: true})
	var mm tea.Model = m
	mm, _ = mm.Update(refreshMsg{entries: entries, at: time.Now()})
	m = mm.(Model)
	if m.rows[m.cursor].entry.Port != 443 {
		t.Errorf("cursor drifted to %d", m.rows[m.cursor].entry.Port)
	}
	if !strings.Contains(m.View(), "● new") {
		t.Error("newly appeared socket should be highlighted")
	}
}

func TestDetailView(t *testing.T) {
	m := loaded(t, Options{})
	m, _ = press(m, "down", "enter") // :80 nginx
	if m.mode != viewDetail {
		t.Fatal("enter should open details")
	}
	view := m.View()
	for _, want := range []string{":80 http", "nginx", "reachable from the network", "Other ports held", "443"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail missing %q", want)
		}
	}
	m, _ = press(m, "esc")
	if m.mode != viewList {
		t.Error("esc should return to list")
	}
	// Hidden process shows the sudo hint.
	m, _ = press(m, "up", "enter")
	if !strings.Contains(m.View(), "Rerun with sudo") {
		t.Error("hidden owner should suggest sudo")
	}
}

func TestKillGuards(t *testing.T) {
	m := loaded(t, Options{})
	// Row 0 is :22 with no PID.
	m, _ = press(m, "d")
	if m.confirm != nil || !strings.Contains(m.message, "sudo") {
		t.Errorf("hidden pid: confirm=%v msg=%q", m.confirm, m.message)
	}
	// :3000 is our own PID.
	m, _ = press(m, "down", "down", "down")
	if m.rows[m.cursor].entry.PID != os.Getpid() {
		t.Fatalf("setup: expected self row, got %+v", m.rows[m.cursor].entry)
	}
	m, _ = press(m, "d")
	if m.confirm != nil || !strings.Contains(m.message, "itself") {
		t.Errorf("self pid: confirm=%v msg=%q", m.confirm, m.message)
	}
	// nginx asks for confirmation and 'n' cancels without killing.
	m, _ = press(m, "up", "up", "d")
	if m.confirm == nil || m.confirm.pid != 100 || m.confirm.force {
		t.Fatalf("expected SIGTERM confirm for nginx, got %+v", m.confirm)
	}
	if !strings.Contains(m.View(), "SIGKILL (force)") {
		t.Error("confirm box should offer force kill")
	}
	m, _ = press(m, "n")
	if m.confirm != nil {
		t.Error("n should cancel")
	}
	m, _ = press(m, "D")
	if m.confirm == nil || !m.confirm.force {
		t.Error("D should preselect force")
	}
	m, _ = press(m, "esc")
	if m.confirm != nil {
		t.Error("esc should cancel")
	}
}

func TestHelpAndQuit(t *testing.T) {
	m := loaded(t, Options{})
	m, _ = press(m, "?")
	if m.mode != viewHelp || !strings.Contains(m.View(), "keyboard reference") {
		t.Error("? should open help")
	}
	m, _ = press(m, "x")
	if m.mode != viewList {
		t.Error("any key should close help back to the list")
	}
	_, cmd := press(m, "q")
	if cmd == nil {
		t.Fatal("q should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q should produce QuitMsg")
	}
}

func TestLiveModeTick(t *testing.T) {
	m := loaded(t, Options{Refresh: time.Second})
	m, cmd := press(m, "w")
	if !m.live || cmd == nil {
		t.Fatal("w should enable live mode and schedule a tick")
	}
	var mm tea.Model = m
	mm, cmd = mm.Update(tickMsg{seq: m.tickSeq})
	m = mm.(Model)
	if cmd == nil || !m.fetching {
		t.Error("tick should trigger a fetch")
	}
	// Stale ticks from a previous live session are ignored.
	m.fetching = false
	mm, cmd = m.Update(tickMsg{seq: m.tickSeq - 1})
	if cmd != nil {
		t.Error("stale tick should be ignored")
	}
}

func TestNarrowTerminalStillRenders(t *testing.T) {
	m := loaded(t, Options{})
	var mm tea.Model = m
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	view := mm.View()
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line wider than terminal (%d): %q", w, line)
		}
	}
}

func TestErrorScreen(t *testing.T) {
	var m tea.Model = New(Options{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(refreshMsg{err: os.ErrPermission, at: time.Now()})
	if !strings.Contains(m.View(), "Could not read") {
		t.Error("error screen not shown")
	}
}
