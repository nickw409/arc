package daemon

import (
	"net"
	"os"
	"path/filepath"
	"time"
)

// Client communicates with the arc daemon over a unix socket.
type Client struct {
	conn net.Conn
}

// Connect dials the daemon's unix socket with the given timeout.
func Connect(socketPath string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}

// Submit sends a submit request to the daemon.
func (c *Client) Submit(req Request) (*Response, error) {
	req.Cmd = "submit"
	return c.roundTrip(req)
}

// Status queries the status of a plan.
func (c *Client) Status(planName string) (*Response, error) {
	return c.roundTrip(Request{Cmd: "status", Plan: planName})
}

// Cancel requests cancellation of a plan.
func (c *Client) Cancel(planName string) (*Response, error) {
	return c.roundTrip(Request{Cmd: "cancel", Plan: planName})
}

// Drain requests the daemon to drain (stop accepting new work and shut down after completion).
func (c *Client) Drain() (*Response, error) {
	return c.roundTrip(Request{Cmd: "drain"})
}

// List requests the list of all active plans from the daemon.
func (c *Client) List() (*Response, error) {
	return c.roundTrip(Request{Cmd: "list"})
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) roundTrip(req Request) (*Response, error) {
	if err := WriteMessage(c.conn, req); err != nil {
		return nil, err
	}
	var resp Response
	if err := ReadMessage(c.conn, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DefaultSocketPath returns the default path for the daemon's unix socket.
func DefaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".arc", "daemon.sock")
}

// IsRunning probes the daemon socket to check if a daemon is listening.
func IsRunning(socketPath string) bool {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
