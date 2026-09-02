// Package tui implements the interactive dashboard.
package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Viljoen13/port-explorer/internal/ports"
	tea "github.com/charmbracelet/bubbletea"
)

// Options configures the dashboard at start-up.
type Options struct {
	Query   string        // initial filter expression
	ShowAll bool          // start with every socket, not only listening ones
	Refresh time.Duration // auto-refresh interval when live mode is on
}

type viewMode int

const (
	viewList viewMode = iota
	viewDetail
	viewHelp
)

type rowKind int

const (
	rowEntry rowKind = iota
	rowGroup
)

// group bundles every socket held by one process.
type group struct {
	key       string
	name      string
	pid       int
	user      string
	container string
	entries   []ports.PortInfo
	stats     ports.Stats
	expanded  bool
}

type row struct {
	kind  rowKind
	entry ports.PortInfo // valid when kind == rowEntry
	group *group         // valid for both kinds (nil in flat mode)
}

type msgLevel int

const (
	levelInfo msgLevel = iota
	levelSuccess
	levelWarn
	levelError
)

type confirmKill struct {
	pid     int
	process string
	port    uint16
	count   int // ports held, when killing from a group header
	force   bool
}

// Model is the Bubble Tea model for the dashboard.
type Model struct {
	opts Options

	all     []ports.PortInfo // every socket, merged across IP families
	visible []ports.PortInfo // after listening/all toggle, filter and sort
	groups  []*group
	rows    []row

	expanded map[string]bool // group expansion state, keyed by group.key
	seen     map[string]time.Time

	cursor int
	offset int
	width  int
	height int

	mode      viewMode
	query     string
	filtering bool
	showAll   bool
	grouped   bool
	sortIdx   int
	sortAsc   bool
	live      bool
	tickSeq   int

	message   string
	msgLevel  msgLevel
	msgAt     time.Time
	confirm   *confirmKill
	err       error
	loaded    bool
	lastFetch time.Time
	fetching  bool

	privileged bool
	selfPID    int

	detail       ports.PortInfo
	detailKey    string
	detailGone   bool
	detailScroll int
	helpScroll   int
	helpFrom     viewMode

	killedPID  int
	killedName string
	killedAt   time.Time
	killForce  bool
}

type refreshMsg struct {
	entries []ports.PortInfo
	err     error
	at      time.Time
}

type tickMsg struct {
	seq int
}

func fetch() tea.Msg {
	entries, err := ports.List()
	return refreshMsg{entries: ports.Merge(entries), err: err, at: time.Now()}
}

func fetchAfter(d time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(d)
		return fetch()
	}
}

func (m Model) tick() tea.Cmd {
	seq := m.tickSeq
	return tea.Tick(m.opts.Refresh, func(time.Time) tea.Msg { return tickMsg{seq: seq} })
}

// New creates a dashboard model.
func New(opts Options) Model {
	if opts.Refresh <= 0 {
		opts.Refresh = 2 * time.Second
	}
	return Model{
		opts:       opts,
		width:      100,
		height:     30,
		query:      strings.TrimSpace(opts.Query),
		showAll:    opts.ShowAll,
		sortAsc:    true,
		expanded:   map[string]bool{},
		seen:       map[string]time.Time{},
		privileged: ports.IsPrivileged(),
		selfPID:    os.Getpid(),
	}
}

// Init starts the first scan.
func (m Model) Init() tea.Cmd {
	return fetch
}

// Update handles events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampCursor()
		return m, nil

	case refreshMsg:
		return m.onRefresh(msg)

	case tickMsg:
		if !m.live || msg.seq != m.tickSeq {
			return m, nil
		}
		if m.message != "" && time.Since(m.msgAt) > 6*time.Second {
			m.message = ""
		}
		var cmds []tea.Cmd
		if !m.fetching {
			m.fetching = true
			cmds = append(cmds, fetch)
		}
		cmds = append(cmds, m.tick())
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch {
		case m.confirm != nil:
			return m.onConfirmKey(msg)
		case m.filtering:
			return m.onFilterKey(msg)
		case m.mode == viewHelp:
			return m.onHelpKey(msg)
		case m.mode == viewDetail:
			return m.onDetailKey(msg)
		default:
			return m.onListKey(msg)
		}
	}
	return m, nil
}

func (m Model) onRefresh(msg refreshMsg) (tea.Model, tea.Cmd) {
	m.fetching = false
	if msg.err != nil {
		if !m.loaded {
			m.err = msg.err
		} else {
			m.setMessage(levelError, "refresh failed: "+msg.err.Error())
		}
		return m, nil
	}
	m.err = nil
	m.all = msg.entries
	m.lastFetch = msg.at

	present := make(map[string]bool, len(m.all))
	for i := range m.all {
		k := m.all[i].Key()
		present[k] = true
		if _, ok := m.seen[k]; !ok {
			if m.loaded {
				m.seen[k] = msg.at
			} else {
				m.seen[k] = time.Time{}
			}
		}
	}
	for k := range m.seen {
		if !present[k] {
			delete(m.seen, k)
		}
	}
	m.loaded = true
	m.rebuild(true)
	m.refreshDetail()

	var cmd tea.Cmd
	if m.killedPID != 0 {
		alive := false
		for i := range m.all {
			if m.all[i].PID == m.killedPID {
				alive = true
				break
			}
		}
		elapsed := msg.at.Sub(m.killedAt)
		switch {
		case !alive:
			m.setMessage(levelSuccess, fmt.Sprintf("%s (PID %d) has exited", m.killedName, m.killedPID))
			m.killedPID = 0
		case elapsed > 3*time.Second:
			if m.killForce {
				m.setMessage(levelError, fmt.Sprintf("%s (PID %d) survived SIGKILL — is it a zombie or in uninterruptible I/O?", m.killedName, m.killedPID))
			} else {
				m.setMessage(levelWarn, fmt.Sprintf("%s (PID %d) is still running — press D to force kill", m.killedName, m.killedPID))
			}
			m.killedPID = 0
		default:
			cmd = fetchAfter(500 * time.Millisecond)
		}
	}
	return m, cmd
}

// rebuild recomputes visible entries, groups and rows from m.all.
// When keepSelection is set it tries to leave the cursor on the same socket.
func (m *Model) rebuild(keepSelection bool) {
	selected := ""
	if keepSelection {
		selected = m.selectedKey()
	}

	src := m.all
	if !m.showAll {
		src = ports.Listening(src)
	}
	m.visible = ports.ParseQuery(m.query).Filter(src)
	ports.Sort(m.visible, ports.SortFields[m.sortIdx], m.sortAsc)

	m.rows = m.rows[:0]
	if m.grouped {
		m.buildGroups()
		for _, g := range m.groups {
			m.rows = append(m.rows, row{kind: rowGroup, group: g})
			if g.expanded {
				for _, e := range g.entries {
					m.rows = append(m.rows, row{kind: rowEntry, entry: e, group: g})
				}
			}
		}
	} else {
		m.groups = nil
		for _, e := range m.visible {
			m.rows = append(m.rows, row{kind: rowEntry, entry: e})
		}
	}

	if selected != "" {
		for i := range m.rows {
			if m.rowKey(i) == selected {
				m.cursor = i
				break
			}
		}
	}
	m.clampCursor()
}

func (m *Model) buildGroups() {
	byKey := map[string]*group{}
	m.groups = m.groups[:0]
	for _, e := range m.visible {
		key := "pid:" + strconv.Itoa(e.PID)
		if e.PID == 0 {
			key = "unknown"
		}
		g, ok := byKey[key]
		if !ok {
			name := e.Process
			if e.PID == 0 {
				name = "unknown process"
			} else if name == "" {
				name = "pid " + strconv.Itoa(e.PID)
			}
			g = &group{key: key, name: name, pid: e.PID, user: e.User, container: e.Container, expanded: m.expanded[key]}
			byKey[key] = g
			m.groups = append(m.groups, g)
		}
		g.entries = append(g.entries, e)
	}
	for _, g := range m.groups {
		g.stats = ports.Summarize(g.entries)
	}
}

func (m *Model) rowKey(i int) string {
	if i < 0 || i >= len(m.rows) {
		return ""
	}
	r := m.rows[i]
	if r.kind == rowGroup {
		return "group:" + r.group.key
	}
	return r.entry.Key()
}

func (m *Model) selectedKey() string { return m.rowKey(m.cursor) }

func (m *Model) selectedRow() *row {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m *Model) clampCursor() {
	if m.cursor > len(m.rows)-1 {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *Model) move(delta int) {
	m.cursor += delta
	m.clampCursor()
	if !m.filtering {
		m.message = ""
	}
}

func (m *Model) setMessage(level msgLevel, text string) {
	m.message = text
	m.msgLevel = level
	m.msgAt = time.Now()
}

func (m *Model) toggleGroup(g *group) {
	g.expanded = !g.expanded
	m.expanded[g.key] = g.expanded
	m.rebuild(false)
}

func (m *Model) refreshDetail() {
	if m.mode != viewDetail {
		return
	}
	for i := range m.all {
		if m.all[i].Key() == m.detailKey {
			m.detail = m.all[i]
			m.detailGone = false
			return
		}
	}
	m.detailGone = true
}

// ---- key handling ---------------------------------------------------------

func (m Model) onListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup", "ctrl+u":
		m.move(-m.listHeight())
	case "pgdown", "ctrl+d":
		m.move(m.listHeight())
	case "home":
		m.move(-len(m.rows))
	case "end", "G":
		m.move(len(m.rows))

	case "enter", "right", "l":
		r := m.selectedRow()
		if r == nil {
			break
		}
		if r.kind == rowGroup {
			m.toggleGroup(r.group)
		} else {
			m.openDetail(r.entry)
		}
	case " ":
		if r := m.selectedRow(); r != nil && r.group != nil {
			if r.kind == rowEntry {
				m.cursor = m.groupHeaderIndex(r.group)
			}
			m.toggleGroup(r.group)
		}

	case "esc", "left", "h":
		r := m.selectedRow()
		switch {
		case r != nil && r.group != nil && (r.kind == rowEntry || r.group.expanded):
			m.cursor = m.groupHeaderIndex(r.group)
			if r.group.expanded {
				m.toggleGroup(r.group)
			}
		case m.query != "":
			m.query = ""
			m.rebuild(true)
			m.setMessage(levelInfo, "filter cleared")
		}

	case "/":
		m.filtering = true
		m.message = ""

	case "g":
		m.grouped = !m.grouped
		m.rebuild(true)
		if m.grouped {
			m.setMessage(levelInfo, "grouped by process — enter expands, esc collapses")
		} else {
			m.message = ""
		}

	case "a":
		m.showAll = !m.showAll
		m.rebuild(true)
		if m.showAll {
			m.setMessage(levelInfo, "showing every socket, including established connections")
		} else {
			m.setMessage(levelInfo, "showing listening sockets only")
		}

	case "s":
		m.sortIdx = (m.sortIdx + 1) % len(ports.SortFields)
		m.rebuild(true)
		m.setMessage(levelInfo, "sorted by "+string(ports.SortFields[m.sortIdx]))
	case "S":
		m.sortAsc = !m.sortAsc
		m.rebuild(true)
		dir := "ascending"
		if !m.sortAsc {
			dir = "descending"
		}
		m.setMessage(levelInfo, "sorted by "+string(ports.SortFields[m.sortIdx])+" "+dir)

	case "w":
		m.live = !m.live
		m.tickSeq++
		if m.live {
			m.setMessage(levelInfo, fmt.Sprintf("live mode on — refreshing every %s, new sockets are highlighted", m.opts.Refresh))
			return m, m.tick()
		}
		m.setMessage(levelInfo, "live mode off")

	case "r":
		if !m.fetching {
			m.fetching = true
			m.setMessage(levelInfo, "refreshing…")
			return m, fetch
		}

	case "d", "x":
		m.askKill(false)
	case "D", "X":
		m.askKill(true)

	case "c", "y":
		if r := m.selectedRow(); r != nil {
			if r.kind == rowGroup {
				m.copyText(strconv.Itoa(r.group.pid), "PID")
			} else {
				m.copyText(strconv.Itoa(int(r.entry.Port)), "port")
			}
		}
	case "C", "Y":
		if r := m.selectedRow(); r != nil {
			e := r.entry
			if r.kind == rowGroup && len(r.group.entries) > 0 {
				e = r.group.entries[0]
			}
			if e.Cmdline != "" {
				m.copyText(e.Cmdline, "command line")
			} else if e.PID > 0 {
				m.copyText(strconv.Itoa(e.PID), "PID")
			}
		}

	case "?":
		m.openHelp()
	}
	return m, nil
}

func (m *Model) openHelp() {
	m.helpFrom = m.mode
	m.helpScroll = 0
	m.mode = viewHelp
}

func (m *Model) groupHeaderIndex(g *group) int {
	for i := range m.rows {
		if m.rows[i].kind == rowGroup && m.rows[i].group == g {
			return i
		}
	}
	return m.cursor
}

func (m *Model) openDetail(e ports.PortInfo) {
	m.detail = e
	m.detailKey = e.Key()
	m.detailGone = false
	m.detailScroll = 0
	m.mode = viewDetail
	m.message = ""
}

func (m *Model) askKill(force bool) {
	var c confirmKill
	if m.mode == viewDetail {
		c = confirmKill{pid: m.detail.PID, process: m.detail.Process, port: m.detail.Port}
	} else {
		r := m.selectedRow()
		if r == nil {
			return
		}
		if r.kind == rowGroup {
			c = confirmKill{pid: r.group.pid, process: r.group.name, count: len(r.group.entries)}
		} else {
			c = confirmKill{pid: r.entry.PID, process: r.entry.Process, port: r.entry.Port}
		}
	}
	switch {
	case c.pid <= 0:
		hint := "the owner is not visible"
		if !m.privileged {
			hint += " — rerun with sudo"
		}
		m.setMessage(levelError, "can't kill: "+hint)
		return
	case c.pid == m.selfPID:
		m.setMessage(levelWarn, "that's port-explorer itself — press q to quit instead")
		return
	}
	if c.process == "" {
		c.process = "process"
	}
	c.force = force
	m.confirm = &c
	m.message = ""
}

func (m Model) onConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := m.confirm
	switch msg.String() {
	case "y", "Y", "enter":
		return m.doKill(c, c.force)
	case "f", "F", "K":
		return m.doKill(c, true)
	case "n", "N", "esc", "q", "ctrl+c":
		m.confirm = nil
	}
	return m, nil
}

func (m Model) doKill(c *confirmKill, force bool) (tea.Model, tea.Cmd) {
	m.confirm = nil
	if err := ports.Kill(c.pid, force); err != nil {
		m.setMessage(levelError, err.Error())
		return m, nil
	}
	m.killedPID = c.pid
	m.killedName = c.process
	m.killedAt = time.Now()
	m.killForce = force
	m.setMessage(levelInfo, fmt.Sprintf("sent %s to %s (PID %d)…", ports.SignalName(force), c.process, c.pid))
	return m, fetchAfter(300 * time.Millisecond)
}

func (m Model) onFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filtering = false
		m.query = strings.TrimSpace(m.query)
	case "esc":
		m.filtering = false
		m.query = ""
		m.rebuild(true)
	case "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if len(m.query) > 0 {
			runes := []rune(m.query)
			m.query = string(runes[:len(runes)-1])
			m.rebuild(true)
		}
	case "ctrl+u", "ctrl+w":
		m.query = ""
		m.rebuild(true)
	case "up":
		m.move(-1)
	case "down":
		m.move(1)
	case "tab":
		// Cycle through handy presets.
		presets := []string{"exposed", "docker", "estab", "udp", ">1024", ""}
		next := presets[0]
		for i, p := range presets {
			if p == m.query && i+1 < len(presets) {
				next = presets[i+1]
			}
		}
		m.query = next
		m.rebuild(true)
	default:
		switch msg.Type {
		case tea.KeyRunes:
			m.query += string(msg.Runes)
			m.rebuild(true)
		case tea.KeySpace:
			m.query += " "
		}
	}
	return m, nil
}

func (m Model) onDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "left", "h", "backspace", "enter":
		m.mode = viewList
		m.message = ""
	case "up", "k":
		if m.detailScroll > 0 {
			m.detailScroll--
		}
	case "down", "j":
		m.detailScroll++
	case "pgup":
		m.detailScroll = max(0, m.detailScroll-10)
	case "pgdown":
		m.detailScroll += 10
	case "home":
		m.detailScroll = 0
	case "d", "x":
		m.askKill(false)
	case "D", "X":
		m.askKill(true)
	case "c", "y":
		m.copyText(strconv.Itoa(int(m.detail.Port)), "port")
	case "C", "Y":
		if m.detail.Cmdline != "" {
			m.copyText(m.detail.Cmdline, "command line")
		}
	case "r":
		if !m.fetching {
			m.fetching = true
			return m, fetch
		}
	case "?":
		m.openHelp()
	}
	return m, nil
}

func (m Model) onHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.helpScroll > 0 {
			m.helpScroll--
		}
	case "down", "j":
		m.helpScroll++
	default:
		m.mode = m.helpFrom
	}
	return m, nil
}

func (m *Model) copyText(text, what string) {
	if err := copyToClipboard(text); err != nil {
		m.setMessage(levelError, "copy failed: "+err.Error())
		return
	}
	shown := text
	if len(shown) > 40 {
		shown = shown[:39] + "…"
	}
	m.setMessage(levelSuccess, fmt.Sprintf("copied %s to clipboard: %s", what, shown))
}

// Run starts the dashboard and blocks until the user quits.
func Run(opts Options) error {
	p := tea.NewProgram(New(opts), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
