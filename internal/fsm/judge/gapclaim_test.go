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

// The #140 failure was a gap claim judged without ever seeing the diff's covering spec.
// ContextForGapClaim must return the covering spec as evidence AND put its hunks in the
// diff the judge sees, headed by the disclosure that says what the extra hunks are for.
func TestContextForGapClaimInjectsTheCoveringSpec(t *testing.T) {
	f := run.Finding{File: "app/models/topic_embed.rb", Line: 37,
		IssueText: "per-host category assignment has no test verifying an embedded topic's category"}
	out, _, hash, ev := ContextForGapClaim(gapDiff, false, f, MaxDiffBytes)
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
		"expect(post.topic.category)",                 // the covering spec's assertion
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
	out, truncated, _, ev := ContextForGapClaim(gapDiff, false, f, MaxDiffBytes)
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
	out, _, _, ev := ContextForGapClaim(gapDiff, false, f, MaxDiffBytes)
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
	ev := GapClaimEvidence(gapDiff, f, 3)
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
