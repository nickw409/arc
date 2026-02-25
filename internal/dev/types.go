package dev

import (
	"encoding/json"
	"fmt"
)

// TaskComplexity represents how much orchestration a task needs.
type TaskComplexity string

const (
	ComplexitySimple  TaskComplexity = "simple"
	ComplexityMedium  TaskComplexity = "medium"
	ComplexityComplex TaskComplexity = "complex"
)

// ValidComplexities returns all valid TaskComplexity values.
func ValidComplexities() []TaskComplexity {
	return []TaskComplexity{ComplexitySimple, ComplexityMedium, ComplexityComplex}
}

// IsValid returns true if the complexity is a recognized value.
func (c TaskComplexity) IsValid() bool {
	switch c {
	case ComplexitySimple, ComplexityMedium, ComplexityComplex:
		return true
	}
	return false
}

// UnmarshalJSON implements json.Unmarshaler.
func (c *TaskComplexity) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("TaskComplexity must be a string: %w", err)
	}
	tc := TaskComplexity(s)
	if !tc.IsValid() {
		return fmt.Errorf("invalid TaskComplexity: %q", s)
	}
	*c = tc
	return nil
}

// FileRef is a reference to a file with a description of its relevance.
type FileRef struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

// PhaseSpec describes a suggested phase from discovery.
type PhaseSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Clarification holds a question posed to the user and their answer.
type Clarification struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// DiscoveryResult holds the structured output from a discovery agent.
type DiscoveryResult struct {
	TaskSummary     string              `json:"task_summary"`
	Complexity      TaskComplexity      `json:"complexity"`
	Reasoning       string              `json:"reasoning"`
	RelevantFiles   []FileRef           `json:"relevant_files"`
	Requirements    []string            `json:"requirements"`
	Approach        string              `json:"approach"`
	WorkflowType    string              `json:"workflow_type"`
	SuggestedPhases []PhaseSpec         `json:"suggested_phases"`
	Dependencies    map[string][]string `json:"dependencies,omitempty"`
	Conventions     []string            `json:"conventions,omitempty"`
	Risks           []string            `json:"risks,omitempty"`
	Questions       []string            `json:"questions,omitempty"`
	Clarifications  []Clarification     `json:"clarifications,omitempty"`
}

// MarshalJSON ensures nil slices are marshaled as [] instead of null.
func (dr DiscoveryResult) MarshalJSON() ([]byte, error) {
	if dr.RelevantFiles == nil {
		dr.RelevantFiles = []FileRef{}
	}
	if dr.Requirements == nil {
		dr.Requirements = []string{}
	}
	if dr.SuggestedPhases == nil {
		dr.SuggestedPhases = []PhaseSpec{}
	}
	if dr.Conventions == nil {
		dr.Conventions = []string{}
	}
	if dr.Risks == nil {
		dr.Risks = []string{}
	}
	if dr.Questions == nil {
		dr.Questions = []string{}
	}
	if dr.Clarifications == nil {
		dr.Clarifications = []Clarification{}
	}
	type Alias DiscoveryResult
	return json.Marshal(Alias(dr))
}

// UnmarshalJSON validates the complexity field and ensures slices are non-nil after unmarshaling.
func (dr *DiscoveryResult) UnmarshalJSON(data []byte) error {
	type Alias DiscoveryResult
	var raw Alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if !raw.Complexity.IsValid() {
		return fmt.Errorf("invalid complexity: %q", raw.Complexity)
	}
	*dr = DiscoveryResult(raw)
	if dr.RelevantFiles == nil {
		dr.RelevantFiles = []FileRef{}
	}
	if dr.Requirements == nil {
		dr.Requirements = []string{}
	}
	if dr.SuggestedPhases == nil {
		dr.SuggestedPhases = []PhaseSpec{}
	}
	if dr.Conventions == nil {
		dr.Conventions = []string{}
	}
	if dr.Risks == nil {
		dr.Risks = []string{}
	}
	if dr.Questions == nil {
		dr.Questions = []string{}
	}
	if dr.Clarifications == nil {
		dr.Clarifications = []Clarification{}
	}
	return nil
}

// ArchitectProposal holds a single architect agent's design.
type ArchitectProposal struct {
	Name            string            `json:"name"`
	Philosophy      string            `json:"philosophy"`
	Architecture    string            `json:"architecture"`
	FilesCreate     []FileRef         `json:"files_create"`
	FilesModify     []FileRef         `json:"files_modify"`
	Tradeoffs       []string          `json:"tradeoffs"`
	SuggestedPhases []PhaseSpec       `json:"suggested_phases"`
	PlanContent     map[string]string `json:"plan_content"`
}

// MarshalJSON ensures nil slices are marshaled as [] and nil maps as {} instead of null.
func (ap ArchitectProposal) MarshalJSON() ([]byte, error) {
	if ap.FilesCreate == nil {
		ap.FilesCreate = []FileRef{}
	}
	if ap.FilesModify == nil {
		ap.FilesModify = []FileRef{}
	}
	if ap.Tradeoffs == nil {
		ap.Tradeoffs = []string{}
	}
	if ap.SuggestedPhases == nil {
		ap.SuggestedPhases = []PhaseSpec{}
	}
	if ap.PlanContent == nil {
		ap.PlanContent = map[string]string{}
	}
	type Alias ArchitectProposal
	return json.Marshal(Alias(ap))
}

// UnmarshalJSON ensures slices and maps are non-nil after unmarshaling.
func (ap *ArchitectProposal) UnmarshalJSON(data []byte) error {
	type Alias ArchitectProposal
	var raw Alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*ap = ArchitectProposal(raw)
	if ap.FilesCreate == nil {
		ap.FilesCreate = []FileRef{}
	}
	if ap.FilesModify == nil {
		ap.FilesModify = []FileRef{}
	}
	if ap.Tradeoffs == nil {
		ap.Tradeoffs = []string{}
	}
	if ap.SuggestedPhases == nil {
		ap.SuggestedPhases = []PhaseSpec{}
	}
	if ap.PlanContent == nil {
		ap.PlanContent = map[string]string{}
	}
	return nil
}
