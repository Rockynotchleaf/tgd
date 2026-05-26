package ipc

import (
	"encoding/json"
	"net"
	"time"
)

// Message is the wire protocol sent over the unix socket.
type Message struct {
	Type string `json:"type"` // "refresh" | "ping"
	CWD  string `json:"cwd,omitempty"`
}

// Send connects to the unix socket at sockPath and sends a message.
// Returns an error if the socket is not reachable (tgd not running).
func Send(sockPath string, msg Message) error {
	conn, err := net.DialTimeout("unix", sockPath, 500*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(500 * time.Millisecond)) //nolint:errcheck
	return json.NewEncoder(conn).Encode(msg)
}
