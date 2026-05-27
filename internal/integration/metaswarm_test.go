package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestInspectMetaswarmRecordsConcreteSurfaces(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	metaswarm := filepath.Join(parent, "metaswarm")
	for _, path := range []string{
		"AGENTS.md",
		"CLAUDE.md",
		".claude/commands/pr-shepherd.md",
		".claude/commands/self-reflect.md",
		".claude/commands/start-task.md",
		"commands/pr-shepherd.md",
		"skills/pr-shepherd/SKILL.md",
		"skills/orchestrated-execution/SKILL.md",
		"skills/plan-review-gate/SKILL.md",
		"rubrics/adversarial-review-rubric.md",
	} {
		writeFile(t, filepath.Join(metaswarm, filepath.FromSlash(path)), "surface\n")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, parent)

	descriptor := InspectMetaswarm(root)

	if !descriptor.Present || descriptor.Path != "../metaswarm" {
		t.Fatalf("metaswarm sibling not detected: %+v", descriptor)
	}
	assertSurface(t, descriptor, "command", ".claude/commands/pr-shepherd.md")
	assertSurface(t, descriptor, "command", ".claude/commands/self-reflect.md")
	assertSurface(t, descriptor, "skill", "skills/orchestrated-execution/SKILL.md")
	assertSurface(t, descriptor, "rubric", "rubrics/adversarial-review-rubric.md")
	if !reflect.DeepEqual(before, snapshotTree(t, parent)) {
		t.Fatalf("metaswarm inspection mutated files")
	}
}

func TestMetaswarmContractDefinesPostMergeAndStrictBehavior(t *testing.T) {
	descriptor := InspectMetaswarm(t.TempDir())

	if descriptor.Contract.PostMergeLearning.Command != "metareview learn --post-merge <pr-number> --base <pre-merge-ref>" {
		t.Fatalf("unexpected post-merge command: %+v", descriptor.Contract.PostMergeLearning)
	}
	if descriptor.Contract.PostMergeLearning.StrictByDefault {
		t.Fatalf("post-merge learning should be advisory by default")
	}
	if descriptor.Contract.HookInstallation.InScope {
		t.Fatalf("automatic hook installation must stay out of scope")
	}
	if len(descriptor.Contract.Flow) != 4 {
		t.Fatalf("expected task, epic, PR, and post-merge flow entries: %+v", descriptor.Contract.Flow)
	}
	bytes, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatalf("descriptor should be JSON serializable: %v", err)
	}
	if len(bytes) == 0 {
		t.Fatalf("empty JSON descriptor")
	}
}

func assertSurface(t *testing.T, descriptor MetaswarmDescriptor, kind, path string) {
	t.Helper()
	for _, surface := range descriptor.Surfaces {
		if surface.Kind == kind && surface.Path == path {
			return
		}
	}
	t.Fatalf("missing %s surface %s in %+v", kind, path, descriptor.Surfaces)
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func snapshotTree(t *testing.T, root string) []string {
	t.Helper()
	paths := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}
