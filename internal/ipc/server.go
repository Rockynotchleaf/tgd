package ipc

import (
	"encoding/json"
	"net"
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// RefreshMsg is sent into the bubbletea program when a debounced refresh fires.
type RefreshMsg struct {
	CWD string
	// Files are the repo-relative paths reported by hooks during the debounce
	// window. tgd merges them into its session "touched" set.
	Files []string
}

// StartServer listens on sockPath for incoming IPC messages from tgd-hook.
// When a "refresh" message arrives, it resets a 500ms debounce timer; once
// the timer fires it sends a RefreshMsg into p.
//
// StartServer blocks until the listener is closed. It removes the socket file
// on return. Call it in a goroutine.
func StartServer(sockPath string, p *tea.Program) error {
	// Remove any stale socket from a previous run
	os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return err
	}
	defer func() {
		ln.Close()
		os.Remove(sockPath)
	}()

	const debounce = 500 * time.Millisecond
	var (
		mu      sync.Mutex
		timer   *time.Timer
		lastCWD string
		pending = map[string]bool{} // files touched since the last fire
	)

	fire := func() {
		mu.Lock()
		cwd := lastCWD
		files := make([]string, 0, len(pending))
		for f := range pending {
			files = append(files, f)
		}
		pending = map[string]bool{}
		mu.Unlock()
		p.Send(RefreshMsg{CWD: cwd, Files: files})
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			return nil // listener was closed
		}
		go func(c net.Conn) {
			defer c.Close()
			c.SetDeadline(time.Now().Add(time.Second)) //nolint:errcheck

			var msg Message
			if err := json.NewDecoder(c).Decode(&msg); err != nil {
				return
			}
			if msg.Type != "refresh" {
				return
			}

			mu.Lock()
			lastCWD = msg.CWD
			if msg.File != "" {
				pending[msg.File] = true
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounce, fire)
			mu.Unlock()
		}(conn)
	}
}
