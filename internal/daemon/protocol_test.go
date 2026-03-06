package daemon

import (
	"net"
	"testing"
)

func TestWriteReadMessage(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	req := Request{
		Cmd:         "submit",
		Plan:        "my-plan",
		Project:     "/tmp/project",
		Timeout:     300,
		UseWorktree: true,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- WriteMessage(client, req)
	}()

	var got Request
	if err := ReadMessage(server, &got); err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	if got.Cmd != "submit" {
		t.Errorf("Cmd: got %q, want %q", got.Cmd, "submit")
	}
	if got.Plan != "my-plan" {
		t.Errorf("Plan: got %q, want %q", got.Plan, "my-plan")
	}
	if got.Timeout != 300 {
		t.Errorf("Timeout: got %d, want 300", got.Timeout)
	}
	if !got.UseWorktree {
		t.Error("UseWorktree: got false, want true")
	}
}

func TestWriteReadResponse(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	resp := Response{
		OK:         true,
		PlanName:   "test-plan",
		PlanStatus: "running",
		Phases: []PhaseInfo{
			{Name: "impl", Status: "complete", Iteration: 3, TestsPassing: 10, TestsTotal: 10},
			{Name: "review", Status: "pending"},
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- WriteMessage(server, resp)
	}()

	var got Response
	if err := ReadMessage(client, &got); err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	if !got.OK {
		t.Error("OK: got false, want true")
	}
	if len(got.Phases) != 2 {
		t.Fatalf("Phases: got %d, want 2", len(got.Phases))
	}
	if got.Phases[0].Name != "impl" {
		t.Errorf("Phases[0].Name: got %q, want %q", got.Phases[0].Name, "impl")
	}
	if got.Phases[0].TestsPassing != 10 {
		t.Errorf("Phases[0].TestsPassing: got %d, want 10", got.Phases[0].TestsPassing)
	}
}
