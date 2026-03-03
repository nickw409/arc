package resources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestResolverWorkflowDiskHit(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "my-flow.yaml"), "name: my-flow\n")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	got, err := r.WorkflowBytes("my-flow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "name: my-flow\n" {
		t.Errorf("got %q, want %q", got, "name: my-flow\n")
	}
}

func TestResolverWorkflowHomeDirFallback(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "home", ".arc", "workflows", "shared.yaml"), "name: shared\n")
	r := NewResolver("", filepath.Join(tmp, "home"))
	got, err := r.WorkflowBytes("shared")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "name: shared\n" {
		t.Errorf("got %q, want %q", got, "name: shared\n")
	}
}

func TestResolverWorkflowProjectTakesPrecedence(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "feature.yaml"), "name: override\n")
	r := NewResolver(filepath.Join(tmp, "proj"), filepath.Join(tmp, "home"))
	got, err := r.WorkflowBytes("feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(got), "override") {
		t.Errorf("expected 'override' in %q", got)
	}
}

func TestResolverWorkflowEmbeddedFallback(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.WorkflowBytes("feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Error("expected non-empty embedded feature workflow")
	}
}

func TestResolverWorkflowNotFound(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.WorkflowBytes("nonexistent-xyz-123")
	if err == nil {
		t.Error("expected error, got nil")
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v", got)
	}
}

func TestResolverBlockDiskHit(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "blocks", "my-block.yaml"), "name: my-block\n")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	got, err := r.BlockBytes("my-block")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "name: my-block\n" {
		t.Errorf("got %q, want %q", got, "name: my-block\n")
	}
}

func TestResolverBlockEmbeddedFallback(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.BlockBytes("adversary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Error("expected non-empty embedded adversary block")
	}
}

func TestResolverNameValidationDotDot(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.WorkflowBytes("../etc/passwd")
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid resource name") {
		t.Errorf("expected 'invalid resource name' in error, got %q", err.Error())
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v", got)
	}
}

func TestResolverNameValidationSlash(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.WorkflowBytes("sub/dir")
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid resource name") {
		t.Errorf("expected 'invalid resource name' in error, got %q", err.Error())
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v", got)
	}
}

func TestResolverBlockNameValidation(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.BlockBytes("../secret")
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid resource name") {
		t.Errorf("expected 'invalid resource name' in error, got %q", err.Error())
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v", got)
	}
}

func TestResolverBlockNameValidationSlash(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.BlockBytes("sub/dir")
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid resource name") {
		t.Errorf("expected 'invalid resource name' in error, got %q", err.Error())
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v", got)
	}
}

func TestResolverBlockNameValidationBackslash(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.BlockBytes("sub\\dir")
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid resource name") {
		t.Errorf("expected 'invalid resource name' in error, got %q", err.Error())
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v", got)
	}
}

func TestResolverWorkflowNameValidationBackslash(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.WorkflowBytes("sub\\dir")
	if err == nil {
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid resource name") {
		t.Errorf("expected 'invalid resource name' in error, got %q", err.Error())
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v", got)
	}
}

func TestResolverBlockHomeDirFallback(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "home", ".arc", "blocks", "shared.yaml"), "name: shared\n")
	r := NewResolver("", filepath.Join(tmp, "home"))
	got, err := r.BlockBytes("shared")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "name: shared\n" {
		t.Errorf("got %q, want %q", got, "name: shared\n")
	}
}

func TestResolverBlockProjectTakesPrecedence(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "blocks", "adversary.yaml"), "name: override\n")
	writeFile(t, filepath.Join(tmp, "home", ".arc", "blocks", "adversary.yaml"), "name: home\n")
	r := NewResolver(filepath.Join(tmp, "proj"), filepath.Join(tmp, "home"))
	got, err := r.BlockBytes("adversary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(got), "override") {
		t.Errorf("expected 'override' in %q", got)
	}
}

func TestResolverBlockNotFound(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.BlockBytes("nonexistent-block-xyz-123")
	if err == nil {
		t.Error("expected error, got nil")
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v", got)
	}
}

func TestResolverWorkflowZeroByteFile(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "empty.yaml"), "")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	got, err := r.WorkflowBytes("empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestResolverBlockZeroByteFile(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "blocks", "empty.yaml"), "")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	got, err := r.BlockBytes("empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestResolverWorkflowPermissionDenied(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "proj", ".arc", "workflows", "secret.yaml")
	writeFile(t, path, "secret content\n")
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0644) })

	r := NewResolver(filepath.Join(tmp, "proj"), "")
	got, err := r.WorkflowBytes("secret")
	if err == nil {
		t.Error("expected permission error, got nil")
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v", got)
	}
}

func TestResolverBlockPermissionDenied(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "proj", ".arc", "blocks", "secret.yaml")
	writeFile(t, path, "secret content\n")
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0644) })

	r := NewResolver(filepath.Join(tmp, "proj"), "")
	got, err := r.BlockBytes("secret")
	if err == nil {
		t.Error("expected permission error, got nil")
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v", got)
	}
}

func TestResolverListBlocksDedupe(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "blocks", "adversary.yaml"), "name: adversary\n")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListBlocks()
	count := 0
	for _, n := range names {
		if n == "adversary" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'adversary' exactly once, got %d times in %v", count, names)
	}
}

func TestResolverListWorkflowsReadDirError(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "proj", ".arc", "workflows")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(dir, 0000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListWorkflows()
	// Should return embedded workflows, not error
	if len(names) == 0 {
		t.Error("expected embedded workflows, got empty slice")
	}
	// Verify embedded workflows are present (feature is a known embedded workflow)
	found := false
	for _, n := range names {
		if n == "feature" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'feature' in embedded workflows, got %v", names)
	}
}

func TestResolverListBlocksReadDirError(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "proj", ".arc", "blocks")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(dir, 0000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListBlocks()
	// Should return embedded blocks, not error
	if len(names) == 0 {
		t.Error("expected embedded blocks, got empty slice")
	}
	// Verify embedded blocks are present
	found := false
	for _, n := range names {
		if n == "adversary" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'adversary' in embedded blocks, got %v", names)
	}
}

func TestResolverListWorkflowsMerged(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "custom.yaml"), "name: custom\n")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListWorkflows()

	hasCustom := false
	hasFeature := false
	customIdx := -1
	featureIdx := -1
	for i, n := range names {
		if n == "custom" {
			hasCustom = true
			customIdx = i
		}
		if n == "feature" {
			hasFeature = true
			featureIdx = i
		}
	}
	if !hasCustom {
		t.Errorf("expected 'custom' in %v", names)
	}
	if !hasFeature {
		t.Errorf("expected 'feature' in %v", names)
	}
	if customIdx >= featureIdx {
		t.Errorf("expected 'custom' (idx %d) before 'feature' (idx %d)", customIdx, featureIdx)
	}
	// Check no duplicates
	seen := make(map[string]int)
	for _, n := range names {
		seen[n]++
	}
	for n, c := range seen {
		if c > 1 {
			t.Errorf("duplicate %q appears %d times", n, c)
		}
	}
}

func TestResolverListWorkflowsDedupe(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "feature.yaml"), "name: override\n")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListWorkflows()
	count := 0
	for _, n := range names {
		if n == "feature" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'feature' exactly once, got %d times in %v", count, names)
	}
}

func TestResolverListBlocksMerged(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "blocks", "my-block.yaml"), "name: my-block\n")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListBlocks()

	hasMyBlock := false
	hasAdversary := false
	myBlockIdx := -1
	adversaryIdx := -1
	for i, n := range names {
		if n == "my-block" {
			hasMyBlock = true
			myBlockIdx = i
		}
		if n == "adversary" {
			hasAdversary = true
			adversaryIdx = i
		}
	}
	if !hasMyBlock {
		t.Errorf("expected 'my-block' in %v", names)
	}
	if !hasAdversary {
		t.Errorf("expected 'adversary' in %v", names)
	}
	if myBlockIdx >= adversaryIdx {
		t.Errorf("expected 'my-block' (idx %d) before 'adversary' (idx %d)", myBlockIdx, adversaryIdx)
	}
	// Check no duplicates
	seen := make(map[string]int)
	for _, n := range names {
		seen[n]++
	}
	for n, c := range seen {
		if c > 1 {
			t.Errorf("duplicate %q appears %d times", n, c)
		}
	}
}

func TestResolverNonExistentDirs(t *testing.T) {
	r := NewResolver("/nonexistent/path/abc", "/another/nonexistent/xyz")
	got, err := r.WorkflowBytes("feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Error("expected non-empty embedded feature workflow")
	}
}
