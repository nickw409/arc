package review

import "context"

// Adversary defines a single adversarial reviewer.
type Adversary struct {
	Name        string
	PromptPath  string
	PassVerdict string
	Required    bool
}

// DefaultAdversaries returns the 5 standard adversarial reviewers.
func DefaultAdversaries() []Adversary {
	panic("not implemented")
}

// RunAdversary spawns a single adversary agent and extracts its verdict.
func RunAdversary(ctx context.Context, adv Adversary, planDir string, phaseName string) (*AdversaryResult, error) {
	panic("not implemented")
}
