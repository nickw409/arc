package orchestrator

import (
	"fmt"
	"strings"

	"github.com/nwiley/arc/internal/arc"
)

// StuckDetector analyzes a sliding window of TurnEvents to detect when an
// agent is spinning without making progress. Detection is role-aware:
// impl phases look for lack of file edits, all roles look for repetition.
type StuckDetector struct {
	role      string // "impl", "review", "investigate", "audit"
	threshold int    // consecutive stuck turns before triggering
	window    []arc.TurnEvent
}

// StuckSignal describes why the detector thinks the agent is stuck.
type StuckSignal struct {
	Reason    string // human-readable explanation
	TurnCount int    // how many turns were in the stuck window
	Pattern   string // short pattern label for logging
}

// NewStuckDetector creates a detector with role-appropriate defaults.
func NewStuckDetector(role string, threshold int) *StuckDetector {
	if threshold <= 0 {
		switch role {
		case "impl":
			threshold = 8
		default:
			threshold = 12
		}
	}
	return &StuckDetector{
		role:      role,
		threshold: threshold,
	}
}

// Record adds a turn event and returns a StuckSignal if the agent appears stuck.
// Returns nil if no stuck pattern is detected.
func (d *StuckDetector) Record(ev arc.TurnEvent) *StuckSignal {
	d.window = append(d.window, ev)

	// Keep window bounded to 2x threshold to limit memory
	if len(d.window) > d.threshold*2 {
		d.window = d.window[len(d.window)-d.threshold*2:]
	}

	if len(d.window) < d.threshold {
		return nil
	}

	recent := d.window[len(d.window)-d.threshold:]

	// Check 1: Repeated identical Bash commands with no edits (any role)
	if sig := d.checkRepeatedCommands(recent); sig != nil {
		return sig
	}

	// Check 2: No file writes for N turns (impl only)
	if d.role == "impl" {
		if sig := d.checkNoEdits(recent); sig != nil {
			return sig
		}
	}

	// Check 3: Reading the same files repeatedly (any role)
	if sig := d.checkRepeatedReads(recent); sig != nil {
		return sig
	}

	return nil
}

// Reset clears the turn history (e.g., between retry attempts).
func (d *StuckDetector) Reset() {
	d.window = nil
}

// checkRepeatedCommands detects when the same Bash command runs repeatedly
// without any Edit/Write in between.
func (d *StuckDetector) checkRepeatedCommands(recent []arc.TurnEvent) *StuckSignal {
	// Find all Bash commands in the window
	var cmds []string
	hasEdit := false
	for _, ev := range recent {
		for _, tu := range ev.Tools {
			switch tu.Name {
			case "Edit", "MultiEdit", "Write":
				hasEdit = true
			case "Bash":
				if tu.Cmd != "" {
					cmds = append(cmds, tu.Cmd)
				}
			}
		}
	}

	if hasEdit || len(cmds) < 3 {
		return nil
	}

	// Check if >60% of commands are identical
	freq := make(map[string]int)
	for _, c := range cmds {
		freq[c]++
	}
	for cmd, count := range freq {
		if count >= len(cmds)*6/10 {
			short := cmd
			if len(short) > 50 {
				short = short[:47] + "..."
			}
			return &StuckSignal{
				Reason:    fmt.Sprintf("ran %q %d times in %d turns without editing any files", short, count, len(recent)),
				TurnCount: len(recent),
				Pattern:   "repeated_command",
			}
		}
	}
	return nil
}

// checkNoEdits detects when an impl agent goes N turns without any file modifications.
func (d *StuckDetector) checkNoEdits(recent []arc.TurnEvent) *StuckSignal {
	for _, ev := range recent {
		for _, tu := range ev.Tools {
			if tu.Name == "Edit" || tu.Name == "MultiEdit" || tu.Name == "Write" {
				return nil
			}
		}
	}
	return &StuckSignal{
		Reason:    fmt.Sprintf("no file edits in %d consecutive turns", len(recent)),
		TurnCount: len(recent),
		Pattern:   "no_edits",
	}
}

// checkRepeatedReads detects when the agent keeps reading the same files
// without making progress (works for all roles).
func (d *StuckDetector) checkRepeatedReads(recent []arc.TurnEvent) *StuckSignal {
	// Collect all read targets per turn
	readSets := make([]map[string]bool, 0, len(recent))
	for _, ev := range recent {
		reads := make(map[string]bool)
		for _, tu := range ev.Tools {
			if (tu.Name == "Read" || tu.Name == "View") && tu.File != "" {
				reads[tu.File] = true
			}
		}
		if len(reads) > 0 {
			readSets = append(readSets, reads)
		}
	}

	if len(readSets) < d.threshold*2/3 {
		return nil
	}

	// Count file frequency across all turns
	fileFreq := make(map[string]int)
	for _, s := range readSets {
		for f := range s {
			fileFreq[f]++
		}
	}

	// If a single file appears in >70% of read-turns, flag it
	for file, count := range fileFreq {
		if count >= len(readSets)*7/10 {
			return &StuckSignal{
				Reason:    fmt.Sprintf("read %q in %d of %d turns — appears to be going in circles", file, count, len(recent)),
				TurnCount: len(recent),
				Pattern:   "repeated_reads",
			}
		}
	}
	return nil
}

// FormatStuckNote produces a human-readable note for state.json when a phase is
// detected as stuck.
func FormatStuckNote(sig *StuckSignal, attempt int) string {
	return fmt.Sprintf("stuck (attempt %d): %s", attempt, sig.Reason)
}

// StuckGuidance produces a prompt addendum telling the agent what it was doing wrong.
func StuckGuidance(sig *StuckSignal) string {
	var sb strings.Builder
	sb.WriteString("## Stuck Detection\n\n")
	sb.WriteString("Your previous session was cancelled because you appeared stuck:\n\n")
	sb.WriteString(fmt.Sprintf("  %s\n\n", sig.Reason))

	switch sig.Pattern {
	case "repeated_command":
		sb.WriteString("Do NOT run the same command repeatedly hoping for a different result. ")
		sb.WriteString("If a test or build fails, read the error output carefully, edit the relevant files to fix the issue, then try again.\n")
	case "no_edits":
		sb.WriteString("You spent too long reading and running commands without making any changes. ")
		sb.WriteString("Commit to an approach and start editing files. If you're unsure, make a small focused change and test it.\n")
	case "repeated_reads":
		sb.WriteString("You kept reading the same files without making progress. ")
		sb.WriteString("You likely already have the information you need. Focus on making changes rather than re-reading.\n")
	}

	return sb.String()
}
