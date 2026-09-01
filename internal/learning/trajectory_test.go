package learning

import (
	"testing"

	"github.com/dsifry/metareview/internal/findings"
)

func TestWhitespaceOnly(t *testing.T) {
	// whitespace-only: same code, different indentation → flag.
	ws := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-\tx := 1\n+  x := 1\n"
	if !whitespaceOnly(ws) {
		t.Fatal("a whitespace-only change must be flagged")
	}
	// comment-only: comments dropped on both sides, no code change → flag.
	co := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,2 @@\n-// old note\n+// new note\n code()\n"
	if !whitespaceOnly(co) {
		t.Fatal("a comment-only change must be flagged")
	}
	// LOAD-BEARING NEGATIVE: a substantive code change must NOT be flagged.
	sub := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-x := 1\n+x := 2\n"
	if whitespaceOnly(sub) {
		t.Fatal("a substantive code change must NOT be flagged")
	}
	// a pure code addition (added, nothing removed) is substantive → not flagged.
	add := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,2 @@\n ctx\n+newCode()\n"
	if whitespaceOnly(add) {
		t.Fatal("a real code addition must NOT be flagged")
	}
	// no change at all → nothing to flag.
	if whitespaceOnly("") || whitespaceOnly("diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n unchanged\n") {
		t.Fatal("an empty or no-op diff must not be flagged")
	}
}

func TestSelfServed(t *testing.T) {
	// same actor requested and granted → flag.
	if !selfServed(findings.Record{OverrideRequestedBy: "alice", OverrideGrantedBy: "Alice"}) {
		t.Fatal("requester == granter (case-insensitive) must be self-served")
	}
	// granted with no request at all → flag (the separation was bypassed).
	if !selfServed(findings.Record{OverrideGrantedBy: "bob"}) {
		t.Fatal("a grant with no request must be self-served")
	}
	// LOAD-BEARING NEGATIVE: distinct requester and granter → NOT self-served.
	if selfServed(findings.Record{OverrideRequestedBy: "alice", OverrideGrantedBy: "bob"}) {
		t.Fatal("a request+grant by two distinct actors must NOT be self-served")
	}
	// not granted → nothing to flag (a pending request is not a self-served grant).
	if selfServed(findings.Record{OverrideRequestedBy: "alice"}) {
		t.Fatal("an ungranted (pending) override must not be self-served")
	}
}

func TestTrajectoryFlagsAdvisoryAndDiscriminating(t *testing.T) {
	ws := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-\tx := 1\n+  x := 1\n"
	records := []findings.Record{
		{ID: "f1", Title: "self grant", OverrideRequestedBy: "alice", OverrideGrantedBy: "alice"},
		{ID: "f2", Title: "clean grant", OverrideRequestedBy: "alice", OverrideGrantedBy: "bob"}, // must NOT flag
	}
	flags := trajectoryFlags(ws, records)
	if len(flags) != 2 {
		t.Fatalf("expected a whitespace flag and one self-served flag (not the clean grant): %+v", flags)
	}
	for _, f := range flags {
		if f.Kind != "trajectory-flag" || f.Disposition != "advisory" {
			t.Fatalf("trajectory flags must be advisory: %+v", f)
		}
	}
	// A substantive diff with only a clean override → no flags at all (no over-flagging).
	sub := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-x := 1\n+x := 2\n"
	if got := trajectoryFlags(sub, []findings.Record{{OverrideRequestedBy: "alice", OverrideGrantedBy: "bob"}}); len(got) != 0 {
		t.Fatalf("a substantive fix with a clean override must raise no flags: %+v", got)
	}
}

// The whitespace-only heuristic must not over-flag the fragile cases Bugbot found: content lines that
// look like diff headers (`++i`/`--i`), ambiguous prefixes (`*p`, `//go:build` directives), and
// file-moves/reorders (per-file, order-sensitive).
func TestWhitespaceOnlyRobustness(t *testing.T) {
	no := map[string]string{
		"real pointer-deref edit":      "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-\t*p = old\n+\t*p = new\n",
		"go:build directive edit":      "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-//go:build linux\n+//go:build windows\n",
		"increment content edit":       "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-\t--i\n+\t++i\n",
		"reorder within a file":        "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,2 @@\n-a()\n-b()\n+b()\n+a()\n",
		"move across files (add side)": "diff --git a/b.go b/b.go\n--- a/b.go\n+++ b/b.go\n@@ -0,0 +1,1 @@\n+moved()\n",
	}
	for name, d := range no {
		if whitespaceOnly(d) {
			t.Fatalf("%s must NOT be flagged whitespace-only", name)
		}
	}
	// True positives still fire: a pure reindent, and a normal comment-only edit.
	yes := map[string]string{
		"reindent":     "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-\tx := 1\n+  x := 1\n",
		"comment-only": "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,2 +1,2 @@\n-// old note\n+// new note\n code()\n",
	}
	for name, d := range yes {
		if !whitespaceOnly(d) {
			t.Fatalf("%s must be flagged whitespace-only", name)
		}
	}
}

func TestExtractCandidatesEmitsTrajectoryFlags(t *testing.T) {
	in := Input{
		Findings: []findings.Record{{ID: "f1", OverrideRequestedBy: "alice", OverrideGrantedBy: "alice"}},
		Diff:     "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-\tx := 1\n+ x := 1\n",
	}
	res := ExtractCandidates(in)
	if len(res.Flags) != 2 {
		t.Fatalf("ExtractCandidates must surface trajectory flags: %+v", res.Flags)
	}
}
