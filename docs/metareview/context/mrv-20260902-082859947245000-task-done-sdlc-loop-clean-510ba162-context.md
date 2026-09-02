# metareview task-done context

Run ID: `mrv-20260902-082859947245000-task-done-sdlc-loop-clean-510ba162`

## Task

workflow: sdlc-loop-clean
version: 1
# Like sdlc-loop, but it does not stop when the ORIGINAL bugs are fixed: after every fix it RE-REVIEWS
# the (now fix-modified) diff and only reaches `done` when a fresh review finds nothing OR the adjudicator
# confirms none of the fresh candidates are real. This catches bugs a fix itself introduces — "review the
# code you just wrote". sdlc-loop terminates via `verify` (a still-present mutation check that the ORIGINAL
# bugs are gone), which can exit while a fix-introduced bug is still present; this workflow removes that
# early exit.
#
# It is a graph with ONE loop. The single framework rule that shapes it: the loop:true edge clears this
# iteration's provisional Findings/Confirmed at the boundary, and only a REVIEW node re-derives them
# afterward — so the loop's target must be a review node (`discover`), never `adjudicate`/`fix` (they would
# be handed an empty set). Consequences that follow purely from that rule:
#   * `discover` is the loop target, so a fresh iteration's authoritative re-review happens there.
#   * `recheck` is the loop-carrying state (exactly one terminal + the loop edge). It re-reviews the fix;
#     any non-clean re-review starts a fresh iteration at `discover`.
#   * The clean exit that actually FIRES is a fresh `discover` finding nothing (findings_empty) or the
#     `adjudicate`-tor confirming nothing real (confirmed_empty). Findings are provisional candidates; only a
#     review's emptiness or the adjudicator's verdict ends the run.
#   * NOTE: `recheck → done` (findings_empty) is a STRUCTURAL requirement of Validate's loop_terminal rule
#     (the loop-carrying state must carry exactly one outcome-bearing terminal), but it is DEAD at runtime
#     and never taken: findings accumulate within an iteration and are cleared only at the loop boundary
#     (Delta.Findings is json omitempty, so an empty re-review is a fold no-op — it does not clear the
#     findings `discover` already recorded this iteration). So `recheck` always sees a non-empty finding set
#     and always loops. Do not "optimize" this edge away (Validate rejects a loop state with no terminal)
#     and do not rely on it firing — the authoritative clean exit is the fresh `discover → done`.
vars:
  REVIEWER:     {default: claude-opus-5}
  JUDGE:        {required: true}
  REV_EFFORT:   {default: low}
  JUDGE_EFFORT: {required: true}
states: [discover, adjudicate, fix, recheck, done, failed]
transitions:                                  # ordered
  # findings_empty (AllFound-blind), NOT nothing_found: on the loop AllFound>0 (bugs already known), and
  # nothing_found REFUSES once bugs are known — a clean re-review would then match no edge and dead-end.
  # findings_empty reaches done whenever THIS review found no candidates, iteration 0 or later.
  - {from: discover,   to: done,       gate: findings_empty,    outcome: clean}   # a review found no candidates → bug-free
  - {from: discover,   to: adjudicate, gate: findings_nonempty}
  # confirmed_empty (this iteration's Confirmed == 0), NOT nothing_confirmed: once ANY bug has been
  # confirmed in the run, AllFound>0, so nothing_confirmed would fail (ERR_BUGS_KNOWN) and dead-end on the
  # common good case — a re-review that surfaces a candidate the adjudicator then rejects. confirmed_empty
  # is AllFound-blind, so that case correctly reaches done.
  - {from: adjudicate, to: done,       gate: confirmed_empty,   outcome: clean}   # adjudicator confirmed none real → bug-free
  - {from: adjudicate, to: fix,        gate: confirmed_nonempty}
  - {from: fix,        to: recheck,    gate: commit_exists}
  # recheck is the loop-carrying state: exactly one outcome-bearing terminal (→done) plus the loop edge.
  # This terminal satisfies Validate's loop_terminal rule but is DEAD at runtime (see header): findings
  # linger within an iteration, so recheck's finding set is never empty and this edge never fires.
  - {from: recheck,    to: done,       gate: findings_empty,    outcome: clean}   # STRUCTURAL terminal (never taken); real clean exit is discover→done
  - {from: recheck,    to: discover,   gate: findings_nonempty, loop: true}       # the re-review is not clean → new iteration re-discovers → adjudicate → fix → recheck …
nodes:
  discover:   {kind: review-lenses,        exec: subagent, model: $REVIEWER, effort: $REV_EFFORT}
  adjudicate: {kind: match-then-adjudicate, exec: fork,     model: $JUDGE, effort: $JUDGE_EFFORT}
  fix:        {kind: agent-edit}
  recheck:    {kind: review-lenses,        exec: subagent, model: $REVIEWER, effort: $REV_EFFORT}
# No `no_fixation_progress` here. That atom measures progress by bugs marked fixed via a still-present
# oracle (BugStatus), which this workflow has no node to emit — its termination signal is a clean re-review
# / adjudication, a different oracle. With no fixed-marking every bug stays in UnfixedAtEntry, so
# no_fixation_progress would stall the loop at its first boundary. max_iterations + budget bound the loop
# (Validate requires a looping workflow to carry one of them).
convergence:
  any: [{max_iterations: 8}, {budget: {tokens: 4000000}}]
repo_mode: advisory


## Git

- Base: `f785654630d864f11f98cc47fd73502a428bcefa`
- Head: `91afd700dce3610e9318865371337dce18d0b6f6`
- Branch: `fsm-iterate-until-clean`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `132633`
- Filtered diff bytes: `40268`
- Risk level: `none`
- Generated files excluded: docs/metareview/context/mrv-20260902-065946842006000-task-done-sdlc-loop-clean-510ba162-context.md, docs/metareview/context/mrv-20260902-081935946182000-task-done-sdlc-loop-clean-510ba162-context.md, docs/metareview/context/mrv-20260902-081939762800000-task-done-sdlc-loop-clean-510ba162-context.md, docs/metareview/reviews/mrv-20260902-065946842006000-task-done-sdlc-loop-clean-510ba162.md, docs/metareview/reviews/mrv-20260902-081935946182000-task-done-sdlc-loop-clean-510ba162.md, docs/metareview/reviews/mrv-20260902-081939762800000-task-done-sdlc-loop-clean-510ba162.md

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- CLAUDE.md
- internal/fsm/cli/cli_test.go
- internal/fsm/machine/machine_test.go
- internal/fsm/workflow/workflow_test.go
- workflows/embed_test.go
- workflows/sdlc-loop-clean.yaml

### Path Dispositions
- docs/metareview/context/mrv-20260902-065946842006000-task-done-sdlc-loop-clean-510ba162-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260902-081935946182000-task-done-sdlc-loop-clean-510ba162-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260902-081939762800000-task-done-sdlc-loop-clean-510ba162-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260902-065946842006000-task-done-sdlc-loop-clean-510ba162.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260902-081935946182000-task-done-sdlc-loop-clean-510ba162.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260902-081939762800000-task-done-sdlc-loop-clean-510ba162.md: generated (metareview generated review artifact excluded from source manifest)

### Manifest Blockers
No manifest blockers.

## Changed Files

- CLAUDE.md
- internal/fsm/cli/cli_test.go
- internal/fsm/machine/machine_test.go
- internal/fsm/workflow/workflow_test.go
- workflows/embed_test.go
- workflows/sdlc-loop-clean.yaml

## Diff

```diff
diff --git a/CLAUDE.md b/CLAUDE.md
index 7e34281..549a75e 100644
--- a/CLAUDE.md
+++ b/CLAUDE.md
@@ -59,6 +59,16 @@ metareview override list [--pending]
 ## Lifecycle Placement
 
 - Before implementing a plan or spec: review the artifact.
+- **After writing a fix or new code, review the code you just wrote — and iterate until it is bug-free —
+  BEFORE opening the PR.** Run `metareview fsm --workflow sdlc-loop-clean --base <ref>` on your own diff: it
+  discovers → adjudicates → fixes, then **re-reviews the fix at `recheck` and only stops when a fresh review
+  finds nothing**. A fix often introduces its own bug (a broadened matcher over-matches, a parsed value
+  overflows); the deterministic task-done gate and the external bots will not always catch it, but a fresh
+  adversarial review of the diff does. **Ordering matters for measurement:** opening the PR starts the
+  external reviewers (CodeRabbit/Bugbot), so our review must finish and the diff must be clean FIRST — then
+  the bots measure only the *residual* we missed (the metareview-first-then-bots recall yardstick). Running
+  our review after the PR opens confounds the two and pollutes the recall stats. (Reviews parallelize — fan
+  out lenses and run one loop per fix concurrently.)
 - After each small implementation chunk: run task-done.
 - After all child tasks for an epic are complete: run epic-ready.
 - Before opening, pushing, or merging a PR: run pr-ready.
diff --git a/internal/fsm/cli/cli_test.go b/internal/fsm/cli/cli_test.go
index 5cd5c24..aeb0d55 100644
--- a/internal/fsm/cli/cli_test.go
+++ b/internal/fsm/cli/cli_test.go
@@ -5,6 +5,7 @@ import (
 	"context"
 	"encoding/json"
 	"errors"
+	"fmt"
 	"io"
 	"net/http"
 	"os"
@@ -172,10 +173,13 @@ func TestUsageAndPrompt(t *testing.T) {
 	}
 	env := h.must(StatusOK, 0, "workflows")
 	list := env["workflows"].([]any)
-	if len(list) != 3 || list[1].(map[string]any)["name"] != "sdlc-loop" || len(list[1].(map[string]any)["states"].([]any)) != 6 {
+	if len(list) != 4 || list[1].(map[string]any)["name"] != "sdlc-loop" || len(list[1].(map[string]any)["states"].([]any)) != 6 {
 		t.Fatalf("workflows: %v", list)
 	}
-	if list[2].(map[string]any)["name"] != "sdlc-loop-proved" || len(list[2].(map[string]any)["states"].([]any)) != 7 {
+	if list[2].(map[string]any)["name"] != "sdlc-loop-clean" || len(list[2].(map[string]any)["states"].([]any)) != 6 {
+		t.Fatalf("sdlc-loop-clean not listed: %v", list)
+	}
+	if list[3].(map[string]any)["name"] != "sdlc-loop-proved" || len(list[3].(map[string]any)["states"].([]any)) != 7 {
 		t.Fatalf("sdlc-loop-proved not listed: %v", list)
 	}
 	// outside a repository: state refuses, workflows works
@@ -305,6 +309,261 @@ func TestHappySdlcLoop(t *testing.T) {
 	}
 }
 
+// cleanScenario is the mock judge for a sdlc-loop-clean drive: the fork adjudicate node confirms the one
+// candidate it is handed in iteration 0 and again in iteration 1 (the re-review's re-surfaced bug). No
+// still-present/verify calls — this workflow has no such node.
+const cleanScenario = `calls:
+  - {kind: adjudicate, node: adjudicate, iter: 0, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 10, output: 5}}
+  - {kind: adjudicate, node: adjudicate, iter: 1, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 10, output: 5}}
+`
+
+// findingsB is a SECOND, distinct finding. It stays on f.go (distinct issue text → distinct candidate):
+// a candidate must name a file the diff actually carries, or adjudicate keeps it as unverified_no_evidence
+// WITHOUT calling the judge (kind.go), and the re-review's mock verdict would never be exercised.
+const findingsB = `{"findings":[{"issue_text":"off-by-one in f.go","file":"f.go","line":7,"severity":"high","category":"bug","source":"lens"}]}`
+
+// TestSdlcLoopCleanReReviewLoopMockAI drives the WHOLE multi-iteration re-review loop of sdlc-loop-clean
+// through the real machine and the real fork/adjudicate path against an injected mock judge — a real temp
+// git repo, the real event log, deterministic, no real LLM and no real subprocess. It proves the FSM
+// ENFORCES re-review of the fix: after fixing B1 the fix INTRODUCES B2, the re-review catches it, the loop
+// re-adjudicates and re-fixes, and the run reaches done(clean) only when a fresh review finds nothing.
+func TestSdlcLoopCleanReReviewLoopMockAI(t *testing.T) {
+	h := newHarness(t)
+	_ = os.WriteFile(filepath.Join(h.root, "mock", "judge.yaml"), []byte(cleanScenario), 0o644)
+	// Ignore .metareview/ so the store's run files never dirty the tree the fix commits into.
+	_ = os.WriteFile(filepath.Join(h.root, ".gitignore"), []byte("mock/\nfixtures/\nexp/\nsmall/\ndocs/\n.metareview/\n"), 0o644)
+	git(t, h.root, "add", ".gitignore")
+	git(t, h.root, "commit", "-q", "-m", "ignore .metareview")
+
+	id := h.must(StatusOK, 0, "init", "--workflow", "sdlc-loop-clean", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock")["run_id"].(string)
+
+	// review discovers `find`, the fork adjudicator confirms it, we commit a fix, and control lands at
+	// recheck (the re-review) — asserting the fix is re-reviewed, not waved through to done.
+	fixLeg := func(node, find, msg string) {
+		h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
+		h.must(StatusOK, 0, "record", "node-output", "--node", node, "--data", h.file(node+".json", find), "--run", id)
+		if env := h.must(machine.StatusAdvanced, 0, "advance", "--run", id); env["to"] != "adjudicate" {
+			t.Fatalf("%s→adjudicate: %v", node, env)
+		}
+		if env := h.must(machine.StatusAdvanced, 0, "advance", "--run", id); env["to"] != "fix" { // real fork adjudicate vs the mock judge
+			t.Fatalf("adjudicate→fix (mock judge confirms): %v", env)
+		}
+		h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
+		sha := h.commit(msg)
+		h.must(StatusOK, 0, "record", "node-output", "--node", "fix", "--data", h.file("fix-"+msg+".json", `{"commit":"`+sha+`","summary":"`+msg+`"}`), "--run", id)
+		if env := h.must(machine.StatusAdvanced, 0, "advance", "--run", id); env["to"] != "recheck" {
+			t.Fatalf("fix→recheck — the FSM must re-review the fix: %v", env)
+		}
+		h.must(machine.StatusNeedsInput, 3, "advance", "--run", id) // recheck awaits its review
+	}
+
+	// iteration 0: fix B1; the fix INTRODUCES B2, which the re-review at recheck catches → loop to discover.
+	fixLeg("discover", findingsData, "fix-b1")
+	h.must(StatusOK, 0, "record", "node-output", "--node", "recheck", "--data", h.file("rc0.json", findingsB), "--run", id)
+	if env := h.must(machine.StatusAdvanced, 0, "advance", "--run", id); env["to"] != "discover" || env["iteration"] != float64(1) {
+		t.Fatalf("a non-clean recheck must LOOP to a fresh discover: %v", env)
+	}
+
+	// iteration 1: the fresh discover re-finds B2, adjudicate re-confirms, we re-fix. recheck is now clean,
+	// but B2 lingers in this iteration's Findings, so the loop takes one more fresh pass at discover.
+	fixLeg("discover", findingsB, "fix-b2")
+	h.must(StatusOK, 0, "record", "node-output", "--node", "recheck", "--data", h.file("rc1.json", `{"findings":[]}`), "--run", id)
+	if env := h.must(machine.StatusAdvanced, 0, "advance", "--run", id); env["to"] != "discover" || env["iteration"] != float64(2) {
+		t.Fatalf("recheck loops once more to a fresh discover: %v", env)
+	}
+
+	// iteration 2: a fresh review of the now-fixed code finds nothing → done(clean).
+	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
+	h.must(StatusOK, 0, "record", "node-output", "--node", "discover", "--data", h.file("d2.json", `{"findings":[]}`), "--run", id)
+	env := h.must(machine.StatusDone, 0, "advance", "--run", id)
+	if env["outcome"] != "clean" {
+		t.Fatalf("a fresh review that finds nothing → done(clean): %v", env)
+	}
+}
+
+// ---- sdlc-loop-clean scenario helpers (real machine + real fork adjudicate vs a mock judge) ----------
+
+// setupClean inits a sdlc-loop-clean mock run on the temp repo and returns the harness and run id.
+// `.metareview/` is ignored so the store's files never dirty the tree a fix commits into.
+func setupClean(t *testing.T, scenario string) (*harness, string) {
+	t.Helper()
+	h := newHarness(t)
+	_ = os.WriteFile(filepath.Join(h.root, "mock", "judge.yaml"), []byte(scenario), 0o644)
+	_ = os.WriteFile(filepath.Join(h.root, ".gitignore"), []byte("mock/\nfixtures/\nexp/\nsmall/\ndocs/\n.metareview/\n"), 0o644)
+	git(t, h.root, "add", ".gitignore")
+	git(t, h.root, "commit", "-q", "-m", "ignore .metareview")
+	return h, h.must(StatusOK, 0, "init", "--workflow", "sdlc-loop-clean", "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock")["run_id"].(string)
+}
+
+// review advances into a review node (discover/recheck), which parks for input, and records its findings.
+func (h *harness) review(id, node, data string) {
+	h.t.Helper()
+	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
+	h.must(StatusOK, 0, "record", "node-output", "--node", node, "--data", h.file("nd.json", data), "--run", id)
+}
+
+// fixCommit advances into the fix node, makes a real commit, and records it.
+func (h *harness) fixCommit(id, msg string) {
+	h.t.Helper()
+	h.must(machine.StatusNeedsInput, 3, "advance", "--run", id)
+	sha := h.commit(msg)
+	h.must(StatusOK, 0, "record", "node-output", "--node", "fix", "--data", h.file("fx.json", `{"commit":"`+sha+`","summary":"`+msg+`"}`), "--run", id)
+}
+
+// adv takes one transition and asserts the state it lands in.
+func (h *harness) adv(id, to string) map[string]any {
+	h.t.Helper()
+	env := h.must(machine.StatusAdvanced, 0, "advance", "--run", id)
+	if env["to"] != to {
+		h.t.Fatalf("advance → %v, want %s: %v", env["to"], to, env)
+	}
+	return env
+}
+
+func finding(text, file string, line int) string {
+	return fmt.Sprintf(`{"issue_text":%q,"file":%q,"line":%d,"severity":"high","category":"bug","source":"lens"}`, text, file, line)
+}
+
+func findingsJSON(items ...string) string { return `{"findings":[` + strings.Join(items, ",") + `]}` }
+
+// TestSdlcLoopCleanTwoBugsOneFixIntroducesAnother: iteration 0 finds TWO bugs, both confirmed and fixed;
+// one fix is correct but the other leaves a FURTHER bug, which the re-review catches → the loop
+// re-adjudicates and re-fixes it → a fresh review is clean → done. (Dave's example scenario.)
+func TestSdlcLoopCleanTwoBugsOneFixIntroducesAnother(t *testing.T) {
+	scenario := `calls:
+  - {kind: adjudicate, node: adjudicate, iter: 0, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 1, output: 1}}
+  - {kind: adjudicate, node: adjudicate, iter: 0, index: 1, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 1, output: 1}}
+  - {kind: adjudicate, node: adjudicate, iter: 1, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 1, output: 1}}
+`
+	b1 := finding("nil deref in f.go", "f.go", 3)
+	b2 := finding("unchecked error in f.go", "f.go", 7)
+	b3 := finding("off-by-one the fix introduced in f.go", "f.go", 8)
+	h, id := setupClean(t, scenario)
+
+	// iter 0: two bugs, both confirmed, both fixed; the re-review finds the further bug b3.
+	h.review(id, "discover", findingsJSON(b1, b2))
+	h.adv(id, "adjudicate")
+	h.adv(id, "fix") // this advance RUNS the adjudicate node against the mock judge
+	if env := h.must(StatusOK, 0, "state", "--run", id); env["counts"].(map[string]any)["confirmed"] != float64(2) {
+		t.Fatalf("both candidates must be confirmed: %v", env["counts"])
+	}
+	h.fixCommit(id, "fix-b1-b2")
+	h.adv(id, "recheck")
+	h.review(id, "recheck", findingsJSON(b3)) // one fix left a further bug
+	if env := h.adv(id, "discover"); env["iteration"] != float64(1) {
+		t.Fatalf("the re-review's finding must start a fresh iteration: %v", env)
+	}
+
+	// iter 1: the fresh review re-finds b3, it is confirmed and fixed, and the re-review is clean.
+	h.review(id, "discover", findingsJSON(b3))
+	h.adv(id, "adjudicate")
+	h.adv(id, "fix")
+	h.fixCommit(id, "fix-b3")
+	h.adv(id, "recheck")
+	h.review(id, "recheck", `{"findings":[]}`)
+	if env := h.adv(id, "discover"); env["iteration"] != float64(2) {
+		t.Fatalf("recheck loops once more for an authoritative fresh review: %v", env)
+	}
+
+	// iter 2: a fresh review of the now-fixed code finds nothing → done(clean).
+	h.review(id, "discover", `{"findings":[]}`)
+	if env := h.must(machine.StatusDone, 0, "advance", "--run", id); env["outcome"] != "clean" {
+		t.Fatalf("done(clean): %v", env)
+	}
+}
+
+// TestSdlcLoopCleanAdjudicatorSplitsVerdict: iteration 0 finds two candidates; the fork adjudicator
+// confirms ONE and REJECTS the other, so only the confirmed one is fixed. The run then converges to
+// done(clean) — the rejected candidate never forces a fix.
+func TestSdlcLoopCleanAdjudicatorSplitsVerdict(t *testing.T) {
+	scenario := `calls:
+  - {kind: adjudicate, node: adjudicate, iter: 0, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 1, output: 1}}
+  - {kind: adjudicate, node: adjudicate, iter: 0, index: 1, raw: '{"reasoning":"r","is_real":false,"confidence":0.8}', tokens: {input: 1, output: 1}}
+`
+	real := finding("real nil deref in f.go", "f.go", 3)
+	notReal := finding("style nit in f.go", "f.go", 7)
+	h, id := setupClean(t, scenario)
+
+	h.review(id, "discover", findingsJSON(real, notReal))
+	h.adv(id, "adjudicate")
+	h.adv(id, "fix") // this advance RUNS adjudicate; only the confirmed candidate drives a fix
+	if env := h.must(StatusOK, 0, "state", "--run", id); env["counts"].(map[string]any)["confirmed"] != float64(1) {
+		t.Fatalf("exactly one of the two candidates must be confirmed: %v", env["counts"])
+	}
+	h.fixCommit(id, "fix-real")
+	h.adv(id, "recheck")
+	h.review(id, "recheck", `{"findings":[]}`)
+	if env := h.adv(id, "discover"); env["iteration"] != float64(1) {
+		t.Fatalf("loop to a fresh review: %v", env)
+	}
+	h.review(id, "discover", `{"findings":[]}`)
+	if env := h.must(machine.StatusDone, 0, "advance", "--run", id); env["outcome"] != "clean" {
+		t.Fatalf("done(clean): %v", env)
+	}
+}
+
+// TestSdlcLoopCleanReReviewCandidateRejectedReachesDone: after a fix (so AllFound>0), the re-review
+// surfaces a candidate and the adjudicator REJECTS it. nothing_confirmed would dead-end here
+// (ERR_BUGS_KNOWN); confirmed_empty (AllFound-blind) must reach done(clean). The blocker, through the
+// real fork.
+func TestSdlcLoopCleanReReviewCandidateRejectedReachesDone(t *testing.T) {
+	scenario := `calls:
+  - {kind: adjudicate, node: adjudicate, iter: 0, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 1, output: 1}}
+  - {kind: adjudicate, node: adjudicate, iter: 1, index: 0, raw: '{"reasoning":"r","is_real":false,"confidence":0.8}', tokens: {input: 1, output: 1}}
+`
+	b1 := finding("nil deref in f.go", "f.go", 3)
+	phantom := finding("suspicious but fine in f.go", "f.go", 7)
+	h, id := setupClean(t, scenario)
+
+	h.review(id, "discover", findingsJSON(b1))
+	h.adv(id, "adjudicate")
+	h.adv(id, "fix")
+	h.fixCommit(id, "fix-b1")
+	h.adv(id, "recheck")
+	h.review(id, "recheck", findingsJSON(phantom)) // re-review surfaces a candidate
+	if env := h.adv(id, "discover"); env["iteration"] != float64(1) {
+		t.Fatalf("loop to a fresh review: %v", env)
+	}
+	h.review(id, "discover", findingsJSON(phantom)) // the fresh review re-surfaces it
+	h.adv(id, "adjudicate")
+	// AllFound>0 (b1 known) and the adjudicator confirms nothing → confirmed_empty → done, not dead-end.
+	if env := h.must(machine.StatusDone, 0, "advance", "--run", id); env["outcome"] != "clean" {
+		t.Fatalf("adjudicator rejects the re-surfaced candidate → done(clean) via confirmed_empty: %v", env)
+	}
+}
+
+// TestSdlcLoopCleanLoopIsBounded: a fix that keeps leaving a bug would loop forever; max_iterations bounds
+// it. A sdlc-loop-clean derived with max_iterations:1 STOPS at the first loop boundary instead of
+// re-reviewing endlessly.
+func TestSdlcLoopCleanLoopIsBounded(t *testing.T) {
+	scenario := `calls:
+  - {kind: adjudicate, node: adjudicate, iter: 0, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 1, output: 1}}
+`
+	h := newHarness(t)
+	_ = os.WriteFile(filepath.Join(h.root, "mock", "judge.yaml"), []byte(scenario), 0o644)
+	_ = os.WriteFile(filepath.Join(h.root, ".gitignore"), []byte("mock/\nfixtures/\nexp/\nsmall/\ndocs/\n.metareview/\n"), 0o644)
+	git(t, h.root, "add", ".gitignore")
+	git(t, h.root, "commit", "-q", "-m", "ignore .metareview")
+	raw, _ := h.deps.Workflows("sdlc-loop-clean")
+	renamed := strings.Replace(string(raw), "workflow: sdlc-loop-clean", "workflow: sdlc-clean-bounded", 1)
+	bounded := h.file("bounded.yaml", strings.Replace(renamed, "max_iterations: 8", "max_iterations: 1", 1))
+	id := h.must(StatusOK, 0, "init", "--workflow", bounded, "--var", "JUDGE=gpt-5.2", "--var", "JUDGE_EFFORT=medium", "--mock-ai", "mock")["run_id"].(string)
+
+	b1 := finding("nil deref in f.go", "f.go", 3)
+	stillBad := finding("the fix did not take in f.go", "f.go", 7)
+	h.review(id, "discover", findingsJSON(b1))
+	h.adv(id, "adjudicate")
+	h.adv(id, "fix")
+	h.fixCommit(id, "fix-attempt")
+	h.adv(id, "recheck")
+	h.review(id, "recheck", findingsJSON(stillBad)) // the fix did not clear it
+	// recheck→discover would begin iteration 1, but max_iterations:1 stops the loop at that boundary.
+	env := h.must(machine.StatusStopped, 1, "advance", "--run", id)
+	if env["stop_reason"] == nil {
+		t.Fatalf("a non-converging loop must STOP at the iteration bound: %v", env)
+	}
+}
+
 func TestRunResolutionAndRecords(t *testing.T) {
 	h := newHarness(t)
 	h.mustErr(CodeNoRuns, 2, "state")
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index 7a75437..73685ca 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -2261,3 +2261,91 @@ func TestRunNodeFixScopedDiffUsesFixEntryHead(t *testing.T) {
 		t.Fatalf("a fix-scoped kind must see the FixEntryHead..head diff, got %q", got)
 	}
 }
+
+// End-to-end proof that sdlc-loop-clean ENFORCES re-review of the fix. It is a graph with one loop:
+// discover → adjudicate → fix → recheck, and recheck loops back to discover for a fresh iteration until
+// a review is clean (discover/recheck → done on findings_empty) or the adjudicator confirms nothing real
+// (adjudicate → done on confirmed_empty). The loop:true edge is recheck→discover: fold clears this
+// iteration's Findings/Confirmed at the boundary, so the loop MUST target a review node (discover) that
+// re-derives them — the authoritative fresh re-review happens there. This test drives:
+//
+//	(A) a fix that INTRODUCES a new bug — the re-review catches it, the loop re-adjudicates and re-fixes,
+//	    and the run only reaches done when a fresh discover is clean;
+//	(B) the blocker the pre-push review caught — once bugs are known (AllFound>0) a re-surfaced candidate
+//	    the adjudicator REJECTS must still reach done via confirmed_empty, not dead-end.
+func TestSdlcLoopCleanEnforcesReReviewOfTheFix(t *testing.T) {
+	distinct := func(text, file string, line int) string {
+		return string(run.MarshalCanonical(run.Delta{Findings: []run.Finding{{IssueText: text, File: file, Line: line}}}))
+	}
+	// discover finds `bug` → adjudicate confirms → fix → recheck, leaving m AT recheck awaiting its review.
+	// Asserts the fix RE-REVIEWS (fix→recheck) rather than exiting straight to done.
+	fixThenRecheck := func(h *harness, m *Machine, bug, file string, line int) {
+		h.record(m, "discover", distinct(bug, file, line))
+		if r := h.advance(m); r.To != "adjudicate" {
+			t.Fatalf("discover→adjudicate: %+v", r)
+		}
+		if r := h.advance(m); r.To != "fix" {
+			t.Fatalf("adjudicate→fix (confirms %s): %+v", bug, r)
+		}
+		h.advance(m)
+		h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"fixed `+bug+`"}`)
+		if r := h.advance(m); r.To != "recheck" {
+			t.Fatalf("fix→recheck — the FSM must re-review the fix, not exit: %+v", r)
+		}
+		h.advance(m) // enter recheck, awaiting its review
+	}
+
+	// (A) iter0: fix B1, and the fix INTRODUCES B2. recheck sees a non-clean diff → loops to a fresh
+	// iteration at discover, which re-reviews and re-finds B2 → re-adjudicate → re-fix. iter1's recheck is
+	// clean, so it loops once more to a fresh discover, which finds nothing → done.
+	h := newHarness(t)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m := h.mustInit(InitOptions{Workflow: "sdlc-loop-clean", Vars: sdlcVars})
+	h.advance(m)
+	fixThenRecheck(h, m, "bug a", "f.go", 1)
+	h.record(m, "recheck", distinct("bug b", "g.go", 2)) // the fix introduced B2; the re-review catches it
+	if r := h.advance(m); r.To != "discover" || r.Status != StatusAdvanced || m.View().Snapshot.Iteration != 1 {
+		t.Fatalf("a non-clean recheck must LOOP to a fresh discover: %+v iter=%d", r, m.View().Snapshot.Iteration)
+	}
+	h.advance(m)
+	fixThenRecheck(h, m, "bug b", "g.go", 2)  // fresh re-review re-finds the fix's bug and fixes it
+	h.record(m, "recheck", `{"findings":[]}`) // recheck now clean, but B2 lingers in Findings → loops once more
+	if r := h.advance(m); r.To != "discover" || m.View().Snapshot.Iteration != 2 {
+		t.Fatalf("recheck loops to a fresh discover for an authoritative re-review: %+v iter=%d", r, m.View().Snapshot.Iteration)
+	}
+	h.advance(m)
+	h.record(m, "discover", `{"findings":[]}`) // fresh review of the now-fixed code: nothing
+	if r := h.advance(m); r.Status != StatusDone || r.Outcome != run.OutcomeClean {
+		t.Fatalf("a fresh review that finds nothing → done(clean): %+v", r)
+	}
+
+	// (B) the blocker: after a fix, a re-surfaced candidate reaches adjudicate with AllFound>0. The
+	// adjudicator REJECTS it. nothing_confirmed would dead-end (ERR_BUGS_KNOWN); confirmed_empty must
+	// reach done.
+	h = newHarness(t)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: "sdlc-loop-clean", Vars: sdlcVars})
+	h.advance(m)
+	fixThenRecheck(h, m, "bug a", "f.go", 1)
+	h.record(m, "recheck", distinct("bug c", "h.go", 3)) // recheck surfaces a distinct candidate N
+	if r := h.advance(m); r.To != "discover" {
+		t.Fatalf("non-clean recheck→discover (loop): %+v", r)
+	}
+	h.advance(m)
+	h.record(m, "discover", distinct("bug c", "h.go", 3)) // the fresh re-review re-surfaces N
+	if r := h.advance(m); r.To != "adjudicate" {
+		t.Fatalf("iter1 discover→adjudicate: %+v", r)
+	}
+	// adjudicator now REJECTS every candidate (Confirmed empty).
+	h.reg.execs["match-then-adjudicate"].fn = func(in ExecInput) (json.RawMessage, error) {
+		for i := range in.Snap.Findings {
+			if err := llmCall(in, i, 10); err != nil {
+				return nil, err
+			}
+		}
+		return json.RawMessage(run.MarshalCanonical(run.Delta{})), nil
+	}
+	if r := h.advance(m); r.Status != StatusDone || r.Outcome != run.OutcomeClean {
+		t.Fatalf("adjudicator rejects the re-surfaced finding → done (confirmed_empty), NOT dead-end: %+v", r)
+	}
+}
diff --git a/internal/fsm/workflow/workflow_test.go b/internal/fsm/workflow/workflow_test.go
index e9b08b8..adf4e5f 100644
--- a/internal/fsm/workflow/workflow_test.go
+++ b/internal/fsm/workflow/workflow_test.go
@@ -659,3 +659,71 @@ func TestW5Accessors(t *testing.T) {
 		t.Fatal("cmdNames")
 	}
 }
+
+// sdlc-loop-clean is the "review the code you just wrote" workflow: after every fix it RE-REVIEWS the
+// change at `recheck` and only reaches done when a fresh review is clean, so a bug the fix itself
+// introduces is caught. This pins that topology (distinct from sdlc-loop, which exits on all_fixed
+// without re-reviewing the fix).
+func TestSDLCLoopCleanReReviewsAfterFix(t *testing.T) {
+	raw, err := workflows.Read("sdlc-loop-clean")
+	if err != nil {
+		t.Fatal(err)
+	}
+	w, err := Parse(raw, Options{Kinds: kinds()})
+	if err != nil {
+		t.Fatalf("sdlc-loop-clean must be a valid workflow: %v", err)
+	}
+	has := func(from, to run.State) *Transition {
+		for i := range w.Transitions {
+			if w.Transitions[i].From == from && w.Transitions[i].To == to {
+				return &w.Transitions[i]
+			}
+		}
+		return nil
+	}
+	// After a fix, control goes to `recheck` (a re-review), not straight to done.
+	if has("fix", "recheck") == nil {
+		t.Fatal("fix must transition to recheck (re-review the fix), not done")
+	}
+	if has("fix", "done") != nil {
+		t.Fatal("fix must NOT exit directly to done without re-review")
+	}
+	// recheck exits clean only when a FRESH review finds nothing, and otherwise LOOPS back to discover.
+	// The loop MUST target a review node: fold clears this iteration's Findings/Confirmed at the loop
+	// boundary, so only a review (discover) re-derives them — targeting adjudicate/fix would hand them an
+	// empty set. discover is therefore where the authoritative fresh re-review of the fix happens.
+	clean := has("recheck", "done")
+	if clean == nil || clean.Gate != "findings_empty" || clean.Outcome == "" {
+		t.Fatalf("recheck must exit to done on findings_empty with an outcome, got %+v", clean)
+	}
+	loop := has("recheck", "discover")
+	if loop == nil || !loop.Loop || loop.Gate != "findings_nonempty" {
+		t.Fatalf("recheck must LOOP back to discover (a review node) on findings_nonempty, got %+v", loop)
+	}
+	if has("recheck", "adjudicate") != nil {
+		t.Fatal("recheck must not loop directly to adjudicate: the loop reset would clear the findings it needs")
+	}
+	// discover→done must be AllFound-BLIND (findings_empty), NOT nothing_found: on the loop AllFound>0
+	// (bugs known), and nothing_found refuses then — a clean re-review would match no edge and dead-end.
+	discClean := has("discover", "done")
+	if discClean == nil || discClean.Outcome == "" || discClean.Gate != "findings_empty" {
+		t.Fatalf("discover→done must be findings_empty (AllFound-blind, reachable on the loop), got %+v", discClean)
+	}
+	// The adjudicator-clean exit must use an AllFound-BLIND gate (confirmed_empty), NOT nothing_confirmed:
+	// once a bug has been confirmed earlier in the run, AllFound > 0, so nothing_confirmed would fail
+	// (ERR_BUGS_KNOWN) and the loop would dead-end on a clean fix whose re-review is then rejected. This
+	// pins the fix for that blocker.
+	adjClean := has("adjudicate", "done")
+	if adjClean == nil || adjClean.Outcome == "" || adjClean.Gate == "nothing_confirmed" {
+		t.Fatalf("adjudicate must exit to done on an AllFound-blind gate (confirmed_empty), got %+v", adjClean)
+	}
+	if adjClean.Gate != "confirmed_empty" {
+		t.Fatalf("adjudicate→done gate should be confirmed_empty (reachable inside the loop), got %q", adjClean.Gate)
+	}
+	// It never exits on all_fixed (the sdlc-loop behavior this workflow deliberately replaces).
+	for _, tr := range w.Transitions {
+		if tr.Gate == "all_fixed" {
+			t.Fatal("sdlc-loop-clean must not exit on all_fixed; it re-reviews until a fresh review is clean")
+		}
+	}
+}
diff --git a/workflows/embed_test.go b/workflows/embed_test.go
index 91b26bd..0882105 100644
--- a/workflows/embed_test.go
+++ b/workflows/embed_test.go
@@ -7,7 +7,7 @@ import (
 
 func TestNamesAndRead(t *testing.T) {
 	names := Names()
-	if len(names) != 3 || names[0] != "review-loop" || names[1] != "sdlc-loop" || names[2] != "sdlc-loop-proved" {
+	if len(names) != 4 || names[0] != "review-loop" || names[1] != "sdlc-loop" || names[2] != "sdlc-loop-clean" || names[3] != "sdlc-loop-proved" {
 		t.Fatalf("Names = %v", names)
 	}
 	for _, n := range names {
diff --git a/workflows/sdlc-loop-clean.yaml b/workflows/sdlc-loop-clean.yaml
new file mode 100644
index 0000000..4c2d5a1
--- /dev/null
+++ b/workflows/sdlc-loop-clean.yaml
@@ -0,0 +1,54 @@
+workflow: sdlc-loop-clean
+version: 1
+# Like sdlc-loop, but it does not stop when the ORIGINAL bugs are fixed: after every fix it RE-REVIEWS
+# the (now fix-modified) diff and only reaches `done` when a fresh review finds nothing OR the adjudicator
+# confirms none of the fresh candidates are real. This catches bugs a fix itself introduces — "review the
+# code you just wrote". sdlc-loop terminates via `verify` (a still-present mutation check that the ORIGINAL
+# bugs are gone), which can exit while a fix-introduced bug is still present; this workflow removes that
+# early exit.
+#
+# It is a graph with ONE loop. The single framework rule that shapes it: the loop:true edge clears this
+# iteration's provisional Findings/Confirmed at the boundary, and only a REVIEW node re-derives them
+# afterward — so the loop's target must be a review node (`discover`), never `adjudicate`/`fix` (they would
+# be handed an empty set). Consequences that follow purely from that rule:
+#   * `discover` is the loop target, so a fresh iteration's authoritative re-review happens there.
+#   * `recheck` is the loop-carrying state (exactly one terminal + the loop edge). It re-reviews the fix;
+#     a clean recheck can exit directly, a non-clean one starts a fresh iteration at `discover`.
+#   * The clean exits are a REVIEW finding nothing (`discover`/`recheck` → done on findings_empty) or the
+#     ADJUDICATOR confirming nothing real (`adjudicate` → done on confirmed_empty). Findings are provisional
+#     candidates; only a review's emptiness or the adjudicator's verdict ends the run.
+vars:
+  REVIEWER:     {default: claude-opus-5}
+  JUDGE:        {required: true}
+  REV_EFFORT:   {default: low}
+  JUDGE_EFFORT: {required: true}
+states: [discover, adjudicate, fix, recheck, done, failed]
+transitions:                                  # ordered
+  # findings_empty (AllFound-blind), NOT nothing_found: on the loop AllFound>0 (bugs already known), and
+  # nothing_found REFUSES once bugs are known — a clean re-review would then match no edge and dead-end.
+  # findings_empty reaches done whenever THIS review found no candidates, iteration 0 or later.
+  - {from: discover,   to: done,       gate: findings_empty,    outcome: clean}   # a review found no candidates → bug-free
+  - {from: discover,   to: adjudicate, gate: findings_nonempty}
+  # confirmed_empty (this iteration's Confirmed == 0), NOT nothing_confirmed: once ANY bug has been
+  # confirmed in the run, AllFound>0, so nothing_confirmed would fail (ERR_BUGS_KNOWN) and dead-end on the
+  # common good case — a re-review that surfaces a candidate the adjudicator then rejects. confirmed_empty
+  # is AllFound-blind, so that case correctly reaches done.
+  - {from: adjudicate, to: done,       gate: confirmed_empty,   outcome: clean}   # adjudicator confirmed none real → bug-free
+  - {from: adjudicate, to: fix,        gate: confirmed_nonempty}
+  - {from: fix,        to: recheck,    gate: commit_exists}
+  # recheck is the loop-carrying state: exactly one outcome-bearing terminal (→done) plus the loop edge.
+  - {from: recheck,    to: done,       gate: findings_empty,    outcome: clean}   # a fresh review of the fix found nothing → bug-free
+  - {from: recheck,    to: discover,   gate: findings_nonempty, loop: true}       # the re-review is not clean → new iteration re-discovers → adjudicate → fix → recheck …
+nodes:
+  discover:   {kind: review-lenses,        exec: subagent, model: $REVIEWER, effort: $REV_EFFORT}
+  adjudicate: {kind: match-then-adjudicate, exec: fork,     model: $JUDGE, effort: $JUDGE_EFFORT}
+  fix:        {kind: agent-edit}
+  recheck:    {kind: review-lenses,        exec: subagent, model: $REVIEWER, effort: $REV_EFFORT}
+# No `no_fixation_progress` here. That atom measures progress by bugs marked fixed via a still-present
+# oracle (BugStatus), which this workflow has no node to emit — its termination signal is a clean re-review
+# / adjudication, a different oracle. With no fixed-marking every bug stays in UnfixedAtEntry, so
+# no_fixation_progress would stall the loop at its first boundary. max_iterations + budget bound the loop
+# (Validate requires a looping workflow to carry one of them).
+convergence:
+  any: [{max_iterations: 8}, {budget: {tokens: 4000000}}]
+repo_mode: advisory

diff --git a/internal/fsm/cli/cli_test.go b/internal/fsm/cli/cli_test.go
index aeb0d55..30488e7 100644
--- a/internal/fsm/cli/cli_test.go
+++ b/internal/fsm/cli/cli_test.go
@@ -559,8 +559,8 @@ func TestSdlcLoopCleanLoopIsBounded(t *testing.T) {
 	h.review(id, "recheck", findingsJSON(stillBad)) // the fix did not clear it
 	// recheck→discover would begin iteration 1, but max_iterations:1 stops the loop at that boundary.
 	env := h.must(machine.StatusStopped, 1, "advance", "--run", id)
-	if env["stop_reason"] == nil {
-		t.Fatalf("a non-converging loop must STOP at the iteration bound: %v", env)
+	if sr, _ := env["stop_reason"].(string); !strings.Contains(sr, "max_iterations") {
+		t.Fatalf("the loop must STOP for the iteration bound (max_iterations), got stop_reason %q: %v", sr, env)
 	}
 }
 
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index 73685ca..1ca8c51 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -2267,12 +2267,18 @@ func TestRunNodeFixScopedDiffUsesFixEntryHead(t *testing.T) {
 // a review is clean (discover/recheck → done on findings_empty) or the adjudicator confirms nothing real
 // (adjudicate → done on confirmed_empty). The loop:true edge is recheck→discover: fold clears this
 // iteration's Findings/Confirmed at the boundary, so the loop MUST target a review node (discover) that
-// re-derives them — the authoritative fresh re-review happens there. This test drives:
+// re-derives them — the authoritative fresh re-review happens there.
+//
+// This is MACHINE-LAYER coverage: adjudicate is the harness's fake executor (it does not run kind.go's
+// real judging), so what it pins is the graph/gate wiring, not the fork's rejection logic. The real
+// fork/adjudicate rejection is exercised end-to-end in cli's TestSdlcLoopCleanReReviewCandidateRejected...
+// This test drives:
 //
 //	(A) a fix that INTRODUCES a new bug — the re-review catches it, the loop re-adjudicates and re-fixes,
 //	    and the run only reaches done when a fresh discover is clean;
-//	(B) the blocker the pre-push review caught — once bugs are known (AllFound>0) a re-surfaced candidate
-//	    the adjudicator REJECTS must still reach done via confirmed_empty, not dead-end.
+//	(B) the blocker — once bugs are known (AllFound>0), when adjudicate confirms nothing the run must still
+//	    reach done via confirmed_empty (AllFound-blind), not dead-end. Here the fake adjudicate returns an
+//	    empty Delta to model "nothing confirmed"; the gate wiring is what's under test.
 func TestSdlcLoopCleanEnforcesReReviewOfTheFix(t *testing.T) {
 	distinct := func(text, file string, line int) string {
 		return string(run.MarshalCanonical(run.Delta{Findings: []run.Finding{{IssueText: text, File: file, Line: line}}}))
diff --git a/internal/fsm/workflow/workflow_test.go b/internal/fsm/workflow/workflow_test.go
index adf4e5f..6fb3543 100644
--- a/internal/fsm/workflow/workflow_test.go
+++ b/internal/fsm/workflow/workflow_test.go
@@ -688,13 +688,19 @@ func TestSDLCLoopCleanReReviewsAfterFix(t *testing.T) {
 	if has("fix", "done") != nil {
 		t.Fatal("fix must NOT exit directly to done without re-review")
 	}
-	// recheck exits clean only when a FRESH review finds nothing, and otherwise LOOPS back to discover.
-	// The loop MUST target a review node: fold clears this iteration's Findings/Confirmed at the loop
-	// boundary, so only a review (discover) re-derives them — targeting adjudicate/fix would hand them an
-	// empty set. discover is therefore where the authoritative fresh re-review of the fix happens.
+	// recheck LOOPS back to discover on findings_nonempty; the loop MUST target a review node: fold clears
+	// this iteration's Findings/Confirmed at the loop boundary, so only a review (discover) re-derives them
+	// — targeting adjudicate/fix would hand them an empty set. discover is therefore where the authoritative
+	// fresh re-review of the fix happens.
+	//
+	// This pins that recheck carries a findings_empty→done terminal, which Validate's loop_terminal rule
+	// REQUIRES (a loop-carrying state needs exactly one outcome-bearing terminal). It pins the edge EXISTS,
+	// not that it ever fires — at runtime it is dead (findings linger within an iteration, so recheck's set
+	// is never empty and it always loops). The clean exit the suite actually exercises is discover→done; do
+	// not read this assertion as proof recheck exits clean directly.
 	clean := has("recheck", "done")
 	if clean == nil || clean.Gate != "findings_empty" || clean.Outcome == "" {
-		t.Fatalf("recheck must exit to done on findings_empty with an outcome, got %+v", clean)
+		t.Fatalf("recheck must carry the (structural) findings_empty→done terminal, got %+v", clean)
 	}
 	loop := has("recheck", "discover")
 	if loop == nil || !loop.Loop || loop.Gate != "findings_nonempty" {
diff --git a/workflows/sdlc-loop-clean.yaml b/workflows/sdlc-loop-clean.yaml
index 4c2d5a1..8a98e65 100644
--- a/workflows/sdlc-loop-clean.yaml
+++ b/workflows/sdlc-loop-clean.yaml
@@ -13,10 +13,17 @@ version: 1
 # be handed an empty set). Consequences that follow purely from that rule:
 #   * `discover` is the loop target, so a fresh iteration's authoritative re-review happens there.
 #   * `recheck` is the loop-carrying state (exactly one terminal + the loop edge). It re-reviews the fix;
-#     a clean recheck can exit directly, a non-clean one starts a fresh iteration at `discover`.
-#   * The clean exits are a REVIEW finding nothing (`discover`/`recheck` → done on findings_empty) or the
-#     ADJUDICATOR confirming nothing real (`adjudicate` → done on confirmed_empty). Findings are provisional
-#     candidates; only a review's emptiness or the adjudicator's verdict ends the run.
+#     any non-clean re-review starts a fresh iteration at `discover`.
+#   * The clean exit that actually FIRES is a fresh `discover` finding nothing (findings_empty) or the
+#     `adjudicate`-tor confirming nothing real (confirmed_empty). Findings are provisional candidates; only a
+#     review's emptiness or the adjudicator's verdict ends the run.
+#   * NOTE: `recheck → done` (findings_empty) is a STRUCTURAL requirement of Validate's loop_terminal rule
+#     (the loop-carrying state must carry exactly one outcome-bearing terminal), but it is DEAD at runtime
+#     and never taken: findings accumulate within an iteration and are cleared only at the loop boundary
+#     (Delta.Findings is json omitempty, so an empty re-review is a fold no-op — it does not clear the
+#     findings `discover` already recorded this iteration). So `recheck` always sees a non-empty finding set
+#     and always loops. Do not "optimize" this edge away (Validate rejects a loop state with no terminal)
+#     and do not rely on it firing — the authoritative clean exit is the fresh `discover → done`.
 vars:
   REVIEWER:     {default: claude-opus-5}
   JUDGE:        {required: true}
@@ -37,7 +44,9 @@ transitions:                                  # ordered
   - {from: adjudicate, to: fix,        gate: confirmed_nonempty}
   - {from: fix,        to: recheck,    gate: commit_exists}
   # recheck is the loop-carrying state: exactly one outcome-bearing terminal (→done) plus the loop edge.
-  - {from: recheck,    to: done,       gate: findings_empty,    outcome: clean}   # a fresh review of the fix found nothing → bug-free
+  # This terminal satisfies Validate's loop_terminal rule but is DEAD at runtime (see header): findings
+  # linger within an iteration, so recheck's finding set is never empty and this edge never fires.
+  - {from: recheck,    to: done,       gate: findings_empty,    outcome: clean}   # STRUCTURAL terminal (never taken); real clean exit is discover→done
   - {from: recheck,    to: discover,   gate: findings_nonempty, loop: true}       # the re-review is not clean → new iteration re-discovers → adjudicate → fix → recheck …
 nodes:
   discover:   {kind: review-lenses,        exec: subagent, model: $REVIEWER, effort: $REV_EFFORT}

```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

{"schemaVersion":1,"kind":"validation","command":["go","test","./internal/fsm/...","./workflows/"],"cwd":"/Users/dsifry/Developer/metareview","exitCode":0,"startedAt":"2026-09-02T08:28:59.550653Z","finishedAt":"2026-09-02T08:28:59.940203Z","stdoutSha256":"5646d88ab2da19d69c548f02cd4377a9676acc04a9a9340b040328ca52c7b72a","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./internal/fsm/... ./workflows/ exited 0"}

