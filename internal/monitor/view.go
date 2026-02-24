package monitor

import (
	"fmt"
	"strings"
)

// renderHeader renders plan name, overall progress, and last update time.
func (m Model) renderHeader() string {
	complete := 0
	for _, p := range m.phases {
		if p.Status == "complete" {
			complete++
		}
	}

	header := headerStyle.Render(fmt.Sprintf("Arc Monitor: %s", m.planName))
	progress := fmt.Sprintf("  Progress: %d/%d phases complete", complete, len(m.phases))
	if !m.lastUpdate.IsZero() {
		progress += fmt.Sprintf("  (updated %s)", m.lastUpdate.Format("15:04:05"))
	}

	return header + "\n" + progress
}

// renderPhaseTable renders a table of phase rows.
func (m Model) renderPhaseTable() string {
	if len(m.phases) == 0 {
		return "\n  No phases found.\n"
	}

	var b strings.Builder
	b.WriteString("\n")
	for _, pv := range m.phases {
		b.WriteString(renderPhaseRow(pv, m.width))
		b.WriteString("\n")
	}
	return b.String()
}

// renderPhaseRow renders one phase row.
func renderPhaseRow(pv PhaseView, width int) string {
	if width <= 0 {
		width = 80
	}

	name := pv.Name
	maxNameLen := 30
	if width < 60 {
		maxNameLen = 15
	}
	if len(name) > maxNameLen {
		name = name[:maxNameLen-1] + "~"
	}

	icon := pv.Icon
	var style = pendingStyle

	switch pv.Status {
	case "implementing", "qa", "qa_review", "impl_review":
		style = activeStyle
	case "adversary":
		style = activeStyle
	case "complete":
		style = completedStyle
	case "blocked":
		style = blockedStyle
	case "disputed":
		style = disputedStyle
	}

	// Build status detail
	var detail string
	switch pv.Status {
	case "complete":
		detail = "complete"
	case "pending":
		detail = "pending"
	case "blocked":
		detail = "blocked"
	default:
		detail = pv.Status
		if pv.Status == "adversary" && pv.AdversaryRound > 0 {
			detail = fmt.Sprintf("adversary (round %d)", pv.AdversaryRound)
		} else if pv.Iteration > 0 {
			detail += fmt.Sprintf(" (iter %d", pv.Iteration)
			if pv.TestsTotal > 0 && width >= 60 {
				detail += fmt.Sprintf(", %d/%d tests", pv.TestsPassing, pv.TestsTotal)
			}
			detail += ")"
		}
	}

	if pv.Disputes > 0 && width >= 60 {
		detail += fmt.Sprintf(" [%d disputes]", pv.Disputes)
	}

	row := fmt.Sprintf("  %s %-*s  %s", icon, maxNameLen, name, detail)
	return style.Render(row)
}

// renderFooter renders the footer with quit instructions.
func (m Model) renderFooter() string {
	return footerStyle.Render("  q: quit | Refreshing every 3s")
}
