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
	"github.com/dsifry/metareview/internal/mutation"
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
	// Both shapes matter, and the second is the one that actually occurs: gate.RealExec converts
	// an *exec.ExitError into (code, nil), so the production form of "git failed" is a nonzero
	// code with a nil error. An earlier version of this test used os.ErrPermission in both fakes -
	// a channel git failures never use - so it asserted the invariant while never exercising the
	// path that breaks it.
	for _, shape := range []struct {
		name string
		fail func() ([]byte, []byte, int, error)
	}{
		{"nonzero exit with nil error (the production shape)", func() ([]byte, []byte, int, error) { return []byte("fatal: bad revision"), nil, 128, nil }},
		{"transport error", func() ([]byte, []byte, int, error) { return nil, nil, 1, os.ErrPermission }},
	} {
		t.Run("listing the changed files fails: "+shape.name, func(t *testing.T) {
			h, c, snap, _ := escalationHarness(t)
			c.deps.Exec = func(_ context.Context, _ string, _ []string, args ...string) ([]byte, []byte, int, error) {
				return shape.fail()
			}
			if _, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"}); err == nil {
				t.Error("want the git failure surfaced, not a silent (nil, nil) that reads as agreement")
			}
		})

		t.Run("reading a file at a revision fails: "+shape.name, func(t *testing.T) {
			h, c, snap, _ := escalationHarness(t)
			realExec := c.deps.Exec
			c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
				if len(args) > 0 && (args[0] == "ls-tree" || args[0] == "cat-file") {
					return shape.fail()
				}
				return realExec(ctx, dir, env, args...)
			}
			if _, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"}); err == nil {
				t.Error("want the file-read failure surfaced")
			}
		})
	}

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
	// notes.md exists at HEAD and is NOT in base..head - the harness commits it before taking the
	// base SHA, so it can only reach the evidence tree by being named in a finding
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

// The tree has to hold the file, not an approximation of it. showFile went through ctxDeps.git,
// which TrimSpaces its output - a helper written for SHAs and ref names. Every materialized file
// lost its trailing newline, and one with leading blank lines lost those too, moving every
// declaration below them up by as many lines. The judge is pointed at Finding.Line, taken from the
// diff's post-image numbering, so it reads the wrong line of a file that matches neither revision.
func TestSandboxMaterializesExactBytes(t *testing.T) {
	h, c, snap, _ := escalationHarness(t)
	// Leading and trailing blank lines are the whole point: they are what a trim destroys.
	body := "\n\n// leading blanks are significant\npackage f\n\nfunc Added() {}\n\n\n"
	if err := os.WriteFile(filepath.Join(h.root, "f.go"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "add", "f.go")
	git(t, h.root, "commit", "-q", "-m", "whitespace that matters")
	snap.Head = git(t, h.root, "rev-parse", "HEAD")

	esc, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"})
	if err != nil || esc == nil {
		t.Fatalf("escalationFor: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(c.sandboxRoots[len(c.sandboxRoots)-1], "head", "f.go"))
	if err != nil {
		t.Fatalf("read materialized file: %v", err)
	}
	if string(got) != body {
		t.Errorf("materialized bytes differ from the file at that revision:\n got %q\nwant %q", got, body)
	}
}

// A path a judge's prose invented is untrusted input, and one malformed string must not cost the
// run its escalation. sandbox.Materialize refuses an escaping path and that refusal is fatal to
// the batch, while escalationFor's error is cached under a sync.Once - so without filtering here,
// a finding whose text contained "../../etc/passwd" would turn every cross-file rejection in the
// run into checked_but_unverified.
func TestFindingPathsThatEscapeTheTreeAreSkippedNotFatal(t *testing.T) {
	h, c, snap, _ := escalationHarness(t)
	snap.Findings = []run.Finding{{
		File: "f.go",
		// ../secrets.env and ./../../x.go both escape the tree once cleaned, and
		// AllReferencedPaths does return them - verified rather than assumed.
		IssueText: "this mirrors ../secrets.env and ./../../x.go and ./notes.md",
	}}
	esc, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"})
	if err != nil {
		t.Fatalf("a malformed path in judge prose must not fail the batch: %v", err)
	}
	if esc == nil {
		t.Fatal("escalation was not built")
	}
	// The legitimate relative path is still carried.
	if _, err := os.Stat(filepath.Join(c.sandboxRoots[len(c.sandboxRoots)-1], "head", "notes.md")); err != nil {
		t.Errorf("a well-formed referenced path must still reach the tree: %v", err)
	}
}

// The flag-to-field mapping, which was unpinned end to end: every escalation test set c.escalate
// directly, so rewriting the assignment in Run to a constant made the documented flag a no-op
// with the whole package still green. This drives the real argument parser into the real
// assignment, so the two cannot drift.
func TestEscalateFlagReachesTheField(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"state"}, false},
		{[]string{"state", "--escalate"}, true},
	} {
		p, err := parseArgs(tc.args[1:])
		if err != nil {
			t.Fatalf("parseArgs(%v): %v", tc.args, err)
		}
		c := newCtxDeps(context.Background(), Deps{}, "/repo")
		c.applyGlobalFlags(p)
		if c.escalate != tc.want {
			t.Errorf("%v: c.escalate = %v, want %v", tc.args, c.escalate, tc.want)
		}
	}
}

// The context the executor hands EscalateFunc has to govern the work it triggers. Every git call
// inside the closure went through ctxDeps.git, which passes the invocation's context, so the
// executor's context governed nothing: materializing this repository's own branch is ~540 files
// and so ~1,000 git subprocesses, and a cancelled or timed-out node could not stop any of them.
func TestEscalationHonoursTheContextItIsGiven(t *testing.T) {
	h, c, snap, _ := escalationHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var sawCancelled bool
	real := c.deps.Exec
	c.deps.Exec = func(got context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		if got.Err() != nil {
			sawCancelled = true
		}
		return real(got, dir, env, args...)
	}
	_, _ = c.escalationFor(h.root)(ctx, snap, &workflow.Node{Model: "codex/x"})
	if !sawCancelled {
		t.Error("git ran with a context that was not the one the executor passed, so cancelling a node cannot stop the ~1,000 subprocesses it starts")
	}
}

// Routing strips the codex/ prefix case-insensitively, so this guard has to match the same way or
// a run started with --judge-model Codex/gpt-5.6-sol reaches the Codex CLI while escalation
// decides it is not agentic and silently declines. Every existing fixture used a lowercase model,
// so dropping strings.ToLower left the suite green.
func TestEscalationMatchesTheCodexPrefixCaseInsensitively(t *testing.T) {
	h, c, snap, _ := escalationHarness(t)
	for _, model := range []string{"codex/gpt-5.6-sol", "Codex/gpt-5.6-sol", "CODEX/gpt-5.6-sol"} {
		esc, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: model})
		if err != nil {
			t.Fatalf("%s: %v", model, err)
		}
		if esc == nil {
			t.Errorf("model %q routes to the Codex CLI but escalation declined it as non-agentic", model)
		}
	}
	// A non-codex model is still declined: an HTTP judge cannot read a tree.
	esc, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "gpt-5.2"})
	if err != nil || esc != nil {
		t.Errorf("a non-agentic judge must not be escalated to: esc=%v err=%v", esc, err)
	}
}

// A tree with no files in it is not evidence: escalating into an empty directory records
// evidence=sandbox and a well-formed TreeHash over nothing, so the audit says the judge read a
// materialized tree when it read an empty folder. Tree.Files existed only to be incremented -
// no production caller read it - and this is what makes it load-bearing.
func TestEscalationRefusesAnEmptyEvidenceTree(t *testing.T) {
	h, c, snap, _ := escalationHarness(t)
	real := c.deps.Exec
	c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		// Every path reports as absent at both revisions: ls-tree succeeds and lists nothing.
		if len(args) > 0 && args[0] == "ls-tree" {
			return nil, nil, 0, nil
		}
		return real(ctx, dir, env, args...)
	}
	if _, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"}); err == nil {
		t.Error("an empty evidence tree must be an error, not a sandbox the judge is pointed at")
	}
}

// The two failure paths of the blob read itself: ls-tree says the file is there, and reading it
// then fails. Both must surface, for the same reason as every other sandbox failure - a tree
// missing a file it was supposed to carry is not evidence, and the caller must keep the finding
// rather than record a verdict against a partial tree.
func TestShowFileSurfacesBlobReadFailures(t *testing.T) {
	for _, shape := range []struct {
		name string
		fail func() ([]byte, []byte, int, error)
	}{
		{"nonzero exit", func() ([]byte, []byte, int, error) { return []byte("fatal: bad object"), nil, 128, nil }},
		{"transport error", func() ([]byte, []byte, int, error) { return nil, nil, 1, os.ErrPermission }},
	} {
		t.Run(shape.name, func(t *testing.T) {
			h, c, snap, _ := escalationHarness(t)
			real := c.deps.Exec
			c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
				// ls-tree still reports the file as present; only the blob read fails.
				if len(args) > 1 && args[0] == "cat-file" && args[1] == "blob" {
					return shape.fail()
				}
				return real(ctx, dir, env, args...)
			}
			if _, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"}); err == nil {
				t.Error("a file that ls-tree lists but cat-file cannot read must surface as an error")
			}
		})
	}
}

// The evidence trees are removed when the invocation ends. They are written read-only so a judge
// cannot edit its own evidence, which is also what makes removal need a chmod pass first: without
// it RemoveAll fails on the 0444 files and the tree survives, which is how one machine reached
// 1015 of them.
func TestRemoveSandboxesDeletesReadOnlyTrees(t *testing.T) {
	h, c, snap, _ := escalationHarness(t)
	if _, err := c.escalationFor(h.root)(context.Background(), snap, &workflow.Node{Model: "codex/x"}); err != nil {
		t.Fatalf("escalationFor: %v", err)
	}
	if len(c.sandboxRoots) == 0 {
		t.Fatal("no sandbox root was recorded")
	}
	root := c.sandboxRoots[0]
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("the tree should exist before removal: %v", err)
	}
	c.removeSandboxes()
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("the evidence tree survived removal: %v", err)
	}
	if len(c.sandboxRoots) != 0 {
		t.Error("the recorded roots must be cleared, or a second call would retry deleted paths")
	}

	// A root that is already gone - removed by hand, or by a previous call - must not stop the
	// rest being cleaned up. WalkDir reports the failure to the callback, which ignores it: the
	// point of this pass is best-effort chmod, and a missing tree needs no chmod.
	live := t.TempDir()
	c.sandboxRoots = []string{filepath.Join(t.TempDir(), "already-removed"), live}
	c.removeSandboxes()
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Errorf("a missing earlier root stopped the rest being removed: %v", err)
	}
}

// escapesTree decides which judge-authored paths may reach the evidence tree, so each way out is
// worth stating: an absolute path, a "." that names no file, and a ".." component - but NOT a name
// that merely begins with dots, which is an ordinary directory.
func TestEscapesTree(t *testing.T) {
	for _, tc := range []struct {
		path    string
		escapes bool
	}{
		{"internal/x.go", false},
		{"./internal/x.go", false},
		{"..dir/file.go", false},
		{"pkg/..name.go", false},
		{"../secrets.env", true},
		{"./../../x.go", true},
		{"a/../../b.go", true},
		{"/etc/shadow", true},
		{".", true},
		{"", true},
	} {
		if got := escapesTree(tc.path); got != tc.escapes {
			t.Errorf("escapesTree(%q) = %v, want %v", tc.path, got, tc.escapes)
		}
	}
}

// The CLI's half of mutation-verify: a real Verifier rooted at the repository, running the
// repository's own test command — which comes from the operator's environment or the language
// default, never from the agent whose fix is under review.
func TestVerifyPinsRunsAgainstTheRepository(t *testing.T) {
	h, c, _, _ := escalationHarness(t)
	// A tiny module with a test that genuinely pins a boundary.
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(h.root, rel)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// package f, matching the harness's existing f.go: two packages in one directory would not
	// build, and the verifier's baseline check would (correctly) refuse to conclude anything.
	write("go.mod", "module fixture\n\ngo 1.22\n")
	write("calc.go", "package f\n\nfunc Allow(n int) bool {\n\treturn n < 10\n}\n")
	write("calc_test.go", "package f\n\nimport \"testing\"\n\nfunc TestBoundary(t *testing.T) {\n\tif Allow(10) {\n\t\tt.Fatal(\"10 must not be allowed\")\n\t}\n}\n")

	verify := c.verifyPins(h.root, nil)
	if verify == nil {
		t.Fatal("a real run must get a verifier")
	}
	got, err := verify(context.Background(), []run.Pin{
		{File: "calc.go", From: "n < 10", To: "n <= 10", Test: "TestBoundary"},
		{File: "calc.go", From: "func Allow", To: "func allow", Test: "TestBoundary"},
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("one result per pin, got %d", len(got))
	}
	if !got[0].Proven {
		t.Errorf("a pinned boundary must verify: %s", got[0].Detail)
	}
	// Renaming the function breaks the build, which proves nothing about the test.
	if got[1].Proven || !strings.Contains(got[1].Detail, "compile") {
		t.Errorf("a non-compiling mutation must be refused as such: %+v", got[1])
	}
	// The pin travels through unchanged, so a finding can name it.
	if got[0].Pin.File != "calc.go" || got[0].Pin.Test != "TestBoundary" {
		t.Errorf("the pin must survive the round trip: %+v", got[0].Pin)
	}
}

// A mock run cannot prove anything: there is no real code to break and no real tests to run.
// Saying "unproven" is the truthful answer — a mock that reported proof would let a fixture stand
// in for evidence.
func TestVerifyPinsRefusesEverythingOnAMockRun(t *testing.T) {
	h, c, _, _ := escalationHarness(t)
	verify := c.verifyPins(h.root, &mockai.Scenario{})
	got, err := verify(context.Background(), []run.Pin{{File: "a.go", From: "x", To: "y", Test: "T"}})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(got) != 1 || got[0].Proven {
		t.Fatalf("a mock run must not prove anything: %+v", got)
	}
	if !strings.Contains(got[0].Detail, "mock run") {
		t.Errorf("the reason must say it was a mock: %q", got[0].Detail)
	}
	// With nothing claimed there is nothing to refuse, so a mock fix that declares no pins still
	// passes the gate.
	if empty, _ := verify(context.Background(), nil); len(empty) != 0 {
		t.Errorf("no pins means no results, got %+v", empty)
	}
}

// The test command is the operator's, never the agent's.
func TestTestCommandComesFromTheOperator(t *testing.T) {
	h, c, _, _ := escalationHarness(t)
	_ = h
	if got := strings.Join(c.testCommand(), " "); got != "go test ./..." {
		t.Errorf("default = %q", got)
	}
	h.env[EnvTestCmd] = "  make  check  "
	if got := strings.Join(c.testCommand(), "|"); got != "make|check" {
		t.Errorf("override = %q", got)
	}
}

// The CLI adapter must surface a verifier failure rather than swallow it. With no test command
// configured nothing can be checked, and reporting every pin as survived would blame the fixes
// for the harness's own misconfiguration.
func TestVerifyPinsSurfacesAVerifierFailure(t *testing.T) {
	h, c, _, _ := escalationHarness(t)
	h.env[EnvTestCmd] = " " // whitespace only: no command at all
	verify := c.verifyPins(h.root, nil)
	if _, err := verify(context.Background(), []run.Pin{{File: "f.go", From: "x", To: "y", Test: "T"}}); err == nil {
		t.Error("a verifier failure must reach the node, not be reported as nothing wrong")
	}
}

// The verifier's vocabulary and the schema's are separate on purpose — internal/mutation is a
// standalone tool that must not depend on the FSM's types — so the seam between them is exactly
// where a value can be lost. The mapping must be total: every outcome the verifier can return
// has a name here, and anything it cannot name becomes PinUnverifiable, never something that
// reads as a pass. If a fifth outcome is ever added to the verifier this test fails, which is
// the point: the translation is a decision, not a default.
func TestOutcomeForIsTotal(t *testing.T) {
	for verifier, schema := range map[mutation.Outcome]run.PinOutcome{
		mutation.PinProven:        run.PinProven,
		mutation.PinSurvived:      run.PinSurvived,
		mutation.PinMalformed:     run.PinMalformed,
		mutation.PinUnverifiable:  run.PinUnverifiable,
		mutation.Outcome("later"): run.PinUnverifiable,
		mutation.Outcome(""):      run.PinUnverifiable,
	} {
		got := outcomeFor(verifier)
		if got != schema {
			t.Errorf("outcomeFor(%q) = %q, want %q", verifier, got, schema)
		}
		if !got.Valid() {
			t.Errorf("outcomeFor(%q) produced %q, which the schema does not recognise", verifier, got)
		}
	}
	if outcomeFor(mutation.Outcome("later")) == run.PinProven {
		t.Error("an unrecognised outcome must never map to a pass")
	}
}
