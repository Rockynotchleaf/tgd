package ipc

import (
	"os"
	"path/filepath"
	"testing"
)

// liveInstance writes the PID + socket + meta files for a fake running tgd in
// dir, using the current (alive) process PID so IsAlive passes.
func liveInstance(t *testing.T, dir, sessionID, repoRoot string) string {
	t.Helper()
	sock, pid := Paths(dir, sessionID)
	if err := WritePID(pid); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sock, nil, 0600); err != nil { // stand-in for the unix socket file
		t.Fatal(err)
	}
	if err := WriteMeta(dir, sessionID, repoRoot); err != nil {
		t.Fatal(err)
	}
	return sock
}

func TestFindLiveByRepoRoot(t *testing.T) {
	dir := t.TempDir()
	wantSock := liveInstance(t, dir, "sess-a", "/repos/alpha")
	liveInstance(t, dir, "sess-b", "/repos/beta")

	got, ok := FindLiveByRepoRoot(dir, "/repos/alpha")
	if !ok {
		t.Fatal("expected to find live instance for /repos/alpha")
	}
	if got != wantSock {
		t.Fatalf("got sock %q, want %q", got, wantSock)
	}

	if _, ok := FindLiveByRepoRoot(dir, "/repos/gamma"); ok {
		t.Fatal("expected no instance for unknown repo /repos/gamma")
	}
}

func TestFindLiveByRepoRootSkipsDead(t *testing.T) {
	dir := t.TempDir()
	// Write a meta + socket for a dead PID (1 is init; signal 0 from a normal
	// test process is not permitted, so IsAlive returns false). Use a PID that
	// is essentially never a live child of the test: a very large unused PID.
	sessionID := "dead"
	sock, pidPath := Paths(dir, sessionID)
	if err := os.WriteFile(pidPath, []byte("2147483646"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sock, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteMeta(dir, sessionID, "/repos/zombie"); err != nil {
		t.Fatal(err)
	}

	if _, ok := FindLiveByRepoRoot(dir, "/repos/zombie"); ok {
		t.Fatal("expected dead instance to be skipped")
	}
}

func TestMetaPathMatchesPaths(t *testing.T) {
	dir := "/state"
	meta := MetaPath(dir, "abc")
	sock, _ := Paths(dir, "abc")
	wantBase := "tgd-abc.meta"
	if filepath.Base(meta) != wantBase {
		t.Fatalf("meta base = %q, want %q", filepath.Base(meta), wantBase)
	}
	if filepath.Dir(meta) != filepath.Dir(sock) {
		t.Fatalf("meta dir %q != sock dir %q", filepath.Dir(meta), filepath.Dir(sock))
	}
}
