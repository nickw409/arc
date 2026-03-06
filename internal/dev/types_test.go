package dev

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// --- TaskComplexity IsValid tests ---

func TestTaskComplexityIsValid_Simple(t *testing.T) {
	if !ComplexitySimple.IsValid() {
		t.Error("expected ComplexitySimple.IsValid() to be true")
	}
}

func TestTaskComplexityIsValid_Medium(t *testing.T) {
	if !ComplexityMedium.IsValid() {
		t.Error("expected ComplexityMedium.IsValid() to be true")
	}
}

func TestTaskComplexityIsValid_Complex(t *testing.T) {
	if !ComplexityComplex.IsValid() {
		t.Error("expected ComplexityComplex.IsValid() to be true")
	}
}

func TestTaskComplexityIsValid_Invalid(t *testing.T) {
	if TaskComplexity("unknown").IsValid() {
		t.Error("expected TaskComplexity(\"unknown\").IsValid() to be false")
	}
}

func TestTaskComplexityIsValid_Empty(t *testing.T) {
	if TaskComplexity("").IsValid() {
		t.Error("expected TaskComplexity(\"\").IsValid() to be false")
	}
}

func TestTaskComplexityIsValid_CaseSensitive_Upper(t *testing.T) {
	if TaskComplexity("SIMPLE").IsValid() {
		t.Error("expected TaskComplexity(\"SIMPLE\").IsValid() to be false")
	}
}

func TestTaskComplexityIsValid_CaseSensitive_Mixed(t *testing.T) {
	if TaskComplexity("Simple").IsValid() {
		t.Error("expected TaskComplexity(\"Simple\").IsValid() to be false")
	}
}

func TestTaskComplexityIsValid_Whitespace(t *testing.T) {
	if TaskComplexity(" simple ").IsValid() {
		t.Error("expected TaskComplexity(\" simple \").IsValid() to be false")
	}
}

// --- ValidComplexities tests ---

func TestValidComplexities(t *testing.T) {
	got := ValidComplexities()
	want := []TaskComplexity{ComplexitySimple, ComplexityMedium, ComplexityComplex}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ValidComplexities() = %v, want %v", got, want)
	}
}

func TestValidComplexities_FreshSlice(t *testing.T) {
	slice1 := ValidComplexities()
	slice1[0] = TaskComplexity("modified")
	slice2 := ValidComplexities()
	if slice2[0] != ComplexitySimple {
		t.Errorf("ValidComplexities() returned shared backing array: slice2[0] = %q, want %q", slice2[0], ComplexitySimple)
	}
}

// --- TaskComplexity UnmarshalJSON tests ---

func TestTaskComplexityUnmarshalJSON_Valid(t *testing.T) {
	var c TaskComplexity
	if err := json.Unmarshal([]byte(`"simple"`), &c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != ComplexitySimple {
		t.Errorf("got %q, want %q", c, ComplexitySimple)
	}
}

func TestTaskComplexityUnmarshalJSON_Null(t *testing.T) {
	var c TaskComplexity
	if err := json.Unmarshal([]byte("null"), &c); err == nil {
		t.Error("expected error for null, got nil")
	}
}

func TestTaskComplexityUnmarshalJSON_EmptyString(t *testing.T) {
	var c TaskComplexity
	if err := json.Unmarshal([]byte(`""`), &c); err == nil {
		t.Error("expected error for empty string, got nil")
	}
}

func TestTaskComplexityUnmarshalJSON_Number(t *testing.T) {
	var c TaskComplexity
	if err := json.Unmarshal([]byte("123"), &c); err == nil {
		t.Error("expected error for number, got nil")
	}
}

func TestTaskComplexityUnmarshalJSON_Boolean(t *testing.T) {
	var c TaskComplexity
	if err := json.Unmarshal([]byte("true"), &c); err == nil {
		t.Error("expected error for boolean, got nil")
	}
}

// --- DiscoveryResult JSON tests ---

func TestDiscoveryResultJSONRoundTrip(t *testing.T) {
	original := DiscoveryResult{
		TaskSummary:     "Add OAuth authentication",
		Complexity:      ComplexityComplex,
		Reasoning:       "Touches 8 files, needs auth architecture decisions",
		RelevantFiles:   []FileRef{{Path: "internal/auth/handler.go", Description: "existing auth"}},
		Requirements:    []string{"Support Google OAuth", "Refresh tokens"},
		Approach:        "Add OAuth middleware using existing auth package",
		WorkflowType:    "feature",
		SuggestedPhases: []PhaseSpec{{Name: "auth-types", Description: "Define OAuth types"}},
		Conventions:     []string{"errors wrapped with fmt.Errorf"},
		Risks:           []string{"breaking auth middleware"},
		Questions:       []string{},
		Clarifications:  []Clarification{},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var unmarshaled DiscoveryResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(original, unmarshaled) {
		t.Errorf("round-trip mismatch:\n  original:    %+v\n  unmarshaled: %+v", original, unmarshaled)
	}
}

func TestDiscoveryResultJSON_EmptySlices(t *testing.T) {
	dr := DiscoveryResult{
		TaskSummary:     "Fix typo",
		Complexity:      ComplexitySimple,
		RelevantFiles:   []FileRef{},
		Requirements:    []string{},
		SuggestedPhases: []PhaseSpec{},
	}
	data, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"relevant_files":[]`) {
		t.Errorf("expected relevant_files:[], got: %s", s)
	}
	if !strings.Contains(s, `"requirements":[]`) {
		t.Errorf("expected requirements:[], got: %s", s)
	}
	if !strings.Contains(s, `"suggested_phases":[]`) {
		t.Errorf("expected suggested_phases:[], got: %s", s)
	}
}

func TestDiscoveryResultJSON_NilSlices(t *testing.T) {
	dr := DiscoveryResult{
		TaskSummary:     "Fix typo",
		Complexity:      ComplexitySimple,
		RelevantFiles:   nil,
		Requirements:    nil,
		SuggestedPhases: nil,
	}
	data, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var unmarshaled DiscoveryResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if unmarshaled.RelevantFiles == nil {
		t.Error("expected RelevantFiles to be non-nil after unmarshal")
	}
	if unmarshaled.Requirements == nil {
		t.Error("expected Requirements to be non-nil after unmarshal")
	}
	if unmarshaled.SuggestedPhases == nil {
		t.Error("expected SuggestedPhases to be non-nil after unmarshal")
	}
}

func TestDiscoveryResultJSON_InvalidComplexity(t *testing.T) {
	data := []byte(`{"task_summary":"test","complexity":"invalid","reasoning":"test"}`)
	var dr DiscoveryResult
	if err := json.Unmarshal(data, &dr); err == nil {
		t.Error("expected error for invalid complexity, got nil")
	}
}

func TestDiscoveryResultJSON_MalformedJSON(t *testing.T) {
	var dr DiscoveryResult
	if err := json.Unmarshal([]byte("{invalid json"), &dr); err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}

func TestDiscoveryResultJSON_WrongTypeForComplexity(t *testing.T) {
	data := []byte(`{"task_summary":"test","complexity":123,"reasoning":"test"}`)
	var dr DiscoveryResult
	if err := json.Unmarshal(data, &dr); err == nil {
		t.Error("expected error when complexity is number, got nil")
	}
}

func TestDiscoveryResultJSON_MissingComplexity(t *testing.T) {
	data := []byte(`{"task_summary":"test","reasoning":"test"}`)
	var dr DiscoveryResult
	if err := json.Unmarshal(data, &dr); err == nil {
		t.Error("expected error for missing complexity, got nil")
	}
}

func TestDiscoveryResultJSON_SpecialCharacters(t *testing.T) {
	original := DiscoveryResult{
		TaskSummary: "Fix bug with \"quotes\" and\nnewlines",
		Complexity:  ComplexitySimple,
		Reasoning:   "Unicode: 日本語, emoji: 🚀",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var unmarshaled DiscoveryResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if original.TaskSummary != unmarshaled.TaskSummary {
		t.Errorf("TaskSummary mismatch: %q vs %q", original.TaskSummary, unmarshaled.TaskSummary)
	}
	if original.Reasoning != unmarshaled.Reasoning {
		t.Errorf("Reasoning mismatch: %q vs %q", original.Reasoning, unmarshaled.Reasoning)
	}
}

// --- ArchitectProposal JSON tests ---

func TestArchitectProposalJSONRoundTrip(t *testing.T) {
	original := ArchitectProposal{
		Name:            "pragmatic",
		Philosophy:      "Balance speed and quality",
		Architecture:    "Add OAuth as middleware layer",
		FilesCreate:     []FileRef{{Path: "internal/auth/oauth.go", Description: "OAuth handler"}},
		FilesModify:     []FileRef{{Path: "internal/auth/handler.go", Description: "Add OAuth route"}},
		Tradeoffs:       []string{"pro: reuses existing auth", "con: tighter coupling"},
		SuggestedPhases: []PhaseSpec{{Name: "oauth-core", Description: "Core OAuth logic"}},
		PlanContent:     map[string]string{"oauth-core": "# Phase: oauth-core\n\n## Objective\n..."},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var unmarshaled ArchitectProposal
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(original, unmarshaled) {
		t.Errorf("round-trip mismatch:\n  original:    %+v\n  unmarshaled: %+v", original, unmarshaled)
	}
}

func TestArchitectProposalJSON_EmptyPlanContent(t *testing.T) {
	ap := ArchitectProposal{PlanContent: map[string]string{}}
	data, err := json.Marshal(ap)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"plan_content":{}`) {
		t.Errorf("expected plan_content:{}, got: %s", s)
	}
}

func TestArchitectProposalJSON_NilPlanContent(t *testing.T) {
	ap := ArchitectProposal{
		Name:        "test",
		PlanContent: nil,
	}
	data, err := json.Marshal(ap)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"plan_content":{}`) {
		t.Errorf("expected plan_content:{}, got: %s", s)
	}
}

func TestArchitectProposalJSON_NilSlices(t *testing.T) {
	ap := ArchitectProposal{
		Name:            "test",
		FilesCreate:     nil,
		FilesModify:     nil,
		Tradeoffs:       nil,
		SuggestedPhases: nil,
		PlanContent:     nil,
	}
	data, err := json.Marshal(ap)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var unmarshaled ArchitectProposal
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if unmarshaled.FilesCreate == nil {
		t.Error("expected FilesCreate to be non-nil after unmarshal")
	}
	if unmarshaled.FilesModify == nil {
		t.Error("expected FilesModify to be non-nil after unmarshal")
	}
	if unmarshaled.Tradeoffs == nil {
		t.Error("expected Tradeoffs to be non-nil after unmarshal")
	}
	if unmarshaled.SuggestedPhases == nil {
		t.Error("expected SuggestedPhases to be non-nil after unmarshal")
	}
	if unmarshaled.PlanContent == nil {
		t.Error("expected PlanContent to be non-nil after unmarshal")
	}
}

func TestArchitectProposalJSON_MalformedJSON(t *testing.T) {
	var ap ArchitectProposal
	if err := json.Unmarshal([]byte("{invalid json"), &ap); err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}

func TestArchitectProposalJSON_SpecialCharacters(t *testing.T) {
	original := ArchitectProposal{
		Name:         "test",
		Philosophy:   "Use \"composition\" over\ninheritance",
		Architecture: "Unicode: 中文, symbols: <>&",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var unmarshaled ArchitectProposal
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if original.Philosophy != unmarshaled.Philosophy {
		t.Errorf("Philosophy mismatch: %q vs %q", original.Philosophy, unmarshaled.Philosophy)
	}
	if original.Architecture != unmarshaled.Architecture {
		t.Errorf("Architecture mismatch: %q vs %q", original.Architecture, unmarshaled.Architecture)
	}
}

// --- FileRef tests ---

func TestFileRef_EmptyStrings(t *testing.T) {
	original := FileRef{Path: "", Description: ""}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var unmarshaled FileRef
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(original, unmarshaled) {
		t.Errorf("round-trip mismatch: %+v vs %+v", original, unmarshaled)
	}
}

func TestFileRefJSONRoundTrip(t *testing.T) {
	original := FileRef{Path: "internal/auth/handler.go", Description: "existing auth handler"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var unmarshaled FileRef
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(original, unmarshaled) {
		t.Errorf("round-trip mismatch: %+v vs %+v", original, unmarshaled)
	}
}

func TestFileRefJSON_SpecialCharactersInPath(t *testing.T) {
	original := FileRef{Path: "path/with\"quotes\"/file.go", Description: "Test\nfile"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var unmarshaled FileRef
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(original, unmarshaled) {
		t.Errorf("round-trip mismatch: %+v vs %+v", original, unmarshaled)
	}
}

// --- PhaseSpec tests ---

func TestPhaseSpec_EmptyStrings(t *testing.T) {
	original := PhaseSpec{Name: "", Description: ""}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var unmarshaled PhaseSpec
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(original, unmarshaled) {
		t.Errorf("round-trip mismatch: %+v vs %+v", original, unmarshaled)
	}
}

func TestPhaseSpecJSONRoundTrip(t *testing.T) {
	original := PhaseSpec{Name: "auth-types", Description: "Define OAuth types and interfaces"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var unmarshaled PhaseSpec
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(original, unmarshaled) {
		t.Errorf("round-trip mismatch: %+v vs %+v", original, unmarshaled)
	}
}

// --- Direct workflow tests ---

