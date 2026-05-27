package tasksource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveBeadsMarkdownAndAdvisorySources(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, ".beads"))
	mustWrite(t, filepath.Join(root, ".beads", "issues.jsonl"), strings.Join([]string{
		`{"id":"MRV-1","title":"Review task","description":"Check the implementation","status":"open"}`,
		`{"id":"MRV-2","title":"Other task","description":"Ignore me"}`,
		"",
	}, "\n"))
	mustMkdir(t, filepath.Join(root, "docs"))
	mustWrite(t, filepath.Join(root, "docs", "task.md"), "# Markdown Task\n\nReview the plan.\n")

	beads, err := Resolve(root, "MRV-1")
	if err != nil {
		t.Fatalf("resolve beads: %v", err)
	}
	if beads.Kind != "beads" || beads.ID != "MRV-1" || beads.Title != "Review task" || !strings.Contains(beads.Body, "Check the implementation") {
		t.Fatalf("unexpected beads source: %+v", beads)
	}

	markdown, err := Resolve(root, "docs/task.md")
	if err != nil {
		t.Fatalf("resolve markdown: %v", err)
	}
	if markdown.Kind != "markdown" || markdown.ID != "docs/task.md" || markdown.Title != "Markdown Task" || !strings.Contains(markdown.Body, "Review the plan") {
		t.Fatalf("unexpected markdown source: %+v", markdown)
	}

	advisory, err := Resolve(root, "TASK-404")
	if err != nil {
		t.Fatalf("resolve advisory: %v", err)
	}
	if advisory.Kind != "advisory" || advisory.ID != "TASK-404" || advisory.Title != "TASK-404" {
		t.Fatalf("unexpected advisory source: %+v", advisory)
	}
}

func TestResolveBeadsUsesCommonBodyFields(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, ".beads"))
	mustWrite(t, filepath.Join(root, ".beads", "issues.jsonl"), `{"id":"MRV-3","title":"Content task","content":"Keep original intent","acceptance":"Acceptance detail"}`+"\n")

	source, err := Resolve(root, "MRV-3")
	if err != nil {
		t.Fatalf("resolve beads content task: %v", err)
	}
	if !strings.Contains(source.Body, "Keep original intent") || !strings.Contains(source.Body, "Acceptance detail") {
		t.Fatalf("expected common body fields in source body, got: %+v", source)
	}
}

func TestResolveRejectsOutsideRepoPaths(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	mustWrite(t, outside, "# Outside\n")

	for _, target := range []string{"../outside.md", outside} {
		if _, err := Resolve(root, target); err == nil || !strings.Contains(err.Error(), "outside repository root") {
			t.Fatalf("expected outside-repo error for %q, got %v", target, err)
		}
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
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
