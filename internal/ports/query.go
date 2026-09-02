package ports

import (
	"strconv"
	"strings"
)

// Query is a parsed filter expression. Terms are whitespace separated and all
// must match (AND). A term may be negated with a leading '!' or '-'.
//
//	8080            port starts with 8080, or PID is 8080
//	:8080           port is exactly 8080
//	3000-4000       port in range
//	>1024  <1024    port comparison (also >=, <=)
//	tcp  udp        protocol
//	listen  estab   state (prefix match: time_wait, close_wait, syn...)
//	exposed         listening on a non-loopback interface
//	local           bound to loopback only
//	docker          running in a container or forwarded by docker-proxy
//	pid:123         exact PID
//	user:root       owning user
//	proc:nginx      process name contains
//	svc:http        service name contains
//	anything else   substring of process, command, service, user, address, container
type Query struct {
	raw   string
	terms []term
}

type term struct {
	negate bool
	match  func(*PortInfo) bool
}

// ParseQuery parses a filter expression. An empty string matches everything.
func ParseQuery(s string) Query {
	q := Query{raw: s}
	for _, word := range strings.Fields(s) {
		t := term{}
		if len(word) > 1 && (word[0] == '!' || (word[0] == '-' && !isDigit(word[1]))) {
			t.negate = true
			word = word[1:]
		}
		t.match = parseTerm(word)
		q.terms = append(q.terms, t)
	}
	return q
}

// String returns the original expression.
func (q Query) String() string { return q.raw }

// Empty reports whether the query has no terms.
func (q Query) Empty() bool { return len(q.terms) == 0 }

// Match reports whether an entry satisfies every term.
func (q Query) Match(e *PortInfo) bool {
	for _, t := range q.terms {
		if t.match(e) == t.negate {
			return false
		}
	}
	return true
}

// Filter returns the entries matching the query, preserving order.
func (q Query) Filter(entries []PortInfo) []PortInfo {
	if q.Empty() {
		return entries
	}
	out := make([]PortInfo, 0, len(entries))
	for i := range entries {
		if q.Match(&entries[i]) {
			out = append(out, entries[i])
		}
	}
	return out
}

// ExactPort returns the port when the query is a single exact-port term
// (e.g. "8080" or ":8080"), which lets callers give a friendlier "port is free"
// message when nothing matches.
func (q Query) ExactPort() (uint16, bool) {
	fields := strings.Fields(q.raw)
	if len(fields) != 1 {
		return 0, false
	}
	p, err := strconv.ParseUint(strings.TrimPrefix(fields[0], ":"), 10, 16)
	if err != nil {
		return 0, false
	}
	return uint16(p), true
}

func parseTerm(word string) func(*PortInfo) bool {
	lower := strings.ToLower(word)

	// Keywords.
	switch lower {
	case "tcp", "udp":
		proto := strings.ToUpper(lower)
		return func(e *PortInfo) bool { return e.Protocol == proto }
	case "listen", "listening", "open":
		return func(e *PortInfo) bool { return e.IsListening() }
	case "estab", "established", "connected":
		return func(e *PortInfo) bool { return e.State == "ESTABLISHED" }
	case "exposed", "public", "external":
		return func(e *PortInfo) bool { return e.Exposed }
	case "local", "loopback", "localhost":
		return func(e *PortInfo) bool { return IsLoopback(e.Address) }
	case "docker", "container", "containers":
		return func(e *PortInfo) bool { return e.Container != "" || e.Forward != "" }
	case "unknown", "hidden":
		return func(e *PortInfo) bool { return e.PID == 0 }
	case "ipv6", "v6":
		return func(e *PortInfo) bool { return e.IPv6 }
	case "ipv4", "v4":
		return func(e *PortInfo) bool { return e.IPv4 }
	}

	// Prefixed terms.
	if k, v, ok := strings.Cut(lower, ":"); ok {
		switch k {
		case "":
			if p, err := strconv.ParseUint(v, 10, 16); err == nil {
				return func(e *PortInfo) bool { return uint64(e.Port) == p }
			}
		case "port":
			if m := parsePortTerm(v); m != nil {
				return m
			}
			if p, err := strconv.ParseUint(v, 10, 16); err == nil {
				return func(e *PortInfo) bool { return uint64(e.Port) == p }
			}
		case "pid", "p":
			if n, err := strconv.Atoi(v); err == nil {
				return func(e *PortInfo) bool { return e.PID == n }
			}
		case "user", "u":
			return func(e *PortInfo) bool { return strings.ToLower(e.User) == v }
		case "proc", "process", "name", "n":
			return func(e *PortInfo) bool { return contains(e.Process, v) || contains(e.Cmdline, v) }
		case "svc", "service":
			return func(e *PortInfo) bool { return contains(e.Service, v) }
		case "state", "s":
			return func(e *PortInfo) bool { return stateMatches(e.State, v) }
		case "addr", "address", "bind", "ip":
			return func(e *PortInfo) bool { return contains(e.Address, v) || contains(e.RemoteAddress, v) }
		case "remote", "peer":
			return func(e *PortInfo) bool { return contains(e.Remote(), v) }
		}
	}

	// Port comparisons and ranges.
	if m := parsePortTerm(lower); m != nil {
		return m
	}

	// Bare number: port prefix or exact PID.
	if isNumber(lower) {
		n, _ := strconv.Atoi(lower)
		return func(e *PortInfo) bool {
			return strings.HasPrefix(strconv.Itoa(int(e.Port)), lower) || (n > 0 && e.PID == n)
		}
	}

	// TCP state names.
	if stateLike(lower) {
		return func(e *PortInfo) bool { return stateMatches(e.State, lower) }
	}

	// Free text.
	return func(e *PortInfo) bool {
		return contains(e.Process, lower) || contains(e.Cmdline, lower) ||
			contains(e.Service, lower) || contains(e.User, lower) ||
			contains(e.Address, lower) || contains(e.RemoteAddress, lower) ||
			contains(e.Container, lower) || contains(e.Forward, lower)
	}
}

// parsePortTerm handles "3000-4000", ">1024", "<=443". Returns nil if the word
// is not a port expression.
func parsePortTerm(w string) func(*PortInfo) bool {
	for _, op := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(w, op) {
			n, err := strconv.ParseUint(w[len(op):], 10, 16)
			if err != nil {
				return nil
			}
			switch op {
			case ">=":
				return func(e *PortInfo) bool { return uint64(e.Port) >= n }
			case "<=":
				return func(e *PortInfo) bool { return uint64(e.Port) <= n }
			case ">":
				return func(e *PortInfo) bool { return uint64(e.Port) > n }
			case "<":
				return func(e *PortInfo) bool { return uint64(e.Port) < n }
			default:
				return func(e *PortInfo) bool { return uint64(e.Port) == n }
			}
		}
	}
	if lo, hi, ok := strings.Cut(w, "-"); ok && isNumber(lo) && isNumber(hi) {
		l, err1 := strconv.ParseUint(lo, 10, 16)
		h, err2 := strconv.ParseUint(hi, 10, 16)
		if err1 != nil || err2 != nil {
			return nil
		}
		if l > h {
			l, h = h, l
		}
		return func(e *PortInfo) bool { return uint64(e.Port) >= l && uint64(e.Port) <= h }
	}
	return nil
}

func stateLike(w string) bool {
	w = strings.ReplaceAll(w, "-", "_")
	for _, s := range []string{"listen", "established", "syn_sent", "syn_recv", "fin_wait1",
		"fin_wait2", "time_wait", "close", "close_wait", "last_ack", "closing", "unconn"} {
		if strings.HasPrefix(s, w) {
			return true
		}
	}
	return false
}

func stateMatches(state, w string) bool {
	w = strings.ReplaceAll(strings.ToLower(w), "-", "_")
	return strings.HasPrefix(strings.ToLower(state), w)
}

func contains(haystack, needle string) bool {
	return needle != "" && strings.Contains(strings.ToLower(haystack), needle)
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}
