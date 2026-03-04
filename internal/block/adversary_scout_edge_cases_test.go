package block_test

import (
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/resources"
)

// TestScoutPromptContainsScoutReportRef verifies the scout prompt references
// "scout-report.md" as required by the spec.
//
// Spec (TestScoutPromptExists section): "Expected: err == nil, len(data) > 0,
// content contains 'scout-report.md' and 'Edge Cases'"
//
// The existing TestScoutPromptExists only checks for "Edge Cases" but the spec
// also requires "scout-report.md" to be present in the prompt content.
func TestScoutPromptContainsScoutReportRef(t *testing.T) {
	data, err := resources.PromptBytes("blocks/scout.md")
	if err != nil {
		t.Fatalf("PromptBytes(blocks/scout.md) failed: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "scout-report.md") {
		t.Errorf("scout prompt does not contain 'scout-report.md'; spec requires it to reference the scout report output file")
	}
}
