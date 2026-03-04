package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nwiley/arc/internal/arc"
	"github.com/nwiley/arc/internal/plan"
	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
)

// addPlanSpecSubcommands registers plan spec subcommands onto the given plan command.
func addPlanSpecSubcommands(planCmd *cobra.Command) {
	planCmd.AddCommand(
		newPlanAddPhaseCmd(),
		newPlanRemovePhaseCmd(),
		newPlanUpdatePhaseCmd(),
		newPlanUpdateGateCmd(),
		newPlanUpdateDepsCmd(),
		newPlanShowSpecCmd(),
	)
}

func plansDir() string {
	return filepath.Join(".plans", "active")
}

func newPlanAddPhaseCmd() *cobra.Command {
	var (
		specText   string
		testCmd    string
		complexity string
		deps       string
		files      string
	)

	cmd := &cobra.Command{
		Use:   "add-phase <plan> <phase>",
		Short: "Add a new phase to an existing plan",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			phaseName := args[1]

			if specText == "" {
				return fmt.Errorf("--spec is required")
			}

			spec := &arc.PhaseSpec{
				Spec:       specText,
				Test:       testCmd,
				Complexity: complexity,
			}

			if files != "" {
				spec.Files = splitCSV(files)
			}
			if deps != "" {
				spec.Deps = splitCSV(deps)
			}

			if err := plan.AddPhase(plansDir(), planName, phaseName, spec); err != nil {
				return err
			}

			fmt.Printf("Added phase %q to plan %q\n", phaseName, planName)
			return nil
		},
	}

	cmd.Flags().StringVar(&specText, "spec", "", "Phase description/objective (required)")
	cmd.Flags().StringVar(&testCmd, "test", "", "Scoped test command")
	cmd.Flags().StringVar(&complexity, "complexity", "medium", "Task complexity: simple, medium, or complex")
	cmd.Flags().StringVar(&deps, "deps", "", "Comma-separated dependency phases")
	cmd.Flags().StringVar(&files, "file", "", "Comma-separated relevant file paths")

	return cmd
}

func newPlanRemovePhaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-phase <plan> <phase>",
		Short: "Remove a pending phase from a plan",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			phaseName := args[1]

			if err := plan.RemovePhase(plansDir(), planName, phaseName); err != nil {
				return err
			}

			fmt.Printf("Removed phase %q from plan %q\n", phaseName, planName)
			return nil
		},
	}
}

func newPlanUpdatePhaseCmd() *cobra.Command {
	var (
		specText   string
		testCmd    string
		complexity string
	)

	cmd := &cobra.Command{
		Use:   "update-phase <plan> <phase>",
		Short: "Update the spec for a phase",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			phaseName := args[1]

			// Read existing spec and overlay the provided flags
			existing, err := plan.ReadSpec(plansDir(), planName, phaseName)
			if err != nil {
				// If spec.yaml doesn't exist yet, start from an empty spec
				existing = &arc.PhaseSpec{}
			}

			if specText != "" {
				existing.Spec = specText
			}
			if cmd.Flags().Changed("test") {
				existing.Test = testCmd
			}
			if cmd.Flags().Changed("complexity") {
				existing.Complexity = complexity
			}

			if err := plan.UpdateSpec(plansDir(), planName, phaseName, existing); err != nil {
				return err
			}

			fmt.Printf("Updated spec for %s/%s\n", planName, phaseName)
			return nil
		},
	}

	cmd.Flags().StringVar(&specText, "spec", "", "New phase description")
	cmd.Flags().StringVar(&testCmd, "test", "", "Scoped test command")
	cmd.Flags().StringVar(&complexity, "complexity", "", "Task complexity: simple, medium, or complex")

	return cmd
}

func newPlanUpdateGateCmd() *cobra.Command {
	var (
		addAssertion  string
		verifierAgent bool
	)

	cmd := &cobra.Command{
		Use:   "update-gate <plan> <phase>",
		Short: "Update the gate section of a phase's spec",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			phaseName := args[1]

			existing, err := plan.ReadSpec(plansDir(), planName, phaseName)
			if err != nil {
				existing = &arc.PhaseSpec{}
			}

			gate := existing.Gate

			if cmd.Flags().Changed("add-assertion") {
				parsed, err := parseAssertion(addAssertion)
				if err != nil {
					return err
				}
				gate.Assertions = append(gate.Assertions, parsed)
			}

			if cmd.Flags().Changed("verifier-agent") {
				gate.VerifierAgent = verifierAgent
			}

			if err := plan.UpdateGate(plansDir(), planName, phaseName, gate); err != nil {
				return err
			}

			fmt.Printf("Updated gate for %s/%s\n", planName, phaseName)
			return nil
		},
	}

	cmd.Flags().StringVar(&addAssertion, "add-assertion", "", "Add an assertion (e.g. file_exists:path, grep:pattern, test_exists:name)")
	cmd.Flags().BoolVar(&verifierAgent, "verifier-agent", false, "Enable AI verifier agent")

	return cmd
}

func newPlanUpdateDepsCmd() *cobra.Command {
	var depsStr string

	cmd := &cobra.Command{
		Use:   "update-deps <plan> <phase>",
		Short: "Update the dependency edges for a phase",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			phaseName := args[1]

			var deps []string
			if depsStr != "" {
				deps = splitCSV(depsStr)
			}

			if err := plan.UpdateDeps(plansDir(), planName, phaseName, deps); err != nil {
				return err
			}

			if len(deps) == 0 {
				fmt.Printf("Cleared dependencies for %s/%s\n", planName, phaseName)
			} else {
				fmt.Printf("Updated dependencies for %s/%s: %s\n", planName, phaseName, strings.Join(deps, ", "))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&depsStr, "deps", "", "Comma-separated dependency phases (empty clears all deps)")
	return cmd
}

func newPlanShowSpecCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show-spec <plan> <phase>",
		Short: "Print the spec.yaml for a phase",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			planName := args[0]
			phaseName := args[1]

			spec, err := plan.ReadSpec(plansDir(), planName, phaseName)
			if err != nil {
				return err
			}

			data, err := yaml.Marshal(spec)
			if err != nil {
				return fmt.Errorf("formatting spec: %w", err)
			}

			_, err = os.Stdout.Write(data)
			return err
		},
	}
}

// splitCSV splits a comma-separated string, trimming whitespace from each element.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// parseAssertion parses a "type:target" string into a GateAssertion.
// Supported types: file_exists, grep, test_exists.
func parseAssertion(a string) (arc.GateAssertion, error) {
	idx := strings.Index(a, ":")
	if idx < 0 {
		return arc.GateAssertion{}, fmt.Errorf("invalid assertion %q: must be in format type:target (e.g. file_exists:path)", a)
	}
	assertType := a[:idx]
	target := a[idx+1:]
	ga := arc.GateAssertion{Type: assertType, Target: target}
	switch assertType {
	case "file_exists":
		ga.FileExists = target
	case "grep":
		ga.Grep = target
	case "test_exists":
		ga.TestExists = target
	default:
		return arc.GateAssertion{}, fmt.Errorf("unknown assertion type %q: supported types are file_exists, grep, test_exists", assertType)
	}
	return ga, nil
}
