package validate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nwiley/arc/internal/agent"
	"github.com/nwiley/arc/internal/config"
	"github.com/nwiley/arc/internal/resources"
)

// Severity represents the severity level of a finding.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityWarning  Severity = "WARNING"
	SeverityInfo     Severity = "INFO"
)

// Finding represents a single audit finding.
type Finding struct {
	Severity    Severity
	Location    string
	Category    string
	Description string
}

// Report holds the parsed output of a test quality audit.
type Report struct {
	Findings  []Finding
	Summary   Summary
	Verdict   string
	RawOutput string
}

// Summary holds aggregate counts from the audit.
type Summary struct {
	FilesAudited int
	Critical     int
	Warning      int
	Info         int
}

// HasCritical returns true if there are any critical findings.
func (r *Report) HasCritical() bool {
	return r.Summary.Critical > 0
}

// Options configures a validate run.
type Options struct {
	Paths      []string
	Language   string
	PromptPath string // custom prompt file path; if empty, uses embedded default
	Timeout    time.Duration
	MaxTurns   int
	Model      string
	Logger     *slog.Logger
}

// Run executes a test quality audit by spawning an AI agent.
func Run(ctx context.Context, opts Options) (*Report, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	promptBytes, err := loadPrompt(opts.PromptPath)
	if err != nil {
		return nil, err
	}

	prompt := renderPrompt(string(promptBytes), opts)

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	maxTurns := opts.MaxTurns
	if maxTurns == 0 {
		maxTurns = 30
	}

	opts.Logger.Info("starting test quality audit", "paths", opts.Paths, "language", opts.Language)

	result, err := agent.Spawn(ctx, agent.SpawnOptions{
		Prompt:       prompt,
		AllowedTools: []string{"View", "Grep", "Glob"},
		MaxTurns:     maxTurns,
		Timeout:      timeout,
		Model:        opts.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("spawning audit agent: %w", err)
	}

	if result.TimedOut {
		return &Report{
			Findings: []Finding{{
				Severity:    SeverityCritical,
				Location:    "",
				Category:    "Timeout",
				Description: "Audit agent timed out before completing analysis",
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
				Location:    "",
				Category:    "ParseError",
				Description: fmt.Sprintf("Failed to parse audit output: %v", err),
			}},
			Summary:   Summary{Critical: 1},
			Verdict:   "fail",
			RawOutput: result.Output,
		}, nil
	}

	report.RawOutput = result.Output
	return report, nil
}

func renderPrompt(template string, opts Options) string {
	pathsStr := strings.Join(opts.Paths, "\n")
	result := strings.Replace(template, "{{paths}}", pathsStr, 1)

	if opts.Language != "" {
		langBlock := fmt.Sprintf("The project language is **%s**. Use language-specific conventions when evaluating test quality.", opts.Language)
		result = strings.Replace(result, "{{#if language}}\nThe project language is **{{language}}**. Use language-specific conventions when evaluating test quality.\n{{/if}}", langBlock, 1)
	} else {
		result = strings.Replace(result, "{{#if language}}\nThe project language is **{{language}}**. Use language-specific conventions when evaluating test quality.\n{{/if}}", "", 1)
	}

	return result
}

// findingLineRe matches lines like: - [file:line] Category: Description
var findingLineRe = regexp.MustCompile(`^- \[([^\]]+)\] ([^:]+): (.+)$`)

// ParseReport extracts structured data from raw audit agent output.
func ParseReport(output string) (*Report, error) {
	verdict, err := extractVerdict(output)
	if err != nil {
		return nil, err
	}

	findings := parseFindings(output)

	summary := Summary{}
	for _, f := range findings {
		switch f.Severity {
		case SeverityCritical:
			summary.Critical++
		case SeverityWarning:
			summary.Warning++
		case SeverityInfo:
			summary.Info++
		}
	}

	// Parse files audited from summary section
	summary.FilesAudited = parseSummaryCount(output, "Files audited")

	return &Report{
		Findings: findings,
		Summary:  summary,
		Verdict:  verdict,
	}, nil
}

func extractVerdict(output string) (string, error) {
	lines := strings.Split(output, "\n")
	verdictIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if lower == "## verdict" || lower == "### verdict" {
			verdictIdx = i
		}
	}

	if verdictIdx == -1 {
		return "", fmt.Errorf("no verdict section found")
	}

	for i := verdictIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if trimmed == "pass" || trimmed == "fail" {
			return trimmed, nil
		}
		return "", fmt.Errorf("invalid verdict value: %q", trimmed)
	}

	return "", fmt.Errorf("no verdict value after header")
}

func parseFindings(output string) []Finding {
	lines := strings.Split(output, "\n")
	var findings []Finding
	var currentSeverity Severity

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		if lower == "### critical" {
			currentSeverity = SeverityCritical
			continue
		}
		if lower == "### warning" {
			currentSeverity = SeverityWarning
			continue
		}
		if lower == "### info" {
			currentSeverity = SeverityInfo
			continue
		}

		// Reset severity on new top-level section
		if strings.HasPrefix(trimmed, "## ") {
			currentSeverity = ""
			continue
		}

		if currentSeverity == "" {
			continue
		}

		if trimmed == "None" || trimmed == "none" {
			continue
		}

		if f, ok := parseFindingLine(trimmed, currentSeverity); ok {
			findings = append(findings, f)
		}
	}

	return findings
}

func parseFindingLine(line string, severity Severity) (Finding, bool) {
	m := findingLineRe.FindStringSubmatch(line)
	if m == nil {
		return Finding{}, false
	}
	return Finding{
		Severity:    severity,
		Location:    m[1],
		Category:    m[2],
		Description: m[3],
	}, true
}

func parseSummaryCount(output, label string) int {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, label) {
			// Extract number after the label
			parts := strings.SplitAfter(trimmed, label+":")
			if len(parts) == 2 {
				numStr := strings.TrimSpace(parts[1])
				// Handle "N, Warning: ..." by taking only the first number
				if commaIdx := strings.Index(numStr, ","); commaIdx != -1 {
					numStr = numStr[:commaIdx]
				}
				var n int
				fmt.Sscanf(numStr, "%d", &n)
				return n
			}
		}
	}
	return 0
}

// ProjectConfig holds validate-relevant settings loaded from .arc.yaml.
type ProjectConfig struct {
	Language   string
	PromptPath string
}

// TryLoadConfig attempts to read validate-relevant settings from .arc.yaml.
// Returns zero-value fields on any failure.
func TryLoadConfig(projectRoot string) ProjectConfig {
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return ProjectConfig{}
	}
	promptPath := cfg.Audit.Prompt
	if promptPath != "" && !filepath.IsAbs(promptPath) {
		promptPath = filepath.Join(projectRoot, promptPath)
	}
	return ProjectConfig{
		Language:   cfg.Language,
		PromptPath: promptPath,
	}
}

func loadPrompt(customPath string) ([]byte, error) {
	if customPath != "" {
		data, err := os.ReadFile(customPath)
		if err != nil {
			return nil, fmt.Errorf("loading custom prompt %q: %w", customPath, err)
		}
		return data, nil
	}
	data, err := resources.PromptBytes("validate/audit.md")
	if err != nil {
		return nil, fmt.Errorf("loading embedded audit prompt: %w", err)
	}
	return data, nil
}
