package kind

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/sandbox"
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
		Escalate: &Escalation{
			Judge: confined, Model: "codex/gpt-5.6-sol", Effort: "medium",
			Evidence: run.EvidenceSandbox, TreeHash: tree.TreeHash,
			BaseSHA: tree.BaseSHA, HeadSHA: tree.HeadSHA,
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
