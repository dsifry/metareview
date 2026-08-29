package kind

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/sandbox"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// End to end with every service real and only the outermost seams faked: a real materialized
// sandbox, the real judge router, the real registry and executor. Only the CLI subprocess and
// the source of file contents are stubbed, because those are the two things that reach outside
// the process. This is the shape the production wiring has to satisfy before it is written.
func TestEscalationEndToEndWithMocks(t *testing.T) {
	// 1. the evidence, materialized as production will materialize it
	root := t.TempDir()
	tree, err := sandbox.Materialize(root, "base-sha", "head-sha",
		[]string{"server.go", "scripts/deploy.py"},
		func(rev, path string) ([]byte, bool, error) {
			return []byte(rev + " contents of " + path + "\n"), true, nil
		})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}

	// 2. the judge: real router, real codex arm, fake subprocess
	var ranIn []string
	fakeCLI := func(_ context.Context, dir string, _ []string, _ string) ([]byte, int, error) {
		ranIn = append(ranIn, dir)
		return []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"{\"reasoning\":\"r\",\"is_real\":true,\"confidence\":0.95}"}}` + "\n" +
			`{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":3,"reasoning_output_tokens":1}}` + "\n"), 0, nil
	}
	router, err := judge.NewWithCodex(nil, judge.Keys{}, judge.URLs{},
		func() string { return "nonce0" },
		judge.Clock{Now: func() time.Time { return time.Unix(0, 0) }, After: time.After}, fakeCLI)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	confined := judge.WithCodexWorkDir(router, tree.Root)

	// 3. the registry, with escalation injected
	reg, err := New(Deps{
		Judge: &scriptedJudge{real: false}, // the cheap arm rejects
		Escalate: func(context.Context, run.Snapshot, *workflow.Node) (*Escalation, error) {
			return &Escalation{
				Judge: confined, Model: "codex/gpt-5.6-sol", Effort: "medium",
				Evidence: run.EvidenceSandbox, TreeHash: tree.TreeHash,
				BaseSHA: tree.BaseSHA, HeadSHA: tree.HeadSHA,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	ex, _ := reg.Executor(MatchThenAdjudicate)

	a := &audits{}
	raw, err := ex.Execute(context.Background(), machine.ExecInput{
		Snap: escalationSnap(), Node: adjNode, Diff: machine.Diff{Text: escalationDiff}, StartIndex: 0, Audit: a.fn})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// 4. the escalation actually ran, confined to the materialized evidence
	if len(ranIn) != 1 || ranIn[0] != tree.Root {
		t.Fatalf("codex ran in %v, want exactly one call in %q", ranIn, tree.Root)
	}

	// 5. the cross-file rejection was recovered
	var out adjudicateOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	var rescued bool
	for _, b := range out.Confirmed {
		if strings.Contains(b.Desc, "deploy.py") {
			rescued = true
		}
	}
	if !rescued {
		t.Errorf("the escalated finding was not recovered: %+v", out)
	}

	// 6. the audit says HOW it was judged, and against what
	var esc *run.LLMCallData
	for i := range a.events {
		if a.events[i].Evidence == run.EvidenceSandbox {
			esc = &a.events[i]
		}
	}
	if esc == nil {
		t.Fatal("no llm_call recorded with evidence=sandbox: a replayer cannot tell how this verdict was reached")
	}
	if esc.Model != "codex/gpt-5.6-sol" {
		t.Errorf("escalation model = %q, want the escalation judge's own model", esc.Model)
	}
	if esc.TreeHash != tree.TreeHash || esc.BaseSHA != "base-sha" || esc.HeadSHA != "head-sha" {
		t.Errorf("escalation row does not content-address its evidence: %+v", esc)
	}
}

// Materializing a sandbox costs real time (540 files on this repo). A run whose candidates
// are all confirmed, or all local, must never pay it - so the escalation is resolved lazily,
// at most once, and only when a cross-file rejection actually needs a second opinion.
func TestEscalationIsResolvedLazilyAndOnce(t *testing.T) {
	build := func(real bool) (*int, Deps) {
		calls := new(int)
		return calls, Deps{
			Judge: &scriptedJudge{real: real},
			Escalate: func(context.Context, run.Snapshot, *workflow.Node) (*Escalation, error) {
				*calls++
				return &Escalation{Judge: &scriptedJudge{real: true}, Model: "codex/x", Effort: "medium", Evidence: run.EvidenceSandbox}, nil
			},
		}
	}
	// TWO cross-file candidates: with one, resolving once and resolving per candidate are
	// indistinguishable, and the memoization assertion would pass either way.
	twoCrossFile := run.Snapshot{RunID: "mrv-lazy", Iteration: 1, Findings: []run.Finding{
		{IssueText: "server.go disagrees with scripts/deploy.py", File: "server.go", Line: 1},
		{IssueText: "server.go also contradicts scripts/deploy.py elsewhere", File: "server.go", Line: 2},
	}}
	exec := func(d Deps) {
		t.Helper()
		reg, err := New(d)
		if err != nil {
			t.Fatalf("registry: %v", err)
		}
		ex, _ := reg.Executor(MatchThenAdjudicate)
		if _, err := ex.Execute(context.Background(), machine.ExecInput{
			Snap: twoCrossFile, Node: adjNode, Diff: machine.Diff{Text: escalationDiff}, StartIndex: 0, Audit: (&audits{}).fn}); err != nil {
			t.Fatalf("execute: %v", err)
		}
	}

	confirmed, d := build(true) // nothing is rejected: never resolved
	exec(d)
	if *confirmed != 0 {
		t.Errorf("escalation resolved %d times on a run with no rejections, want 0", *confirmed)
	}

	rejected, d2 := build(false) // two cross-file rejections: a per-candidate resolve would be 2
	exec(d2)
	if *rejected != 1 {
		t.Errorf("escalation resolved %d times, want exactly 1 (memoized across candidates)", *rejected)
	}
}

// A provider that fails must not take the run down: the finding is kept for a human, which is
// what happens for any escalation that cannot produce an answer.
func TestEscalationProviderErrorKeepsTheFinding(t *testing.T) {
	reg, err := New(Deps{
		Judge: &scriptedJudge{real: false},
		Escalate: func(context.Context, run.Snapshot, *workflow.Node) (*Escalation, error) {
			return nil, errSandboxUnavailable
		},
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	ex, _ := reg.Executor(MatchThenAdjudicate)
	raw, err := ex.Execute(context.Background(), machine.ExecInput{
		Snap: escalationSnap(), Node: adjNode, Diff: machine.Diff{Text: escalationDiff}, StartIndex: 0, Audit: (&audits{}).fn})
	if err != nil {
		t.Fatalf("a failed escalation provider must not fail the run: %v", err)
	}
	var out adjudicateOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	var kept bool
	for _, b := range out.Confirmed {
		if strings.Contains(b.Desc, "deploy.py") && b.Verdict == run.VerdictCheckedButUnverified {
			kept = true
		}
	}
	if !kept {
		t.Errorf("want the finding kept as checked_but_unverified: %+v", out)
	}
}

var errSandboxUnavailable = errors.New("sandbox unavailable")

// A provider may decide there is nothing to escalate to - no agentic judge configured, or the
// operator turned it off for this run. That is not a failure: the first arm's rejection stands.
func TestEscalationProviderReturningNilLeavesTheRejection(t *testing.T) {
	reg, err := New(Deps{
		Judge:    &scriptedJudge{real: false},
		Escalate: func(context.Context, run.Snapshot, *workflow.Node) (*Escalation, error) { return nil, nil },
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	ex, _ := reg.Executor(MatchThenAdjudicate)
	raw, err := ex.Execute(context.Background(), machine.ExecInput{
		Snap: escalationSnap(), Node: adjNode, Diff: machine.Diff{Text: escalationDiff}, StartIndex: 0, Audit: (&audits{}).fn})
	if err != nil {
		t.Fatal(err)
	}
	var out adjudicateOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Rejected) != 2 || len(out.Confirmed) != 0 {
		t.Errorf("no escalation available: both rejections stand, got confirmed=%d rejected=%d", len(out.Confirmed), len(out.Rejected))
	}
}

// The finding that motivated the widened trigger: code changed on the branch contradicts a
// document the branch never touched. Filtering the trigger on the diff means the named file is
// absent, the finding looks local, and it is never escalated - which is what happened to the
// five-versus-eight lens finding in four consecutive runs.
func TestEscalatesAClaimAgainstAnUnchangedFile(t *testing.T) {
	esc := &scriptedJudge{real: true}
	reg, err := New(Deps{
		Judge: &scriptedJudge{real: false},
		Escalate: func(context.Context, run.Snapshot, *workflow.Node) (*Escalation, error) {
			return &Escalation{Judge: esc, Model: "codex/x", Effort: "medium", Evidence: run.EvidenceSandbox}, nil
		},
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	ex, _ := reg.Executor(MatchThenAdjudicate)
	// server.go IS in escalationDiff; docs/unchanged-guide.md is NOT in it at all
	snap := run.Snapshot{RunID: "mrv-unchanged", Iteration: 1, Findings: []run.Finding{
		{IssueText: "server.go requires eight but docs/unchanged-guide.md still says five", File: "server.go", Line: 1},
	}}
	raw, err := ex.Execute(context.Background(), machine.ExecInput{
		Snap: snap, Node: adjNode, Diff: machine.Diff{Text: escalationDiff}, StartIndex: 0, Audit: (&audits{}).fn})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(esc.calls) != 1 {
		t.Fatalf("escalation ran %d times; a claim against an unchanged file must still escalate", len(esc.calls))
	}
	var out adjudicateOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Confirmed) != 1 || len(out.Rejected) != 0 {
		t.Errorf("the second opinion should have recovered it: confirmed=%d rejected=%d", len(out.Confirmed), len(out.Rejected))
	}
}
