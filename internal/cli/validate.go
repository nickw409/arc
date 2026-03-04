package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/validate"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	var (
		timeout  int
		maxTurns int
		workers  int
		model    string
	)

	cmd := &cobra.Command{
		Use:   "validate [paths...]",
		Short: "Audit test quality using an AI agent",
		Long:  "Spawns a read-only AI agent to audit test quality. Works on any project without plan or phase context.",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := args
			if len(paths) == 0 {
				paths = []string{"."}
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			projCfg := validate.TryLoadConfig(cwd)

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			report, err := validate.Run(context.Background(), validate.Options{
				Paths:      paths,
				Language:   projCfg.Language,
				PromptPath: projCfg.PromptPath,
				Timeout:    time.Duration(timeout) * time.Second,
				MaxTurns:   maxTurns,
				Workers:    workers,
				Model:      model,
				Logger:     logger,
			})
			if err != nil {
				return err
			}

			printReport(cmd, report)

			if report.Verdict == "fail" {
				cmd.SilenceUsage = true
				return fmt.Errorf("audit failed: verdict=%s", report.Verdict)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&timeout, "timeout", 600, "agent timeout in seconds")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 30, "max agent conversation turns")
	cmd.Flags().IntVar(&workers, "workers", 4, "max parallel audit agents")
	cmd.Flags().StringVar(&model, "model", "", "model override")

	cmd.AddCommand(
		newValidateSetPromptCmd(),
		newValidateClearPromptCmd(),
	)

	return cmd
}

func newValidateSetPromptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-prompt <path>",
		Short: "Set a custom audit prompt file",
		Long:  "Persists a custom prompt path in .arc.yaml. The file is read on each validate run.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			promptPath := args[0]

			if _, err := os.Stat(promptPath); err != nil {
				return fmt.Errorf("prompt file not found: %w", err)
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			cfg, err := config.Load(cwd)
			if err != nil {
				return fmt.Errorf("loading .arc.yaml: %w", err)
			}

			cfg.Audit.Prompt = promptPath

			if err := config.Save(cwd, cfg); err != nil {
				return fmt.Errorf("saving .arc.yaml: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Validate prompt set to: %s\n", promptPath)
			return nil
		},
	}
}

func newValidateClearPromptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-prompt",
		Short: "Revert to the built-in audit prompt",
		Long:  "Removes the custom prompt path from .arc.yaml, reverting to the embedded default.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			cfg, err := config.Load(cwd)
			if err != nil {
				return fmt.Errorf("loading .arc.yaml: %w", err)
			}

			cfg.Audit.Prompt = ""

			if err := config.Save(cwd, cfg); err != nil {
				return fmt.Errorf("saving .arc.yaml: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Validate prompt reset to built-in default")
			return nil
		},
	}
}

func printReport(cmd *cobra.Command, report *validate.Report) {
	out := cmd.OutOrStdout()

	// Group findings by severity
	var critical, warning, info []validate.Finding
	for _, f := range report.Findings {
		switch f.Severity {
		case validate.SeverityCritical:
			critical = append(critical, f)
		case validate.SeverityWarning:
			warning = append(warning, f)
		case validate.SeverityInfo:
			info = append(info, f)
		}
	}

	if len(critical) > 0 {
		fmt.Fprintln(out, "CRITICAL:")
		for _, f := range critical {
			fmt.Fprintf(out, "  [%s] %s: %s\n", f.Location, f.Category, f.Description)
		}
		fmt.Fprintln(out)
	}

	if len(warning) > 0 {
		fmt.Fprintln(out, "WARNING:")
		for _, f := range warning {
			fmt.Fprintf(out, "  [%s] %s: %s\n", f.Location, f.Category, f.Description)
		}
		fmt.Fprintln(out)
	}

	if len(info) > 0 {
		fmt.Fprintln(out, "INFO:")
		for _, f := range info {
			fmt.Fprintf(out, "  [%s] %s: %s\n", f.Location, f.Category, f.Description)
		}
		fmt.Fprintln(out)
	}

	fmt.Fprintf(out, "Files audited: %d\n", report.Summary.FilesAudited)
	fmt.Fprintf(out, "Critical: %d, Warning: %d, Info: %d\n",
		report.Summary.Critical, report.Summary.Warning, report.Summary.Info)
	fmt.Fprintf(out, "Verdict: %s\n", report.Verdict)
}
