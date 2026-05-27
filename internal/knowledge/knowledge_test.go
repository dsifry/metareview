package knowledge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectReadsServiceInventoryAndSortedKnowledgeFacts(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "SERVICE_INVENTORY.md"), "# Service Inventory\n\nReview API service.\n")
	mustWrite(t, filepath.Join(root, ".beads", "knowledge", "z-last.jsonl"), "{\"id\":\"z\",\"fact\":\"last fact\"}\n")
	mustWrite(t, filepath.Join(root, ".beads", "knowledge", "a-first.jsonl"), "{\"id\":\"a\",\"fact\":\"first fact\"}\n")

	context, err := Collect(root)
	if err != nil {
		t.Fatalf("collect knowledge: %v", err)
	}
	if context.ServiceInventoryPath != "docs/SERVICE_INVENTORY.md" {
		t.Fatalf("unexpected service inventory path: %q", context.ServiceInventoryPath)
	}
	if context.ServiceInventory == "" {
		t.Fatal("expected service inventory content")
	}
	if len(context.Facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(context.Facts))
	}
	if context.Facts[0].Source != ".beads/knowledge/a-first.jsonl" || context.Facts[0].Text != "first fact" {
		t.Fatalf("facts not sorted or parsed: %+v", context.Facts)
	}
	if context.Facts[1].Source != ".beads/knowledge/z-last.jsonl" || context.Facts[1].Text != "last fact" {
		t.Fatalf("facts not sorted or parsed: %+v", context.Facts)
	}
}

func TestCollectRejectsKnowledgeSymlinkOutsideRepo(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "external.jsonl"), "{\"fact\":\"outside\"}\n")
	if err := os.MkdirAll(filepath.Join(root, ".beads", "knowledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "external.jsonl"), filepath.Join(root, ".beads", "knowledge", "external.jsonl")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, err := Collect(root); err == nil {
		t.Fatal("expected outside symlink error")
	}
}

func mustWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
