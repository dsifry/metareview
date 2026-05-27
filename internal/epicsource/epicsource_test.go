package epicsource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveBeadsEpicAndChildGraph(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".beads", "issues.jsonl"), strings.Join([]string{
		`{"id":"epic-1","title":"Parser epic","description":"Parse safely","children":["task-1"],"dependencies":["task-0"]}`,
		`{"id":"task-1","title":"Child one","parent":"epic-1","description":"Use JSON parser"}`,
		`{"id":"task-2","title":"Child two","epic":"epic-1","description":"Avoid eval"}`,
		`{"id":"task-3","title":"Child three","depends_on":["epic-1"],"description":"Integrate parser"}`,
		"",
	}, "\n"))

	epic, err := Resolve(root, "epic-1")
	if err != nil {
		t.Fatalf("resolve epic: %v", err)
	}
	if epic.Kind != "beads" || epic.ID != "epic-1" || epic.Title != "Parser epic" || !strings.Contains(epic.Body, "Parse safely") {
		t.Fatalf("unexpected epic source: %+v", epic)
	}
	wantChildren := []string{"task-0", "task-1", "task-2", "task-3"}
	if strings.Join(epic.ChildIDs, ",") != strings.Join(wantChildren, ",") {
		t.Fatalf("unexpected children: got %+v want %+v", epic.ChildIDs, wantChildren)
	}
}

func TestResolveMarkdownEpicAndRejectsOutsidePaths(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "epic.md"), "# Parser Epic\n\n- task-1\n- task-2\n")

	epic, err := Resolve(root, "docs/epic.md")
	if err != nil {
		t.Fatalf("resolve markdown epic: %v", err)
	}
	if epic.Kind != "markdown" || epic.ID != "docs/epic.md" || epic.Title != "Parser Epic" || !strings.Contains(epic.Body, "task-1") {
		t.Fatalf("unexpected markdown epic: %+v", epic)
	}

	outside := filepath.Join(t.TempDir(), "outside.md")
	mustWrite(t, outside, "# Outside\n")
	if _, err := Resolve(root, outside); err == nil || !strings.Contains(err.Error(), "outside repository root") {
		t.Fatalf("expected outside path rejection, got %v", err)
	}
}

func TestResolveAdvisoryEpic(t *testing.T) {
	root := t.TempDir()
	epic, err := Resolve(root, "epic-missing")
	if err != nil {
		t.Fatalf("resolve advisory: %v", err)
	}
	if epic.Kind != "advisory" || epic.ID != "epic-missing" || epic.Title != "epic-missing" {
		t.Fatalf("unexpected advisory epic: %+v", epic)
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
