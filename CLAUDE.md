# tgd — Terminal Git Diff

A 3-pane TUI that shows side-by-side git diffs, automatically triggered
when Claude Code edits files via a PostToolUse hook.

## Architecture

Two binaries:
- `cmd/tgd/` — the TUI (Go + Bubbletea). 3-pane layout: file list | original (HEAD) | modified.
- `cmd/tgd-hook/` — the Claude Code hook binary. Reads PostToolUse JSON from stdin, signals tgd via unix socket.

IPC: unix socket at `~/.local/share/tgd/tgd-<session>.sock`. Hook sends `{"type":"refresh","cwd":"...","file":"<repo-relative path>"}`. tgd debounces 500ms, merges the reported file(s) into its session "touched" set, then re-runs git diff.

Per session: the socket/PID are keyed by Claude's `session_id` (`ipc.Paths`), so each session gets its own isolated tgd window and never sees another session's edits. The hook passes `--session <id>` to the spawned tgd. An empty session id falls back to the shared `tgd.sock`/`tgd.pid` (manual launch).

Sub-agent edits: when Claude spawns a sub-agent (Task tool), its PostToolUse hook fires with the sub-agent's *own* fresh `session_id` plus an `agent_id`. Routing that to a per-session socket would spawn a throwaway window and lose the edit. Instead, on `agent_id != ""` the hook looks up the live tgd serving the same repo (via the `.meta` registry, keyed by repo root) and forwards the edit there — so sub-agent edits land in the parent session's window. Each running tgd records its repo root in `tgd-<session>.meta`; the hook scans these and picks the most-recently-started live match. tgd also ignores any refresh whose repo root differs from the one it's showing, so a session that touches two repos can't cross-contaminate the touched set.

## Build & Install

```bash
go build ./...          # compile check
go test ./...           # run tests (diff engine tests in internal/diff/)
./scripts/install.sh    # build + install to ~/.local/bin + patch ~/.claude/settings.json
```

## Key Packages

| Package | Responsibility |
|---------|---------------|
| `internal/diff/git.go` | Git operations: `ChangedFiles`, `FilterTouched`, `OrigLines`, `CurrentLines`, `LoadAligned` |
| `internal/diff/align.go` | Core algorithm: unified diff hunks → `[]AlignedLine` (true line-aligned side-by-side) |
| `internal/diff/parse.go` | Wraps `sourcegraph/go-diff` to parse unified diff into `[]*Hunk` |
| `internal/ipc/server.go` | Unix socket listener + 500ms debounce → `RefreshMsg` into tea.Program |
| `internal/ipc/client.go` | Dial socket + send JSON message (`Message{Type, CWD, File}`) |
| `internal/ipc/pid.go` | PID file write/read/liveness check |
| `internal/ipc/paths.go` | Per-session socket/PID paths (`Paths(stateDir, sessionID)`) |
| `internal/ipc/registry.go` | Per-instance `.meta` record (repo root + IPC paths); `FindLiveByRepoRoot` routes sub-agent edits to the parent window |
| `internal/launcher/launch.go` | Spawn tgd in tmux split (if `$TMUX` set) or new Ghostty window |
| `internal/app/model.go` | Bubbletea Model struct, Init(), cmdLoadAll, cmdLoadFile |
| `internal/app/update.go` | Update() — key dispatch, viewport sync, async message handling |
| `internal/app/view.go` | View() — lipgloss 3-pane layout, renderSide, status bar |

## Diff Baseline

`git diff HEAD` — changes since last commit, plus untracked files — **filtered
to files edited this session**. The hook reports each edited file's repo-relative
path; tgd accumulates them into a "touched" set (`Model.touched`) and shows only
the intersection (`diff.FilterTouched`). This keeps intentionally-untracked
clutter (build artifacts, scratch files) out of the view while still surfacing
new files Claude creates.

Empty touched set (cold start / manual launch): file list shows "no session
changes yet". The set is in-memory and resets when tgd restarts.

New files (status `A` or `??`): shown as all-additions, empty original side.
Deleted files (status `D`): shown as all-removals, empty modified side.

Long lines soft-wrap to the pane width rather than truncating (`renderBothSides`
in `view.go`). Both sides are rendered together so a line that wraps to N visual
rows on one side pads the other side to N rows, keeping the two viewports aligned
under lockstep scrolling. Consequence: `j`/`k`/`J`/`K` and the scroll-% indicator
count visual rows, not source lines.

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Navigate down (file list or scroll diff, depending on focus) |
| `k` / `↑` | Navigate up |
| `J` / `K` | Scroll diff by one line — works regardless of focus |
| `Ctrl+D` / `PgDn` | Half-page down in diff — works regardless of focus |
| `Ctrl+U` / `PgUp` | Half-page up in diff — works regardless of focus |
| Mouse wheel | Scroll diff — works regardless of focus |
| `Tab` | Toggle focus: file list ↔ diff panes |
| `Enter` | Switch focus to diff panes |
| `g` | Jump to top of diff (diff focus) |
| `G` | Jump to bottom of diff (diff focus) |
| `r` | Manual refresh |
| `q` / `Ctrl+C` | Quit |

## Dependencies

- `github.com/charmbracelet/bubbletea` — TUI framework
- `github.com/charmbracelet/lipgloss` — styling
- `github.com/charmbracelet/bubbles` — viewport component
- `github.com/sourcegraph/go-diff` — unified diff parser
- `github.com/mattn/go-runewidth` — unicode display width

## Claude Code Hook Config

Added to `~/.claude/settings.json` by `scripts/install.sh`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit|MultiEdit",
        "hooks": [{ "type": "command", "command": "~/.local/bin/tgd-hook", "timeout": 5 }]
      }
    ]
  }
}
```
