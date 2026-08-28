package reviewers

import (
	"strings"
	"testing"
)

// The two markers are assembled at run time and written as placeholders inside
// the fixtures below, so that this file does not trip the very lints it tests
// when metareview reviews the branch that adds it. sharded_test.go does the same.
var (
	evalCall = "eval" + "("
	todoWord = "TO" + "DO"
)

// fixture expands the placeholders in a diff literal into the real markers.
func fixture(diff string) string {
	diff = strings.ReplaceAll(diff, "@EVAL@", evalCall)
	return strings.ReplaceAll(diff, "@TODO@", todoWord)
}

// A diff whose prose *describes* the lints must not trip them, while the same
// markers in source must still block. Before this, a design document discussing
// them failed the gate — and metareview's own specs discuss exactly that.
func TestLintsIgnoreDocumentationProse(t *testing.T) {
	docsOnly := fixture(`diff --git a/docs/specs/design.md b/docs/specs/design.md
--- a/docs/specs/design.md
+++ b/docs/specs/design.md
@@ -0,0 +1,3 @@
+The deterministic gates block on @EVAL@/@TODO@/missing-test.
+The winning pair comes from the judge-eval (§17), not gpt-5.2.
+@TODO@ markers in a design document are not code defects.
diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -0,0 +1,1 @@
+Any @TODO@ in prose here is discussion, not a defect.
`)
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
		if f.Title == "Unsafe eval introduced" || f.Title == todoWord+" left in task-done diff" {
			t.Fatalf("lint fired on documentation: %s — %s", f.Title, f.Found)
		}
	}
}

func TestLintsStillSeeSourceChanges(t *testing.T) {
	source := fixture(`diff --git a/internal/app/run.go b/internal/app/run.go
--- a/internal/app/run.go
+++ b/internal/app/run.go
@@ -0,0 +1,2 @@
+	result := @EVAL@userInput)
+	// @TODO@: handle the error path
`)
	git := GitContext{Diff: source, ChangedFiles: []string{"internal/app/run.go"}}
	lines := addedLines(git)
	if firstMatching(lines, evalPattern) == "" {
		t.Fatal("an unsafe call in source must still reach the lints")
	}
	if firstMatching(lines, todoPattern) == "" {
		t.Fatal("a leftover marker in source must still reach the lints")
	}
}

// Untracked excerpts and staged/worktree diffs are scanned too, and the same
// source-vs-docs rule applies to them.
func TestLintScopeAppliesToEverySource(t *testing.T) {
	git := GitContext{
		StagedDiff: fixture(`diff --git a/docs/notes.md b/docs/notes.md
+++ b/docs/notes.md
+A staged @TODO@ in prose.
`),
		WorkingTreeDiff: fixture(`diff --git a/lib/handler.js b/lib/handler.js
+++ b/lib/handler.js
+  const out = @EVAL@payload);
`),
	}
	lines := addedLines(git)
	if firstMatching(lines, todoPattern) != "" {
		t.Fatal("a staged documentation marker must not reach the lints")
	}
	if firstMatching(lines, evalPattern) == "" {
		t.Fatal("a working-tree source call must still reach the lints")
	}
}

// A .txt outside the docs tree is content, not prose: excluding it would widen
// the blind spot, and the sharded-review suite depends on this staying lintable.
func TestPlainTextOutsideDocsStaysLintable(t *testing.T) {
	git := GitContext{StagedDiff: fixture(`diff --git a/src/staged.txt b/src/staged.txt
+++ b/src/staged.txt
+value := @EVAL@untrusted)
`)}
	if firstMatching(addedLines(git), evalPattern) == "" {
		t.Fatal("src/staged.txt must still reach the lints")
	}
	docs := GitContext{StagedDiff: fixture(`diff --git a/docs/notes.txt b/docs/notes.txt
+++ b/docs/notes.txt
+we block on @EVAL@ calls
`)}
	if firstMatching(addedLines(docs), evalPattern) != "" {
		t.Fatal("docs/notes.txt is documentation and must not reach the lints")
	}
}

// An added line of source that itself begins with "++" arrives in the diff as
// "+++<line>". Treating every "+++" prefix as metadata walked an unsafe call
// straight past the security lint.
func TestAddedLineBeginningWithPlusPlusIsNotMistakenForAHeader(t *testing.T) {
	diff := fixture(`diff --git a/src/app.js b/src/app.js
--- a/src/app.js
+++ b/src/app.js
@@ -1 +1,2 @@
+++@EVAL@userInput)
`)
	lines := addedLines(GitContext{BranchDiffFull: diff})
	if len(lines) != 1 || !strings.Contains(lines[0], "++"+evalCall) {
		t.Fatalf("the ++ line was dropped as a header: %#v", lines)
	}
}

// git C-quotes a path containing spaces; with the quotes left on, neither the
// docs/ prefix nor the .md suffix matched, and prose reached the lints.
func TestQuotedDocumentationPathIsStillRecognizedAsProse(t *testing.T) {
	diff := fixture(`diff --git "a/docs/design notes.md" "b/docs/design notes.md"
--- "a/docs/design notes.md"
+++ "b/docs/design notes.md"
@@ -1 +1,2 @@
+the gates block on @EVAL@) and @TODO@ markers
`)
	if lines := addedLines(GitContext{BranchDiffFull: diff}); len(lines) != 0 {
		t.Fatalf("quoted docs path was scanned as source: %#v", lines)
	}
}

func TestMarkdownExtensionMatchingIsCaseInsensitive(t *testing.T) {
	for _, path := range []string{"README.MD", "docs/NOTES.Markdown", "DOCS/plan.txt"} {
		if lintable(path) {
			t.Fatalf("%s should be treated as prose", path)
		}
	}
	if !lintable("src/app.md.go") {
		t.Fatal("a .go file merely containing .md must stay lintable")
	}
}

// Content must never choose its own lint scope. An added source line reading
// "++ b/docs/x.md; <call>" reaches the parser as "+++ b/docs/x.md; <call>", and
// honouring that as a header would point the scope at the docs tree and hide
// the rest of the file. A "+++ " line only names a file inside a file header,
// before the first hunk.
func TestHunkBodyCannotRedirectTheLintScope(t *testing.T) {
	diff := fixture(`diff --git a/src/app.js b/src/app.js
--- a/src/app.js
+++ b/src/app.js
@@ -1 +1,3 @@
+++ b/docs/decoy.md
+  const out = @EVAL@payload);
`)
	if firstMatching(addedLines(GitContext{BranchDiffFull: diff}), evalPattern) == "" {
		t.Fatal("a decoy header inside the hunk body hid an unsafe call from the lints")
	}
}

// The same trick through an untracked file. gitcontext.untrackedExcerpt prefixes
// every content line with "+", so a bare "--- " can only be a record header —
// this pins that contract from the consuming side.
func TestUntrackedContentCannotRedirectTheLintScope(t *testing.T) {
	excerpt := fixture(`--- src/dropped.js
+--- docs/decoy.md
+const out = @EVAL@payload);
`)
	if firstMatching(addedLines(GitContext{UntrackedExcerpts: excerpt}), evalPattern) == "" {
		t.Fatal("a decoy header in untracked content hid an unsafe call from the lints")
	}
}

// A leading or trailing space can be part of a real path. Trimming it turned
// " docs/x.js" — a file in a directory literally named " docs" — into something
// that looked like it lived under the docs tree, excluding it from the lints.
func TestWhitespaceInAPathIsNotTrimmedIntoTheDocsTree(t *testing.T) {
	excerpt := fixture(`--- ` + ` docs/check.js
+const out = @EVAL@payload);
`)
	if firstMatching(addedLines(GitContext{UntrackedExcerpts: excerpt}), evalPattern) == "" {
		t.Fatal("a path with a leading space was trimmed into the docs tree and skipped")
	}
	if !lintable(" docs/check.js") {
		t.Fatal(`" docs/check.js" is not under docs/ and must stay lintable`)
	}
	if lintable("docs/check.js") {
		t.Fatal("docs/check.js is documentation")
	}
}

// CRLF line endings must still not leave a stray CR on the path, or a genuine
// .md would miss the suffix check.
func TestCarriageReturnIsStrippedFromHeaderPaths(t *testing.T) {
	// README.md, deliberately not under docs/: the prefix check would catch a
	// docs/ path whether or not the CR was stripped, so only a path recognised
	// by its .md suffix actually exercises this.
	diff := fixture("diff --git a/README.md b/README.md\r\n+++ b/README.md\r\n@@ -1 +1 @@\r\n+a @TODO@ in prose\r\n")
	if lines := addedLines(GitContext{BranchDiffFull: diff}); len(lines) != 0 {
		t.Fatalf("CRLF header hid the .md suffix: %#v", lines)
	}
}

// The two remaining branches of the path decoders: an empty path (a header the
// parser could not read) stays lintable, because failing open here would hide
// real code, and a diff --git line with no recognisable post-image half yields
// nothing rather than a partial path.
func TestPathDecodersFailOpen(t *testing.T) {
	if !lintable("") {
		t.Fatal("an unreadable path must stay lintable rather than be skipped")
	}
	for _, header := range []string{
		"diff --git a/only-one-half",
		"diff --git",
		"diff --git a/x.md c/x.md",
	} {
		if got := diffHeaderPath(header); got != "" {
			t.Fatalf("%q yielded %q, want no path", header, got)
		}
	}
	// The unquoted " b/" form, which the quoted branch does not cover.
	if got := diffHeaderPath("diff --git a/src/app.go b/src/app.go"); got != "src/app.go" {
		t.Fatalf("plain header path: %q", got)
	}
}
