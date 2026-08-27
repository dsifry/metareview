package reviewers

import "testing"

// A diff whose prose *describes* the lints must not trip them, while the same
// tokens in source must still block. Before this, a design doc discussing
// "eval/TODO" failed the gate — and metareview's own specs discuss exactly that.
func TestLintsIgnoreDocumentationProse(t *testing.T) {
	docsOnly := `diff --git a/docs/specs/design.md b/docs/specs/design.md
--- a/docs/specs/design.md
+++ b/docs/specs/design.md
@@ -0,0 +1,3 @@
+The deterministic gates block on eval/TODO/missing-test.
+The winning pair comes from the judge-eval (§17), not gpt-5.2.
+TODO markers in a design document are not code defects.
diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -0,0 +1,1 @@
+Any TODO in prose here is discussion, not a defect.
`
	git := GitContext{Diff: docsOnly, ChangedFiles: []string{"docs/specs/design.md", "README.md"}}
	lines := addedLines(git)
	for _, line := range lines {
		if evalPattern.MatchString(line) || todoPattern.MatchString(line) {
			t.Fatalf("documentation prose reached the lints: %q", line)
		}
	}

	findings := RunTaskDone(Context{
		Task: TaskContext{Type: "path", ID: "docs/tasks/t.md", Text: "task"},
		Git:  git,
	})
	for _, f := range findings {
		if f.Title == "Unsafe eval introduced" || f.Title == "TODO left in task-done diff" {
			t.Fatalf("lint fired on documentation: %s — %s", f.Title, f.Found)
		}
	}
}

func TestLintsStillSeeSourceChanges(t *testing.T) {
	source := `diff --git a/internal/app/run.go b/internal/app/run.go
--- a/internal/app/run.go
+++ b/internal/app/run.go
@@ -0,0 +1,2 @@
+	result := eval(userInput)
+	// TODO: handle the error path
`
	git := GitContext{Diff: source, ChangedFiles: []string{"internal/app/run.go"}}
	lines := addedLines(git)
	if firstMatching(lines, evalPattern) == "" {
		t.Fatal("an eval( in source must still reach the lints")
	}
	if firstMatching(lines, todoPattern) == "" {
		t.Fatal("a TODO in source must still reach the lints")
	}
}

// Untracked excerpts and staged/worktree diffs are scanned too, and the same
// source-vs-docs rule applies to them.
func TestLintScopeAppliesToEverySource(t *testing.T) {
	git := GitContext{
		StagedDiff: `diff --git a/docs/notes.md b/docs/notes.md
+++ b/docs/notes.md
+A staged TODO in prose.
`,
		WorkingTreeDiff: `diff --git a/lib/handler.js b/lib/handler.js
+++ b/lib/handler.js
+  const out = eval(payload);
`,
	}
	lines := addedLines(git)
	if firstMatching(lines, todoPattern) != "" {
		t.Fatal("a staged documentation TODO must not reach the lints")
	}
	if firstMatching(lines, evalPattern) == "" {
		t.Fatal("a working-tree source eval( must still reach the lints")
	}
}

// A .txt outside the docs tree is content, not prose: excluding it would widen
// the blind spot, and the sharded-review suite depends on this staying lintable.
func TestPlainTextOutsideDocsStaysLintable(t *testing.T) {
	git := GitContext{StagedDiff: `diff --git a/src/staged.txt b/src/staged.txt
+++ b/src/staged.txt
+value := eval(untrusted)
`}
	if firstMatching(addedLines(git), evalPattern) == "" {
		t.Fatal("src/staged.txt must still reach the lints")
	}
	docs := GitContext{StagedDiff: `diff --git a/docs/notes.txt b/docs/notes.txt
+++ b/docs/notes.txt
+we block on eval( calls
`}
	if firstMatching(addedLines(docs), evalPattern) != "" {
		t.Fatal("docs/notes.txt is documentation and must not reach the lints")
	}
}
