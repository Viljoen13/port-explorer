# Changelog

## Unreleased

A ground-up rework of the dashboard and data model.

### Added
- Rich socket model: remote peer, owning user, well-known service name, command line, executable, working directory, parent PID, start time / uptime.
- Exposure analysis: sockets reachable from the network are flagged **⚠ exposed**; loopback, link-local and multicast binds are not.
- Container awareness: processes inside Docker/Podman containers show the container name; `docker-proxy` rows show their forwarding target.
- IPv4/IPv6 twins of the same socket are merged into one row (`tcp46`).
- Filter language shared by the CLI and the dashboard: `8080`, `:8080`, `3000-4000`, `>1024`, `tcp`, `estab`, `exposed`, `docker`, `pid:`, `user:`, `proc:`, `svc:`, `remote:`, negation with `!`.
- Dashboard: summary header, responsive columns, sort (`s`/`S`), listening/all toggle (`a`), live mode with new-socket highlighting (`w`), clipboard copy (`c`/`C`), help overlay (`?`), redesigned detail view with connections and sibling ports, graceful/force kill confirmation that verifies the process exited.
- `free [start]` subcommand: find the next free port(s), ideal for scripts.
- `wait <port>` subcommand: block until a port opens (or closes) with `--timeout`.
- Output formats `--output table|json|csv|plain`, `--wide`, `--sort`, `--desc`, `--no-color`, `--quiet`.
- Exit code 1 when a filter matches nothing, so `port-explorer -q 8080 && …` works.
- `--version`, Makefile, GitHub Actions CI on Linux/macOS/Windows, GoReleaser release pipeline.
- Unit tests for parsing, filtering, merging, sorting, rendering and dashboard behaviour.

### Fixed
- Windows build was broken (`syscall.Kill` does not exist there); killing now uses `taskkill`.
- macOS: UDP sockets were dropped by the column parser; switched to `lsof -F` field output, which also survives command names containing spaces.
- IPv4-mapped IPv6 peers (`::ffff:a.b.c.d`) are shown as plain IPv4.
- Listening sockets no longer report a bogus `0.0.0.0` remote address in JSON.
- Refusing to kill `port-explorer` itself; clearer message when the owner is hidden and `sudo` is needed.

## 0.1.0

- Initial release: interactive port list, detail view, kill, group by process, `--list`/`--json` output.
