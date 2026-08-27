# metareview task-done context

Run ID: `mrv-20260827-083957589704000-task-done-m1-m6-fsm-packages-a99c72f1`

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

- Base: `da6aa55a989146a58cfe65501fa085bc7a596cf1`
- Head: `3e183a964ed0efe80be24b553d6391d393bb36bc`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `48720`
- Filtered diff bytes: `30202`
- Risk level: `none`
- Generated files excluded: docs/metareview/context/mrv-20260827-073257644607000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f-context.md, docs/metareview/context/mrv-20260827-073257743851000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb-context.md, docs/metareview/reviews/mrv-20260827-073257644607000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f.md, docs/metareview/reviews/mrv-20260827-073257743851000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb.md



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `1fe2d72a362ac358`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
- docs/tasks/m1-m6-fsm-packages.md
- internal/fsm/machine/machine_test.go

### Path Dispositions
- docs/metareview/context/mrv-20260827-073257644607000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260827-073257743851000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260827-073257644607000-artifact-2026-08-27-metareview-0-9-0-fsm-core-a0b8592f.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260827-073257743851000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb.md: generated (metareview generated review artifact excluded from source manifest)

### Shards
- shard-01: docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md, docs/tasks/m1-m6-fsm-packages.md, internal/fsm/machine/machine_test.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
- internal/fsm/machine/machine_test.go
- docs/tasks/m1-m6-fsm-packages.md

## Diff

````diff
diff --git a/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md b/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
index aa33bcc..2b04a29 100644
--- a/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
+++ b/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
@@ -48,7 +48,8 @@
 ```
 internal/fsm/errs       Error{Code, Detail, Fields}; E, Wrap, Is, Code, As                       (implemented)
 internal/fsm/converge   AllFixed; Result; Predicate; Validate/Parse; atoms; Payload; CmdResult/Runner  (implemented)
-internal/fsm/gate       Git (exec seam + Fake); ValidSHA/ValidRef; 7 gates; TreeHash; Cut          (implemented)
+internal/fsm/gate       Git (exec seam + Fake); ValidSHA/ValidRef; 9 gates; TreeHash; Cut          (implemented)
+workflows/              embedded shipped YAMLs: Names(), Read(name)                                  (implemented, M0)
 internal/fsm/workflow   YAML → Workflow; validation; $VAR resolution; ResolveCmds/VerifyCmds + cmds_sha256
 internal/fsm/machine    Deps; Init/Open/Advance/Record/View; consumed interfaces; Sidecar (FS + Mem)
 ```
@@ -95,9 +96,9 @@ nodes:
   fix:        { kind: agent-edit }
   verify:     { kind: still-present, model: $JUDGE, effort: $JUDGE_EFFORT }
 transitions:                                               # list form (shipped)
-  - { from: discover, to: done, gate: findings_empty, outcome: clean }
+  - { from: discover, to: done, gate: nothing_found, outcome: clean }
   - { from: discover, to: adjudicate, gate: findings_nonempty }
-  - { from: adjudicate, to: done, gate: confirmed_empty, outcome: clean }
+  - { from: adjudicate, to: done, gate: nothing_confirmed, outcome: clean }
   - { from: adjudicate, to: fix, gate: confirmed_nonempty }
   - { from: fix, to: verify, gate: commit_exists }
   - { from: verify, to: done, gate: all_fixed, outcome: fixed }
@@ -113,14 +114,15 @@ accepted and ignored. `nodes.<state>`: `kind` + optional `exec`, `model`, `effor
 (names). Node `cmd:`, `on_overflow:`, and `{cmd: <name>}` atoms reference `cmds` by name. Unknown top-level keys →
 `unknown_key`. `$NAME` grammar: `\$([A-Z_][A-Z0-9_]*)` (longest match; `$JUDGE_EFFORT` is one token); `$$` is a literal
 `$`; any other `$` (`$1`, `${X}`, a trailing `$`) is left literal. Substitution covers `model`, `effort`, top-level string
-params and strings inside list params (nested maps are not walked), and every argv element. Shipped YAMLs: `sdlc-loop` gained `discover→done findings_empty (clean)` and
-`adjudicate→done confirmed_empty (clean)` rows and the r2-form escape-hatch comment (ledger); `review-loop` unchanged.
+params and strings inside list params (nested maps are not walked), and every argv element. Shipped YAMLs: `sdlc-loop` gained `discover→done nothing_found (clean)` and
+`adjudicate→done nothing_confirmed (clean)` rows, omits `exec` on `fix`/`verify` (inferred), and carries the r2-form
+escape-hatch comment (ledger); `review-loop` unchanged.
 
 ### 2.3 Static validation (Parse) — `ERR_WORKFLOW_INVALID{reason, at}`, first failure in this order
 | reason | rule |
 |---|---|
 | `missing_kinds` | `Options.Kinds` nil/empty |
-| `bad_yaml` | the document does not decode (malformed YAML, duplicate mapping keys, non-mapping root); `at = document` |
+| `bad_yaml` | the document does not decode (malformed YAML, duplicate mapping keys, non-mapping root, wrong scalar type such as `version: one`); `at = document` |
 | `unknown_key` | unknown key at the top level, inside a `cmds.<name>` or transition entry, or a reserved node key with a non-string value |
 | `missing_name`, `bad_version` | `workflow` non-empty; `version == 1` |
 | `no_initial`, `bad_state` | `states` non-empty; each `^[a-z][a-z0-9_-]{0,31}$`, unique, `judge` reserved (spec 5's `fsm judge` node) |
@@ -200,6 +202,7 @@ Exit ≥ 2 → `ERR_GIT{op, exit}` with stderr; exit 1 is an answer only for `Is
 | `confirmed_nonempty` / `confirmed_empty` | `len(Confirmed) > 0` / `== 0` | `ERR_NO_CONFIRMED` / `ERR_CONFIRMED_PRESENT` |
 | `commit_exists` | `FixEntryHead == ""` → `ERR_GATE_INAPPLICABLE`; `CommitCount(FixEntryHead, Head) > 0 && clean` | `ERR_NO_COMMIT`, Detail = count + porcelain + `WorkingDiff(MaxDetail)` via `CapDetail`; any Git failure → `ERR_GIT` gate error (evidence in the audit; recovery by fork — ledgered) |
 | `all_fixed` / `bugs_remain` | `converge.AllFixed` / `!` | `ERR_BUGS_REMAIN` / `ERR_ALL_FIXED` |
+| `nothing_found` / `nothing_confirmed` | `len(AllFound) == 0 && len(Findings|Confirmed) == 0` — the iteration-0 clean exits; refuse once any bug is known | `ERR_BUGS_KNOWN` (bugs known) / `ERR_FINDINGS_PRESENT` · `ERR_CONFIRMED_PRESENT` |
 
 ## 4. `converge` (implemented)
 ```go
@@ -247,7 +250,9 @@ type Sidecar interface {
 }
 // Sidecar names: ^[a-z][a-z0-9._-]{0,63}$, never `audit.*` or `lock`; runID must pass run.ValidateRunID. FS impl under <root>/.metareview/runs/<id>/; Mem for tests.
 ```
-`Audit` appends immediately (durable) and rebinds the machine's state; it returns store errors so the executor stops.
+`Audit` appends immediately (durable) and rebinds the machine's state; it returns store errors so the executor stops,
+and the machine remembers that store error: when `Execute`/`Runner.Run` then fails, the store error is returned unchanged
+(no pseudo-gate, no handler warn) so the next `Advance` resumes.
 Executors number `llm_call.Index` from `StartIndex` (the fold's next index for the key), so an interrupted execution
 resumes with a continuing index and its earlier spend stays audited. `Execute` is never retried by the machine inside
 one `Advance`; a failure → `executor` pseudo-gate (§5.4 step 5), except `ctx.Err() != nil` (interrupt), which `Advance`
@@ -257,7 +262,7 @@ returns unchanged so the next `Advance` resumes from `StartIndex`.
 ```go
 type Deps struct {
     Store run.RunStore; Sidecar Sidecar; Kinds Registry; Git func(workDir string) gate.Git
-    Runner func(allowed []run.AllowedCmd, workDir, runID string, audit func(run.Event) error) converge.Runner
+    Runner func(RunnerDeps) converge.Runner   // RunnerDeps{Allowed, WorkDir, RunID, Audit, CmdCalls func(name) int /* prior cmd_call count: the mock ordinal */}
     Clock Clock; LookPath func(string) (string, error); FileHash func(string) (string, error)
     Workflows func(name string) ([]byte, error); ReadFile func(string) ([]byte, error); Nonce func() string
     MockLoad func(dir string) (hash string, err error); Terminal func(ctx, View) error   // spec 3's runs.jsonl record; nil → no-op
@@ -267,18 +272,20 @@ type OpenOptions struct { Repair bool }
 // Typed string sets: Status ∈ {ADVANCED, NEEDS_INPUT, DONE, STOPPED, GATE_FAILED}; NextAction ∈ {advance, record, none}; RecordOptions.Kind ∈ {node-output, tokens, event} — exported constants.
 func Init(ctx, Deps, InitOptions) (*Machine, error); func Open(ctx, Deps, runID string, OpenOptions) (*Machine, error)
 func (m *Machine) Advance(ctx) (AdvanceResult, error); func (m *Machine) Record(ctx, RecordOptions) (RecordResult, error); func (m *Machine) View() View
-type AdvanceResult struct { Status string; From, To run.State; Gate *run.GateData /* first failing */; Outcome run.Outcome; StopReason string; NeedsInput *NeedsInput; Warnings []string /* warn events appended by this call */; Untrusted []string /* "gate.detail","warnings","stop_reason","error.detail" when non-empty — every Detail may carry repo/third-party bytes */; ExitCode int; RunID string }
+type AdvanceResult struct { Status string; From, To run.State; Gate *run.GateData /* first failing */; Outcome run.Outcome; StopReason string; NeedsInput *NeedsInput; Warnings []string /* warn events appended by this call */; Untrusted []string /* "gate.detail","warnings","stop_reason" when non-empty; every returned error's Detail is untrusted too and spec 5 marks it "error.detail" */; ExitCode int; RunID string }
 type NeedsInput struct { Node string; Kind, Exec, Model, Effort string; Instructions Instructions; Record string }
 type RecordOptions struct { Kind string /* node-output|tokens|event */; Node string; Data json.RawMessage; Replace bool; Name string }
-type RecordResult struct { Seq int64; Type run.EventType; Key string }
-type View struct { RunID, Workflow string; Snapshot run.Snapshot; Node *NodeView; NextAction string /* advance|record|none */; Torn bool; FailedGate *run.GateData /* last gate{passed:false} before a failed transition */ }
+type RecordResult struct { Seq int64; Type string; Key string }
+type View struct { RunID, Workflow string; Snapshot run.Snapshot; Node *NodeView; NextAction string /* none when terminal; record when the node is host-executed, its output is absent, and a needs_input for the key exists; else advance */; Torn bool; FailedGate *run.GateData /* last gate{passed:false} in the log when Outcome == failed */ }
+func (m *Machine) RunID() string
 type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
 ```
 
 ### 5.3 `Init`
 1. Load YAML: name → `Deps.Workflows` (`ERR_WORKFLOW_NOT_FOUND`); path (`/` or `.yaml`) → `ReadFile`, ≤ 256 KB
    (`ERR_WORKFLOW_TOO_LARGE`). `Parse(raw, Options{Kinds: Kinds.Info()})`; `Resolve(vars, calibration)`.
-   `RepoMode` override must be `""` or `enforcing` (tighten-only; `advisory` over a workflow's `enforcing` → `ERR_BAD_REPO_MODE`).
+   `RepoMode` override must be `""` or `enforcing` (tighten-only; anything else → `ERR_BAD_REPO_MODE`); `Open` applies the
+   stored `snap.RepoMode` over the sidecar's. `WorkDir` and `RepoRoot` must be absolute (`ERR_WORKDIR_FOREIGN{reason: relative}`).
    Git failures during `Init`/`Advance` outside gates are returned as the `ERR_GIT`/`ERR_GIT_REF` error unchanged (retryable);
    only gates convert Git failures into gate errors.
 2. `ResolveCmds`; `len(Cmds) > 0 && AllowCustomCmds != sha` → `ERR_CMDS_NOT_ALLOWED{sha}`, Detail = the printed list
@@ -287,7 +294,8 @@ type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
    `BaseSHA`; `Status` + `WorkTree` → `TreeHash`.
 4. Goldens: `ReadFile` ≤ 512 KB, JSON array of `run.Golden` with `DisallowUnknownFields`, ≤ `MaxGoldens` → else
    `ERR_GOLDENS_INVALID{path}`.
-5. Mock: `MockDir` (made relative to `RepoRoot`) → `MockLoad` → `Mock = rel + "#" + hash[:16]` (`ERR_MOCK_INVALID`);
+5. Mock: `MockDir` (relative → under `RepoRoot`; must be inside `RepoRoot` else `ERR_MOCK_INVALID{reason: outside}`) →
+   `MockLoad` → `Mock = rel + "#" + hash[:16]` (`ERR_MOCK_INVALID`);
    `Kinds.Mock()` must equal `MockDir != ""` (`ERR_MOCK_MISMATCH`).
 6. `runID` ← `RunID` or `run.RunID(w.Name, Clock().Time)`; **`run.Create` first**, then `Sidecar.Write(runID,
    "workflow.yaml", raw)` (failure → the error; the run exists without a sidecar and `Open` reports `ERR_SIDECAR`); then
@@ -345,8 +353,9 @@ type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
        if chosen == nil:
            r, err ← pred.Evaluate(snap)                                        (always when tt failed — bounded loops)
            err → append gate{Name:"converge", Passed:false, Error:{ERR_CONVERGE_FAILED, Detail}} → step 8
-           r.Class == fixed → same, ERR_CONVERGE_FAILED{reason: fixed_class}   (defense in depth over converge's placement rule)
-           append converge{Atom: r.Atom, Class: r.Class, Stop: r.Stop, Reason}
+           r.Stop && r.Class == fixed && r.Atom != "all_fixed" → same, ERR_CONVERGE_FAILED{reason: fixed_class}   (defense in depth; the real
+           all_fixed atom firing is legitimate — design §9's example — and a non-firing result's class is irrelevant)
+           append converge{Atom: CapText(r.Atom, MaxShort), Class: r.Class, Stop: r.Stop, Reason: CapText(MaxText)}
            if r.Stop: chosen ← synthetic {From: state, To: tt.To, Gate: r.Atom, Outcome: r.Class}
        if chosen == nil: for t in Outgoing(state) except tt: err ← gate(t.Gate)(snap); append gate; pass → chosen ← t; break; else failures += err
    else: for t in Outgoing(state): err ← gate(t.Gate)(snap); append gate; pass → chosen ← t; break; else failures += err
@@ -370,7 +379,8 @@ node-scoped events (`needs_input`, `node_output`, `delta_applied`, `llm_call`);
 - `node-output`: not terminal (`ERR_RUN_TERMINAL`); state has a node and `Node == node.Name` (`ERR_NODE_MISMATCH`); exec
   `inline|subagent` (`ERR_NODE_NOT_HOST`); `!Applied[k]` (`ERR_NODE_OUTPUT_APPLIED`); `NodeOutputs[k]` absent unless
   `Replace` (`ERR_NODE_OUTPUT_EXISTS`); `Decode` ok (`ERR_NODE_OUTPUT_INVALID`, nothing appended); append `node_output`.
-- `tokens`: `run.TokenTotals`, `DisallowUnknownFields`, no negative field → else `ERR_RECORD_TOKENS`; append `tokens`.
+- `tokens`: `run.TokenTotals`, `DisallowUnknownFields`, no negative field, no field above `MaxTokenCounter` → else `ERR_RECORD_TOKENS`; append `tokens`.
+- any other `Kind` → `ERR_RECORD_NAME{reason: kind}`.
 - `event`: `Name` ~ `^[a-z][a-z0-9_-]{0,63}$`, not a run event type, not `mrv_*` → else `ERR_RECORD_NAME{reason:
   syntax|event_type|reserved}`; `Data ≤ MaxPayload − 128` (`ERR_RECORD_TOO_LARGE`); append `record{name, data}`.
 
@@ -406,14 +416,14 @@ behind `FSM_MACHINE_UPDATE_GOLDEN=1` with the run package's "drift ≠ regenerat
 | pkg | rows |
 |---|---|
 | workflow | W0 order row: a document with an unknown top-level key **and** `version: 2` → `unknown_key` (first failure in table order). W1 both shipped YAMLs + the §2.2 example + a mapping-form twin of review-loop (order preserved, `*→failed` ignored, `->` and `→`): assert `Transitions`, `Nodes` (exec defaulted for `fix`/`verify`), `Cmds` (`Timeout 30s`, default `60s`), `Refs`/`CmdRefs` literals, `Hash` literal, `Warnings` empty; W2 one fixture per reason **and per sub-rule** (each one edit from a valid base): `bad_cmd` ×8 (argv empty / non-string / empty element / timeout non-integer / 0 / 3601 / name regex / > MaxAllowedCmds / > MaxArgv), `bad_env` one per reserved literal and prefix + regex + duplicate + count, `bad_var` ×3, `bad_state` ×3 (incl. `judge`), `failed_reserved` ×3 (undeclared / has node / in a transition), `duplicate_transition` (same `(from, gate)`, different `to`), `loop_terminal` ×2, `unknown_cmd` ×3, `cmd_without_kind` ×2, `unknown_var` ×3 (node, cmd, list param), `bad_outcome` ×2 (`great`, `failed`), `bad_yaml` ×2 (malformed, duplicate key), `bad_version` (`one`), `initial_terminal`, `bad_params`, `missing_kinds`; acceptance boundaries: timeout 1 and 3600, 32-char state, `MaxVars`/`MaxEnv`/`MaxArgv`/`MaxAllowedCmds` at cap; assert `reason` + `at`; W3 `Resolve`: `$JUDGE`/`$JUDGE_EFFORT` prefix pair → `Model=="a"`, `Effort=="b"`; caller value beats `Default`; list params substituted, non-string list elements and nested maps untouched (literal asserts), `$1`/`${X}` left literal; `Refs`/`CmdRefs` from a list param; `$$` literal; `$JUDGEX` → `ERR_VAR_UNKNOWN`; calibration pins asserted as literals; caller `FOO` → `ERR_VAR_UNKNOWN`; required unset; calibration refuses caller `JUDGE` **and** `JUDGE_EFFORT`; calibration on a workflow without `JUDGE` var is a no-op; re-resolve of stored pinned vars succeeds; argv substitution; W4 `ResolveCmds`: fake lookPath/hash; `argv[0]` rewritten absolute (`bash` → `/bin/bash`, `./s.sh` → `<workDir>/s.sh`), relative lookPath result → `ERR_CMD_NOT_FOUND`; closure over `["bash","./s.sh"]` + absolute path; `/bin/bash` itself appears in `FileHashes`; non-nil empty map; **hand-authored preimage** (`testdata/cmds-preimage.json` + `.sha256` from `shasum`) with two cmds declared out of order, `TimeoutMS 1500`, `Env` set; one-byte edit → different sha; `VerifyCmds` mismatch/missing/appeared; a directory argv element is not hashed; **no re-resolution**: after pinning `/bin/bash`, point `lookPath` elsewhere and edit the pinned file → `ERR_CMD_CHANGED{/bin/bash, mismatch}`, and the inverse (pinned intact, lookPath moved) → no error; non-absolute `workDir` refused; W5 `VarsReferencedBy` (node ∪ cmd, sorted, resolved copy), `Outgoing`, `IsTerminal` incl. `failed`, `LoopTransition`, `TerminalFor`, `loop_without_clean_exit` warning |
-| machine | M0 fakes: `gate.FailingFake{Fake; FailAt string}` (exported from `gate`, fails exactly one method) drives per-call-site `ERR_GIT{op}` rows for every Git call in `Init`/`Advance`; a counting store fails `Lock`, `EventsWithLines`, or append #N; seam row: `Fake.Diffs["<base>..<head>"] = "D"` with a small `MaxDiffBytes` → the fake kind/executor observe `diff.Text == "D"`, `Truncated == true`, `nonce == "n1"`. M1 `Init`: hand-written expected sequence `[init(no stamps), tree(State=initial, Iter 0), warn?]` with every `InitData` field asserted literally (embedded + path workflows; `workflow.yaml` sidecar bytes == raw; `Create` before sidecar observed via a fake store/sidecar call log; `ERR_RUN_EXISTS` leaves the victim's sidecar intact); `ERR_WORKFLOW_NOT_FOUND`, `ERR_WORKFLOW_TOO_LARGE`; goldens ok/unknown field/over cap/over bytes; consent list as a hand-written literal (pinned/unpinned marks, env **names** only, no process env values) + sha in Detail, wrong sha, no cmds; `ERR_MOCK_INVALID`; `RepoMode` override `enforcing` accepted / `advisory` over `enforcing` refused; `RevParse` base (`main` → sha); `ERR_WORKDIR_FOREIGN`; `ERR_BAD_REPO_MODE`; mock hash pinned + `Kinds.Mock()` mismatch; unknown `--var`; M2 `Advance` on both shipped workflows with a fake Registry: hand-written expected event-type sequences per path (`review-loop` clean/reviewed; `sdlc-loop` clean at discover, clean at adjudicate, fixed after 1 iteration, loop once then fixed), literal asserts on transition fields, `needs_input` once across `advance, record tokens, advance` and again at `discover@1`, `View.NextAction` per step; goldens regression-only; M3 gate failure: two failing gates → `Gate` is the first in evaluation order and `transition.Gate` names it; loop-boundary variant (tt and the loop gate both fail → tt named); two passing gates → the first taken; `ERR_INSTRUCTIONS_FAILED` (needs_input already present stays, nothing further); ctx cancelled during `Execute` → error returned, no pseudo-gate, next `Advance` resumes with `StartIndex`; `ERR_GATE_INAPPLICABLE`; executor error → `executor` pseudo-gate with earlier `llm_call`s kept and `StartIndex` honoured on the next fork (interrupted-execution fixture: pre-seeded `llm_call` index 0, executor asserts `StartIndex == 1`); decode error / Reduce error / rejected `delta_applied` (status subset from a fake cmd kind) → `node_output` pseudo-gate; M4 loop: cumulative regression (iter 3 fixes its own bug, 7 remain: loop taken, `AllFound == 8`, `Unfixed == 7`, all 8 statuses, not `fixed`); **gate-first**: `max_iterations: 1` with all bugs fixed at verify → `fixed`, zero `converge` events; negative control one bug left → `converge{max_iterations}` → `overflow`; `stalled` via nil-then-plateau and via regression (`Prev 3 → Unfixed 5`); `budget` via `llm_call` tokens and via `record tokens`; `custom` via cmd atom; converge error → `converge` pseudo-gate and no loop taken; `fixed_class` guard via a fake predicate; user workflow whose terminal gate is `confirmed_empty` (not `all_fixed`) with all bugs fixed and findings present → convergence evaluated (bounded) — the `max_iterations` stop fires; emitter caps: a 5 KB cmd-atom reason → `converge.Reason` and `StopReason` capped; overflow handler once, `overflow_handler` fields literal, failure warn, **not** run for `stalled`/`custom`, resumed after a crash (fixture: terminal overflow run without handler → `Advance` runs it, then `ERR_RUN_TERMINAL`); M5 tree: identical porcelain + different `WorkTree` → advisory warn (+ tree) vs enforcing `repo_mode` gate with **no** tree (a second `Advance` re-detects); porcelain ≈ 5 KB → warn Detail capped at `MaxText`, tree Status intact; 70 KB porcelain → `tree.Status` capped with flag; agent-edit exempt; baseline `tree` appended when `TreeHash == ""` (fork-from-initial fixture); `tree` only on change (count); M6 `Record` refusals per code and sub-reason (`syntax`, `event_type` (`transition`), `reserved`), `ERR_RECORD_TOKENS` on unknown field and on `-1`, `Replace`, terminal `tokens` allowed, `ERR_NODE_OUTPUT_INVALID` leaves `Events` byte-identical; M7 `Open`: `ERR_WORKFLOW_CHANGED` via sidecar edit; embedded bytes replaced by a workflow with different transitions while the sidecar is intact → `Advance` follows the **sidecar's** transitions; `ERR_CMD_CHANGED`; `ERR_MOCK_MISMATCH` via scenario edit and via registry mismatch; torn → `ERR_AUDIT_TORN`; `Repair` → warn Detail literal + fold ok; `Repair` at offset 0 → `ERR_RUN_NOT_FOUND`; `ERR_SIDECAR{missing}`; M8 stamps: every event's `At` equals the injected clock sequence, `State/Iter/Mock/Node` per §5.4 tail (`cmd_call` has no Node), non-mock runs never carry `Mock: true` (`MockTainted == false`), mock runs carry it on every non-init event; M9 §5.7 table incl. `StopReason` ("atom: reason"), `Untrusted` list, `Deps.Terminal` called for every terminal outcome incl. `failed`, again on a later `Advance` of a terminal run (idempotency is spec 3's), and on the 9b resume path; `Terminal` error returned with the transition durable; `ERR_AUDIT_FULL` surfaced; a counting store that fails append #N for every N of the happy sequence returns the error unchanged; FS `Sidecar`: symlink refused, exists refused, mode 0600, missing run → `ERR_SIDECAR` |
+| machine | M0 fakes: a per-call-site failing Git wrapper (`failingGit{Git; at}` in `machine/harness_test.go`, fails exactly one method) drives per-call-site `ERR_GIT{op}` rows for every Git call in `Init`/`Advance`; a counting store fails `Lock`, `EventsWithLines`, or append #N; seam row: `Fake.Diffs["<base>..<head>"] = "D"` with a small `MaxDiffBytes` → the fake kind/executor observe `diff.Text == "D"`, `Truncated == true`, `nonce == "n1"`. M1 `Init`: hand-written expected sequence `[init(no stamps), tree(State=initial, Iter 0), warn?]` with every `InitData` field asserted literally (embedded + path workflows; `workflow.yaml` sidecar bytes == raw; `Create` before sidecar observed via a fake store/sidecar call log; `ERR_RUN_EXISTS` leaves the victim's sidecar intact); `ERR_WORKFLOW_NOT_FOUND`, `ERR_WORKFLOW_TOO_LARGE`; goldens ok/unknown field/over cap/over bytes; consent list as a hand-written literal (pinned/unpinned marks, env **names** only, no process env values) + sha in Detail, wrong sha, no cmds; `ERR_MOCK_INVALID`; `RepoMode` override `enforcing` accepted / `advisory` over `enforcing` refused; `RevParse` base (`main` → sha); `ERR_WORKDIR_FOREIGN`; `ERR_BAD_REPO_MODE`; mock hash pinned + `Kinds.Mock()` mismatch; unknown `--var`; M2 `Advance` on both shipped workflows with a fake Registry: hand-written expected event-type sequences per path (`review-loop` clean/reviewed; `sdlc-loop` clean at discover, clean at adjudicate, fixed after 1 iteration, loop once then fixed), literal asserts on transition fields, `needs_input` once across `advance, record tokens, advance` and again at `discover@1`, `View.NextAction` per step; goldens regression-only; M3 gate failure: two failing gates → `Gate` is the first in evaluation order and `transition.Gate` names it; loop-boundary variant (tt and the loop gate both fail → tt named); two passing gates → the first taken; `ERR_INSTRUCTIONS_FAILED` (needs_input already present stays, nothing further); ctx cancelled during `Execute` → error returned, no pseudo-gate, next `Advance` resumes with `StartIndex`; `ERR_GATE_INAPPLICABLE`; executor error → `executor` pseudo-gate with earlier `llm_call`s kept and `StartIndex` honoured on the next fork (interrupted-execution fixture: pre-seeded `llm_call` index 0, executor asserts `StartIndex == 1`); decode error / Reduce error / rejected `delta_applied` (status subset from a fake cmd kind) → `node_output` pseudo-gate; M4 loop: cumulative regression (iter 3 fixes its own bug, 7 remain: loop taken, `AllFound == 8`, `Unfixed == 7`, all 8 statuses, not `fixed`); **gate-first**: `max_iterations: 1` with all bugs fixed at verify → `fixed`, zero `converge` events; negative control one bug left → `converge{max_iterations}` → `overflow`; `stalled` via nil-then-plateau and via regression (`Prev 3 → Unfixed 5`); `budget` via `llm_call` tokens and via `record tokens`; `custom` via cmd atom; converge error → `converge` pseudo-gate and no loop taken; `fixed_class` guard via a fake predicate; user workflow whose terminal gate is `confirmed_empty` (not `all_fixed`) with all bugs fixed and findings present → convergence evaluated (bounded) — the `max_iterations` stop fires; emitter caps: a 5 KB cmd-atom reason → `converge.Reason` and `StopReason` capped; overflow handler once, `overflow_handler` fields literal, failure warn, **not** run for `stalled`/`custom`, resumed after a crash (fixture: terminal overflow run without handler → `Advance` runs it, then `ERR_RUN_TERMINAL`); M5 tree: identical porcelain + different `WorkTree` → advisory warn (+ tree) vs enforcing `repo_mode` gate with **no** tree (a second `Advance` re-detects); porcelain ≈ 5 KB → warn Detail capped at `MaxText`, tree Status intact; 70 KB porcelain → `tree.Status` capped with flag; agent-edit exempt; baseline `tree` appended when `TreeHash == ""` (fork-from-initial fixture); `tree` only on change (count); M6 `Record` refusals per code and sub-reason (`syntax`, `event_type` (`transition`), `reserved`), `ERR_RECORD_TOKENS` on unknown field and on `-1`, `Replace`, terminal `tokens` allowed, `ERR_NODE_OUTPUT_INVALID` leaves `Events` byte-identical; M7 `Open`: `ERR_WORKFLOW_CHANGED` via sidecar edit; embedded bytes replaced by a workflow with different transitions while the sidecar is intact → `Advance` follows the **sidecar's** transitions; `ERR_CMD_CHANGED`; `ERR_MOCK_MISMATCH` via scenario edit and via registry mismatch; torn → `ERR_AUDIT_TORN`; `Repair` → warn Detail literal + fold ok; `Repair` at offset 0 → `ERR_RUN_NOT_FOUND`; `ERR_SIDECAR{missing}`; M8 stamps: every event's `At` equals the injected clock sequence, `State/Iter/Mock/Node` per §5.4 tail (`cmd_call` has no Node), non-mock runs never carry `Mock: true` (`MockTainted == false`), mock runs carry it on every non-init event; M9 §5.7 table incl. `StopReason` ("atom: reason"), `Untrusted` list, `Deps.Terminal` called for every terminal outcome incl. `failed`, again on a later `Advance` of a terminal run (idempotency is spec 3's), and on the 9b resume path; `Terminal` error returned with the transition durable; `ERR_AUDIT_FULL` surfaced; a counting store that fails append #N for every N of the happy sequence returns the error unchanged; FS `Sidecar`: symlink refused, exists refused, mode 0600, missing run → `ERR_SIDECAR` |
 
 ## 8. Ledger
 - `cmds:` single top-level declaration referenced by name; per-cmd `timeout`/`env` are consent-covered (design §16 inline argv retired).
 - `failed` reserved; `duplicate_transition` on `(from, gate)`; loop safety reasons; `bad_state` (`judge` reserved for spec 5); `bad_env` reserved names.
 - Loop boundary is order-independent: `TerminalFor` gate first, convergence only when `!AllFixed`, then the loop gate and remaining transitions (C3 gate-first, made structural).
 - Converge errors are the `converge` pseudo-gate; enforcing edits, executor and decode failures are `repo_mode`/`executor`/`node_output` pseudo-gates; the failed transition names the first failing gate.
-- `needs_input` once per key; `tree` at `Init` and on change (content-aware `WorkTree`; agent-edit states may emit one per advance while the agent edits — accepted).
+- `needs_input` once per key; `tree` at `Init`, as a baseline when `TreeHash == ""`, and on change (content-aware `WorkTree`; agent-edit states may emit one per advance while the agent edits — accepted).
 - `commit_exists` = `FixEntryHead..HEAD` + `ERR_GATE_INAPPLICABLE` (SCP3-5); Git failures inside gates are gate errors (recovery by fork) — accepted.
 - `Open` verifies the run's `workflow.yaml` sidecar (written after `Create`, `O_EXCL`); forks copy the parent's sidecar (spec 3 r2 obligation; also `Export` includes it).
 - `ERR_RECORD_NAME` narrows locked C15 (reserved names refused; plan E13's `record transition` row becomes an `ERR_RECORD_NAME` row in spec 5).
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index e2a061e..f1ebc44 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -777,9 +777,11 @@ func TestM4Convergence(t *testing.T) {
 	h = newHarness(t)
 	var names, decls []string
 	for i := 0; i < 32; i++ {
-		n := "cmd-" + strings.Repeat("x", 25) + string(rune('a'+i/10)) + string(rune('0'+i%10))
+		n := "cmd-" + strings.Repeat("x", 26) + string(rune('a'+i%16)) // 16 declared names, each referenced twice
 		names = append(names, "{cmd: "+n+"}")
-		decls = append(decls, "  "+n+": {argv: [bash, -c, echo]}")
+		if i < 16 {
+			decls = append(decls, "  "+n+": {argv: [bash, -c, echo]}")
+		}
 	}
 	wf = sdlcWith(t, h, "wide.yaml", "  any: [no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]\nrepo_mode: advisory",
 		"  all: ["+strings.Join(names, ", ")+"]\ncmds:\n"+strings.Join(decls, "\n")+"\nrepo_mode: advisory")


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
````

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

