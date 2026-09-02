// Package theme holds the colour palette and reusable styles shared by the
// interactive UI and the plain CLI output, so both look like one product.
package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// Palette: every colour adapts to light and dark terminals.
var (
	Accent  = lipgloss.AdaptiveColor{Light: "#6D28D9", Dark: "#A78BFA"}
	Green   = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	Blue    = lipgloss.AdaptiveColor{Light: "#1D4ED8", Dark: "#60A5FA"}
	Yellow  = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
	Red     = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	Magenta = lipgloss.AdaptiveColor{Light: "#BE185D", Dark: "#F472B6"}
	Cyan    = lipgloss.AdaptiveColor{Light: "#0E7490", Dark: "#22D3EE"}
	Dim     = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#6B7280"}
	Subtle  = lipgloss.AdaptiveColor{Light: "#9CA3AF", Dark: "#4B5563"}
	Text    = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#E5E7EB"}
	OnAcc   = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#111827"}
	Surface = lipgloss.AdaptiveColor{Light: "#EDE9FE", Dark: "#2E1065"}
)

var (
	Title     = lipgloss.NewStyle().Bold(true).Foreground(OnAcc).Background(Accent).Padding(0, 1)
	Brand     = lipgloss.NewStyle().Bold(true).Foreground(Accent)
	Header    = lipgloss.NewStyle().Bold(true).Foreground(Dim)
	Rule      = lipgloss.NewStyle().Foreground(Subtle)
	Muted     = lipgloss.NewStyle().Foreground(Dim)
	Faint     = lipgloss.NewStyle().Foreground(Subtle)
	Plain     = lipgloss.NewStyle().Foreground(Text)
	Bold      = lipgloss.NewStyle().Bold(true).Foreground(Text)
	Key       = lipgloss.NewStyle().Bold(true).Foreground(Accent)
	Selected  = lipgloss.NewStyle().Bold(true).Foreground(OnAcc).Background(Accent)
	Success   = lipgloss.NewStyle().Bold(true).Foreground(Green)
	Warning   = lipgloss.NewStyle().Bold(true).Foreground(Yellow)
	Error     = lipgloss.NewStyle().Bold(true).Foreground(Red)
	Exposed   = lipgloss.NewStyle().Bold(true).Foreground(Red)
	Container = lipgloss.NewStyle().Foreground(Cyan)
	Service   = lipgloss.NewStyle().Foreground(Magenta)
	PortNum   = lipgloss.NewStyle().Bold(true).Foreground(Text)
	New       = lipgloss.NewStyle().Bold(true).Foreground(Green)
	Label     = lipgloss.NewStyle().Bold(true).Foreground(Blue)
	Box       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Accent).Padding(0, 1)
	DangerBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Red).Padding(0, 1)

	stateListen = lipgloss.NewStyle().Bold(true).Foreground(Green)
	stateUnconn = lipgloss.NewStyle().Foreground(Green)
	stateEstab  = lipgloss.NewStyle().Foreground(Blue)
	stateWait   = lipgloss.NewStyle().Foreground(Yellow)
	stateOther  = lipgloss.NewStyle().Foreground(Dim)
)

// State returns the style for a socket state.
func State(state string) lipgloss.Style {
	switch state {
	case "LISTEN":
		return stateListen
	case "UNCONN":
		return stateUnconn
	case "ESTABLISHED":
		return stateEstab
	case "TIME_WAIT", "CLOSE_WAIT", "FIN_WAIT1", "FIN_WAIT2", "CLOSING", "LAST_ACK", "SYN_SENT", "SYN_RECV":
		return stateWait
	default:
		return stateOther
	}
}

// Pill renders a compact "N label" badge in the given colour.
func Pill(text string, color lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render(text)
}

// Accentish returns a style in the accent colour, for cursors and markers.
func Accentish() lipgloss.Style { return lipgloss.NewStyle().Foreground(Accent) }
