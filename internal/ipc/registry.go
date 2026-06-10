package ipc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Meta is the per-instance registry record a running tgd writes alongside its
// socket/PID files. It lets a hook discover which live tgd window is serving a
// given git repo — used to route sub-agent edits (which arrive under a
// different session id) to the parent session's window.
type Meta struct {
	SessionID string `json:"session_id"`
	RepoRoot  string `json:"repo_root"`
	Sock      string `json:"sock"`
	Pid       string `json:"pid"`
}

// MetaPath returns the registry file path for a session, mirroring Paths.
func MetaPath(stateDir, sessionID string) string {
	base := "tgd"
	if s := sanitizeSession(sessionID); s != "" {
		base = "tgd-" + s
	}
	return filepath.Join(stateDir, base+".meta")
}

// WriteMeta records this instance's repo root and IPC paths so hooks can find
// it by repo root later.
func WriteMeta(stateDir, sessionID, repoRoot string) error {
	sock, pid := Paths(stateDir, sessionID)
	data, err := json.Marshal(Meta{
		SessionID: sessionID,
		RepoRoot:  repoRoot,
		Sock:      sock,
		Pid:       pid,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(MetaPath(stateDir, sessionID), data, 0600)
}

// FindLiveByRepoRoot scans stateDir for a running tgd instance serving
// repoRoot and returns its socket path. When more than one is alive for the
// same repo (e.g. two concurrent sessions in one repo), the most recently
// started instance wins. Returns ok=false if none is alive.
func FindLiveByRepoRoot(stateDir, repoRoot string) (sock string, ok bool) {
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return "", false
	}
	var bestMod time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".meta") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(stateDir, e.Name()))
		if err != nil {
			continue
		}
		var m Meta
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		if m.RepoRoot != repoRoot || !IsAlive(m.Pid, m.Sock) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if sock == "" || info.ModTime().After(bestMod) {
			sock = m.Sock
			bestMod = info.ModTime()
		}
	}
	return sock, sock != ""
}
