package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"

	"github.com/nwiley/arc/internal/arc"
)

// MigrateOptions configures migration.
type MigrateOptions struct {
	PlansDir string
	DryRun   bool
}

// MigrateResult reports migration outcomes.
type MigrateResult struct {
	PlansFound    int
	PlansMigrated int
	Errors        []string
}

// Migrate scans plansDir for plans and normalizes their state files.
func Migrate(opts MigrateOptions) (*MigrateResult, error) {
	result := &MigrateResult{Errors: []string{}}

	entries, err := os.ReadDir(opts.PlansDir)
	if err != nil {
		return nil, fmt.Errorf("reading plans directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		result.PlansFound++

		planDir := filepath.Join(opts.PlansDir, entry.Name())
		if err := MigratePlan(planDir, opts.DryRun); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", entry.Name(), err))
		} else {
			result.PlansMigrated++
		}
	}

	return result, nil
}

// MigratePlan migrates a single plan directory.
func MigratePlan(planDir string, dryRun bool) error {
	phasesDir := filepath.Join(planDir, "phases")
	entries, err := os.ReadDir(phasesDir)
	if err != nil {
		return fmt.Errorf("reading phases directory: %w", err)
	}

	migrated := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		statePath := filepath.Join(phasesDir, entry.Name(), "state.json")
		data, err := os.ReadFile(statePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("reading %s: %w", statePath, err)
		}

		normalized, err := MigrateState(data)
		if err != nil {
			return fmt.Errorf("migrating %s: %w", entry.Name(), err)
		}

		newData, err := json.MarshalIndent(normalized, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling %s: %w", entry.Name(), err)
		}
		newData = append(newData, '\n')

		// Check if anything changed
		var original interface{}
		var updated interface{}
		json.Unmarshal(data, &original)
		json.Unmarshal(newData, &updated)
		if reflect.DeepEqual(original, updated) {
			continue
		}

		migrated = true
		if dryRun {
			fmt.Printf("  Would migrate: %s/%s/state.json\n", filepath.Base(planDir), entry.Name())
			continue
		}

		// Create backup
		backupPath := statePath + ".bak"
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("creating backup for %s: %w", entry.Name(), err)
		}

		// Write normalized
		if err := os.WriteFile(statePath, newData, 0644); err != nil {
			return fmt.Errorf("writing normalized %s: %w", entry.Name(), err)
		}

		fmt.Printf("  Migrated: %s/%s/state.json\n", filepath.Base(planDir), entry.Name())
	}

	if !migrated && !dryRun {
		// nothing to report
	}

	return nil
}

// MigrateState reads old-format state.json bytes and returns a normalized PhaseState.
func MigrateState(data []byte) (*arc.PhaseState, error) {
	// Step 1: Unmarshal into flexible map
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	// Start with defaults
	defaults := arc.NewPhaseState("", "", "feature")

	// Step 2: Extract known fields with type conversion
	result := &arc.PhaseState{
		Phase:        getStringField(raw, "phase", defaults.Phase),
		Plan:         getStringField(raw, "plan", defaults.Plan),
		WorkflowType: getStringField(raw, "workflow_type", defaults.WorkflowType),
		PhaseStatus:  getStringField(raw, "phase_status", defaults.PhaseStatus),
		CurrentState: getStringField(raw, "current_state", defaults.CurrentState),
		LastVerdict:  getStringField(raw, "last_verdict", defaults.LastVerdict),
		LastCommit:   getStringField(raw, "last_commit", defaults.LastCommit),
		ModelOverride: getStringField(raw, "model_override", defaults.ModelOverride),
	}

	// Int fields
	result.TestsPassing = getIntField(raw, "tests_passing", defaults.TestsPassing)
	result.TestsTotal = getIntField(raw, "tests_total", defaults.TestsTotal)
	result.StuckIterations = getIntField(raw, "stuck_iterations", defaults.StuckIterations)
	result.HangCount = getIntField(raw, "hang_count", defaults.HangCount)
	result.LastReviewedIter = getIntField(raw, "last_reviewed_iteration", defaults.LastReviewedIter)
	result.LastQAReviewedIter = getIntField(raw, "last_qa_reviewed_iteration", defaults.LastQAReviewedIter)
	result.RollbackCount = getIntField(raw, "rollback_count", defaults.RollbackCount)
	result.GlobalIterations = getIntField(raw, "global_iterations", defaults.GlobalIterations)

	// Nested struct fields: re-marshal and unmarshal
	result.Iteration = extractNested[arc.Iteration](raw, "iteration", defaults.Iteration)
	result.Chunks = extractNested[arc.Chunks](raw, "chunks", defaults.Chunks)
	result.Blocked = extractNested[arc.BlockedInfo](raw, "blocked", defaults.Blocked)

	// Pointer fields: handle string "null"
	result.ParallelExecution = extractNullablePtr[arc.ParallelExec](raw, "parallel_execution")
	result.InterventionRequest = extractNullablePtr[arc.Intervention](raw, "intervention_request")

	// Slice fields: ensure non-nil
	result.Packages = extractStringSlice(raw, "packages", defaults.Packages)
	result.TestFiles = extractStringSlice(raw, "test_files", defaults.TestFiles)
	result.ExecutedEscalations = extractStringSlice(raw, "executed_escalations", defaults.ExecutedEscalations)
	result.Disputes = extractSlice[arc.Dispute](raw, "disputes", defaults.Disputes)
	result.LastClearedDisputes = extractSlice[arc.Dispute](raw, "last_cleared_disputes", defaults.LastClearedDisputes)
	result.VerdictsHistory = extractSlice[arc.VerdictEntry](raw, "verdicts_history", defaults.VerdictsHistory)

	// Ensure workflow type has a default
	if result.WorkflowType == "" {
		result.WorkflowType = "feature"
	}

	// Ensure iteration.Max has a sane default
	if result.Iteration.Max == 0 {
		result.Iteration.Max = 25
	}

	// Ensure Chunks slices are non-nil
	if result.Chunks.Completed == nil {
		result.Chunks.Completed = []arc.ChunkResult{}
	}
	if result.Chunks.Remaining == nil {
		result.Chunks.Remaining = []int{}
	}

	return result, nil
}

func getStringField(raw map[string]interface{}, key, def string) string {
	v, ok := raw[key]
	if !ok || v == nil {
		return def
	}
	if s, ok := v.(string); ok {
		if s == "null" {
			return def
		}
		return s
	}
	return def
}

func getIntField(raw map[string]interface{}, key string, def int) int {
	v, ok := raw[key]
	if !ok || v == nil {
		return def
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case string:
		n, err := strconv.Atoi(val)
		if err != nil {
			return 0
		}
		return n
	}
	return def
}

func extractNested[T any](raw map[string]interface{}, key string, def T) T {
	v, ok := raw[key]
	if !ok || v == nil {
		return def
	}
	// If it's a string (e.g., "null" or wrong type), return default
	if _, isStr := v.(string); isStr {
		return def
	}
	data, err := json.Marshal(v)
	if err != nil {
		return def
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return def
	}
	return result
}

func extractNullablePtr[T any](raw map[string]interface{}, key string) *T {
	v, ok := raw[key]
	if !ok || v == nil {
		return nil
	}
	if s, isStr := v.(string); isStr && s == "null" {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return &result
}

func extractStringSlice(raw map[string]interface{}, key string, def []string) []string {
	v, ok := raw[key]
	if !ok || v == nil {
		return def
	}
	if _, isStr := v.(string); isStr {
		return []string{}
	}
	arr, ok := v.([]interface{})
	if !ok {
		return def
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func extractSlice[T any](raw map[string]interface{}, key string, def []T) []T {
	v, ok := raw[key]
	if !ok || v == nil {
		return def
	}
	if _, isStr := v.(string); isStr {
		return []T{}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return def
	}
	var result []T
	if err := json.Unmarshal(data, &result); err != nil {
		return def
	}
	if result == nil {
		return []T{}
	}
	return result
}
