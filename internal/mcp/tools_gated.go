package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/plan"
	"gopkg.in/yaml.v3"
)

// registerGatedTools adds the gate-based plan spec tools to the MCP server.
func (h *handlerContext) registerGatedTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("arc_plan_add_phase",
		mcp.WithDescription("Add a new phase to an existing plan with a structured spec."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan")),
		mcp.WithString("phase_name", mcp.Required(), mcp.Description("Name of the new phase")),
		mcp.WithString("spec", mcp.Required(), mcp.Description("What this phase must accomplish")),
		mcp.WithString("verify", mcp.Description("Acceptance criteria for the verifier agent (natural language, not a shell command)")),
		mcp.WithString("complexity", mcp.Description("Estimated complexity: simple, medium, or complex")),
		mcp.WithArray("files", mcp.WithStringItems(), mcp.Description("Key files relevant to this phase")),
		mcp.WithArray("deps", mcp.WithStringItems(), mcp.Description("Phase names this phase depends on")),
	), h.handlePlanAddPhase)

	s.AddTool(mcp.NewTool("arc_plan_remove_phase",
		mcp.WithDescription("Remove a phase from a plan, deleting its directory and dependency entries."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan")),
		mcp.WithString("phase_name", mcp.Required(), mcp.Description("Name of the phase to remove")),
	), h.handlePlanRemovePhase)

	s.AddTool(mcp.NewTool("arc_plan_update_phase",
		mcp.WithDescription("Update the spec fields of an existing phase. Only provided fields are changed."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan")),
		mcp.WithString("phase_name", mcp.Required(), mcp.Description("Name of the phase")),
		mcp.WithString("spec", mcp.Description("Updated description of what the phase must accomplish")),
		mcp.WithString("verify", mcp.Description("Updated acceptance criteria for the verifier agent")),
		mcp.WithString("complexity", mcp.Description("Updated complexity: simple, medium, or complex")),
	), h.handlePlanUpdatePhase)

	s.AddTool(mcp.NewTool("arc_plan_update_gate",
		mcp.WithDescription("Set or replace the gate assertions for a phase. Assertions use 'type:target' format where type is file_exists, grep, or test_exists."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan")),
		mcp.WithString("phase_name", mcp.Required(), mcp.Description("Name of the phase")),
		mcp.WithArray("assertions", mcp.Required(), mcp.WithStringItems(), mcp.Description("Gate assertions in 'type:target' format, e.g. 'file_exists:cmd/main.go' or 'grep:func TestFoo'")),
		mcp.WithBoolean("verifier_agent", mcp.Description("Whether a verifier agent should run after automated assertions")),
	), h.handlePlanUpdateGate)

	s.AddTool(mcp.NewTool("arc_plan_update_deps",
		mcp.WithDescription("Update the dependency list for a phase in both spec.yaml and plan.json."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan")),
		mcp.WithString("phase_name", mcp.Required(), mcp.Description("Name of the phase")),
		mcp.WithArray("deps", mcp.Required(), mcp.WithStringItems(), mcp.Description("Phase names this phase depends on (replaces existing deps)")),
	), h.handlePlanUpdateDeps)

	s.AddTool(mcp.NewTool("arc_plan_show_spec",
		mcp.WithDescription("Show the structured spec for a phase as YAML."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan")),
		mcp.WithString("phase_name", mcp.Required(), mcp.Description("Name of the phase")),
	), h.handlePlanShowSpec)
}

func (h *handlerContext) handlePlanAddPhase(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)
	phaseName, _ := args["phase_name"].(string)
	specText, _ := args["spec"].(string)

	if planName == "" || phaseName == "" {
		return mcp.NewToolResultError("plan_name and phase_name are required"), nil
	}
	if specText == "" {
		return mcp.NewToolResultError("spec is required"), nil
	}

	test, _ := args["verify"].(string)
	complexity, _ := args["complexity"].(string)

	var files []string
	if rawFiles, ok := args["files"].([]any); ok {
		for _, f := range rawFiles {
			if s, ok := f.(string); ok {
				files = append(files, s)
			}
		}
	}

	var deps []string
	if rawDeps, ok := args["deps"].([]any); ok {
		for _, d := range rawDeps {
			if s, ok := d.(string); ok {
				deps = append(deps, s)
			}
		}
	}

	if complexity != "" && complexity != "simple" && complexity != "medium" && complexity != "complex" {
		return mcp.NewToolResultError(fmt.Sprintf("complexity must be simple, medium, or complex; got %q", complexity)), nil
	}

	spec := &arc.PhaseSpec{
		Spec:       specText,
		Verify:     test,
		Complexity: complexity,
		Files:      files,
		Deps:       deps,
	}

	// Auto-generate gate assertions from files list.
	if len(spec.Files) > 0 && len(spec.Gate.Assertions) == 0 {
		for _, f := range spec.Files {
			spec.Gate.Assertions = append(spec.Gate.Assertions, arc.GateAssertion{
				Type:       "file_exists",
				FileExists: f,
				Target:     f,
			})
		}
	}

	if err := plan.AddPhase(h.plansDir(), planName, phaseName, spec); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	msg := fmt.Sprintf("Added phase %q to plan %q", phaseName, planName)
	if warnings := plan.ValidateSpec(spec); len(warnings) > 0 {
		msg += "\n\nWarnings:"
		for _, w := range warnings {
			msg += fmt.Sprintf("\n  - %s", w)
		}
	}
	return mcp.NewToolResultText(msg), nil
}

func (h *handlerContext) handlePlanRemovePhase(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)
	phaseName, _ := args["phase_name"].(string)

	if planName == "" || phaseName == "" {
		return mcp.NewToolResultError("plan_name and phase_name are required"), nil
	}

	if err := plan.RemovePhase(h.plansDir(), planName, phaseName); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Removed phase %q from plan %q", phaseName, planName)), nil
}

func (h *handlerContext) handlePlanUpdatePhase(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)
	phaseName, _ := args["phase_name"].(string)

	if planName == "" || phaseName == "" {
		return mcp.NewToolResultError("plan_name and phase_name are required"), nil
	}

	specText, _ := args["spec"].(string)
	test, _ := args["verify"].(string)
	complexity, _ := args["complexity"].(string)

	if specText == "" && test == "" && complexity == "" {
		return mcp.NewToolResultError("at least one of spec, verify, or complexity must be provided"), nil
	}

	if complexity != "" && complexity != "simple" && complexity != "medium" && complexity != "complex" {
		return mcp.NewToolResultError(fmt.Sprintf("complexity must be simple, medium, or complex; got %q", complexity)), nil
	}

	update := &arc.PhaseSpec{
		Spec:       specText,
		Verify:     test,
		Complexity: complexity,
	}

	if err := plan.UpdateSpec(h.plansDir(), planName, phaseName, update); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var updated []string
	if specText != "" {
		updated = append(updated, "spec")
	}
	if test != "" {
		updated = append(updated, "verify")
	}
	if complexity != "" {
		updated = append(updated, "complexity")
	}

	msg := fmt.Sprintf("Updated %s for phase %q in plan %q", strings.Join(updated, ", "), phaseName, planName)

	// Re-read merged spec and validate.
	if merged, err := plan.ReadSpec(h.plansDir(), planName, phaseName); err == nil {
		if warnings := plan.ValidateSpec(merged); len(warnings) > 0 {
			msg += "\n\nWarnings:"
			for _, w := range warnings {
				msg += fmt.Sprintf("\n  - %s", w)
			}
		}
	}

	return mcp.NewToolResultText(msg), nil
}

func (h *handlerContext) handlePlanUpdateGate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)
	phaseName, _ := args["phase_name"].(string)

	if planName == "" || phaseName == "" {
		return mcp.NewToolResultError("plan_name and phase_name are required"), nil
	}

	rawAssertions, ok := args["assertions"].([]any)
	if !ok || len(rawAssertions) == 0 {
		return mcp.NewToolResultError("assertions is required and must be non-empty"), nil
	}

	assertions := make([]arc.GateAssertion, 0, len(rawAssertions))
	for _, raw := range rawAssertions {
		s, ok := raw.(string)
		if !ok {
			return mcp.NewToolResultError("each assertion must be a string in 'type:target' format"), nil
		}
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return mcp.NewToolResultError(fmt.Sprintf("invalid assertion %q: must be 'type:target'", s)), nil
		}
		t, target := parts[0], parts[1]
		a := arc.GateAssertion{
			Type:   t,
			Target: target,
		}
		switch t {
		case "file_exists":
			a.FileExists = target
		case "grep":
			a.Grep = target
		case "test_exists":
			a.TestExists = target
		default:
			return mcp.NewToolResultError(fmt.Sprintf("unknown assertion type %q: must be file_exists, grep, or test_exists", t)), nil
		}
		assertions = append(assertions, a)
	}

	verifierAgent := false
	if v, ok := args["verifier_agent"].(bool); ok {
		verifierAgent = v
	}

	gate := arc.GateSpec{
		Assertions:    assertions,
		VerifierAgent: verifierAgent,
	}

	if err := plan.UpdateGate(h.plansDir(), planName, phaseName, gate); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Updated gate for phase %q in plan %q (%d assertions)", phaseName, planName, len(assertions))), nil
}

func (h *handlerContext) handlePlanUpdateDeps(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)
	phaseName, _ := args["phase_name"].(string)

	if planName == "" || phaseName == "" {
		return mcp.NewToolResultError("plan_name and phase_name are required"), nil
	}

	rawDeps, _ := args["deps"].([]any)
	deps := make([]string, 0, len(rawDeps))
	for _, d := range rawDeps {
		if s, ok := d.(string); ok {
			deps = append(deps, s)
		}
	}

	if err := plan.UpdateDeps(h.plansDir(), planName, phaseName, deps); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if len(deps) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("Cleared dependencies for phase %q in plan %q", phaseName, planName)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Updated deps for phase %q in plan %q: %s", phaseName, planName, strings.Join(deps, ", "))), nil
}

func (h *handlerContext) handlePlanShowSpec(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)
	phaseName, _ := args["phase_name"].(string)

	if planName == "" || phaseName == "" {
		return mcp.NewToolResultError("plan_name and phase_name are required"), nil
	}

	spec, err := plan.ReadSpec(h.plansDir(), planName, phaseName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	data, err := yaml.Marshal(spec)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling spec: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}
