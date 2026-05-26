// Package launcher handles spawning tgd in an appropriate terminal context.
package launcher

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Launch spawns a new tgd process in a tmux split (if running inside tmux)
// or a new Ghostty window (fallback).
//
// cwd is the working directory to start tgd in.
// It detaches the child process from the parent's session to prevent SIGHUP.
func Launch(cwd string) error {
	tgdBin, err := exec.LookPath("tgd")
	if err != nil {
		tgdBin = "tgd" // hope it's in PATH when the target shell runs
	}

	var cmd *exec.Cmd

	if os.Getenv("TMUX") != "" {
		// Split the current tmux window horizontally, 40% width, start in cwd
		cmd = exec.Command("tmux", "split-window", "-h", "-p", "40", "-c", cwd, tgdBin)
	} else {
		// Fallback: open a new Ghostty window running tgd
		cmd = exec.Command("ghostty", "--", tgdBin)
		cmd.Dir = cwd
	}

	// Detach from parent session so tgd survives when tgd-hook exits
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Brief wait to let tgd create its socket before the hook retries
	time.Sleep(50 * time.Millisecond)
	return nil
}
