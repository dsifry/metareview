package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/mockai"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/sandbox"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// The provider is exercised against a real git repository with a real diff, and only the
// subprocess is faked. It is the last seam before production: everything below it - git,
// materialization, the judge router - is the real implementation.
func escalationHarness(t *testing.T) (*harness, *ctxDeps, run.Snapshot, *[]string) {
	t.Helper()
	h := newHarness(t)
	// a tracked file that exists at HEAD but is NOT in base..head, so it can only reach the
	// evidence tree by being named in a finding
	if err := os.WriteFile(filepath.Join(h.root, "notes.md"), []byte("five lenses\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "add", "notes.md")
	git(t, h.root, "commit", "-q", "-m", "notes")
	base := git(t, h.root, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(h.root, "f.go"), []byte("package f\n\nfunc Added() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, "new file.go"), []byte("package f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "add", "-A")
	git(t, h.root, "commit", "-q", "-m", "change")
	head := git(t, h.root, "rev-parse", "HEAD")

	ranIn := &[]string{}
	h.deps.CodexExec = func(_ context.Context, dir string, _ []string, _ string) ([]byte, int, error) {
		*ranIn = append(*ranIn, dir)
		return []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"{\"reasoning\":\"r\",\"is_real\":true,\"confidence\":0.95}"}}` + "\n" +
			`{"type":"turn.completed","usage":{"input_tokens":9,"cached_input_tokens":1,"output_tokens":2,"reasoning_output_tokens":0}}` + "\n"), 0, nil
	}
	c := &ctxDeps{ctx: context.Background(), deps: h.deps, cwd: h.root}
	return h, c, run.Snapshot{RunID: "mrv-esc", BaseSHA: base, Head: head}, ranIn
}

func TestEscalationForBuildsAConfinedSandboxJudge(t *testing.T) {
	h, c, snap, ranIn := escalationHarness(t)
	esc, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/gpt-5.6-sol", Effort: "medium"})
	if err != nil {
		t.Fatalf("escalationFor: %v", err)
	}
	if esc == nil {
		t.Fatal("want an escalation for a codex judge")
	}
	if esc.Evidence != run.EvidenceSandbox || esc.TreeHash == "" || esc.BaseSHA != snap.BaseSHA || esc.HeadSHA != snap.Head {
		t.Fatalf("escalation does not describe its evidence: %+v", esc)
	}
	if esc.Model != "codex/gpt-5.6-sol" || esc.Effort != "medium" {
		t.Errorf("escalation must use the node's judge: %q %q", esc.Model, esc.Effort)
	}

	// the judge really is confined, and the sandbox really holds both sides
	if _, err := esc.Judge.Call(context.Background(), judge.Request{
		Kind: judge.KindAdjudicate, Model: esc.Model, Effort: esc.Effort,
		Input: judge.AdjudicateInput{Diff: "d", Candidate: run.Finding{IssueText: "x", File: "f.go"}},
	}); err != nil {
		t.Fatalf("judge call: %v", err)
	}
	if len(*ranIn) != 1 {
		t.Fatalf("codex ran %d times, want 1", len(*ranIn))
	}
	dir := (*ranIn)[0]
	if dir == "" || strings.HasPrefix(dir, h.root) {
		t.Errorf("judge ran in %q: the sandbox must be outside the repository it is judging", dir)
	}
	for _, rel := range []string{
		filepath.Join(sandbox.Head, "f.go"),
		filepath.Join(sandbox.Base, "f.go"),
		filepath.Join(sandbox.Head, "new file.go"), // a path with a space survives -z listing
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("sandbox is missing %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, sandbox.Base, "new file.go")); !os.IsNotExist(err) {
		t.Error("a file added on the branch must have no base side")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Error("the sandbox must not carry .git")
	}
}

// An HTTP judge cannot read a tree, so escalating to it re-asks the same question with the
// same evidence at twice the cost. That is "unavailable", not an error.
func TestEscalationForSkipsNonAgenticJudges(t *testing.T) {
	h, c, snap, _ := escalationHarness(t)
	for _, node := range []*workflow.Node{
		{Model: "claude-opus-4-5", Effort: "medium"},
		{Model: "gpt-5.2", Effort: "medium"},
		nil,
	} {
		esc, err := c.escalationFor(h.root)(context.Background(), snap, node)
		if err != nil || esc != nil {
			t.Errorf("node %+v: got (%v, %v), want (nil, nil)", node, esc, err)
		}
	}
}

// A run with no changed files has nothing to materialize.
func TestEscalationForWithNoChangedFiles(t *testing.T) {
	h, c, snap, _ := escalationHarness(t)
	snap.BaseSHA = snap.Head
	esc, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"})
	if err != nil || esc != nil {
		t.Errorf("got (%v, %v), want (nil, nil) when the range is empty", esc, err)
	}
}

// A temp-dir failure must surface as an error, so the caller keeps the finding rather than
// silently treating a broken sandbox as agreement with the rejection.
func TestEscalationForSurfacesTempDirFailure(t *testing.T) {
	h, c, snap, _ := escalationHarness(t)
	c.deps.TempDir = func(string) (string, error) { return "", os.ErrPermission }
	if _, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"}); err == nil {
		t.Error("want an error when the sandbox root cannot be created")
	}
}

// Every way the sandbox can fail to be built must surface as an error, so the caller keeps
// the finding as checked_but_unverified. Silently returning "unavailable" would let a broken
// sandbox read as agreement with the cheap arm's rejection - a finding dropped by an outage.
func TestEscalationForSurfacesGitFailures(t *testing.T) {
	t.Run("listing the changed files fails", func(t *testing.T) {
		h, c, snap, _ := escalationHarness(t)
		c.deps.Exec = func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
			return nil, nil, 1, os.ErrPermission
		}
		if _, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"}); err == nil {
			t.Error("want the git failure surfaced")
		}
	})

	t.Run("reading a file at a revision fails", func(t *testing.T) {
		h, c, snap, _ := escalationHarness(t)
		realExec := c.deps.Exec
		c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
			if len(args) > 0 && args[0] == "show" {
				return nil, nil, 1, os.ErrPermission
			}
			return realExec(ctx, dir, env, args...)
		}
		if _, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"}); err == nil {
			t.Error("want the git show failure surfaced")
		}
	})

	t.Run("a path that escapes the sandbox is refused", func(t *testing.T) {
		h, c, snap, _ := escalationHarness(t)
		realExec := c.deps.Exec
		c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
			if len(args) > 1 && args[0] == "diff" {
				return []byte("../../escape.go\x00"), nil, 0, nil // git would not, but the list is data
			}
			return realExec(ctx, dir, env, args...)
		}
		if _, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"}); err == nil {
			t.Error("want materialization to refuse an escaping path")
		}
	})
}

// The escalation judge is built from the same configuration as the primary, so a bad base URL
// fails here too - and must be reported rather than swallowed into "no escalation available".
func TestEscalationForSurfacesJudgeConstructionFailure(t *testing.T) {
	h, c, snap, _ := escalationHarness(t)
	h.env[EnvAnthropicURL] = "http://not-https.example.com"
	if _, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"}); err == nil {
		t.Error("want the judge construction failure surfaced")
	}
}

// Escalation is OFF unless asked for. It shipped default-on and a nine-reviewer pass found it
// inert and harmful: the escalated judge received the identical prompt (nothing tells it the
// sandbox exists), the materialized evidence was whitespace-trimmed so line numbers shifted, a
// git failure returned (nil, nil) which the caller read as "no escalation available" and
// recorded the finding as a hallucination, and a duplicate destination hit the tree's own 0444
// mode and killed escalation for the whole run via the cached sync.Once. Until those are fixed
// a feature that silently converts real findings into hallucinations is worse than none.
func TestEscalationIsOffUnlessRequested(t *testing.T) {
	h, c, _, _ := escalationHarness(t)
	if c.escalation(h.root, nil, judgeReal) != nil {
		t.Error("escalation must be OFF by default")
	}
	t.Run("--escalate turns it on", func(t *testing.T) {
		c.escalate = true
		defer func() { c.escalate = false }()
		if c.escalation(h.root, nil, judgeReal) == nil {
			t.Error("--escalate must enable it")
		}
	})
	t.Run("mock runs never escalate", func(t *testing.T) {
		c.escalate = true
		defer func() { c.escalate = false }()
		if c.escalation(h.root, &mockai.Scenario{}, judgeReal) != nil {
			t.Error("a mock run's verdicts are fixtures; a second opinion is meaningless")
		}
	})
	t.Run("unjudged runs never escalate", func(t *testing.T) {
		c.escalate = true
		defer func() { c.escalate = false }()
		for _, m := range []judgeMode{judgeNone} {
			if c.escalation(h.root, nil, m) != nil {
				t.Errorf("mode %v must not escalate", m)
			}
		}
	})
}

// A finding can turn on a file the branch never touched. The sandbox must carry it, or the
// escalated judge is handed the same evidence gap that caused the rejection.
func TestSandboxCarriesFilesFindingsNameEvenWhenUnchanged(t *testing.T) {
	h, c, snap, ranIn := escalationHarness(t)
	// seed.txt exists at HEAD and is NOT in base..head (the harness commits it first)
	snap.Findings = []run.Finding{{
		File: "f.go", Line: 1,
		IssueText: "f.go now requires eight, but notes.md still documents five",
	}}
	esc, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x", Effort: "medium"})
	if err != nil || esc == nil {
		t.Fatalf("escalationFor: %v %v", esc, err)
	}
	if _, err := esc.Judge.Call(context.Background(), judge.Request{
		Kind: judge.KindAdjudicate, Model: esc.Model, Effort: esc.Effort,
		Input: judge.AdjudicateInput{Diff: "d", Candidate: run.Finding{IssueText: "x", File: "f.go"}},
	}); err != nil {
		t.Fatalf("judge call: %v", err)
	}
	dir := (*ranIn)[0]
	if _, err := os.Stat(filepath.Join(dir, sandbox.Head, "notes.md")); err != nil {
		t.Errorf("an unchanged file a finding names must be in the evidence tree: %v", err)
	}
}

// A finding naming a path the branch already changed adds nothing to the evidence tree.
func TestReferencedByFindingsSkipsAlreadyChangedPaths(t *testing.T) {
	_, c, _, _ := escalationHarness(t)
	snap := run.Snapshot{Findings: []run.Finding{
		{File: "f.go", IssueText: "f.go contradicts notes.md"},
		{File: "f.go", IssueText: "and again notes.md"},
	}}
	got := c.referencedByFindings(snap, []string{"f.go", "notes.md"})
	if len(got) != 0 {
		t.Errorf("got %v, want none: both paths are already in the changed set", got)
	}
}

// The flag has to survive a mutation to boolFlags and to the wiring line in run: a knob that
// nothing pins is a knob that can be deleted silently, which is the defect class this whole
// change is responding to. It also pins that the retired --no-escalate is not quietly accepted,
// since a driver that keeps passing it would otherwise believe it had disabled something.
func TestEscalateFlagIsPinned(t *testing.T) {
	p, err := parseArgs([]string{"state", "--escalate"})
	if err != nil {
		t.Fatalf("--escalate must parse as a bool flag: %v", err)
	}
	if !p.bools["escalate"] {
		t.Error("--escalate must set bools[escalate]")
	}
	if p2, err := parseArgs([]string{"state"}); err != nil || p2.bools["escalate"] {
		t.Errorf("escalate must default false, got %v (%v)", p2.bools["escalate"], err)
	}
	if _, err := parseArgs([]string{"state", "--no-escalate"}); err == nil {
		t.Error("--no-escalate was retired; accepting it would let a driver think it disabled escalation")
	}
	// The agent contract has to name the knob it now requires.
	if !strings.Contains(AgentPrompt, "--escalate") || strings.Contains(AgentPrompt, "--no-escalate") {
		t.Error("the agent prompt must document --escalate and must not still document --no-escalate")
	}
}
