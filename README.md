<h1 align="center">port-explorer</h1>

<p align="center">
  <b>See what's running on your ports — and do something about it.</b><br>
  A fast, friendly, cross-platform dashboard for sockets and the processes behind them.
</p>

<p align="center">
  <a href="https://github.com/Viljoen13/port-explorer/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/Viljoen13/port-explorer/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/Viljoen13/port-explorer/releases"><img alt="Release" src="https://img.shields.io/github/v/release/Viljoen13/port-explorer?include_prereleases"></a>
  <a href="LICENSE"><img alt="MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <img alt="Platforms" src="https://img.shields.io/badge/linux%20%7C%20macOS%20%7C%20windows-supported-success">
</p>

---

You know the drill. Something is squatting on port 3000, `lsof -iTCP -sTCP:LISTEN -P -n` produces a wall of text, and you still don't know *which* `node` it is or whether it's safe to kill.

`port-explorer` answers that in one keystroke:

```
 ◆ port-explorer                          14 listening  ·  3 exposed  ·  41 established  ·  2 containers          ● live 2s
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
 press / to filter — try  8080  nginx  exposed  docker  >1024                              listening  ·  sort port ↑  ·  1–9 of 9
    PORT  PROTO  SERVICE            PID  PROCESS            USER          STATE        BIND             UPTIME
▸     22  tcp    ssh                812  sshd               root          LISTEN       *                  3d4h  ⚠ exposed
      53  udp    dns                  -  ?                  systemd-res…  UNCONN       localhost
      80  tcp    http              1201  nginx              root          LISTEN       *                 2h15m  ⚠ exposed
     443  tcp    https             1201  nginx              root          LISTEN       *                 2h15m  ⚠ exposed
    3000  tcp    dev-server       44120  node               me            LISTEN       localhost           45s  ● new
    5173  tcp    vite             44088  node               me            LISTEN       localhost         3m12s
    5432  tcp    postgres           900  docker-proxy       root          LISTEN       *                 2h15m  → 172.18.0.2:5432
    6379  tcp    redis              955  redis-server       root          LISTEN       localhost         2h15m  ◧ cache-1
    8080  tcp    http-alt         43001  java               me            LISTEN       *                 1h02m  ⚠ exposed

 ↑↓ move  ⏎ details  / filter  g group  a all  s sort  w live  d kill  c copy  ? help  q quit
```

## Why you'll actually use it

- **Answers, not raw data.** Every row shows the service name, owning process, user, uptime, and whether the port is **exposed to the network** or bound to localhost only.
- **Container-aware.** Sockets inside Docker or Podman containers show the container name. `docker-proxy` rows show where they forward to.
- **Filter the way you think.** Type `/` then `8080`, `nginx`, `exposed`, `docker`, `>1024`, `user:root`, `!tcp`. Terms combine; `!` negates.
- **Kill with confidence.** `d` asks first, sends a graceful SIGTERM, then confirms the process actually exited. `D` force-kills. It refuses to kill itself and tells you when you need `sudo`.
- **Live mode.** `w` refreshes every two seconds and highlights sockets that just appeared. Watch your dev server come up.
- **Group by process.** `g` collapses the table into one row per process with a port summary. Expand what you care about.
- **Scriptable.** JSON, CSV and plain output. Exit code 1 when nothing matches. Plus `free` and `wait` subcommands built for shell scripts and CI.
- **One binary, three OSes.** Linux reads `/proc` directly (no external tools). macOS uses `lsof`. Windows uses `netstat` + PowerShell.

## Install

```sh
go install github.com/Viljoen13/port-explorer@latest
```

Or download a prebuilt binary from the [releases page](https://github.com/Viljoen13/port-explorer/releases), or build from source:

```sh
git clone https://github.com/Viljoen13/port-explorer.git
cd port-explorer
make build      # → ./port-explorer
```

> **Seeing `?` in the PROCESS column?** Sockets owned by other users are visible, but their process names need elevated permissions. Run `sudo port-explorer` to see everything. The dashboard tells you how many are hidden.

## Usage

### The dashboard

```sh
port-explorer                 # everything that's listening
port-explorer -a              # ...plus established connections
port-explorer -i exposed      # open the dashboard pre-filtered
```

| Key | Action |
|---|---|
| `↑` `↓` `j` `k` · `PgUp` `PgDn` · `Home` `End` | Move |
| `Enter` `→` | Open details, or expand a group |
| `Esc` `←` | Collapse group / clear filter |
| `/` | Filter as you type (`Tab` cycles presets) |
| `g` | Group by process |
| `a` | Toggle listening-only / all sockets |
| `s` `S` | Cycle sort column / flip direction |
| `w` | Live mode: auto-refresh, highlight new sockets |
| `d` `D` | Kill selected process (graceful / force), with confirmation |
| `c` `C` | Copy port / full command line to clipboard |
| `r` | Refresh now |
| `?` | Help |
| `q` | Quit |

The **detail view** (`Enter`) shows the full command line, working directory, executable, parent PID, start time, container, every established connection the process has, and every other port it holds.

### Filter language

Works identically in the dashboard (`/`) and on the command line. Terms are ANDed; prefix `!` or `-` to negate.

| Term | Matches |
|---|---|
| `8080` | ports starting with 8080, or PID 8080 |
| `:8080` | exactly port 8080 |
| `3000-4000` | port range |
| `>1024` `<=443` | port comparisons |
| `tcp` `udp` | protocol |
| `listen` `estab` `time_wait` … | socket state (prefix match) |
| `exposed` | listening on a non-loopback interface |
| `local` | bound to loopback |
| `docker` | in a container, or forwarded by docker-proxy |
| `unknown` | owner hidden (needs sudo) |
| `pid:123` `user:root` `proc:node` `svc:http` `addr:10.0` `remote:443` | field-specific |
| anything else | substring of process, command line, service, user, address, container |

### Quick answers on the command line

Any filter argument switches to table output:

```sh
port-explorer 8080                 # what's on 8080?
port-explorer 3000-9000            # anything in my dev range?
port-explorer nginx                # ports held by nginx
port-explorer exposed              # what can the network reach?
port-explorer -a estab remote:443  # outbound HTTPS connections
port-explorer -w docker            # --wide adds user, uptime, command
```

```
PORT  PROTO  SERVICE   PID  PROCESS  STATE   BIND
5432  tcp46  postgres  900  postgres LISTEN  localhost
6379  tcp    redis     955  redis    LISTEN  *          ⚠ exposed

2 listening  ·  1 exposed to the network
```

### Scripting

```sh
port-explorer -o json              # full detail, including cmdline, uptime, container
port-explorer -o csv > ports.csv
port-explorer -o plain | awk '$5=="LISTEN"{print $1}'

port-explorer -q 8080 && echo "busy" || echo "free"     # exit 1 = no match

PORT=$(port-explorer free 3000)    # first free port ≥ 3000
port-explorer free 8000 -n 3       # three of them

npm start &
port-explorer wait 3000 --timeout 30s && npx cypress run
port-explorer wait 5432 --closed   # wait for a port to be released

port-explorer kill 3000            # SIGTERM, asks first, confirms exit
port-explorer kill 3000 -f -y      # SIGKILL, no questions
port-explorer kill --pid 41234
```

Exit codes: `0` success/match, `1` nothing matched or timed out, `2` error.

## What "exposed" means

A socket is flagged **⚠ exposed** when it accepts traffic (TCP `LISTEN`, or a bound UDP socket) on an address other machines can reach: the wildcard (`*`, `0.0.0.0`, `::`) or a routable interface address. Loopback, link-local and multicast binds are not flagged. It is a quick sanity check, not a firewall audit — your firewall may still block the port.

## Platform notes

| | Source | Process details | Container detection |
|---|---|---|---|
| **Linux** | `/proc/net/{tcp,udp}{,6}` + `/proc/*/fd` | cmdline, cwd, exe, ppid, start time | cgroup → `docker ps` / `podman ps` for names |
| **macOS** | `lsof -F` (field output) | `ps`, `lsof -d cwd` | — |
| **Windows** | `netstat -ano` + `tasklist` | PowerShell `Get-CimInstance` | — |

Killing uses `SIGTERM`/`SIGKILL` on Unix and `taskkill` (`/F` for force) on Windows.

## Development

```sh
make test      # go test -race ./...
make lint      # gofmt + go vet on linux, darwin and windows
make cross     # build every platform into dist/
make preview   # render dashboard screens to text files without a terminal
```

Releases are cut by pushing a `v*` tag; GitHub Actions runs [GoReleaser](.goreleaser.yaml) and publishes binaries for Linux, macOS and Windows on amd64 and arm64.

## License

MIT — see [LICENSE](LICENSE).
