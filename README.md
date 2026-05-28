# tgd — Terminal Git Diff

A 3-pane terminal UI that shows side-by-side git diffs of files
[Claude Code](https://claude.com/claude-code) edits, refreshed
automatically as the agent works.

```
┌─ CHANGED FILES ─┬─ ORIGINAL (HEAD) ─────────┬─ MODIFIED ────────────────┐
│ M cmd/tgd/main… │  42 │ if err != nil {     │  42 │ if err != nil {     │
│ M internal/app… │  43 │   return err        │  43 │   return wrap(err)  │
│ A internal/ipc… │  44 │ }                   │  44 │ }                   │
│                 │     │                     │  45 │ defer close(c)      │
└─────────────────┴─────────────────────────────────────────────────────────┘
  j/k:scroll  g/G:top/bot  ^d/^u:page  tab:focus  r:refresh  q:quit
```

## Why

Reading large multi-file edits as they scroll past in Claude Code is
painful. tgd opens a side-by-side diff next to your Claude session and
updates it on every `Write` / `Edit` / `MultiEdit` tool call, so you
can watch what's actually changing as the agent works.

Designed to be **invisible until needed**: nothing pops up until
Claude actually edits a file, and each Claude session gets its own
isolated tgd window — no cross-talk between projects.

## Install

Requires Go 1.21+ and [Claude Code](https://claude.com/claude-code).

```bash
git clone https://github.com/Rockynotchleaf/tgd
cd tgd
./scripts/install.sh
```

The install script:

1. Builds `tgd` and `tgd-hook` into `~/.local/bin`
2. Adds a `PostToolUse` hook to `~/.claude/settings.json` so tgd
   refreshes automatically when Claude edits a file

Make sure `~/.local/bin` is in your `PATH`.

## How it works

Two binaries:

- **`tgd`** — the TUI. 3-pane layout: file list │ original (HEAD) │
  modified working tree. Built with
  [Bubble Tea](https://github.com/charmbracelet/bubbletea).
- **`tgd-hook`** — a tiny binary registered as Claude Code's
  `PostToolUse` hook. Reads the JSON payload Claude sends on stdin,
  signals the running tgd over a unix socket, or spawns one if none
  is running.

IPC happens over `~/.local/share/tgd/tgd-<session>.sock`. Each Claude
session gets its own socket and window, so concurrent sessions never
share a diff view.

The diff is `git diff HEAD` filtered to files this session has
actually touched. That keeps unrelated untracked files (build
artifacts, scratch notes) out of the way while still surfacing brand
new files Claude creates.

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Navigate down (file list or scroll diff, depending on focus) |
| `k` / `↑` | Navigate up |
| `Tab` | Toggle focus: file list ↔ diff panes |
| `Enter` | Switch focus to diff panes |
| `g` | Jump to top of diff |
| `G` | Jump to bottom of diff |
| `Ctrl+D` / `PgDn` | Page down |
| `Ctrl+U` / `PgUp` | Page up |
| `r` | Manual refresh |
| `q` / `Ctrl+C` | Quit |

## Terminal support

tgd spawns its window using whichever multiplexer / terminal it
detects, in priority order:

1. tmux split (if `$TMUX` is set)
2. zellij pane (`$ZELLIJ`)
3. kitty remote control / new window (`$KITTY_WINDOW_ID`)
4. wezterm split / new window (`$WEZTERM_PANE`)
5. `$TGD_TERMINAL` override
6. `$TERM_PROGRAM` (ghostty, wezterm, …)
7. `$TERMINAL` (Arch / freedesktop convention)
8. PATH probe: ghostty → kitty → wezterm → foot → alacritty → xterm

Set `TGD_TERMINAL=<name>` if you want to force a specific terminal.

## Manual launch

You can also run tgd outside of Claude:

```bash
cd path/to/your/repo
tgd
```

Without a Claude session driving it, tgd will show "no session
changes yet" until something writes to the socket — useful as a
sanity check that the binary is installed, but the hook is the
intended workflow.

## Development

```bash
go build ./...    # compile
go test ./...     # tests (diff engine in internal/diff/)
```

| Package | Responsibility |
|---------|----------------|
| `internal/diff` | Git ops + unified-diff parsing + line-aligned side-by-side conversion |
| `internal/ipc` | Unix socket server/client + per-session paths + PID liveness |
| `internal/launcher` | Spawning tgd in the right terminal / multiplexer |
| `internal/app` | Bubble Tea model, update, view |
| `cmd/tgd` | TUI entry point |
| `cmd/tgd-hook` | Claude Code PostToolUse hook |
