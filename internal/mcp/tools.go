package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/guide"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/pipeline"
	"github.com/nwiley/arc/internal/plan"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/review"
	"github.com/nwiley/arc/internal/state"
	"github.com/nwiley/arc/internal/workflow"
)

// runJob tracks a running orchestrator invocation.
type runJob struct {
	PlanName  string
	Cancel    context.CancelFunc
	Done      chan struct{}
	Result    *orchestrator.LaunchResult
	Err       error
	StartedAt time.Time
}

// handlerContext holds shared state for all tool handlers.
type handlerContext struct {
	projectDir string
	arcHome    string
	logger     *slog.Logger
	mu         sync.Mutex
	jobs       map[string]*runJob // keyed by plan name
	jobsCtx    context.Context    // parent context for all background jobs; cancelled on server shutdown
}

func (h *handlerContext) plansDir() string {
	return filepath.Join(h.projectDir, ".plans", "active")
}

func (h *handlerContext) archiveDir() string {
	return filepath.Join(h.projectDir, ".plans", "archive")
}

var validNameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]*[a-zA-Z0-9]$`)

func validateName(name, label string) error {
	if name == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(name) > 64 {
		return fmt.Errorf("%s too long (max 64 chars)", label)
	}
	if !validNameRe.MatchString(name) {
		return fmt.Errorf("invalid %s %q: must be alphanumeric with hyphens", label, name)
	}
	return nil
}

// registerTools adds all Arc tools to the MCP server.
func (h *handlerContext) registerTools(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("arc_status",
		mcp.WithDescription("Show plan and phase status. Returns a summary of all plans or a specific plan's phases with their current state."),
		mcp.WithString("plan_name", mcp.Description("Name of a specific plan to show status for. Omit to list all plans.")),
	), h.handleStatus)

	s.AddTool(mcp.NewTool("arc_plan",
		mcp.WithDescription("Create a new plan with phase scaffolding. Creates the plan directory, plan.json, and phase directories with plan.md files."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name for the plan (used as directory name)")),
		mcp.WithString("workflow_type", mcp.Description("Workflow type: feature, bugfix, investigation, refactor, performance, audit, direct. Defaults to 'custom' when workflow is provided.")),
		mcp.WithArray("phases", mcp.Required(), mcp.WithStringItems(), mcp.Description("Ordered list of phase names")),
		mcp.WithString("workflow", mcp.Description("Custom workflow YAML (pipeline format). When provided, workflow_type defaults to 'custom'.")),
		mcp.WithString("save_workflow_as", mcp.Description("Optional name to save the workflow for reuse in future plans.")),
	), h.handlePlan)

	s.AddTool(mcp.NewTool("arc_run",
		mcp.WithDescription("Launch the orchestrator to run all phases of a plan. The plan must be reviewed (approved or conditional) before running. This is a long-running operation."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan to run")),
		mcp.WithNumber("timeout", mcp.Description("Wall-clock timeout in seconds (default: 14400)")),
		mcp.WithBoolean("worktree", mcp.Description("Run agents in isolated git worktrees (default: true)")),
		mcp.WithBoolean("per_phase_worktree", mcp.Description("Create a separate worktree per phase instead of one shared worktree (default: false)")),
	), h.handleRun)

	s.AddTool(mcp.NewTool("arc_iterate",
		mcp.WithDescription("Run a single iteration for a specific phase. Returns the next state and verdict."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan")),
		mcp.WithString("phase_name", mcp.Required(), mcp.Description("Name of the phase to iterate")),
	), h.handleIterate)

	s.AddTool(mcp.NewTool("arc_review",
		mcp.WithDescription("Run adversarial review on a plan. Reviews all phases concurrently (max 3 at a time) with a single auto-remediation pass. Updates plan.json with review status."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan to review")),
		mcp.WithString("phase", mcp.Description("Review a single phase instead of all phases")),
		mcp.WithString("model", mcp.Description("Model override for review agents")),
	), h.handleReview)

	s.AddTool(mcp.NewTool("arc_manage",
		mcp.WithDescription("Manage phase state. Supports actions: complete, pending, defer, block, tests, packages, note, iteration, copy-from, show, activity, reset."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan")),
		mcp.WithString("phase", mcp.Description("Name of the phase (optional for reset — omit to reset all phases in the plan)")),
		mcp.WithString("action", mcp.Required(), mcp.Description("Action to perform: complete, pending, defer, block, tests, packages, note, iteration, copy-from, show, activity, reset")),
		mcp.WithString("reason", mcp.Description("Reason (required for defer and block)")),
		mcp.WithNumber("passing", mcp.Description("Passing test count (for tests action)")),
		mcp.WithNumber("total", mcp.Description("Total test count (for tests action)")),
		mcp.WithString("packages", mcp.Description("Comma-separated package list (for packages action)")),
		mcp.WithString("note", mcp.Description("Note text (for note action)")),
		mcp.WithNumber("iteration", mcp.Description("Iteration number (for iteration action)")),
		mcp.WithString("source_phase", mcp.Description("Source phase name (for copy-from action)")),
		mcp.WithString("activity", mcp.Description("Activity message (for activity action)")),
	), h.handleManage)

	s.AddTool(mcp.NewTool("arc_guide",
		mcp.WithDescription("Print the Arc reference guide for AI agents. Covers setup, plans, workflows, execution, and common mistakes."),
		mcp.WithString("section", mcp.Description("Specific section: setup, plans, workflows, execution, mistakes. Omit for full guide.")),
	), h.handleGuide)

	s.AddTool(mcp.NewTool("arc_list_plans",
		mcp.WithDescription("List all active plans with their status and workflow type."),
	), h.handleListPlans)

	s.AddTool(mcp.NewTool("arc_archive",
		mcp.WithDescription("Archive a completed plan by moving it from active to archive directory."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan to archive")),
		mcp.WithBoolean("force", mcp.Description("Archive even if phases are not all terminal (default: false)")),
	), h.handleArchive)

	s.AddTool(mcp.NewTool("arc_run_status",
		mcp.WithDescription("Check the status of a running or recently completed arc_run. Returns phase progress, elapsed time, and result details. If no plan is running, falls through to arc_status."),
		mcp.WithString("plan_name", mcp.Description("Name of the plan to check. Omit to list all active jobs.")),
	), h.handleRunStatus)

	s.AddTool(mcp.NewTool("arc_run_cancel",
		mcp.WithDescription("Cancel a running arc_run for a plan."),
		mcp.WithString("plan_name", mcp.Required(), mcp.Description("Name of the plan to cancel")),
	), h.handleRunCancel)

}

// drainJobs cancels all running jobs and waits for them to finish cleanup.
// This ensures child processes (claude CLI) are killed before the MCP server exits.
func (h *handlerContext) drainJobs(logger *slog.Logger) {
	h.mu.Lock()
	var active []*runJob
	for name, job := range h.jobs {
		select {
		case <-job.Done:
			// Already finished.
		default:
			logger.Info("cancelling running job on shutdown", "plan", name)
			job.Cancel()
			active = append(active, job)
		}
	}
	h.mu.Unlock()

	if len(active) == 0 {
		return
	}

	// Wait up to 10 seconds for jobs to finish (agent SIGKILL + cleanup).
	deadline := time.After(10 * time.Second)
	for _, job := range active {
		select {
		case <-job.Done:
		case <-deadline:
			logger.Warn("timed out waiting for jobs to drain, exiting anyway")
			return
		}
	}
}

func (h *handlerContext) handleStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)

	var buf bytes.Buffer
	err := plan.Status(&buf, plan.StatusOptions{
		PlansDir: h.plansDir(),
		PlanName: planName,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(buf.String()), nil
}

func (h *handlerContext) handlePlan(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	name, _ := args["name"].(string)
	workflowType, _ := args["workflow_type"].(string)
	inlineWorkflow, _ := args["workflow"].(string)
	saveWorkflowAs, _ := args["save_workflow_as"].(string)

	var phases []string
	if rawPhases, ok := args["phases"].([]any); ok {
		for _, p := range rawPhases {
			if s, ok := p.(string); ok {
				phases = append(phases, s)
			}
		}
	}

	if err := validateName(name, "name"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if len(phases) == 0 {
		return mcp.NewToolResultError("at least one phase is required"), nil
	}

	homeDir, _ := os.UserHomeDir()
	resolver := resources.NewResolver(h.projectDir, homeDir)

	var customWorkflow []byte
	if inlineWorkflow != "" {
		customWorkflow = []byte(inlineWorkflow)

		// Validate the YAML parses correctly before creating the plan
		if _, err := workflow.LoadBytesWithBlockLoader(customWorkflow, resolver.BlockBytes); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid workflow YAML: %v", err)), nil
		}

		// Default workflow_type to "custom" when inline workflow is provided
		if workflowType == "" {
			workflowType = "custom"
		}

		// Save workflow for reuse if requested
		if saveWorkflowAs != "" {
			wfDir := filepath.Join(h.projectDir, ".arc", "workflows")
			if err := os.MkdirAll(wfDir, 0755); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("creating workflows dir: %v", err)), nil
			}
			if err := os.WriteFile(filepath.Join(wfDir, saveWorkflowAs+".yaml"), customWorkflow, 0644); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("saving workflow: %v", err)), nil
			}
		}
	} else if workflowType == "" {
		return mcp.NewToolResultError("workflow_type is required when workflow is not provided"), nil
	}

	meta, err := plan.Create(plan.CreateOptions{
		PlansDir:       h.plansDir(),
		Name:           name,
		Phases:         phases,
		WorkflowType:   workflowType,
		CustomWorkflow: customWorkflow,
		Resolver:       resolver,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Created plan %q with phases: %s (workflow: %s)", meta.Name, strings.Join(meta.Phases, ", "), meta.WorkflowType)), nil
}

func (h *handlerContext) handleRun(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)

	if err := validateName(planName, "plan_name"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Verify plan exists and is reviewed
	planDir := filepath.Join(h.plansDir(), planName)
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading plan: %v", err)), nil
	}
	if meta.ReviewStatus != "approved" && meta.ReviewStatus != "conditional" {
		return mcp.NewToolResultError(fmt.Sprintf("plan %q has review status %q — run arc_review first", planName, meta.ReviewStatus)), nil
	}

	cfg, err := config.Load(h.projectDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("loading .arc.yaml: %v", err)), nil
	}

	timeout := 14400
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = int(t)
	}
	useWorktree := true
	if w, ok := args["worktree"].(bool); ok {
		useWorktree = w
	}
	perPhaseWorktree := false
	if p, ok := args["per_phase_worktree"].(bool); ok {
		perPhaseWorktree = p
	}

	// Check if plan is already running.
	h.mu.Lock()
	if job, ok := h.jobs[planName]; ok {
		select {
		case <-job.Done:
			// Finished — clean up stale entry.
			delete(h.jobs, planName)
		default:
			h.mu.Unlock()
			return mcp.NewToolResultError(fmt.Sprintf("plan %q is already running (started %s). Use arc_run_status to check progress or arc_run_cancel to stop it.", planName, job.StartedAt.Format(time.RFC3339))), nil
		}
	}

	jobCtx, jobCancel := context.WithCancel(h.jobsCtx)
	job := &runJob{
		PlanName:  planName,
		Cancel:    jobCancel,
		Done:      make(chan struct{}),
		StartedAt: time.Now(),
	}
	h.jobs[planName] = job
	h.mu.Unlock()

	runHomeDir, _ := os.UserHomeDir()
	runResolver := resources.NewResolver(h.projectDir, runHomeDir)

	go func() {
		defer close(job.Done)
		job.Result, job.Err = orchestrator.Launch(jobCtx, orchestrator.LaunchOptions{
			PlanName:         planName,
			PlansDir:         h.plansDir(),
			ArcHome:          h.arcHome,
			ProjectDir:       h.projectDir,
			Config:           cfg,
			Logger:           h.logger,
			Timeout:          timeout,
			UseWorktree:      useWorktree,
			PerPhaseWorktree: perPhaseWorktree,
			StopOnFailure:    true,
			ChatMode:         true,
			Resolver:         runResolver,
		})
	}()

	return mcp.NewToolResultText(fmt.Sprintf("Started run for plan %q. Use arc_run_status to monitor.", planName)), nil
}

func (h *handlerContext) handleIterate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)
	phaseName, _ := args["phase_name"].(string)

	if err := validateName(planName, "plan_name"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := validateName(phaseName, "phase_name"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Verify phase exists
	phaseDir := filepath.Join(h.plansDir(), planName, "phases", phaseName)
	if _, err := os.Stat(phaseDir); os.IsNotExist(err) {
		return mcp.NewToolResultError(fmt.Sprintf("phase %q not found in plan %q", phaseName, planName)), nil
	}

	iterHomeDir, _ := os.UserHomeDir()
	iterResolver := resources.NewResolver(h.projectDir, iterHomeDir)

	result := pipeline.RunState(ctx, h.logger, pipeline.IterateOptions{
		PlanName:  planName,
		PhaseName: phaseName,
		PlansDir:  h.plansDir(),
		ArcHome:   h.arcHome,
		ChatMode:  true,
		Resolver:  iterResolver,
	})

	if result.Err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("iteration failed (%s): %v", result.Action, result.Err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Iteration complete: next_state=%s verdict=%s", result.NextState, result.Verdict)), nil
}

func (h *handlerContext) handleReview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)
	phaseFilter, _ := args["phase"].(string)
	model, _ := args["model"].(string)

	if err := validateName(planName, "plan_name"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if phaseFilter != "" {
		if err := validateName(phaseFilter, "phase"); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	if model == "" {
		model = "claude-sonnet-4-5-20250929"
	}

	// Read plan.json to discover phases
	planDir := filepath.Join(h.plansDir(), planName)
	metaBytes, err := os.ReadFile(filepath.Join(planDir, "plan.json"))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading plan.json: %v", err)), nil
	}
	var meta arc.PlanMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("parsing plan.json: %v", err)), nil
	}

	phases := meta.Phases
	if phaseFilter != "" {
		found := false
		for _, p := range meta.Phases {
			if p == phaseFilter {
				found = true
				break
			}
		}
		if !found {
			return mcp.NewToolResultError(fmt.Sprintf("phase %q not found in plan", phaseFilter)), nil
		}
		phases = []string{phaseFilter}
	}

	// Run phases concurrently (max 3)
	const maxConcurrent = 3

	type phaseResult struct {
		Phase  string
		Result *review.ReviewResult
		Err    error
	}

	resultsCh := make(chan phaseResult, len(phases))
	sem := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup
	for _, phase := range phases {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := review.Run(ctx, review.ReviewOptions{
				PlanName:      planName,
				PlansDir:      h.plansDir(),
				ArcHome:       h.arcHome,
				Phase:         p,
				Model:         model,
				Logger:        h.logger,
				MaxIterations: 1,
			})
			resultsCh <- phaseResult{Phase: p, Result: result, Err: err}
		}(phase)
	}

	wg.Wait()
	close(resultsCh)

	// Collect and format results
	phaseResults := make(map[string]phaseResult, len(phases))
	for r := range resultsCh {
		phaseResults[r.Phase] = r
	}

	var out bytes.Buffer
	overallStatus := "approved"
	reviewResults := make(map[string]string)
	maxIteration := 0

	for _, phase := range phases {
		pr := phaseResults[phase]
		if pr.Err != nil {
			fmt.Fprintf(&out, "Phase %s: ERROR: %v\n", phase, pr.Err)
			overallStatus = "needs_review"
			continue
		}

		fmt.Fprintf(&out, "Phase %s: %s\n", phase, pr.Result.Status)
		for _, v := range pr.Result.Verdicts {
			effectiveStatus := v.Status
			if v.Status == "cached" {
				effectiveStatus = v.CachedStatus
			}
			reviewResults[v.Name] = effectiveStatus
		}

		if pr.Result.Iteration > maxIteration {
			maxIteration = pr.Result.Iteration
		}
		if pr.Result.Status == "needs_review" {
			overallStatus = "needs_review"
		} else if pr.Result.Status == "conditional" && overallStatus == "approved" {
			overallStatus = "conditional"
		}
	}

	fmt.Fprintf(&out, "Review complete: status=%s\n", overallStatus)

	// Update plan.json
	metaBytes, err = os.ReadFile(filepath.Join(planDir, "plan.json"))
	if err == nil {
		var updatedMeta arc.PlanMeta
		if err := json.Unmarshal(metaBytes, &updatedMeta); err == nil {
			updatedMeta.ReviewStatus = overallStatus
			updatedMeta.ReviewedAt = time.Now().UTC().Format(time.RFC3339)
			updatedMeta.ReviewIterations = maxIteration
			updatedMeta.ReviewResults = reviewResults
			if data, err := json.MarshalIndent(updatedMeta, "", "  "); err == nil {
				os.WriteFile(filepath.Join(planDir, "plan.json"), data, 0644)
			}
		}
	}

	return mcp.NewToolResultText(out.String()), nil
}

func (h *handlerContext) handleManage(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)
	phase, _ := args["phase"].(string)
	action, _ := args["action"].(string)

	if err := validateName(planName, "plan_name"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if action == "" {
		return mcp.NewToolResultError("action is required"), nil
	}
	if phase == "" && action != "reset" {
		return mcp.NewToolResultError("phase is required for this action"), nil
	}
	if phase != "" {
		if err := validateName(phase, "phase"); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	opts := plan.ManageOptions{
		PlansDir: h.plansDir(),
		PlanName: planName,
		Phase:    phase,
	}

	switch action {
	case "complete":
		if err := plan.ManageComplete(opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Marked %s/%s as complete", planName, phase)), nil

	case "pending":
		if err := plan.ManagePending(opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Reset %s/%s to pending", planName, phase)), nil

	case "defer":
		reason, _ := args["reason"].(string)
		if reason == "" {
			return mcp.NewToolResultError("reason is required for defer action"), nil
		}
		opts.Reason = reason
		if err := plan.ManageDefer(opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Deferred %s/%s: %s", planName, phase, reason)), nil

	case "block":
		reason, _ := args["reason"].(string)
		if reason == "" {
			return mcp.NewToolResultError("reason is required for block action"), nil
		}
		opts.Reason = reason
		if err := plan.ManageBlock(opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Blocked %s/%s: %s", planName, phase, reason)), nil

	case "tests":
		passing, passingOk := args["passing"].(float64)
		total, totalOk := args["total"].(float64)
		if !passingOk || !totalOk {
			return mcp.NewToolResultError("passing and total must be numeric"), nil
		}
		if passing < 0 || total < 0 {
			return mcp.NewToolResultError("passing and total must be non-negative"), nil
		}
		opts.Passing = int(passing)
		opts.Total = int(total)
		if err := plan.ManageTests(opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Updated tests for %s/%s: %d/%d", planName, phase, opts.Passing, opts.Total)), nil

	case "packages":
		pkgs, _ := args["packages"].(string)
		if pkgs == "" {
			return mcp.NewToolResultError("packages is required for packages action"), nil
		}
		opts.Packages = strings.Split(pkgs, ",")
		if err := plan.ManagePackages(opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Updated packages for %s/%s", planName, phase)), nil

	case "note":
		note, _ := args["note"].(string)
		if note == "" {
			return mcp.NewToolResultError("note is required for note action"), nil
		}
		opts.Note = note
		if err := plan.ManageNote(opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Updated note for %s/%s", planName, phase)), nil

	case "iteration":
		n, nOk := args["iteration"].(float64)
		if !nOk {
			return mcp.NewToolResultError("iteration must be numeric"), nil
		}
		if n < 0 {
			return mcp.NewToolResultError("iteration must be non-negative"), nil
		}
		opts.Iteration = int(n)
		if err := plan.ManageIteration(opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Set iteration for %s/%s to %d", planName, phase, opts.Iteration)), nil

	case "copy-from":
		src, _ := args["source_phase"].(string)
		if src == "" {
			return mcp.NewToolResultError("source_phase is required for copy-from action"), nil
		}
		opts.SourcePhase = src
		if err := plan.ManageCopyFrom(opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Copied state from %s to %s/%s", src, planName, phase)), nil

	case "show":
		var buf bytes.Buffer
		if err := plan.ManageShow(&buf, opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(buf.String()), nil

	case "activity":
		activity, _ := args["activity"].(string)
		opts.Activity = activity
		if err := plan.ManageActivity(opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if activity == "" {
			return mcp.NewToolResultText(fmt.Sprintf("Cleared activity for %s/%s", planName, phase)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Set activity for %s/%s: %s", planName, phase, activity)), nil

	case "reset":
		if phase == "" {
			if err := plan.ManageResetPlan(opts); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Reset all phases in %s", planName)), nil
		}
		if err := plan.ManageReset(opts); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Reset %s/%s", planName, phase)), nil

	default:
		return mcp.NewToolResultError(fmt.Sprintf("unknown action %q — valid actions: complete, pending, defer, block, tests, packages, note, iteration, copy-from, show, activity, reset", action)), nil
	}
}

func (h *handlerContext) handleGuide(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	section, _ := args["section"].(string)

	data, err := guide.Render(section)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}

func (h *handlerContext) handleListPlans(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entries, err := os.ReadDir(h.plansDir())
	if err != nil {
		if os.IsNotExist(err) {
			return mcp.NewToolResultText("No active plans found."), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}

	var out bytes.Buffer
	found := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		planDir := filepath.Join(h.plansDir(), e.Name())
		metaBytes, err := os.ReadFile(filepath.Join(planDir, "plan.json"))
		if err != nil {
			continue
		}
		var meta arc.PlanMeta
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			continue
		}
		found = true
		fmt.Fprintf(&out, "%s  workflow=%s  review=%s  phases=%s\n",
			meta.Name, meta.WorkflowType, meta.ReviewStatus,
			strings.Join(meta.Phases, ","))
	}

	if !found {
		return mcp.NewToolResultText("No active plans found."), nil
	}
	return mcp.NewToolResultText(out.String()), nil
}

func (h *handlerContext) handleArchive(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)

	if err := validateName(planName, "plan_name"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	force := false
	if f, ok := args["force"].(bool); ok {
		force = f
	}

	err := plan.Archive(plan.ArchiveOptions{
		PlansDir:   h.plansDir(),
		ArchiveDir: h.archiveDir(),
		PlanName:   planName,
		ProjectDir: h.projectDir,
		Force:      force,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Archived plan %q", planName)), nil
}

func (h *handlerContext) handleRunStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)

	if planName != "" {
		if err := validateName(planName, "plan_name"); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	// Copy needed data under lock, then release before doing any I/O.
	h.mu.Lock()

	if planName == "" {
		// List all active jobs — no I/O needed, build output under lock.
		if len(h.jobs) == 0 {
			h.mu.Unlock()
			return mcp.NewToolResultText("No active runs."), nil
		}
		var out bytes.Buffer
		for name, job := range h.jobs {
			select {
			case <-job.Done:
				status := "complete"
				if job.Result != nil {
					status = job.Result.Status
				}
				if job.Err != nil {
					status = "error: " + job.Err.Error()
				}
				fmt.Fprintf(&out, "%s: finished (%s) after %s\n", name, status, time.Since(job.StartedAt).Truncate(time.Second))
			default:
				fmt.Fprintf(&out, "%s: running since %s (%s elapsed)\n", name, job.StartedAt.Format(time.RFC3339), time.Since(job.StartedAt).Truncate(time.Second))
			}
		}
		h.mu.Unlock()
		return mcp.NewToolResultText(out.String()), nil
	}

	job, ok := h.jobs[planName]
	if !ok {
		h.mu.Unlock()
		// No job found — fall through to regular status.
		var buf bytes.Buffer
		err := plan.Status(&buf, plan.StatusOptions{
			PlansDir: h.plansDir(),
			PlanName: planName,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(buf.String()), nil
	}

	select {
	case <-job.Done:
		// Job completed — copy result data, clean up, then unlock.
		var out bytes.Buffer
		fmt.Fprintf(&out, "Run for %q finished after %s.\n", planName, time.Since(job.StartedAt).Truncate(time.Second))
		if job.Err != nil {
			fmt.Fprintf(&out, "Error: %v\n", job.Err)
		}
		if job.Result != nil {
			fmt.Fprintf(&out, "Status: %s\n", job.Result.Status)
			if job.Result.FailedPhase != "" {
				fmt.Fprintf(&out, "Failed phase: %s\n", job.Result.FailedPhase)
				fmt.Fprintf(&out, "Reason: %s\n", job.Result.FailedReason)
			}
			for phase, status := range job.Result.PhaseSummary {
				fmt.Fprintf(&out, "  %s: %s\n", phase, status)
			}
			if job.Result.Usage.CostUSD > 0 {
				fmt.Fprintf(&out, "Cost: $%.4f\n", job.Result.Usage.CostUSD)
			}
		}
		delete(h.jobs, planName)
		h.mu.Unlock()
		return mcp.NewToolResultText(out.String()), nil
	default:
		// Still running — copy minimal info, unlock, then do I/O.
		startedAt := job.StartedAt
		h.mu.Unlock()
		var out bytes.Buffer
		fmt.Fprintf(&out, "Run for %q is in progress (started %s, %s elapsed).\n", planName, startedAt.Format(time.RFC3339), time.Since(startedAt).Truncate(time.Second))
		// Include current phase states.
		var statusBuf bytes.Buffer
		plan.Status(&statusBuf, plan.StatusOptions{
			PlansDir: h.plansDir(),
			PlanName: planName,
		})
		if statusBuf.Len() > 0 {
			out.Write(statusBuf.Bytes())
		}
		return mcp.NewToolResultText(out.String()), nil
	}
}

func (h *handlerContext) handleRunCancel(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	planName, _ := args["plan_name"].(string)

	if planName == "" {
		return mcp.NewToolResultError("plan_name is required"), nil
	}

	h.mu.Lock()
	job, ok := h.jobs[planName]
	h.mu.Unlock()

	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("no running job for plan %q", planName)), nil
	}

	select {
	case <-job.Done:
		return mcp.NewToolResultText(fmt.Sprintf("Run for %q already finished.", planName)), nil
	default:
		job.Cancel()
		// Wait briefly for completion.
		select {
		case <-job.Done:
			return mcp.NewToolResultText(fmt.Sprintf("Cancelled run for %q.", planName)), nil
		case <-time.After(100 * time.Millisecond):
			return mcp.NewToolResultText(fmt.Sprintf("Cancel signal sent for %q. Use arc_run_status to check.", planName)), nil
		}
	}
}

