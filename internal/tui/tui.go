package tui

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Viljoen13/port-explorer/internal/ports"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62"))

	normalStyle = lipgloss.NewStyle()

	listenStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	estabStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	waitStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	detailLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	detailValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	helpStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	successStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)

	filterPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	filterInputStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	confirmStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)

	groupHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	groupCountStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

type view int

const (
	viewList view = iota
	viewDetail
)

// A row in the grouped view: either a group header or a port entry
type listRow struct {
	isGroup  bool
	group    *processGroup
	entry    ports.PortInfo
	expanded bool // only meaningful for group headers
}

type processGroup struct {
	name      string
	pid       int
	entries   []ports.PortInfo
	expanded  bool
	listening int
	established int
	other     int
}

type Model struct {
	entries   []ports.PortInfo
	filtered  []ports.PortInfo
	rows      []listRow // flattened rows for display (used in grouped mode)
	cursor    int
	offset    int
	height    int
	width     int
	view      view
	filter    string
	filtering bool
	grouped   bool
	groups    []*processGroup
	message   string
	confirm   bool
	err       error
}

type refreshMsg struct {
	entries []ports.PortInfo
	err     error
}

func refresh() tea.Msg {
	entries, err := ports.List()
	return refreshMsg{entries: entries, err: err}
}

func New() Model {
	return Model{
		height: 24,
		width:  80,
	}
}

func (m Model) Init() tea.Cmd {
	return refresh
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		return m, nil

	case refreshMsg:
		m.err = msg.err
		if msg.err == nil {
			m.entries = msg.entries
			m.applyFilter()
		}
		return m, nil

	case tea.KeyMsg:
		if m.confirm {
			return m.handleConfirm(msg)
		}
		if m.filtering {
			return m.handleFilterInput(msg)
		}
		return m.handleNormal(msg)
	}

	return m, nil
}

func (m Model) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.confirm = false
		pid, process := m.selectedPidProcess()
		if pid > 0 {
			err := syscall.Kill(pid, syscall.SIGTERM)
			if err != nil {
				m.message = errorStyle.Render(fmt.Sprintf("Failed to kill PID %d: %v", pid, err))
			} else {
				m.message = successStyle.Render(fmt.Sprintf("Sent SIGTERM to %s (PID %d)", process, pid))
			}
			return m, refresh
		}
	case "n", "N", "esc", "escape":
		m.confirm = false
		m.message = ""
	}
	return m, nil
}

func (m Model) handleFilterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filtering = false
		m.applyFilter()
	case "esc", "escape":
		m.filtering = false
		m.filter = ""
		m.applyFilter()
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.applyFilter()
		}
	default:
		if len(msg.String()) == 1 {
			m.filter += msg.String()
			m.applyFilter()
		}
	}
	return m, nil
}

func (m Model) handleNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.view == viewList && m.cursor > 0 {
			m.cursor--
			m.message = ""
		}

	case "down", "j":
		maxIdx := m.maxIndex()
		if m.view == viewList && m.cursor < maxIdx {
			m.cursor++
			m.message = ""
		}

	case "enter", "right", "l":
		if m.view == viewList {
			if m.grouped {
				// Toggle expand/collapse on group headers
				if m.cursor < len(m.rows) && m.rows[m.cursor].isGroup {
					m.rows[m.cursor].group.expanded = !m.rows[m.cursor].group.expanded
					m.rebuildRows()
				} else if m.cursor < len(m.rows) && !m.rows[m.cursor].isGroup {
					m.view = viewDetail
				}
			} else if len(m.filtered) > 0 {
				m.view = viewDetail
				m.message = ""
			}
		}

	case "esc", "left", "h", "backspace":
		if m.view == viewDetail {
			m.view = viewList
			m.message = ""
		}

	case "/":
		m.filtering = true
		m.filter = ""
		m.message = ""

	case "r":
		m.message = dimStyle.Render("Refreshing...")
		return m, refresh

	case "g":
		if m.view == viewList {
			m.grouped = !m.grouped
			m.cursor = 0
			m.offset = 0
			m.applyFilter()
			if m.grouped {
				m.message = dimStyle.Render("Grouped by process")
			} else {
				m.message = dimStyle.Render("Ungrouped")
			}
		}

	case "d", "x":
		if m.view == viewList {
			pid, process := m.selectedPidProcess()
			if pid > 0 {
				port := m.selectedPort()
				m.confirm = true
				m.message = confirmStyle.Render(fmt.Sprintf("Kill %s (PID %d) on port %d? (y/n)", process, pid, port))
			} else {
				m.message = errorStyle.Render("No PID available — try running with sudo")
			}
		}

	case "home":
		m.cursor = 0
		m.message = ""

	case "end", "G":
		m.cursor = m.maxIndex()
		m.message = ""
	}

	return m, nil
}

func (m *Model) selectedPidProcess() (int, string) {
	if m.grouped {
		if m.cursor < len(m.rows) {
			row := m.rows[m.cursor]
			if row.isGroup {
				return row.group.pid, row.group.name
			}
			return row.entry.PID, row.entry.Process
		}
		return 0, ""
	}
	if m.cursor < len(m.filtered) {
		return m.filtered[m.cursor].PID, m.filtered[m.cursor].Process
	}
	return 0, ""
}

func (m *Model) selectedPort() uint16 {
	if m.grouped {
		if m.cursor < len(m.rows) {
			row := m.rows[m.cursor]
			if row.isGroup && len(row.group.entries) > 0 {
				return row.group.entries[0].Port
			}
			return row.entry.Port
		}
		return 0
	}
	if m.cursor < len(m.filtered) {
		return m.filtered[m.cursor].Port
	}
	return 0
}

func (m *Model) selectedEntry() *ports.PortInfo {
	if m.grouped {
		if m.cursor < len(m.rows) && !m.rows[m.cursor].isGroup {
			return &m.rows[m.cursor].entry
		}
		return nil
	}
	if m.cursor < len(m.filtered) {
		return &m.filtered[m.cursor]
	}
	return nil
}

func (m *Model) maxIndex() int {
	if m.grouped {
		return max(0, len(m.rows)-1)
	}
	return max(0, len(m.filtered)-1)
}

func (m *Model) applyFilter() {
	if m.filter == "" {
		m.filtered = m.entries
	} else {
		lower := strings.ToLower(m.filter)
		var out []ports.PortInfo
		for _, e := range m.entries {
			portStr := strconv.Itoa(int(e.Port))
			if strings.Contains(portStr, lower) ||
				strings.Contains(strings.ToLower(e.Process), lower) ||
				strings.Contains(strings.ToLower(e.Protocol), lower) ||
				strings.Contains(strings.ToLower(e.State), lower) {
				out = append(out, e)
			}
		}
		m.filtered = out
	}

	if m.grouped {
		m.buildGroups()
		m.rebuildRows()
	}

	if m.cursor > m.maxIndex() {
		m.cursor = m.maxIndex()
	}
}

func (m *Model) buildGroups() {
	groupMap := make(map[string]*processGroup)
	var order []string

	for _, e := range m.filtered {
		key := fmt.Sprintf("%s:%d", e.Process, e.PID)
		if key == ":0" {
			key = fmt.Sprintf("unknown:%s:%d", e.Address, e.Port)
		}
		g, ok := groupMap[key]
		if !ok {
			name := e.Process
			if name == "" {
				name = fmt.Sprintf("(unknown pid:%d)", e.PID)
			}
			g = &processGroup{name: name, pid: e.PID}
			groupMap[key] = g
			order = append(order, key)
		}
		g.entries = append(g.entries, e)
		switch e.State {
		case "LISTEN":
			g.listening++
		case "ESTABLISHED":
			g.established++
		default:
			g.other++
		}
	}

	m.groups = make([]*processGroup, 0, len(order))
	for _, key := range order {
		m.groups = append(m.groups, groupMap[key])
	}

	// Sort groups: by process name, then by port
	sort.Slice(m.groups, func(i, j int) bool {
		if m.groups[i].name != m.groups[j].name {
			return strings.ToLower(m.groups[i].name) < strings.ToLower(m.groups[j].name)
		}
		if len(m.groups[i].entries) > 0 && len(m.groups[j].entries) > 0 {
			return m.groups[i].entries[0].Port < m.groups[j].entries[0].Port
		}
		return false
	})
}

func (m *Model) rebuildRows() {
	m.rows = nil
	for _, g := range m.groups {
		m.rows = append(m.rows, listRow{isGroup: true, group: g, expanded: g.expanded})
		if g.expanded {
			for _, e := range g.entries {
				m.rows = append(m.rows, listRow{isGroup: false, entry: e})
			}
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
}

func (m Model) View() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v\nPress q to quit.", m.err))
	}
	if m.entries == nil {
		return dimStyle.Render("Loading...")
	}

	switch m.view {
	case viewDetail:
		return m.renderDetail()
	default:
		if m.grouped {
			return m.renderGrouped()
		}
		return m.renderList()
	}
}

func (m Model) renderList() string {
	var b strings.Builder

	title := titleStyle.Render(" port-explorer ")
	count := dimStyle.Render(fmt.Sprintf(" %d ports", len(m.filtered)))
	b.WriteString(title + count + "\n\n")

	if m.filtering {
		b.WriteString(filterPromptStyle.Render("filter: ") + filterInputStyle.Render(m.filter) + "█\n\n")
	} else if m.filter != "" {
		b.WriteString(dimStyle.Render(fmt.Sprintf("filter: %s (press / to change, esc to clear)", m.filter)) + "\n\n")
	}

	header := dimStyle.Render(fmt.Sprintf("  %-7s %-6s %-8s %-20s %-14s %s", "PORT", "PROTO", "PID", "PROCESS", "STATE", "ADDRESS"))
	b.WriteString(header + "\n")

	listHeight := m.listHeight()

	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+listHeight {
		m.offset = m.cursor - listHeight + 1
	}

	end := m.offset + listHeight
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	if len(m.filtered) == 0 {
		b.WriteString(dimStyle.Render("\n  No matching ports found.") + "\n")
	}

	for i := m.offset; i < end; i++ {
		e := m.filtered[i]
		line := formatPortLine(e)
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("▸ "+line) + "\n")
		} else {
			b.WriteString(normalStyle.Render("  "+line) + "\n")
		}
	}

	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(m.message + "\n")
	}

	help := "↑/↓ navigate • enter details • d kill • / filter • g group • r refresh • q quit"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func (m Model) renderGrouped() string {
	var b strings.Builder

	title := titleStyle.Render(" port-explorer ")
	groupLabel := dimStyle.Render(fmt.Sprintf(" %d processes, %d ports (grouped)", len(m.groups), len(m.filtered)))
	b.WriteString(title + groupLabel + "\n\n")

	if m.filtering {
		b.WriteString(filterPromptStyle.Render("filter: ") + filterInputStyle.Render(m.filter) + "█\n\n")
	} else if m.filter != "" {
		b.WriteString(dimStyle.Render(fmt.Sprintf("filter: %s (press / to change, esc to clear)", m.filter)) + "\n\n")
	}

	listHeight := m.listHeight()

	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+listHeight {
		m.offset = m.cursor - listHeight + 1
	}

	end := m.offset + listHeight
	if end > len(m.rows) {
		end = len(m.rows)
	}

	if len(m.rows) == 0 {
		b.WriteString(dimStyle.Render("\n  No matching ports found.") + "\n")
	}

	for i := m.offset; i < end; i++ {
		row := m.rows[i]
		if row.isGroup {
			g := row.group
			arrow := "▶"
			if g.expanded {
				arrow = "▼"
			}

			// Build stats summary
			var stats []string
			stats = append(stats, fmt.Sprintf("%d ports", len(g.entries)))
			if g.listening > 0 {
				stats = append(stats, listenStyle.Render(fmt.Sprintf("%d listen", g.listening)))
			}
			if g.established > 0 {
				stats = append(stats, estabStyle.Render(fmt.Sprintf("%d estab", g.established)))
			}
			if g.other > 0 {
				stats = append(stats, dimStyle.Render(fmt.Sprintf("%d other", g.other)))
			}

			pidStr := ""
			if g.pid > 0 {
				pidStr = dimStyle.Render(fmt.Sprintf(" (PID %d)", g.pid))
			}

			line := fmt.Sprintf("%s %s%s  %s",
				arrow,
				groupHeaderStyle.Render(g.name),
				pidStr,
				groupCountStyle.Render(strings.Join(stats, ", ")))

			if i == m.cursor {
				b.WriteString(selectedStyle.Render(line) + "\n")
			} else {
				b.WriteString(line + "\n")
			}
		} else {
			e := row.entry
			line := fmt.Sprintf("    %-7d %-6s %-14s %s",
				e.Port, e.Protocol, styleState(e.State), e.Address)
			if i == m.cursor {
				b.WriteString(selectedStyle.Render("  ▸"+line) + "\n")
			} else {
				b.WriteString(dimStyle.Render("   ")+normalStyle.Render(line) + "\n")
			}
		}
	}

	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(m.message + "\n")
	}

	help := "↑/↓ navigate • enter expand/details • d kill • / filter • g ungroup • r refresh • q quit"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func (m Model) renderDetail() string {
	e := m.selectedEntry()
	if e == nil {
		return "No selection"
	}

	var b strings.Builder

	title := titleStyle.Render(fmt.Sprintf(" Port %d — Details ", e.Port))
	b.WriteString(title + "\n\n")

	writeDetail := func(label, value string) {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			detailLabelStyle.Render(fmt.Sprintf("%-12s", label+":")),
			detailValueStyle.Render(value)))
	}

	writeDetail("Port", strconv.Itoa(int(e.Port)))
	writeDetail("Protocol", e.Protocol)
	writeDetail("State", styleState(e.State))
	writeDetail("Address", e.Address)

	pidStr := "-"
	if e.PID > 0 {
		pidStr = strconv.Itoa(e.PID)
	}
	writeDetail("PID", pidStr)

	process := e.Process
	if process == "" {
		process = "-"
	}
	writeDetail("Process", process)

	if e.PID > 0 {
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("  Other ports by this process:") + "\n")
		found := false
		for _, other := range m.entries {
			if other.PID == e.PID && other.Port != e.Port {
				b.WriteString(fmt.Sprintf("    %s %d/%s (%s)\n",
					dimStyle.Render("•"),
					other.Port, other.Protocol,
					styleState(other.State)))
				found = true
			}
		}
		if !found {
			b.WriteString(dimStyle.Render("    None") + "\n")
		}
	}

	if e.PID > 0 {
		b.WriteString("\n")
		cmdline := readProcFile(e.PID, "cmdline")
		if cmdline != "" {
			writeDetail("Command", cmdline)
		}
		cwd := readProcLink(e.PID, "cwd")
		if cwd != "" {
			writeDetail("Working Dir", cwd)
		}
		exe := readProcLink(e.PID, "exe")
		if exe != "" {
			writeDetail("Executable", exe)
		}
	}

	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(m.message + "\n")
	}

	help := "esc/← go back • d kill process • r refresh • q quit"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func (m Model) listHeight() int {
	h := m.height - 8
	if m.filtering || m.filter != "" {
		h -= 2
	}
	if h < 3 {
		h = 3
	}
	return h
}

func formatPortLine(e ports.PortInfo) string {
	pidStr := "-"
	if e.PID > 0 {
		pidStr = strconv.Itoa(e.PID)
	}
	process := e.Process
	if process == "" {
		process = "-"
	}
	state := styleState(e.State)
	return fmt.Sprintf("%-7d %-6s %-8s %-20s %-14s %s",
		e.Port, e.Protocol, pidStr, truncate(process, 20), state, e.Address)
}

func styleState(state string) string {
	switch state {
	case "LISTEN":
		return listenStyle.Render(state)
	case "ESTABLISHED":
		return estabStyle.Render(state)
	case "TIME_WAIT", "CLOSE_WAIT", "FIN_WAIT1", "FIN_WAIT2":
		return waitStyle.Render(state)
	default:
		return dimStyle.Render(state)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func readProcFile(pid int, name string) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/%s", pid, name))
	if err != nil {
		return ""
	}
	s := strings.ReplaceAll(string(data), "\x00", " ")
	return strings.TrimSpace(s)
}

func readProcLink(pid int, name string) string {
	link, err := os.Readlink(fmt.Sprintf("/proc/%d/%s", pid, name))
	if err != nil {
		return ""
	}
	return link
}

func Run() error {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
