package workflow

import (
	"fmt"
	"testing"

	"github.com/nwiley/arc/internal/resources"
)

// TestAdversaryLoadBytesWithNilBlockLoaderPipeline verifies that calling
// LoadBytesWithBlockLoader with a nil blockLoader on a pipeline-format workflow
// does not panic. The function should return an error instead.
//
// This test will FAIL if the implementation panics on nil function call inside
// loadBlockDef when blockLoader is nil.
func TestAdversaryLoadBytesWithNilBlockLoaderPipeline(t *testing.T) {
	data := []byte(`name: test-pipeline
version: 1
pipeline:
  - block: adversary
terminal_states: [complete, blocked]
`)

	// We expect either an error or no panic. A panic is a bug.
	var panicVal interface{}
	var resultErr error

	func() {
		defer func() {
			panicVal = recover()
		}()
		_, resultErr = LoadBytesWithBlockLoader(data, nil)
	}()

	if panicVal != nil {
		t.Fatalf("LoadBytesWithBlockLoader panicked with nil blockLoader: %v — should return an error instead of panicking", panicVal)
	}

	// If it doesn't panic, it should return an error (not silently succeed with nil loader)
	if resultErr == nil {
		t.Fatal("LoadBytesWithBlockLoader with nil blockLoader should return an error, got nil")
	}
}

// TestAdversaryLoadBytesWithNilBlockLoaderStateMachine verifies that calling
// LoadBytesWithBlockLoader with a nil blockLoader on a state-machine workflow
// (no pipeline: key) does NOT panic and succeeds, because blockLoader is
// unused for state-machine workflows.
func TestAdversaryLoadBytesWithNilBlockLoaderStateMachine(t *testing.T) {
	// Use an inline state-machine workflow (no pipeline: key) so blockLoader is unused.
	data := []byte(`name: sm-workflow
version: 1
description: State-machine workflow
entry_state: impl
terminal_states: [complete, blocked]
states:
  - name: impl
    description: Implement
    prompt: prompts/feature/impl.md
    next: complete
  - name: complete
    description: Done
    prompt: prompts/common/complete.md
  - name: blocked
    description: Blocked
    prompt: prompts/common/blocked.md
`)

	// nil blockLoader is OK for state-machine workflows because blockLoader is unused
	wf, err := LoadBytesWithBlockLoader(data, nil)
	if err != nil {
		t.Fatalf("LoadBytesWithBlockLoader with nil blockLoader on state-machine workflow failed: %v", err)
	}
	if wf == nil {
		t.Fatal("expected non-nil workflow, got nil")
	}
}

// TestAdversaryLoadBytesWithBlockLoaderNilReturn verifies that a blockLoader
// returning (nil, nil) causes an error (not a panic), because block.LoadBlock
// should fail on nil/empty data.
func TestAdversaryLoadBytesWithBlockLoaderNilReturn(t *testing.T) {
	data := []byte(`name: test-pipeline
version: 1
pipeline:
  - block: some-block
terminal_states: [complete, blocked]
`)

	// Loader returns (nil, nil) - both zero values. The function must not panic.
	var panicVal interface{}
	var resultErr error

	func() {
		defer func() {
			panicVal = recover()
		}()
		_, resultErr = LoadBytesWithBlockLoader(data, func(name string) ([]byte, error) {
			return nil, nil // both zero values
		})
	}()

	if panicVal != nil {
		t.Fatalf("LoadBytesWithBlockLoader panicked when blockLoader returns (nil, nil): %v", panicVal)
	}

	// block.LoadBlock(nil) should return an error (block has no name)
	if resultErr == nil {
		t.Fatal("expected error when blockLoader returns (nil, nil), got nil — block.LoadBlock should reject nil/empty data")
	}
}

// TestAdversaryLoadBytesWithBlockLoaderEmptyBytesReturn verifies that a
// blockLoader returning empty bytes causes an appropriate error.
func TestAdversaryLoadBytesWithBlockLoaderEmptyBytesReturn(t *testing.T) {
	data := []byte(`name: test-pipeline
version: 1
pipeline:
  - block: empty-block
terminal_states: [complete, blocked]
`)

	_, err := LoadBytesWithBlockLoader(data, func(name string) ([]byte, error) {
		return []byte{}, nil // empty bytes (not nil, but empty)
	})

	// block.LoadBlock([]byte{}) should fail — empty YAML has no name
	if err == nil {
		t.Fatal("expected error when blockLoader returns empty bytes, got nil")
	}
}

// TestAdversaryLoadBytesWithBlockLoaderCalledForEachUniqueBlock verifies that
// the blockLoader is called exactly once per unique block name, not multiple
// times for the same block.
func TestAdversaryLoadBytesWithBlockLoaderCalledForEachUniqueBlock(t *testing.T) {
	// Pipeline with the same block name used twice
	data := []byte(`name: test-pipeline
version: 1
pipeline:
  - block: adversary
  - block: adversary
terminal_states: [complete, blocked]
`)

	callCount := 0
	adversaryBytes, err := resources.BlockBytes("adversary")
	if err != nil {
		t.Fatalf("failed to load embedded adversary block: %v", err)
	}

	_, err = LoadBytesWithBlockLoader(data, func(name string) ([]byte, error) {
		if name == "adversary" {
			callCount++
			return adversaryBytes, nil
		}
		return nil, fmt.Errorf("unexpected block: %s", name)
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be called exactly once (deduplication in loadComposed)
	if callCount != 1 {
		t.Errorf("blockLoader called %d times for 'adversary', expected exactly 1 (deduplication)", callCount)
	}
}

// TestAdversaryLoadBytesWithBlockLoaderParallelBlocksUseLoader verifies that
// blocks referenced in parallel steps also use the custom loader.
func TestAdversaryLoadBytesWithBlockLoaderParallelBlocksUseLoader(t *testing.T) {
	adversaryBytes, err := resources.BlockBytes("adversary")
	if err != nil {
		t.Fatalf("failed to load embedded adversary block: %v", err)
	}
	actBytes, err := resources.BlockBytes("act")
	if err != nil {
		t.Fatalf("failed to load embedded act block: %v", err)
	}

	loaderCalled := make(map[string]bool)

	data := []byte(`name: parallel-pipeline
version: 1
pipeline:
  - parallel:
      blocks:
        - block: adversary
        - block: act
      strategy: all
terminal_states: [complete, blocked]
`)

	_, _ = LoadBytesWithBlockLoader(data, func(name string) ([]byte, error) {
		loaderCalled[name] = true
		switch name {
		case "adversary":
			return adversaryBytes, nil
		case "act":
			return actBytes, nil
		default:
			return nil, fmt.Errorf("unexpected block: %s", name)
		}
	})

	// Both parallel blocks should have triggered the loader
	if !loaderCalled["adversary"] {
		t.Error("custom loader was NOT called for 'adversary' in parallel block")
	}
	if !loaderCalled["act"] {
		t.Error("custom loader was NOT called for 'act' in parallel block")
	}
}

// TestAdversaryLoadBytesRejectsEmptyData verifies LoadBytes rejects empty data.
// (Backward-compat check: LoadBytes should still reject empty input.)
func TestAdversaryLoadBytesRejectsEmptyData(t *testing.T) {
	_, err := LoadBytes([]byte{})
	if err == nil {
		t.Fatal("expected error for empty data, got nil")
	}
}
