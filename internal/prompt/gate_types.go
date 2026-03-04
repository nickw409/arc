package prompt

// ImplData is the template data for the implementation prompt.
type ImplData struct {
	Spec           string
	Files          []string
	Checkpoints    []CheckpointData
	Plan           string
	Phase          string
	TestCommand    string
	ProjectContext string
}

// CheckpointData describes a single checkpoint in an implementation phase.
type CheckpointData struct {
	Name        string
	Description string
	Test        string
}

// RetryData is the template data for the retry prompt.
type RetryData struct {
	Attempt     int
	MaxAttempts int
	GateOutput  string
	DiffSummary string
}

// PlannerData is the template data for the planner prompt.
type PlannerData struct {
	Description    string
	PlanName       string
	ProjectContext string
}

// OrchestratorData is the template data for the orchestrator agent prompt.
type OrchestratorData struct {
	AttemptCount int
	PhaseName    string
	SpecSummary  string
	Attempts     []AttemptData
	DiffSummary  string
}

// AttemptData describes a single failed attempt in the orchestrator prompt.
type AttemptData struct {
	Attempt           int
	GateOutput        string
	CheckpointsPassed int
	CheckpointsTotal  int
	DiffSummary       string
}

// StrategicData is the template data for the strategic intervention prompt.
type StrategicData struct {
	PhaseName   string
	SpecSummary string
	History     []AttemptData
}

// AdversaryData is the template data for the adversary prompt.
type AdversaryData struct {
	ChangedFiles   []string
	TestCommand    string
	ProjectContext string
}
