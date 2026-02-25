package block

import (
	"testing"

	"github.com/nwiley/arc/internal/resources"
)

func TestQALoopBlockDeleted(t *testing.T) {
	_, err := resources.BlockBytes("qa-loop")
	if err == nil {
		t.Fatal("expected error loading deleted qa-loop block, got nil")
	}
}

func TestQALoopNotInBlockList(t *testing.T) {
	blocks := resources.ListBlocks()
	for _, name := range blocks {
		if name == "qa-loop" {
			t.Fatal("qa-loop should not appear in ListBlocks")
		}
	}
}
