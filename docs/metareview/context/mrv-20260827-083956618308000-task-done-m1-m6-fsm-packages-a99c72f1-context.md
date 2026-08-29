# metareview task-done context

Run ID: `mrv-20260827-083956618308000-task-done-m1-m6-fsm-packages-a99c72f1`

## Task

# M1–M6: internal/fsm core packages

Implement `internal/fsm/{errs,converge,gate,workflow,machine,cmdexec,judge,mockai,kind}` per
`docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md` (r4) and `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
(r5), test-first, under the combined coverage gate (`tests/coverage.sh`), reviewed per commit range (≤ 120 KB each).

## Acceptance

- Every §7/§8 test row has a discriminating test (literal pins; goldens regression-only behind an env flag).
- `go test ./internal/fsm/...` passes; every `internal/fsm/*` package at exactly 100% statements.
- `bash tests/coverage.sh` passes (legacy floor held).
- Dependency direction per spec 2 §1 (machine imports no kinds/judge/cmdexec/workflows).
- Every LLM/shell effect behind an interface; no shell, pinned argv, exact env in `cmdexec`.


## Git

- Base: `afadc0c4b9482667406bcc82f58a76321f3ed6d8`
- Head: `da6aa55a989146a58cfe65501fa085bc7a596cf1`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `16917`
- Filtered diff bytes: `16917`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `03320ead3147df22`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/tasks/m1-m6-fsm-packages.md
- internal/fsm/machine/harness_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/types.go
- internal/fsm/workflow/workflow.go
- internal/fsm/workflow/workflow_test.go

### Shards
- shard-01: docs/tasks/m1-m6-fsm-packages.md, internal/fsm/machine/harness_test.go, internal/fsm/machine/machine.go, internal/fsm/machine/machine_test.go, internal/fsm/machine/types.go, internal/fsm/workflow/workflow.go, internal/fsm/workflow/workflow_test.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- internal/fsm/machine/harness_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/types.go
- internal/fsm/workflow/workflow.go
- internal/fsm/workflow/workflow_test.go
- docs/tasks/m1-m6-fsm-packages.md

## Diff

```diff
diff --git a/internal/fsm/machine/harness_test.go b/internal/fsm/machine/harness_test.go
index 4255251..59ea529 100644
--- a/internal/fsm/machine/harness_test.go
+++ b/internal/fsm/machine/harness_test.go
@@ -259,17 +259,21 @@ func (c *countingStore) RepairTail(id string) error {
 // ---- runner ----
 
 type fakeRunner struct {
-	calls  []string
-	stdins [][]byte
-	res    converge.CmdResult
-	err    error
-	audit  func(run.Event) error
-	name   string
+	calls    []string
+	stdins   [][]byte
+	res      converge.CmdResult
+	err      error
+	audit    func(run.Event) error
+	ordinal  func(string) int
+	ordinals []int
 }
 
 func (f *fakeRunner) Run(_ context.Context, name string, stdin []byte) (converge.CmdResult, error) {
 	f.calls = append(f.calls, name)
 	f.stdins = append(f.stdins, stdin)
+	if f.ordinal != nil {
+		f.ordinals = append(f.ordinals, f.ordinal(name))
+	}
 	if f.audit != nil {
 		d := run.CmdCallData{Name: name, Argv: []string{"/bin/true"}, InputHash: "x", ExitCode: f.res.ExitCode}
 		if err := f.audit(run.Event{Type: run.TypeCmdCall, Data: run.MarshalCanonical(d)}); err != nil {
@@ -305,8 +309,9 @@ func newHarness(t *testing.T) *harness {
 	h.deps = Deps{
 		Store: h.store, Sidecar: h.sidecar, Kinds: h.reg,
 		Git: func(dir string) gate.Git { return h.git.get(dir) },
-		Runner: func(_ []run.AllowedCmd, _, _ string, audit func(run.Event) error) converge.Runner {
-			h.runner.audit = audit
+		Runner: func(d RunnerDeps) converge.Runner {
+			h.runner.audit = d.Audit
+			h.runner.ordinal = d.CmdCalls
 			return h.runner
 		},
 		Clock: func() run.Time {
diff --git a/internal/fsm/machine/machine.go b/internal/fsm/machine/machine.go
index 0869197..11b88bb 100644
--- a/internal/fsm/machine/machine.go
+++ b/internal/fsm/machine/machine.go
@@ -40,7 +40,8 @@ type session struct {
 	runner   converge.Runner
 	git      gate.Git
 	warns    []string
-	auditErr error // the first store error seen through the audit closure
+	auditErr error          // the first store error seen through the audit closure
+	cmdCalls map[string]int // prior cmd_call count per command name (durable ordinal for mocks)
 	unlock   func()
 }
 
@@ -88,6 +89,9 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 	if err != nil {
 		return nil, err
 	}
+	if !filepath.IsAbs(o.WorkDir) || !filepath.IsAbs(o.RepoRoot) {
+		return nil, errs.E(CodeWorkdirForeign, "work dir and repo root must be absolute", "reason", "relative", "work_dir", o.WorkDir, "repo_root", o.RepoRoot)
+	}
 	// 2. commands + consent
 	allowed, sha, err := workflow.ResolveCmds(w, o.WorkDir, deps.LookPath, deps.FileHash)
 	if err != nil {
@@ -148,7 +152,11 @@ func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
 		if err != nil {
 			return nil, errs.E(CodeMockInvalid, err.Error(), "dir", o.MockDir)
 		}
-		rel := strings.TrimPrefix(strings.TrimPrefix(filepath.Clean(dir), filepath.Clean(o.RepoRoot)), string(filepath.Separator))
+		root := filepath.Clean(o.RepoRoot) + string(filepath.Separator)
+		if !strings.HasPrefix(filepath.Clean(dir)+string(filepath.Separator), root) {
+			return nil, errs.E(CodeMockInvalid, "mock scenario must live inside the repository", "dir", o.MockDir, "reason", "outside")
+		}
+		rel := strings.TrimPrefix(filepath.Clean(dir), root)
 		mock = rel + "#" + h[:16]
 	}
 	if deps.Kinds.Mock() != (mock != "") {
@@ -357,7 +365,16 @@ func (m *Machine) load(ctx context.Context, repair bool) (*session, error) {
 		return nil, errs.E(CodeMockMismatch, "the kind registry's mock mode does not match the run")
 	}
 	sess.git = deps.Git(snap.WorkDir)
-	sess.runner = deps.Runner(snap.AllowedCmds, snap.WorkDir, m.runID, sess.audit)
+	sess.cmdCalls = map[string]int{}
+	for _, ev := range log.Events {
+		if ev.Type == run.TypeCmdCall {
+			var cd run.CmdCallData
+			if json.Unmarshal(ev.Data, &cd) == nil {
+				sess.cmdCalls[cd.Name]++
+			}
+		}
+	}
+	sess.runner = deps.Runner(RunnerDeps{Allowed: snap.AllowedCmds, WorkDir: snap.WorkDir, RunID: m.runID, Audit: sess.audit, CmdCalls: func(name string) int { return sess.cmdCalls[name] }})
 	if w.Convergence != nil {
 		sess.pred = converge.MustParse(w.Convergence, sess.runner) // validated by workflow.Parse
 	}
@@ -416,6 +433,12 @@ func (s *session) audit(ev run.Event) error {
 		}
 		return err
 	}
+	if ev.Type == run.TypeCmdCall {
+		var cd run.CmdCallData
+		if json.Unmarshal(ev.Data, &cd) == nil {
+			s.cmdCalls[cd.Name]++
+		}
+	}
 	return nil
 }
 
@@ -674,7 +697,7 @@ func (s *session) transitions(head string) (AdvanceResult, error) {
 			if err != nil && s.auditErr != nil {
 				return AdvanceResult{}, s.auditErr // the atom's cmd_call could not be stored: abort, not a gate failure
 			}
-			if err != nil || (r.Class == run.OutcomeFixed && r.Atom != "all_fixed") {
+			if err != nil || (r.Stop && r.Class == run.OutcomeFixed && r.Atom != "all_fixed") {
 				detail := "convergence evaluation failed"
 				reason := "error"
 				if err != nil {
@@ -691,12 +714,13 @@ func (s *session) transitions(head string) (AdvanceResult, error) {
 				return s.fail(ge, head)
 			}
 			reason, _ := run.CapText(r.Reason, run.MaxText)
-			if err := s.append(run.TypeConverge, run.ConvergeData{Atom: r.Atom, Class: r.Class, Stop: r.Stop, Reason: reason}, ""); err != nil {
+			atom, _ := run.CapText(r.Atom, run.MaxShort)
+			if err := s.append(run.TypeConverge, run.ConvergeData{Atom: atom, Class: r.Class, Stop: r.Stop, Reason: reason}, ""); err != nil {
 				return AdvanceResult{}, err
 			}
 			if r.Stop {
-				chosen = &workflow.Transition{From: snap.State, To: tt.To, Gate: r.Atom, Outcome: r.Class}
-				return s.finish(s.transitionData(*chosen, head), r.Atom+": "+reason)
+				chosen = &workflow.Transition{From: snap.State, To: tt.To, Gate: atom, Outcome: r.Class}
+				return s.finish(s.transitionData(*chosen, head), atom+": "+reason)
 			}
 		}
 		if chosen == nil {
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index fcf7690..e2a061e 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -111,6 +111,8 @@ func TestM1InitErrors(t *testing.T) {
 		{"goldens-null-ok", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, GoldensPath: "/x/gnull.json"}, func() { h.files["/x/gnull.json"] = []byte(`null`) }, ""},
 		{"mock-registry-mismatch", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "scen/ok"}, func() { h.mockHash["/repo/scen/ok"] = strings.Repeat("a", 64) }, CodeMockMismatch},
 		{"base-unknown", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, Base: "nope"}, nil, gate.CodeGit},
+		{"workdir-relative", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, WorkDir: "rel"}, nil, CodeWorkdirForeign},
+		{"mock-outside-root", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, MockDir: "/other/scen"}, func() { h.mockHash["/other/scen"] = strings.Repeat("b", 64) }, CodeMockInvalid},
 		{"workdir-foreign", InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars, WorkDir: "/elsewhere"}, func() {
 			h.git.byDir["/elsewhere"] = &gate.Fake{HeadSHA: shaHead, Common: "/other/.git"}
 		}, CodeWorkdirForeign},
@@ -693,6 +695,9 @@ func TestM4Convergence(t *testing.T) {
 	if !strings.Contains(string(h.runner.stdins[0]), `"vars":{"JUDGE":"sha256:`) {
 		t.Fatal("cmd atom receives the redacted payload")
 	}
+	if len(h.runner.ordinals) != 1 || h.runner.ordinals[0] != 0 {
+		t.Fatalf("first call ordinal 0: %v", h.runner.ordinals)
+	}
 	// converge error
 	h = newHarness(t)
 	wf = sdlcWith(t, h, "custom2.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
@@ -720,12 +725,38 @@ func TestM4Convergence(t *testing.T) {
 	if err != nil {
 		t.Fatal(err)
 	}
-	sess.pred = fixedPred{}
+	sess.pred = fixedPred{stop: true}
 	r, err = sess.advance()
 	sess.unlock()
 	if err != nil || r.Gate == nil || r.Gate.Name != GateConverge || !strings.Contains(r.Gate.Error.Detail, "fixed_class") {
 		t.Fatalf("fixed_class: %+v %v", r, err)
 	}
+	// a NON-firing fixed-class result (any: [all_fixed, …] with bugs remaining) is not a failure: the loop is taken
+	h = newHarness(t)
+	m = loopRun(t, h, 1, "sdlc-loop")
+	sess, _ = m.load(context.Background(), false)
+	sess.pred = fixedPred{stop: false}
+	r, err = sess.advance()
+	sess.unlock()
+	if err != nil || r.Status != StatusAdvanced || r.To != "discover" {
+		t.Fatalf("non-firing fixed class: %+v %v", r, err)
+	}
+	// the real all_fixed atom firing on a confirmed_empty terminal gate → fixed (design §9 example)
+	h = newHarness(t)
+	wf = sdlcWith(t, h, "af.yaml", "  - {from: verify,     to: done,       gate: all_fixed,   outcome: fixed}", "  - {from: verify,     to: done,       gate: confirmed_empty,   outcome: fixed}")
+	h.files["/x/af.yaml"] = []byte(strings.Replace(string(h.files["/x/af.yaml"]), "any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]", "any: [all_fixed, {max_iterations: 5}]", 1))
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	if r := h.advance(m); r.Outcome != run.OutcomeFixed || r.StopReason != "all_fixed: all bugs fixed" {
+		t.Fatalf("all_fixed atom: %+v", r)
+	}
 	// user workflow whose terminal gate is confirmed_empty: convergence still bounds the loop
 	h = newHarness(t)
 	wf = sdlcWith(t, h, "ce.yaml", "  - {from: verify,     to: done,       gate: all_fixed,   outcome: fixed}", "  - {from: verify,     to: done,       gate: confirmed_empty,   outcome: fixed}")
@@ -742,6 +773,31 @@ func TestM4Convergence(t *testing.T) {
 	if r := h.advance(m); r.Outcome != run.OutcomeOverflow {
 		t.Fatalf("bounded even when all fixed: %+v", r)
 	}
+	// Atom cap: an `all` of 32 long-named cmd atoms yields a >1 KB name; it is capped, never a store rejection
+	h = newHarness(t)
+	var names, decls []string
+	for i := 0; i < 32; i++ {
+		n := "cmd-" + strings.Repeat("x", 25) + string(rune('a'+i/10)) + string(rune('0'+i%10))
+		names = append(names, "{cmd: "+n+"}")
+		decls = append(decls, "  "+n+": {argv: [bash, -c, echo]}")
+	}
+	wf = sdlcWith(t, h, "wide.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
+		"  all: ["+strings.Join(names, ", ")+"]\ncmds:\n"+strings.Join(decls, "\n")+"\nrepo_mode: advisory")
+	h.runner.res = converge.CmdResult{Stdout: []byte(`{"stop": true, "reason": "x"}`)}
+	_, sha, _ = workflow.ResolveCmds(mustResolve(t, h, wf), "/repo", h.deps.LookPath, h.deps.FileHash)
+	allPresent(h)
+	h.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
+	m = h.mustInit(InitOptions{Workflow: wf, Vars: sdlcVars, AllowCustomCmds: sha})
+	h.advance(m)
+	h.record(m, "discover", findings(1))
+	h.advance(m)
+	h.advance(m)
+	h.advance(m)
+	h.record(m, "fix", `{"commit":"`+shaFix+`","summary":"s"}`)
+	h.advance(m)
+	if r := h.advance(m); r.Outcome != run.OutcomeCustom || len(r.StopReason) > run.MaxShort+run.MaxText+8 {
+		t.Fatalf("wide all: %+v", r)
+	}
 	// emitter cap: 5 KB cmd reason capped in converge event and StopReason
 	h = newHarness(t)
 	wf = sdlcWith(t, h, "custom3.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
@@ -776,12 +832,12 @@ func allPresent(h *harness) {
 	}
 }
 
-type fixedPred struct{}
+type fixedPred struct{ stop bool }
 
 func (fixedPred) Name() string       { return "evil" }
 func (fixedPred) Class() run.Outcome { return run.OutcomeFixed }
-func (fixedPred) Evaluate(context.Context, run.Snapshot) (converge.Result, error) {
-	return converge.Result{Stop: true, Atom: "evil", Class: run.OutcomeFixed, Reason: "ha"}, nil
+func (p fixedPred) Evaluate(context.Context, run.Snapshot) (converge.Result, error) {
+	return converge.Result{Stop: p.stop, Atom: "evil", Class: run.OutcomeFixed, Reason: "ha"}, nil
 }
 
 func mustResolve(t *testing.T, h *harness, path string) *workflow.Workflow {
@@ -891,6 +947,9 @@ func TestM4OverflowHandler(t *testing.T) {
 	if r.Status != StatusStopped || r.Outcome != run.OutcomeOverflow || !m3.View().Snapshot.OverflowHandled || countType(h3.events(m3), run.TypeOverflowHandler) != 1 {
 		t.Fatalf("resume handler: %+v", r)
 	}
+	if o := h3.runner.ordinals; len(o) != 2 || o[0] != 0 || o[1] != 0 {
+		t.Fatalf("ordinals: the crashed call stored no cmd_call, so the resume is ordinal 0 again: %v", o)
+	}
 	if _, err := m3.Advance(context.Background()); !errs.Is(err, CodeRunTerminal) {
 		t.Fatal("terminal after resume")
 	}
@@ -1641,7 +1700,7 @@ func TestM9Residue(t *testing.T) {
 		hb.git.def.Counts = map[string]int{shaHead + ".." + shaHead: 1}
 		allPresent(hb)
 		mb := hb.mustInit(InitOptions{Workflow: "sdlc-loop", Vars: sdlcVars})
-		for _, step := range []func(*harness, *Machine) error{adv, rec("discover", findings(1)), adv, adv, adv, rec("fix", `{"commit":"` + shaFix + `","summary":"s"}`), adv, adv} {
+		for _, step := range []func(*harness, *Machine) error{adv, rec("discover", findings(1)), adv, adv, adv, rec("fix", `{"commit":"`+shaFix+`","summary":"s"}`), adv, adv} {
 			if err := step(hb, mb); err != nil {
 				t.Fatal(err)
 			}
diff --git a/internal/fsm/machine/types.go b/internal/fsm/machine/types.go
index 32377b6..a3758b5 100644
--- a/internal/fsm/machine/types.go
+++ b/internal/fsm/machine/types.go
@@ -72,13 +72,22 @@ type Sidecar interface {
 	List(runID string) ([]string, error)
 }
 
+// RunnerDeps is what the single Guarded factory receives per session.
+type RunnerDeps struct {
+	Allowed  []run.AllowedCmd
+	WorkDir  string
+	RunID    string
+	Audit    func(run.Event) error
+	CmdCalls func(name string) int // prior cmd_call count for a command (mock ordinal)
+}
+
 // Deps wires the machine. Nothing here is optional except Terminal.
 type Deps struct {
 	Store     run.RunStore
 	Sidecar   Sidecar
 	Kinds     Registry
 	Git       func(workDir string) gate.Git
-	Runner    func(allowed []run.AllowedCmd, workDir, runID string, audit func(run.Event) error) converge.Runner
+	Runner    func(RunnerDeps) converge.Runner
 	Clock     Clock
 	LookPath  func(string) (string, error)
 	FileHash  func(string) (string, error)
diff --git a/internal/fsm/workflow/workflow.go b/internal/fsm/workflow/workflow.go
index f6052ed..58d2e48 100644
--- a/internal/fsm/workflow/workflow.go
+++ b/internal/fsm/workflow/workflow.go
@@ -166,7 +166,10 @@ func Parse(raw []byte, opts Options) (*Workflow, error) {
 	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
 	dec.KnownFields(true)
 	if err := dec.Decode(&rw); err != nil {
-		return nil, invalid("unknown_key", "document", err.Error())
+		if strings.Contains(err.Error(), "not found in type") {
+			return nil, invalid("unknown_key", "document", err.Error())
+		}
+		return nil, invalid("bad_yaml", "document", err.Error())
 	}
 	sum := sha256.Sum256(raw)
 	w := &Workflow{
diff --git a/internal/fsm/workflow/workflow_test.go b/internal/fsm/workflow/workflow_test.go
index 15d9423..d513546 100644
--- a/internal/fsm/workflow/workflow_test.go
+++ b/internal/fsm/workflow/workflow_test.go
@@ -175,6 +175,10 @@ func TestW2Reasons(t *testing.T) {
 		src              string
 	}{
 		{"unknown-top-key", "unknown_key", "document", example + "extra: 1\n"},
+		{"bad-yaml-malformed", "bad_yaml", "document", "workflow: [\n"},
+		{"bad-yaml-dup-key", "bad_yaml", "document", example + "workflow: again\n"},
+		{"bad-yaml-scalar-root", "bad_yaml", "document", "just a string\n"},
+		{"bad-version-string", "bad_yaml", "document", edit("version: 1", "version: one")},
 		{"transitions-scalar", "unknown_key", "transitions", scalarTransitions()},
 		{"node-kind-not-string", "unknown_key", "nodes.fix.kind", edit("fix:        { kind: agent-edit }", "fix:        { kind: [agent-edit] }")},
 		{"transition-unknown-field", "unknown_key", "transitions[0]", edit("gate: findings_empty, outcome: clean }", "gate: findings_empty, outcome: clean, extra: 1 }")},


--- docs/tasks/m1-m6-fsm-packages.md
+# M1–M6: internal/fsm core packages
+
+Implement `internal/fsm/{errs,converge,gate,workflow,machine,cmdexec,judge,mockai,kind}` per
+`docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md` (r4) and `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
+(r5), test-first, under the combined coverage gate (`tests/coverage.sh`), reviewed per commit range (≤ 120 KB each).
+
+## Acceptance
+
+- Every §7/§8 test row has a discriminating test (literal pins; goldens regression-only behind an env flag).
+- `go test ./internal/fsm/...` passes; every `internal/fsm/*` package at exactly 100% statements.
+- `bash tests/coverage.sh` passes (legacy floor held).
+- Dependency direction per spec 2 §1 (machine imports no kinds/judge/cmdexec/workflows).
+- Every LLM/shell effect behind an interface; no shell, pinned argv, exact env in `cmdexec`.
```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

coverage gate run after commit 1d6284b (M4/M5/M6 complete):
internal/markdown                                   70.0%  ok
internal/prready                                    85.7%  ok
internal/repo                                       87.9%  ok
internal/reviewers                                  97.2%  ok
internal/reviewlog                                  90.2%  ok
internal/reviewmanifest                             90.5%  ok
internal/reviewstate                                92.1%  ok
internal/runchain                                   90.1%  ok
internal/sessionhistory                             86.2%  ok
internal/setup                                      88.5%  ok
internal/state                                      81.6%  ok
internal/taskdone                                   87.0%  ok
internal/tasksource                                 79.2%  ok
workflows                                          100.0%  ok
coverage gate passed

[exited with code 0]

