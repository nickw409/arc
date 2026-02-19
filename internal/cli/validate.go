package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nwiley/arc/internal/validate"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	var (
		timeout  int
		maxTurns int
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

			language := validate.TryLoadLanguage(cwd)

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			report, err := validate.Run(context.Background(), validate.Options{
				Paths:    paths,
				Language: language,
				Timeout:  time.Duration(timeout) * time.Second,
				MaxTurns: maxTurns,
				Model:    model,
				Logger:   logger,
			})
			if err != nil {
				return err
			}

			printReport(cmd, report)

			if report.Verdict == "fail" {
				os.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&timeout, "timeout", 600, "agent timeout in seconds")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 30, "max agent conversation turns")
	cmd.Flags().StringVar(&model, "model", "", "model override")

	return cmd
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
