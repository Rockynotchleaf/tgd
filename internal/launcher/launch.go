// Package launcher handles spawning tgd in an appropriate terminal context.
package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Launch spawns tgd in an appropriate terminal, trying in priority order:
//
//  1. tmux split-window        ($TMUX set)
//  2. zellij new pane          ($ZELLIJ set)
//  3. kitty remote control     ($KITTY_WINDOW_ID set) → fallback to kitty new window
//  4. wezterm split-pane       ($WEZTERM_PANE set)    → fallback to wezterm new window
//  5. $TGD_TERMINAL override   (user escape hatch)
//  6. $TERM_PROGRAM            (set by ghostty, WezTerm, etc.)
//  7. $TERMINAL                (Arch / freedesktop convention)
//  8. PATH probe               ghostty → kitty → wezterm → foot → alacritty → xterm
func Launch(cwd string) error {
	tgdBin, err := exec.LookPath("tgd")
	if err != nil {
		tgdBin = "tgd" // hope it's in PATH when the target shell runs
	}

	// 1. tmux: split current window horizontally, 40% width
	if os.Getenv("TMUX") != "" {
		return startDetached(exec.Command(
			"tmux", "split-window", "-h", "-p", "40", "-c", cwd, tgdBin,
		))
	}

	// 2. zellij: new pane to the right
	if os.Getenv("ZELLIJ") != "" {
		return startDetached(exec.Command(
			"zellij", "run", "--direction", "Right", "--cwd", cwd, "--", tgdBin,
		))
	}

	// 3. kitty: try remote control first (requires allow_remote_control yes),
	// fall back to opening a plain new window.
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		if cmd := kittyRemoteCmd(cwd, tgdBin); cmd != nil {
			if err := startDetached(cmd); err == nil {
				return nil
			}
			// remote control failed (probably not enabled) — fall through
		}
		return startDetached(terminalNewWindow("kitty", cwd, tgdBin))
	}

	// 4. wezterm: try split-pane, fall back to a new window
	if os.Getenv("WEZTERM_PANE") != "" {
		if err := startDetached(weztermSplitCmd(cwd, tgdBin)); err == nil {
			return nil
		}
		return startDetached(terminalNewWindow("wezterm", cwd, tgdBin))
	}

	// 5–8: name-based lookup (env vars first, then PATH probe)
	for _, name := range terminalCandidates() {
		if _, err := exec.LookPath(name); err != nil {
			continue // not installed
		}
		if cmd := terminalNewWindow(name, cwd, tgdBin); cmd != nil {
			return startDetached(cmd)
		}
	}

	return fmt.Errorf("no supported terminal emulator found; set $TGD_TERMINAL or run tgd manually")
}

// terminalCandidates returns terminal names to probe, in priority order.
// Names are lowercased and deduplicated.
// Sources: $TGD_TERMINAL, $TERM_PROGRAM, $TERMINAL, then a known-good list.
func terminalCandidates() []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	add(os.Getenv("TGD_TERMINAL")) // explicit user override
	add(os.Getenv("TERM_PROGRAM")) // ghostty → "ghostty", WezTerm → "wezterm"
	add(os.Getenv("TERMINAL"))     // Arch / freedesktop convention
	for _, t := range []string{"ghostty", "kitty", "wezterm", "foot", "alacritty", "xterm"} {
		add(t)
	}
	return out
}

// kittyRemoteCmd builds a `kitty @ launch` command that opens a new OS window.
// Requires allow_remote_control yes in kitty.conf. Returns nil if kitty is not
// in PATH.
func kittyRemoteCmd(cwd, tgdBin string) *exec.Cmd {
	if _, err := exec.LookPath("kitty"); err != nil {
		return nil
	}
	return exec.Command("kitty", "@", "launch", "--type=os-window", "--cwd="+cwd, tgdBin)
}

// weztermSplitCmd splits the active WezTerm pane horizontally.
func weztermSplitCmd(cwd, tgdBin string) *exec.Cmd {
	return exec.Command("wezterm", "cli", "split-pane", "--horizontal", "--cwd="+cwd, "--", tgdBin)
}

// terminalNewWindow returns a command that opens a new terminal window and
// runs tgd in cwd. For terminals without a native --working-directory flag,
// a `sh -c 'cd <cwd> && exec tgd'` wrapper is used. Returns nil only when
// name is empty (should not happen in practice).
func terminalNewWindow(name, cwd, tgdBin string) *exec.Cmd {
	// Portable cwd setter for terminals without a --working-directory flag.
	// exec replaces the shell so it leaves no zombie parent process.
	shellWrapper := fmt.Sprintf("cd %q && exec %s", cwd, tgdBin)

	switch name {
	case "ghostty":
		return exec.Command("ghostty", "--working-directory="+cwd, "-e", tgdBin)
	case "kitty":
		return exec.Command("kitty", "--directory", cwd, tgdBin)
	case "wezterm":
		return exec.Command("wezterm", "start", "--cwd", cwd, "--", tgdBin)
	case "foot":
		return exec.Command("foot", "--working-directory="+cwd, tgdBin)
	case "alacritty":
		return exec.Command("alacritty", "--working-directory", cwd, "-e", tgdBin)
	case "xterm":
		return exec.Command("xterm", "-e", "sh", "-c", shellWrapper)
	default:
		// Unknown terminal: try -e with the sh wrapper. Works for most
		// terminals (urxvt, st, terminator, etc.) even if cwd is wrong.
		return exec.Command(name, "-e", "sh", "-c", shellWrapper)
	}
}

// startDetached starts cmd detached from the parent's process session so tgd
// survives when tgd-hook exits (SIGHUP is not delivered to a new session).
func startDetached(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Brief wait so tgd has time to create its unix socket before the hook
	// starts retrying ipc.Send.
	time.Sleep(50 * time.Millisecond)
	return nil
}
