package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Rockynotchleaf/tgd/internal/app"
	"github.com/Rockynotchleaf/tgd/internal/ipc"
)

func main() {
	var sessionID string
	flag.StringVar(&sessionID, "session", "",
		"Claude session id; scopes this instance's socket/state so each session gets its own window")
	flag.Parse()

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tgd: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}

	stateDir := filepath.Join(home, ".local", "share", "tgd")
	sockPath, pidPath := ipc.Paths(stateDir, sessionID)

	// Bail out if another instance is already running
	if ipc.IsAlive(pidPath, sockPath) {
		fmt.Fprintln(os.Stderr, "tgd: another instance is already running (kill it or press q to quit)")
		os.Exit(1)
	}

	// Ensure state directory exists
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "tgd: cannot create state dir %s: %v\n", stateDir, err)
		os.Exit(1)
	}

	// Write PID file
	if err := ipc.WritePID(pidPath); err != nil {
		fmt.Fprintf(os.Stderr, "tgd: cannot write PID file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(pidPath)

	// Working directory (where diff is computed)
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tgd: cannot determine working directory: %v\n", err)
		os.Exit(1)
	}

	// Exit cleanly when the terminal pane is closed externally (SIGHUP).
	// Without this, tgd can become a zombie: alive and holding its socket open,
	// but with no visible pane — causing the hook to keep refreshing invisibly.
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGHUP)
		<-ch
		os.Remove(pidPath)
		os.Remove(sockPath)
		os.Exit(0)
	}()

	// Create bubbletea program
	model := app.New(cwd, sockPath)
	p := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// Start IPC socket server in a background goroutine.
	// We pass p before calling p.Run() — p.Send() queues messages safely.
	go func() {
		if err := ipc.StartServer(sockPath, p); err != nil {
			// Server error is non-fatal; tgd still works manually
			_ = err
		}
	}()

	// Run the TUI (blocks until q or ctrl+c)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tgd: %v\n", err)
		os.Exit(1)
	}
}
