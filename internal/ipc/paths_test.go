package ipc

import (
	"path/filepath"
	"testing"
)

func TestPaths_perSessionIsolation(t *testing.T) {
	const dir = "/home/u/.local/share/tgd"

	sockA, pidA := Paths(dir, "session-aaa")
	sockB, pidB := Paths(dir, "session-bbb")

	if sockA == sockB {
		t.Errorf("distinct sessions share a socket: %q", sockA)
	}
	if pidA == pidB {
		t.Errorf("distinct sessions share a pid file: %q", pidA)
	}
	if want := filepath.Join(dir, "tgd-session-aaa.sock"); sockA != want {
		t.Errorf("sockA = %q, want %q", sockA, want)
	}
	if want := filepath.Join(dir, "tgd-session-aaa.pid"); pidA != want {
		t.Errorf("pidA = %q, want %q", pidA, want)
	}
}

func TestPaths_emptySessionUsesSharedPaths(t *testing.T) {
	const dir = "/home/u/.local/share/tgd"
	sock, pid := Paths(dir, "")
	if want := filepath.Join(dir, "tgd.sock"); sock != want {
		t.Errorf("sock = %q, want %q", sock, want)
	}
	if want := filepath.Join(dir, "tgd.pid"); pid != want {
		t.Errorf("pid = %q, want %q", pid, want)
	}
}

func TestSanitizeSession(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"a1b2-c3d4_e5.f6", "a1b2-c3d4_e5.f6"}, // UUID-ish: unchanged
		{"../../etc/passwd", "....etcpasswd"},  // path separators stripped
		{"has spaces/and:colons", "hasspacesandcolons"},
		{"", ""},
	}
	for _, c := range cases {
		if got := sanitizeSession(c.in); got != c.want {
			t.Errorf("sanitizeSession(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// Long ids are capped so socket paths stay within the OS limit.
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	if got := sanitizeSession(string(long)); len(got) != 64 {
		t.Errorf("long id not capped: len = %d, want 64", len(got))
	}
}
