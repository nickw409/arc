package cli

import (
	"bytes"
	"sort"
	"testing"
)

func TestCobraRootHasAllSubcommands(t *testing.T) {
	cmd := NewRootCmd()

	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	sort.Strings(names)

	want := []string{"guide", "init", "iterate", "monitor", "plan", "review", "run", "status", "update", "validate"}
	sort.Strings(want)

	if len(names) < len(want) {
		t.Fatalf("got commands %v, want at least %v", names, want)
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	for _, w := range want {
		if !nameSet[w] {
			t.Fatalf("missing subcommand %q, got %v", w, names)
		}
	}
}

func TestCobraVersionFlag(t *testing.T) {
	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("expected version output, got empty string")
	}
	if !bytes.Contains([]byte(output), []byte(Version)) {
		t.Fatalf("output %q does not contain version %q", output, Version)
	}
}
