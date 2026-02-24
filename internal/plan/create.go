package plan

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/resources"
)

var planNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

// CreateOptions configures plan creation.
type CreateOptions struct {
	PlansDir       string
	Name           string
	Phases         []string
	WorkflowType   string
	PlanContent    map[string]string // optional: phase name → plan.md content (skip template)
	CustomWorkflow []byte            // optional: custom workflow YAML (written as workflow.yaml)
}

// Create creates a new plan with directory structure, state files, and templates.
func Create(opts CreateOptions) (*arc.PlanMeta, error) {
	// 1. Validate name
	if len(opts.Name) < 2 || !planNameRe.MatchString(opts.Name) {
		return nil, fmt.Errorf("invalid plan name %q: must match ^[a-z][a-z0-9-]*[a-z0-9]$ (min 2 chars)", opts.Name)
	}

	// 2. Validate phases
	if len(opts.Phases) == 0 {
		return nil, fmt.Errorf("no phases specified")
	}

	seen := make(map[string]bool)
	for _, p := range opts.Phases {
		if seen[p] {
			return nil, fmt.Errorf("duplicate phase %q", p)
		}
		seen[p] = true
	}

	// Default workflow type
	if opts.WorkflowType == "" {
		opts.WorkflowType = "feature"
	}

	// Validate workflow type exists (for non-feature, we'll copy it; for feature, just check)
	validWorkflows := resources.ListWorkflows()
	found := false
	for _, w := range validWorkflows {
		if w == opts.WorkflowType {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("invalid workflow type %q", opts.WorkflowType)
	}

	// 3. Check plan doesn't already exist
	// Truncate directory name to 255 bytes (filesystem limit)
	dirName := opts.Name
	if len(dirName) > 255 {
		dirName = dirName[:255]
	}
	planDir := filepath.Join(opts.PlansDir, dirName)
	if _, err := os.Stat(planDir); err == nil {
		return nil, fmt.Errorf("plan %q already exists", opts.Name)
	}

	// 4. Create plan directory
	if err := os.MkdirAll(planDir, 0755); err != nil {
		return nil, fmt.Errorf("create plan directory: %w", err)
	}

	// 5. Write session_id (UUID v4)
	sessionID, err := generateUUIDv4()
	if err != nil {
		return nil, fmt.Errorf("generate session ID: %w", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "session_id"), []byte(sessionID+"\n"), 0644); err != nil {
		return nil, fmt.Errorf("write session_id: %w", err)
	}

	// 6. Copy workflow YAML for non-feature types, or write custom workflow
	if len(opts.CustomWorkflow) > 0 {
		if err := os.WriteFile(filepath.Join(planDir, "workflow.yaml"), opts.CustomWorkflow, 0644); err != nil {
			return nil, fmt.Errorf("write workflow.yaml: %w", err)
		}
	} else if opts.WorkflowType != "feature" {
		workflowData, err := resources.WorkflowBytes(opts.WorkflowType)
		if err != nil {
			return nil, fmt.Errorf("read workflow %s: %w", opts.WorkflowType, err)
		}
		if err := os.WriteFile(filepath.Join(planDir, "workflow.yaml"), workflowData, 0644); err != nil {
			return nil, fmt.Errorf("write workflow.yaml: %w", err)
		}
	}

	// 7. Build metadata
	meta := arc.NewPlanMeta(opts.Name, opts.WorkflowType, opts.Phases)

	// 8. Write plan.json
	planData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal plan.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), planData, 0644); err != nil {
		return nil, fmt.Errorf("write plan.json: %w", err)
	}

	// 9. Create phase directories with state.json and plan.md
	planTemplate, err := resources.TemplateBytes("plan-template.md")
	if err != nil {
		return nil, fmt.Errorf("read plan template: %w", err)
	}

	for _, phase := range opts.Phases {
		phaseDir := filepath.Join(planDir, "phases", phase)
		if err := os.MkdirAll(phaseDir, 0755); err != nil {
			return nil, fmt.Errorf("create phase directory %s: %w", phase, err)
		}

		// Write state.json
		state := arc.NewPhaseState(opts.Name, phase, opts.WorkflowType)
		stateData, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal state.json for %s: %w", phase, err)
		}
		if err := os.WriteFile(filepath.Join(phaseDir, "state.json"), stateData, 0644); err != nil {
			return nil, fmt.Errorf("write state.json for %s: %w", phase, err)
		}

		// Write plan.md — use custom content if provided, otherwise default template
		planMD := planTemplate
		if opts.PlanContent != nil {
			if content, ok := opts.PlanContent[phase]; ok && content != "" {
				planMD = []byte(content)
			}
		}
		if err := os.WriteFile(filepath.Join(phaseDir, "plan.md"), planMD, 0644); err != nil {
			return nil, fmt.Errorf("write plan.md for %s: %w", phase, err)
		}
	}

	return meta, nil
}

func generateUUIDv4() (string, error) {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		return "", err
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}
