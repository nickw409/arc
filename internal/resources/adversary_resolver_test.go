package resources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAdversaryWorkflowPermissionStopsSearch verifies that a permission error
// on projectDir's file does NOT fall through to homeDir or embedded FS.
// Spec: "Any other error: return nil, err"
func TestAdversaryWorkflowPermissionStopsSearch(t *testing.T) {
	tmp := t.TempDir()
	projPath := filepath.Join(tmp, "proj", ".arc", "workflows", "secret.yaml")
	homePath := filepath.Join(tmp, "home", ".arc", "workflows", "secret.yaml")
	writeFile(t, projPath, "proj secret\n")
	writeFile(t, homePath, "home secret\n")
	if err := os.Chmod(projPath, 0000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(projPath, 0644) })

	r := NewResolver(filepath.Join(tmp, "proj"), filepath.Join(tmp, "home"))
	got, err := r.WorkflowBytes("secret")
	// Must error — must NOT return homeDir version
	if err == nil {
		t.Errorf("expected permission error, got content: %q", string(got))
	}
	if got != nil {
		t.Errorf("expected nil bytes on error, got %q", string(got))
	}
}

// TestAdversaryBlockPermissionStopsSearch mirrors the above for BlockBytes.
func TestAdversaryBlockPermissionStopsSearch(t *testing.T) {
	tmp := t.TempDir()
	projPath := filepath.Join(tmp, "proj", ".arc", "blocks", "secret.yaml")
	homePath := filepath.Join(tmp, "home", ".arc", "blocks", "secret.yaml")
	writeFile(t, projPath, "proj secret\n")
	writeFile(t, homePath, "home secret\n")
	if err := os.Chmod(projPath, 0000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(projPath, 0644) })

	r := NewResolver(filepath.Join(tmp, "proj"), filepath.Join(tmp, "home"))
	got, err := r.BlockBytes("secret")
	if err == nil {
		t.Errorf("expected permission error, got content: %q", string(got))
	}
	if got != nil {
		t.Errorf("expected nil bytes on error, got %q", string(got))
	}
}

// TestAdversaryWorkflowPermissionDoesNotFallThroughToEmbedded verifies that
// a permission error on a disk file does not fall through to the embedded FS
// for a name that exists in embedded (e.g. "feature").
func TestAdversaryWorkflowPermissionDoesNotFallThroughToEmbedded(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "proj", ".arc", "workflows", "feature.yaml")
	writeFile(t, path, "project feature override\n")
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0644) })

	r := NewResolver(filepath.Join(tmp, "proj"), "")
	got, err := r.WorkflowBytes("feature")
	// Must error — must NOT silently return embedded "feature"
	if err == nil {
		t.Errorf("expected permission error, got content: %q (should not have fallen through to embedded)", string(got))
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v bytes", len(got))
	}
}

// TestAdversaryBlockPermissionDoesNotFallThroughToEmbedded same as above for blocks.
func TestAdversaryBlockPermissionDoesNotFallThroughToEmbedded(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "proj", ".arc", "blocks", "adversary.yaml")
	writeFile(t, path, "project adversary override\n")
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0644) })

	r := NewResolver(filepath.Join(tmp, "proj"), "")
	got, err := r.BlockBytes("adversary")
	if err == nil {
		t.Errorf("expected permission error, got content: %q", string(got))
	}
	if got != nil {
		t.Errorf("expected nil bytes, got %v bytes", len(got))
	}
}

// TestAdversaryListWorkflowsDedupAcrossDirs verifies that a name present in
// both projectDir and homeDir appears exactly once in ListWorkflows output.
func TestAdversaryListWorkflowsDedupAcrossDirs(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "shared.yaml"), "proj\n")
	writeFile(t, filepath.Join(tmp, "home", ".arc", "workflows", "shared.yaml"), "home\n")
	r := NewResolver(filepath.Join(tmp, "proj"), filepath.Join(tmp, "home"))
	names := r.ListWorkflows()
	count := 0
	for _, n := range names {
		if n == "shared" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'shared' exactly once across two dirs, got %d times in %v", count, names)
	}
}

// TestAdversaryListBlocksDedupAcrossDirs same as above for blocks.
func TestAdversaryListBlocksDedupAcrossDirs(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "blocks", "shared.yaml"), "proj\n")
	writeFile(t, filepath.Join(tmp, "home", ".arc", "blocks", "shared.yaml"), "home\n")
	r := NewResolver(filepath.Join(tmp, "proj"), filepath.Join(tmp, "home"))
	names := r.ListBlocks()
	count := 0
	for _, n := range names {
		if n == "shared" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'shared' exactly once across two dirs, got %d times in %v", count, names)
	}
}

// TestAdversaryListWorkflowsIgnoresNonYaml verifies that non-.yaml files in
// the disk workflows dir do not appear in ListWorkflows output.
func TestAdversaryListWorkflowsIgnoresNonYaml(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "legit.yaml"), "name: legit\n")
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "notes.txt"), "some notes\n")
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "readme.md"), "# readme\n")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListWorkflows()
	for _, n := range names {
		if n == "notes" || n == "notes.txt" {
			t.Errorf("non-.yaml file 'notes.txt' should not appear in list, got name: %q", n)
		}
		if n == "readme" || n == "readme.md" {
			t.Errorf("non-.yaml file 'readme.md' should not appear in list, got name: %q", n)
		}
	}
	// Verify legit.yaml IS included
	found := false
	for _, n := range names {
		if n == "legit" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'legit' in %v", names)
	}
}

// TestAdversaryListBlocksIgnoresNonYaml same for blocks.
func TestAdversaryListBlocksIgnoresNonYaml(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "blocks", "real-block.yaml"), "name: real\n")
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "blocks", "notes.txt"), "notes\n")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListBlocks()
	for _, n := range names {
		if strings.Contains(n, "notes") {
			t.Errorf("non-.yaml file should not appear in list, got name: %q", n)
		}
	}
	found := false
	for _, n := range names {
		if n == "real-block" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'real-block' in %v", names)
	}
}

// TestAdversaryListWorkflowsDiskNamesBeforeEmbedded verifies that disk names
// appear before embedded names even if the disk name is alphabetically last.
// Spec: "Disk names from all search dirs appear first"
func TestAdversaryListWorkflowsDiskNamesBeforeEmbedded(t *testing.T) {
	tmp := t.TempDir()
	// "zzz-workflow" sorts after all embedded workflow names alphabetically
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "zzz-workflow.yaml"), "")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListWorkflows()

	zzIdx := -1
	// Find the smallest index of any embedded name
	embeddedSet := make(map[string]bool)
	for _, n := range ListWorkflows() {
		embeddedSet[n] = true
	}
	firstEmbeddedIdx := len(names)
	for i, n := range names {
		if n == "zzz-workflow" {
			zzIdx = i
		}
		if embeddedSet[n] && i < firstEmbeddedIdx {
			firstEmbeddedIdx = i
		}
	}
	if zzIdx == -1 {
		t.Fatal("zzz-workflow not found in list")
	}
	if firstEmbeddedIdx == len(names) {
		t.Skip("no embedded workflows found to compare against")
	}
	if zzIdx > firstEmbeddedIdx {
		t.Errorf("disk name 'zzz-workflow' at idx %d should come before first embedded name at idx %d; full list: %v",
			zzIdx, firstEmbeddedIdx, names)
	}
}

// TestAdversaryListBlocksDiskNamesBeforeEmbedded same for blocks.
func TestAdversaryListBlocksDiskNamesBeforeEmbedded(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "blocks", "zzz-block.yaml"), "")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListBlocks()

	zzIdx := -1
	embeddedSet := make(map[string]bool)
	for _, n := range ListBlocks() {
		embeddedSet[n] = true
	}
	firstEmbeddedIdx := len(names)
	for i, n := range names {
		if n == "zzz-block" {
			zzIdx = i
		}
		if embeddedSet[n] && i < firstEmbeddedIdx {
			firstEmbeddedIdx = i
		}
	}
	if zzIdx == -1 {
		t.Fatal("zzz-block not found in list")
	}
	if firstEmbeddedIdx == len(names) {
		t.Skip("no embedded blocks found to compare against")
	}
	if zzIdx > firstEmbeddedIdx {
		t.Errorf("disk name 'zzz-block' at idx %d should come before first embedded name at idx %d; full list: %v",
			zzIdx, firstEmbeddedIdx, names)
	}
}

// TestAdversaryWorkflowBothDirsHaveFileProjectWins verifies projectDir wins
// when both projectDir and homeDir contain the same workflow file.
func TestAdversaryWorkflowBothDirsHaveFileProjectWins(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "my-flow.yaml"), "project version\n")
	writeFile(t, filepath.Join(tmp, "home", ".arc", "workflows", "my-flow.yaml"), "home version\n")
	r := NewResolver(filepath.Join(tmp, "proj"), filepath.Join(tmp, "home"))
	got, err := r.WorkflowBytes("my-flow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(got), "project version") {
		t.Errorf("expected project version, got %q", string(got))
	}
}

// TestAdversaryBlockBothDirsHaveFileProjectWins same for blocks.
func TestAdversaryBlockBothDirsHaveFileProjectWins(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "blocks", "my-block.yaml"), "project version\n")
	writeFile(t, filepath.Join(tmp, "home", ".arc", "blocks", "my-block.yaml"), "home version\n")
	r := NewResolver(filepath.Join(tmp, "proj"), filepath.Join(tmp, "home"))
	got, err := r.BlockBytes("my-block")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(got), "project version") {
		t.Errorf("expected project version, got %q", string(got))
	}
}

// TestAdversaryWorkflowEmptyNameNotFound verifies that empty name returns an
// error (not-found after falling through all dirs and embedded FS).
func TestAdversaryWorkflowEmptyNameNotFound(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.WorkflowBytes("")
	// "" passes validation but no embedded file named ".yaml" exists
	if err == nil {
		t.Errorf("expected error for empty name, got content: %q", string(got))
	}
}

// TestAdversaryBlockEmptyNameNotFound same for blocks.
func TestAdversaryBlockEmptyNameNotFound(t *testing.T) {
	r := NewResolver("", "")
	got, err := r.BlockBytes("")
	if err == nil {
		t.Errorf("expected error for empty name, got content: %q", string(got))
	}
}

// TestAdversaryListWorkflowsEmptyResolverReturnsEmbedded verifies that an
// empty resolver (no search dirs) still returns the embedded workflow names.
func TestAdversaryListWorkflowsEmptyResolverReturnsEmbedded(t *testing.T) {
	r := NewResolver("", "")
	names := r.ListWorkflows()
	if len(names) == 0 {
		t.Fatal("expected non-empty list from empty resolver")
	}
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

// TestAdversaryListBlocksEmptyResolverReturnsEmbedded same for blocks.
func TestAdversaryListBlocksEmptyResolverReturnsEmbedded(t *testing.T) {
	r := NewResolver("", "")
	names := r.ListBlocks()
	if len(names) == 0 {
		t.Fatal("expected non-empty list from empty resolver")
	}
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

// TestAdversaryNameWithEmbeddedDotDot verifies that "a..b" (containing .. as
// a substring but not a standalone path component) is still rejected.
func TestAdversaryNameWithEmbeddedDotDot(t *testing.T) {
	r := NewResolver("", "")
	_, err := r.WorkflowBytes("a..b")
	if err == nil {
		t.Error("expected error for name 'a..b', got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid resource name") {
		t.Errorf("expected 'invalid resource name' in error, got %q", err.Error())
	}
}

// TestAdversaryBlockNameWithEmbeddedDotDot same for blocks.
func TestAdversaryBlockNameWithEmbeddedDotDot(t *testing.T) {
	r := NewResolver("", "")
	_, err := r.BlockBytes("a..b")
	if err == nil {
		t.Error("expected error for name 'a..b', got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid resource name") {
		t.Errorf("expected 'invalid resource name' in error, got %q", err.Error())
	}
}

// TestAdversaryListWorkflowsHomeDirFilesIncluded verifies that workflows in
// homeDir appear in ListWorkflows when projectDir has no files.
func TestAdversaryListWorkflowsHomeDirFilesIncluded(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "home", ".arc", "workflows", "home-only.yaml"), "")
	r := NewResolver("", filepath.Join(tmp, "home"))
	names := r.ListWorkflows()
	found := false
	for _, n := range names {
		if n == "home-only" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'home-only' in list, got %v", names)
	}
}

// TestAdversaryListBlocksHomeDirFilesIncluded same for blocks.
func TestAdversaryListBlocksHomeDirFilesIncluded(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "home", ".arc", "blocks", "home-block.yaml"), "")
	r := NewResolver("", filepath.Join(tmp, "home"))
	names := r.ListBlocks()
	found := false
	for _, n := range names {
		if n == "home-block" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'home-block' in list, got %v", names)
	}
}

// TestAdversaryNewResolverSkipsEmptyProjectDir verifies that NewResolver with
// empty projectDir only has one search dir (homeDir).
func TestAdversaryNewResolverSkipsEmptyProjectDir(t *testing.T) {
	tmp := t.TempDir()
	r := NewResolver("", filepath.Join(tmp, "home"))
	if len(r.searchDirs) != 1 {
		t.Errorf("expected 1 searchDir when projectDir is empty, got %d: %v", len(r.searchDirs), r.searchDirs)
	}
	expected := filepath.Join(tmp, "home", ".arc")
	if len(r.searchDirs) == 1 && r.searchDirs[0] != expected {
		t.Errorf("expected searchDir %q, got %q", expected, r.searchDirs[0])
	}
}

// TestAdversaryNewResolverSkipsEmptyHomeDir verifies that NewResolver with
// empty homeDir only has one search dir (projectDir).
func TestAdversaryNewResolverSkipsEmptyHomeDir(t *testing.T) {
	tmp := t.TempDir()
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	if len(r.searchDirs) != 1 {
		t.Errorf("expected 1 searchDir when homeDir is empty, got %d: %v", len(r.searchDirs), r.searchDirs)
	}
	expected := filepath.Join(tmp, "proj", ".arc")
	if len(r.searchDirs) == 1 && r.searchDirs[0] != expected {
		t.Errorf("expected searchDir %q, got %q", expected, r.searchDirs[0])
	}
}

// TestAdversaryNewResolverBothEmpty verifies that NewResolver("", "") has zero
// search dirs (falls through entirely to embedded FS).
func TestAdversaryNewResolverBothEmpty(t *testing.T) {
	r := NewResolver("", "")
	if len(r.searchDirs) != 0 {
		t.Errorf("expected 0 searchDirs when both args are empty, got %d: %v", len(r.searchDirs), r.searchDirs)
	}
}

// TestAdversaryListWorkflowsNoDuplicatesInFullList verifies there are no
// duplicate entries in the full list returned by ListWorkflows with a disk dir.
func TestAdversaryListWorkflowsNoDuplicatesInFullList(t *testing.T) {
	tmp := t.TempDir()
	// Add a custom disk workflow that doesn't exist in embedded FS
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "workflows", "zzz-unique.yaml"), "")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListWorkflows()
	seen := make(map[string]int)
	for _, n := range names {
		seen[n]++
	}
	for n, c := range seen {
		if c > 1 {
			t.Errorf("duplicate %q appears %d times in ListWorkflows: %v", n, c, names)
		}
	}
}

// TestAdversaryListBlocksNoDuplicatesInFullList same for blocks.
func TestAdversaryListBlocksNoDuplicatesInFullList(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "proj", ".arc", "blocks", "zzz-unique.yaml"), "")
	r := NewResolver(filepath.Join(tmp, "proj"), "")
	names := r.ListBlocks()
	seen := make(map[string]int)
	for _, n := range names {
		seen[n]++
	}
	for n, c := range seen {
		if c > 1 {
			t.Errorf("duplicate %q appears %d times in ListBlocks: %v", n, c, names)
		}
	}
}
