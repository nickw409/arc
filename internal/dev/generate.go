package dev

import (
	"fmt"
	"strings"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/plan"
)

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

	// Each phase runs its own workflow independently. Sequencing between
	// phases is handled by the orchestrator via phase dependencies, not by
	// embedding all phases as states in a single workflow.
	workflowType := opts.Discovery.WorkflowType
	if workflowType == "" || workflowType == "custom" {
		workflowType = "feature"
	}

	return plan.Create(plan.CreateOptions{
		PlansDir:     opts.PlansDir,
		Name:         opts.PlanName,
		Phases:       phases,
		WorkflowType: workflowType,
		PlanContent:  opts.Proposal.PlanContent,
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

	if len(discovery.Clarifications) > 0 {
		b.WriteString("\n## Clarifications\n\n")
		for _, c := range discovery.Clarifications {
			b.WriteString("**Q:** ")
			b.WriteString(c.Question)
			b.WriteString("\n**A:** ")
			b.WriteString(c.Answer)
			b.WriteString("\n\n")
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

	if len(discovery.Clarifications) > 0 {
		b.WriteString("\n## Clarifications\n\n")
		for _, c := range discovery.Clarifications {
			b.WriteString("**Q:** ")
			b.WriteString(c.Question)
			b.WriteString("\n**A:** ")
			b.WriteString(c.Answer)
			b.WriteString("\n\n")
		}
	}

	return b.String()
}

