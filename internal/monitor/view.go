package monitor

import (
	"fmt"
	"strings"
	"time"
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

// renderHeader renders plan name, workflow type, overall progress, and last update time.
func (m Model) renderHeader() string {
	plan := m.firstPlan()
	meta := plan.Meta

	// First line: plan name, workflow type, progress, update time
	line1 := fmt.Sprintf("Arc Monitor: %s", m.planFilter)
	if plan.WorkflowType != "" {
		line1 += fmt.Sprintf("  [%s]", plan.WorkflowType)
	}
	line1 += fmt.Sprintf("  %d/%d phases", meta.CompletedCount, len(plan.Phases))
	if !m.lastUpdate.IsZero() {
		line1 += fmt.Sprintf("  updated %s", m.lastUpdate.Format("15:04:05"))
	}

	// Second line: aggregate stats
	var parts []string
	if meta.TotalIterations > 0 {
		parts = append(parts, fmt.Sprintf("%d iter", meta.TotalIterations))
	}
	if meta.RunningCount > 0 {
		parts = append(parts, fmt.Sprintf("%d active", meta.RunningCount))
	}
	if meta.FailedCount > 0 {
		parts = append(parts, blockedStyle.Render(fmt.Sprintf("%d blocked", meta.FailedCount)))
	}

	header := headerStyle.Render(line1)
	if len(parts) > 0 {
		header += "\n  " + strings.Join(parts, " | ")
	}

	return header
}

// renderInterventionAlerts renders alerts for phases with intervention requests.
// Intervention is no longer tracked in PhaseView; this is a no-op.
func (m Model) renderInterventionAlerts() string {
	return ""
}

// renderPhaseTable renders the column-header and all phase rows.
func (m Model) renderPhaseTable() string {
	phases := m.firstPlan().Phases
	if len(phases) == 0 {
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

	for i, pv := range phases {
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

	stateW := stateColWidth(width)
	header := fmt.Sprintf("  %s %-*s  %-*s %-7s %-9s", "   ", nameW, "PHASE", stateW, "STATE", "ITER", "TESTS")
	if width >= 90 {
		header += fmt.Sprintf(" %-9s", "TOKENS")
	}
	return header
}

func stateColWidth(width int) int {
	if width < 100 {
		return 16
	}
	if width < 120 {
		return 20
	}
	return 28
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

	name := pv.Name
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
	switch {
	case isActiveStatus(pv.Status):
		style = activeStyle
	case pv.Status == "complete":
		style = completedStyle
	case pv.Status == "blocked":
		style = blockedStyle
	case pv.Status == "disputed":
		style = disputedStyle
	}

	// Narrow terminal: just icon, name, state
	if width < 80 {
		row := fmt.Sprintf("%s%s %-*s  %s", cursor, icon, nameW, name, pv.Status)
		if selected {
			return selectedStyle.Render(style.Render(row))
		}
		return style.Render(row)
	}

	// State column: use Status
	state := pv.Status
	switch pv.Status {
	case "pending", "complete", "blocked", "deferred":
		state = "—"
	}

	// Iter column
	var iter string
	if pv.Iteration > 0 || pv.MaxIteration > 0 {
		iter = fmt.Sprintf("%d/%d", pv.Iteration, pv.MaxIteration)
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

	stateW := stateColWidth(width)
	if len(state) > stateW {
		state = state[:stateW-1] + "~"
	}
	row := fmt.Sprintf("%s%s %-*s  %-*s %-7s %-9s", cursor, icon, nameW, name, stateW, state, iter, tests)

	// Tokens column (wide terminal)
	if width >= 90 {
		total := pv.InputTokens + pv.OutputTokens
		if total > 0 {
			row += fmt.Sprintf(" %-9s", formatTokens(total))
		} else {
			row += fmt.Sprintf(" %-9s", "—")
		}
	}

	// Activity line: dim second line shown only for active phases on wide terminals
	if width >= 80 && pv.Activity != "" && isActiveStatus(pv.Status) {
		activityLine := fmt.Sprintf("      ↳ %s", pv.Activity)
		if len(activityLine) > width-2 {
			activityLine = activityLine[:width-5] + "..."
		}
		if selected {
			return selectedStyle.Render(style.Render(row)) + "\n" + selectedStyle.Render(dimStyle.Render(activityLine))
		}
		return style.Render(row) + "\n" + dimStyle.Render(activityLine)
	}

	if selected {
		return selectedStyle.Render(style.Render(row))
	}
	return style.Render(row)
}

// renderDetailPanel renders a full-screen detail view for the selected phase.
func (m Model) renderDetailPanel() string {
	phases := m.firstPlan().Phases
	if m.selectedIdx >= len(phases) {
		return ""
	}
	pv := phases[m.selectedIdx]

	var lines []string

	// Header
	header := fmt.Sprintf("  Phase: %s", pv.Name)
	lines = append(lines, headerStyle.Render(header))
	lines = append(lines, dimStyle.Render("  esc: back  ↑/↓: scroll  r: refresh"))
	lines = append(lines, "")

	// Status row
	lines = append(lines, fmt.Sprintf("  Status: %s", pv.Status))

	// Iter row
	lines = append(lines, fmt.Sprintf("  Iter:   %s", formatIter(pv)))

	// Tests row
	if pv.TestsTotal > 0 {
		lines = append(lines, fmt.Sprintf("  Tests:  %s", formatTests(pv)))
	}

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

	// Activity
	if pv.Activity != "" {
		actLine := fmt.Sprintf("  Activity: %s", pv.Activity)
		if pv.ActivityUpdatedAt != "" {
			if t, err := time.Parse(time.RFC3339, pv.ActivityUpdatedAt); err == nil {
				actLine += dimStyle.Render(fmt.Sprintf("  (at %s)", t.Format("15:04:05")))
			}
		}
		lines = append(lines, actLine)
	} else if isActiveStatus(pv.Status) {
		lines = append(lines, dimStyle.Render("  Activity: —"))
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
		return fmt.Sprintf("%d/%d", pv.Iteration, pv.MaxIteration)
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
