package arc

import "time"

// PhaseState is the runtime state of a single phase, serialized to state.json.
type PhaseState struct {
	SchemaVersion       int             `json:"schema_version"`
	Phase               string          `json:"phase"`
	Plan                string          `json:"plan"`
	WorkflowType        string          `json:"workflow_type"`
	PhaseStatus         string          `json:"phase_status"`
	Iteration           Iteration       `json:"iteration"`
	Blocked             BlockedInfo     `json:"blocked"`
	Packages            []string        `json:"packages"`
	TestsPassing        int             `json:"tests_passing"`
	TestsTotal          int             `json:"tests_total"`
	TestFiles           []string        `json:"test_files"`
	LastCommit          string          `json:"last_commit,omitempty"`
	ModelOverride       string          `json:"model_override,omitempty"`
	SplitInto           []string        `json:"split_into,omitempty"`
	DeferredReason      string          `json:"deferred_reason,omitempty"`
	DeferredAt          string          `json:"deferred_at,omitempty"`
	ParentPhase         string          `json:"parent_phase,omitempty"`
	Notes               string          `json:"notes,omitempty"`
	CompletedAt         string          `json:"completed_at,omitempty"`
	BlockedReason       string              `json:"blocked_reason,omitempty"`
	BlockedAt           string              `json:"blocked_at,omitempty"`
	Usage               Usage               `json:"usage,omitempty"`
	Activity            string              `json:"activity,omitempty"`
	ActivityUpdatedAt   string              `json:"activity_updated_at,omitempty"`
	AdversaryRound      int                 `json:"adversary_round,omitempty"`
	AdversaryTests      map[string][]string `json:"adversary_tests,omitempty"` // round → test file paths
	Checksum            string              `json:"checksum,omitempty"`
	AgentPID            int                 `json:"agent_pid,omitempty"`
	WatchAttempts       int                 `json:"watch_attempts,omitempty"`
	AttemptLog          []AttemptSummary    `json:"attempt_log"`
}

type Iteration struct {
	Current int `json:"current"`
	Max     int `json:"max"`
}

type BlockedInfo struct {
	IsBlocked bool    `json:"is_blocked"`
	Reason    *string `json:"reason"`
}

// AttemptSummary records diagnostic info from a single agent attempt.
// Stored in PhaseState.AttemptLog so arc_manage show is self-sufficient for diagnosis.
type AttemptSummary struct {
	Attempt    int               `json:"attempt"`
	ErrorTier  string            `json:"error_tier"`
	Assertions []AssertionResult `json:"assertions,omitempty"`
	AgentTail  string            `json:"agent_tail,omitempty"`
	Timestamp  string            `json:"timestamp"`
}

// NewPhaseState creates a PhaseState with all slices/maps initialized (never nil).
func NewPhaseState(plan, phase, workflowType string) *PhaseState {
	return &PhaseState{
		Phase:        phase,
		Plan:         plan,
		WorkflowType: workflowType,
		PhaseStatus:  "pending",
		Iteration:    Iteration{Current: 0, Max: 25},
		Blocked:      BlockedInfo{},
		Packages:     []string{},
		TestFiles:    []string{},
		AttemptLog:   []AttemptSummary{},
	}
}

// PhaseReviewStatus tracks the review state for a single phase in plan.json.
type PhaseReviewStatus struct {
	Status     string `json:"status"`
	ReviewedAt string `json:"reviewed_at"`
	Hash       string `json:"hash"`
}

// PlanMeta is the metadata for a plan, serialized to plan.json.
type PlanMeta struct {
	SchemaVersion int                 `json:"schema_version,omitempty"`
	Name         string              `json:"name"`
	Created      string              `json:"created"`
	Status       string              `json:"status"`
	Phases       []string            `json:"phases"`
	PhaseOrder   map[string]int      `json:"phase_order"`
	Dependencies map[string][]string `json:"dependencies"`
	ReviewStatus     string              `json:"review_status"`
	ReviewedAt       string              `json:"reviewed_at,omitempty"`
	ReviewIterations int                 `json:"review_iterations,omitempty"`
	ReviewResults    map[string]string   `json:"review_results,omitempty"`
	PhaseReview      map[string]PhaseReviewStatus `json:"phase_review,omitempty"`
	WorkflowType string              `json:"workflow_type"`
	SplitPhases  map[string][]string `json:"split_phases,omitempty"`
	ArchivedAt   string              `json:"archived_at,omitempty"`
	AdversaryBugs map[string][]string `json:"adversary_bugs,omitempty"` // phase name → list of failing test names
}

// NewPlanMeta creates a PlanMeta with all maps/slices initialized.
func NewPlanMeta(name, workflowType string, phases []string) *PlanMeta {
	phaseOrder := make(map[string]int, len(phases))
	dependencies := make(map[string][]string)
	for i, p := range phases {
		phaseOrder[p] = i + 1
	}
	if phases == nil {
		phases = []string{}
	}
	return &PlanMeta{
		Name:         name,
		Created:      time.Now().UTC().Format(time.RFC3339),
		Status:       "active",
		Phases:       phases,
		PhaseOrder:   phaseOrder,
		Dependencies: dependencies,
		ReviewStatus: "unreviewed",
		WorkflowType: workflowType,
	}
}
