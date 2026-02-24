package arc

import "time"

// PhaseState is the runtime state of a single phase, serialized to state.json.
type PhaseState struct {
	Phase               string          `json:"phase"`
	Plan                string          `json:"plan"`
	WorkflowType        string          `json:"workflow_type"`
	PhaseStatus         string          `json:"phase_status"`
	CurrentState        string          `json:"current_state"`
	Iteration           Iteration       `json:"iteration"`
	Chunks              Chunks          `json:"chunks"`
	Blocked             BlockedInfo     `json:"blocked"`
	Packages            []string        `json:"packages"`
	TestsPassing        int             `json:"tests_passing"`
	TestsTotal          int             `json:"tests_total"`
	StuckIterations     int             `json:"stuck_iterations"`
	HangCount           int             `json:"hang_count"`
	Disputes            []Dispute       `json:"disputes"`
	LastClearedDisputes []Dispute       `json:"last_cleared_disputes"`
	LastReviewedIter    int             `json:"last_reviewed_iteration"`
	LastQAReviewedIter  int             `json:"last_qa_reviewed_iteration"`
	VerdictsHistory     []VerdictEntry  `json:"verdicts_history"`
	LastVerdict         string          `json:"last_verdict"`
	TestFiles           []string        `json:"test_files"`
	ParallelExecution   *ParallelExec   `json:"parallel_execution"`
	InterventionRequest *Intervention   `json:"intervention_request"`
	ExecutedEscalations []string        `json:"executed_escalations"`
	RollbackCount       int             `json:"rollback_count"`
	GlobalIterations    int             `json:"global_iterations"`
	StateIterations     map[string]int  `json:"state_iterations,omitempty"`
	LastCommit          string          `json:"last_commit,omitempty"`
	ModelOverride       string          `json:"model_override,omitempty"`
	SplitInto           []string        `json:"split_into,omitempty"`
	DeferredReason      string          `json:"deferred_reason,omitempty"`
	DeferredAt          string          `json:"deferred_at,omitempty"`
	ParentPhase         string          `json:"parent_phase,omitempty"`
	Notes               string          `json:"notes,omitempty"`
	CompletedAt         string          `json:"completed_at,omitempty"`
	BlockedReason       string          `json:"blocked_reason,omitempty"`
	BlockedAt           string          `json:"blocked_at,omitempty"`
	Usage               Usage           `json:"usage,omitempty"`
}

type Iteration struct {
	Current int `json:"current"`
	Max     int `json:"max"`
}

type Chunks struct {
	Total     int           `json:"total"`
	Completed []ChunkResult `json:"completed"`
	Current   *ChunkCurrent `json:"current"`
	Remaining []int         `json:"remaining"`
}

type ChunkResult struct {
	ID     int    `json:"id"`
	Commit string `json:"commit"`
}

type ChunkCurrent struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type BlockedInfo struct {
	IsBlocked bool    `json:"is_blocked"`
	Reason    *string `json:"reason"`
}

type Dispute struct {
	TestName   string  `json:"test_name"`
	Reason     string  `json:"reason"`
	Resolution *string `json:"resolution"`
}

type VerdictEntry struct {
	Iteration int    `json:"iteration"`
	State     string `json:"state"`
	Verdict   string `json:"verdict"`
	Timestamp string `json:"timestamp"`
}

type ParallelExec struct {
	ResultsDir string                  `json:"results_dir"`
	Branches   map[string]BranchStatus `json:"branches"`
	Verdict    string                  `json:"verdict,omitempty"`
	StartedAt  string                  `json:"started_at"`
	FinishedAt string                  `json:"finished_at,omitempty"`
}

type BranchStatus struct {
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code,omitempty"`
}

type Intervention struct {
	Reason      string   `json:"reason"`
	RequestedAt string   `json:"requested_at"`
	Options     []string `json:"options,omitempty"`
}

// NewPhaseState creates a PhaseState with all slices/maps initialized (never nil).
func NewPhaseState(plan, phase, workflowType string) *PhaseState {
	return &PhaseState{
		Phase:               phase,
		Plan:                plan,
		WorkflowType:        workflowType,
		PhaseStatus:         "pending",
		Iteration:           Iteration{Current: 0, Max: 25},
		Chunks:              Chunks{Completed: []ChunkResult{}, Remaining: []int{}},
		Blocked:             BlockedInfo{},
		Packages:            []string{},
		Disputes:            []Dispute{},
		LastClearedDisputes: []Dispute{},
		VerdictsHistory:     []VerdictEntry{},
		TestFiles:           []string{},
		ExecutedEscalations: []string{},
	}
}

// PlanMeta is the metadata for a plan, serialized to plan.json.
type PlanMeta struct {
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
	WorkflowType string              `json:"workflow_type"`
	SplitPhases  map[string][]string `json:"split_phases,omitempty"`
	ArchivedAt   string              `json:"archived_at,omitempty"`
}

// NewPlanMeta creates a PlanMeta with all maps/slices initialized.
func NewPlanMeta(name, workflowType string, phases []string) *PlanMeta {
	phaseOrder := make(map[string]int, len(phases))
	dependencies := make(map[string][]string)
	for i, p := range phases {
		phaseOrder[p] = i + 1
		if i > 0 {
			dependencies[p] = []string{phases[i-1]}
		}
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
