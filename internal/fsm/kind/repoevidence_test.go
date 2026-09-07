package kind

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/claimcheck"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// Issue #146: the repository-side covering-test search reaches the judge's context
// through the executor. Deps.RepoSearch is optional; nil (mock and judge-less runs)
// keeps the diff-only behavior byte for byte.

// repoAppOnlyDiff touches only app code — the covering test, if any, lives in the repo.
var repoAppOnlyDiff = "diff --git a/app/models/topic_embed.rb b/app/models/topic_embed.rb\n" +
	"--- a/app/models/topic_embed.rb\n+++ b/app/models/topic_embed.rb\n@@ -36,3 +36,7 @@\n" +
	"+  def category_for(eh)\n+    eh.try(:category_id)\n+  end\n"

// The repo search found the covering spec outside the diff: the gap call carries its
// content under the repo disclosure, and the audit's claim evidence lists the path.
func TestAdjudicateRepoSearchInjectsOutOfDiffEvidence(t *testing.T) {
	rec := &recordingJudge{}
	r, err := New(Deps{Judge: rec, RepoSearch: func(context.Context, run.Snapshot, run.Finding) (judge.RepoEvidence, error) {
		return judge.RepoEvidence{
			Ran: true,
			Evidence: []claimcheck.Evidence{
				{Path: "spec/models/topic_embed_spec.rb", Tokens: []string{"topic", "category"}, Score: 4},
			},
			Content: map[string]string{"spec/models/topic_embed_spec.rb": "RSpec.describe TopicEmbed do\n  expect(post.topic.category).to eq(host.category)\nend\n"},
		}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	ex, _ := r.Executor(MatchThenAdjudicate)
	snap := run.Snapshot{RunID: "mrv-repo", Iteration: 1, Findings: []run.Finding{
		{IssueText: "per-host category assignment has no test verifying an embedded topic's category", File: "app/models/topic_embed.rb", Line: 37},
	}}
	a := &allAudits{}
	if _, err := ex.Execute(context.Background(), machine.ExecInput{
		Snap: snap, Node: adjNode, Diff: machine.Diff{Text: repoAppOnlyDiff}, StartIndex: 0, Audit: a.fn}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(rec.diffs) != 1 {
		t.Fatalf("judge called %d times, want 1", len(rec.diffs))
	}
	gap := rec.diffs[0]
	for _, want := range []string{
		"repository head was also searched",       // the repo disclosure
		"repository head, unchanged by this diff", // the per-file header
		"expect(post.topic.category)",             // the covering assertion from head
	} {
		if !strings.Contains(gap, want) {
			t.Errorf("gap call missing %q:\n%s", want, gap)
		}
	}
	var claimRow *run.LLMCallData
	for _, ev := range a.events {
		if ev.Type != run.TypeLLMCall {
			continue
		}
		var d run.LLMCallData
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			t.Fatal(err)
		}
		if d.ClaimClass == string(claimcheck.ClassTestingGap) {
			claimRow = &d
		}
	}
	if claimRow == nil {
		t.Fatal("no claim-class audit row")
	}
	found := false
	for _, p := range claimRow.ClaimEvidence {
		if p == "spec/models/topic_embed_spec.rb" {
			found = true
		}
	}
	if !found {
		t.Errorf("claim evidence = %v, want the repo spec path", claimRow.ClaimEvidence)
	}
}

// A repo-search seam error fails open: the node runs with the diff-only context, and the
// failure is memoized so one broken seam costs one attempt per node, not one per claim.
func TestAdjudicateRepoSearchErrorFailsOpen(t *testing.T) {
	calls := 0
	r, err := New(Deps{Judge: &recordingJudge{}, RepoSearch: func(context.Context, run.Snapshot, run.Finding) (judge.RepoEvidence, error) {
		calls++
		return judge.RepoEvidence{}, errors.New("git grep failed")
	}})
	if err != nil {
		t.Fatal(err)
	}
	ex, _ := r.Executor(MatchThenAdjudicate)
	snap := run.Snapshot{RunID: "mrv-err", Iteration: 1, Findings: []run.Finding{
		{IssueText: "parseFoo is untested", File: "pkg/parse.go", Line: 1},
		{IssueText: "parseFoo is also untested", File: "pkg/parse.go", Line: 9},
	}}
	a := &allAudits{}
	raw, err := ex.Execute(context.Background(), machine.ExecInput{
		Snap: snap, Node: adjNode, Diff: machine.Diff{Text: repoAppOnlyDiff}, StartIndex: 0, Audit: a.fn})
	if err != nil {
		t.Fatalf("a repo-search failure must fail open, not fail the node: %v", err)
	}
	var out adjudicateOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Confirmed) != 2 {
		t.Errorf("confirmed = %d, want 2 (both claims still adjudicated)", len(out.Confirmed))
	}
	if calls != 1 {
		t.Errorf("RepoSearch called %d times across two claims after the first error; want 1 (memoized)", calls)
	}
}

// Two gap claims whose subject tokens coincide search the repository once.
func TestAdjudicateRepoSearchMemoizedBySubject(t *testing.T) {
	calls := 0
	r, err := New(Deps{Judge: &recordingJudge{}, RepoSearch: func(context.Context, run.Snapshot, run.Finding) (judge.RepoEvidence, error) {
		calls++
		return judge.RepoEvidence{Ran: true}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	ex, _ := r.Executor(MatchThenAdjudicate)
	snap := run.Snapshot{RunID: "mrv-memo", Iteration: 1, Findings: []run.Finding{
		{IssueText: "parseFoo is untested", File: "pkg/parse.go", Line: 1},
		{IssueText: "parseFoo is also untested", File: "pkg/parse.go", Line: 9},
	}}
	a := &allAudits{}
	if _, err := ex.Execute(context.Background(), machine.ExecInput{
		Snap: snap, Node: adjNode, Diff: machine.Diff{Text: repoAppOnlyDiff}, StartIndex: 0, Audit: a.fn}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if calls != 1 {
		t.Errorf("RepoSearch called %d times for two claims with the same subject; want 1", calls)
	}
}

// A repo search that ran and found nothing does not count as covering-test evidence:
// the claim stays made, and the metric's with_evidence stays zero (verified-absent is
// the judge's verdict, not the search's).
func TestAdjudicateRepoSearchEmptyCountsClaimMade(t *testing.T) {
	r, err := New(Deps{Judge: &recordingJudge{}, RepoSearch: func(context.Context, run.Snapshot, run.Finding) (judge.RepoEvidence, error) {
		return judge.RepoEvidence{Ran: true}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	ex, _ := r.Executor(MatchThenAdjudicate)
	snap := run.Snapshot{RunID: "mrv-empty", Iteration: 1, Findings: []run.Finding{
		{IssueText: "nothing asserts the embedded topic's category", File: "app/models/topic_embed.rb", Line: 37},
	}}
	a := &allAudits{}
	if _, err := ex.Execute(context.Background(), machine.ExecInput{
		Snap: snap, Node: adjNode, Diff: machine.Diff{Text: repoAppOnlyDiff}, StartIndex: 0, Audit: a.fn}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var rollup *run.RecordData
	for _, ev := range a.events {
		if ev.Type != run.TypeRecord {
			continue
		}
		var d run.RecordData
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			t.Fatal(err)
		}
		if d.Name == "claimcheck" {
			rollup = &d
		}
	}
	if rollup == nil {
		t.Fatal("no claimcheck record")
	}
	if !strings.Contains(string(rollup.Data), `"claims":1`) || strings.Contains(string(rollup.Data), `"with_evidence":1`) {
		t.Errorf("an empty repo search must not count as evidence: %s", rollup.Data)
	}
}

// A path both sources offer lands in claim.Evidence once: the audit list stays
// one-entry-per-file even when the diff and the repo search agree.
func TestAdjudicateRepoSearchDedupsAgainstDiffEvidence(t *testing.T) {
	diff := "diff --git a/app/models/topic_embed.rb b/app/models/topic_embed.rb\n" +
		"--- a/app/models/topic_embed.rb\n+++ b/app/models/topic_embed.rb\n@@ -36,3 +36,7 @@\n" +
		"+  def category_for(eh)\n+    eh.try(:category_id)\n+  end\n" +
		"diff --git a/spec/models/topic_embed_spec.rb b/spec/models/topic_embed_spec.rb\n" +
		"--- a/spec/models/topic_embed_spec.rb\n+++ b/spec/models/topic_embed_spec.rb\n@@ -10,2 +10,5 @@\n" +
		"+    expect(post.topic.category).to eq(embeddable_host.category)\n"
	r, err := New(Deps{Judge: &recordingJudge{}, RepoSearch: func(context.Context, run.Snapshot, run.Finding) (judge.RepoEvidence, error) {
		return judge.RepoEvidence{
			Ran: true,
			Evidence: []claimcheck.Evidence{
				{Path: "b/spec/models/topic_embed_spec.rb", Tokens: []string{"topic", "category"}, Score: 4}, // diff-header spelling of the same file
			},
			Content: map[string]string{"b/spec/models/topic_embed_spec.rb": "FULL BODY\n"},
		}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	ex, _ := r.Executor(MatchThenAdjudicate)
	snap := run.Snapshot{RunID: "mrv-dedup", Iteration: 1, Findings: []run.Finding{
		{IssueText: "per-host category assignment has no test verifying an embedded topic's category", File: "app/models/topic_embed.rb", Line: 37},
	}}
	a := &allAudits{}
	if _, err := ex.Execute(context.Background(), machine.ExecInput{
		Snap: snap, Node: adjNode, Diff: machine.Diff{Text: diff}, StartIndex: 0, Audit: a.fn}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	n := 0
	for _, ev := range a.events {
		if ev.Type != run.TypeLLMCall {
			continue
		}
		var d run.LLMCallData
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			t.Fatal(err)
		}
		if d.ClaimClass != string(claimcheck.ClassTestingGap) {
			continue
		}
		for _, p := range d.ClaimEvidence {
			if strings.HasSuffix(p, "topic_embed_spec.rb") {
				n++
			}
		}
	}
	if n != 1 {
		t.Errorf("claim evidence lists the covering spec %d times, want 1", n)
	}
}
