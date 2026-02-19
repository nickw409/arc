package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nwiley/arc/internal/plan"
	"github.com/spf13/cobra"
)

func newManageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manage <plan> <phase> <action> [args...]",
		Short: "Manage phase state",
		Long: `Manage phase state for a plan.

Actions:
  complete              Mark phase as complete
  pending               Reset phase to pending
  defer --reason <msg>  Defer phase with reason
  block --reason <msg>  Block phase with reason
  tests <passing> <total>  Update test counts
  packages <pkg1,pkg2,...>  Set packages list
  note <text>           Set notes field
  iteration <n>         Set current iteration
  copy-from <phase>     Copy state from another phase
  show                  Pretty-print state.json`,
	}

	cmd.AddCommand(
		newManageCompleteCmd(),
		newManagePendingCmd(),
		newManageDeferCmd(),
		newManageBlockCmd(),
		newManageTestsCmd(),
		newManagePackagesCmd(),
		newManageNoteCmd(),
		newManageIterationCmd(),
		newManageCopyFromCmd(),
		newManageShowCmd(),
	)

	return cmd
}

func baseManageOpts(args []string) plan.ManageOptions {
	return plan.ManageOptions{
		PlansDir: filepath.Join(".plans", "active"),
		PlanName: args[0],
		Phase:    args[1],
	}
}

func newManageCompleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "complete <plan> <phase>",
		Short: "Mark phase as complete",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := baseManageOpts(args)
			if err := plan.ManageComplete(opts); err != nil {
				return err
			}
			fmt.Printf("Marked %s/%s as complete\n", args[0], args[1])
			return nil
		},
	}
}

func newManagePendingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pending <plan> <phase>",
		Short: "Reset phase to pending",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := baseManageOpts(args)
			if err := plan.ManagePending(opts); err != nil {
				return err
			}
			fmt.Printf("Reset %s/%s to pending\n", args[0], args[1])
			return nil
		},
	}
}

func newManageDeferCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "defer <plan> <phase>",
		Short: "Defer phase with reason",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := baseManageOpts(args)
			opts.Reason = reason
			if err := plan.ManageDefer(opts); err != nil {
				return err
			}
			fmt.Printf("Deferred %s/%s\n", args[0], args[1])
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Reason for deferral")
	cmd.MarkFlagRequired("reason")
	return cmd
}

func newManageBlockCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "block <plan> <phase>",
		Short: "Block phase with reason",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := baseManageOpts(args)
			opts.Reason = reason
			if err := plan.ManageBlock(opts); err != nil {
				return err
			}
			fmt.Printf("Blocked %s/%s\n", args[0], args[1])
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Reason for blocking")
	cmd.MarkFlagRequired("reason")
	return cmd
}

func newManageTestsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tests <plan> <phase> <passing> <total>",
		Short: "Update test counts",
		Args:  cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := baseManageOpts(args)

			passing, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("invalid passing count: %w", err)
			}
			total, err := strconv.Atoi(args[3])
			if err != nil {
				return fmt.Errorf("invalid total count: %w", err)
			}

			opts.Passing = passing
			opts.Total = total
			if err := plan.ManageTests(opts); err != nil {
				return err
			}
			fmt.Printf("Updated tests for %s/%s: %d/%d\n", args[0], args[1], passing, total)
			return nil
		},
	}
}

func newManagePackagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "packages <plan> <phase> <pkg1,pkg2,...>",
		Short: "Set packages list",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := baseManageOpts(args)
			opts.Packages = strings.Split(args[2], ",")
			if err := plan.ManagePackages(opts); err != nil {
				return err
			}
			fmt.Printf("Updated packages for %s/%s\n", args[0], args[1])
			return nil
		},
	}
}

func newManageNoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "note <plan> <phase> <text>",
		Short: "Set notes field",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := baseManageOpts(args)
			opts.Note = args[2]
			if err := plan.ManageNote(opts); err != nil {
				return err
			}
			fmt.Printf("Updated note for %s/%s\n", args[0], args[1])
			return nil
		},
	}
}

func newManageIterationCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "iteration <plan> <phase> <n>",
		Short: "Set current iteration",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := baseManageOpts(args)
			n, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("invalid iteration number: %w", err)
			}
			opts.Iteration = n
			if err := plan.ManageIteration(opts); err != nil {
				return err
			}
			fmt.Printf("Set iteration for %s/%s to %d\n", args[0], args[1], n)
			return nil
		},
	}
}

func newManageCopyFromCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "copy-from <plan> <phase> <source-phase>",
		Short: "Copy state from another phase",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := baseManageOpts(args)
			opts.SourcePhase = args[2]
			if err := plan.ManageCopyFrom(opts); err != nil {
				return err
			}
			fmt.Printf("Copied state from %s to %s/%s\n", args[2], args[0], args[1])
			return nil
		},
	}
}

func newManageShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <plan> <phase>",
		Short: "Pretty-print state.json",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := baseManageOpts(args)
			return plan.ManageShow(os.Stdout, opts)
		},
	}
}
