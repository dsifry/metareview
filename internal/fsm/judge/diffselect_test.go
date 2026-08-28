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
