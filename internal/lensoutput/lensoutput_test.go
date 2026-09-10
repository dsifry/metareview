package lensoutput

import (
	"errors"
	"testing"
)

func validFinding() TypedFinding {
	return TypedFinding{
		Tag: TagBug, File: "a.go", StartLine: 3, EndLine: 5,
		Issue: "getUserById dereferences the nil user", Consequence: "500 on every deleted user",
		Confidence: 75, Severity: "P1",
	}
}

func TestValidate(t *testing.T) {
	rows := []struct {
		name string
		mut  func(*TypedFinding)
		want error
	}{
		{"ok", func(*TypedFinding) {}, nil},
		{"tag", func(f *TypedFinding) { f.Tag = "smell" }, ErrTag},
		{"severity", func(f *TypedFinding) { f.Severity = "high" }, ErrSeverity},
		{"confidence-high", func(f *TypedFinding) { f.Confidence = 101 }, ErrConfidence},
		{"confidence-negative", func(f *TypedFinding) { f.Confidence = -1 }, ErrConfidence},
		{"file-empty", func(f *TypedFinding) { f.File = "" }, ErrFile},
		{"file-blank", func(f *TypedFinding) { f.File = "   " }, ErrFile},
		{"start-zero", func(f *TypedFinding) { f.StartLine = 0 }, ErrLineRange},
		{"end-before-start", func(f *TypedFinding) { f.EndLine = 2 }, ErrLineRange},
		{"issue-empty", func(f *TypedFinding) { f.Issue = "" }, ErrIssue},
		{"issue-blank", func(f *TypedFinding) { f.Issue = "  " }, ErrIssue},
		{"consequence-empty", func(f *TypedFinding) { f.Consequence = "" }, ErrConsequence},
	}
	for _, r := range rows {
		f := validFinding()
		r.mut(&f)
		err := f.Validate()
		if r.want == nil {
			if err != nil {
				t.Errorf("%s: unexpected error %v", r.name, err)
			}
			continue
		}
		if !errors.Is(err, r.want) {
			t.Errorf("%s: got %v want %v", r.name, err, r.want)
		}
	}
}

func TestValidateFirstFailingCheckWins(t *testing.T) {
	// Both tag and severity invalid: the tag is reported first (declaration order), so a
	// caller fixing errors one at a time makes progress, and the error identity is stable.
	f := validFinding()
	f.Tag, f.Severity = "smell", "high"
	if err := f.Validate(); !errors.Is(err, ErrTag) {
		t.Fatalf("tag must win: %v", err)
	}
}

const testDiff = `diff --git a/a.go b/a.go
index 111..222 100644
--- a/a.go
+++ b/a.go
@@ -1,4 +1,6 @@
 context
+added one
+added two
 context
 context
 context
@@ -40,3 +48,4 @@
 context
+added three
 context
 context
diff --git a/b/deleted.go b/deleted.go
deleted file mode 100644
--- b/deleted.go
+++ /dev/null
@@ -1,2 +0,0 @@
-lost one
-lost two
`

func TestAnchorMap(t *testing.T) {
	m := AnchorMap(testDiff)
	ranges, ok := m["a.go"]
	if !ok || len(ranges) != 2 {
		t.Fatalf("a.go hunks: %v %v", ok, m)
	}
	// @@ -1,4 +1,6 @@ -> lines 1..6 of the new side.
	if ranges[0] != (LineRange{1, 6}) {
		t.Errorf("hunk 1: %v", ranges[0])
	}
	// @@ -40,3 +48,4 @@ -> lines 48..51 (far enough from hunk 1 that the ±10 slack
	// zones do not overlap: hunk 1 covers -9..16, hunk 2 covers 38..61).
	if ranges[1] != (LineRange{48, 51}) {
		t.Errorf("hunk 2: %v", ranges[1])
	}
	// /dev/null deletions contribute no new-side file entry: there is no file to anchor to.
	if _, ok := m["b/deleted.go"]; ok {
		t.Error("deleted file must not appear in the anchor map")
	}
}

func TestAnchorMapCRLFAndContextHeaders(t *testing.T) {
	// A CRLF diff leaves \r on split lines; trimming it is what keeps lookups working
	// (the lab's .strip() equivalence). Function-context after @@ must not break the hunk.
	crlf := "diff --git a/x.go b/x.go\r\n--- a/x.go\r\n+++ b/x.go\r\n@@ -1,2 +3,4 @@ func name(void)\r\n ctx\r\n+new\r\n ctx\r\n"
	m := AnchorMap(crlf)
	ranges, ok := m["x.go"]
	if !ok || len(ranges) != 1 || ranges[0] != (LineRange{3, 6}) {
		t.Fatalf("crlf hunk: %+v", m)
	}
}

func TestAnchorInDiff(t *testing.T) {
	m := AnchorMap(testDiff)
	rows := []struct {
		name       string
		file       string
		start, end int
		want       bool
	}{
		{"in-hunk", "a.go", 2, 3, true},
		{"exact-hunk-edge", "a.go", 6, 6, true},
		{"context-slack-before", "a.go", 1, 1, true},          // hunk starts at 1; slack below
		{"context-slack-after", "a.go", 14, 16, true},         // hunk1 ends 6; 16 = 6+10
		{"span-straddling-slack", "a.go", 15, 20, true},       // range intersects hunk1's slack zone
		{"between-hunks-out", "a.go", 17, 17, false},          // 17 > 6+10 and < 48-10
		{"second-hunk-slack-edge-in", "a.go", 38, 38, true},   // 38 = 48-10
		{"second-hunk-slack-edge-out", "a.go", 37, 37, false}, // one below the slack
		{"second-hunk", "a.go", 50, 51, true},                 // inside hunk 2 (48..51)
		{"wrong-file", "nope.go", 2, 2, false},
		{"empty-file", "", 2, 2, false},
	}
	for _, r := range rows {
		f := TypedFinding{File: r.file, StartLine: r.start, EndLine: r.end}
		if got := AnchorInDiff(f, m); got != r.want {
			t.Errorf("%s: AnchorInDiff=%v want %v", r.name, got, r.want)
		}
	}
}

func TestCanonicalTextAndAnchorAndToCandidate(t *testing.T) {
	f := validFinding()
	f.Issue, f.Consequence = " trimmed issue ", " trimmed consequence "
	if got, want := f.CanonicalText(), "[BUG] trimmed issue trimmed consequence"; got != want {
		t.Errorf("canonical: %q want %q", got, want)
	}
	if got, want := f.Anchor(), "a.go:3-5"; got != want {
		t.Errorf("anchor: %q want %q", got, want)
	}
	c := f.ToCandidate("metareview-lens/security")
	if c.IssueText != f.CanonicalText() || c.File != f.File || c.Line != f.StartLine ||
		c.Severity != "p1" || c.Category != "bug" || c.Source != "metareview-lens/security" {
		t.Errorf("candidate: %+v", c)
	}
	f.Tag = TagAdvisory
	c = f.ToCandidate("x")
	if c.Category != "advisory" || c.Severity != "p1" {
		t.Errorf("advisory candidate: %+v", c)
	}
}
