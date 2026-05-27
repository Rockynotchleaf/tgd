package ipc

import (
	"path/filepath"
	"strings"
)

// Paths returns the socket and PID file paths for a tgd instance under
// stateDir, scoped to a Claude session. Each session gets its own files
// (tgd-<session>.sock / .pid) so concurrent sessions never share a window
// or accumulate each other's touched files.
//
// An empty sessionID yields the legacy shared paths (tgd.sock / tgd.pid),
// used for manual launches with no session context.
func Paths(stateDir, sessionID string) (sock, pid string) {
	base := "tgd"
	if s := sanitizeSession(sessionID); s != "" {
		base = "tgd-" + s
	}
	return filepath.Join(stateDir, base+".sock"), filepath.Join(stateDir, base+".pid")
}

// sanitizeSession reduces a session id to a filename-safe slug: it keeps
// [A-Za-z0-9._-] and drops everything else. Claude session ids are UUIDs, so
// this is normally a no-op; it just guards against an unexpected id breaking
// the path. The result is capped to keep socket paths within the OS limit
// (sun_path is ~104 bytes on macOS).
func sanitizeSession(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}
