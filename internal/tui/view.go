package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Viljoen13/port-explorer/internal/display"
	"github.com/Viljoen13/port-explorer/internal/ports"
	"github.com/Viljoen13/port-explorer/internal/theme"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const newHighlight = 6 * time.Second

// View renders the whole screen.
func (m Model) View() string {
	if m.err != nil {
		body := theme.Error.Render("Could not read the socket table") + "\n\n" +
			theme.Plain.Render(m.err.Error()) + "\n\n" +
			theme.Muted.Render("Press q to quit or r to retry.")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, theme.DangerBox.Render(body))
	}
	switch m.mode {
	case viewHelp:
		return m.renderHelp()
	case viewDetail:
		return m.renderDetail()
	}
	return m.renderList()
}

// ---- layout ---------------------------------------------------------------

// chromeHeight is the number of lines used around the list.
func (m Model) chromeHeight() int {
	h := 6 // header, rule, status, column header, message, footer
	if m.confirm != nil {
		h += 3 // bordered confirm box replaces the message line
	}
	return h
}

func (m Model) listHeight() int {
	return max(3, m.height-m.chromeHeight())
}

type column struct {
	title string
	width int
	right bool
	value func(*ports.PortInfo) string
	style func(*ports.PortInfo) lipgloss.Style
}

// columns picks and sizes columns for the current terminal width.
func (m Model) columns() []column {
	avail := m.width - 2 // row prefix
	muted := func(*ports.PortInfo) lipgloss.Style { return theme.Muted }
	plain := func(*ports.PortInfo) lipgloss.Style { return theme.Plain }

	cols := []column{
		{title: "PORT", width: 6, right: true,
			value: func(e *ports.PortInfo) string { return strconv.Itoa(int(e.Port)) },
			style: func(*ports.PortInfo) lipgloss.Style { return theme.PortNum }},
		{title: "PROTO", width: 5, value: proto, style: muted},
	}
	if avail >= 96 {
		cols = append(cols, column{title: "SERVICE", width: 13,
			value: func(e *ports.PortInfo) string { return e.Service },
			style: func(*ports.PortInfo) lipgloss.Style { return theme.Service }})
	}
	cols = append(cols, column{title: "PID", width: 7, right: true,
		value: func(e *ports.PortInfo) string {
			if e.PID <= 0 {
				return "-"
			}
			return strconv.Itoa(e.PID)
		}, style: muted})
	processIdx := len(cols)
	cols = append(cols, column{title: "PROCESS", width: 0,
		value: func(e *ports.PortInfo) string {
			if e.Process == "" {
				return "?"
			}
			return e.Process
		},
		style: func(e *ports.PortInfo) lipgloss.Style {
			if e.Process == "" {
				return theme.Muted
			}
			return theme.Bold
		}})
	if avail >= 112 {
		cols = append(cols, column{title: "USER", width: 12,
			value: func(e *ports.PortInfo) string { return e.User }, style: muted})
	}
	stateWidth, bindWidth := 11, 15
	if avail < 72 {
		stateWidth, bindWidth = 7, 10
	}
	cols = append(cols, column{title: "STATE", width: stateWidth,
		value: func(e *ports.PortInfo) string { return e.State },
		style: func(e *ports.PortInfo) lipgloss.Style { return theme.State(e.State) }})
	cols = append(cols, column{title: "BIND", width: bindWidth,
		value: func(e *ports.PortInfo) string { return e.BindLabel() },
		style: func(e *ports.PortInfo) lipgloss.Style {
			if e.Exposed {
				return theme.Exposed
			}
			return theme.Plain
		}})
	if m.showAll && avail >= 100 {
		cols = append(cols, column{title: "REMOTE", width: 21,
			value: func(e *ports.PortInfo) string { return e.Remote() }, style: plain})
	}
	if avail >= 128 {
		cols = append(cols, column{title: "UPTIME", width: 7, right: true,
			value: func(e *ports.PortInfo) string { return ports.FormatDuration(e.Uptime()) }, style: muted})
	}
	tagsIdx := len(cols)
	cols = append(cols, column{title: "", width: 0, value: m.tags,
		style: func(e *ports.PortInfo) lipgloss.Style {
			switch {
			case m.isNew(e):
				return theme.New
			case e.Exposed:
				return theme.Exposed
			case e.Container != "" || e.Forward != "":
				return theme.Container
			}
			return theme.Muted
		}})

	fixed := 0
	for i, c := range cols {
		if i != processIdx && i != tagsIdx {
			fixed += c.width
		}
	}
	fixed += 2 * (len(cols) - 1)
	remaining := avail - fixed
	process := min(26, max(6, remaining-14))
	cols[processIdx].width = process
	cols[tagsIdx].width = max(0, remaining-process)
	return cols
}

func proto(e *ports.PortInfo) string {
	p := strings.ToLower(e.Protocol)
	if e.IPv6 && !e.IPv4 {
		p += "6"
	}
	return p
}

func (m Model) isNew(e *ports.PortInfo) bool {
	t, ok := m.seen[e.Key()]
	return ok && !t.IsZero() && time.Since(t) < newHighlight
}

func (m Model) tags(e *ports.PortInfo) string {
	tags := display.Tags(e)
	if m.isNew(e) {
		if tags == "" {
			return "● new"
		}
		return "● new  " + tags
	}
	return tags
}

// ---- list view ------------------------------------------------------------

func (m Model) renderList() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteByte('\n')
	b.WriteString(theme.Rule.Render(strings.Repeat("─", max(0, m.width))))
	b.WriteByte('\n')
	b.WriteString(m.renderStatusLine())
	b.WriteByte('\n')

	cols := m.columns()
	b.WriteString(m.renderColumnHeader(cols))
	b.WriteByte('\n')

	height := m.listHeight()
	end := min(len(m.rows), m.offset+height)
	lines := 0
	if !m.loaded {
		b.WriteString(theme.Muted.Render("  scanning sockets…"))
		b.WriteByte('\n')
		lines++
	} else if len(m.rows) == 0 {
		b.WriteString(m.renderEmpty())
		b.WriteByte('\n')
		lines++
	}
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderRow(i, cols))
		b.WriteByte('\n')
		lines++
	}
	for ; lines < height; lines++ {
		b.WriteByte('\n')
	}

	if m.confirm != nil {
		b.WriteString(m.renderConfirm())
	} else {
		b.WriteString(m.renderMessage())
	}
	b.WriteByte('\n')
	b.WriteString(m.renderFooter(listKeys))
	return b.String()
}

func (m Model) renderHeader() string {
	left := theme.Title.Render("◆ port-explorer")

	// Pills in priority order; the least important are shed first when the
	// terminal is narrow.
	var pills []string
	if m.live {
		pills = append(pills, theme.Success.Render("● live "+m.opts.Refresh.String()))
	} else if m.fetching {
		pills = append(pills, theme.Muted.Render("↻ refreshing"))
	}
	if m.loaded {
		s := ports.Summarize(m.all)
		pills = append(pills, theme.Pill(fmt.Sprintf("%d listening", s.Listening), theme.Green))
		if s.Exposed > 0 {
			pills = append(pills, theme.Pill(fmt.Sprintf("%d exposed", s.Exposed), theme.Red))
		}
		if s.Hidden > 0 && !m.privileged {
			pills = append(pills, theme.Pill(fmt.Sprintf("%d hidden · sudo", s.Hidden), theme.Yellow))
		}
		if s.Established > 0 {
			pills = append(pills, theme.Pill(fmt.Sprintf("%d established", s.Established), theme.Blue))
		}
		if s.Containers > 0 {
			pills = append(pills, theme.Pill(fmt.Sprintf("%d containers", s.Containers), theme.Cyan))
		}
	}
	sep := theme.Faint.Render("  ·  ")
	for len(pills) > 0 {
		right := strings.Join(pills, sep)
		if lipgloss.Width(left)+lipgloss.Width(right)+2 <= m.width {
			return joinEnds(left, right, m.width)
		}
		pills = pills[:len(pills)-1]
	}
	return joinEnds(left, "", m.width)
}

func (m Model) renderStatusLine() string {
	var left string
	switch {
	case m.filtering:
		left = theme.Key.Render("/ ") + theme.Plain.Render(m.query) + theme.Accentish().Render("▏") +
			theme.Faint.Render("  enter apply · esc clear · tab presets")
	case m.query != "":
		left = theme.Muted.Render("filter: ") + theme.Bold.Render(m.query) + theme.Faint.Render("   (esc clears)")
	default:
		left = theme.Faint.Render("press / to filter — try  8080  nginx  exposed  docker  >1024")
	}

	var parts []string
	if m.showAll {
		parts = append(parts, "all sockets")
	} else {
		parts = append(parts, "listening")
	}
	dir := "↑"
	if !m.sortAsc {
		dir = "↓"
	}
	parts = append(parts, "sort "+string(ports.SortFields[m.sortIdx])+" "+dir)
	if m.grouped {
		parts = append(parts, fmt.Sprintf("%d processes", len(m.groups)))
	}
	if len(m.rows) > 0 {
		end := min(len(m.rows), m.offset+m.listHeight())
		parts = append(parts, fmt.Sprintf("%d–%d of %d", m.offset+1, end, len(m.rows)))
	} else if m.loaded {
		parts = append(parts, "0 rows")
	}
	right := theme.Muted.Render(strings.Join(parts, "  ·  "))
	return joinEnds(left, right, m.width)
}

func (m Model) renderColumnHeader(cols []column) string {
	var b strings.Builder
	b.WriteString("  ")
	for i, c := range cols {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(theme.Header.Render(fit(c.title, c.width, c.right)))
	}
	return ansi.Truncate(b.String(), m.width, "")
}

func (m Model) renderEmpty() string {
	switch {
	case m.query != "" && !m.showAll:
		return theme.Muted.Render(fmt.Sprintf("  nothing matches %q among listening sockets — press a to include established connections", m.query))
	case m.query != "":
		return theme.Muted.Render(fmt.Sprintf("  nothing matches %q", m.query))
	case !m.showAll:
		return theme.Muted.Render("  nothing is listening — press a to see all sockets")
	}
	return theme.Muted.Render("  no sockets found")
}

func (m Model) renderRow(i int, cols []column) string {
	r := m.rows[i]
	selected := i == m.cursor
	if r.kind == rowGroup {
		return m.renderGroupRow(r.group, selected)
	}
	e := &r.entry
	var cells []string
	for _, c := range cols {
		cells = append(cells, fit(c.value(e), c.width, c.right))
	}
	prefix := "  "
	if selected {
		prefix = "▸ "
	}
	if r.group != nil {
		// Nested under a group header: indent, and give the room back from the
		// flexible tags column so the row still fits the terminal.
		prefix += "  "
		last := len(cells) - 1
		cells[last] = fit(strings.TrimRight(cells[last], " "), max(0, cols[last].width-2), false)
	}
	if selected {
		line := ansi.Truncate(prefix+strings.Join(cells, "  "), m.width, "")
		return theme.Selected.Width(m.width).Render(line)
	}
	var b strings.Builder
	b.WriteString(prefix)
	for j, c := range cols {
		if j > 0 {
			b.WriteString("  ")
		}
		b.WriteString(c.style(e).Render(cells[j]))
	}
	return ansi.Truncate(b.String(), m.width, "")
}

func (m Model) renderGroupRow(g *group, selected bool) string {
	arrow := "▸"
	if g.expanded {
		arrow = "▾"
	}
	name := g.name
	meta := []string{}
	if g.pid > 0 {
		meta = append(meta, "PID "+strconv.Itoa(g.pid))
	}
	if g.user != "" {
		meta = append(meta, g.user)
	}
	if g.container != "" {
		meta = append(meta, "◧ "+g.container)
	}
	var counts []string
	if g.stats.Listening > 0 {
		counts = append(counts, fmt.Sprintf("%d listening", g.stats.Listening))
	}
	if g.stats.Established > 0 {
		counts = append(counts, fmt.Sprintf("%d established", g.stats.Established))
	}
	if other := g.stats.Total - g.stats.Listening - g.stats.Established; other > 0 {
		counts = append(counts, fmt.Sprintf("%d other", other))
	}
	exposed := ""
	if g.stats.Exposed > 0 {
		exposed = fmt.Sprintf("⚠ %d exposed", g.stats.Exposed)
	}
	portList := ""
	if !g.expanded {
		var ps []string
		seen := map[uint16]bool{}
		for _, e := range g.entries {
			if e.IsListening() && !seen[e.Port] {
				seen[e.Port] = true
				ps = append(ps, ":"+strconv.Itoa(int(e.Port)))
			}
		}
		portList = strings.Join(ps, " ")
	}

	prefix := "  "
	if selected {
		prefix = "▸ "
	}
	if selected {
		parts := []string{arrow + " " + name}
		if len(meta) > 0 {
			parts = append(parts, strings.Join(meta, " · "))
		}
		parts = append(parts, strings.Join(counts, ", "))
		if exposed != "" {
			parts = append(parts, exposed)
		}
		if portList != "" {
			parts = append(parts, portList)
		}
		line := ansi.Truncate(prefix+strings.Join(parts, "   "), m.width, "…")
		return theme.Selected.Width(m.width).Render(line)
	}
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString(theme.Brand.Render(arrow + " " + name))
	if len(meta) > 0 {
		b.WriteString(theme.Muted.Render("   " + strings.Join(meta, " · ")))
	}
	b.WriteString(theme.Muted.Render("   " + strings.Join(counts, ", ")))
	if exposed != "" {
		b.WriteString("   " + theme.Exposed.Render(exposed))
	}
	if portList != "" {
		b.WriteString("   " + theme.Faint.Render(portList))
	}
	return ansi.Truncate(b.String(), m.width, "…")
}

func (m Model) renderMessage() string {
	if m.message == "" {
		return ""
	}
	var style lipgloss.Style
	switch m.msgLevel {
	case levelSuccess:
		style = theme.Success
	case levelWarn:
		style = theme.Warning
	case levelError:
		style = theme.Error
	default:
		style = theme.Muted
	}
	return " " + style.Render(ansi.Truncate(m.message, m.width-2, "…"))
}

func (m Model) renderConfirm() string {
	c := m.confirm
	var what string
	if c.count > 0 {
		what = fmt.Sprintf("Kill %s (PID %d) holding %d sockets?", c.process, c.pid, c.count)
	} else {
		what = fmt.Sprintf("Kill %s (PID %d) on port %d?", c.process, c.pid, c.port)
	}
	var keys string
	if c.force {
		keys = theme.Key.Render("y") + theme.Plain.Render(" SIGKILL   ") +
			theme.Key.Render("n") + theme.Plain.Render(" cancel")
	} else {
		keys = theme.Key.Render("y") + theme.Plain.Render(" SIGTERM (graceful)   ") +
			theme.Key.Render("f") + theme.Plain.Render(" SIGKILL (force)   ") +
			theme.Key.Render("n") + theme.Plain.Render(" cancel")
	}
	body := theme.Error.Render(what) + "\n" + keys
	return theme.DangerBox.Render(body)
}

type keyHint struct{ key, label string }

var listKeys = []keyHint{
	{"↑↓", "move"}, {"⏎", "details"}, {"/", "filter"}, {"g", "group"}, {"a", "all"},
	{"s", "sort"}, {"w", "live"}, {"d", "kill"}, {"c", "copy"}, {"?", "help"}, {"q", "quit"},
}

var detailKeys = []keyHint{
	{"esc", "back"}, {"↑↓", "scroll"}, {"d", "kill"}, {"D", "force kill"}, {"c", "copy port"},
	{"C", "copy command"}, {"r", "refresh"}, {"q", "quit"},
}

func (m Model) renderFooter(keys []keyHint) string {
	var parts []string
	for _, k := range keys {
		parts = append(parts, theme.Key.Render(k.key)+" "+theme.Muted.Render(k.label))
	}
	return " " + ansi.Truncate(strings.Join(parts, "  "), m.width-1, "…")
}

// ---- detail view ----------------------------------------------------------

func (m Model) renderDetail() string {
	e := &m.detail
	var b strings.Builder

	title := fmt.Sprintf(":%d", e.Port)
	if e.Service != "" {
		title += " " + e.Service
	}
	crumb := theme.Title.Render("◆ port-explorer") + theme.Muted.Render(" › ") + theme.Brand.Render(title)
	if e.Process != "" {
		crumb += theme.Muted.Render("  ·  ") + theme.Bold.Render(e.Process)
	}
	status := ""
	if m.detailGone {
		status = theme.Warning.Render("socket has gone away")
	} else if m.live {
		status = theme.Success.Render("● live")
	}
	b.WriteString(joinEnds(crumb, status, m.width))
	b.WriteByte('\n')
	b.WriteString(theme.Rule.Render(strings.Repeat("─", max(0, m.width))))
	b.WriteByte('\n')

	content := m.detailLines(e)
	bodyHeight := max(3, m.height-4) // crumb, rule, message, footer
	scroll := m.detailScroll
	if scroll > max(0, len(content)-bodyHeight) {
		scroll = max(0, len(content)-bodyHeight)
	}
	end := min(len(content), scroll+bodyHeight)
	for i := scroll; i < end; i++ {
		b.WriteString(content[i])
		b.WriteByte('\n')
	}
	for i := end - scroll; i < bodyHeight; i++ {
		b.WriteByte('\n')
	}

	if m.confirm != nil {
		// Keep the confirm box compact in the detail view: overwrite the last lines.
		b.WriteString(m.renderConfirm())
	} else {
		b.WriteString(m.renderMessage())
	}
	b.WriteByte('\n')
	b.WriteString(m.renderFooter(detailKeys))
	return b.String()
}

func (m Model) detailLines(e *ports.PortInfo) []string {
	var lines []string
	section := func(title string) {
		lines = append(lines, "")
		rule := strings.Repeat("─", max(0, m.width-lipgloss.Width(title)-5))
		lines = append(lines, theme.Rule.Render(" ── ")+theme.Label.Render(title)+theme.Rule.Render(" "+rule))
	}
	field := func(label, value string) {
		if value == "" {
			return
		}
		lines = append(lines, wrapField(label, value, m.width))
	}

	section("Endpoint")
	portLabel := strconv.Itoa(int(e.Port))
	if e.Service != "" {
		portLabel += "  " + theme.Service.Render(e.Service)
	}
	field("Port", portLabel)
	fam := "IPv4"
	switch {
	case e.IPv4 && e.IPv6:
		fam = "IPv4 + IPv6"
	case e.IPv6:
		fam = "IPv6"
	}
	field("Protocol", e.Protocol+"  "+theme.Muted.Render(fam))
	field("State", theme.State(e.State).Render(e.State))
	bind := e.BindLabel()
	if bind != e.Address && e.Address != "" {
		bind += theme.Muted.Render("  (" + e.Address + ")")
	}
	field("Bind", bind)
	field("Remote", e.Remote())
	switch {
	case e.Exposed:
		field("Exposure", theme.Exposed.Render("⚠ reachable from the network"))
	case e.IsListening():
		field("Exposure", theme.Success.Render("local only — bound to loopback"))
	}

	section("Process")
	if e.PID == 0 {
		owner := "another user"
		if e.User != "" {
			owner = e.User
		}
		msg := fmt.Sprintf("Owner not visible — this socket belongs to %s.", owner)
		if !m.privileged {
			msg += " Rerun with sudo to identify the process."
		}
		lines = append(lines, "   "+theme.Warning.Render(msg))
	} else {
		field("Name", theme.Bold.Render(orDash(e.Process)))
		pid := strconv.Itoa(e.PID)
		if e.PPID > 0 {
			pid += theme.Muted.Render("  (parent " + strconv.Itoa(e.PPID) + ")")
		}
		field("PID", pid)
		field("User", e.User)
		if e.StartedAt != nil {
			field("Started", ports.FormatDuration(e.Uptime())+" ago"+theme.Muted.Render("  ("+e.StartedAt.Local().Format("2006-01-02 15:04:05")+")"))
		}
		if e.Container != "" {
			field("Container", theme.Container.Render("◧ "+e.Container))
		}
		if e.Forward != "" {
			field("Forwards to", theme.Container.Render("→ "+e.Forward))
		}
		field("Executable", e.Exe)
		field("Directory", e.Cwd)
		field("Command", e.Cmdline)
	}

	if e.PID > 0 {
		var conns, others []ports.PortInfo
		for i := range m.all {
			o := &m.all[i]
			if o.PID != e.PID {
				continue
			}
			switch {
			case o.State == "ESTABLISHED":
				conns = append(conns, *o)
			case o.IsListening() && (o.Port != e.Port || o.Protocol != e.Protocol):
				others = append(others, *o)
			}
		}
		if len(conns) > 0 {
			section(fmt.Sprintf("Connections (%d established)", len(conns)))
			ports.Sort(conns, ports.SortPort, true)
			for i, c := range conns {
				if i == 12 {
					lines = append(lines, theme.Muted.Render(fmt.Sprintf("   … and %d more", len(conns)-i)))
					break
				}
				lines = append(lines, fmt.Sprintf("   %s :%-6d %s %s",
					theme.State("ESTABLISHED").Render("●"), c.Port, theme.Muted.Render("⇄"), c.Remote()))
			}
		}
		if len(others) > 0 {
			section("Other ports held by this process")
			ports.Sort(others, ports.SortPort, true)
			for _, o := range others {
				svc := ""
				if o.Service != "" {
					svc = theme.Service.Render("  " + o.Service)
				}
				exp := ""
				if o.Exposed {
					exp = theme.Exposed.Render("  ⚠ exposed")
				}
				lines = append(lines, fmt.Sprintf("   %s %-6d %s %s%s%s",
					theme.State(o.State).Render("●"), o.Port, theme.Muted.Render(strings.ToLower(o.Protocol)),
					o.BindLabel(), svc, exp))
			}
		}
	}
	return lines
}

func wrapField(label, value string, width int) string {
	lab := theme.Label.Render(fit(label, 12, false))
	indent := 3 + 12 + 2
	avail := max(20, width-indent)
	if lipgloss.Width(value) <= avail {
		return "   " + lab + "  " + value
	}
	// Wrap long plain values (command lines) at word boundaries.
	var out []string
	line := ""
	for _, w := range strings.Fields(value) {
		if line != "" && lipgloss.Width(line)+1+lipgloss.Width(w) > avail {
			out = append(out, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += w
	}
	if line != "" {
		out = append(out, line)
	}
	for i := range out {
		if i == 0 {
			out[i] = "   " + lab + "  " + out[i]
		} else {
			out[i] = strings.Repeat(" ", indent) + theme.Muted.Render(out[i])
		}
	}
	return strings.Join(out, "\n")
}

// ---- help view ------------------------------------------------------------

func (m Model) renderHelp() string {
	type entry struct{ key, desc string }
	sections := []struct {
		title string
		keys  []entry
	}{
		{"Navigate", []entry{
			{"↑ ↓  j k", "move"}, {"pgup pgdn", "page"}, {"home end", "jump to first / last"},
			{"⏎  →  l", "open details, or expand a group"}, {"esc  ←  h", "collapse group / clear filter"},
			{"space", "expand or collapse the group under the cursor"},
		}},
		{"Find", []entry{
			{"/", "filter as you type — terms are ANDed, ! negates"},
			{"", "8080  :8080  3000-4000  >1024  <=443"},
			{"", "tcp  udp  listen  estab  exposed  local  docker  unknown"},
			{"", "pid:123  user:root  proc:node  svc:http  remote:443  free text"},
			{"tab", "cycle filter presets while typing"},
			{"g", "group sockets by process"}, {"a", "toggle listening-only / all sockets"},
			{"s  S", "cycle sort column / flip direction"},
		}},
		{"Act", []entry{
			{"d  x", "kill the selected process (asks first; graceful SIGTERM)"},
			{"D  X", "force kill (SIGKILL)"},
			{"c  y", "copy port (or PID on a group) to the clipboard"},
			{"C  Y", "copy the full command line"},
			{"w", "live mode: auto-refresh and highlight new sockets"},
			{"r", "refresh now"}, {"?", "this help"}, {"q", "quit"},
		}},
		{"Reading the table", []entry{
			{"⚠ exposed", "listening on a non-loopback interface: reachable from the network"},
			{"◧ name", "process runs inside a container"},
			{"→ ip:port", "docker-proxy forwarding to a container"},
			{"● new", "appeared since the last refresh (live mode)"},
			{"?", "process hidden — belongs to another user; rerun with sudo"},
		}},
	}

	var lines []string
	lines = append(lines, theme.Brand.Render("port-explorer — keyboard reference"), "")
	for _, s := range sections {
		lines = append(lines, theme.Label.Render(s.title))
		for _, k := range s.keys {
			lines = append(lines, "  "+theme.Key.Render(fit(k.key, 12, false))+"  "+theme.Plain.Render(k.desc))
		}
		lines = append(lines, "")
	}
	lines = append(lines, theme.Muted.Render("press any key to go back"))

	bodyHeight := max(3, m.height-2)
	scroll := min(m.helpScroll, max(0, len(lines)-bodyHeight))
	end := min(len(lines), scroll+bodyHeight)
	body := strings.Join(lines[scroll:end], "\n")
	box := theme.Box.Render(body)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// ---- helpers --------------------------------------------------------------

// fit truncates or pads s to exactly width cells.
func fit(s string, width int, right bool) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) > width {
		s = ansi.Truncate(s, width, "…")
	}
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	if right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

// joinEnds places left and right on one line, right-aligned, dropping right
// when it does not fit.
func joinEnds(left, right string, width int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if rw == 0 || lw+rw+2 > width {
		return ansi.Truncate(left, width, "…")
	}
	return left + strings.Repeat(" ", width-lw-rw) + right
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
