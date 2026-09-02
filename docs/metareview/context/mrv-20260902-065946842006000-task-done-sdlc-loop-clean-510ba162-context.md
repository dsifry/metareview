# metareview task-done context

Run ID: `mrv-20260902-065946842006000-task-done-sdlc-loop-clean-510ba162`

## Task

workflow: sdlc-loop-clean
version: 1
# Like sdlc-loop, but it does not stop when the ORIGINAL bugs are fixed: after every fix it RE-REVIEWS
# the (now fix-modified) diff at `recheck` and only reaches `done` when that fresh review finds nothing
# (or the adjudicator confirms none of the new candidates are real). This catches bugs a fix itself
# introduces — "review the code you just wrote" — iterating fix -> recheck -> adjudicate -> fix until the
# review/adjudicator pronounces the change bug-free.
vars:
  REVIEWER:     {default: claude-opus-5}
  JUDGE:        {required: true}
  REV_EFFORT:   {default: low}
  JUDGE_EFFORT: {required: true}
states: [discover, adjudicate, fix, recheck, done, failed]
transitions:                                  # ordered
  - {from: discover,   to: done,       gate: findings_empty,    outcome: clean}    # nothing found at all → bug-free
  - {from: discover,   to: adjudicate, gate: findings_nonempty}
  - {from: adjudicate, to: done,       gate: nothing_confirmed, outcome: clean}    # adjudicator rejected every candidate → bug-free
  - {from: adjudicate, to: fix,        gate: confirmed_nonempty}
  - {from: fix,        to: recheck,    gate: commit_exists}
  - {from: recheck,    to: done,       gate: findings_empty,    outcome: clean}    # a FRESH review of the fix found nothing → bug-free
  - {from: recheck,    to: adjudicate, gate: findings_nonempty, loop: true}        # the fix introduced/left findings → adjudicate → fix → recheck …
nodes:
  discover:   {kind: review-lenses,        exec: subagent, model: $REVIEWER, effort: $REV_EFFORT}
  adjudicate: {kind: match-then-adjudicate, exec: fork,     model: $JUDGE, effort: $JUDGE_EFFORT}
  fix:        {kind: agent-edit}
  recheck:    {kind: review-lenses,        exec: subagent, model: $REVIEWER, effort: $REV_EFFORT}
convergence:
  any: [no_fixation_progress, {max_iterations: 8}, {budget: {tokens: 4000000}}]
repo_mode: advisory


## Git

- Base: `f785654630d864f11f98cc47fd73502a428bcefa`
- Head: `aac43b1b66eb9f962f1d512fd2060181e4c697df`
- Branch: `fsm-iterate-until-clean`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `7183`
- Filtered diff bytes: `7183`
- Risk level: `none`

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- CLAUDE.md
- internal/fsm/cli/cli_test.go
- internal/fsm/workflow/workflow_test.go
- workflows/embed_test.go
- workflows/sdlc-loop-clean.yaml

### Manifest Blockers
No manifest blockers.

## Changed Files

- CLAUDE.md
- internal/fsm/cli/cli_test.go
- internal/fsm/workflow/workflow_test.go
- workflows/embed_test.go
- workflows/sdlc-loop-clean.yaml

## Diff

```diff
diff --git a/CLAUDE.md b/CLAUDE.md
index 7e34281..f0667e9 100644
--- a/CLAUDE.md
+++ b/CLAUDE.md
@@ -59,6 +59,13 @@ metareview override list [--pending]
 ## Lifecycle Placement
 
 - Before implementing a plan or spec: review the artifact.
+- **After writing a fix or new code, review the code you just wrote — and iterate until it is bug-free.**
+  Run `metareview fsm --workflow sdlc-loop-clean --base <ref>` on your own diff: it discovers → adjudicates
+  → fixes, then **re-reviews the fix at `recheck` and only stops when a fresh review finds nothing**. A fix
+  often introduces its own bug (a broadened matcher over-matches, a parsed value overflows); the
+  deterministic task-done gate and the external bots will not always catch it, but a fresh adversarial
+  review of the diff does. Do this before opening the PR. (Reviews parallelize — fan out lenses and run one
+  loop per fix concurrently.)
 - After each small implementation chunk: run task-done.
 - After all child tasks for an epic are complete: run epic-ready.
 - Before opening, pushing, or merging a PR: run pr-ready.
diff --git a/internal/fsm/cli/cli_test.go b/internal/fsm/cli/cli_test.go
index 5cd5c24..8b30bfc 100644
--- a/internal/fsm/cli/cli_test.go
+++ b/internal/fsm/cli/cli_test.go
@@ -172,10 +172,13 @@ func TestUsageAndPrompt(t *testing.T) {
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
diff --git a/internal/fsm/workflow/workflow_test.go b/internal/fsm/workflow/workflow_test.go
index e9b08b8..813efdf 100644
--- a/internal/fsm/workflow/workflow_test.go
+++ b/internal/fsm/workflow/workflow_test.go
@@ -659,3 +659,48 @@ func TestW5Accessors(t *testing.T) {
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
+	// recheck exits clean only when a FRESH review finds nothing, and otherwise LOOPS back to adjudicate.
+	clean := has("recheck", "done")
+	if clean == nil || clean.Gate != "findings_empty" || clean.Outcome == "" {
+		t.Fatalf("recheck must exit to done on findings_empty with an outcome, got %+v", clean)
+	}
+	loop := has("recheck", "adjudicate")
+	if loop == nil || !loop.Loop || loop.Gate != "findings_nonempty" {
+		t.Fatalf("recheck must LOOP back to adjudicate on findings_nonempty, got %+v", loop)
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
index 0000000..06474fb
--- /dev/null
+++ b/workflows/sdlc-loop-clean.yaml
@@ -0,0 +1,29 @@
+workflow: sdlc-loop-clean
+version: 1
+# Like sdlc-loop, but it does not stop when the ORIGINAL bugs are fixed: after every fix it RE-REVIEWS
+# the (now fix-modified) diff at `recheck` and only reaches `done` when that fresh review finds nothing
+# (or the adjudicator confirms none of the new candidates are real). This catches bugs a fix itself
+# introduces — "review the code you just wrote" — iterating fix -> recheck -> adjudicate -> fix until the
+# review/adjudicator pronounces the change bug-free.
+vars:
+  REVIEWER:     {default: claude-opus-5}
+  JUDGE:        {required: true}
+  REV_EFFORT:   {default: low}
+  JUDGE_EFFORT: {required: true}
+states: [discover, adjudicate, fix, recheck, done, failed]
+transitions:                                  # ordered
+  - {from: discover,   to: done,       gate: findings_empty,    outcome: clean}    # nothing found at all → bug-free
+  - {from: discover,   to: adjudicate, gate: findings_nonempty}
+  - {from: adjudicate, to: done,       gate: nothing_confirmed, outcome: clean}    # adjudicator rejected every candidate → bug-free
+  - {from: adjudicate, to: fix,        gate: confirmed_nonempty}
+  - {from: fix,        to: recheck,    gate: commit_exists}
+  - {from: recheck,    to: done,       gate: findings_empty,    outcome: clean}    # a FRESH review of the fix found nothing → bug-free
+  - {from: recheck,    to: adjudicate, gate: findings_nonempty, loop: true}        # the fix introduced/left findings → adjudicate → fix → recheck …
+nodes:
+  discover:   {kind: review-lenses,        exec: subagent, model: $REVIEWER, effort: $REV_EFFORT}
+  adjudicate: {kind: match-then-adjudicate, exec: fork,     model: $JUDGE, effort: $JUDGE_EFFORT}
+  fix:        {kind: agent-edit}
+  recheck:    {kind: review-lenses,        exec: subagent, model: $REVIEWER, effort: $REV_EFFORT}
+convergence:
+  any: [no_fixation_progress, {max_iterations: 8}, {budget: {tokens: 4000000}}]
+repo_mode: advisory



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

{"schemaVersion":1,"kind":"validation","command":["go","test","./workflows/","./internal/fsm/workflow/","./internal/fsm/cli/"],"cwd":".","exitCode":0,"startedAt":"2026-09-02T06:59:46.567129Z","finishedAt":"2026-09-02T06:59:46.835427Z","stdoutSha256":"97cc63622579adb6c902d115eaa132b11b87c151a638a4b6300128df8e679a48","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./workflows/ ./internal/fsm/workflow/ ./internal/fsm/cli/ exited 0"}

