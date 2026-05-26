package launcher

import (
	"os"
	"testing"
)

func TestTerminalCandidates_ordering(t *testing.T) {
	t.Setenv("TGD_TERMINAL", "myterm")
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TERMINAL", "foot")

	got := terminalCandidates()

	// TGD_TERMINAL must come first
	if got[0] != "myterm" {
		t.Errorf("expected TGD_TERMINAL first, got %q", got[0])
	}
	// TERM_PROGRAM second
	if got[1] != "ghostty" {
		t.Errorf("expected TERM_PROGRAM second, got %q", got[1])
	}
	// TERMINAL third
	if got[2] != "foot" {
		t.Errorf("expected TERMINAL third, got %q", got[2])
	}
}

func TestTerminalCandidates_dedup(t *testing.T) {
	t.Setenv("TGD_TERMINAL", "ghostty") // also in the known list
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TERMINAL", "")

	got := terminalCandidates()

	count := 0
	for _, name := range got {
		if name == "ghostty" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("ghostty appeared %d times in candidates, want 1; got %v", count, got)
	}
}

func TestTerminalCandidates_caseNormalization(t *testing.T) {
	t.Setenv("TGD_TERMINAL", "")
	t.Setenv("TERM_PROGRAM", "WezTerm") // mixed case as set by wezterm itself
	t.Setenv("TERMINAL", "")

	got := terminalCandidates()

	// Should be lowercased to "wezterm"
	found := false
	for _, name := range got {
		if name == "wezterm" {
			found = true
		}
		if name == "WezTerm" {
			t.Error("candidate list contains un-lowercased WezTerm")
		}
	}
	if !found {
		t.Error("wezterm not found in candidates after normalization")
	}
}

func TestTerminalCandidates_emptyEnv(t *testing.T) {
	t.Setenv("TGD_TERMINAL", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TERMINAL", "")

	got := terminalCandidates()

	// Should still return the known-good list
	known := []string{"ghostty", "kitty", "wezterm", "foot", "alacritty", "xterm"}
	for i, want := range known {
		if i >= len(got) {
			t.Fatalf("candidates too short: want %v, got %v", known, got)
		}
		if got[i] != want {
			t.Errorf("candidates[%d] = %q, want %q", i, got[i], want)
		}
	}
}

func TestTerminalNewWindow_args(t *testing.T) {
	const cwd = "/tmp/test-repo"
	const bin = "/usr/local/bin/tgd"

	cases := []struct {
		name     string
		wantArgs []string
	}{
		{"ghostty", []string{"ghostty", "--working-directory=" + cwd, "-e", bin}},
		{"kitty", []string{"kitty", "--directory", cwd, bin}},
		{"wezterm", []string{"wezterm", "start", "--cwd", cwd, "--", bin}},
		{"foot", []string{"foot", "--working-directory=" + cwd, bin}},
		{"alacritty", []string{"alacritty", "--working-directory", cwd, "-e", bin}},
		{"xterm", []string{"xterm", "-e", "sh", "-c", `cd "/tmp/test-repo" && exec /usr/local/bin/tgd`}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := terminalNewWindow(tc.name, cwd, bin)
			if cmd == nil {
				t.Fatal("terminalNewWindow returned nil")
			}
			if len(cmd.Args) != len(tc.wantArgs) {
				t.Fatalf("Args length %d, want %d\n  got:  %v\n  want: %v",
					len(cmd.Args), len(tc.wantArgs), cmd.Args, tc.wantArgs)
			}
			for i, arg := range tc.wantArgs {
				if cmd.Args[i] != arg {
					t.Errorf("Args[%d] = %q, want %q", i, cmd.Args[i], arg)
				}
			}
		})
	}
}

func TestTerminalNewWindow_unknownTerminal(t *testing.T) {
	cmd := terminalNewWindow("urxvt", "/tmp/repo", "/usr/bin/tgd")
	if cmd == nil {
		t.Fatal("expected non-nil cmd for unknown terminal")
	}
	// Should use the generic -e sh -c wrapper
	if len(cmd.Args) < 4 || cmd.Args[1] != "-e" {
		t.Errorf("expected generic -e sh -c wrapper, got args: %v", cmd.Args)
	}
}

func TestTerminalCandidates_noTMUXEnvLeakage(t *testing.T) {
	// Ensure candidates() itself doesn't check $TMUX — that's Launch()'s job.
	// If TMUX is set, candidates should still return the full list.
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	t.Setenv("TGD_TERMINAL", "")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TERMINAL", "")

	got := terminalCandidates()
	if len(got) == 0 {
		t.Error("candidates returned empty list even with TMUX set")
	}
}

func TestKittyRemoteCmd_nilWhenNotInPath(t *testing.T) {
	// Temporarily shadow PATH so kitty can't be found.
	old := os.Getenv("PATH")
	t.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", old)

	cmd := kittyRemoteCmd("/tmp/repo", "tgd")
	if cmd != nil {
		t.Error("expected nil when kitty is not in PATH")
	}
}
