package dev

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/daemon"
	"github.com/nwiley/arc/internal/review"
	"github.com/nwiley/arc/internal/state"
)

// DevOptions configures the arc dispatch pipeline.
type DevOptions struct {
	TaskDescription string
	ProjectDir      string
	ArcHome         string
	Config          *config.Config
	Logger          *slog.Logger
	Interactive     bool
	Model           string
	Timeout         int    // wall-clock timeout seconds (0 = default 14400)
	CommandName     string // agent binary name for testing
	SkipReview      bool
	AutoYes         bool   // skip clarification questions (for CI)
	SocketPath      string // daemon socket path; empty means daemon.DefaultSocketPath()
}

// DevResult holds the outcome of an arc dispatch run.
type DevResult struct {
	PlanName   string            `json:"plan_name"`
	Complexity TaskComplexity    `json:"complexity"`
	Discovery  *DiscoveryResult  `json:"discovery,omitempty"`
	Proposal   *ArchitectProposal `json:"proposal,omitempty"`
	Reviewed   bool              `json:"reviewed"`
	CodeReview *CodeReviewOutput `json:"code_review,omitempty"`
	Usage      arc.Usage         `json:"usage"`
}

// RunDev executes the full arc dispatch pipeline:
//  1. Spawn discovery agent → get DiscoveryResult
//  2. Validate and possibly override complexity
//  3. Branch by complexity:
//     a. Simple: generate plan with direct workflow, skip review, launch orchestrator
//     b. Medium: generate plan with built-in workflow, run review, launch orchestrator
//     c. Complex: spawn architects, select proposal, generate plan with custom workflow, run review, launch orchestrator
//  4. Return DevResult
func RunDev(ctx context.Context, opts DevOptions) (*DevResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if opts.TaskDescription == "" {
		return nil, fmt.Errorf("task description is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	plansDir := filepath.Join(opts.ProjectDir, ".plans", "active")

	result := &DevResult{}

	// 1. Discovery
	fmt.Println("[dispatch] Analyzing task...")
	discoveryOut, err := RunDiscovery(ctx, DiscoveryOptions{
		TaskDescription: opts.TaskDescription,
		Model:           opts.Model,
		CommandName:     opts.CommandName,
	})
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}
	discovery := &discoveryOut.Result
	result.Discovery = discovery
	result.Usage = result.Usage.Add(discoveryOut.Usage)

	// 2. Validate complexity
	complexity := ValidateComplexity(discovery)
	discovery.Complexity = complexity
	result.Complexity = complexity

	workflowType := ValidateWorkflowType(discovery.WorkflowType)

	// Clarification step: ask user questions if discovery surfaced any
	if !opts.AutoYes {
		clarifications, clarifyUsage, err := RunClarificationLoop(ctx, ClarifyOptions{
			Discovery:   discovery,
			Complexity:  complexity,
			MaxRounds:   3,
			Model:       opts.Model,
			CommandName: opts.CommandName,
			Stdin:       os.Stdin,
			Stdout:      os.Stdout,
		})
		if err != nil {
			return nil, fmt.Errorf("clarification failed: %w", err)
		}
		discovery.Clarifications = clarifications
		result.Usage = result.Usage.Add(clarifyUsage)
	}

	// Generate plan name
	planName := GeneratePlanName(opts.TaskDescription, plansDir)
	result.PlanName = planName

	switch complexity {
	case ComplexitySimple:
		fmt.Printf("[dispatch] Complexity: simple (direct execution)\n")
		fmt.Printf("[dispatch] Creating plan: %s\n", planName)

		_, err := GeneratePlan(GenerateOptions{
			PlanName:  planName,
			PlansDir:  plansDir,
			Discovery: discovery,
		})
		if err != nil {
			return nil, fmt.Errorf("plan generation failed: %w", err)
		}

		// Set review status to skip review check
		planDir := filepath.Join(plansDir, planName)
		if err := setReviewStatus(planDir, "approved"); err != nil {
			return nil, fmt.Errorf("setting review status: %w", err)
		}

	case ComplexityMedium:
		fmt.Printf("[dispatch] Complexity: medium (%s workflow)\n", workflowType)
		fmt.Printf("[dispatch] Creating plan: %s\n", planName)

		_, err := GeneratePlan(GenerateOptions{
			PlanName:  planName,
			PlansDir:  plansDir,
			Discovery: discovery,
		})
		if err != nil {
			return nil, fmt.Errorf("plan generation failed: %w", err)
		}

		planDir := filepath.Join(plansDir, planName)

		if !opts.SkipReview {
			if err := runReviewForPlan(ctx, opts, planDir, planName, plansDir, result); err != nil {
				opts.Logger.Warn("review failed, continuing", "error", err)
			}
		}

		if err := setReviewStatus(planDir, reviewStatusForResult(result)); err != nil {
			return nil, fmt.Errorf("setting review status: %w", err)
		}

	case ComplexityComplex:
		fmt.Printf("[dispatch] Complexity: complex (custom workflow)\n")

		var proposal *ArchitectProposal

		fmt.Println("[dispatch] Generating architecture proposals...")
		archOut, err := RunArchitects(ctx, ArchitectOptions{
			Discovery:   discovery,
			Model:       opts.Model,
			CommandName: opts.CommandName,
			Interactive: opts.Interactive,
		})
		if err != nil {
			opts.Logger.Warn("all architects failed, falling back to medium flow", "error", err)
			// Fall back to medium flow
		} else {
			result.Usage = result.Usage.Add(archOut.Usage)
			proposal = archOut.Selected
		}

		result.Proposal = proposal

		fmt.Printf("[dispatch] Creating plan: %s\n", planName)

		_, err = GeneratePlan(GenerateOptions{
			PlanName:  planName,
			PlansDir:  plansDir,
			Discovery: discovery,
			Proposal:  proposal,
		})
		if err != nil {
			return nil, fmt.Errorf("plan generation failed: %w", err)
		}

		planDir := filepath.Join(plansDir, planName)

		if !opts.SkipReview {
			if err := runReviewForPlan(ctx, opts, planDir, planName, plansDir, result); err != nil {
				opts.Logger.Warn("review failed, continuing", "error", err)
			}
		}

		if err := setReviewStatus(planDir, reviewStatusForResult(result)); err != nil {
			return nil, fmt.Errorf("setting review status: %w", err)
		}
	}

	// Submit plan to daemon for execution.
	fmt.Println("[dispatch] Submitting to daemon...")
	socketPath := opts.SocketPath
	if socketPath == "" {
		socketPath = daemon.DefaultSocketPath()
	}
	if err := daemon.EnsureRunning(socketPath); err != nil {
		return result, fmt.Errorf("starting daemon: %w", err)
	}
	client, err := daemon.Connect(socketPath, 5*time.Second)
	if err != nil {
		return result, fmt.Errorf("connecting to daemon: %w", err)
	}
	defer client.Close()
	resp, err := client.Submit(daemon.Request{
		Plan:    planName,
		Project: opts.ProjectDir,
	})
	if err != nil {
		return result, fmt.Errorf("submitting plan to daemon: %w", err)
	}
	if !resp.OK {
		return result, fmt.Errorf("daemon rejected plan: %s", resp.Error)
	}
	fmt.Printf("[dispatch] Plan %q submitted (%d phases queued).\n", planName, resp.QueuedPhases)

	return result, nil
}

// SubmitPlan submits an existing plan to the daemon without blocking.
func SubmitPlan(planName, projectDir, socketPath string) error {
	if socketPath == "" {
		socketPath = daemon.DefaultSocketPath()
	}
	if err := daemon.EnsureRunning(socketPath); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}
	client, err := daemon.Connect(socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to daemon: %w", err)
	}
	defer client.Close()
	resp, err := client.Submit(daemon.Request{
		Plan:    planName,
		Project: projectDir,
	})
	if err != nil {
		return fmt.Errorf("submitting plan to daemon: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("daemon rejected plan: %s", resp.Error)
	}
	fmt.Printf("[dispatch] Plan %q submitted (%d phases queued).\n", planName, resp.QueuedPhases)
	return nil
}

func runReviewForPlan(ctx context.Context, opts DevOptions, planDir, planName, plansDir string, result *DevResult) error {
	fmt.Println("[dispatch] Running adversarial review...")

	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return fmt.Errorf("reading plan for review: %w", err)
	}

	for _, phase := range meta.Phases {
		reviewResult, err := review.Run(ctx, review.ReviewOptions{
			PlanName: planName,
			PlansDir: plansDir,
			Phase:    phase,
			Model:    opts.Model,
			Logger:   opts.Logger,
		})
		if err != nil {
			return fmt.Errorf("review phase %s: %w", phase, err)
		}
		result.Usage = result.Usage.Add(reviewResult.Usage)
	}

	result.Reviewed = true
	return nil
}

func reviewStatusForResult(result *DevResult) string {
	if result.Reviewed {
		return "approved"
	}
	return "conditional" // skip-review: not reviewed, so mark conditional
}

func setReviewStatus(planDir, status string) error {
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return err
	}
	meta.ReviewStatus = status
	return state.WritePlan(planDir, meta)
}

// stop words to remove from plan names.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "to": true,
	"in": true, "for": true, "of": true, "and": true,
	"or": true, "is": true, "it": true, "on": true,
	"at": true, "by": true, "with": true,
}

var planNameValidRe = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

// GeneratePlanName creates a valid plan name from a task description.
func GeneratePlanName(description string, plansDir string) string {
	// Lowercase
	s := strings.ToLower(description)

	// Remove non-alphanumeric (keep spaces and hyphens)
	var cleaned []rune
	for _, r := range s {
		if unicode.IsLetter(r) && r < 128 || unicode.IsDigit(r) || r == ' ' || r == '-' {
			cleaned = append(cleaned, r)
		}
	}
	s = string(cleaned)

	// Split into words
	words := strings.Fields(s)

	// Remove stop words
	var significant []string
	for _, w := range words {
		if !stopWords[w] {
			significant = append(significant, w)
		}
	}

	// Take first 4 words
	if len(significant) > 4 {
		significant = significant[:4]
	}

	// Join with hyphens
	name := strings.Join(significant, "-")

	// Truncate to 40 chars (trim trailing hyphens)
	if len(name) > 40 {
		name = name[:40]
	}
	name = strings.TrimRight(name, "-")

	// Validate
	if name == "" || !planNameValidRe.MatchString(name) {
		// Fallback: "dev-" + first 6 chars of sha256
		h := sha256.Sum256([]byte(description))
		name = fmt.Sprintf("dev-%x", h[:3])
	}

	// Check for conflicts
	base := name
	suffix := 2
	for i := 0; ; i++ {
		if i > 100 {
			// Safety cap: give up after 100 attempts
			h := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", description, suffix)))
			name = fmt.Sprintf("%s-%x", base, h[:3])
			break
		}
		path := filepath.Join(plansDir, name)
		_, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			// Non-ENOENT error (e.g. permission denied): stop looping
			name = fmt.Sprintf("%s-%d", base, suffix)
			break
		}
		name = fmt.Sprintf("%s-%d", base, suffix)
		suffix++
	}

	return name
}

// getHeadCommit returns the current HEAD commit hash, or empty string on error.
func getHeadCommit(projectDir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runPostReview computes the diff and runs the code review agent.
func runPostReview(ctx context.Context, opts DevOptions, result *DevResult, beforeCommit, planName, plansDir string) (*CodeReviewOutput, error) {
	// Compute diff from before orchestration to HEAD.
	diff, err := gitDiff(opts.ProjectDir, beforeCommit)
	if err != nil {
		return nil, fmt.Errorf("computing diff: %w", err)
	}
	if strings.TrimSpace(diff) == "" {
		return nil, nil // no changes to review
	}

	// Collect plan.md content from all phases.
	planMD, err := collectPlanMD(filepath.Join(plansDir, planName))
	if err != nil {
		return nil, fmt.Errorf("collecting plan content: %w", err)
	}

	fmt.Println("[dispatch] Running code review...")
	reviewOut, err := RunCodeReview(ctx, ReviewOptions{
		PlanDir:     filepath.Join(plansDir, planName),
		ProjectDir:  opts.ProjectDir,
		Diff:        diff,
		PlanMD:      planMD,
		Discovery:   result.Discovery,
		Model:       opts.Model,
		CommandName: opts.CommandName,
	})
	if err != nil {
		return nil, err
	}

	// Print review summary.
	printReviewSummary(reviewOut)
	return reviewOut, nil
}

// gitDiff returns the diff between a commit and HEAD.
func gitDiff(projectDir, fromCommit string) (string, error) {
	cmd := exec.Command("git", "diff", fromCommit+"..HEAD")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// collectPlanMD reads all plan.md files from a plan directory's phases.
func collectPlanMD(planDir string) (string, error) {
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return "", err
	}

	var parts []string
	for _, phase := range meta.Phases {
		planFile := filepath.Join(planDir, phase, "plan.md")
		data, err := os.ReadFile(planFile)
		if err != nil {
			continue // skip phases without plan.md
		}
		parts = append(parts, string(data))
	}

	return strings.Join(parts, "\n\n---\n\n"), nil
}

// printReviewSummary prints the code review results to stdout.
func printReviewSummary(review *CodeReviewOutput) {
	if review == nil {
		return
	}

	var critical, warnings, suggestions int
	for _, issue := range review.Issues {
		switch issue.Severity {
		case "critical":
			critical++
		case "warning":
			warnings++
		case "suggestion":
			suggestions++
		}
	}

	if len(review.Issues) == 0 {
		fmt.Println("[dispatch] Code review: no issues found")
	} else {
		fmt.Printf("[dispatch] Code review: %d critical, %d warnings, %d suggestions\n", critical, warnings, suggestions)
	}
	if review.Summary != "" {
		fmt.Printf("[dispatch] Review summary: %s\n", review.Summary)
	}
}

// saveCodeReview writes the code review output to a JSON file in the plan directory.
func saveCodeReview(planDir string, review *CodeReviewOutput) error {
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(planDir, "code_review.json"), data, 0644)
}
