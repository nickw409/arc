package daemon

import (
	"encoding/json"
	"net"
)

// Request is the client-to-daemon message format.
type Request struct {
	Cmd              string `json:"cmd"`
	Plan             string `json:"plan,omitempty"`
	Project          string `json:"project,omitempty"`
	ConfigPath       string `json:"config_path,omitempty"`
	Timeout          int    `json:"timeout,omitempty"`
	UseWorktree      bool   `json:"use_worktree,omitempty"`
	PerPhaseWorktree bool   `json:"per_phase_worktree,omitempty"`
	StopOnFailure    bool   `json:"stop_on_failure,omitempty"`
	ChatMode         bool   `json:"chat_mode,omitempty"`
}

// Response is the daemon-to-client message format.
type Response struct {
	OK           bool        `json:"ok"`
	Error        string      `json:"error,omitempty"`
	QueuedPhases int         `json:"queued_phases,omitempty"`
	PlanName     string      `json:"plan_name,omitempty"`
	PlanStatus   string      `json:"plan_status,omitempty"`
	Phases       []PhaseInfo `json:"phases,omitempty"`
}

// PhaseInfo describes the status of a single phase in a response.
type PhaseInfo struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	Iteration    int    `json:"iteration,omitempty"`
	TestsPassing int    `json:"tests_passing,omitempty"`
	TestsTotal   int    `json:"tests_total,omitempty"`
}

// WriteMessage encodes v as a JSON newline-delimited message and writes it to conn.
func WriteMessage(conn net.Conn, v any) error {
	return json.NewEncoder(conn).Encode(v)
}

// ReadMessage reads a JSON newline-delimited message from conn and decodes it into v.
func ReadMessage(conn net.Conn, v any) error {
	return json.NewDecoder(conn).Decode(v)
}
