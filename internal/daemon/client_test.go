package daemon

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSocketPath(t *testing.T) {
	path := DefaultSocketPath()
	if path == "" {
		t.Fatal("DefaultSocketPath returned empty string")
	}
	if filepath.Base(path) != "daemon.sock" {
		t.Errorf("expected daemon.sock, got %q", filepath.Base(path))
	}
}

func TestIsRunningNoSocket(t *testing.T) {
	if IsRunning("/tmp/nonexistent-arc-test-socket.sock") {
		t.Error("IsRunning should return false for nonexistent socket")
	}
}

func TestIsRunningWithSocket(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	if !IsRunning(sockPath) {
		t.Error("IsRunning should return true for active socket")
	}
}

func TestConnectAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Server goroutine: read request, write response
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var req Request
		if err := ReadMessage(conn, &req); err != nil {
			return
		}
		resp := Response{
			OK:       true,
			PlanName: req.Plan,
		}
		WriteMessage(conn, resp)
	}()

	client, err := Connect(sockPath, 5e9) // 5s
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	resp, err := client.Status("test-plan")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !resp.OK {
		t.Error("expected OK=true")
	}
	if resp.PlanName != "test-plan" {
		t.Errorf("PlanName: got %q, want %q", resp.PlanName, "test-plan")
	}
}

func TestConnectFails(t *testing.T) {
	_, err := Connect("/tmp/nonexistent-arc-test.sock", 1e9)
	if err == nil {
		t.Error("expected error connecting to nonexistent socket")
	}
}

func TestClientSubmit(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var req Request
		if err := ReadMessage(conn, &req); err != nil {
			return
		}
		if req.Cmd != "submit" {
			WriteMessage(conn, Response{Error: "expected submit"})
			return
		}
		WriteMessage(conn, Response{OK: true, QueuedPhases: 3})
	}()

	client, err := Connect(sockPath, 5e9)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	resp, err := client.Submit(Request{Plan: "my-plan", Project: "/proj"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !resp.OK {
		t.Error("expected OK=true")
	}
	if resp.QueuedPhases != 3 {
		t.Errorf("QueuedPhases: got %d, want 3", resp.QueuedPhases)
	}

	// Cleanup
	os.Remove(sockPath)
}

func TestClientList(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var req Request
		if err := ReadMessage(conn, &req); err != nil {
			return
		}
		if req.Cmd != "list" {
			WriteMessage(conn, Response{OK: false, Error: "expected list command"})
			return
		}
		WriteMessage(conn, Response{
			OK: true,
			ActivePlans: []ActivePlanInfo{
				{
					PlanName:    "my-plan",
					ProjectDir:  "/proj",
					Phases:      []PhaseInfo{{Name: "impl", Status: "running"}},
					SubmittedAt: "2026-03-09T12:00:00Z",
				},
			},
		})
	}()

	client, err := Connect(sockPath, 5e9)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	resp, err := client.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !resp.OK {
		t.Errorf("expected OK=true, got error: %s", resp.Error)
	}
	if len(resp.ActivePlans) != 1 {
		t.Fatalf("expected 1 active plan, got %d", len(resp.ActivePlans))
	}
	if resp.ActivePlans[0].PlanName != "my-plan" {
		t.Errorf("PlanName: got %q, want my-plan", resp.ActivePlans[0].PlanName)
	}
	if len(resp.ActivePlans[0].Phases) != 1 || resp.ActivePlans[0].Phases[0].Status != "running" {
		t.Errorf("unexpected phase data: %+v", resp.ActivePlans[0].Phases)
	}

	os.Remove(sockPath)
}
