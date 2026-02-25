package monitor

import (
	"fmt"
	"sort"
	"strings"
)

// formatTokens formats a token count in human-readable form (e.g. 52401 → "52k").
func formatTokens(n int) string {
	if n <= 0 {
		return ""
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 10000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%dk", n/1000)
}

// renderHeader renders plan name, workflow type, overall progress, token usage, and last update time.
func (m Model) renderHeader() string {
	meta := m.planMeta

	// First line: plan name, workflow type, progress, update time
	line1 := fmt.Sprintf("Arc Monitor: %s", m.planName)
	if meta.WorkflowType != "" {
		line1 += fmt.Sprintf("  [%s]", meta.WorkflowType)
	}
	line1 += fmt.Sprintf("  %d/%d phases", meta.PhasesComplete, meta.PhasesTotal)
	if !m.lastUpdate.IsZero() {
		line1 += fmt.Sprintf("  updated %s", m.lastUpdate.Format("15:04:05"))
	}

	// Second line: aggregate stats
	var parts []string
	if meta.TotalIterations > 0 {
		parts = append(parts, fmt.Sprintf("%d iter", meta.TotalIterations))
	}
	if meta.TotalTests > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d tests", meta.TotalTestsPassing, meta.TotalTests))
	}
	if meta.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("%s tokens", formatTokens(meta.TotalTokens)))
	}
	if meta.PhasesActive > 0 {
		parts = append(parts, fmt.Sprintf("%d active", meta.PhasesActive))
	}
	if meta.InterventionCount > 0 {
		parts = append(parts, alertStyle.Render(fmt.Sprintf("%d intervention", meta.InterventionCount)))
	}
	if meta.BlockedCount > 0 {
		parts = append(parts, blockedStyle.Render(fmt.Sprintf("%d blocked", meta.BlockedCount)))
	}
	if meta.StuckCount > 0 {
		parts = append(parts, verdictWarnStyle.Render(fmt.Sprintf("%d stuck", meta.StuckCount)))
	}

	header := headerStyle.Render(line1)
	if len(parts) > 0 {
		header += "\n  " + strings.Join(parts, " | ")
	}

	return header
}

// renderInterventionAlerts renders alerts for phases with intervention requests.
func (m Model) renderInterventionAlerts() string {
	var alerts []string
	for _, pv := range m.phases {
		if pv.HasIntervention {
			line := fmt.Sprintf("  [!] INTERVENTION: %s", pv.Name)
			if pv.InterventionReason != "" {
				reason := pv.InterventionReason
				if len(reason) > 70 {
					reason = reason[:67] + "..."
				}
				line += fmt.Sprintf(" — %q", reason)
			}
			alerts = append(alerts, alertStyle.Render(line))
		}
	}
	if len(alerts) == 0 {
		return ""
	}
	return "\n" + strings.Join(alerts, "\n") + "\n"
}

// renderPhaseTable renders the column-header and all phase rows.
func (m Model) renderPhaseTable() string {
	if len(m.phases) == 0 {
		return "\n  No phases found.\n"
	}

	width := m.width
	if width <= 0 {
		width = 80
	}

	var b strings.Builder
	b.WriteString("\n")

	// Column header
	b.WriteString(dimStyle.Render(renderColumnHeader(width)))
	b.WriteString("\n")

	for i, pv := range m.phases {
		selected := i == m.selectedIdx
		b.WriteString(renderPhaseRow(pv, width, selected))
		b.WriteString("\n")
	}
	return b.String()
}

// renderColumnHeader renders the column header row.
func renderColumnHeader(width int) string {
	nameW := phaseNameWidth(width)

	if width < 80 {
		return fmt.Sprintf("  %s %-*s  %s", "   ", nameW, "PHASE", "STATE")
	}

	header := fmt.Sprintf("  %s %-*s  %-12s %-7s %-9s", "   ", nameW, "PHASE", "STATE", "ITER", "TESTS")
	if width >= 90 {
		header += fmt.Sprintf(" %-9s", "TOKENS")
	}
	if width >= 90 {
		header += fmt.Sprintf(" %s", "VERDICT")
	}
	return header
}

func phaseNameWidth(width int) int {
	if width < 60 {
		return 15
	}
	if width < 100 {
		return 20
	}
	return 24
}

// renderPhaseRow renders one phase row with all columns.
func renderPhaseRow(pv PhaseView, width int, selected bool) string {
	if width <= 0 {
		width = 80
	}

	nameW := phaseNameWidth(width)

	// Phase name with model override indicator
	name := pv.Name
	if pv.ModelOverride != "" {
		name += "*"
	}
	if len(name) > nameW {
		name = name[:nameW-1] + "~"
	}

	// Selection cursor
	cursor := "  "
	if selected {
		cursor = "> "
	}

	icon := pv.Icon

	// Style based on status
	style := pendingStyle
	switch pv.Status {
	case "implementing", "qa", "qa_review", "impl_review", "adversary":
		style = activeStyle
	case "complete":
		style = completedStyle
	case "blocked":
		style = blockedStyle
	case "disputed":
		style = disputedStyle
	}

	// Narrow terminal: just icon, name, state
	if width < 80 {
		state := pv.CurrentState
		if state == "" {
			state = pv.Status
		}
		row := fmt.Sprintf("%s%s %-*s  %s", cursor, icon, nameW, name, state)
		if selected {
			return selectedStyle.Render(style.Render(row))
		}
		return style.Render(row)
	}

	// State column
	state := pv.CurrentState
	if state == "" {
		switch pv.Status {
		case "pending", "complete", "blocked", "deferred":
			state = "—"
		default:
			state = pv.Status
		}
	}

	// Iter column
	var iter string
	if pv.Iteration > 0 || pv.MaxIteration > 0 {
		prefix := ""
		if pv.StuckIterations > 0 {
			prefix = "~"
		}
		iter = fmt.Sprintf("%s%d/%d", prefix, pv.Iteration, pv.MaxIteration)
	} else {
		iter = "—"
	}

	// Tests column
	var tests string
	if pv.TestsTotal > 0 {
		tests = fmt.Sprintf("%d/%d", pv.TestsPassing, pv.TestsTotal)
	} else {
		tests = "—"
	}

	row := fmt.Sprintf("%s%s %-*s  %-12s %-7s %-9s", cursor, icon, nameW, name, state, iter, tests)

	// Tokens column (wide terminal)
	if width >= 90 {
		total := pv.InputTokens + pv.OutputTokens
		if total > 0 {
			row += fmt.Sprintf(" %-9s", formatTokens(total))
		} else {
			row += fmt.Sprintf(" %-9s", "—")
		}
	}

	// Verdict column (wide terminal)
	if width >= 90 {
		if pv.LastVerdict != "" {
			row += " " + VerdictStyle(pv.LastVerdict).Render(pv.LastVerdict)
		} else {
			row += " " + dimStyle.Render("—")
		}

		// Completed timestamp
		if pv.Status == "complete" && pv.CompletedAt != "" {
			row += "  " + dimStyle.Render(pv.CompletedAt)
		}
	}

	if selected {
		return selectedStyle.Render(style.Render(row))
	}
	return style.Render(row)
}

// renderDetailPanel renders a full-screen detail view for the selected phase.
func (m Model) renderDetailPanel() string {
	if m.selectedIdx >= len(m.phases) {
		return ""
	}
	pv := m.phases[m.selectedIdx]

	var lines []string

	// Header
	header := fmt.Sprintf("  Phase: %s", pv.Name)
	lines = append(lines, headerStyle.Render(header))
	lines = append(lines, dimStyle.Render("  esc: back  ↑/↓: scroll  r: refresh"))
	lines = append(lines, "")

	// Status row
	statusLine := fmt.Sprintf("  Status: %-14s Workflow: %-12s", pv.Status, pv.WorkflowType)
	if pv.ModelOverride != "" {
		statusLine += fmt.Sprintf(" Model: %s", pv.ModelOverride)
	}
	lines = append(lines, statusLine)

	// State/iter row
	stateLine := fmt.Sprintf("  State:  %-14s Iter: %-14s", pv.CurrentState, formatIter(pv))
	if pv.GlobalIterations > 0 {
		stateLine += fmt.Sprintf(" Global iters: %d", pv.GlobalIterations)
	}
	lines = append(lines, stateLine)

	// Tests/rollback row
	testsLine := fmt.Sprintf("  Tests:  %-14s Rollbacks: %-9d Stuck: %-8d Hangs: %d",
		formatTests(pv), pv.RollbackCount, pv.StuckIterations, pv.HangCount)
	lines = append(lines, testsLine)

	// Commit/tokens row
	commitLine := "  Commit: "
	if pv.LastCommit != "" {
		commitLine += fmt.Sprintf("%-14s", pv.LastCommit)
	} else {
		commitLine += fmt.Sprintf("%-14s", "—")
	}
	total := pv.InputTokens + pv.OutputTokens
	if total > 0 {
		commitLine += fmt.Sprintf(" Tokens: %s (in: %s, out: %s)",
			formatTokens(total), formatTokens(pv.InputTokens), formatTokens(pv.OutputTokens))
	}
	if pv.CompletedAt != "" {
		commitLine += fmt.Sprintf("  Completed: %s", pv.CompletedAt)
	}
	lines = append(lines, commitLine)

	// Blocked/deferred reason
	if pv.BlockedReason != "" {
		lines = append(lines, blockedStyle.Render(fmt.Sprintf("  Blocked: %s", pv.BlockedReason)))
	}
	if pv.DeferredReason != "" {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  Deferred: %s", pv.DeferredReason)))
	}

	// Notes
	if pv.Notes != "" {
		lines = append(lines, fmt.Sprintf("  Notes:  %s", pv.Notes))
	}

	// Chunks
	if pv.ChunksTotal > 0 {
		lines = append(lines, "")
		chunkLine := fmt.Sprintf("  Chunks: %d/%d complete", pv.ChunksDone, pv.ChunksTotal)
		if pv.ChunkCurrent != "" {
			chunkLine += fmt.Sprintf("   Current: %q", pv.ChunkCurrent)
		}
		lines = append(lines, chunkLine)
	}

	// Intervention
	if pv.HasIntervention {
		lines = append(lines, "")
		lines = append(lines, alertStyle.Render("  INTERVENTION REQUIRED"))
		lines = append(lines, alertStyle.Render(fmt.Sprintf("  %s", pv.InterventionReason)))
		if len(pv.InterventionOptions) > 0 {
			lines = append(lines, fmt.Sprintf("  Options: %s", strings.Join(pv.InterventionOptions, " | ")))
		}
	}

	// Verdict history
	if len(pv.VerdictHistory) > 0 {
		lines = append(lines, "")
		lines = append(lines, sectionHeaderStyle.Render("  Verdict History"))
		for _, v := range pv.VerdictHistory {
			verdictText := VerdictStyle(v.Verdict).Render(v.Verdict)
			lines = append(lines, fmt.Sprintf("    iter %-3d %-14s %s  %s",
				v.Iteration, v.State, verdictText, dimStyle.Render(v.Timestamp)))
		}
	}

	// Escalations
	if len(pv.ExecutedEscalations) > 0 {
		lines = append(lines, "")
		lines = append(lines, sectionHeaderStyle.Render("  Escalations"))
		for _, e := range pv.ExecutedEscalations {
			lines = append(lines, fmt.Sprintf("    %s", e))
		}
	}

	// Disputes
	if len(pv.DisputeDetails) > 0 {
		lines = append(lines, "")
		lines = append(lines, sectionHeaderStyle.Render(fmt.Sprintf("  Disputes (%d)", len(pv.DisputeDetails))))
		for _, d := range pv.DisputeDetails {
			lines = append(lines, fmt.Sprintf("    %s — %q", d.TestName, d.Reason))
		}
	}

	// Parallel execution
	if pv.HasParallel {
		lines = append(lines, "")
		header := "  Parallel Execution"
		if pv.ParallelVerdict != "" {
			header += fmt.Sprintf("  verdict: %s", pv.ParallelVerdict)
		}
		lines = append(lines, sectionHeaderStyle.Render(header))

		// Sort branch names for stable display
		names := make([]string, 0, len(pv.ParallelBranches))
		for name := range pv.ParallelBranches {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			status := pv.ParallelBranches[name]
			lines = append(lines, fmt.Sprintf("    %s: %s", name, status))
		}
	}

	// Apply scroll offset
	visibleHeight := m.height - 2 // leave room for scroll indicator
	if visibleHeight <= 0 {
		visibleHeight = 20
	}

	scroll := m.detailScroll
	if scroll > len(lines)-visibleHeight {
		scroll = len(lines) - visibleHeight
	}
	if scroll < 0 {
		scroll = 0
	}

	end := scroll + visibleHeight
	if end > len(lines) {
		end = len(lines)
	}

	visible := lines[scroll:end]

	// Scroll indicator
	if len(lines) > visibleHeight {
		indicator := dimStyle.Render(fmt.Sprintf("  — line %d-%d of %d —", scroll+1, end, len(lines)))
		visible = append(visible, indicator)
	}

	return strings.Join(visible, "\n")
}

func formatIter(pv PhaseView) string {
	if pv.Iteration > 0 || pv.MaxIteration > 0 {
		prefix := ""
		if pv.StuckIterations > 0 {
			prefix = "~"
		}
		return fmt.Sprintf("%s%d/%d", prefix, pv.Iteration, pv.MaxIteration)
	}
	return "—"
}

func formatTests(pv PhaseView) string {
	if pv.TestsTotal > 0 {
		return fmt.Sprintf("%d/%d", pv.TestsPassing, pv.TestsTotal)
	}
	return "—"
}

// renderFooter renders the footer with keybinding hints.
func (m Model) renderFooter() string {
	return footerStyle.Render("  ↑/↓: select  enter: detail  r: refresh  q: quit  Refreshing every 3s")
}
