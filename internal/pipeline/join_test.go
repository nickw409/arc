package pipeline

import (
	"strings"
	"testing"

	"github.com/nwiley/arc/internal/arc"
)

func TestJoinParallel(t *testing.T) {
	tests := []struct {
		name       string
		strategy   string
		results    map[string]int
		n          int
		want       string
		wantErr    bool
		wantErrMsg string // substring to match in error message
	}{
		// all strategy
		{
			name:     "all/all_succeed",
			strategy: "all",
			results:  map[string]int{"a": 0, "b": 0, "c": 0},
			want:     "all_complete",
		},
		{
			name:     "all/one_fails",
			strategy: "all",
			results:  map[string]int{"a": 0, "b": 1, "c": 0},
			want:     "any_failed",
		},
		{
			name:     "all/all_fail",
			strategy: "all",
			results:  map[string]int{"a": 1, "b": 2},
			want:     "any_failed",
		},
		{
			name:     "all/single_success",
			strategy: "all",
			results:  map[string]int{"a": 0},
			want:     "all_complete",
		},
		// any strategy
		{
			name:     "any/one_succeeds",
			strategy: "any",
			results:  map[string]int{"a": 1, "b": 0},
			want:     "first_complete",
		},
		{
			name:     "any/all_fail",
			strategy: "any",
			results:  map[string]int{"a": 1, "b": 2},
			want:     "all_failed",
		},
		{
			name:     "any/all_succeed",
			strategy: "any",
			results:  map[string]int{"a": 0, "b": 0},
			want:     "first_complete",
		},
		{
			name:     "any/single_fail",
			strategy: "any",
			results:  map[string]int{"a": 1},
			want:     "all_failed",
		},
		// n_of_m strategy
		{
			name:     "n_of_m/meets_threshold",
			strategy: "n_of_m",
			results:  map[string]int{"a": 0, "b": 0, "c": 1},
			n:        2,
			want:     "n_complete",
		},
		{
			name:     "n_of_m/below_threshold",
			strategy: "n_of_m",
			results:  map[string]int{"a": 0, "b": 1, "c": 1},
			n:        2,
			want:     "insufficient",
		},
		{
			name:     "n_of_m/exact_threshold",
			strategy: "n_of_m",
			results:  map[string]int{"a": 0, "b": 0},
			n:        2,
			want:     "n_complete",
		},
		{
			name:     "n_of_m/n_exceeds_total",
			strategy: "n_of_m",
			results:  map[string]int{"a": 0, "b": 0, "c": 0},
			n:        5,
			want:     "insufficient",
		},
		{
			name:       "n_of_m/n_zero",
			strategy:   "n_of_m",
			results:    map[string]int{"a": 1},
			n:          0,
			wantErr:    true,
			wantErrMsg: "n > 0",
		},
		{
			name:       "n_of_m/n_negative",
			strategy:   "n_of_m",
			results:    map[string]int{"a": 0},
			n:          -1,
			wantErr:    true,
			wantErrMsg: "n > 0",
		},
		// error cases
		{
			name:       "unknown_strategy",
			strategy:   "bogus",
			results:    map[string]int{"a": 0},
			wantErr:    true,
			wantErrMsg: "unknown strategy",
		},
		{
			name:       "empty_results",
			strategy:   "all",
			results:    map[string]int{},
			wantErr:    true,
			wantErrMsg: "no results",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := JoinParallel(tc.strategy, tc.results, tc.n)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got verdict %q", got)
				}
				if tc.wantErrMsg != "" && !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tc.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("JoinParallel(%q, %v, %d) = %q, want %q", tc.strategy, tc.results, tc.n, got, tc.want)
			}
		})
	}
}

// ── MergeVerdicts tests ──────────────────────────────────────────────────────

var bugsFoundNoBugsFound = []arc.Verdict{"bugs_found", "no_bugs_found"}

func TestMergeVerdictsAllBugsFound(t *testing.T) {
	v, err := MergeVerdicts("all", map[string]arc.Verdict{"a": "bugs_found", "b": "bugs_found"}, bugsFoundNoBugsFound)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "bugs_found" {
		t.Fatalf("got %q, want %q", v, "bugs_found")
	}
}

func TestMergeVerdictsAllNoBugsFound(t *testing.T) {
	v, err := MergeVerdicts("all", map[string]arc.Verdict{"a": "no_bugs_found", "b": "no_bugs_found"}, bugsFoundNoBugsFound)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "no_bugs_found" {
		t.Fatalf("got %q, want %q", v, "no_bugs_found")
	}
}

func TestMergeVerdictsMixedAllStrategy(t *testing.T) {
	v, err := MergeVerdicts("all", map[string]arc.Verdict{"a": "bugs_found", "b": "no_bugs_found"}, bugsFoundNoBugsFound)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// bugs_found < no_bugs_found alphabetically, so bugs_found wins
	if v != "bugs_found" {
		t.Fatalf("got %q, want %q", v, "bugs_found")
	}
}

func TestMergeVerdictsEmptyBranchVerdicts(t *testing.T) {
	_, err := MergeVerdicts("all", map[string]arc.Verdict{}, bugsFoundNoBugsFound)
	if err == nil {
		t.Fatal("expected error for empty branch verdicts")
	}
}

func TestMergeVerdictsInvalidStrategy(t *testing.T) {
	_, err := MergeVerdicts("unknown", map[string]arc.Verdict{"a": "bugs_found"}, bugsFoundNoBugsFound)
	if err == nil {
		t.Fatal("expected error for invalid strategy")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error = %q, want containing 'unsupported'", err.Error())
	}
}

func TestMergeVerdictsInvalidVerdict(t *testing.T) {
	_, err := MergeVerdicts("all", map[string]arc.Verdict{"a": "invalid_verdict", "b": "bugs_found"}, bugsFoundNoBugsFound)
	if err == nil {
		t.Fatal("expected error for invalid verdict")
	}
	if !strings.Contains(err.Error(), "invalid verdict") {
		t.Fatalf("error = %q, want containing 'invalid verdict'", err.Error())
	}
}

func TestMergeVerdictsAnyStrategy(t *testing.T) {
	// For "any" strategy, the last alphabetically wins when mixed
	v, err := MergeVerdicts("any", map[string]arc.Verdict{"a": "bugs_found", "b": "no_bugs_found"}, bugsFoundNoBugsFound)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// no_bugs_found > bugs_found alphabetically
	if v != "no_bugs_found" {
		t.Fatalf("got %q, want %q", v, "no_bugs_found")
	}
}

func TestMergeVerdictsAnyStrategyAllSame(t *testing.T) {
	v, err := MergeVerdicts("any", map[string]arc.Verdict{"a": "bugs_found", "b": "bugs_found"}, bugsFoundNoBugsFound)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "bugs_found" {
		t.Fatalf("got %q, want %q", v, "bugs_found")
	}
}

func TestMergeVerdictsSingleBranch(t *testing.T) {
	v, err := MergeVerdicts("all", map[string]arc.Verdict{"solo": "no_bugs_found"}, bugsFoundNoBugsFound)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "no_bugs_found" {
		t.Fatalf("got %q, want %q", v, "no_bugs_found")
	}
}
