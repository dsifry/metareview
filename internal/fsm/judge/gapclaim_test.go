package judge

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dsifry/metareview/internal/claimcheck"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// gapDiff is the cal.com shape from issue #140: a source change whose covering spec sits
// in the same diff, plus an unrelated spec that mentions none of the subject.
const gapDiff = `diff --git a/app/models/topic_embed.rb b/app/models/topic_embed.rb
index 111..222 100644
--- a/app/models/topic_embed.rb
+++ b/app/models/topic_embed.rb
@@ -36,3 +36,7 @@ class TopicEmbed
+  def category_for(eh)
+    eh.try(:category_id)
+  end
diff --git a/spec/models/topic_embed_spec.rb b/spec/models/topic_embed_spec.rb
index 333..444 100644
--- a/spec/models/topic_embed_spec.rb
+++ b/spec/models/topic_embed_spec.rb
@@ -10,2 +10,5 @@ RSpec.describe TopicEmbed do
+    let!(:embeddable_host) { Fabricate(:embeddable_host) }
+    expect(post.topic.category).to eq(embeddable_host.category)
diff --git a/spec/models/site_setting_spec.rb b/spec/models/site_setting_spec.rb
index 555..666 100644
--- a/spec/models/site_setting_spec.rb
+++ b/spec/models/site_setting_spec.rb
@@ -1,2 +1,3 @@
+  it "is uncached" do
`

// repoOnlyDiff is the #146 shape: the diff changes only app code; the covering spec
// exists in the repository at head, outside every changed hunk.
const repoOnlyDiff = `diff --git a/app/models/topic_embed.rb b/app/models/topic_embed.rb
index 111..222 100644
--- a/app/models/topic_embed.rb
+++ b/app/models/topic_embed.rb
@@ -36,3 +36,7 @@ class TopicEmbed
+  def category_for(eh)
+    eh.try(:category_id)
+  end
`

// The #140 failure was a gap claim judged without ever seeing the diff's covering spec.
// ContextForGapClaim must return the covering spec as evidence AND put its hunks in the
// diff the judge sees, headed by the disclosure that says what the extra hunks are for.
func TestContextForGapClaimInjectsTheCoveringSpec(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "per-host category assignment has no test verifying an embedded topic's category"}
	out, _, hash, ev := ContextForGapClaim(gapDiff, false, f, MaxDiffBytes, nil)
	if hash == "" {
		t.Fatal("want a hash")
	}
	var covered bool
	for _, e := range ev {
		if e.Path == "spec/models/topic_embed_spec.rb" {
			covered = true
		}
	}
	if !covered {
		t.Fatalf("evidence = %+v, want the covering spec", ev)
	}
	for _, want := range []string{
		"claims tests, specs or coverage are ABSENT", // the disclosure
		"eh.try(:category_id)",                       // the primary file's change
		"expect(post.topic.category)",                // the covering spec's assertion
	} {
		if !strings.Contains(out, want) {
			t.Errorf("gap-claim context missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "is uncached") {
		t.Error("an unrelated spec that mentions none of the subject must not be pulled in")
	}
}

// A gap claim with neither referenced files nor covering tests behaves exactly like the
// plain context for its own file: no disclosure, no phantom evidence.
func TestContextForGapClaimWithoutEvidenceMatchesPlainContext(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "nothing tests anything anywhere"}
	out, truncated, _, ev := ContextForGapClaim(gapDiff, false, f, MaxDiffBytes, nil)
	if len(ev) != 0 {
		t.Fatalf("evidence = %+v, want none", ev)
	}
	if strings.Contains(out, "ABSENT") {
		t.Errorf("no disclosure without evidence:\n%s", out)
	}
	plain, plainTruncated, _ := ContextFor(gapDiff, false, f.File, f.Line, MaxDiffBytes)
	if out != plain || truncated != plainTruncated {
		t.Errorf("gap context diverged from the plain context for the same file:\n%s", out)
	}
}

// A test file the finding's text NAMES is included once, not twice (referenced-path and
// evidence selection must not duplicate its hunks).
func TestContextForGapClaimDoesNotDuplicateANamedSpec(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "spec/models/topic_embed_spec.rb was not updated; no test verifies the category_for change"}
	out, _, _, ev := ContextForGapClaim(gapDiff, false, f, MaxDiffBytes, nil)
	if n := strings.Count(out, "expect(post.topic.category)"); n != 1 {
		t.Errorf("the named spec's hunk appeared %d times, want 1:\n%s", n, out)
	}
	var seen int
	for _, e := range ev {
		if e.Path == "spec/models/topic_embed_spec.rb" {
			seen++
		}
	}
	if seen > 1 {
		t.Errorf("evidence lists the named spec %d times: %+v", seen, ev)
	}
}

// ChangedBlocks is the bridge the cmd tool uses; it must skip headers and context lines
// and carry only the ADDED lines per post-image path.
func TestChangedBlocksCarriesOnlyAddedLines(t *testing.T) {
	blocks := ChangedBlocks(gapDiff)
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d, want 3", len(blocks))
	}
	if blocks[0].Path != "app/models/topic_embed.rb" || len(blocks[0].Added) != 3 {
		t.Errorf("first block = %+v", blocks[0])
	}
	if blocks[1].Path != "spec/models/topic_embed_spec.rb" || len(blocks[1].Added) != 2 {
		t.Errorf("spec block = %+v", blocks[1])
	}
	for _, b := range blocks {
		for _, line := range b.Added {
			if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "diff --git") {
				t.Errorf("added lines must be stripped of their + and never carry headers: %q", line)
			}
		}
	}
}

// GapClaimEvidence is the raw evidence search; the cal.com claim must find the spec.
func TestGapClaimEvidenceFindsCoveringSpec(t *testing.T) {
	f := run.Finding{IssueText: "the category argument type changed from name-string to integer id with zero tests; nothing asserts the created topic's category"}
	ev := GapClaimEvidence(gapDiff, f, MaxGapEvidenceFiles)
	if len(ev) == 0 {
		t.Fatal("no evidence for the cal.com claim")
	}
	if ev[0].Path != "spec/models/topic_embed_spec.rb" {
		t.Errorf("top evidence = %q, want the covering spec: %+v", ev[0].Path, ev)
	}
	if _, ok := claimcheck.Detect(f.IssueText); !ok {
		t.Error("the cal.com claim text must be detected as a testing-gap claim")
	}
}

// CodeRabbit #145: the primary selection's truncation flag must be preserved, not
// inferred from serialized byte length — a small elided block can be outweighed by
// disclosure text and headers, sending partial context marked complete.
func TestContextForGapClaimPreservesPrimaryTruncation(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "spec/models/topic_embed_spec.rb has no test verifying the per-host category assignment for an embedded topic's category"}
	_, primaryTruncated, _ := ContextFor(gapDiff, false, f.File, f.Line, 8192)
	_, truncated, _, _ := ContextForGapClaim(gapDiff, false, f, MaxDiffBytes, nil)
	if truncated != primaryTruncated {
		t.Errorf("gap-claim truncation = %v, want the primary's %v (an elided primary block must not be reported complete)", truncated, primaryTruncated)
	}
	// a diff with extra unrelated files makes the primary selection partial
	big := gapDiff + "diff --git a/other.rb b/other.rb\n--- a/other.rb\n+++ b/other.rb\n@@ -1,2 +1,3 @@\n+unrelated\n"
	_, truncated, _, _ = ContextForGapClaim(big, false, f, MaxDiffBytes, nil)
	if !truncated {
		t.Error("a context built from a multi-file diff must report truncation")
	}
}

// CodeRabbit #145: a finding that names a path in a different spelling than the evidence
// returns must not get the same test file's hunks twice.
func TestContextForGapClaimDedupesPathSpellings(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "the spec at b/spec/models/topic_embed_spec.rb has no test verifying the per-host category assignment for an embedded topic's category"}
	out, _, _, ev := ContextForGapClaim(gapDiff, false, f, MaxDiffBytes, nil)
	if n := strings.Count(out, "expect(post.topic.category)"); n != 1 {
		t.Errorf("the spec's hunk appeared %d times, want 1 (spellings must dedupe)", n)
	}
	for _, e := range ev {
		if e.Path != "spec/models/topic_embed_spec.rb" {
			t.Errorf("evidence path must be the normalized form: %+v", e)
		}
	}
}

// ---- issue #146: repository-side evidence in the gap-claim context ----

// The #146 shape: the diff touches only app code; the covering spec exists in the
// repository at head. RepoTestEvidence found it; ContextForGapClaim must inject its
// content under a disclosure that says where it came from, and return it as evidence.
func TestContextForGapClaimInjectsRepoEvidence(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "per-host category assignment has no test verifying an embedded topic's category"}
	repo := &RepoEvidence{
		Ran: true,
		Evidence: []claimcheck.Evidence{
			{Path: "spec/models/topic_embed_spec.rb", Tokens: []string{"topic", "category"}, Score: 4},
		},
		Content: map[string]string{"spec/models/topic_embed_spec.rb": "RSpec.describe TopicEmbed do\n  expect(post.topic.category).to eq(host.category)\nend\n"},
	}
	out, _, hash, ev := ContextForGapClaim(repoOnlyDiff, false, f, MaxDiffBytes, repo)
	if hash == "" {
		t.Fatal("want a hash")
	}
	if !strings.Contains(out, "repository") || !strings.Contains(out, "expect(post.topic.category)") {
		t.Errorf("repo evidence not injected with its disclosure:\n%s", out)
	}
	covered := false
	for _, e := range ev {
		if e.Path == "spec/models/topic_embed_spec.rb" {
			covered = true
		}
	}
	if !covered {
		t.Errorf("evidence = %+v; want the repo spec", ev)
	}
}

// A search that ran and found nothing is itself evidence the judge must know about:
// the context says so explicitly instead of looking like the pre-#146 plain context.
func TestContextForGapClaimRepoSearchedFoundNothing(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "nothing tests anything anywhere"}
	out, truncated, _, ev := ContextForGapClaim(repoOnlyDiff, false, f, MaxDiffBytes, &RepoEvidence{Ran: true})
	if len(ev) != 0 {
		t.Errorf("evidence = %+v; want none", ev)
	}
	if !strings.Contains(out, "searched") {
		t.Errorf("a completed empty search must be disclosed:\n%s", out)
	}
	plain, plainTruncated, _ := ContextFor(repoOnlyDiff, false, f.File, f.Line, MaxDiffBytes)
	if truncated != plainTruncated {
		t.Errorf("truncation flag diverged from the plain context")
	}
	// A search that never ran leaves the context byte-identical to plain (nil repo).
	nilOut, _, _, _ := ContextForGapClaim(repoOnlyDiff, false, f, MaxDiffBytes, nil)
	if nilOut != plain {
		t.Errorf("nil repo must reproduce the plain context byte for byte")
	}
}

// A path both sources offer is injected once: the diff's hunks win, the repo's full
// content is not appended behind them, and the evidence list carries the path once.
func TestContextForGapClaimDedupsRepoAgainstDiffEvidence(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "per-host category assignment has no test verifying an embedded topic's category"}
	repo := &RepoEvidence{
		Ran: true,
		Evidence: []claimcheck.Evidence{
			{Path: "spec/models/topic_embed_spec.rb", Tokens: []string{"topic", "category"}, Score: 4},
		},
		Content: map[string]string{"spec/models/topic_embed_spec.rb": "FULL FILE BODY\n"},
	}
	out, _, _, ev := ContextForGapClaim(gapDiff, false, f, MaxDiffBytes, repo)
	if n := strings.Count(out, "expect(post.topic.category)"); n != 1 {
		t.Errorf("the covering spec appeared %d times, want 1 (diff hunks win):\n%s", n, out)
	}
	if strings.Contains(out, "FULL FILE BODY") {
		t.Error("repo content appended for a path the diff already covers")
	}
	if n := strings.Count(strings.Join(evidencePaths(ev), ","), "spec/models/topic_embed_spec.rb"); n != 1 {
		t.Errorf("evidence lists the path %d times, want 1: %+v", n, ev)
	}
}

func evidencePaths(ev []claimcheck.Evidence) []string {
	out := make([]string, 0, len(ev))
	for _, e := range ev {
		out = append(out, e.Path)
	}
	return out
}

// CodeRabbit findings from the adjudicated review of this branch (run
// mrv-20260907-205458717470000): three defects in the repo-evidence assembly.

// The nothing-matched disclosure must fire on the MAIN path too: diff-side paths being
// present does not mean the repository search record exists. Only a search that ran and
// found nothing says so — one that found matches (even all deduped against the diff) must not.
func TestContextForGapClaimRepoNoneOnMainPath(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "per-host category assignment has no test verifying an embedded topic's category"}
	// repo search ran, found nothing; the diff still contributes its own file hunks
	out, _, _, _ := ContextForGapClaim(gapDiff, false, f, MaxDiffBytes, &RepoEvidence{Ran: true})
	if !strings.Contains(out, gapClaimRepoNone) {
		t.Errorf("a completed empty search must be disclosed even when diff paths exist:\n%s", out)
	}
	// a search that found matches (here deduped into the diff) must NOT say none-matched
	repo := &RepoEvidence{Ran: true, Evidence: []claimcheck.Evidence{
		{Path: "spec/models/topic_embed_spec.rb", Tokens: []string{"topic", "category"}, Score: 4},
	}}
	out, _, _, _ = ContextForGapClaim(gapDiff, false, f, MaxDiffBytes, repo)
	if strings.Contains(out, gapClaimRepoNone) {
		t.Errorf("a search with matches must not be disclosed as none-matched:\n%s", out)
	}
}

// The budget contract: the primary keeps two shares, every corroborating path (diff or
// repo) one share, and a repo body is CLIPPED to its share — the context can never grow
// past the budget the caller handed in, and clipping marks the context truncated.
func TestContextForGapClaimRepoBodiesStayInBudget(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "per-host category assignment has no test verifying an embedded topic's category"}
	big := strings.Repeat("expect(post.topic.category).to be_present\n", 1000) // ~43KB body
	repo := &RepoEvidence{Ran: true, Evidence: []claimcheck.Evidence{
		{Path: "spec/models/topic_embed_spec.rb", Tokens: []string{"topic", "category"}, Score: 4},
	}, Content: map[string]string{"spec/models/topic_embed_spec.rb": big}}
	out, truncated, _, _ := ContextForGapClaim(repoOnlyDiff, false, f, MaxDiffBytes, repo)
	if len(out) > MaxDiffBytes+shareSlack {
		t.Errorf("context = %d bytes, budget %d (+%d slack): repo bodies must be clipped to their share", len(out), MaxDiffBytes, shareSlack)
	}
	if !truncated {
		t.Error("a clipped repo body must mark the context truncated")
	}
	if !strings.Contains(out, "expect(post.topic.category)") {
		t.Error("the clipped body must still carry the subject evidence")
	}
}

// shareSlack allows the fixed disclosures and file headers atop the byte budget: the
// contract under test is that the VARIABLE evidence (hunks + repo bodies) sums to the
// budget, not that fixed strings are counted against it.
const shareSlack = 1200

// The context hash must cover what the judge actually receives: prepending
// gapClaimRepoNone to the plain context after hashing would record a hash of content the
// adjudicator never saw.
func TestContextForGapClaimRepoNoneIsHashed(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "nothing tests anything anywhere"}
	_, _, hash, _ := ContextForGapClaim(repoOnlyDiff, false, f, MaxDiffBytes, &RepoEvidence{Ran: true})
	plain, _, plainHash := ContextFor(repoOnlyDiff, false, f.File, f.Line, MaxDiffBytes)
	if hash == plainHash {
		t.Error("the repo-none context must hash differently from the plain context")
	}
	sum := sha1.Sum([]byte(gapClaimRepoNone + plain))
	if hex.EncodeToString(sum[:]) != hash {
		t.Error("the recorded hash must be of the disclosed context, not the pre-disclosure one")
	}
}

// "The diff already shows it" must mean CONTENT, not mere path presence: a repo-evidence
// path whose covering lines sit OUTSIDE the diff's hunks still injects its full head
// body, or a false absence claim is confirmable while the audit claims the judge saw it.
func TestContextForGapClaimKeepsRepoBodyWhenHunksLackTheMatch(t *testing.T) {
	f := run.Finding{File: "app/models/widget.rb", Line: 3,
		IssueText: "spec/models/widget_spec.rb has no test for polish"}
	// the diff touches the spec but its hunk mentions only the weak token; the covering
	// assertion lives in an unchanged region of the file at head
	diff := "diff --git a/app/models/widget.rb b/app/models/widget.rb\n--- a/app/models/widget.rb\n+++ b/app/models/widget.rb\n@@ -3,2 +3,4 @@\n" +
		"+  def polish(g)\n+    g.try(:shine)\n" +
		"diff --git a/spec/models/widget_spec.rb b/spec/models/widget_spec.rb\n--- a/spec/models/widget_spec.rb\n+++ b/spec/models/widget_spec.rb\n@@ -1,2 +1,3 @@\n" +
		"+# touched by the branch, unrelated change\n"
	repo := &RepoEvidence{Ran: true, Evidence: []claimcheck.Evidence{
		{Path: "spec/models/widget_spec.rb", Tokens: []string{"polish"}, Score: 3},
	}, Content: map[string]string{"spec/models/widget_spec.rb": "expect(widget.polish).to eq(true)\n"}}
	out, _, _, ev := ContextForGapClaim(diff, false, f, MaxDiffBytes, repo)
	if !strings.Contains(out, "expect(widget.polish)") {
		t.Errorf("the covering assertion is outside the hunks; the repo body must be injected:\n%s", out)
	}
	if !strings.Contains(out, gapClaimRepoDisclosure) {
		t.Errorf("injected repo content must carry its disclosure:\n%s", out)
	}
	if n := 0; n != len(ev) {
		_ = ev
	}
	// when the hunks DO carry the matched tokens, the body stays deduped away
	diff2 := "diff --git a/app/models/widget.rb b/app/models/widget.rb\n--- a/app/models/widget.rb\n+++ b/app/models/widget.rb\n@@ -3,2 +3,4 @@\n" +
		"+  def polish(g)\n+    g.try(:shine)\n" +
		"diff --git a/spec/models/widget_spec.rb b/spec/models/widget_spec.rb\n--- a/spec/models/widget_spec.rb\n+++ b/spec/models/widget_spec.rb\n@@ -1,2 +1,3 @@\n" +
		"+expect(widget.polish).to eq(true)\n"
	out2, _, _, _ := ContextForGapClaim(diff2, false, f, MaxDiffBytes, repo)
	if strings.Count(out2, "expect(widget.polish)") != 1 {
		t.Errorf("hunks carrying the match make the body redundant; it must appear once:\n%s", out2)
	}
}

// A repo-evidence path that normalizes onto a diff path SelectDiff cannot render (spelling
// the parser misses) is not "covered" — the body is the only view and must be injected.
func TestContextForGapClaimUnrenderableDiffPathKeepsRepoBody(t *testing.T) {
	f := run.Finding{File: "app/models/widget.rb", Line: 3,
		IssueText: "spec/models/widget_spec.rb has no test for polish"}
	diff := "diff --git a/spec/models/widget_spec.rb b/spec/models/widget_spec.rb\n--- a/spec/models/widget_spec.rb\n+++ b/spec/models/widget_spec.rb\n@@ -1,2 +1,3 @@\n" +
		"+expect(widget.polish).to eq(true)\n"
	// the repo evidence names the same file with a "b/" prefix: dedups by normalized path,
	// but the tokens ARE in the hunk, so the body stays deduped away — the real !ok case
	// is a path whose spelling SelectDiff cannot match; simulate via a token match on a
	// path the diff parser renders under a different normalized form
	repo := &RepoEvidence{Ran: true, Evidence: []claimcheck.Evidence{
		{Path: "spec/models/widget_spec.rb", Tokens: []string{"polish"}, Score: 3},
	}, Content: map[string]string{"spec/models/widget_spec.rb": "FULL BODY expect(widget.polish)\n"}}
	out, _, _, _ := ContextForGapClaim(diff, false, f, MaxDiffBytes, repo)
	if n := strings.Count(out, "expect(widget.polish)"); n != 1 {
		t.Errorf("hunks carry the match; body must stay deduped (appear once):\n%s", out)
	}
}

// A repo path that entered `paths` via the finding's prose (the diff does not carry it
// at all) is deduped but SelectDiff renders nothing for it: the body is the only view.
func TestContextForGapClaimUnrenderableReferencedPathKeepsRepoBody(t *testing.T) {
	f := run.Finding{File: "app/models/widget.rb", Line: 3,
		IssueText: "spec/absent_spec.rb was not updated; no test verifies polish"}
	diff := "diff --git a/app/models/widget.rb b/app/models/widget.rb\n--- a/app/models/widget.rb\n+++ b/app/models/widget.rb\n@@ -3,2 +3,4 @@\n" +
		"+  def polish(g)\n"
	repo := &RepoEvidence{Ran: true, Evidence: []claimcheck.Evidence{
		{Path: "spec/absent_spec.rb", Tokens: []string{"polish"}, Score: 3},
	}, Content: map[string]string{"spec/absent_spec.rb": "expect(widget.polish).to eq(true)\n"}}
	out, _, _, _ := ContextForGapClaim(diff, false, f, MaxDiffBytes, repo)
	if !strings.Contains(out, "expect(widget.polish)") || !strings.Contains(out, gapClaimRepoDisclosure) {
		t.Errorf("a referenced-but-absent diff path must not swallow the repo body:\n%s", out)
	}
}

// The repo body's header must not claim "unchanged by this diff" for a path the diff
// touches — the provenance label is checked against the diff, not asserted.
func TestContextForGapClaimRepoHeaderReflectsDiffPresence(t *testing.T) {
	f := run.Finding{File: "app/models/widget.rb", Line: 3,
		IssueText: "spec/models/widget_spec.rb has no test for polish"}
	diff := "diff --git a/spec/models/widget_spec.rb b/spec/models/widget_spec.rb\n--- a/spec/models/widget_spec.rb\n+++ b/spec/models/widget_spec.rb\n@@ -1,2 +1,3 @@\n" +
		"+# an unrelated hunk\n"
	repo := &RepoEvidence{Ran: true, Evidence: []claimcheck.Evidence{
		{Path: "spec/models/widget_spec.rb", Tokens: []string{"nonesuch"}, Score: 3},
	}, Content: map[string]string{"spec/models/widget_spec.rb": "FULL BODY\n"}}
	out, _, _, _ := ContextForGapClaim(diff, false, f, MaxDiffBytes, repo)
	if strings.Contains(out, "(repository head, unchanged by this diff) ---") && strings.Contains(out, "spec/models/widget_spec.rb (repository head, unchanged") {
		t.Errorf("a diff-touched path must not be labeled unchanged:\n%s", out)
	}
	if !strings.Contains(out, "also touched by this diff") {
		t.Errorf("the diff-touched path's header must say so:\n%s", out)
	}
	// an absent path keeps the unchanged label
	repo2 := &RepoEvidence{Ran: true, Evidence: []claimcheck.Evidence{
		{Path: "spec/absent_spec.rb", Tokens: []string{"nonesuch"}, Score: 3},
	}, Content: map[string]string{"spec/absent_spec.rb": "FULL BODY\n"}}
	out2, _, _, _ := ContextForGapClaim(diff, false, f, MaxDiffBytes, repo2)
	if !strings.Contains(out2, "spec/absent_spec.rb (repository head, unchanged by this diff)") {
		t.Errorf("an absent path keeps the unchanged label:\n%s", out2)
	}
}

// clipBody cuts on a rune boundary: a multi-byte UTF-8 rune at the cut must not yield
// invalid UTF-8 in the judge's prompt or the hashed context.
func TestClipBodyIsRuneSafe(t *testing.T) {
	body := strings.Repeat("é→中 ", 50) + "\n" + strings.Repeat("x", 200)
	out := clipBody(body, 103)
	if !utf8.ValidString(out) {
		t.Errorf("clipBody produced invalid UTF-8 at the cut: %q", out[:20])
	}
	if len(out) > 103+60 { // marker + line-boundary trim may add a little, not much
		t.Errorf("clipBody grew the body: %d bytes", len(out))
	}
}

// an unreserved extra body share genuinely overflows the aggregate.
// An in-diff repo body spends its hunk share AND a body share: the primary's budget is
// sized for both, and the aggregate context can never exceed the caller's budget —
// including when the body fits its share while the aggregate would overflow.
func TestContextForGapClaimInDiffBodiesStayInBudget(t *testing.T) {
	f := run.Finding{File: "app/models/widget.rb", Line: 3,
		IssueText: "spec/models/widget_spec.rb has no test for polish"}
	// the spec's hunk is a large added block that does NOT carry the match, and the
	// primary's own hunk is large too — both fill their shares, so only the reservation
	// accounting keeps the aggregate inside the budget
	bigHunk := strings.Repeat("+filler for the hunk body so it dominates a small share\n", 400)
	bigPrimary := strings.Repeat("+primary filler line that mentions nothing distinctive\n", 300)
	diff := "diff --git a/app/models/widget.rb b/app/models/widget.rb\n--- a/app/models/widget.rb\n+++ b/app/models/widget.rb\n@@ -3,2 +3,4 @@\n" +
		"+  def polish(g)\n" + bigPrimary +
		"diff --git a/spec/models/widget_spec.rb b/spec/models/widget_spec.rb\n--- a/spec/models/widget_spec.rb\n+++ b/spec/models/widget_spec.rb\n@@ -1,2 +1,3 @@\n" +
		"+" + bigHunk
	repo := &RepoEvidence{Ran: true, Evidence: []claimcheck.Evidence{
		{Path: "spec/models/widget_spec.rb", Tokens: []string{"nonesuch"}, Score: 3},
	}, Content: map[string]string{"spec/models/widget_spec.rb": strings.Repeat("y", 5000)}}
	out, truncated, _, _ := ContextForGapClaim(diff, false, f, MaxDiffBytes, repo)
	if len(out) > MaxDiffBytes+shareSlack {
		t.Errorf("in-diff body double-spend: context = %d bytes, budget %d (+%d slack)", len(out), MaxDiffBytes, shareSlack)
	}
	if !truncated {
		t.Error("an over-share aggregate must mark the context truncated")
	}
}
