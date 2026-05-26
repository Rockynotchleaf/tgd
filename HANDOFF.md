# tgd — Session Handoff

**Date:** 2026-05-26  
**Repo:** `/home/chris/dev/terminal-git-diff`  
**Binaries installed:** `~/.local/bin/tgd`, `~/.local/bin/tgd-hook`  
**Hook:** Live in `~/.claude/settings.json` — fires on every Claude Write/Edit/MultiEdit

---

## What This Is

A 3-pane terminal git diff TUI that auto-triggers whenever Claude Code edits a file.

```
┌─ CHANGED FILES ──┬──── ORIGINAL (HEAD) ────┬──── MODIFIED ────┐
│ M src/main.go    │   old line               │                  │
│ A src/new.go     │                          │   new line       │
│ D src/gone.go    │   unchanged              │   unchanged      │
└──────────────────┴──────────────────────────┴──────────────────┘
```

- **Left panel:** changed files only (`git diff HEAD` + untracked)
- **Middle panel:** file as it exists in HEAD (red = removed lines)
- **Right panel:** current working-tree file (green = added lines)
- **True line-aligned:** delete/insert pairs share the same row; blank filler lines fill the shorter side
- **Diff baseline:** `git diff HEAD` — all changes since last commit

---

## How It Works

Two binaries:

**`tgd`** — the TUI itself (Go + Bubbletea)
- Opens in alt screen, 3-pane layout
- Listens on a unix socket at `~/.local/share/tgd/tgd.sock`
- Writes PID to `~/.local/share/tgd/tgd.pid`
- Debounces refresh: waits 500ms after the last IPC signal, then re-runs `git diff HEAD`

**`tgd-hook`** — Claude Code PostToolUse hook
- Runs on every Write/Edit/MultiEdit tool call
- **Guard 1:** resolves `git repo root` from `cwd` — exits silently if cwd isn't in a git repo
- **Guard 2:** checks that the edited `file_path` is under that repo root — exits silently if not (prevents spurious triggers from Claude editing `~/.claude/settings.json` etc.)
- If tgd is running: sends `{"type":"refresh","cwd":"..."}` to the socket
- If tgd is not running: spawns it, then retries sending up to 10×200ms

**Spawn behavior:**
- If `$TMUX` is set: `tmux split-window -h -p 40 -c <cwd> tgd` (40% horizontal split)
- Otherwise: `ghostty --working-directory=<cwd> -e tgd` (new Ghostty window)

---

## Current State

**3 commits on `main`, all passing:**

```
2d3251e fix: use ghostty -e instead of ghostty -- for spawning tgd
de20473 fix: only trigger tgd when edited file is inside cwd's git repo
be5bae9 feat: initial tgd implementation
```

**Tests:** `go test ./...` — 8 unit tests in `internal/diff/`, all pass. These cover the core alignment algorithm (replace, insert, delete, new file, deleted file, no-hunk, context preservation).

**Hook is live.** The hook was already triggered once (when we edited `~/.claude/settings.json` during install), which exposed the scoping bug and was immediately fixed.

---

## Bugs Fixed This Session

### 1. Hook fired for files outside the project repo
**Symptom:** tgd opened a pane after Claude edited `~/.claude/settings.json`.  
**Root cause:** hook had no guard on whether the edited file belonged to the project.  
**Fix:** `tgd-hook` now runs `git rev-parse --show-toplevel` on `cwd`, then checks `strings.HasPrefix(absFilePath, repoRoot+"/")` before doing anything. See `cmd/tgd-hook/main.go`.

### 2. `ghostty -- tgd` produced "invalid field" errors
**Symptom:** New Ghostty window opened but showed `tgd: invalid field` / `echo: invalid field`.  
**Root cause:** Ghostty doesn't use `--` as a subcommand delimiter — it treats all positional args as config `key=value` pairs. So `tgd` was interpreted as a config key lookup, not a command.  
**Fix:** Changed launcher to `ghostty --working-directory=<cwd> -e tgd`. See `internal/launcher/launch.go`.

---

## Known Gaps / What Hasn't Been Tested Yet

These are things discussed or implied but not yet verified end-to-end:

1. **Full visual rendering** — The TUI compiles and the diff engine is verified via unit tests, but the actual 3-pane visual layout in a real terminal hasn't been seen yet. Open `tgd` manually in a tmux pane in any git repo to see it.

2. **tmux spawn path** — The tmux split-window command was written but not tested (all testing happened outside tmux). Verify with: unset nothing, just run the hook while inside a tmux session.

3. **Stale socket handling** — If tgd crashes without cleaning up, `tgd.sock` and `tgd.pid` can be left behind. `IsAlive` handles a stale PID (kill -0 fails → returns false), and `StartServer` removes a stale socket before binding. But a zombie socket where the PID is actually alive from a different process is an edge case not stress-tested.

4. **Large files / performance** — The alignment algorithm loads full file content into memory for every refresh. Not tested against files with thousands of lines or diffs with hundreds of hunks.

5. **Worktree sub-agents** — When Claude uses `isolation: "worktree"` in an Agent call, the sub-agent works in a different path. Theoretically this spawns a second tgd for the worktree. Not tested.

6. **Terminal resize** — `tea.WindowSizeMsg` triggers `rebuildViewports()`, which should reflow all panes. Not visually verified.

7. **`r` refresh keybind and manual tgd invocation** — Designed and wired but not explicitly tested.

---

## File Map

```
terminal-git-diff/
├── cmd/
│   ├── tgd/main.go              Entry: PID check → socket server → tea.NewProgram
│   └── tgd-hook/main.go         Hook: parse stdin JSON → guard → signal/spawn
├── internal/
│   ├── app/
│   │   ├── model.go             Model struct, Init(), cmdLoadAll, cmdLoadFile, rebuildViewports
│   │   ├── update.go            Update(): keys, RefreshMsg, DiffLoadedMsg, viewport sync
│   │   ├── view.go              View(): 3-pane lipgloss layout, renderSide, status bar
│   │   └── keys.go              keyIs() helper
│   ├── diff/
│   │   ├── git.go               RepoRoot, ChangedFiles, OrigLines, CurrentLines, LoadAligned
│   │   ├── parse.go             ParseHunks (wraps sourcegraph/go-diff)
│   │   ├── align.go             Align, AlignNewFile, AlignDeletedFile, flushPending
│   │   └── align_test.go        8 unit tests for the alignment algorithm
│   ├── ipc/
│   │   ├── server.go            Unix socket listener + 500ms time.AfterFunc debounce
│   │   ├── client.go            Dial + send JSON Message
│   │   └── pid.go               WritePID, ReadPID, IsAlive (kill -0 probe)
│   └── launcher/
│       └── launch.go            tmux split-window OR ghostty --working-directory -e tgd
├── scripts/
│   └── install.sh               go build → ~/.local/bin, patch ~/.claude/settings.json
├── CLAUDE.md                    Project context for Claude Code sessions
└── HANDOFF.md                   This file
```

---

## Key Design Decisions (Context for Future Work)

**Why two binaries?** `tgd-hook` must be lightweight and start fast — it runs on every file write. Keeping it separate from the TUI means no bubbletea/lipgloss startup cost in the hook path.

**Why unix socket + debounce instead of a file watcher?** A file watcher (inotify) would need to watch arbitrary repo paths. The hook already knows exactly which file was written, so IPC is more precise. The debounce (500ms `time.AfterFunc`) handles bursts from sub-agents or multi-file writes naturally.

**Why `git diff HEAD` as baseline, not session-start snapshots?** Simpler to implement correctly, works across tmux detach/reattach, and represents the natural "what has changed since I last committed" mental model. Session-start snapshots would need their own storage and bookkeeping.

**Why true line-aligned diff?** The `flushPending` algorithm in `align.go` pairs delete/insert runs 1:1, inserting blank filler lines for any excess on either side. This is what vimdiff does. The alternative (just two independent scrolling panes) is simpler but makes it hard to visually correlate what changed on each line.

**Why `viewport.SetYOffset` mirroring for sync scroll?** `bubbles/viewport` v1.0.0 has no follower/slave mode. After each scroll key, we read `leftVP.YOffset` and call `rightVP.SetYOffset(n)` to mirror. This works because we intercept scroll keys before delegating to the viewport's own Update().

---

## Quick Start for Next Session

```bash
cd /home/chris/dev/terminal-git-diff

# Build
go build ./...

# Test the diff engine
go test ./internal/diff/ -v

# Install binaries
go build -o ~/.local/bin/tgd ./cmd/tgd
go build -o ~/.local/bin/tgd-hook ./cmd/tgd-hook

# Run tgd manually in a git repo to see the TUI
cd /some/git/repo && tgd

# Simulate hook firing (tgd must already be running)
echo '{"hook_event_name":"PostToolUse","tool_name":"Write","cwd":"/path/to/repo","tool_input":{"file_path":"/path/to/repo/somefile.go"}}' \
  | tgd-hook
```

---

## Immediate Next Steps (Suggested)

1. **Open `tgd` in a real tmux pane and look at the layout** — hasn't been seen yet; visual bugs likely exist (panel widths, color scheme, status bar content)
2. **Test the tmux spawn path** — trigger the hook while inside tmux with tgd not running
3. **Verify `r` refresh** and `g`/`G` jump-to-top/bottom work correctly
4. **Test with a deleted file** — `git rm` a file and confirm tgd shows it correctly (all-red, no right side)
5. **Profile a large repo** — run `tgd` in a repo with many changed files to check for slowness in `ChangedFiles` or `LoadAligned`
