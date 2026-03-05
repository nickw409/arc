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

	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/orchestrator"
	"github.com/nwiley/arc/internal/recipe"
	"github.com/nwiley/arc/internal/resources"
	"github.com/nwiley/arc/internal/state"
	"github.com/spf13/cobra"
)

func newRecipeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipe",
		Short: "Manage and instantiate plan recipes",
	}

	cmd.AddCommand(
		newRecipeInstantiateCmd(),
		newRecipeListCmd(),
		newRecipeShowCmd(),
	)

	return cmd
}

// recipesDir returns the path to the .arc/recipes directory relative to the
// project root.
func recipesDir(projectRoot string) string {
	return filepath.Join(projectRoot, ".arc", "recipes")
}

func newRecipeInstantiateCmd() *cobra.Command {
	var params []string
	var planName string
	var run bool

	cmd := &cobra.Command{
		Use:   "instantiate <name> [--param key=value]...",
		Short: "Instantiate a recipe and create a plan",
		Long: `Load the named recipe from .arc/recipes/, substitute parameters, and create
a plan under .plans/active/. Use --param key=value (repeatable) to supply
parameters. Use --plan-name to override the default plan name (which is the
recipe name). Use --run to launch the orchestrator after plan creation.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			recipeName := args[0]

			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			// Load the recipe.
			dir := recipesDir(projectRoot)
			r, err := findRecipe(dir, recipeName)
			if err != nil {
				return err
			}

			// Parse --param flags.
			provided, err := parseParams(params)
			if err != nil {
				return err
			}

			// Instantiate.
			inst, err := recipe.Instantiate(r, provided)
			if err != nil {
				return fmt.Errorf("instantiate recipe %q: %w", recipeName, err)
			}

			// Resolve plan name.
			if planName == "" {
				planName = r.Name
			}

			plansDir := filepath.Join(projectRoot, ".plans", "active")
			if err := os.MkdirAll(plansDir, 0755); err != nil {
				return fmt.Errorf("creating plans directory: %w", err)
			}

			// Create plan from instantiated recipe.
			if err := recipe.ToPlan(inst, plansDir, planName); err != nil {
				return fmt.Errorf("creating plan: %w", err)
			}

			fmt.Printf("Created plan %q from recipe %q with %d phases\n",
				planName, recipeName, len(inst.Phases))

			if !run {
				return nil
			}

			// Auto-run: mark as approved and launch orchestrator.
			planDir := filepath.Join(plansDir, planName)
			meta, err := state.ReadPlan(planDir)
			if err != nil {
				return fmt.Errorf("reading plan: %w", err)
			}
			meta.ReviewStatus = "approved"
			if err := state.WritePlan(planDir, meta); err != nil {
				return fmt.Errorf("writing plan review status: %w", err)
			}

			cfg, _ := config.Load(projectRoot)
			if cfg == nil {
				cfg = &config.Config{}
			}
			arcHome := resolveArcHome()
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("getting home directory: %w", err)
			}
			resolver := resources.NewResolver(projectRoot, homeDir)

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

			fmt.Printf("Launching orchestrator for plan %q...\n", planName)
			_, err = orchestrator.Launch(ctx, orchestrator.LaunchOptions{
				PlanName:    planName,
				PlansDir:    plansDir,
				ArcHome:     arcHome,
				ProjectDir:  projectRoot,
				Config:      cfg,
				Logger:      logger,
				UseWorktree: true,
				Resolver:    resolver,
			})
			return err
		},
	}

	cmd.Flags().StringArrayVar(&params, "param", nil, "Parameter key=value (repeatable)")
	cmd.Flags().StringVar(&planName, "plan-name", "", "Override plan name (default: recipe name)")
	cmd.Flags().BoolVar(&run, "run", false, "Launch orchestrator after creating plan")

	return cmd
}

func newRecipeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available recipes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			// Load project-local recipes (may not exist).
			dir := recipesDir(projectRoot)
			localRecipes, _ := recipe.LoadAll(dir)

			// Track names already shown so we don't duplicate built-ins that
			// have been overridden locally.
			seen := make(map[string]bool, len(localRecipes))

			// Load built-in recipes.
			builtIns, _ := recipe.LoadAllBuiltIn()

			total := len(localRecipes) + len(builtIns)
			if total == 0 {
				fmt.Println("No recipes found.")
				return nil
			}

			if len(localRecipes) > 0 {
				fmt.Println("Project recipes (.arc/recipes/):")
				for _, r := range localRecipes {
					seen[r.Name] = true
					if r.Description != "" {
						fmt.Printf("  %-24s  %s\n", r.Name, r.Description)
					} else {
						fmt.Printf("  %s\n", r.Name)
					}
				}
			}

			// Print built-ins that aren't overridden locally.
			var shownBuiltIns []*recipe.Recipe
			for _, r := range builtIns {
				if !seen[r.Name] {
					shownBuiltIns = append(shownBuiltIns, r)
				}
			}
			if len(shownBuiltIns) > 0 {
				if len(localRecipes) > 0 {
					fmt.Println()
				}
				fmt.Println("Built-in recipes:")
				for _, r := range shownBuiltIns {
					if r.Description != "" {
						fmt.Printf("  %-24s  %s [built-in]\n", r.Name, r.Description)
					} else {
						fmt.Printf("  %-24s  [built-in]\n", r.Name)
					}
				}
			}
			return nil
		},
	}
}

func newRecipeShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show recipe details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			recipeName := args[0]

			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			dir := recipesDir(projectRoot)
			r, err := findRecipe(dir, recipeName)
			if err != nil {
				return err
			}

			fmt.Printf("Recipe: %s\n", r.Name)
			if r.Description != "" {
				fmt.Printf("Description: %s\n", r.Description)
			}

			if len(r.Params) > 0 {
				fmt.Println("\nParameters:")
				for _, p := range r.Params {
					if p.Default != "" {
						fmt.Printf("  %-20s  (default: %s)\n", p.Name, p.Default)
					} else {
						fmt.Printf("  %-20s  (required)\n", p.Name)
					}
				}
			}

			if len(r.Phases) > 0 {
				fmt.Println("\nPhases:")
				for _, ph := range r.Phases {
					fmt.Printf("  %s", ph.Name)
					if ph.Complexity != "" {
						fmt.Printf(" [%s]", ph.Complexity)
					}
					if len(ph.Deps) > 0 {
						fmt.Printf(" deps: %s", strings.Join(ph.Deps, ", "))
					}
					fmt.Println()
				}
			}

			return nil
		},
	}
}

// findRecipe looks for a recipe named name in dir, trying both <name>.yaml
// and <name>.yml extensions, and also a file whose decoded Name field matches.
// If not found in the project directory, falls back to built-in recipes.
func findRecipe(dir, name string) (*recipe.Recipe, error) {
	// Try direct filename matches in project directory first.
	for _, ext := range []string{".yaml", ".yml"} {
		path := filepath.Join(dir, name+ext)
		if _, err := os.Stat(path); err == nil {
			return recipe.Load(path)
		}
	}

	// Fall back: load all project recipes and find by name field.
	localRecipes, err := recipe.LoadAll(dir)
	if err == nil || len(localRecipes) > 0 {
		for _, r := range localRecipes {
			if r.Name == name {
				return r, nil
			}
		}
	}

	// Fall back to built-in embedded recipes.
	if r, err := recipe.LoadBuiltIn(name); err == nil {
		return r, nil
	}

	// Check all built-ins by Name field (in case name field differs from filename).
	builtIns, _ := recipe.LoadAllBuiltIn()
	for _, r := range builtIns {
		if r.Name == name {
			return r, nil
		}
	}

	return nil, fmt.Errorf("recipe %q not found (checked .arc/recipes/ and built-ins)", name)
}

// parseParams converts a slice of "key=value" strings to a map.
// Returns an error if any entry is missing the "=" separator.
func parseParams(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(pairs))
	for _, p := range pairs {
		idx := strings.Index(p, "=")
		if idx < 0 {
			return nil, fmt.Errorf("invalid --param %q: must be in key=value format", p)
		}
		key := p[:idx]
		value := p[idx+1:]
		result[key] = value
	}
	return result, nil
}
