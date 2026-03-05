package gate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nwiley/arc/internal/arc"
)

const gateStatusFile = "gate-status.json"

// WriteStatus writes gate-status.json to phaseDir.
// If gate-status.json already exists, the run_count from the existing file is
// preserved; all other fields reflect the current result.
func WriteStatus(phaseDir string, result *arc.GateResult) error {
	statusPath := filepath.Join(phaseDir, gateStatusFile)

	// Load existing status to preserve run_count.
	existing, _ := ReadStatus(phaseDir) // ignore error — file may not exist yet
	runCount := 1
	if existing != nil {
		runCount = existing.RunCount
	}

	cps := make(map[string]string, len(result.Checkpoints))
	for _, cp := range result.Checkpoints {
		cps[cp.Name] = cp.Status
	}

	status := &arc.GateStatus{
		LastRun:     time.Now().UTC().Format(time.RFC3339),
		RunCount:    runCount,
		Passed:      result.Passed,
		Checkpoints: cps,
	}

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling gate status: %w", err)
	}
	// Backup existing file before overwriting
	if _, statErr := os.Stat(statusPath); statErr == nil {
		backupPath := statusPath + ".bak"
		// Best-effort backup — don't fail the write if backup fails
		if existing, readErr := os.ReadFile(statusPath); readErr == nil {
			os.WriteFile(backupPath, existing, 0o644)
		}
	}
	if err := os.WriteFile(statusPath, data, 0o644); err != nil {
		return fmt.Errorf("writing gate-status.json to %q: %w", phaseDir, err)
	}
	return nil
}

// ReadStatus reads gate-status.json from phaseDir.
// Returns an error if the file does not exist or cannot be parsed.
func ReadStatus(phaseDir string) (*arc.GateStatus, error) {
	statusPath := filepath.Join(phaseDir, gateStatusFile)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return nil, fmt.Errorf("reading gate-status.json from %q: %w", phaseDir, err)
	}
	var status arc.GateStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("parsing gate-status.json in %q: %w", phaseDir, err)
	}
	if status.Checkpoints == nil {
		status.Checkpoints = make(map[string]string)
	}
	return &status, nil
}

// IncrementRunCount reads the existing gate-status.json, increments the
// run_count field, writes it back, and returns the new count.
// If the file does not exist, the count starts at 1.
func IncrementRunCount(phaseDir string) (int, error) {
	statusPath := filepath.Join(phaseDir, gateStatusFile)

	existing, err := ReadStatus(phaseDir)
	if err != nil {
		// File may not exist yet — start fresh.
		status := &arc.GateStatus{
			LastRun:     time.Now().UTC().Format(time.RFC3339),
			RunCount:    1,
			Checkpoints: make(map[string]string),
		}
		data, merr := json.MarshalIndent(status, "", "  ")
		if merr != nil {
			return 0, fmt.Errorf("marshalling initial gate status: %w", merr)
		}
		if werr := os.WriteFile(statusPath, data, 0o644); werr != nil {
			return 0, fmt.Errorf("writing gate-status.json: %w", werr)
		}
		return 1, nil
	}

	existing.RunCount++
	existing.LastRun = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshalling gate status: %w", err)
	}
	if err := os.WriteFile(statusPath, data, 0o644); err != nil {
		return 0, fmt.Errorf("writing gate-status.json: %w", err)
	}
	return existing.RunCount, nil
}
