package dev

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/plan"
)

var phaseNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// GenerateOptions configures plan generation from discovery/architect results.
type GenerateOptions struct {
	PlanName  string
	PlansDir  string
	Discovery *DiscoveryResult
	Proposal  *ArchitectProposal // nil for simple/medium tasks
}

// GeneratePlan creates a complete arc plan from discovery results.
func GeneratePlan(opts GenerateOptions) (*arc.PlanMeta, error) {
	if opts.Discovery == nil {
		return nil, fmt.Errorf("discovery result is required")
	}
	if opts.PlanName == "" {
		return nil, fmt.Errorf("plan name cannot be empty")
	}

	switch opts.Discovery.Complexity {
	case ComplexitySimple:
		return generateSimplePlan(opts)
	case ComplexityMedium:
		return generateMediumPlan(opts)
	case ComplexityComplex:
		return generateComplexPlan(opts)
	default:
		return nil, fmt.Errorf("unsupported complexity: %q", opts.Discovery.Complexity)
	}
}

func generateSimplePlan(opts GenerateOptions) (*arc.PlanMeta, error) {
	phases := make([]string, len(opts.Discovery.SuggestedPhases))
	planContent := make(map[string]string)

	for i, p := range opts.Discovery.SuggestedPhases {
		phases[i] = p.Name
		planContent[p.Name] = GenerateSimplePlanMD(opts.Discovery)
	}

	workflowType := opts.Discovery.WorkflowType
	if workflowType == "" {
		workflowType = "direct"
	}

	return plan.Create(plan.CreateOptions{
		PlansDir:     opts.PlansDir,
		Name:         opts.PlanName,
		Phases:       phases,
		WorkflowType: workflowType,
		PlanContent:  planContent,
	})
}

func generateMediumPlan(opts GenerateOptions) (*arc.PlanMeta, error) {
	phases := make([]string, len(opts.Discovery.SuggestedPhases))
	planContent := make(map[string]string)

	for i, p := range opts.Discovery.SuggestedPhases {
		phases[i] = p.Name
		planContent[p.Name] = GeneratePhasePlanMD(opts.Discovery, p)
	}

	workflowType := opts.Discovery.WorkflowType
	if workflowType == "" {
		workflowType = "feature"
	}

	return plan.Create(plan.CreateOptions{
		PlansDir:     opts.PlansDir,
		Name:         opts.PlanName,
		Phases:       phases,
		WorkflowType: workflowType,
		PlanContent:  planContent,
	})
}

func generateComplexPlan(opts GenerateOptions) (*arc.PlanMeta, error) {
	if opts.Proposal == nil {
		return nil, fmt.Errorf("complex tasks require an architect proposal")
	}

	phases := make([]string, len(opts.Proposal.SuggestedPhases))
	for i, p := range opts.Proposal.SuggestedPhases {
		phases[i] = p.Name
	}

	customWorkflow, err := BuildCustomWorkflow("custom", opts.Proposal.SuggestedPhases)
	if err != nil {
		return nil, fmt.Errorf("build custom workflow: %w", err)
	}

	// Use a valid built-in workflow type for plan.Create validation,
	// but supply the custom workflow YAML to override it.
	workflowType := opts.Discovery.WorkflowType
	if workflowType == "" || workflowType == "custom" {
		workflowType = "feature"
	}

	return plan.Create(plan.CreateOptions{
		PlansDir:       opts.PlansDir,
		Name:           opts.PlanName,
		Phases:         phases,
		WorkflowType:   workflowType,
		PlanContent:    opts.Proposal.PlanContent,
		CustomWorkflow: customWorkflow,
	})
}

// GenerateSimplePlanMD generates plan.md content for a simple task's execute phase.
func GenerateSimplePlanMD(discovery *DiscoveryResult) string {
	var b strings.Builder

	b.WriteString("## Objective\n\n")
	b.WriteString(discovery.TaskSummary)
	b.WriteString("\n")

	if discovery.Approach != "" {
		b.WriteString("\n## Approach\n\n")
		b.WriteString(discovery.Approach)
		b.WriteString("\n")
	}

	if len(discovery.RelevantFiles) > 0 {
		b.WriteString("\n## Relevant Files\n\n")
		for _, f := range discovery.RelevantFiles {
			b.WriteString("- `")
			b.WriteString(f.Path)
			b.WriteString("`")
			if f.Description != "" {
				b.WriteString(" — ")
				b.WriteString(f.Description)
			}
			b.WriteString("\n")
		}
	}

	if len(discovery.Requirements) > 0 {
		b.WriteString("\n## Requirements\n\n")
		for _, r := range discovery.Requirements {
			b.WriteString("- ")
			b.WriteString(r)
			b.WriteString("\n")
		}
	}

	return b.String()
}

// GeneratePhasePlanMD generates plan.md content for a medium task's phase.
func GeneratePhasePlanMD(discovery *DiscoveryResult, phase PhaseSpec) string {
	var b strings.Builder

	b.WriteString("## Phase: ")
	b.WriteString(phase.Name)
	b.WriteString("\n\n")

	if phase.Description != "" {
		b.WriteString(phase.Description)
		b.WriteString("\n")
	}

	b.WriteString("\n## Context\n\n")
	b.WriteString(discovery.TaskSummary)
	b.WriteString("\n")

	if len(discovery.RelevantFiles) > 0 {
		b.WriteString("\n## Relevant Files\n\n")
		for _, f := range discovery.RelevantFiles {
			b.WriteString("- `")
			b.WriteString(f.Path)
			b.WriteString("`")
			if f.Description != "" {
				b.WriteString(" — ")
				b.WriteString(f.Description)
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// BuildCustomWorkflow generates a workflow YAML definition for complex tasks.
func BuildCustomWorkflow(name string, phases []PhaseSpec) ([]byte, error) {
	if len(phases) == 0 {
		return nil, fmt.Errorf("cannot build workflow with zero phases")
	}

	// Validate phase names
	seen := make(map[string]bool)
	reserved := map[string]bool{"complete": true, "blocked": true}

	for _, p := range phases {
		if !phaseNameRe.MatchString(p.Name) {
			return nil, fmt.Errorf("invalid phase name %q: must be lowercase with hyphens, no spaces", p.Name)
		}
		if reserved[p.Name] {
			return nil, fmt.Errorf("phase name %q conflicts with terminal state", p.Name)
		}
		if seen[p.Name] {
			return nil, fmt.Errorf("duplicate phase name %q", p.Name)
		}
		seen[p.Name] = true
	}

	var b strings.Builder

	b.WriteString("name: ")
	b.WriteString(name)
	b.WriteString("\nversion: 1\n")
	b.WriteString("description: Custom workflow generated by arc dev\n")
	b.WriteString("\nstates:\n")

	for i, p := range phases {
		reviewName := p.Name + "_review"
		var nextAfterReview string
		if i < len(phases)-1 {
			nextAfterReview = phases[i+1].Name
		} else {
			nextAfterReview = "complete"
		}

		// Work state
		b.WriteString("  - name: ")
		b.WriteString(p.Name)
		b.WriteString("\n")
		b.WriteString("    description: ")
		b.WriteString(yamlEscapeString(p.Description))
		b.WriteString("\n")
		b.WriteString("    prompt: prompts/feature/impl.md\n")
		b.WriteString("    next: ")
		b.WriteString(reviewName)
		b.WriteString("\n\n")

		// Review state
		b.WriteString("  - name: ")
		b.WriteString(reviewName)
		b.WriteString("\n")
		b.WriteString("    description: Review ")
		b.WriteString(p.Name)
		b.WriteString("\n")
		b.WriteString("    prompt: prompts/feature/impl-review.md\n")
		b.WriteString("    verdicts:\n")
		b.WriteString("      - approved\n")
		b.WriteString("      - concerns\n")
		b.WriteString("    next:\n")
		b.WriteString("      approved: ")
		b.WriteString(nextAfterReview)
		b.WriteString("\n")
		b.WriteString("      concerns: ")
		b.WriteString(p.Name)
		b.WriteString("\n\n")
	}

	// Terminal states
	b.WriteString("  - name: complete\n")
	b.WriteString("    description: Task completed\n")
	b.WriteString("    prompt: prompts/common/complete.md\n\n")

	b.WriteString("  - name: blocked\n")
	b.WriteString("    description: Task blocked\n")
	b.WriteString("    prompt: prompts/common/blocked.md\n\n")

	b.WriteString("entry_state: ")
	b.WriteString(phases[0].Name)
	b.WriteString("\n")
	b.WriteString("terminal_states: [complete, blocked]\n")

	return []byte(b.String()), nil
}

// yamlEscapeString handles simple YAML escaping for description strings.
func yamlEscapeString(s string) string {
	if s == "" {
		return "''"
	}
	// If it contains characters that need quoting, wrap in quotes
	if strings.ContainsAny(s, ":#{}[]&*?|>!%@`\"'\n\\") {
		escaped := strings.ReplaceAll(s, "'", "''")
		return "'" + escaped + "'"
	}
	return s
}
