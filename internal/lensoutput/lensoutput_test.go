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
		// path-spelling equivalence (PR #162 review): ./, b/, and C-quoted variants of the
		// same file must not be judged fabricated
		{"dot-slash-spelling", "./a.go", 2, 3, true},
		{"b-prefix-spelling", "b/a.go", 2, 3, true},
		{"c-quoted-finding", "\"a.go\"", 2, 3, true},
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

// TestAnchorMapHeaderPairing pins the phantom-file fix: a content line inside a hunk can
// forge the `+++ b/…` to-header shape (an added line whose text is `++ b/fake.go` renders as
// `+++ b/fake.go`), but it cannot forge the header PAIR — only a `+++ b/` that follows a
// `--- ` from-header switches the current file. Without the pair check, one crafted content
// line steals every later hunk of the real file into a phantom path.
func TestAnchorMapHeaderPairing(t *testing.T) {
	// forge 1: a bare to-header shape inside hunk body — ignored
	d := "diff --git a/real.go b/real.go\n--- a/real.go\n+++ b/real.go\n@@ -1,3 +1,4 @@\n ctx\n+++ b/fake.go\n+added\n ctx\n@@ -10,2 +11,2 @@\n-old\n+new\n"
	m := AnchorMap(d)
	if _, ok := m["fake.go"]; ok {
		t.Fatalf("forged to-header must not create a phantom file: %v", m)
	}
	if rs := m["real.go"]; len(rs) != 2 {
		t.Fatalf("both real hunks must attribute to real.go: %v", m)
	}
	// forge 2: the crafted pair — a deleted line whose text is `-- from` renders as
	// `--- from` (matching the from-header prefix), and an added line whose text is
	// `++ b/evil.go` renders as `+++ b/evil.go` right after it. The pair check raises the
	// forgery bar from ONE crafted line to TWO ADJACENT crafted lines; this pins that bar
	// as the documented contract (the residual is accepted deliberately: the threat model
	// is accidental corruption and degenerate patch-emitting code, not adversarial
	// construction — see the AnchorMap doc comment).
	d2 := "diff --git a/real.go b/real.go\n--- a/real.go\n+++ b/real.go\n@@ -1,3 +1,4 @@\n ctx\n--- from\n+++ b/evil.go\n+added\n@@ -10,2 +11,2 @@\n-old\n+new\n"
	m2 := AnchorMap(d2)
	if _, ok := m2["evil.go"]; !ok {
		t.Fatalf("adjacent crafted pair switches files — the documented two-line bar: %v", m2)
	}
	// rename: hunks accumulate under the NEW path; the old path never appears
	d3 := "diff --git a/old.go b/new.go\nsimilarity index 85%\nrename from a/old.go\nrename to b/new.go\n--- a/old.go\n+++ b/new.go\n@@ -1,2 +1,3 @@\n ctx\n+one\n@@ -20,2 +30,3 @@\n ctx\n+two\n"
	m3 := AnchorMap(d3)
	if _, ok := m3["old.go"]; ok {
		t.Fatalf("rename must key the new path only: %v", m3)
	}
	if rs := m3["new.go"]; len(rs) != 2 || rs[0] != (LineRange{1, 3}) || rs[1] != (LineRange{30, 32}) {
		t.Fatalf("rename hunks under new path: %v", m3)
	}
	// a path whose to-header appears twice with proper pairs (two `---`/`+++` blocks naming
	// the same target — not something git emits, but the union is the defensive contract)
	// accumulates ranges across both blocks.
	d5 := "diff --git a/r.go b/r.go\n--- a/r.go\n+++ b/r.go\n@@ -1,2 +1,3 @@\n ctx\n+one\n--- a/r.go\n+++ b/r.go\n@@ -50,2 +60,3 @@\n ctx\n+two\n"
	m5 := AnchorMap(d5)
	if rs := m5["r.go"]; len(rs) != 2 || rs[0] != (LineRange{1, 3}) || rs[1] != (LineRange{60, 62}) {
		t.Fatalf("repeated paired headers accumulate: %v", m5)
	}
	// atoi saturates instead of wrapping: an absurd hunk start stays a huge positive range,
	// never a negative/inverted one.
	d4 := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +99999999999999999999,2 @@\n+x\n"
	m4 := AnchorMap(d4)
	if rs := m4["x.go"]; len(rs) != 1 || rs[0].Start <= 0 || rs[0].End < rs[0].Start {
		t.Fatalf("saturation: %v", m4)
	}
}

func TestAnchorInDiffPathEquivalence(t *testing.T) {
	// The diff side may itself carry unusual spellings: a C-quoted header key
	// (core.quotePath default) or prefixed forms. Both sides normalize.
	m := AnchorMap("diff --git a/p.go b/p.go\n--- a/p.go\n+++ b/p.go\n@@ -1,1 +1,1 @@\n+x\n")
	if len(m) != 1 {
		t.Fatalf("plain key parse: %v", m)
	}
	quoted := AnchorMap("diff --git a/q.go b/q.go\n--- a/q.go\n+++ \"b/q u.go\"\n@@ -1,1 +1,1 @@\n+x\n")
	// the quoted header names q u.go; a finding citing the plain spelling must match
	if got := AnchorInDiff(TypedFinding{File: "q u.go", StartLine: 1, EndLine: 1}, quoted); !got {
		t.Fatalf("finding did not match its own C-quoted header: %v", quoted)
	}
	for _, spelling := range []string{"p.go", "./p.go", "b/p.go", "a/p.go"} {
		if got := AnchorInDiff(TypedFinding{File: spelling, StartLine: 1, EndLine: 1}, m); !got {
			t.Errorf("spelling %q judged fabricated against its own hunk", spelling)
		}
	}
	if got := AnchorInDiff(TypedFinding{File: "other.go", StartLine: 1, EndLine: 1}, m); got {
		t.Error("different file matched")
	}
}

func TestAnchorInDiffFallbackMatchesLaterKey(t *testing.T) {
	// The fallback loop scans the whole map when the finding's normalized key is not
	// literally present — keys themselves may carry spellings the normalizer strips
	// (a hand-recorded diff, or a prefixed key). With several keys the match may come
	// after non-matches: the loop must keep scanning, break on the hit, and miss cleanly
	// when nothing matches.
	m := map[string][]LineRange{
		"zzz.go":  {{1, 1}},
		"b/mm.go": {{5, 5}}, // prefixed key: only reachable via the fallback
	}
	if !AnchorInDiff(TypedFinding{File: "mm.go", StartLine: 5, EndLine: 5}, m) {
		t.Fatal("fallback must match a later prefixed key after passing non-matches")
	}
	if AnchorInDiff(TypedFinding{File: "absent.go", StartLine: 1, EndLine: 1}, m) {
		t.Fatal("no key matches — must be rejected")
	}
}
