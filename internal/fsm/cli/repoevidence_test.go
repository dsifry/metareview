package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/sandbox"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// Issue #146: the production git seams for the repository-side covering-test search.
// The seams run against a real git repository; only the judge is absent, because the
// search never talks to one.

// repoHarness commits a covering spec that exists at HEAD but is NOT touched by the
// branch diff — the exact shape the diff-only search cannot see.
func repoHarness(t *testing.T) (*harness, *ctxDeps, run.Snapshot) {
	t.Helper()
	h := newHarness(t)
	spec := "RSpec.describe TopicEmbed do\n  expect(post.topic.category).to eq(host.category)\nend\n"
	if err := os.MkdirAll(filepath.Join(h.root, "spec", "models"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, "spec", "models", "topic_embed_spec.rb"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "add", "-A")
	git(t, h.root, "commit", "-q", "-m", "spec")
	base := git(t, h.root, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(h.root, "app.rb"), []byte("def category_for(eh)\n  eh.try(:category_id)\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, h.root, "add", "-A")
	git(t, h.root, "commit", "-q", "-m", "app change")
	head := git(t, h.root, "rev-parse", "HEAD")
	c := &ctxDeps{ctx: context.Background(), deps: h.deps, cwd: h.root}
	return h, c, run.Snapshot{RunID: "mrv-repo", BaseSHA: base, Head: head}
}

func gapFinding() run.Finding {
	return run.Finding{File: "app.rb", Line: 1,
		IssueText: "per-host category assignment has no test verifying an embedded topic's category"}
}

// The wired RepoSearchFunc reads the pinned head through real git and finds the
// out-of-diff covering spec. A mock scenario disables it.
func TestRepoSearchSeamReadsThePinnedHead(t *testing.T) {
	h, c, snap := repoHarness(t)
	search := c.repoSearch(h.root, nil, judgeReal)
	if search == nil {
		t.Fatal("a real-judge run must wire the repo search")
	}
	ev, err := search(c.ctx, snap, gapFinding())
	if err != nil {
		t.Fatalf("repoSearch: %v", err)
	}
	if !ev.Ran || len(ev.Evidence) != 1 || ev.Evidence[0].Path != "spec/models/topic_embed_spec.rb" {
		t.Fatalf("evidence = %+v, ran = %v; want the covering spec", ev.Evidence, ev.Ran)
	}
	if !strings.Contains(ev.Content["spec/models/topic_embed_spec.rb"], "expect(post.topic.category)") {
		t.Errorf("content missing the covering assertion: %q", ev.Content)
	}
	// A mock scenario keeps the search off: mock verdicts are fixtures.
	if c.repoSearch(h.root, nil, judgeNone) != nil {
		t.Error("a mock-judge run must not wire the repo search")
	}
}

// The escalation tree carries the repo evidence paths for gap-claim findings, or the
// second opinion would get worse evidence than the primary arm.
func TestEscalationForCarriesRepoEvidencePaths(t *testing.T) {
	h, c, snap := repoHarness(t)
	c.escalate = true
	c.deps.CodexExec = func(_ context.Context, dir string, _ []string, _ string) ([]byte, int, error) {
		return []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"{\"reasoning\":\"r\",\"is_real\":true,\"confidence\":0.95}"}}` + "\n" +
			`{"type":"turn.completed","usage":{"input_tokens":9,"cached_input_tokens":1,"output_tokens":2,"reasoning_output_tokens":0}}` + "\n"), 0, nil
	}
	snap.Findings = []run.Finding{gapFinding()}
	esc, err := c.escalationFor(h.root)(c.ctx, snap, &workflow.Node{Model: "codex/gpt-5.6-sol", Effort: "medium"})
	if err != nil {
		t.Fatalf("escalationFor: %v", err)
	}
	if esc == nil {
		t.Fatal("want an escalation for a codex judge")
	}
	// The tree hash changes when the spec joins, so verify content, not just a hash:
	// read the materialized file back from the recorded root.
	// the call is exercised for the tree; the verdict is a fixture, so the return is
	// deliberately discarded — assigned to _ to keep the linter honest about it.
	_, _ = esc.Judge.Call(c.ctx, judge.Request{Kind: judge.KindAdjudicate, Model: esc.Model, Effort: esc.Effort,
		Input: judge.AdjudicateInput{Diff: "d", Candidate: gapFinding(), Sandbox: true}})
	spec := filepath.Join(esc.Root, sandbox.Head, "spec", "models", "topic_embed_spec.rb")
	body, err := os.ReadFile(spec)
	if err != nil {
		t.Fatalf("escalation tree is missing the repo evidence spec %s: %v", spec, err)
	}
	if !strings.Contains(string(body), "expect(post.topic.category)") {
		t.Errorf("materialized spec is wrong: %q", body)
	}
}

// git grep's contract at the seam: exit 1 is "no matches" (no error, no paths), any other
// nonzero exit is a failure, and blank lines in the output are skipped.
func TestGrepHeadSeparatesNoMatchesFromFailure(t *testing.T) {
	h, c, snap := repoHarness(t)
	grep := c.grepHead(c.ctx, h.root, snap.Head)
	if paths, err := grep("zzz_no_such_token_zzz"); err != nil || len(paths) != 0 {
		t.Errorf("no matches = %v, %v; want nil, nil (exit 1 is not a failure)", paths, err)
	}
	if paths, err := grep("topic"); err != nil || len(paths) != 1 || paths[0] != "spec/models/topic_embed_spec.rb" {
		t.Errorf("matches = %v, %v; want the spec path", paths, err)
	}
	// blank lines in the output are skipped, not returned as paths
	realExec := c.deps.Exec
	c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		return []byte("\n" + snap.Head + ":spec/models/topic_embed_spec.rb\n\n"), nil, 0, nil
	}
	if paths, err := grep("topic"); err != nil || len(paths) != 1 {
		t.Errorf("blank-line handling = %v, %v; want one path", paths, err)
	}
	// any other nonzero exit is a failure the caller must fail open on
	c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		return nil, nil, 128, nil
	}
	if _, err := grep("topic"); err == nil {
		t.Error("exit 128 must be an error, not an empty result")
	}
	// a transport error is an error too
	c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		return nil, nil, 0, context.Canceled
	}
	if _, err := grep("topic"); err == nil {
		t.Error("a transport error must surface")
	}
	c.deps.Exec = realExec
}

// A failing git seam fails open in repoEvidencePaths: no paths, escalation stays up.
func TestRepoEvidencePathsFailOpen(t *testing.T) {
	h, c, snap := repoHarness(t)
	c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		return nil, nil, 128, nil
	}
	snap.Findings = []run.Finding{gapFinding()}
	if paths := c.repoEvidencePaths(c.ctx, h.root, snap); len(paths) != 0 {
		t.Errorf("paths = %v; want none on a failed search", paths)
	}
}

// grepHead reads NUL-separated raw output: a filename with spaces survives, absence
// (exit 1) stays distinct from failure, and a transport error surfaces.
func TestGrepHeadNulSeparated(t *testing.T) {
	h, c, snap := repoHarness(t)
	grep := c.grepHead(c.ctx, h.root, snap.Head)
	if paths, err := grep("zzz_no_such_token_zzz"); err != nil || len(paths) != 0 {
		t.Errorf("no matches = %v, %v; want nil, nil", paths, err)
	}
	if paths, err := grep("topic"); err != nil || len(paths) != 1 || paths[0] != "spec/models/topic_embed_spec.rb" {
		t.Errorf("matches = %v, %v", paths, err)
	}
	realExec := c.deps.Exec
	c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		return []byte(snap.Head + ":spec/with space.rb\x00"), nil, 0, nil
	}
	if paths, err := grep("topic"); err != nil || len(paths) != 1 || paths[0] != "spec/with space.rb" {
		t.Errorf("NUL-separated grep output = %v, %v; want the spaced path", paths, err)
	}
	c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		return nil, nil, 128, nil
	}
	if _, err := grep("topic"); err == nil {
		t.Error("exit 128 must be an error, not an empty result")
	}
	c.deps.Exec = func(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
		return nil, nil, 0, context.Canceled
	}
	if _, err := grep("topic"); err == nil {
		t.Error("a transport error must surface")
	}
	c.deps.Exec = realExec
}
