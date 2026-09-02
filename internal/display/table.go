// Package display renders port listings for non-interactive use: an aligned
// coloured table for humans, and JSON/CSV/plain for machines.
package display

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Viljoen13/port-explorer/internal/ports"
	"github.com/Viljoen13/port-explorer/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

// Options controls table rendering.
type Options struct {
	ShowRemote bool // include the REMOTE column (for --all)
	Wide       bool // include USER, UPTIME and COMMAND columns
}

type column struct {
	title string
	right bool
	value func(*ports.PortInfo) string
	style func(*ports.PortInfo) lipgloss.Style
}

func columns(opts Options) []column {
	plain := func(*ports.PortInfo) lipgloss.Style { return theme.Plain }
	muted := func(*ports.PortInfo) lipgloss.Style { return theme.Muted }
	cols := []column{
		{title: "PORT", right: true, value: func(e *ports.PortInfo) string { return strconv.Itoa(int(e.Port)) },
			style: func(*ports.PortInfo) lipgloss.Style { return theme.PortNum }},
		{title: "PROTO", value: func(e *ports.PortInfo) string { return strings.ToLower(e.Protocol) + family(e) }, style: muted},
		{title: "SERVICE", value: func(e *ports.PortInfo) string { return e.Service },
			style: func(*ports.PortInfo) lipgloss.Style { return theme.Service }},
		{title: "PID", right: true, value: func(e *ports.PortInfo) string { return orDash(pidString(e)) }, style: muted},
		{title: "PROCESS", value: func(e *ports.PortInfo) string { return orDash(e.Process) },
			style: func(e *ports.PortInfo) lipgloss.Style {
				if e.Process == "" {
					return theme.Muted
				}
				return theme.Bold
			}},
	}
	if opts.Wide {
		cols = append(cols,
			column{title: "USER", value: func(e *ports.PortInfo) string { return e.User }, style: muted},
			column{title: "UPTIME", right: true, value: func(e *ports.PortInfo) string { return ports.FormatDuration(e.Uptime()) }, style: muted},
		)
	}
	cols = append(cols, column{title: "STATE", value: func(e *ports.PortInfo) string { return e.State },
		style: func(e *ports.PortInfo) lipgloss.Style { return theme.State(e.State) }})
	cols = append(cols, column{title: "BIND", value: func(e *ports.PortInfo) string { return e.BindLabel() },
		style: func(e *ports.PortInfo) lipgloss.Style {
			if e.Exposed {
				return theme.Exposed
			}
			return theme.Plain
		}})
	if opts.ShowRemote {
		cols = append(cols, column{title: "REMOTE", value: func(e *ports.PortInfo) string { return e.Remote() }, style: plain})
	}
	cols = append(cols, column{title: "", value: Tags, style: func(e *ports.PortInfo) lipgloss.Style {
		switch {
		case e.Exposed:
			return theme.Exposed
		case e.Container != "" || e.Forward != "":
			return theme.Container
		}
		return theme.Muted
	}})
	if opts.Wide {
		cols = append(cols, column{title: "COMMAND", value: func(e *ports.PortInfo) string { return e.Cmdline }, style: muted})
	}
	return cols
}

// Tags summarises noteworthy facts about an entry: exposure, container, forwarding.
func Tags(e *ports.PortInfo) string {
	var tags []string
	if e.Exposed {
		tags = append(tags, "⚠ exposed")
	}
	if e.Container != "" {
		tags = append(tags, "◧ "+e.Container)
	}
	if e.Forward != "" {
		tags = append(tags, "→ "+e.Forward)
	}
	return strings.Join(tags, "  ")
}

func family(e *ports.PortInfo) string {
	switch {
	case e.IPv4 && e.IPv6:
		return "46"
	case e.IPv6:
		return "6"
	default:
		return ""
	}
}

func pidString(e *ports.PortInfo) string {
	if e.PID <= 0 {
		return ""
	}
	return strconv.Itoa(e.PID)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// PrintTable writes an aligned, coloured table.
func PrintTable(w io.Writer, entries []ports.PortInfo, opts Options) {
	if len(entries) == 0 {
		return
	}
	cols := columns(opts)
	widths := make([]int, len(cols))
	cells := make([][]string, len(entries))
	for i, c := range cols {
		widths[i] = len(c.title)
	}
	for r := range entries {
		cells[r] = make([]string, len(cols))
		for i, c := range cols {
			v := c.value(&entries[r])
			cells[r][i] = v
			if l := lipgloss.Width(v); l > widths[i] {
				widths[i] = l
			}
		}
	}
	// Drop trailing columns that are entirely empty (e.g. no tags anywhere).
	for len(cols) > 0 && widths[len(cols)-1] == 0 {
		cols, widths = cols[:len(cols)-1], widths[:len(widths)-1]
	}

	var b strings.Builder
	for i, c := range cols {
		if i > 0 {
			b.WriteString("  ")
		}
		if c.title == "" {
			continue
		}
		if i == len(cols)-1 {
			b.WriteString(theme.Header.Render(c.title))
			continue
		}
		b.WriteString(theme.Header.Render(pad(c.title, widths[i], c.right)))
	}
	fmt.Fprintln(w, strings.TrimRight(b.String(), " "))

	for r := range entries {
		b.Reset()
		for i, c := range cols {
			if i > 0 {
				b.WriteString("  ")
			}
			cell := pad(cells[r][i], widths[i], c.right)
			if i == len(cols)-1 {
				cell = strings.TrimRight(cell, " ")
			}
			if strings.TrimSpace(cell) == "" {
				b.WriteString(cell)
				continue
			}
			b.WriteString(c.style(&entries[r]).Render(cell))
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}
}

func pad(s string, width int, right bool) string {
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	if right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

// PrintJSON writes entries as an indented JSON array.
func PrintJSON(w io.Writer, entries []ports.PortInfo) error {
	if entries == nil {
		entries = []ports.PortInfo{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

// PrintCSV writes entries as CSV with a header row.
func PrintCSV(w io.Writer, entries []ports.PortInfo) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"port", "protocol", "service", "pid", "process", "user", "state", "address", "remote", "exposed", "container", "forward", "cmdline"}); err != nil {
		return err
	}
	for i := range entries {
		e := &entries[i]
		if err := cw.Write([]string{
			strconv.Itoa(int(e.Port)), e.Protocol, e.Service, pidString(e), e.Process, e.User,
			e.State, e.Address, e.Remote(), strconv.FormatBool(e.Exposed), e.Container, e.Forward, e.Cmdline,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// PrintPlain writes tab-separated rows with no header or colour, ideal for
// awk and cut: port, protocol, pid, process, state, address.
func PrintPlain(w io.Writer, entries []ports.PortInfo) {
	for i := range entries {
		e := &entries[i]
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", e.Port, e.Protocol, orDash(pidString(e)), orDash(e.Process), e.State, e.Address)
	}
}

// Summary renders a one-line coloured summary of the listing.
func Summary(entries []ports.PortInfo, privileged bool) string {
	s := ports.Summarize(entries)
	parts := []string{theme.Pill(fmt.Sprintf("%d listening", s.Listening), theme.Green)}
	if s.Established > 0 {
		parts = append(parts, theme.Pill(fmt.Sprintf("%d established", s.Established), theme.Blue))
	}
	if other := s.Total - s.Listening - s.Established; other > 0 {
		parts = append(parts, theme.Muted.Render(fmt.Sprintf("%d other", other)))
	}
	if s.Exposed > 0 {
		parts = append(parts, theme.Pill(fmt.Sprintf("%d exposed to the network", s.Exposed), theme.Red))
	}
	if s.Containers > 0 {
		parts = append(parts, theme.Pill(fmt.Sprintf("%d in containers", s.Containers), theme.Cyan))
	}
	line := strings.Join(parts, theme.Faint.Render("  ·  "))
	if s.Hidden > 0 && !privileged {
		line += "\n" + theme.Warning.Render(fmt.Sprintf("%d socket(s) belong to other users — run with sudo to see their processes.", s.Hidden))
	}
	return line
}
