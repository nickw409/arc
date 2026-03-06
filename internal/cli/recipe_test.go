package cli

import (
	"testing"
)

func TestParseParams(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "nil input returns nil",
			input: nil,
			want:  nil,
		},
		{
			name:  "empty slice returns nil",
			input: []string{},
			want:  nil,
		},
		{
			name:  "single param",
			input: []string{"key=value"},
			want:  map[string]string{"key": "value"},
		},
		{
			name:  "multiple params",
			input: []string{"a=1", "b=2", "c=three"},
			want:  map[string]string{"a": "1", "b": "2", "c": "three"},
		},
		{
			name:  "value with equals sign",
			input: []string{"url=http://example.com/path=foo"},
			want:  map[string]string{"url": "http://example.com/path=foo"},
		},
		{
			name:  "empty value",
			input: []string{"key="},
			want:  map[string]string{"key": ""},
		},
		{
			name:    "missing equals sign",
			input:   []string{"badparam"},
			wantErr: true,
		},
		{
			name:    "missing equals in one of many",
			input:   []string{"good=value", "bad"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseParams(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseParams(%v) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseParams(%v) = %v, want %v", tt.input, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("param[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestRecipeCmdRegistered(t *testing.T) {
	cmd := NewRootCmd()

	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "recipe" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("recipe subcommand not registered on root command")
	}
}

func TestRecipeCmdHasSubcommands(t *testing.T) {
	recipeCmd := newRecipeCmd()

	wantSubs := []string{"list", "show"}
	subNames := make(map[string]bool)
	for _, c := range recipeCmd.Commands() {
		subNames[c.Name()] = true
	}

	for _, want := range wantSubs {
		if !subNames[want] {
			t.Errorf("recipe subcommand %q not found; got %v", want, subNames)
		}
	}

	// The default "recipe <name>" command is the root recipe command itself
	// (when given an arg), not a subcommand.
	if recipeCmd.Use != "recipe" {
		t.Errorf("recipe command Use = %q, want %q", recipeCmd.Use, "recipe")
	}
}
