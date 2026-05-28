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
//
// sessionID, when non-empty, is passed to the spawned tgd as `--session <id>`
// so each Claude session gets its own isolated window and socket.
func Launch(cwd, sessionID string) error {
	tgdBin, err := exec.LookPath("tgd")
	if err != nil {
		tgdBin = "tgd" // hope it's in PATH when the target shell runs
	}

	// tgd invocation as argv, plus a shell-quoted form for sh -c wrappers.
	argv := []string{tgdBin}
	if sessionID != "" {
		argv = append(argv, "--session", sessionID)
	}
	shellCmd := shellQuoteAll(argv)

	// 1. tmux: split the pane the hook was invoked from, not whatever pane the
	// user happens to be focused on. Without -t, tmux targets the active pane
	// of the active window, which causes tgd to pop up in the wrong window
	// when multiple Claude sessions run side-by-side. $TMUX_PANE is set by
	// tmux for every process running inside a pane.
	if os.Getenv("TMUX") != "" {
		args := []string{"split-window", "-h", "-p", "40", "-c", cwd}
		if pane := os.Getenv("TMUX_PANE"); pane != "" {
			args = append(args, "-t", pane)
		}
		args = append(args, argv...)
		return startDetached(exec.Command("tmux", args...))
	}

	// 2. zellij: new pane to the right
	if os.Getenv("ZELLIJ") != "" {
		return startDetached(exec.Command(
			"zellij", append([]string{"run", "--direction", "Right", "--cwd", cwd, "--"}, argv...)...,
		))
	}

	// 3. kitty: try remote control first (requires allow_remote_control yes),
	// fall back to opening a plain new window.
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		if cmd := kittyRemoteCmd(cwd, argv); cmd != nil {
			if err := startDetached(cmd); err == nil {
				return nil
			}
			// remote control failed (probably not enabled) — fall through
		}
		return startDetached(terminalNewWindow("kitty", cwd, argv, shellCmd))
	}

	// 4. wezterm: try split-pane, fall back to a new window
	if os.Getenv("WEZTERM_PANE") != "" {
		if err := startDetached(weztermSplitCmd(cwd, argv)); err == nil {
			return nil
		}
		return startDetached(terminalNewWindow("wezterm", cwd, argv, shellCmd))
	}

	// 5–8: name-based lookup (env vars first, then PATH probe)
	for _, name := range terminalCandidates() {
		if _, err := exec.LookPath(name); err != nil {
			continue // not installed
		}
		if cmd := terminalNewWindow(name, cwd, argv, shellCmd); cmd != nil {
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
func kittyRemoteCmd(cwd string, argv []string) *exec.Cmd {
	if _, err := exec.LookPath("kitty"); err != nil {
		return nil
	}
	return exec.Command("kitty", append([]string{"@", "launch", "--type=os-window", "--cwd=" + cwd}, argv...)...)
}

// weztermSplitCmd splits the active WezTerm pane horizontally.
func weztermSplitCmd(cwd string, argv []string) *exec.Cmd {
	return exec.Command("wezterm", append([]string{"cli", "split-pane", "--horizontal", "--cwd=" + cwd, "--"}, argv...)...)
}

// terminalNewWindow returns a command that opens a new terminal window and
// runs tgd (argv) in cwd. For terminals without a native --working-directory
// flag, a `sh -c 'cd <cwd> && exec <shellCmd>'` wrapper is used, where shellCmd
// is the shell-quoted tgd invocation. Returns nil only when name is empty
// (should not happen in practice).
func terminalNewWindow(name, cwd string, argv []string, shellCmd string) *exec.Cmd {
	// Portable cwd setter for terminals without a --working-directory flag.
	// exec replaces the shell so it leaves no zombie parent process.
	shellWrapper := fmt.Sprintf("cd %q && exec %s", cwd, shellCmd)

	switch name {
	case "ghostty":
		return exec.Command("ghostty", append([]string{"--working-directory=" + cwd, "-e"}, argv...)...)
	case "kitty":
		return exec.Command("kitty", append([]string{"--directory", cwd}, argv...)...)
	case "wezterm":
		return exec.Command("wezterm", append([]string{"start", "--cwd", cwd, "--"}, argv...)...)
	case "foot":
		return exec.Command("foot", append([]string{"--working-directory=" + cwd}, argv...)...)
	case "alacritty":
		return exec.Command("alacritty", append([]string{"--working-directory", cwd, "-e"}, argv...)...)
	case "xterm":
		return exec.Command("xterm", "-e", "sh", "-c", shellWrapper)
	default:
		// Unknown terminal: try -e with the sh wrapper. Works for most
		// terminals (urxvt, st, terminator, etc.) even if cwd is wrong.
		return exec.Command(name, "-e", "sh", "-c", shellWrapper)
	}
}

// shellQuoteAll joins argv into a single shell command string, quoting each
// argument so it survives an `sh -c` wrapper intact.
func shellQuoteAll(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

// shellQuote returns s safe for use as a single POSIX shell word. Words made
// only of shell-safe characters are returned unquoted; anything else is
// single-quoted with embedded single quotes escaped.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '/', r == '.', r == '_', r == '-', r == ':':
			// safe
		default:
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
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
