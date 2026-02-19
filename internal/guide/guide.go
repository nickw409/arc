package guide

import (
	"bytes"
	"fmt"

	"github.com/nwiley/arc/internal/resources"
)

var sections = []string{"setup", "plans", "workflows", "execution", "mistakes"}

// ValidSections returns the list of valid section names for the guide.
func ValidSections() []string {
	return sections
}

// Render returns the guide content. If section is empty, the full guide is
// returned with all section markers stripped. If section is non-empty, only
// that section's content is returned.
func Render(section string) ([]byte, error) {
	data, err := resources.GuideBytes("guide.md")
	if err != nil {
		return nil, fmt.Errorf("reading guide: %w", err)
	}

	if section == "" {
		return stripMarkers(data), nil
	}

	return extractSection(data, section)
}

// extractSection returns the content between <!-- section: name --> and
// <!-- /section: name --> markers.
func extractSection(data []byte, section string) ([]byte, error) {
	open := []byte(fmt.Sprintf("<!-- section: %s -->", section))
	close := []byte(fmt.Sprintf("<!-- /section: %s -->", section))

	start := bytes.Index(data, open)
	if start == -1 {
		return nil, fmt.Errorf("unknown section %q", section)
	}
	start += len(open)

	end := bytes.Index(data[start:], close)
	if end == -1 {
		return nil, fmt.Errorf("unclosed section %q", section)
	}

	content := data[start : start+end]
	return bytes.TrimSpace(content), nil
}

// stripMarkers removes all section marker lines from the data.
func stripMarkers(data []byte) []byte {
	var out []byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("<!-- section:")) || bytes.HasPrefix(trimmed, []byte("<!-- /section:")) {
			continue
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	// Trim trailing newline to match original behavior, then add exactly one.
	return bytes.TrimRight(out, "\n")
}
