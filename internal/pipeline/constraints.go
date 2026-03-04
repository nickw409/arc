package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwiley/arc/internal/arc"
)

// CheckPreConstraints validates constraints before a state executes.
func CheckPreConstraints(state *arc.PhaseState, constraints *arc.ConstraintConfig, phaseDir string) error {
	if constraints == nil {
		return nil
	}

	if constraints.MaxIterations > 0 && state.Iteration.Current >= constraints.MaxIterations {
		return fmt.Errorf("max iterations reached (%d >= %d)", state.Iteration.Current, constraints.MaxIterations)
	}

	for _, artifact := range constraints.RequireArtifactsIn {
		if err := checkArtifact(phaseDir, artifact); err != nil {
			return err
		}
	}

	return nil
}

// CheckPostConstraints validates constraints after a state executes.
func CheckPostConstraints(constraints *arc.ConstraintConfig, phaseDir string) error {
	if constraints == nil {
		return nil
	}

	for _, artifact := range constraints.RequireArtifactsOut {
		if err := checkArtifact(phaseDir, artifact); err != nil {
			return err
		}
	}

	return nil
}

func checkArtifact(phaseDir, artifact string) error {
	if strings.Contains(artifact, "..") {
		return fmt.Errorf("required artifact path contains '..': %s", artifact)
	}

	fullPath := filepath.Join(phaseDir, artifact)
	resolved, err := filepath.Abs(fullPath)
	if err != nil {
		return fmt.Errorf("required artifact: invalid path %s: %w", artifact, err)
	}
	absPhaseDir, err := filepath.Abs(phaseDir)
	if err != nil {
		return fmt.Errorf("required artifact: invalid phase dir: %w", err)
	}
	if !strings.HasPrefix(resolved, absPhaseDir+string(os.PathSeparator)) && resolved != absPhaseDir {
		return fmt.Errorf("required artifact path traversal: %s", artifact)
	}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("required artifact missing: %s", artifact)
	}
	return nil
}
