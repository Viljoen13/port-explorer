# port-explorer

A friendly, cross-platform CLI tool to see what's running on your ports.

Like `lsof -i` or `netstat -tlnp`, but interactive and actually readable.

## Features

- **Interactive TUI** — browse ports, view details, kill processes
- **Cross-platform** — works on Linux, macOS, and Windows
- **Filter & search** — by port, port range, or process name
- **Kill processes** — directly from the interface with confirmation
- **Detail view** — see full command line, working directory, and related ports
- **Scriptable** — JSON and table output modes for pipelines

## Install

```bash
go install github.com/Viljoen13/port-explorer@latest
```

Or build from source:

```bash
git clone https://github.com/Viljoen13/port-explorer.git
cd port-explorer
go build -o port-explorer .
```

## Usage

### Interactive mode (default)

```bash
port-explorer
```

| Key | Action |
|---|---|
| `↑/↓` or `j/k` | Navigate |
| `Enter` or `→` | View port details |
| `Esc` or `←` | Go back |
| `/` | Search / filter |
| `d` | Kill process on selected port |
| `r` | Refresh |
| `q` | Quit |

### Non-interactive mode

```bash
# Table output
port-explorer --list

# Filter by port
port-explorer 8080

# Filter by port range
port-explorer 3000-9000

# Filter by process name
port-explorer --process nginx

# JSON output (great for scripting)
port-explorer --json

# Show all connections, not just listening
port-explorer --all

# Kill a process on a port
port-explorer kill 8080
port-explorer kill 8080 --force --yes
```

## Why?

Every developer has typed some variant of `lsof -iTCP -sTCP:LISTEN -P -n` or `netstat -tlnp` and squinted at the output. port-explorer gives you one command that works the same on every OS, with an interactive interface for browsing and managing ports.

## License

MIT
