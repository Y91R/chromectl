# chromectl

**English** · [Русский](README.ru.md)

A CLI and a Claude Code plugin for controlling Chrome over the Chrome DevTools Protocol
(CDP): accessibility snapshots with stable uids, clicks and forms, screenshots, device
emulation, console and network inspection, performance traces, Lighthouse audits and
heap snapshots.

The only long-running process is Chrome itself, started with a debug port. Every
`chromectl` command connects, performs one action, detaches from the tab and exits —
no MCP server, no daemon, nothing left running in the background. Why, and what this
trades away: [ADR-0009](docs/adr/0009-cli-bez-demona.md) (in Russian).

## Requirements

- Chrome or Chromium (a non-standard location is set via `CHROME_PATH`)
- Go 1.26+ — the plugin builds the binary from source on first use
- Claude Code 2.1.265+ for the plugin (its `bin/` is added to `PATH`)
- Node.js — only for `chromectl audit lighthouse`

## Install as a Claude Code plugin

```
/plugin marketplace add Y91R/chromectl
/plugin install chromectl@chromectl
```

Skills provided by the plugin:

| Skill | Use it to |
|---|---|
| `/chromectl:browser` | open pages, take snapshots and screenshots, click, fill forms, run JS, wait for text |
| `/chromectl:browser-a11y` | audit accessibility: Lighthouse, semantics, labels, focus, contrast |
| `/chromectl:browser-lcp` | debug and optimize Largest Contentful Paint: trace, LCP element, network waterfall |
| `/chromectl:browser-troubleshooting` | diagnose a CLI that does not start Chrome, cannot connect or hangs |

The skills themselves are written in Russian; Claude follows them regardless of the
language you use.

**Sandbox.** Chrome does not start inside the Claude Code Bash sandbox (macOS Seatbelt),
and the first `chromectl` call writes the built binary into the plugin directory. The
skills tell Claude to run such commands outside the sandbox; you will be asked for
permission. Sandbox rules are managed with `/sandbox`.

## Install without the plugin

```bash
go install github.com/Y91R/chromectl/cmd/chromectl@latest
```

## Quick start

```bash
chromectl browser start --headless
chromectl pages new https://example.com
chromectl snapshot                 # accessibility tree with uids
chromectl click 1_5 --snapshot     # act by uid, get the updated tree back
chromectl screenshot -o page.png
chromectl browser stop
```

| Group | Commands |
|---|---|
| Browser and tabs | `browser start/stop/status`, `pages list/new/select/close`, `navigate` |
| Page | `snapshot`, `screenshot`, `eval`, `wait-for`, `dialog` |
| Input | `click`, `hover`, `drag`, `fill`, `fill-form`, `type`, `press`, `upload` |
| Emulation | `emulate` (viewport, color scheme, network, CPU, geolocation…), `resize` |
| Inspection | `console list/get`, `network list/get` |
| Diagnostics | `perf trace`, `perf insight`, `heap snapshot`, `audit lighthouse` |

Global flags: `--port` (default 9222; each port has its own state and profile),
`--page <id>`, `--json`. The full reference with every flag is the command table in
[skills/browser/SKILL.md](skills/browser/SKILL.md) and `chromectl <command> --help`.

## What the daemon-less model does not give you

- No network history before a command: repeat the action with `--capture`, then
  `network list` / `network get`. Sensitive headers are redacted by default.
- Emulation is stored per tab and re-applied on every connection.
- A performance trace is recorded in one command (`perf trace`), not as separate
  start/stop calls.
- State lives in `~/.cache/chromectl/<port>/state.json` (override with
  `CHROMECTL_STATE_DIR`).

## Development

```bash
make tools        # once: golangci-lint
make build        # build/chromectl
make test         # unit tests, no browser
make verify       # lint + guard + unit + e2e (needs Chrome; run outside the sandbox)
claude --plugin-dir .   # try the plugin from this checkout
```

Internal documentation is in Russian: [CLAUDE.md](CLAUDE.md), [AGENTS.md](AGENTS.md),
stage plans in [docs/plans](docs/plans/README.md), decisions in [docs/adr](docs/adr/README.md).
