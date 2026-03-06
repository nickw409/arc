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

// renderHeader renders "Arc Monitor" or "Arc Monitor: plan-name" with aggregate totals.
func (m Model) renderHeader() string {
	// Aggregate totals across all plans
	var totalCompleted, totalPhases, totalRunning, totalFailed, totalIter int
	for _, p := range m.plans {
		totalPhases += len(p.Phases)
		totalCompleted += p.Meta.CompletedCount
		totalRunning += p.Meta.RunningCount
		totalFailed += p.Meta.FailedCount
		totalIter += p.Meta.TotalIterations
	}

	title := "Arc Monitor"
	if m.planFilter != "" {
		title = fmt.Sprintf("Arc Monitor: %s", m.planFilter)
	}

	line1 := fmt.Sprintf("%s  %d/%d phases", title, totalCompleted, totalPhases)
	if !m.lastUpdate.IsZero() {
		line1 += fmt.Sprintf("  updated %s", m.lastUpdate.Format("15:04:05"))
	}

	var parts []string
	if totalIter > 0 {
		parts = append(parts, fmt.Sprintf("%d iter", totalIter))
	}
	if totalRunning > 0 {
		parts = append(parts, fmt.Sprintf("%d active", totalRunning))
	}
	if totalFailed > 0 {
		parts = append(parts, blockedStyle.Render(fmt.Sprintf("%d blocked", totalFailed)))
	}

	header := headerStyle.Render(line1)
	if len(parts) > 0 {
		header += "\n  " + strings.Join(parts, " | ")
	}

	return header
}

// renderInterventionAlerts is a no-op; intervention is no longer tracked in PhaseView.
func (m Model) renderInterventionAlerts() string {
	return ""
}

// renderPhaseTable renders the column-header and all phase rows across all plans.
func (m Model) renderPhaseTable() string {
	if len(m.plans) == 0 {
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

	flatIdx := 0
	for _, plan := range m.plans {
		// Plan section header
		planHeader := fmt.Sprintf("  %s", plan.Name)
		if plan.WorkflowType != "" {
			planHeader += fmt.Sprintf("  [%s]", plan.WorkflowType)
		}
		planHeader += fmt.Sprintf("  %d/%d phases", plan.Meta.CompletedCount, len(plan.Phases))
		if plan.Meta.RunningCount > 0 {
			planHeader += fmt.Sprintf("  (%d active)", plan.Meta.RunningCount)
		}
		b.WriteString(dimStyle.Render(planHeader))
		b.WriteString("\n")

		for _, pv := range plan.Phases {
			selected := flatIdx == m.selectedIdx
			b.WriteString(renderPhaseRow(pv, width, selected))
			b.WriteString("\n")
			flatIdx++
		}
	}
	return b.String()
}

// renderColumnHeader renders the column header row (PHASE, ITER, TESTS, TOKENS).
func renderColumnHeader(width int) string {
	nameW := phaseNameWidth(width)

	if width < 80 {
		return fmt.Sprintf("    %s %-*s", "   ", nameW, "PHASE")
	}

	header := fmt.Sprintf("    %s %-*s  %-7s %-9s", "   ", nameW, "PHASE", "ITER", "TESTS")
	if width >= 90 {
		header += fmt.Sprintf(" %-9s", "TOKENS")
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

// renderPhaseRow renders one phase row with indented cursor (4 chars: "  > " or "    ").
func renderPhaseRow(pv PhaseView, width int, selected bool) string {
	if width <= 0 {
		width = 80
	}

	nameW := phaseNameWidth(width)

	name := pv.Name
	if len(name) > nameW {
		name = name[:nameW-1] + "~"
	}

	// Selection cursor: 2-space indent + 2-char cursor
	cursor := "    "
	if selected {
		cursor = "  > "
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

	// Narrow terminal: just icon, name, status
	if width < 80 {
		row := fmt.Sprintf("%s%s %-*s  %s", cursor, icon, nameW, name, pv.Status)
		if selected {
			return selectedStyle.Render(style.Render(row))
		}
		return style.Render(row)
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

	row := fmt.Sprintf("%s%s %-*s  %-7s %-9s", cursor, icon, nameW, name, iter, tests)

	// Tokens column (wide terminal)
	if width >= 90 {
		total := pv.InputTokens + pv.OutputTokens
		if total > 0 {
			row += fmt.Sprintf(" %-9s", formatTokens(total))
		} else {
			row += fmt.Sprintf(" %-9s", "—")
		}
	}

	// Activity line for running phases: NOT dimmed, shown immediately after the phase row
	if pv.Status == "running" && pv.Activity != "" {
		activityLine := fmt.Sprintf("          %s", pv.Activity)
		if len(activityLine) > width-2 {
			activityLine = activityLine[:width-5] + "..."
		}
		if selected {
			return selectedStyle.Render(style.Render(row)) + "\n" + selectedStyle.Render(activityLine)
		}
		return style.Render(row) + "\n" + activityLine
	}

	if selected {
		return selectedStyle.Render(style.Render(row))
	}
	return style.Render(row)
}

// renderDetailPanel renders a full-screen detail view for the selected phase.
func (m Model) renderDetailPanel() string {
	planIdx, phaseIdx := m.selectedPhase()
	if planIdx < 0 || phaseIdx < 0 {
		return ""
	}
	pv := m.plans[planIdx].Phases[phaseIdx]
	planName := m.plans[planIdx].Name

	var lines []string

	// Header
	header := fmt.Sprintf("  Phase: %s  (plan: %s)", pv.Name, planName)
	lines = append(lines, headerStyle.Render(header))
	lines = append(lines, dimStyle.Render("  esc: back  ↑/↓: scroll  r: refresh"))
	lines = append(lines, "")

	// Status row
	lines = append(lines, fmt.Sprintf("  Status:    %s", pv.Status))

	// Iter row
	lines = append(lines, fmt.Sprintf("  Iter:      %s", formatIter(pv)))

	// Tests row
	if pv.TestsTotal > 0 {
		lines = append(lines, fmt.Sprintf("  Tests:     %s", formatTests(pv)))
	}

	// Tokens row
	total := pv.InputTokens + pv.OutputTokens
	if total > 0 {
		lines = append(lines, fmt.Sprintf("  Tokens:    %s (in: %s, out: %s)",
			formatTokens(total), formatTokens(pv.InputTokens), formatTokens(pv.OutputTokens)))
	}

	// Commit row
	if pv.LastCommit != "" {
		commitLine := fmt.Sprintf("  Commit:    %s", pv.LastCommit)
		if pv.CompletedAt != "" {
			commitLine += fmt.Sprintf("  completed: %s", pv.CompletedAt)
		}
		lines = append(lines, commitLine)
	}

	// Activity
	if pv.Activity != "" {
		actLine := fmt.Sprintf("  Activity:  %s", pv.Activity)
		if pv.ActivityUpdatedAt != "" {
			if t, err := time.Parse(time.RFC3339, pv.ActivityUpdatedAt); err == nil {
				actLine += dimStyle.Render(fmt.Sprintf("  (at %s)", t.Format("15:04:05")))
			}
		}
		lines = append(lines, actLine)
	} else if isActiveStatus(pv.Status) {
		lines = append(lines, dimStyle.Render("  Activity:  —"))
	}

	// Notes
	if pv.Notes != "" {
		lines = append(lines, fmt.Sprintf("  Notes:     %s", pv.Notes))
	}

	// Blocked/deferred reason
	if pv.BlockedReason != "" {
		lines = append(lines, blockedStyle.Render(fmt.Sprintf("  Blocked:   %s", pv.BlockedReason)))
	}
	if pv.DeferredReason != "" {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  Deferred:  %s", pv.DeferredReason)))
	}

	// Adversary round (only if > 0)
	if pv.AdversaryRound > 0 {
		lines = append(lines, fmt.Sprintf("  Adversary: round %d", pv.AdversaryRound))
	}

	// Apply scroll offset
	visibleHeight := m.height - 2
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
