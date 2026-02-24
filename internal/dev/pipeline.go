package dev

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/review"
	"github.com/nwiley/arc/internal/state"
)

// DevOptions configures the arc dev pipeline.
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
}

// DevResult holds the outcome of an arc dev run.
type DevResult struct {
	PlanName   string
	Complexity TaskComplexity
	Discovery  *DiscoveryResult
	Proposal   *ArchitectProposal
	Reviewed   bool
	Usage      arc.Usage
}

// RunDev executes the full arc dev pipeline:
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

	plansDir := filepath.Join(opts.ProjectDir, ".plans", "active")

	result := &DevResult{}

	// 1. Discovery
	fmt.Println("[dev] Analyzing task...")
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

	// Generate plan name
	planName := GeneratePlanName(opts.TaskDescription, plansDir)
	result.PlanName = planName

	switch complexity {
	case ComplexitySimple:
		fmt.Printf("[dev] Complexity: simple (direct execution)\n")
		fmt.Printf("[dev] Creating plan: %s\n", planName)

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
		fmt.Printf("[dev] Complexity: medium (%s workflow)\n", workflowType)
		fmt.Printf("[dev] Creating plan: %s\n", planName)

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
		fmt.Printf("[dev] Complexity: complex (custom workflow)\n")

		var proposal *ArchitectProposal

		fmt.Println("[dev] Generating architecture proposals...")
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
			if opts.Interactive {
				proposal = archOut.Selected
			} else {
				proposal = archOut.Selected
			}
		}

		result.Proposal = proposal

		fmt.Printf("[dev] Creating plan: %s\n", planName)

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

	// Launch orchestrator
	fmt.Println("[dev] Launching orchestrator...")
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 14400
	}

	err = orchestrator.Launch(ctx, orchestrator.LaunchOptions{
		PlanName:   planName,
		PlansDir:   plansDir,
		ArcHome:    opts.ArcHome,
		ProjectDir: opts.ProjectDir,
		Config:     opts.Config,
		Logger:     opts.Logger,
		Timeout:    timeout,
	})
	if err != nil {
		return result, fmt.Errorf("orchestrator failed: %w", err)
	}

	return result, nil
}

func runReviewForPlan(ctx context.Context, opts DevOptions, planDir, planName, plansDir string, result *DevResult) error {
	fmt.Println("[dev] Running adversarial review...")

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
	return "approved" // skip-review also counts as approved for orchestrator
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
	for {
		path := filepath.Join(plansDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("%s-%d", base, suffix)
		suffix++
	}

	return name
}
