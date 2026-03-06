package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/plan"
	"gopkg.in/yaml.v3"
)

// --- plan command --role flag tests ---

func TestNewPlanCmd_RoleFlagAccepted(t *testing.T) {
	cmd := newPlanCmd()
	if err := cmd.ParseFlags([]string{"--role", "review"}); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
	val, err := cmd.Flags().GetString("role")
	if err != nil {
		t.Fatalf("getting role flag: %v", err)
	}
	if val != "review" {
		t.Errorf("role = %q, want %q", val, "review")
	}
}

func TestNewPlanCmd_RoleFlagDefault(t *testing.T) {
	cmd := newPlanCmd()
	val, err := cmd.Flags().GetString("role")
	if err != nil {
		t.Fatalf("getting role flag: %v", err)
	}
	if val != "" {
		t.Errorf("role default = %q, want empty string", val)
	}
}

func TestNewPlanCmd_RoleFlagAllValues(t *testing.T) {
	for _, role := range []string{"impl", "review", "investigate", "audit"} {
		cmd := newPlanCmd()
		if err := cmd.ParseFlags([]string{"--role", role}); err != nil {
			t.Fatalf("unexpected flag parse error for role %q: %v", role, err)
		}
		val, _ := cmd.Flags().GetString("role")
		if val != role {
			t.Errorf("role = %q, want %q", val, role)
		}
	}
}

// --- add-phase --role flag tests ---

func TestNewPlanAddPhaseCmd_RoleFlagAccepted(t *testing.T) {
	cmd := newPlanAddPhaseCmd()
	if err := cmd.ParseFlags([]string{"--role", "audit"}); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
	val, err := cmd.Flags().GetString("role")
	if err != nil {
		t.Fatalf("getting role flag: %v", err)
	}
	if val != "audit" {
		t.Errorf("role = %q, want %q", val, "audit")
	}
}

// --- update-phase --role flag tests ---

func TestNewPlanUpdatePhaseCmd_RoleFlagAccepted(t *testing.T) {
	cmd := newPlanUpdatePhaseCmd()
	if err := cmd.ParseFlags([]string{"--role", "investigate"}); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
	val, err := cmd.Flags().GetString("role")
	if err != nil {
		t.Fatalf("getting role flag: %v", err)
	}
	if val != "investigate" {
		t.Errorf("role = %q, want %q", val, "investigate")
	}
}

// --- show-spec includes role ---

func TestShowSpec_IncludesRole(t *testing.T) {
	// Create a temp plan with a phase that has a role set
	dir := t.TempDir()
	plansDir := filepath.Join(dir, ".plans", "active")

	// Create the plan structure manually
	phaseDir := filepath.Join(plansDir, "test-plan", "phases", "phase-a")
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write plan.json
	meta := arc.NewPlanMeta("test-plan", "feature", []string{"phase-a"})
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(plansDir, "test-plan", "plan.json"), metaData, 0644)

	// Write state.json
	state := arc.NewPhaseState("test-plan", "phase-a", "feature")
	stateData, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(filepath.Join(phaseDir, "state.json"), stateData, 0644)

	// Write spec.yaml with a role
	spec := &arc.PhaseSpec{
		Spec: "Test phase",
		Role: "review",
	}
	if err := plan.WriteSpec(plansDir, "test-plan", "phase-a", spec); err != nil {
		t.Fatal(err)
	}

	// Read it back and marshal — verify role appears in YAML output
	readSpec, err := plan.ReadSpec(plansDir, "test-plan", "phase-a")
	if err != nil {
		t.Fatal(err)
	}

	data, err := yaml.Marshal(readSpec)
	if err != nil {
		t.Fatal(err)
	}

	output := string(data)
	if !strings.Contains(output, "role: review") {
		t.Errorf("show-spec output missing role field:\n%s", output)
	}
}

func TestShowSpec_OmitsEmptyRole(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, ".plans", "active")

	phaseDir := filepath.Join(plansDir, "test-plan", "phases", "phase-a")
	if err := os.MkdirAll(phaseDir, 0755); err != nil {
		t.Fatal(err)
	}

	spec := &arc.PhaseSpec{
		Spec: "Test phase",
	}
	if err := plan.WriteSpec(plansDir, "test-plan", "phase-a", spec); err != nil {
		t.Fatal(err)
	}

	readSpec, err := plan.ReadSpec(plansDir, "test-plan", "phase-a")
	if err != nil {
		t.Fatal(err)
	}

	data, err := yaml.Marshal(readSpec)
	if err != nil {
		t.Fatal(err)
	}

	output := string(data)
	if strings.Contains(output, "role:") {
		t.Errorf("show-spec output should omit empty role field:\n%s", output)
	}
}
