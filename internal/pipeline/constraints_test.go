package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestCheckPreConstraintsMaxIterations(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 25

	constraints := &arc.ConstraintConfig{MaxIterations: 25}
	phaseDir := t.TempDir()

	err := CheckPreConstraints(state, constraints, phaseDir)
	if err == nil {
		t.Fatal("expected error for max iterations reached")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "max iterations") {
		t.Fatalf("expected error containing 'max iterations', got: %v", err)
	}
}

func TestCheckPreConstraintsUnderMax(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 24

	constraints := &arc.ConstraintConfig{MaxIterations: 25}
	phaseDir := t.TempDir()

	err := CheckPreConstraints(state, constraints, phaseDir)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheckPreConstraintsArtifactsInExists(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	// Create the artifact
	if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	constraints := &arc.ConstraintConfig{RequireArtifactsIn: []string{"plan.md"}}

	err := CheckPreConstraints(state, constraints, phaseDir)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheckPreConstraintsArtifactsInMissing(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	constraints := &arc.ConstraintConfig{RequireArtifactsIn: []string{"missing.md"}}

	err := CheckPreConstraints(state, constraints, phaseDir)
	if err == nil {
		t.Fatal("expected error for missing artifact")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "required artifact") {
		t.Fatalf("expected error containing 'required artifact', got: %v", err)
	}
}

func TestCheckPreConstraintsNil(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	err := CheckPreConstraints(state, nil, phaseDir)
	if err != nil {
		t.Fatalf("expected nil for nil constraints, got %v", err)
	}
}

func TestCheckPostConstraintsNil(t *testing.T) {
	phaseDir := t.TempDir()

	err := CheckPostConstraints(nil, phaseDir)
	if err != nil {
		t.Fatalf("expected nil for nil constraints, got %v", err)
	}
}

func TestCheckPostConstraintsArtifactsOut(t *testing.T) {
	phaseDir := t.TempDir()

	// Create the artifact
	if err := os.WriteFile(filepath.Join(phaseDir, "output.md"), []byte("output"), 0644); err != nil {
		t.Fatal(err)
	}

	constraints := &arc.ConstraintConfig{RequireArtifactsOut: []string{"output.md"}}

	err := CheckPostConstraints(constraints, phaseDir)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheckPostConstraintsArtifactsOutMissing(t *testing.T) {
	phaseDir := t.TempDir()

	constraints := &arc.ConstraintConfig{RequireArtifactsOut: []string{"output.md"}}

	err := CheckPostConstraints(constraints, phaseDir)
	if err == nil {
		t.Fatal("expected error for missing artifact")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "required artifact") {
		t.Fatalf("expected error containing 'required artifact', got: %v", err)
	}
}

func TestCheckPreConstraintsMaxZero(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	state.Iteration.Current = 5

	constraints := &arc.ConstraintConfig{MaxIterations: 0}
	phaseDir := t.TempDir()

	err := CheckPreConstraints(state, constraints, phaseDir)
	if err != nil {
		t.Fatalf("expected nil (0 means no limit), got %v", err)
	}
}

func TestCheckPreConstraintsMultipleArtifacts(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	// Create a.md and c.md but NOT b.md
	if err := os.WriteFile(filepath.Join(phaseDir, "a.md"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(phaseDir, "c.md"), []byte("c"), 0644); err != nil {
		t.Fatal(err)
	}

	constraints := &arc.ConstraintConfig{RequireArtifactsIn: []string{"a.md", "b.md", "c.md"}}

	err := CheckPreConstraints(state, constraints, phaseDir)
	if err == nil {
		t.Fatal("expected error for missing b.md")
	}
	if !strings.Contains(err.Error(), "b.md") {
		t.Fatalf("expected error containing 'b.md', got: %v", err)
	}
}

func TestCheckPostConstraintsMultipleArtifacts(t *testing.T) {
	phaseDir := t.TempDir()

	// Create out1.md but NOT out2.md
	if err := os.WriteFile(filepath.Join(phaseDir, "out1.md"), []byte("out"), 0644); err != nil {
		t.Fatal(err)
	}

	constraints := &arc.ConstraintConfig{RequireArtifactsOut: []string{"out1.md", "out2.md"}}

	err := CheckPostConstraints(constraints, phaseDir)
	if err == nil {
		t.Fatal("expected error for missing out2.md")
	}
	if !strings.Contains(err.Error(), "out2.md") {
		t.Fatalf("expected error containing 'out2.md', got: %v", err)
	}
}

func TestCheckPreConstraintsPathTraversal(t *testing.T) {
	phaseDir := t.TempDir()
	state := arc.NewPhaseState("plan", "phase", "feature")

	constraints := &arc.ConstraintConfig{RequireArtifactsIn: []string{"../../etc/passwd"}}

	err := CheckPreConstraints(state, constraints, phaseDir)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestCheckPostConstraintsPathTraversal(t *testing.T) {
	phaseDir := t.TempDir()

	constraints := &arc.ConstraintConfig{RequireArtifactsOut: []string{"../../etc/passwd"}}

	err := CheckPostConstraints(constraints, phaseDir)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestCheckPreConstraintsEmptyArtifactsIn(t *testing.T) {
	state := arc.NewPhaseState("plan", "phase", "feature")
	phaseDir := t.TempDir()

	constraints := &arc.ConstraintConfig{RequireArtifactsIn: []string{}}

	err := CheckPreConstraints(state, constraints, phaseDir)
	if err != nil {
		t.Fatalf("expected nil for empty artifacts list, got %v", err)
	}
}
