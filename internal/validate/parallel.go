package validate

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/resources"
)

// ParallelOptions configures a parallel validate run.
type ParallelOptions struct {
	Batches  []Batch
	Language string
	Workers  int
	Timeout  time.Duration
	Model    string
	Logger   *slog.Logger
}

// RunParallel fans out one agent per batch and merges the results.
func RunParallel(ctx context.Context, opts ParallelOptions) (*Report, error) {
	if len(opts.Batches) == 0 {
		return &Report{Verdict: "pass"}, nil
	}

	tmplBytes, err := resources.PromptBytes("validate/batch-audit.md")
	if err != nil {
		return nil, fmt.Errorf("loading batch-audit prompt: %w", err)
	}
	tmpl := string(tmplBytes)

	workers := opts.Workers
	if workers <= 0 {
		workers = 4
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	type indexedResult struct {
		index  int
		report *Report
		err    error
	}

	results := make([]indexedResult, len(opts.Batches))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, batch := range opts.Batches {
		wg.Add(1)
		go func(idx int, b Batch) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = indexedResult{index: idx, err: ctx.Err()}
				return
			}

			report, err := runBatch(ctx, b, tmpl, opts)
			results[idx] = indexedResult{index: idx, report: report, err: err}
		}(i, batch)
	}

	wg.Wait()

	var reports []*Report
	for _, r := range results {
		if r.err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			opts.Logger.Warn("batch failed", "package", opts.Batches[r.index].Package, "error", r.err)
			continue
		}
		if r.report != nil {
			reports = append(reports, r.report)
		}
	}

	return MergeReports(reports), nil
}

func runBatch(ctx context.Context, batch Batch, tmpl string, opts ParallelOptions) (*Report, error) {
	prompt := renderBatchPrompt(tmpl, batch, opts.Language)

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	opts.Logger.Info("auditing batch", "package", batch.Package, "files", len(batch.Files), "lines", batch.Lines)

	result, err := agent.Spawn(ctx, agent.SpawnOptions{
		Prompt:       prompt,
		AllowedTools: []string{"none"},
		MaxTurns:     2,
		Timeout:      timeout,
		Model:        opts.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("spawning batch agent for %s: %w", batch.Package, err)
	}

	if result.TimedOut {
		return &Report{
			Findings: []Finding{{
				Severity:    SeverityCritical,
				Location:    batch.Package,
				Category:    "Timeout",
				Description: fmt.Sprintf("Audit agent timed out for package %s", batch.Package),
			}},
			Summary:   Summary{Critical: 1},
			Verdict:   "fail",
			RawOutput: result.Output,
		}, nil
	}

	report, err := ParseReport(result.Output)
	if err != nil {
		return &Report{
			Findings: []Finding{{
				Severity:    SeverityCritical,
				Location:    batch.Package,
				Category:    "ParseError",
				Description: fmt.Sprintf("Failed to parse audit output for %s: %v", batch.Package, err),
			}},
			Summary:   Summary{Critical: 1},
			Verdict:   "fail",
			RawOutput: result.Output,
		}, nil
	}

	report.RawOutput = result.Output
	return report, nil
}

func renderBatchPrompt(tmpl string, batch Batch, language string) string {
	result := strings.Replace(tmpl, "{{package}}", batch.Package, 1)

	if language != "" {
		langBlock := fmt.Sprintf("The project language is **%s**. Use language-specific conventions when evaluating test quality.", language)
		result = strings.Replace(result, "{{#if language}}\nThe project language is **{{language}}**. Use language-specific conventions when evaluating test quality.\n{{/if}}", langBlock, 1)
	} else {
		result = strings.Replace(result, "{{#if language}}\nThe project language is **{{language}}**. Use language-specific conventions when evaluating test quality.\n{{/if}}", "", 1)
	}

	result = strings.Replace(result, "{{files}}", formatFileContents(batch.Files), 1)
	return result
}

func formatFileContents(files []FileEntry) string {
	var sb strings.Builder
	for i, f := range files {
		if i > 0 {
			sb.WriteString("\n")
		}
		label := "source"
		if f.IsTest {
			label = "test"
		}
		fmt.Fprintf(&sb, "### `%s` (%s)\n\n", f.Path, label)
		sb.WriteString("```\n")
		sb.WriteString(f.Content)
		if !strings.HasSuffix(f.Content, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n")
	}
	return sb.String()
}

// MergeReports combines multiple batch reports into a single report.
func MergeReports(reports []*Report) *Report {
	if len(reports) == 0 {
		return &Report{Verdict: "pass"}
	}

	merged := &Report{Verdict: "pass"}
	var rawParts []string

	for _, r := range reports {
		merged.Findings = append(merged.Findings, r.Findings...)
		merged.Summary.FilesAudited += r.Summary.FilesAudited
		merged.Summary.Critical += r.Summary.Critical
		merged.Summary.Warning += r.Summary.Warning
		merged.Summary.Info += r.Summary.Info
		if r.RawOutput != "" {
			rawParts = append(rawParts, r.RawOutput)
		}
	}

	if merged.Summary.Critical > 0 {
		merged.Verdict = "fail"
	}

	merged.RawOutput = strings.Join(rawParts, "\n---\n")
	return merged
}
