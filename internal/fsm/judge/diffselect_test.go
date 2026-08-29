package judge

import (
	"strconv"
	"strings"
	"testing"
)

// multiLangDiff carries five languages plus a path with a space, so any accidental
// dependence on language shows up as one entry behaving differently from the others.
const multiLangDiff = `diff --git a/internal/app/server.go b/internal/app/server.go
index 111..222 100644
--- a/internal/app/server.go
+++ b/internal/app/server.go
@@ -10,3 +10,4 @@ func Serve() {
 ctx := context.Background()
+	go listen(ctx)
 return nil
@@ -200,2 +201,3 @@ func Close() {
+	_ = conn.Close()
 return
diff --git a/scripts/deploy.py b/scripts/deploy.py
index 333..444 100644
--- a/scripts/deploy.py
+++ b/scripts/deploy.py
@@ -5,2 +5,3 @@ def deploy():
+    subprocess.run(cmd, shell=True)
     return 0
diff --git a/config/ci.yaml b/config/ci.yaml
index 555..666 100644
--- a/config/ci.yaml
+++ b/config/ci.yaml
@@ -1,2 +1,3 @@
 jobs:
+  deploy: {runs-on: ubuntu-latest}
diff --git a/docs/guide.md b/docs/guide.md
index 777..888 100644
--- a/docs/guide.md
+++ b/docs/guide.md
@@ -40,2 +41,3 @@
 ## Setup
+Run the installer.
diff --git a/src/legacy code.rb b/src/legacy code.rb
index 999..aaa 100644
--- a/src/legacy code.rb	
+++ b/src/legacy code.rb	
@@ -7,2 +7,3 @@ def run
+  system(cmd)
 end
`

func TestSelectDiffIsLanguageAgnostic(t *testing.T) {
	// Every language is selected by the same diff-format rules; none may behave differently.
	for _, tc := range []struct{ path, want string }{
		{"internal/app/server.go", "go listen(ctx)"},
		{"scripts/deploy.py", "subprocess.run"},
		{"config/ci.yaml", "runs-on: ubuntu-latest"},
		{"docs/guide.md", "Run the installer."},
		{"src/legacy code.rb", "system(cmd)"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if !DiffHasFile(multiLangDiff, tc.path) {
				t.Fatalf("DiffHasFile(%q) = false", tc.path)
			}
			out, ok, hash := SelectDiff(multiLangDiff, tc.path, 0, 4096)
			if !ok || hash == "" {
				t.Fatalf("SelectDiff(%q) ok=%v hash=%q", tc.path, ok, hash)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("selection for %s lost its content:\n%s", tc.path, out)
			}
			if !strings.Contains(out, "diff --git") || !strings.Contains(out, "@@") {
				t.Errorf("selection for %s is not a valid diff:\n%s", tc.path, out)
			}
			// full markers only: a truncated marker would be a substring of its own file's content
			for _, other := range []string{"go listen(ctx)", "subprocess.run", "runs-on: ubuntu-latest", "Run the installer.", "system(cmd)"} {
				if other != tc.want && strings.Contains(out, other) {
					t.Errorf("selection for %s leaked another file's content (%s)", tc.path, other)
				}
			}
		})
	}
}

// The candidate's line picks the hunk. server.go has hunks at 10 and 201.
func TestSelectDiffPicksTheHunkCoveringTheLine(t *testing.T) {
	out, _, _ := SelectDiff(multiLangDiff, "internal/app/server.go", 201, 120)
	if !strings.Contains(out, "conn.Close()") {
		t.Errorf("line 201 must select the hunk at 201:\n%s", out)
	}
	if strings.Contains(out, "go listen(ctx)") {
		t.Errorf("a tight budget must not also carry the distant hunk:\n%s", out)
	}
	if !strings.Contains(out, "[metareview: showing 1 of 2 hunks") {
		t.Errorf("eliding hunks must be disclosed in the diff value:\n%s", out)
	}
}

// A candidate whose file is absent cannot be judged: that is knowable before spending a call.
func TestSelectDiffRefusesAnAbsentFile(t *testing.T) {
	if DiffHasFile(multiLangDiff, "internal/nope/missing.go") {
		t.Error("DiffHasFile must be false for a file the diff does not carry")
	}
	out, ok, hash := SelectDiff(multiLangDiff, "internal/nope/missing.go", 12, 4096)
	if ok || out != "" || hash != "" {
		t.Errorf("absent file must yield ok=false, got ok=%v out=%q", ok, out)
	}
}

func TestChangedPathsListsEveryFile(t *testing.T) {
	got := ChangedPaths(multiLangDiff)
	if len(got) != 5 {
		t.Fatalf("ChangedPaths = %v, want 5 entries", got)
	}
	if got[4] != "src/legacy code.rb" {
		t.Errorf("a path with a space must survive: %q", got[4])
	}
}

// A single hunk can exceed the budget on its own (this repo has hunks over 300 KB). The
// selection must still fit, so an oversized hunk is cut to a line window around the
// candidate - positionally, with no language knowledge - and the cut is disclosed.
func TestSelectDiffEnforcesTheBudgetOnAnOversizedHunk(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/big/file.txt b/big/file.txt\n--- a/big/file.txt\n+++ b/big/file.txt\n@@ -1,4000 +1,4000 @@\n")
	for i := 1; i <= 4000; i++ {
		b.WriteString("+line " + strings.Repeat("z", 40) + " " + strings.Repeat("0", 4) + "\n")
	}
	diff := b.String()
	const budget = 4096
	out, ok, _ := SelectDiff(diff, "big/file.txt", 2000, budget)
	if !ok {
		t.Fatal("file is present, want ok")
	}
	if len(out) > budget {
		t.Errorf("selection is %d bytes, over the %d budget", len(out), budget)
	}
	if !strings.Contains(out, "exceeded the budget") {
		t.Errorf("cutting inside a hunk must be disclosed:\n%s", out[:200])
	}
	if !strings.Contains(out, "@@") {
		t.Error("the hunk header must survive the window cut")
	}
}

// Malformed or unusual diff text must degrade, never panic or silently drop a file. These
// are all shapes real git output or a hostile payload can take.
func TestSelectDiffHandlesMalformedInput(t *testing.T) {
	t.Run("preamble before the first file block", func(t *testing.T) {
		d := "warning: something\nnot a diff line\n" + multiLangDiff
		if !DiffHasFile(d, "scripts/deploy.py") {
			t.Error("a preamble must not hide the file blocks")
		}
	})
	t.Run("header with no b/ side", func(t *testing.T) {
		d := "diff --git nonsense\n@@ -1 +1 @@\n+x\n"
		if got := ChangedPaths(d); len(got) != 0 {
			t.Errorf("unparseable header should contribute no path, got %v", got)
		}
	})
	t.Run("C-quoted non-ASCII path", func(t *testing.T) {
		d := "diff --git \"a/src/caf\\303\\251.go\" \"b/src/caf\\303\\251.go\"\n--- \"a/src/caf\\303\\251.go\"\n+++ \"b/src/caf\\303\\251.go\"\n@@ -1,1 +1,2 @@\n+brewed()\n"
		paths := ChangedPaths(d)
		if len(paths) != 1 || !strings.Contains(paths[0], "caf") {
			t.Fatalf("quoted path not decoded: %v", paths)
		}
		if out, ok, _ := SelectDiff(d, paths[0], 1, 4096); !ok || !strings.Contains(out, "brewed()") {
			t.Errorf("quoted path not selectable: ok=%v out=%q", ok, out)
		}
	})
	t.Run("hunk header with no post-image range", func(t *testing.T) {
		d := "diff --git a/x.txt b/x.txt\n@@ malformed @@\n+y\n"
		if out, ok, _ := SelectDiff(d, "x.txt", 1, 4096); !ok || !strings.Contains(out, "+y") {
			t.Errorf("a malformed hunk header must still carry its lines: ok=%v out=%q", ok, out)
		}
	})
	t.Run("hunk with a single-line range", func(t *testing.T) {
		d := "diff --git a/y.txt b/y.txt\n@@ -3 +3 @@\n+only\n"
		if out, ok, _ := SelectDiff(d, "y.txt", 3, 4096); !ok || !strings.Contains(out, "+only") {
			t.Errorf("count-less range must parse as 1: ok=%v out=%q", ok, out)
		}
	})
	t.Run("file block with no hunks", func(t *testing.T) {
		d := "diff --git a/bin.dat b/bin.dat\nBinary files differ\n"
		out, ok, _ := SelectDiff(d, "bin.dat", 0, 4096)
		if !ok || !strings.Contains(out, "Binary files differ") {
			t.Errorf("a hunkless block must still be selectable: ok=%v out=%q", ok, out)
		}
	})
	t.Run("zero budget", func(t *testing.T) {
		if out, ok, _ := SelectDiff(multiLangDiff, "docs/guide.md", 41, 0); !ok || out == "" {
			t.Errorf("a zero budget must still return the nearest evidence: ok=%v", ok)
		}
	})
}

// A finding's line can sit outside every hunk: reviewers report the line they read in the
// file, which may precede the first change or fall past the end of a windowed hunk.
func TestSelectDiffHandlesLinesOutsideEveryHunk(t *testing.T) {
	t.Run("line before the first hunk picks the nearest", func(t *testing.T) {
		// server.go has hunks at 10 and 201; line 1 is nearer the first.
		out, ok, _ := SelectDiff(multiLangDiff, "internal/app/server.go", 1, 130)
		if !ok {
			t.Fatal("want ok")
		}
		if !strings.Contains(out, "go listen(ctx)") {
			t.Errorf("line 1 should select the hunk at 10:\n%s", out)
		}
		if strings.Contains(out, "conn.Close()") {
			t.Errorf("a tight budget should not also carry the hunk at 201:\n%s", out)
		}
	})
	t.Run("line past the end of an oversized hunk clamps", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("diff --git a/big/f.txt b/big/f.txt\n--- a/big/f.txt\n+++ b/big/f.txt\n@@ -1,3000 +1,3000 @@\n")
		for i := 0; i < 3000; i++ {
			b.WriteString("+" + strings.Repeat("q", 60) + "\n")
		}
		const budget = 3000
		out, ok, _ := SelectDiff(b.String(), "big/f.txt", 999999, budget)
		if !ok {
			t.Fatal("want ok")
		}
		if len(out) > budget {
			t.Errorf("selection is %d bytes, over the %d budget", len(out), budget)
		}
		if !strings.Contains(out, "@@") {
			t.Error("the hunk header must survive")
		}
	})
}

func TestContextForAbsentFileSaysSoAndListsPaths(t *testing.T) {
	out, truncated, hash := ContextFor(multiLangDiff, false, "internal/nope/missing.go", 4, 4096)
	if !truncated || hash == "" {
		t.Fatalf("absent file must report truncated with a hash: %v %q", truncated, hash)
	}
	if !strings.Contains(out, "no diff is available for internal/nope/missing.go") {
		t.Errorf("the judge must be told the evidence is missing:\n%s", out)
	}
	if !strings.Contains(out, "scripts/deploy.py") {
		t.Errorf("the changed paths must be listed:\n%s", out)
	}
	if strings.Contains(out, "subprocess.run") {
		t.Errorf("only paths, not content:\n%s", out)
	}
}

func TestContextForPresentFileReturnsItsHunks(t *testing.T) {
	out, truncated, hash := ContextFor(multiLangDiff, false, "scripts/deploy.py", 5, 4096)
	if hash == "" || !strings.Contains(out, "subprocess.run") {
		t.Fatalf("present file must yield its hunks: %q", out)
	}
	if !truncated {
		t.Error("selecting one file out of five elides the rest, so truncated must be true")
	}
}

func TestContextForCapsThePathList(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 2000; i++ {
		p := "pkg/dir" + strconv.Itoa(i) + "/file.go"
		b.WriteString("diff --git a/" + p + " b/" + p + "\n@@ -1 +1 @@\n+x\n")
	}
	out, _, _ := ContextFor(b.String(), false, "not/here.go", 1, 30000)
	if len(out) > maxPathList+200 {
		t.Errorf("path list is %d bytes, must be capped near %d", len(out), maxPathList)
	}
	if !strings.Contains(out, "...") {
		t.Error("a capped list must say it was cut")
	}
}

// A finding may carry no file (an unlocalised reviewer note). There is nothing to select on,
// so it falls back to the head of the diff rather than reporting the evidence as missing.
func TestContextForUnlocalisedFindingFallsBackToTheHead(t *testing.T) {
	out, _, hash := ContextFor(multiLangDiff, false, "", 0, 4096)
	if hash == "" || !strings.Contains(out, "diff --git") {
		t.Fatalf("empty file must fall back to the diff head: %q", out)
	}
	if strings.Contains(out, "no diff is available") {
		t.Error("an unlocalised finding is not the same as a missing file")
	}
}

// Findings are routinely cross-file: 45 of this repo's 100 open blockers name another file
// in their prose. Selecting only the declared file hands the judge one side of a comparison,
// and it correctly declines - 9 of 37 rejections in run 3 cited a file they were not shown.
func TestContextForClaimIncludesReferencedFiles(t *testing.T) {
	text := "the guide at docs/guide.md says five, but scripts/deploy.py enforces eight"
	out, _, hash := ContextForClaim(multiLangDiff, false, "internal/app/server.go", 10, text, 8192)
	if hash == "" {
		t.Fatal("want a hash")
	}
	for _, want := range []string{"go listen(ctx)", "Run the installer.", "subprocess.run"} {
		if !strings.Contains(out, want) {
			t.Errorf("claim context is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "runs-on: ubuntu-latest") {
		t.Error("a file the text never names must not be pulled in")
	}
}

func TestReferencedPathsIgnoresUnknownAndSelf(t *testing.T) {
	text := "internal/app/server.go and docs/guide.md and internal/ghost/missing.go"
	got := ReferencedPaths(multiLangDiff, "internal/app/server.go", text)
	if len(got) != 1 || got[0] != "docs/guide.md" {
		t.Fatalf("got %v, want only docs/guide.md (self excluded, absent file excluded)", got)
	}
}

func TestReferencedPathsIsCapped(t *testing.T) {
	var b strings.Builder
	var text strings.Builder
	for i := 0; i < 12; i++ {
		p := "internal/p" + strconv.Itoa(i) + "/f.go"
		b.WriteString("diff --git a/" + p + " b/" + p + "\n@@ -1 +1 @@\n+x\n")
		text.WriteString(p + " ")
	}
	if got := ReferencedPaths(b.String(), "other.go", text.String()); len(got) != maxReferencedFiles {
		t.Fatalf("got %d refs, want the cap of %d", len(got), maxReferencedFiles)
	}
}

// The finding text is untrusted reviewer output, so it may only choose among hunks the diff
// already carries - never introduce content, and never reach outside the diff.
func TestReferencedPathsCannotIntroduceContent(t *testing.T) {
	text := "see /etc/passwd and ../../secrets.go and internal/nope/absent.go"
	if got := ReferencedPaths(multiLangDiff, "x.go", text); len(got) != 0 {
		t.Fatalf("untrusted text pulled in %v; only paths present in the diff are selectable", got)
	}
}

// Most findings name no other file; those must behave exactly as before.
func TestContextForClaimWithNoReferencesMatchesContextFor(t *testing.T) {
	const text = "a nil dereference happens on the second call"
	got, gotTrunc, gotHash := ContextForClaim(multiLangDiff, false, "docs/guide.md", 41, text, 4096)
	want, wantTrunc, wantHash := ContextFor(multiLangDiff, false, "docs/guide.md", 41, 4096)
	if got != want || gotTrunc != wantTrunc || gotHash != wantHash {
		t.Errorf("no-reference path diverged from ContextFor:\n got %q\nwant %q", got, want)
	}
}

// A finding can contradict a file the branch never touched - "the code now requires eight
// lenses but these four documents still say five". Nothing in the diff can settle that, and a
// sandbox of only the changed files cannot either. The trigger must still fire, and the
// evidence set must be able to include an unchanged file.
func TestMentionsOtherFilesSeesUnchangedPaths(t *testing.T) {
	text := "internal/reviewlog/reviewlog.go requires eight, but skills/review-artifact/SKILL.md says five"
	if !MentionsOtherFiles("internal/reviewlog/reviewlog.go", text) {
		t.Error("a claim against an unchanged file must still count as cross-file")
	}
	if MentionsOtherFiles("internal/reviewlog/reviewlog.go", "a purely local bug on line 170") {
		t.Error("a finding naming no other file is not cross-file")
	}
	if MentionsOtherFiles("internal/reviewlog/reviewlog.go", "see internal/reviewlog/reviewlog.go again") {
		t.Error("naming only its own file is not cross-file")
	}
}

// The paths a finding names are evidence whether or not the branch changed them, so the
// sandbox has to be able to carry them. ReferencedPaths stays diff-filtered for selecting
// hunks; this is the wider set used to build the evidence tree.
func TestAllReferencedPathsIgnoresTheDiff(t *testing.T) {
	text := "docs/guide.md and skills/nowhere/SKILL.md and internal/x/y.go"
	got := AllReferencedPaths("internal/x/y.go", text)
	if len(got) != 2 || got[0] != "docs/guide.md" || got[1] != "skills/nowhere/SKILL.md" {
		t.Fatalf("got %v, want both named paths excluding the finding's own file", got)
	}
	// distinct paths: repeating one would dedup to a single entry and never reach the cap
	var many strings.Builder
	for i := 0; i < 50; i++ {
		many.WriteString("dir/f" + strconv.Itoa(i) + ".go ")
	}
	if n := len(AllReferencedPaths("a.go", many.String())); n != maxReferencedFiles {
		t.Errorf("got %d refs, want the cap of %d", n, maxReferencedFiles)
	}
}

func TestNamedPathsEdgeCases(t *testing.T) {
	t.Run("a slash-bearing match from rootFile is not double counted", func(t *testing.T) {
		// rootFile can match inside a longer path; only bare names may be added.
		got := namedPaths("see internal/x/y.go for details")
		if len(got) != 1 || got[0] != "internal/x/y.go" {
			t.Fatalf("got %v, want just the full path", got)
		}
	})
	t.Run("prose abbreviations are not files", func(t *testing.T) {
		if got := namedPaths("e.g. a bug, i.e. a real one"); len(got) != 0 {
			t.Errorf("got %v, want none: a two-character stem minimum excludes these", got)
		}
	})
	t.Run("a bare name and its own full path collapse to one", func(t *testing.T) {
		got := namedPaths("docs/guide.md, referred to later as guide.md")
		if len(got) != 1 || got[0] != "docs/guide.md" {
			t.Fatalf("got %v, want one entry", got)
		}
	})
	t.Run("a bare name that is nobody's tail is kept", func(t *testing.T) {
		got := namedPaths("docs/guide.md and README.md")
		if len(got) != 2 {
			t.Fatalf("got %v, want both", got)
		}
	})
}

func TestAllReferencedPathsSkipsRepeats(t *testing.T) {
	got := AllReferencedPaths("a.go", "docs/x.md and docs/x.md again and docs/y.md")
	if len(got) != 2 || got[0] != "docs/x.md" || got[1] != "docs/y.md" {
		t.Fatalf("got %v, want each path once", got)
	}
}

// DiffTruncated is the one signal telling a judge it is not seeing the whole story, so its two
// states both have to be constrained. ContextForClaim bound ContextFor's second return - named
// `truncated` - to a variable named `ok` and then OR'd in `!ok`, so the term contributed
// "truncated" exactly when the primary file was NOT truncated and contributed nothing when it
// was. The bug was masked by a third term, len(out) < len(diff), which is true for essentially
// any multi-file diff: the function returned the right answer for the wrong reason, and mutating
// !ok to ok left the whole suite green.
func TestContextForClaimReportsTruncationOfThePrimaryFile(t *testing.T) {
	two := "diff --git a/pkg/a.go b/pkg/a.go\n--- a/pkg/a.go\n+++ b/pkg/a.go\n@@ -1,2 +1,3 @@\n+\taMarker()\n" +
		"diff --git a/pkg/b.go b/pkg/b.go\n--- a/pkg/b.go\n+++ b/pkg/b.go\n@@ -1,2 +1,3 @@\n+\tbMarker()\n"

	// Both files carried in full: the judge IS seeing the whole story, so the flag must be false.
	// The old code got this right only by accident - its `!ok` term was false here because the
	// primary selection reports itself truncated by construction on this path, so the answer came
	// entirely from the length test. Pinning it means the inverted term cannot be reintroduced
	// without a failure.
	out, truncated, _ := ContextForClaim(two, false, "pkg/a.go", 1, "compare with pkg/b.go", 1<<20)
	if !strings.Contains(out, "aMarker") || !strings.Contains(out, "bMarker") {
		t.Fatalf("fixture invalid: both files must be carried, got %q", out)
	}
	if truncated {
		t.Error("a claim whose primary and referenced files are both carried whole is not truncated")
	}

	// An upstream truncation always survives.
	if _, truncated, _ := ContextForClaim(two, true, "pkg/a.go", 1, "compare with pkg/b.go", 1<<20); !truncated {
		t.Error("alreadyTruncated must survive")
	}
}

// h.lines[0] is the @@ header, and the window is taken from h.lines[lo:hi+1] with the header
// prepended - so a hunk whose only line IS the header comes back with it twice. With
// len(h.lines)==1 the clamp drives centre to 0, giving lo==hi==0, and the result is a malformed
// hunk: two @@ headers and no body. A judge reading that sees a hunk that cannot be parsed back.
func TestWindowLinesDoesNotDuplicateTheHunkHeader(t *testing.T) {
	h := hunk{start: 1, lines: []string{"@@ -1,1 +1,1 @@"}}
	got := windowLines(h, 1, 4000)
	if len(got) != 1 {
		t.Errorf("a header-only hunk must come back once, got %d lines: %q", len(got), got)
	}
	// And the ordinary case must be unaffected: header, then the window.
	h2 := hunk{start: 1, lines: []string{"@@ -1,3 +1,3 @@", " a", "+b", " c"}}
	got2 := windowLines(h2, 2, 4000)
	if len(got2) == 0 || got2[0] != "@@ -1,3 +1,3 @@" {
		t.Fatalf("the header must lead: %q", got2)
	}
	for _, line := range got2[1:] {
		if strings.HasPrefix(line, "@@") {
			t.Errorf("a second @@ header appears in the body: %q", got2)
		}
	}
}

// "The same file" has to mean the same thing everywhere, or a finding is denied evidence the diff
// plainly carries. DiffHasFile compared raw strings, so a discover node emitting "./internal/x.go"
// or "a/internal/x.go" - both ordinary spellings, both produced by real tools - was treated as
// naming a file absent from the diff, and its finding was kept as unverified_no_evidence for a
// human who would have found it right there in the patch.
func TestDiffHasFileNormalisesOrdinaryPathSpellings(t *testing.T) {
	diff := "diff --git a/internal/x.go b/internal/x.go\n--- a/internal/x.go\n+++ b/internal/x.go\n@@ -1,2 +1,3 @@\n+\tmarker()\n"
	for _, spelling := range []string{
		"internal/x.go",
		"./internal/x.go",
		"a/internal/x.go",
		"b/internal/x.go",
		"internal/./x.go",
		" internal/x.go ",
	} {
		if !DiffHasFile(diff, spelling) {
			t.Errorf("DiffHasFile(%q) = false; the diff carries that file", spelling)
		}
	}
	for _, absent := range []string{"internal/y.go", "x.go", "other/internal/x.go"} {
		if DiffHasFile(diff, absent) {
			t.Errorf("DiffHasFile(%q) = true; that file is not in the diff", absent)
		}
	}
}
