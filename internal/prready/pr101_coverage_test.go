package prready

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/reviewlog"
)

// contextIdentityDoc builds a pr-ready context pack the way contextMarkdown does: a top-level "Run ID:"
// before any section, then a "## Git" section carrying the base/head/branch/digest bullets.
func contextIdentityDoc(runID, base, head, branch, digest string) string {
	return "# metareview pr-ready context\n\n" +
		"Run ID: `" + runID + "`\n\n" +
		"## Git\n\n" +
		"- Base: `" + base + "`\n" +
		"- Head: `" + head + "`\n" +
		"- Branch: `" + branch + "`\n" +
		"- " + reviewlog.ReviewerInputDigestLabel + " `" + digest + "`\n"
}

func TestParsePRReadyContextIdentity(t *testing.T) {
	id := parsePRReadyContextIdentity(contextIdentityDoc("mrv-1", "base-sha", "head-sha", "feature", "sha256:abc"))
	if id.RunID != "mrv-1" || id.Base != "base-sha" || id.Head != "head-sha" || id.Branch != "feature" || id.ReviewInputDigest != "sha256:abc" {
		t.Fatalf("full parse mismatch: %+v", id)
	}
}

func TestParsePRReadyContextIdentityRespectsSections(t *testing.T) {
	// "Run ID:" is only captured before any section; the Git bullets only inside "## Git". A Run ID under
	// Git, and a Head bullet outside Git, must both be ignored — this kills the section-guard mutants.
	text := "# metareview pr-ready context\n\n" +
		"- Head: `stray-head-outside-git`\n\n" + // outside "## Git": must be ignored
		"## Git\n\n" +
		"Run ID: `wrong-under-git`\n" + // "Run ID:" under a section: must be ignored
		"- Head: `real-head`\n" +
		"- Branch: `feature`\n"
	id := parsePRReadyContextIdentity(text)
	if id.RunID != "" {
		t.Fatalf("Run ID under a section must not be captured, got %q", id.RunID)
	}
	if id.Head != "real-head" {
		t.Fatalf("Head must come from the Git section only, got %q", id.Head)
	}
}

func TestParsePRReadyContextIdentityFirstMatchWins(t *testing.T) {
	// A second "- Head:" must not override the first (the `identity.Head == ""` guards).
	text := "# metareview pr-ready context\n\n" +
		"## Git\n\n" +
		"- Head: `first`\n" +
		"- Head: `second`\n"
	if id := parsePRReadyContextIdentity(text); id.Head != "first" {
		t.Fatalf("first Head must win, got %q", id.Head)
	}
}

func TestLegacyEscalationLocksCurrentHead(t *testing.T) {
	writeContext := func(t *testing.T, root, rel, doc string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("empty current head locks (hard stop)", func(t *testing.T) {
		if !legacyEscalationLocksCurrentHead(t.TempDir(), reviewlog.Summary{}, gitcontext.Context{}) {
			t.Fatal("with no current head the escalation must lock")
		}
	})

	t.Run("reviewed head equal to current locks", func(t *testing.T) {
		log := reviewlog.Summary{HeadSHA: "h1"}
		if !legacyEscalationLocksCurrentHead(t.TempDir(), log, gitcontext.Context{HeadSHA: "h1"}) {
			t.Fatal("same reviewed head must lock")
		}
	})

	t.Run("reviewed head different from current does not lock", func(t *testing.T) {
		log := reviewlog.Summary{HeadSHA: "old"}
		if legacyEscalationLocksCurrentHead(t.TempDir(), log, gitcontext.Context{HeadSHA: "new"}) {
			t.Fatal("a different reviewed head must not lock a later commit")
		}
	})

	t.Run("no reviewed head falls back to context identity: match locks", func(t *testing.T) {
		root := t.TempDir()
		rel := "docs/metareview/context/c.md"
		writeContext(t, root, rel, contextIdentityDoc("mrv-1", "b", "h1", "feature", "sha256:x"))
		log := reviewlog.Summary{ContextRel: rel} // no HeadSHA
		if !legacyEscalationLocksCurrentHead(root, log, gitcontext.Context{HeadSHA: "h1"}) {
			t.Fatal("context head equal to current must lock")
		}
	})

	t.Run("no reviewed head, context head differs: does not lock", func(t *testing.T) {
		root := t.TempDir()
		rel := "docs/metareview/context/c.md"
		writeContext(t, root, rel, contextIdentityDoc("mrv-1", "b", "old", "feature", "sha256:x"))
		log := reviewlog.Summary{ContextRel: rel}
		if legacyEscalationLocksCurrentHead(root, log, gitcontext.Context{HeadSHA: "new"}) {
			t.Fatal("context head different from current must not lock")
		}
	})

	t.Run("no reviewed head, context unreadable: locks (no evidence of staleness)", func(t *testing.T) {
		log := reviewlog.Summary{ContextRel: "docs/metareview/context/missing.md"}
		if !legacyEscalationLocksCurrentHead(t.TempDir(), log, gitcontext.Context{HeadSHA: "new"}) {
			t.Fatal("an unreadable context must preserve the hard stop")
		}
	})

	t.Run("no reviewed head, context has no head: locks", func(t *testing.T) {
		root := t.TempDir()
		rel := "docs/metareview/context/c.md"
		// Branch present but no Head bullet, so readLegacyPRReadyContextIdentity succeeds yet identity.Head is "".
		writeContext(t, root, rel, "# metareview pr-ready context\n\n## Git\n\n- Branch: `feature`\n")
		log := reviewlog.Summary{ContextRel: rel}
		if !legacyEscalationLocksCurrentHead(root, log, gitcontext.Context{HeadSHA: "new"}) {
			t.Fatal("a context with no head must preserve the hard stop")
		}
	})
}

func TestLatestLogsByTargetSortsByTarget(t *testing.T) {
	logs := []reviewlog.Summary{
		{Target: "c", RunID: "mrv-3"},
		{Target: "a", RunID: "mrv-1"},
		{Target: "a", RunID: "mrv-9"}, // same target: only the latest (max run id) is kept
		{Target: "", RunID: "mrv-x"},  // empty target is dropped
		{Target: "b", RunID: "mrv-2"},
	}
	got := latestLogsByTarget(logs)
	if len(got) != 3 {
		t.Fatalf("expected one entry per non-empty target, got %+v", got)
	}
	if got[0].Target != "a" || got[1].Target != "b" || got[2].Target != "c" {
		t.Fatalf("results must be sorted by target, got %+v", got)
	}
	if got[0].RunID != "mrv-9" {
		t.Fatalf("the latest run for a target must win, got %q", got[0].RunID)
	}
}
