package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/nwiley/arc/internal/adapter"
	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/prompt"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/review"
	"github.com/nwiley/arc/internal/state"
	"github.com/spf13/cobra"
)

func newTaskCmd() *cobra.Command {
	var run bool
	var planName string
	var timeout int
	var model string
	var skipReview bool

	cmd := &cobra.Command{
		Use:   "task [description...]",
		Short: "Plan and run a task from a natural language description",
		Long:  "Spawns a planning agent to create a structured plan from a description, then optionally reviews and runs it.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			description := strings.Join(args, " ")

			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			// Load config (non-fatal if missing).
			cfg, _ := config.Load(projectRoot)
			if cfg == nil {
				cfg = &config.Config{}
			}

			plansDir := filepath.Join(projectRoot, ".plans", "active")
			if err := os.MkdirAll(plansDir, 0755); err != nil {
				return fmt.Errorf("creating plans directory: %w", err)
			}

			// Generate plan name if not explicitly provided.
			if planName == "" {
				planName = taskPlanName(description, plansDir)
			}

			fmt.Printf("[task] Plan name: %s\n", planName)
			fmt.Printf("[task] Spawning planning agent...\n")

			// Render planner prompt.
			projectContext := prompt.LoadProjectContext(projectRoot)
			plannerPrompt, err := prompt.RenderGatePrompt("planner", prompt.PlannerData{
				Description:    description,
				PlanName:       planName,
				ProjectContext: projectContext,
			})
			if err != nil {
				return fmt.Errorf("rendering planner prompt: %w", err)
			}

			// Set up context with signal handling.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				fmt.Fprintf(os.Stderr, "\nReceived interrupt, shutting down...\n")
				cancel()
			}()

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			// Resolve and spawn the planning agent.
			adapterName := cfg.AgentForRole("planner")
			ag := adapter.Get(adapterName)

			sessionCfg := arc.SessionConfig{
				Timeout: time.Duration(timeout) * time.Second,
			}
			if model != "" {
				sessionCfg.Model = model
			}

			result, err := ag.Spawn(ctx, plannerPrompt, projectRoot, sessionCfg)
			if err != nil {
				return fmt.Errorf("planning agent failed: %w", err)
			}

			if result.ExitCode != 0 {
				return fmt.Errorf("planning agent exited with code %d", result.ExitCode)
			}

			// Verify the plan was actually created.
			planDir := filepath.Join(plansDir, planName)
			if _, err := os.Stat(planDir); os.IsNotExist(err) {
				return fmt.Errorf("planning agent did not create plan %q at %s", planName, planDir)
			}

			fmt.Printf("[task] Plan created: %s\n", planName)
			if result.Usage.CostUSD > 0 {
				fmt.Printf("[task] Planning cost: $%.4f\n", result.Usage.CostUSD)
			}

			if !run {
				fmt.Printf("[task] Skipping review and run (--run=false)\n")
				return nil
			}

			arcHome := resolveArcHome()
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("getting home directory: %w", err)
			}
			resolver := resources.NewResolver(projectRoot, homeDir)

			// Run adversarial review.
			if !skipReview {
				fmt.Printf("[task] Running adversarial review...\n")

				meta, err := state.ReadPlan(planDir)
				if err != nil {
					return fmt.Errorf("reading plan for review: %w", err)
				}

				overallStatus := "approved"
				for _, phase := range meta.Phases {
					reviewResult, err := review.Run(ctx, review.ReviewOptions{
						PlanName: planName,
						PlansDir: plansDir,
						ArcHome:  arcHome,
						Phase:    phase,
						Model:    model,
						Logger:   logger,
					})
					if err != nil {
						logger.Warn("review failed, continuing", "phase", phase, "error", err)
						overallStatus = "conditional"
						continue
					}
					if reviewResult.Status == "needs_review" {
						overallStatus = "needs_review"
					} else if reviewResult.Status == "conditional" && overallStatus == "approved" {
						overallStatus = "conditional"
					}
				}

				// Write review status into plan.json.
				if err := setTaskReviewStatus(planDir, overallStatus); err != nil {
					logger.Warn("failed to write review status", "error", err)
				}
				fmt.Printf("[task] Review complete: status=%s\n", overallStatus)
			} else {
				// Mark as approved so the orchestrator accepts it.
				if err := setTaskReviewStatus(planDir, "approved"); err != nil {
					return fmt.Errorf("setting review status: %w", err)
				}
			}

			// Launch orchestrator.
			fmt.Printf("[task] Launching orchestrator...\n")
			_, err = orchestrator.Launch(ctx, orchestrator.LaunchOptions{
				PlanName:    planName,
				PlansDir:    plansDir,
				ArcHome:     arcHome,
				ProjectDir:  projectRoot,
				Config:      cfg,
				Logger:      logger,
				Timeout:     timeout,
				UseWorktree: true,
				Resolver:    resolver,
			})
			if err != nil {
				return fmt.Errorf("orchestrator failed: %w", err)
			}

			fmt.Printf("[task] Done. Plan: %s\n", planName)
			return nil
		},
	}

	cmd.Flags().BoolVar(&run, "run", true, "Review and run the plan after planning (set false to plan only)")
	cmd.Flags().StringVar(&planName, "plan-name", "", "Override the auto-generated plan name")
	cmd.Flags().IntVar(&timeout, "timeout", 14400, "Wall-clock timeout in seconds")
	cmd.Flags().StringVar(&model, "model", "", "Model override for all agents")
	cmd.Flags().BoolVar(&skipReview, "skip-review", false, "Skip adversarial review before running")

	return cmd
}

// taskStopWords is the set of common words excluded from plan name generation.
var taskStopWords = map[string]bool{
	"the": true, "a": true, "an": true, "to": true,
	"in": true, "for": true, "of": true, "and": true,
	"or": true, "is": true, "it": true, "on": true,
	"at": true, "by": true, "with": true,
}

// taskPlanName generates a valid plan name from a description. It lowercases
// the description, strips non-ASCII-alphanumeric characters, removes stop
// words, takes the first four significant words, joins them with hyphens, and
// truncates to 30 characters. A numeric suffix (-2, -3, …) is appended when
// the name conflicts with an existing plan directory.
func taskPlanName(description, plansDir string) string {
	s := strings.ToLower(description)

	var cleaned []rune
	for _, r := range s {
		if (unicode.IsLetter(r) && r < 128) || unicode.IsDigit(r) || r == ' ' || r == '-' {
			cleaned = append(cleaned, r)
		}
	}
	s = string(cleaned)

	words := strings.Fields(s)

	var significant []string
	for _, w := range words {
		if !taskStopWords[w] {
			significant = append(significant, w)
		}
	}

	if len(significant) > 4 {
		significant = significant[:4]
	}

	name := strings.Join(significant, "-")

	if len(name) > 30 {
		name = name[:30]
	}
	name = strings.TrimRight(name, "-")

	// Fall back when the result is not a valid plan name.
	if !isValidPlanName(name) {
		name = "task-plan"
	}

	// Resolve conflicts by appending -2, -3, etc.
	base := name
	suffix := 2
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(filepath.Join(plansDir, name)); os.IsNotExist(err) {
			break
		}
		name = fmt.Sprintf("%s-%d", base, suffix)
		suffix++
	}

	return name
}

// isValidPlanName reports whether s satisfies the plan name regex
// ^[a-z][a-z0-9-]*[a-z0-9]$ with a minimum length of 2.
func isValidPlanName(s string) bool {
	if len(s) < 2 {
		return false
	}
	// First character must be a lowercase ASCII letter.
	if s[0] < 'a' || s[0] > 'z' {
		return false
	}
	// Last character must be a lowercase letter or digit.
	last := s[len(s)-1]
	if !((last >= 'a' && last <= 'z') || (last >= '0' && last <= '9')) {
		return false
	}
	// Middle characters: lowercase letter, digit, or hyphen.
	for _, r := range s[1 : len(s)-1] {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

// setTaskReviewStatus writes the review_status field of plan.json.
func setTaskReviewStatus(planDir, status string) error {
	meta, err := state.ReadPlan(planDir)
	if err != nil {
		return err
	}
	meta.ReviewStatus = status
	return state.WritePlan(planDir, meta)
}
