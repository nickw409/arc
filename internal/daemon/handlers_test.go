package daemon

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/state"
)

func TestHandleConnection_UnknownCmd(t *testing.T) {
	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	c1, c2 := net.Pipe()
	defer c1.Close()

	go HandleConnection(c2, sched, &cfg)

	_ = WriteMessage(c1, Request{Cmd: "bogus"})
	var resp Response
	if err := ReadMessage(c1, &resp); err != nil {
		t.Fatalf("reading response: %v", err)
	}
	if resp.OK {
		t.Error("expected OK=false for unknown command")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error")
	}
}

func TestHandleConnection_Drain(t *testing.T) {
	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	c1, c2 := net.Pipe()
	defer c1.Close()

	go HandleConnection(c2, sched, &cfg)

	_ = WriteMessage(c1, Request{Cmd: "drain"})
	var resp Response
	if err := ReadMessage(c1, &resp); err != nil {
		t.Fatalf("reading response: %v", err)
	}
	if !resp.OK {
		t.Errorf("expected OK=true, got error: %s", resp.Error)
	}
}

func TestHandleConnection_StatusEmpty(t *testing.T) {
	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	c1, c2 := net.Pipe()
	defer c1.Close()

	go HandleConnection(c2, sched, &cfg)

	_ = WriteMessage(c1, Request{Cmd: "status"})
	var resp Response
	if err := ReadMessage(c1, &resp); err != nil {
		t.Fatalf("reading response: %v", err)
	}
	if !resp.OK {
		t.Errorf("expected OK=true for global status, got error: %s", resp.Error)
	}
}

func TestHandleConnection_StatusPlan(t *testing.T) {
	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	c1, c2 := net.Pipe()
	defer c1.Close()

	go HandleConnection(c2, sched, &cfg)

	_ = WriteMessage(c1, Request{Cmd: "status", Plan: "nonexistent"})
	var resp Response
	if err := ReadMessage(c1, &resp); err != nil {
		t.Fatalf("reading response: %v", err)
	}
	if resp.OK {
		t.Error("expected OK=false for missing plan status")
	}
}

func TestHandleConnection_CancelMissing(t *testing.T) {
	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	c1, c2 := net.Pipe()
	defer c1.Close()

	go HandleConnection(c2, sched, &cfg)

	_ = WriteMessage(c1, Request{Cmd: "cancel", Plan: "nope"})
	var resp Response
	if err := ReadMessage(c1, &resp); err != nil {
		t.Fatalf("reading response: %v", err)
	}
	if resp.OK {
		t.Error("expected OK=false for cancel of missing plan")
	}
}

func TestHandleConnection_BadJSON(t *testing.T) {
	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	c1, c2 := net.Pipe()
	defer c1.Close()

	go HandleConnection(c2, sched, &cfg)

	// Write invalid JSON
	c1.Write([]byte("not json\n"))
	var resp Response
	if err := ReadMessage(c1, &resp); err != nil {
		t.Fatalf("reading response: %v", err)
	}
	if resp.OK {
		t.Error("expected OK=false for bad JSON")
	}
}

// setupTestPlan creates a minimal plan directory structure for testing handleSubmit.
func setupTestPlan(t *testing.T) (projectDir string, planName string) {
	t.Helper()
	projectDir = t.TempDir()
	planName = "test-plan"

	plansDir := filepath.Join(projectDir, ".plans", "active")
	planDir := filepath.Join(plansDir, planName)
	phaseDir := filepath.Join(planDir, "phases", "phase1")

	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write plan.json
	meta := arc.NewPlanMeta(planName, "feature", []string{"phase1"})
	meta.ReviewStatus = "approved"
	data, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Write minimal state.json for phase1
	sf := state.NewStateFile(filepath.Join(phaseDir, "state.json"))
	if err := sf.Write(&arc.PhaseState{
		PhaseStatus:  "pending",
		WorkflowType: "feature",
	}); err != nil {
		t.Fatal(err)
	}

	// Write minimal .arc.yaml
	if err := os.WriteFile(filepath.Join(projectDir, ".arc.yaml"), []byte("project: test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	return projectDir, planName
}

func TestHandleSubmit_Success(t *testing.T) {
	projectDir, planName := setupTestPlan(t)

	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	req := Request{
		Cmd:     "submit",
		Plan:    planName,
		Project: projectDir,
	}

	resp := handleSubmit(req, sched, &cfg)
	if !resp.OK {
		t.Fatalf("expected OK=true, got error: %s", resp.Error)
	}
	if resp.QueuedPhases != 1 {
		t.Errorf("expected 1 queued phase, got %d", resp.QueuedPhases)
	}

	// Verify registration exists
	regs := sched.Registrations()
	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(regs))
	}
	if regs[0].PlanName != planName {
		t.Errorf("expected plan %q, got %q", planName, regs[0].PlanName)
	}

	// Clean up: cancel context to release resources
	regs[0].Cancel()
	planDir := filepath.Join(projectDir, ".plans", "active", planName)
	orchestrator.ReleasePlanLock(planDir)
}

func TestHandleSubmit_PlanNotFound(t *testing.T) {
	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	resp := handleSubmit(Request{
		Cmd:     "submit",
		Plan:    "nonexistent",
		Project: t.TempDir(),
	}, sched, &cfg)
	if resp.OK {
		t.Error("expected OK=false for missing plan")
	}
}

func TestHandleSubmit_NotReviewed(t *testing.T) {
	projectDir, planName := setupTestPlan(t)

	// Overwrite plan.json with unreviewed status
	planDir := filepath.Join(projectDir, ".plans", "active", planName)
	meta := arc.NewPlanMeta(planName, "feature", []string{"phase1"})
	meta.ReviewStatus = "unreviewed"
	data, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(planDir, "plan.json"), data, 0644)

	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	resp := handleSubmit(Request{
		Cmd:     "submit",
		Plan:    planName,
		Project: projectDir,
	}, sched, &cfg)
	if resp.OK {
		t.Error("expected OK=false for unreviewed plan")
	}
	if resp.Error == "" {
		t.Error("expected error message")
	}
}

func TestHandleSubmit_DuplicateReject(t *testing.T) {
	projectDir, planName := setupTestPlan(t)

	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	// First submit succeeds.
	resp := handleSubmit(Request{Cmd: "submit", Plan: planName, Project: projectDir}, sched, &cfg)
	if !resp.OK {
		t.Fatalf("first submit failed: %s", resp.Error)
	}

	// Release the orchestrator lock so the second submit can get past the lock step.
	planDir := filepath.Join(projectDir, ".plans", "active", planName)
	orchestrator.ReleasePlanLock(planDir)

	// Second submit should fail (duplicate registration in scheduler).
	resp = handleSubmit(Request{Cmd: "submit", Plan: planName, Project: projectDir}, sched, &cfg)
	if resp.OK {
		t.Error("expected OK=false for duplicate plan")
	}

	// Clean up
	for _, reg := range sched.Registrations() {
		reg.Cancel()
	}
}

func TestHandleSubmit_WithTimeout(t *testing.T) {
	projectDir, planName := setupTestPlan(t)

	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	resp := handleSubmit(Request{
		Cmd:     "submit",
		Plan:    planName,
		Project: projectDir,
		Timeout: 60,
	}, sched, &cfg)
	if !resp.OK {
		t.Fatalf("submit failed: %s", resp.Error)
	}

	regs := sched.Registrations()
	if len(regs) != 1 {
		t.Fatal("expected 1 registration")
	}
	reg := regs[0]
	if reg.Timeout != 60 {
		t.Errorf("expected timeout 60, got %d", reg.Timeout)
	}

	// Verify context has a deadline
	deadline, ok := reg.Ctx.Deadline()
	if !ok {
		t.Error("expected context to have a deadline")
	}
	if deadline.Before(time.Now()) {
		t.Error("deadline should be in the future")
	}

	reg.Cancel()
	planDir := filepath.Join(projectDir, ".plans", "active", planName)
	orchestrator.ReleasePlanLock(planDir)
}

func TestHandleConnection_SubmitEndToEnd(t *testing.T) {
	projectDir, planName := setupTestPlan(t)

	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	c1, c2 := net.Pipe()
	defer c1.Close()

	go HandleConnection(c2, sched, &cfg)

	_ = WriteMessage(c1, Request{
		Cmd:     "submit",
		Plan:    planName,
		Project: projectDir,
	})

	var resp Response
	if err := ReadMessage(c1, &resp); err != nil {
		t.Fatalf("reading response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected OK, got error: %s", resp.Error)
	}
	if resp.QueuedPhases < 1 {
		t.Errorf("expected at least 1 queued phase, got %d", resp.QueuedPhases)
	}

	// Clean up
	for _, reg := range sched.Registrations() {
		reg.Cancel()
	}
	planDir := filepath.Join(projectDir, ".plans", "active", planName)
	orchestrator.ReleasePlanLock(planDir)
}

func TestHandleSubmit_Draining(t *testing.T) {
	projectDir, planName := setupTestPlan(t)

	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	sched.Drain()

	resp := handleSubmit(Request{Cmd: "submit", Plan: planName, Project: projectDir}, sched, &cfg)
	if resp.OK {
		t.Error("expected OK=false when draining")
	}
}

// TestHandleSubmit_ConditionalReview verifies that plans with "conditional" review pass.
func TestHandleSubmit_ConditionalReview(t *testing.T) {
	projectDir, planName := setupTestPlan(t)

	planDir := filepath.Join(projectDir, ".plans", "active", planName)
	meta := arc.NewPlanMeta(planName, "feature", []string{"phase1"})
	meta.ReviewStatus = "conditional"
	data, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(planDir, "plan.json"), data, 0644)

	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	resp := handleSubmit(Request{Cmd: "submit", Plan: planName, Project: projectDir}, sched, &cfg)
	if !resp.OK {
		t.Fatalf("expected OK for conditional review, got: %s", resp.Error)
	}

	for _, reg := range sched.Registrations() {
		reg.Cancel()
	}
	orchestrator.ReleasePlanLock(planDir)
}

func TestHandleListEmpty(t *testing.T) {
	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	c1, c2 := net.Pipe()
	defer c1.Close()

	go HandleConnection(c2, sched, &cfg)

	_ = WriteMessage(c1, Request{Cmd: "list"})
	var resp Response
	if err := ReadMessage(c1, &resp); err != nil {
		t.Fatalf("reading response: %v", err)
	}
	if !resp.OK {
		t.Errorf("expected OK=true, got error: %s", resp.Error)
	}
	if len(resp.ActivePlans) != 0 {
		t.Errorf("expected 0 active plans, got %d", len(resp.ActivePlans))
	}
}

func TestHandleListWithPlans(t *testing.T) {
	sched := testScheduler()
	cfg := tempDaemonConfig(t)

	// Register two plans directly in the scheduler.
	for _, name := range []string{"plan-b", "plan-a"} {
		reg := makeReg(name, []string{"impl"}, nil)
		reg.ProjectDir = "/proj/" + name
		sched.mu.Lock()
		sched.registrations[name] = reg
		sched.running[name] = nil
		sched.mu.Unlock()
	}

	c1, c2 := net.Pipe()
	defer c1.Close()

	go HandleConnection(c2, sched, &cfg)

	_ = WriteMessage(c1, Request{Cmd: "list"})
	var resp Response
	if err := ReadMessage(c1, &resp); err != nil {
		t.Fatalf("reading response: %v", err)
	}
	if !resp.OK {
		t.Errorf("expected OK=true, got error: %s", resp.Error)
	}
	if len(resp.ActivePlans) != 2 {
		t.Fatalf("expected 2 active plans, got %d", len(resp.ActivePlans))
	}
	// Sorted: plan-a first.
	if resp.ActivePlans[0].PlanName != "plan-a" {
		t.Errorf("ActivePlans[0].PlanName: got %q, want plan-a", resp.ActivePlans[0].PlanName)
	}
	if resp.ActivePlans[1].PlanName != "plan-b" {
		t.Errorf("ActivePlans[1].PlanName: got %q, want plan-b", resp.ActivePlans[1].PlanName)
	}
}
