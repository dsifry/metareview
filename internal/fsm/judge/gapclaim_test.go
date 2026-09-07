package judge

import (
	"strings"
	"testing"

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
