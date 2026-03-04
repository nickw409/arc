package prompt

import (
	"testing"
)

// TestAdversary_BareUndefinedParamRendersEmpty verifies the spec requirement:
//
//	"Render a template containing {{params.nonexistent}} with
//	 TemplateContext{Params: map[string]string{\"other\": \"value\"}}.
//	 Expected: undefined param renders as empty string (Go template default behavior)."
//
// The existing TestAdversaryPromptRendersUndefinedParam test dodges this by using
// {{#if params.nonexistent}} (conditional guard) instead of bare {{params.nonexistent}}.
// This test verifies the actual spec requirement: bare access to an undefined param
// should render as empty string, not error.
func TestAdversary_BareUndefinedParamRendersEmpty(t *testing.T) {
	result, err := RenderString("Value={{params.nonexistent}}End", TemplateContext{
		Params: map[string]string{"other": "value"},
	})
	if err != nil {
		t.Fatalf("spec says undefined param should render as empty string, but got error: %v", err)
	}
	if result != "Value=End" {
		t.Fatalf("expected undefined param to render as empty, got %q", result)
	}
}

// TestAdversary_BareUndefinedParamWithNilParamsMap verifies that bare
// {{params.nonexistent}} doesn't panic or error when Params is nil.
func TestAdversary_BareUndefinedParamWithNilParamsMap(t *testing.T) {
	result, err := RenderString("Value={{params.nonexistent}}End", TemplateContext{
		Params: nil,
	})
	if err != nil {
		t.Fatalf("bare undefined param with nil Params should render as empty string, but got error: %v", err)
	}
	if result != "Value=End" {
		t.Fatalf("expected undefined param to render as empty, got %q", result)
	}
}
