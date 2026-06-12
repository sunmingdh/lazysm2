# lazysm2

`lazysm2` is a terminal UI for [hmrc/sm2](https://github.com/hmrc/sm2), built in the style of tools like `lazygit` and `lazydocker`.

It keeps the normal `sm2` command-line workflow, but puts the day-to-day actions in a navigable TUI: inspect profiles, see running services, start or stop services, restart with a chosen version or port, view debug output, and follow `stdout.log` without leaving the terminal.

## Features

- Three navigation panels for profiles, running services, and stopped services.
- Profile-aware stopped-service list: selecting a profile shows only services from that profile that are not already running.
- Details panel for the selected profile or service.
- Right-hand panel for profile contents, `sm2 --debug` output, or live service logs.
- Streaming action popup for start and restart commands.
- Start or restart with an explicit service version and optional port.
- Per-panel filtering.
- Keyboard and mouse navigation.
- Automatic status refresh after service actions.
- Online/offline start mode indicator based on VPN detection or `START_SERVICE_OFFLINE`.

## Requirements

- Go, matching the version in [go.mod](go.mod).
- `sm2` installed and available on your `PATH`.
- A configured `sm2` workspace, including `service-manager-config`.

Check your `sm2` installation with:

```sh
sm2 --diagnostic
```

## Run

From this repository:

```sh
go run .
```

Build a local binary:

```sh
go build -o lazysm2 .
./lazysm2
```

Install from this repository:

```sh
go install
lazysm2
```

The binary is installed to `$(go env GOPATH)/bin`; make sure that directory is on your `PATH`.

## How It Works

`lazysm2` shells out to `sm2` rather than replacing it:

- `sm2 --status` loads running service state.
- `sm2 --list` loads profiles and their services.
- `sm2 --start SERVICE[:VERSION] [--port PORT]` starts services.
- `sm2 --stop SERVICE` stops services.
- `sm2 --debug SERVICE` populates the debug panel and discovers the service log directory.

When logs are opened, the app reads `stdout.log` from the directory reported by `sm2 --debug`. It initially tails the latest lines, then polls for appended lines and follows the bottom until you scroll away.

## Keybindings

| Key | Action |
| --- | --- |
| `tab` | Cycle between profiles, running services, and stopped services |
| `1`, `2`, `3` | Jump directly to a panel |
| `j` / `down` | Move selection down |
| `k` / `up` | Move selection up |
| `f` | Filter the active panel |
| `enter` / `esc` | Leave filter or version/port input mode |
| `s` | Start selected service or profile |
| `S` | Start selected service with version and port input |
| `r` | Restart selected service |
| `R` | Restart selected service with version and port input |
| `x` | Stop selected service or profile |
| `l` | Open logs for the selected service |
| `PgUp` / `PgDn` | Scroll the debug/log panel |
| `G` | Jump to bottom and resume log following |
| `?` | Show help |
| `q` / `ctrl+c` | Quit |

Mouse support is also enabled for selecting and scrolling the left-hand panels. Scrolling the log panel pauses follow mode; pressing `G` resumes it.

## Debug Logging

By default no log file is written. Pass `--debug-log` to write a log file to the OS cache directory:

```sh
lazysm2 --debug-log
```

The log is written to:
- **macOS**: `~/Library/Caches/lazysm2/lazysm2.log`
- **Linux**: `~/.cache/lazysm2/lazysm2.log`

## Start Mode

Starts are run online when a VPN-like network interface is detected. If no VPN interface is detected, `lazysm2` adds `--offline` to `sm2 --start`.

To force offline starts regardless of VPN state:

```sh
START_SERVICE_OFFLINE=true go run .
```

Truthy values are `1`, `true`, `yes`, `on`, and `y`.

## Development

Run the test suite:

```sh
go test ./...
```

The code is split into two main packages:

- [sm2](sm2/runner.go): command execution and parsing for `sm2` output.
- [tui](tui/model.go): Bubble Tea model, event handling, layout, popups, filtering, and log following.

The TUI uses Charmbracelet libraries:

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the event loop.
- [Bubbles](https://github.com/charmbracelet/bubbles) for the viewport.
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) for layout and styling.
