package display

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/Viljoen13/port-explorer/internal/ports"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	listenStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green
	estabStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // blue
	waitStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	otherStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // gray
)

func stateStyle(state string) lipgloss.Style {
	switch state {
	case "LISTEN":
		return listenStyle
	case "ESTABLISHED":
		return estabStyle
	case "TIME_WAIT", "CLOSE_WAIT", "FIN_WAIT1", "FIN_WAIT2":
		return waitStyle
	default:
		return otherStyle
	}
}

func PrintTable(w io.Writer, entries []PortInfo) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "No ports found.")
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Port < entries[j].Port
	})

	headers := []string{"PORT", "PROTO", "PID", "PROCESS", "STATE", "ADDRESS"}

	rows := make([][]string, len(entries))
	for i, e := range entries {
		style := stateStyle(e.State)
		pidStr := "-"
		if e.PID > 0 {
			pidStr = strconv.Itoa(e.PID)
		}
		process := e.Process
		if process == "" {
			process = "-"
		}
		rows[i] = []string{
			strconv.Itoa(int(e.Port)),
			e.Protocol,
			pidStr,
			process,
			style.Render(e.State),
			e.Address,
		}
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("8"))).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return lipgloss.NewStyle()
		})

	fmt.Fprintln(w, t)
}

func PrintJSON(w io.Writer, entries []PortInfo) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// Re-export PortInfo so display consumers don't need to import ports package directly.
type PortInfo = ports.PortInfo

func FormatSummary(entries []PortInfo) string {
	listening := 0
	established := 0
	for _, e := range entries {
		switch e.State {
		case "LISTEN":
			listening++
		case "ESTABLISHED":
			established++
		}
	}
	parts := []string{fmt.Sprintf("%d total", len(entries))}
	if listening > 0 {
		parts = append(parts, fmt.Sprintf("%d listening", listening))
	}
	if established > 0 {
		parts = append(parts, fmt.Sprintf("%d established", established))
	}
	return strings.Join(parts, ", ")
}
