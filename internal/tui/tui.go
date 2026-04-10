package tui

import (
	"fmt"
	"os"
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
)

type view int

const (
	viewList view = iota
	viewDetail
)

type Model struct {
	entries    []ports.PortInfo
	filtered  []ports.PortInfo
	cursor    int
	offset    int
	height    int
	width     int
	view      view
	filter    string
	filtering bool
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
		// Handle confirm dialog
		if m.confirm {
			return m.handleConfirm(msg)
		}

		// Handle filter input
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
		if m.cursor < len(m.filtered) {
			entry := m.filtered[m.cursor]
			if entry.PID > 0 {
				err := syscall.Kill(entry.PID, syscall.SIGTERM)
				if err != nil {
					m.message = errorStyle.Render(fmt.Sprintf("Failed to kill PID %d: %v", entry.PID, err))
				} else {
					m.message = successStyle.Render(fmt.Sprintf("Sent SIGTERM to %s (PID %d)", entry.Process, entry.PID))
				}
				return m, refresh
			}
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
		if m.view == viewList && m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.message = ""
		}

	case "enter", "right", "l":
		if m.view == viewList && len(m.filtered) > 0 {
			m.view = viewDetail
			m.message = ""
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

	case "d", "x":
		if len(m.filtered) > 0 {
			entry := m.filtered[m.cursor]
			if entry.PID > 0 {
				m.confirm = true
				m.message = confirmStyle.Render(fmt.Sprintf("Kill %s (PID %d) on port %d? (y/n)", entry.Process, entry.PID, entry.Port))
			} else {
				m.message = errorStyle.Render("No PID available — try running with sudo")
			}
		}

	case "home", "g":
		m.cursor = 0
		m.message = ""

	case "end", "G":
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
		m.message = ""
	}

	return m, nil
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
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
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
		return m.renderList()
	}
}

func (m Model) renderList() string {
	var b strings.Builder

	// Title bar
	title := titleStyle.Render(" port-explorer ")
	count := dimStyle.Render(fmt.Sprintf(" %d ports", len(m.filtered)))
	b.WriteString(title + count + "\n\n")

	// Filter bar
	if m.filtering {
		b.WriteString(filterPromptStyle.Render("filter: ") + filterInputStyle.Render(m.filter) + "█\n\n")
	} else if m.filter != "" {
		b.WriteString(dimStyle.Render(fmt.Sprintf("filter: %s (press / to change, esc to clear)", m.filter)) + "\n\n")
	}

	// Column header
	header := dimStyle.Render(fmt.Sprintf("  %-7s %-6s %-8s %-20s %-14s %s", "PORT", "PROTO", "PID", "PROCESS", "STATE", "ADDRESS"))
	b.WriteString(header + "\n")

	// Calculate visible range
	listHeight := m.height - 8 // reserve for title, header, footer, etc.
	if m.filtering || m.filter != "" {
		listHeight -= 2
	}
	if listHeight < 3 {
		listHeight = 3
	}

	// Scroll offset
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
		pidStr := "-"
		if e.PID > 0 {
			pidStr = strconv.Itoa(e.PID)
		}
		process := e.Process
		if process == "" {
			process = "-"
		}

		state := styleState(e.State)

		line := fmt.Sprintf("%-7d %-6s %-8s %-20s %-14s %s",
			e.Port, e.Protocol, pidStr, truncate(process, 20), state, e.Address)

		if i == m.cursor {
			b.WriteString(selectedStyle.Render("▸ "+line) + "\n")
		} else {
			b.WriteString(normalStyle.Render("  "+line) + "\n")
		}
	}

	// Message / status bar
	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(m.message + "\n")
	}

	// Help bar
	help := "↑/↓ navigate • enter view details • d kill • / filter • r refresh • q quit"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func (m Model) renderDetail() string {
	if m.cursor >= len(m.filtered) {
		return "No selection"
	}

	e := m.filtered[m.cursor]
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

	// Show other connections from the same process
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

	// Process details from /proc if available
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
	// cmdline uses null bytes as separators
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
