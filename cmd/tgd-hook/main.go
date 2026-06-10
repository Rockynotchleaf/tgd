// tgd-hook is the Claude Code PostToolUse hook binary.
// It reads a PostToolUse JSON payload from stdin, extracts the working
// directory, and either signals a running tgd instance to refresh or
// launches a new one.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Rockynotchleaf/tgd/internal/ipc"
	"github.com/Rockynotchleaf/tgd/internal/launcher"
)

// hookPayload mirrors the JSON Claude Code sends on stdin for PostToolUse.
type hookPayload struct {
	SessionID     string    `json:"session_id"`
	CWD           string    `json:"cwd"`
	HookEventName string    `json:"hook_event_name"`
	ToolName      string    `json:"tool_name"`
	ToolInput     toolInput `json:"tool_input"`
	// AgentID is set only when the hook fires inside a sub-agent (Task tool).
	// Sub-agents run with their own fresh session_id, so without this we'd
	// route their edits to a nonexistent per-session socket and spawn a
	// throwaway window. Its presence is the signal to route to the parent.
	AgentID string `json:"agent_id"`
}

type toolInput struct {
	FilePath string `json:"file_path"`
}

func main() {
	var payload hookPayload
	if err := json.NewDecoder(os.Stdin).Decode(&payload); err != nil {
		// Can't parse payload — exit silently (don't break Claude)
		os.Exit(0)
	}

	// Only react to Write and Edit tool calls
	switch payload.ToolName {
	case "Write", "Edit", "MultiEdit":
		// proceed
	default:
		os.Exit(0)
	}

	cwd := payload.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			os.Exit(0)
		}
	}

	// Resolve the git repo root for cwd. If cwd isn't inside a git repo,
	// there's nothing to diff — exit silently.
	repoRoot, err := gitRepoRoot(cwd)
	if err != nil {
		os.Exit(0)
	}

	// Guard: only proceed if the edited file lives inside the repo.
	// This prevents tgd from popping open when Claude edits files outside
	// the current project (e.g. ~/.claude/settings.json, /tmp/scratch, etc.).
	// relPath is the repo-relative path reported to tgd so it can scope the
	// diff to files touched this session.
	var relPath string
	filePath := payload.ToolInput.FilePath
	if filePath != "" {
		abs, err := filepath.Abs(filePath)
		if err != nil || !strings.HasPrefix(abs, repoRoot+string(filepath.Separator)) {
			os.Exit(0)
		}
		if rel, err := filepath.Rel(repoRoot, abs); err == nil {
			relPath = filepath.ToSlash(rel) // match git's forward-slash paths
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		os.Exit(0)
	}

	// Scope the socket/PID to this Claude session so each session drives its
	// own tgd window and never sees another session's edits.
	stateDir := filepath.Join(home, ".local", "share", "tgd")

	msg := ipc.Message{Type: "refresh", CWD: cwd, RepoRoot: repoRoot, File: relPath}

	// Sub-agent edits: a sub-agent (Task tool) fires this hook with its own
	// fresh session_id, not the parent's. Routing to its own per-session socket
	// would spawn a throwaway window the user never sees and lose the edit.
	// Instead, hand the edit to whatever live tgd is already serving this repo
	// (the parent session's window). If none is running, drop silently rather
	// than spawning a new window for an ephemeral sub-agent.
	if payload.AgentID != "" {
		if sock, ok := ipc.FindLiveByRepoRoot(stateDir, repoRoot); ok {
			_ = ipc.Send(sock, msg)
		}
		os.Exit(0)
	}

	sockPath, pidPath := ipc.Paths(stateDir, payload.SessionID)

	// Fast path: tgd is already running — send refresh and exit
	if err := ipc.Send(sockPath, msg); err == nil {
		os.Exit(0)
	}

	// tgd is not running — launch it (scoped to this session) and retry a few times
	if err := launcher.Launch(cwd, payload.SessionID); err != nil {
		fmt.Fprintf(os.Stderr, "tgd-hook: failed to launch tgd: %v\n", err)
		os.Exit(0)
	}

	// Retry sending the refresh message until tgd's socket is ready
	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		if ipc.IsAlive(pidPath, sockPath) {
			if err := ipc.Send(sockPath, msg); err == nil {
				os.Exit(0)
			}
		}
	}

	// If we still can't connect, tgd is starting up and will load the diff
	// on its own from Init() — no action needed.
	os.Exit(0)
}

// gitRepoRoot returns the absolute path to the git repository root containing
// dir. Returns an error if dir is not inside a git repository.
func gitRepoRoot(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
