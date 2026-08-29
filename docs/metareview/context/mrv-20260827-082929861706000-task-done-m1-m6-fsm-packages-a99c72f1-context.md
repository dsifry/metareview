# metareview task-done context

Run ID: `mrv-20260827-082929861706000-task-done-m1-m6-fsm-packages-a99c72f1`

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

- Base: `07bdc1f4b47740066107f86a6c53ee35c103936b`
- Head: `f2301db770ede277c4f214babde09a7a52abc315`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `129626`
- Filtered diff bytes: `129626`
- Risk level: `context-risk`
- Risk reasons: `DIFF_TRUNCATED`, `LARGE_DIFF`

## Context Shard Plan

- Source diff hash: `68017121165f5e69`
- shard-01: docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md, docs/tasks/m1-m6-fsm-packages.md, internal/fsm/converge/converge.go, internal/fsm/converge/converge_test.go, internal/fsm/machine/sidecar.go, internal/fsm/machine/types.go (51736 bytes, prompt pack `docs/metareview/shards/68017121165f5e69-shard-01.md`)
- shard-02: docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md (37511 bytes, prompt pack `docs/metareview/shards/68017121165f5e69-shard-02.md`)
- shard-03: internal/fsm/machine/machine.go (31638 bytes, prompt pack `docs/metareview/shards/68017121165f5e69-shard-03.md`)

## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `90f14b0a3ab0ca0b`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
- docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
- docs/tasks/m1-m6-fsm-packages.md
- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/sidecar.go
- internal/fsm/machine/types.go

### Shards
- shard-01: docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md, docs/tasks/m1-m6-fsm-packages.md, internal/fsm/converge/converge.go, internal/fsm/converge/converge_test.go, internal/fsm/machine/sidecar.go, internal/fsm/machine/types.go
- shard-02: docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
- shard-03: internal/fsm/machine/machine.go

### Manifest Blockers
- missing cross-shard result
- missing shard result for shard-01
- missing shard result for shard-02
- missing shard result for shard-03

## Changed Files

- docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
- docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/machine/machine.go
- internal/fsm/machine/sidecar.go
- internal/fsm/machine/types.go
- docs/tasks/m1-m6-fsm-packages.md

## Diff

````diff
diff --git a/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md b/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
index 5005763..aa33bcc 100644
--- a/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
+++ b/docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md
@@ -1,6 +1,6 @@
 # metareview 0.9.0 — spec 2: workflow, gates, convergence, and the machine core
 
-> **Status:** DRAFT r3 (2026-08-27). Second of the five split 0.9.0 artifacts (ownership ledger: run spec §12).
+> **Status:** r4 — BUILD BASELINE after ESCALATION (2026-08-27). Attempt 3 (`mrv-20260827-070456832125000-…`) ended NEEDS_REVISION on 5/8 lenses with every blocker mechanical; per the same-target rule the chain is ESCALATED and a human must accept this r4 (Dave's "proceed" precedent for the run spec is applied provisionally so the unattended build can continue). r3 note: Second of the five split 0.9.0 artifacts (ownership ledger: run spec §12).
 > Builds on `internal/fsm/run` (r4 + the §9 amendments below, implemented, 100%). Owns plan r3 §1.1 (layout), §1.3
 > gate/converge/workflow types, §1.5 `Advance`, `Record`, §1.6 outcomes (§5.7), §6 shipped workflows, the canonical
 > `cmds_sha256` preimage, the `PrevUnfixed == nil` rule, `tree` cadence + `TreeHash` preimage + `UNSANCTIONED_EDIT`,
@@ -16,9 +16,26 @@
 > `bad_env`, `bad_params`, `missing_kinds` reasons; negative tokens refused; `Deps.Terminal` hook; every test row
 > names its discriminating fixture and the authority is hand-written, goldens regression-only.
 >
-> **Open for Dave (not blocking the build):** D1 — `AllFixed` requires a non-empty `AllFound`, so a user loop whose
-> discover finds nothing ends `overflow`/exit 1 unless the workflow has a `findings_empty`/`confirmed_empty` exit (the
-> shipped `sdlc-loop` now has both). Parse warns when a loop-carrying workflow lacks such an exit. Accept or redesign.
+> **r4 changes (attempt 3, all mechanical; code already reflects them):** `all_fixed` legal only at the top level or as a
+> direct child of a top-level `any`, `not` always classes `custom`, tree depth ≤ 4 / width ≤ 32 (`converge`); the loop
+> boundary evaluates convergence whenever the terminal gate fails (no `!AllFixed` guard); `sdlc-loop`'s clean exits use
+> the new `nothing_found`/`nothing_confirmed` gates (iteration-0 only; a later discovery miss is `GATE_FAILED`, visible);
+> `Deps.Terminal` is idempotent and runs on every terminal advance (incl. `failed`, incl. resume); `tree` baseline when
+> `TreeHash == ""`; enforcing edits append the gate and **no** tree; `first` = pseudo-gate else evaluation order; `Open`
+> algorithm (§5.3b); `tree.Status` capped; `Workflow.Hash` preimage; `initial_terminal`, `unknown_var`, `bad_yaml` reasons
+> in order; `RepoMode` override tighten-only; token counters capped per record (`tokens_too_large`); `GIT_*` scrubbed by
+> prefix and excludes/attributes disabled for `WorkTree`; `Snapshot.Clone` keeps `TimeoutMS/Env`; `CmdsSHA256` uses
+> `run.MarshalCanonical`; `Sidecar.Read` capped + `O_NOFOLLOW` + `ValidateRunID`; `Untrusted` includes `error.detail`;
+> ctx cancellation is returned, never a pseudo-gate; sidecar obligation for forks restated (child's own bytes).
+>
+> **Open for Dave — D1 (`AllFixed` needs a non-empty `AllFound`):**
+>
+> | option | loop state reached with nothing ever found | discover finds nothing at iteration 0 | discover misses remaining bugs at iteration ≥ 1 |
+> |---|---|---|---|
+> | **A (current, r4)** `AllFixed = len(AllFound) > 0 && Unfixed == 0`; `nothing_found`/`nothing_confirmed` clean exits | convergence runs → `overflow`/`stalled`, exit 1 (shipped loop cannot reach this: verify needs `confirmed_nonempty`) | `clean`, exit 0 (sdlc-loop) / `GATE_FAILED` if the workflow has no clean exit (Parse warns) | `findings_nonempty` fails → `GATE_FAILED`, exit 1 — visible, fork to retry |
+> | **B (design-literal)** `AllFixed = Unfixed == 0` | `fixed`, exit 0 with nothing found | same as A | same as A |
+>
+> **Decided 2026-08-27 by Dave: keep A.** (B remains documented here for the record.)
 >
 > **Scope rule:** what the deterministic core *decides*: how a YAML becomes a `Workflow`, what each gate and atom
 > returns, and exactly which `run` events `Init`/`Advance`/`Record` append. Kinds' prompts/executors are spec 4, forks/
@@ -53,7 +70,7 @@ type Options struct { Kinds map[string]KindInfo /* required (missing_kinds); the
 type Workflow struct {
     Name string; Version int; Vars map[string]VarSpec; States []run.State; Initial run.State
     Transitions []Transition; Nodes map[run.State]*Node; Cmds map[string]*CmdDecl
-    Convergence *yaml.Node; RepoMode string; OnOverflow string; Hash string
+    Convergence *yaml.Node; RepoMode string; OnOverflow string; Hash string /* hex sha256 of the raw bytes given to Parse */
     Refs map[run.State][]string; CmdRefs map[string][]string   // $VARs per node / per cmd, computed at Parse (pre-resolution)
     Warnings []string                                          // non-fatal Parse observations (§2.3 end)
 }
@@ -95,25 +112,28 @@ accepted and ignored. `nodes.<state>`: `kind` + optional `exec`, `model`, `effor
 `cmds.<name>`: `argv` (non-empty list of non-empty strings), `timeout` seconds (integer 1..3600, default 60), `env`
 (names). Node `cmd:`, `on_overflow:`, and `{cmd: <name>}` atoms reference `cmds` by name. Unknown top-level keys →
 `unknown_key`. `$NAME` grammar: `\$([A-Z_][A-Z0-9_]*)` (longest match; `$JUDGE_EFFORT` is one token); `$$` is a literal
-`$`; `${X}` is not supported. Shipped YAMLs: `sdlc-loop` gained `discover→done findings_empty (clean)` and
+`$`; any other `$` (`$1`, `${X}`, a trailing `$`) is left literal. Substitution covers `model`, `effort`, top-level string
+params and strings inside list params (nested maps are not walked), and every argv element. Shipped YAMLs: `sdlc-loop` gained `discover→done findings_empty (clean)` and
 `adjudicate→done confirmed_empty (clean)` rows and the r2-form escape-hatch comment (ledger); `review-loop` unchanged.
 
 ### 2.3 Static validation (Parse) — `ERR_WORKFLOW_INVALID{reason, at}`, first failure in this order
 | reason | rule |
 |---|---|
 | `missing_kinds` | `Options.Kinds` nil/empty |
-| `unknown_key` | unknown top-level or node-reserved key misuse |
+| `bad_yaml` | the document does not decode (malformed YAML, duplicate mapping keys, non-mapping root); `at = document` |
+| `unknown_key` | unknown key at the top level, inside a `cmds.<name>` or transition entry, or a reserved node key with a non-string value |
 | `missing_name`, `bad_version` | `workflow` non-empty; `version == 1` |
 | `no_initial`, `bad_state` | `states` non-empty; each `^[a-z][a-z0-9_-]{0,31}$`, unique, `judge` reserved (spec 5's `fsm judge` node) |
 | `bad_var` | var name not `^[A-Z_][A-Z0-9_]*$`; `required` with `default`; more than `MaxVars` |
 | `bad_cmd` | argv empty / non-string element / empty element; `timeout` non-integer or outside 1..3600; name not `^[a-z][a-z0-9_-]{0,31}$`; more than `MaxAllowedCmds`; argv longer than `MaxArgv` |
-| `bad_env` | name not `^[A-Z_][A-Z0-9_]*$`, duplicate, more than `MaxEnv`, or reserved: `PATH HOME LANG TMPDIR`, `MRV_*`, `LD_*`, `DYLD_*`, `BASH_ENV`, `ENV`, `PYTHONPATH`, `NODE_OPTIONS`, `PERL5OPT`, `GIT_*` |
+| `bad_env` | name not `^[A-Z_][A-Z0-9_]*$`, duplicate, more than `MaxEnv`, or reserved: `PATH HOME LANG TMPDIR`, `MRV_*`, `LD_*`, `DYLD_*`, `GIT_*`, `BASH_ENV ENV SHELLOPTS PS4 IFS CDPATH GLOBIGNORE PROMPT_COMMAND`, `PYTHONPATH PYTHONSTARTUP PYTHONHOME`, `NODE_OPTIONS NODE_PATH`, `PERL5OPT PERL5LIB`, `RUBYOPT RUBYLIB`, `JAVA_TOOL_OPTIONS` (best-effort denylist; the consent list shows env **names**) |
 | `duplicate_cmd` | two `cmds` keys with the same name (detected on the `yaml.Node`) |
 | `unknown_state` | transition `from`/`to` or node key not in `states` |
 | `failed_reserved` | `failed` must be declared, has no node, appears in no transition |
 | `node_without_kind`, `unknown_kind`, `unknown_exec`, `exec_kind_mismatch` | kind ∈ `Kinds`; exec ∈ {inline, subagent, fork} and ∈ `AllowedExec` (omitted → `DefaultExec`) |
 | `bad_params` | `KindInfo.ValidateParams(params)` failed (`Fields.detail`) |
 | `cmd_without_kind`, `unknown_cmd` | `cmd:` on a non-`cmd` kind / `cmd` kind without `cmd:`; any reference (node, `on_overflow`, atom) to an undeclared name |
+| `initial_terminal` | the initial state has no outgoing transition |
 | `terminal_with_node` | a state with no outgoing transition carries a node |
 | `unknown_gate` | gate ∉ `gate.Names()` |
 | `duplicate_transition` | same `(from, gate)` twice |
@@ -126,6 +146,7 @@ accepted and ignored. `nodes.<state>`: `kind` + optional `exec`, `model`, `effor
 | `cycle_without_loop` | a cycle among non-loop transitions |
 | `bad_convergence` | `converge.Validate(node, cmdNames)` failed (`Fields.detail`) |
 | `bad_repo_mode` | not `advisory`/`enforcing` |
+| `unknown_var` | a `$X` in model/effort/params/argv names an undeclared var (`at` = the node or cmd) |
 
 Parse never refuses on consent grounds. **Warnings** (`w.Warnings`): `loop_without_clean_exit` when a loop exists and no
 transition carries outcome `clean` (D1). `Init` appends `warn{WORKFLOW_WARNING, Detail}` per warning.
@@ -140,13 +161,16 @@ values for either (`ERR_CALIBRATION_PINNED{name}`). `Open` re-resolves from the
 `calibration=false`. The resolved copy carries `Refs`/`CmdRefs`/`Warnings` unchanged.
 
 ### 2.5 Commands and `cmds_sha256`
-`ResolveCmds(w /* resolved */, workDir, lookPath, hash) ([]run.AllowedCmd, string, error)`: for every `CmdDecl` in name
-order: `argv[0]` → if it contains `/`, `Abs(Join(workDir, argv[0]))`, else `lookPath` → **absolute** path
-(`ERR_CMD_NOT_FOUND{name}`; a non-absolute result is also `ERR_CMD_NOT_FOUND`), written into `AllowedCmd.Argv[0]`; every
+`ResolveCmds(w /* resolved */, workDir /* absolute */, lookPath, hash) ([]run.AllowedCmd, string, error)`: for every
+`CmdDecl` in name order: `argv[0]` → if it contains `/`, `Join(workDir, argv[0])` (or as-is when absolute) and then
+`lookPath` on that path (design §16.2: exists **and** executable, fail fast), else `lookPath(name)`; the result must be
+absolute (`ERR_CMD_NOT_FOUND{name}` otherwise), written into `AllowedCmd.Argv[0]`; every
 argv element (including the rewritten `argv[0]`) that names an existing regular file (absolute, or relative to
 `workDir`) is sha256-hashed into `FileHashes` (absolute path → hash; always a non-nil map). `AllowedCmd{Name, Argv,
 FileHashes, TimeoutMS, Env}` (`Env` nil when none — `omitempty` keeps the preimage stable). **Preimage:**
-`run.Canonical(json of []AllowedCmd sorted by name)` → sha256 hex; independent of declaration order (W4). The runner
+`run.MarshalCanonical([]AllowedCmd sorted by name)` (escape-off encoder, the one every fsm struct→JSON path uses) → sha256
+hex; independent of declaration order (W4). `hash(path)` returns an error for missing or non-regular files (that is how
+"names an existing regular file" is decided; `workflow.FileSHA256` is the real one). The runner
 executes `AllowedCmd.Argv` verbatim. `VerifyCmds(allowed /* = snap.AllowedCmds, never re-resolved */, workDir, hash)`:
 each pinned hash → `ERR_CMD_CHANGED{path, reason: mismatch|missing}`; an argv element that now resolves to a regular file
 without a `FileHashes` entry → `ERR_CMD_CHANGED{path, reason: appeared}`. Ledger: absolute paths make the sha per-machine
@@ -189,8 +213,11 @@ func Payload(s run.Snapshot) []byte      // canonical snapshot with Vars → "sh
 ```
 **Grammar:** an atom is either a bare scalar `all_fixed` | `no_fixation_progress`, or a one-key mapping: `{all_fixed:
 true}`, `{no_fixation_progress: true}`, `{max_iterations: N>0}`, `{budget: {tokens: N>0}}`, `{cmd: <name>}`; composites
-`{any: [..]}`, `{all: [..]}` (non-empty), `{not: <atom>}`. Anything else → `ERR_BAD_CONVERGENCE{detail}` (Parse
-reason `bad_convergence`). Atoms: `all_fixed` (class `fixed`), `no_fixation_progress` (class `stalled`: `PrevUnfixed != nil
+`{any: [..]}`, `{all: [..]}` (1..`MaxAtoms`=32 children), `{not: <predicate>}`; nesting depth ≤ `MaxDepth`=4. `all_fixed`
+is legal only at the root or as a direct child of a root-level `any` (a give-up can never carry class `fixed` through
+`all`/`not` — plan C3); `not` always classes `custom`. `Validate(node, cmdNames)`: `workflow.Parse` passes the declared
+names (an empty non-nil slice when none), so an unknown atom name is `bad_convergence`. Anything else →
+`ERR_BAD_CONVERGENCE{detail}` (Parse reason `bad_convergence`). The cmd atom's `reason` field is optional. Atoms: `all_fixed` (class `fixed`), `no_fixation_progress` (class `stalled`: `PrevUnfixed != nil
 && Unfixed >= *PrevUnfixed`; nil ⇒ false), `max_iterations: N` (class `overflow`: stop iff `Iteration+1 >= N`; `N: 5`
 stops at `Iteration == 4`), `budget` (class `overflow`: `Tokens.Total() >= N`), `cmd` (class `custom`; stdout must be
 exactly `{"stop": bool, "reason": string}` → `ERR_CMD_OUTPUT_INVALID`; non-zero exit → `ERR_CMD_FAILED{exit}`). `any`
@@ -213,13 +240,18 @@ type NodeKind interface {
 type Executor interface { Execute(ctx, ExecInput) (json.RawMessage, error) }     // returns output already accepted by the kind's Decode
 type Registry interface { Kind(name) (NodeKind, bool); Executor(name) (Executor, bool); Info() map[string]workflow.KindInfo; Mock() bool }
 type Clock func() run.Time
-type Sidecar interface { Write(runID, name string, b []byte) error /* O_CREAT|O_EXCL|O_NOFOLLOW 0600 in the run's 0700 dir; ERR_SIDECAR{reason: exists|path} */; Read(runID, name string) ([]byte, error) /* ERR_SIDECAR{reason: missing} */ }
-// Sidecar names: ^[a-z][a-z0-9._-]{0,63}$, never `audit.*` or `lock`. FS impl under <root>/.metareview/runs/<id>/; Mem for tests.
+type Sidecar interface {
+    Write(runID, name string, b []byte) error      // O_CREAT|O_EXCL|O_NOFOLLOW 0600 in the run's 0700 dir; ERR_SIDECAR{reason: exists|path|name}
+    Read(runID, name string) ([]byte, error)       // O_NOFOLLOW; ≤ MaxPayload else ERR_SIDECAR{reason: too_large}; ERR_SIDECAR{reason: missing}
+    List(runID string) ([]string, error)           // names (spec 3 Export/Fork)
+}
+// Sidecar names: ^[a-z][a-z0-9._-]{0,63}$, never `audit.*` or `lock`; runID must pass run.ValidateRunID. FS impl under <root>/.metareview/runs/<id>/; Mem for tests.
 ```
 `Audit` appends immediately (durable) and rebinds the machine's state; it returns store errors so the executor stops.
 Executors number `llm_call.Index` from `StartIndex` (the fold's next index for the key), so an interrupted execution
 resumes with a continuing index and its earlier spend stays audited. `Execute` is never retried by the machine inside
-one `Advance`; a failure → `executor` pseudo-gate (§5.4 step 5).
+one `Advance`; a failure → `executor` pseudo-gate (§5.4 step 5), except `ctx.Err() != nil` (interrupt), which `Advance`
+returns unchanged so the next `Advance` resumes from `StartIndex`.
 
 ### 5.2 Deps and API
 ```go
@@ -232,9 +264,10 @@ type Deps struct {
 }
 type InitOptions struct { Workflow string; RunID string; Vars map[string]string; Base string; RepoMode string; AllowCustomCmds string; Calibration bool; MockDir string; GoldensPath string; WorkDir, RepoRoot string }
 type OpenOptions struct { Repair bool }
+// Typed string sets: Status ∈ {ADVANCED, NEEDS_INPUT, DONE, STOPPED, GATE_FAILED}; NextAction ∈ {advance, record, none}; RecordOptions.Kind ∈ {node-output, tokens, event} — exported constants.
 func Init(ctx, Deps, InitOptions) (*Machine, error); func Open(ctx, Deps, runID string, OpenOptions) (*Machine, error)
 func (m *Machine) Advance(ctx) (AdvanceResult, error); func (m *Machine) Record(ctx, RecordOptions) (RecordResult, error); func (m *Machine) View() View
-type AdvanceResult struct { Status string; From, To run.State; Gate *run.GateData /* first failing */; Outcome run.Outcome; StopReason string; NeedsInput *NeedsInput; Warnings []string /* warn events appended by this call */; Untrusted []string /* "gate.detail","warnings","stop_reason" when non-empty */; ExitCode int; RunID string }
+type AdvanceResult struct { Status string; From, To run.State; Gate *run.GateData /* first failing */; Outcome run.Outcome; StopReason string; NeedsInput *NeedsInput; Warnings []string /* warn events appended by this call */; Untrusted []string /* "gate.detail","warnings","stop_reason","error.detail" when non-empty — every Detail may carry repo/third-party bytes */; ExitCode int; RunID string }
 type NeedsInput struct { Node string; Kind, Exec, Model, Effort string; Instructions Instructions; Record string }
 type RecordOptions struct { Kind string /* node-output|tokens|event */; Node string; Data json.RawMessage; Replace bool; Name string }
 type RecordResult struct { Seq int64; Type run.EventType; Key string }
@@ -245,7 +278,9 @@ type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
 ### 5.3 `Init`
 1. Load YAML: name → `Deps.Workflows` (`ERR_WORKFLOW_NOT_FOUND`); path (`/` or `.yaml`) → `ReadFile`, ≤ 256 KB
    (`ERR_WORKFLOW_TOO_LARGE`). `Parse(raw, Options{Kinds: Kinds.Info()})`; `Resolve(vars, calibration)`.
-   `RepoMode` override must be `advisory|enforcing|""` (`ERR_BAD_REPO_MODE`).
+   `RepoMode` override must be `""` or `enforcing` (tighten-only; `advisory` over a workflow's `enforcing` → `ERR_BAD_REPO_MODE`).
+   Git failures during `Init`/`Advance` outside gates are returned as the `ERR_GIT`/`ERR_GIT_REF` error unchanged (retryable);
+   only gates convert Git failures into gate errors.
 2. `ResolveCmds`; `len(Cmds) > 0 && AllowCustomCmds != sha` → `ERR_CMDS_NOT_ALLOWED{sha}`, Detail = the printed list
    (name, argv with pinned/unpinned elements marked, file hashes, timeout, env **names**).
 3. Git in `WorkDir`: `CommonDir` must equal `RepoRoot`'s (`ERR_WORKDIR_FOREIGN`); `Head`; `RevParse(Base||"HEAD")` →
@@ -256,63 +291,75 @@ type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
    `Kinds.Mock()` must equal `MockDir != ""` (`ERR_MOCK_MISMATCH`).
 6. `runID` ← `RunID` or `run.RunID(w.Name, Clock().Time)`; **`run.Create` first**, then `Sidecar.Write(runID,
    "workflow.yaml", raw)` (failure → the error; the run exists without a sidecar and `Open` reports `ERR_SIDECAR`); then
-   under the lock append `tree{Head, TreeHash, Status}` and one `warn{WORKFLOW_WARNING}` per `w.Warnings`.
+   under the lock append `tree{Head, TreeHash, Status: CapDetail(porcelain)}` and one `warn{WORKFLOW_WARNING}` per
+   `w.Warnings`. A crash between `Create` and the sidecar write leaves a run that `Open` reports as `ERR_SIDECAR{missing}`;
+   spec 5 documents manual deletion of `.metareview/runs/<id>/` (no automatic repair).
    `init` carries no `State`/`Iter`/`Node` stamps (run's `init_stamp` rule); `Mock` stamp on every later event.
 7. Return; `View().NextAction == "advance"`.
 
+### 5.3b `Open(ctx, deps, runID, opts)`
+1. `Store.Lock(runID)` for the duration. `log ← EventsWithLines` (`ERR_RUN_NOT_FOUND` passes through). If `log.Torn`: with
+   `opts.Repair` → `RepairTail` (offset 0 → run removed → `ERR_RUN_NOT_FOUND{detail: "run removed; torn bytes in
+   runs/.torn/"}`), reload, then append `warn{AUDIT_TORN_LINE_DROPPED, Detail: "<n> bytes dropped after seq <s> from
+   audit.jsonl"}`; without `Repair` → the machine opens with `View.Torn = true` and every `Advance`/`Record` returns
+   `ERR_AUDIT_TORN`. `opts.Repair` on a clean log → `ERR_AUDIT_NOT_TORN` passes through.
+2. `st ← FoldFull`, `st.ChainHead ← log.Head`; `raw ← Sidecar.Read("workflow.yaml")`; `sha256(raw) == snap.WorkflowHash`
+   else `ERR_WORKFLOW_CHANGED{expected, got}`; `Parse(raw, Options{Kinds: Kinds.Info()})` (a vocabulary change in a newer
+   binary surfaces here as `ERR_WORKFLOW_INVALID` — the "older-binary run" signal; `list`/`export` still work);
+   `Resolve(snap.Vars, false)`; `VerifyCmds(snap.AllowedCmds, WorkDir, FileHash)`; `Mock`: split on the **last** `#`, resolve
+   `rel` against `snap.RepoRoot`, `MockLoad` hash vs stored, `Kinds.Mock() == (snap.Mock != "")`, else `ERR_MOCK_MISMATCH`;
+   build `runner ← Deps.Runner(snap.AllowedCmds, WorkDir, runID, audit)` and, when `w.Convergence != nil`, `pred ←
+   converge.Parse(w.Convergence, runner)`.
+`Advance` and `Record` re-run steps 1–2 under their own lock (no cached state is trusted across calls).
+
 ### 5.4 `Advance`
 ```
-1  Lock; log ← EventsWithLines; Torn → ERR_AUDIT_TORN{seq, bytes}. st ← FoldFull(log.Events); st.ChainHead ← log.Head;
-   snap ← st.Snapshot. "append X" ≡ st, snap ← Store.Append(runID, st, X) (rebind; the Audit closure is the same path).
-   Any store error aborts with that error; nothing is rolled back. Emitter caps: ConvergeData.Reason/warn.Detail
-   CapText(MaxText); GateError.Detail CapDetail.
-2  snap.Outcome != "": if Outcome == overflow && OnOverflow != "" && !OverflowHandled → step 9b (resume the handler);
-   else ERR_RUN_TERMINAL.
-3  Integrity: sha256(Sidecar.Read("workflow.yaml")) == snap.WorkflowHash else ERR_WORKFLOW_CHANGED; reparse the SIDECAR
-   bytes + re-resolve from snap.Vars; VerifyCmds(snap.AllowedCmds) → ERR_CMD_CHANGED; MockLoad(dir) hash vs snap.Mock
-   else ERR_MOCK_MISMATCH; Kinds.Mock() == (snap.Mock != "") else ERR_MOCK_MISMATCH. Build the predicate with
-   converge.Parse(w.Convergence, Deps.Runner(snap.AllowedCmds, WorkDir, runID, audit)).
-4  head ← Git.Head; porcelain ← Status; wt ← WorkTree; h ← TreeHash(head, wt); node ← w.NodeFor(state)
-   changed ← snap.TreeHash != "" && h != snap.TreeHash
-   if changed && (node == nil || node.Kind != agent-edit):
-       advisory  → append warn{UNSANCTIONED_EDIT, Detail: porcelain}
-       enforcing → append gate{Name:"repo_mode", Passed:false, Error:{ERR_UNSANCTIONED_EDIT, Detail: porcelain}}   (BEFORE the tree, so a crash re-detects)
-                   append tree{head, h, porcelain}; → step 8
-   if changed: append tree{head, h, porcelain}
+1  §5.3b steps 1–2 (lock, fold, ChainHead, sidecar/cmds/mock integrity, runner, predicate). "append X" ≡ st, snap ←
+   Store.Append(runID, st, X) (rebind; the Audit closure is the same path). Any store error aborts with that error;
+   nothing is rolled back. Emitter caps: ConvergeData.Reason / warn.Detail CapText(MaxText); ConvergeData.Atom ≤ MaxShort
+   (guaranteed by MaxDepth/MaxAtoms); GateError.Detail and tree.Status CapDetail (+ flags).
+2  snap.Outcome != "": if Outcome == overflow && w.OnOverflow != "" && !OverflowHandled → step 9b; else Deps.Terminal(ctx,
+   View()) (idempotent — spec 3 dedups by run id) then ERR_RUN_TERMINAL.
+3  (folded into step 1)
+4  head ← Git.Head; porcelain ← Status; wt ← WorkTree; h ← TreeHash(head, wt); node ← w.NodeFor(state); mode ← snap.RepoMode
+   if snap.TreeHash == "": append tree{head, h, porcelain}                         (baseline: forks from the initial state, Init crash)
+   else if h != snap.TreeHash && (node == nil || node.Kind != agent-edit):
+       advisory  → append warn{UNSANCTIONED_EDIT, Detail: porcelain}; append tree{head, h, porcelain}
+       enforcing → append gate{Name:"repo_mode", Passed:false, Error:{ERR_UNSANCTIONED_EDIT, Detail: porcelain}} → step 8   (NO tree: a crash re-detects)
+   else if h != snap.TreeHash: append tree{head, h, porcelain}
 5  if node != nil:
-       k ← Key(node, iter); diff ← Git.Diff(BaseSHA, head, MaxDiffBytes)
+       k ← Key(node, iter); diff ← Git.Diff(BaseSHA, head, MaxDiffBytes)   (only when needed: output absent, or fork)
        if NodeOutputs[k] absent:
            fork: out, err ← Executor.Execute(ExecInput{snap, node, diff, st.NextIndex(k), audit})
-                 err → append gate{Name:"executor", Passed:false, Error:{ERR_EXECUTOR_FAILED, Detail}} → step 8
-                 append node_output{out}     (out is already Decode-valid; an append rejection is still a store error → abort)
-           host: if the last node-scoped event for k is not needs_input: append needs_input
-                 ins, err ← kind.Instructions(snap, node, diff, Nonce()); err → ERR_INSTRUCTIONS_FAILED (returned, nothing appended)
+                 ctx.Err() != nil → return err (resumable); other err → append gate{Name:"executor", Passed:false, Error:{ERR_EXECUTOR_FAILED, Detail: CapDetail(err)}} → step 8
+                 append node_output{out}
+           host: ins, err ← kind.Instructions(snap, node, diff, Nonce()); err → ERR_INSTRUCTIONS_FAILED (nothing further appended)
+                 if no needs_input event exists for k: append needs_input
                  return NEEDS_INPUT (exit 3)
        if !Applied[k]: out ← Decode(output); delta ← Reduce(snap, out); error → append gate{Name:"node_output", Passed:false,
                        Error:{ERR_NODE_OUTPUT_INVALID, Detail}} → step 8; else append delta_applied{delta, OutputHash(output)}
                        (a store rejection of delta_applied is treated the same way: node_output pseudo-gate → step 8)
-6  chosen, first ← nil, nil
-   if w.LoopTransition() != nil && LoopTransition().From == state:            (loop boundary — order-independent)
-       tt ← w.TerminalFor(state)
-       err ← gate(tt.Gate)(snap); append gate; pass → chosen ← tt                    (gate-first: fixed/clean wins before any atom)
+6  chosen ← nil; failures ← [] (GateData in evaluation order)
+   if w.TerminalFor(state) != nil:                                            (loop boundary — order-independent)
+       tt ← w.TerminalFor(state); err ← gate(tt.Gate)(snap); append gate; pass → chosen ← tt else failures += err
        if chosen == nil:
-           if !AllFixed(snap):
-               r, err ← Convergence.Evaluate(snap)
-               err → append gate{Name:"converge", Passed:false, Error:{ERR_CONVERGE_FAILED, Detail: err}} → step 8
-               append converge{Atom: r.Atom, Class: r.Class, Stop: r.Stop, Reason}
-               if r.Stop: chosen ← synthetic {From: state, To: tt.To, Gate: r.Atom, Outcome: r.Class}
-           if chosen == nil: for t in Outgoing(state) except tt (declaration order): err ← gate(t.Gate)(snap); append gate; pass → chosen ← t; break; else first ??= err
-           first ??= the tt gate's error
-   else: for t in Outgoing(state): err ← gate(t.Gate)(snap); append gate{t.Gate, passed, err}; pass → chosen ← t; break; else first ??= err
+           r, err ← pred.Evaluate(snap)                                        (always when tt failed — bounded loops)
+           err → append gate{Name:"converge", Passed:false, Error:{ERR_CONVERGE_FAILED, Detail}} → step 8
+           r.Class == fixed → same, ERR_CONVERGE_FAILED{reason: fixed_class}   (defense in depth over converge's placement rule)
+           append converge{Atom: r.Atom, Class: r.Class, Stop: r.Stop, Reason}
+           if r.Stop: chosen ← synthetic {From: state, To: tt.To, Gate: r.Atom, Outcome: r.Class}
+       if chosen == nil: for t in Outgoing(state) except tt: err ← gate(t.Gate)(snap); append gate; pass → chosen ← t; break; else failures += err
+   else: for t in Outgoing(state): err ← gate(t.Gate)(snap); append gate; pass → chosen ← t; break; else failures += err
 7  (every gate evaluation is appended when evaluated, in order)
-8  chosen == nil: append transition{From: state, To: failed, Gate: first.Gate, Outcome: failed, Head: head}; return GATE_FAILED with
-   Gate = the FIRST failing GateData (pseudo-gates repo_mode/executor/node_output/converge included), exit 1
+8  chosen == nil: first ← the pseudo-gate that sent us here, else failures[0]; append transition{From: state, To: failed, Gate:
+   first.Gate, Outcome: failed, Head: head}; Deps.Terminal(ctx, View()); return GATE_FAILED with Gate = first, exit 1
 9  append transition{From, To, Gate, Outcome, Loop, ToKind, Head}
-9b if Outcome == overflow && OnOverflow != "" && !OverflowHandled: res, err ← Runner.Run(OnOverflow, converge.Payload(snap));
+9b if Outcome == overflow && OnOverflow != "" && !OverflowHandled: res, err ← runner.Run(OnOverflow, converge.Payload(snap));
    append overflow_handler{Name, Argv: snap.AllowedCmds[name].Argv, InputHash: sha256(payload), Stdout/Stderr: CapText(MaxDetail/MaxStderr)+flags,
-   ExitCode (−1 when err), DurationMS, Error: code}; err or exit≠0 → also warn{OVERFLOW_HANDLER_FAILED}
-   if Outcome != "": Deps.Terminal(ctx, View())    (spec 3 writes the runs.jsonl row; its error is returned)
-   return per §5.7
+   ExitCode (−1 when err), DurationMS, Error: code}; err or exit≠0 → also warn{OVERFLOW_HANDLER_FAILED}   (at-least-once: a crash between the
+   runner's cmd_call and this append re-runs the command)
+   if Outcome != "": Deps.Terminal(ctx, View()) (its error is returned; the transition is already durable)
+   return per §5.7 — StopReason = "<Atom>: <Reason>" (the fold keeps only Atom in Snapshot.StopReason)
 ```
 **Stamps** on every event appended by `Init`/`Advance`/`Record`/`Audit`: `At ← Clock()`, `State ← current` (`From` for
 transitions), `Iter ← current` (`N+1` on the loop transition), `Mock ← snap.Mock != ""`, `Node ← node.Name` for
@@ -358,8 +405,8 @@ behind `FSM_MACHINE_UPDATE_GOLDEN=1` with the run package's "drift ≠ regenerat
 
 | pkg | rows |
 |---|---|
-| workflow | W1 both shipped YAMLs + the §2.2 example + a mapping-form twin of review-loop (order preserved, `*→failed` ignored, `->` and `→`): assert `Transitions`, `Nodes` (exec defaulted for `fix`/`verify`), `Cmds` (`Timeout 30s`, default `60s`), `Refs`/`CmdRefs` literals, `Hash` literal, `Warnings` empty; W2 one fixture per reason **and per sub-rule** (each one edit from a valid base): `bad_cmd` ×6, `bad_env` ×5 (incl. `MRV_X`, `LD_PRELOAD`, duplicate), `bad_var` ×3, `bad_state` ×3 (incl. `judge`), `duplicate_transition` (same `(from, gate)`, different `to`), `loop_terminal` ×2 (zero and two), `unknown_cmd` ×3 (node, on_overflow, atom), `cmd_without_kind` ×2, `unknown_var`, `bad_params` via a fake `ValidateParams`, `missing_kinds`; assert `reason` + `at`; W3 `Resolve`: `$JUDGE`/`$JUDGE_EFFORT` prefix pair → `Model=="a"`, `Effort=="b"`; `$$` literal; `$JUDGEX` → `ERR_VAR_UNKNOWN`; caller `FOO` → `ERR_VAR_UNKNOWN`; required unset; calibration refuses caller `JUDGE` **and** `JUDGE_EFFORT`; calibration on a workflow without `JUDGE` var is a no-op; re-resolve of stored pinned vars succeeds; argv substitution; W4 `ResolveCmds`: fake lookPath/hash; `argv[0]` rewritten absolute (`bash` → `/bin/bash`, `./s.sh` → `<workDir>/s.sh`), relative lookPath result → `ERR_CMD_NOT_FOUND`; closure over `["bash","./s.sh"]` + absolute path; `/bin/bash` itself appears in `FileHashes`; non-nil empty map; **hand-authored preimage** (`testdata/cmds-preimage.json` + `.sha256` from `shasum`) with two cmds declared out of order, `TimeoutMS 1500`, `Env` set; one-byte edit → different sha; `VerifyCmds` mismatch/missing/appeared; W5 `VarsReferencedBy` (node ∪ cmd, sorted, resolved copy), `Outgoing`, `IsTerminal` incl. `failed`, `LoopTransition`, `TerminalFor`, `loop_without_clean_exit` warning |
-| machine | M1 `Init`: hand-written expected sequence `[init(no stamps), tree(State=initial, Iter 0), warn?]` with every `InitData` field asserted literally (embedded + path workflows; `workflow.yaml` sidecar bytes == raw; `Create` before sidecar observed via a fake store/sidecar call log; `ERR_RUN_EXISTS` leaves the victim's sidecar intact); `ERR_WORKFLOW_NOT_FOUND`, `ERR_WORKFLOW_TOO_LARGE`; goldens ok/unknown field/over cap/over bytes; consent list + sha in Detail, wrong sha, no cmds; `RevParse` base (`main` → sha); `ERR_WORKDIR_FOREIGN`; `ERR_BAD_REPO_MODE`; mock hash pinned + `Kinds.Mock()` mismatch; unknown `--var`; M2 `Advance` on both shipped workflows with a fake Registry: hand-written expected event-type sequences per path (`review-loop` clean/reviewed; `sdlc-loop` clean at discover, clean at adjudicate, fixed after 1 iteration, loop once then fixed), literal asserts on transition fields, `needs_input` once across `advance, record tokens, advance` and again at `discover@1`, `View.NextAction` per step; goldens regression-only; M3 gate failure: two failing gates → `Gate` is the first and `transition.Gate` names it; `ERR_GATE_INAPPLICABLE`; executor error → `executor` pseudo-gate with earlier `llm_call`s kept and `StartIndex` honoured on the next fork (interrupted-execution fixture: pre-seeded `llm_call` index 0, executor asserts `StartIndex == 1`); decode error / Reduce error / rejected `delta_applied` (status subset from a fake cmd kind) → `node_output` pseudo-gate; M4 loop: cumulative regression (iter 3 fixes its own bug, 7 remain: loop taken, `AllFound == 8`, `Unfixed == 7`, all 8 statuses, not `fixed`); **gate-first**: `max_iterations: 1` with all bugs fixed at verify → `fixed`, zero `converge` events; negative control one bug left → `converge{max_iterations}` → `overflow`; `stalled` via nil-then-plateau and via regression (`Prev 3 → Unfixed 5`); `budget` via `llm_call` tokens and via `record tokens`; `custom` via cmd atom; converge error → `converge` pseudo-gate and no loop taken; overflow handler once, `overflow_handler` fields literal, failure warn, **not** run for `stalled`/`custom`, resumed after a crash (fixture: terminal overflow run without handler → `Advance` runs it, then `ERR_RUN_TERMINAL`); M5 tree: identical porcelain + different `WorkTree` → advisory warn vs enforcing `repo_mode` gate **appended before** the `tree`; agent-edit exempt; `tree` only on change (count); M6 `Record` refusals per code and sub-reason (`syntax`, `event_type` (`transition`), `reserved`), `ERR_RECORD_TOKENS` on unknown field and on `-1`, `Replace`, terminal `tokens` allowed, `ERR_NODE_OUTPUT_INVALID` leaves `Events` byte-identical; M7 `Open`: `ERR_WORKFLOW_CHANGED` via sidecar edit; embedded bytes replaced by a workflow with different transitions while the sidecar is intact → `Advance` follows the **sidecar's** transitions; `ERR_CMD_CHANGED`; `ERR_MOCK_MISMATCH` via scenario edit and via registry mismatch; torn → `ERR_AUDIT_TORN`; `Repair` → warn Detail literal + fold ok; `Repair` at offset 0 → `ERR_RUN_NOT_FOUND`; `ERR_SIDECAR{missing}`; M8 stamps: every event's `At` equals the injected clock sequence, `State/Iter/Mock/Node` per §5.4 tail (`cmd_call` has no Node), non-mock runs never carry `Mock: true` (`MockTainted == false`), mock runs carry it on every non-init event; M9 §5.7 table incl. `StopReason`, `Untrusted` list, `Deps.Terminal` called once with the terminal View; `ERR_AUDIT_FULL` surfaced; a counting store that fails append #N for every N of the happy sequence returns the error unchanged; FS `Sidecar`: symlink refused, exists refused, mode 0600, missing run → `ERR_SIDECAR` |
+| workflow | W0 order row: a document with an unknown top-level key **and** `version: 2` → `unknown_key` (first failure in table order). W1 both shipped YAMLs + the §2.2 example + a mapping-form twin of review-loop (order preserved, `*→failed` ignored, `->` and `→`): assert `Transitions`, `Nodes` (exec defaulted for `fix`/`verify`), `Cmds` (`Timeout 30s`, default `60s`), `Refs`/`CmdRefs` literals, `Hash` literal, `Warnings` empty; W2 one fixture per reason **and per sub-rule** (each one edit from a valid base): `bad_cmd` ×8 (argv empty / non-string / empty element / timeout non-integer / 0 / 3601 / name regex / > MaxAllowedCmds / > MaxArgv), `bad_env` one per reserved literal and prefix + regex + duplicate + count, `bad_var` ×3, `bad_state` ×3 (incl. `judge`), `failed_reserved` ×3 (undeclared / has node / in a transition), `duplicate_transition` (same `(from, gate)`, different `to`), `loop_terminal` ×2, `unknown_cmd` ×3, `cmd_without_kind` ×2, `unknown_var` ×3 (node, cmd, list param), `bad_outcome` ×2 (`great`, `failed`), `bad_yaml` ×2 (malformed, duplicate key), `bad_version` (`one`), `initial_terminal`, `bad_params`, `missing_kinds`; acceptance boundaries: timeout 1 and 3600, 32-char state, `MaxVars`/`MaxEnv`/`MaxArgv`/`MaxAllowedCmds` at cap; assert `reason` + `at`; W3 `Resolve`: `$JUDGE`/`$JUDGE_EFFORT` prefix pair → `Model=="a"`, `Effort=="b"`; caller value beats `Default`; list params substituted, non-string list elements and nested maps untouched (literal asserts), `$1`/`${X}` left literal; `Refs`/`CmdRefs` from a list param; `$$` literal; `$JUDGEX` → `ERR_VAR_UNKNOWN`; calibration pins asserted as literals; caller `FOO` → `ERR_VAR_UNKNOWN`; required unset; calibration refuses caller `JUDGE` **and** `JUDGE_EFFORT`; calibration on a workflow without `JUDGE` var is a no-op; re-resolve of stored pinned vars succeeds; argv substitution; W4 `ResolveCmds`: fake lookPath/hash; `argv[0]` rewritten absolute (`bash` → `/bin/bash`, `./s.sh` → `<workDir>/s.sh`), relative lookPath result → `ERR_CMD_NOT_FOUND`; closure over `["bash","./s.sh"]` + absolute path; `/bin/bash` itself appears in `FileHashes`; non-nil empty map; **hand-authored preimage** (`testdata/cmds-preimage.json` + `.sha256` from `shasum`) with two cmds declared out of order, `TimeoutMS 1500`, `Env` set; one-byte edit → different sha; `VerifyCmds` mismatch/missing/appeared; a directory argv element is not hashed; **no re-resolution**: after pinning `/bin/bash`, point `lookPath` elsewhere and edit the pinned file → `ERR_CMD_CHANGED{/bin/bash, mismatch}`, and the inverse (pinned intact, lookPath moved) → no error; non-absolute `workDir` refused; W5 `VarsReferencedBy` (node ∪ cmd, sorted, resolved copy), `Outgoing`, `IsTerminal` incl. `failed`, `LoopTransition`, `TerminalFor`, `loop_without_clean_exit` warning |
+| machine | M0 fakes: `gate.FailingFake{Fake; FailAt string}` (exported from `gate`, fails exactly one method) drives per-call-site `ERR_GIT{op}` rows for every Git call in `Init`/`Advance`; a counting store fails `Lock`, `EventsWithLines`, or append #N; seam row: `Fake.Diffs["<base>..<head>"] = "D"` with a small `MaxDiffBytes` → the fake kind/executor observe `diff.Text == "D"`, `Truncated == true`, `nonce == "n1"`. M1 `Init`: hand-written expected sequence `[init(no stamps), tree(State=initial, Iter 0), warn?]` with every `InitData` field asserted literally (embedded + path workflows; `workflow.yaml` sidecar bytes == raw; `Create` before sidecar observed via a fake store/sidecar call log; `ERR_RUN_EXISTS` leaves the victim's sidecar intact); `ERR_WORKFLOW_NOT_FOUND`, `ERR_WORKFLOW_TOO_LARGE`; goldens ok/unknown field/over cap/over bytes; consent list as a hand-written literal (pinned/unpinned marks, env **names** only, no process env values) + sha in Detail, wrong sha, no cmds; `ERR_MOCK_INVALID`; `RepoMode` override `enforcing` accepted / `advisory` over `enforcing` refused; `RevParse` base (`main` → sha); `ERR_WORKDIR_FOREIGN`; `ERR_BAD_REPO_MODE`; mock hash pinned + `Kinds.Mock()` mismatch; unknown `--var`; M2 `Advance` on both shipped workflows with a fake Registry: hand-written expected event-type sequences per path (`review-loop` clean/reviewed; `sdlc-loop` clean at discover, clean at adjudicate, fixed after 1 iteration, loop once then fixed), literal asserts on transition fields, `needs_input` once across `advance, record tokens, advance` and again at `discover@1`, `View.NextAction` per step; goldens regression-only; M3 gate failure: two failing gates → `Gate` is the first in evaluation order and `transition.Gate` names it; loop-boundary variant (tt and the loop gate both fail → tt named); two passing gates → the first taken; `ERR_INSTRUCTIONS_FAILED` (needs_input already present stays, nothing further); ctx cancelled during `Execute` → error returned, no pseudo-gate, next `Advance` resumes with `StartIndex`; `ERR_GATE_INAPPLICABLE`; executor error → `executor` pseudo-gate with earlier `llm_call`s kept and `StartIndex` honoured on the next fork (interrupted-execution fixture: pre-seeded `llm_call` index 0, executor asserts `StartIndex == 1`); decode error / Reduce error / rejected `delta_applied` (status subset from a fake cmd kind) → `node_output` pseudo-gate; M4 loop: cumulative regression (iter 3 fixes its own bug, 7 remain: loop taken, `AllFound == 8`, `Unfixed == 7`, all 8 statuses, not `fixed`); **gate-first**: `max_iterations: 1` with all bugs fixed at verify → `fixed`, zero `converge` events; negative control one bug left → `converge{max_iterations}` → `overflow`; `stalled` via nil-then-plateau and via regression (`Prev 3 → Unfixed 5`); `budget` via `llm_call` tokens and via `record tokens`; `custom` via cmd atom; converge error → `converge` pseudo-gate and no loop taken; `fixed_class` guard via a fake predicate; user workflow whose terminal gate is `confirmed_empty` (not `all_fixed`) with all bugs fixed and findings present → convergence evaluated (bounded) — the `max_iterations` stop fires; emitter caps: a 5 KB cmd-atom reason → `converge.Reason` and `StopReason` capped; overflow handler once, `overflow_handler` fields literal, failure warn, **not** run for `stalled`/`custom`, resumed after a crash (fixture: terminal overflow run without handler → `Advance` runs it, then `ERR_RUN_TERMINAL`); M5 tree: identical porcelain + different `WorkTree` → advisory warn (+ tree) vs enforcing `repo_mode` gate with **no** tree (a second `Advance` re-detects); porcelain ≈ 5 KB → warn Detail capped at `MaxText`, tree Status intact; 70 KB porcelain → `tree.Status` capped with flag; agent-edit exempt; baseline `tree` appended when `TreeHash == ""` (fork-from-initial fixture); `tree` only on change (count); M6 `Record` refusals per code and sub-reason (`syntax`, `event_type` (`transition`), `reserved`), `ERR_RECORD_TOKENS` on unknown field and on `-1`, `Replace`, terminal `tokens` allowed, `ERR_NODE_OUTPUT_INVALID` leaves `Events` byte-identical; M7 `Open`: `ERR_WORKFLOW_CHANGED` via sidecar edit; embedded bytes replaced by a workflow with different transitions while the sidecar is intact → `Advance` follows the **sidecar's** transitions; `ERR_CMD_CHANGED`; `ERR_MOCK_MISMATCH` via scenario edit and via registry mismatch; torn → `ERR_AUDIT_TORN`; `Repair` → warn Detail literal + fold ok; `Repair` at offset 0 → `ERR_RUN_NOT_FOUND`; `ERR_SIDECAR{missing}`; M8 stamps: every event's `At` equals the injected clock sequence, `State/Iter/Mock/Node` per §5.4 tail (`cmd_call` has no Node), non-mock runs never carry `Mock: true` (`MockTainted == false`), mock runs carry it on every non-init event; M9 §5.7 table incl. `StopReason` ("atom: reason"), `Untrusted` list, `Deps.Terminal` called for every terminal outcome incl. `failed`, again on a later `Advance` of a terminal run (idempotency is spec 3's), and on the 9b resume path; `Terminal` error returned with the transition durable; `ERR_AUDIT_FULL` surfaced; a counting store that fails append #N for every N of the happy sequence returns the error unchanged; FS `Sidecar`: symlink refused, exists refused, mode 0600, missing run → `ERR_SIDECAR` |
 
 ## 8. Ledger
 - `cmds:` single top-level declaration referenced by name; per-cmd `timeout`/`env` are consent-covered (design §16 inline argv retired).
@@ -376,7 +423,15 @@ behind `FSM_MACHINE_UPDATE_GOLDEN=1` with the run package's "drift ≠ regenerat
 - Design §9's atom params (`window: 2`) dropped: cmd atoms take no params (the command reads the payload).
 - Vars are configuration, not secrets (argv/consent lists show them); secrets use `env` pass-through.
 - `sdlc-loop` gained `clean` exits at discover and adjudicate (D1 mitigation); comment shows the r2 escape-hatch form.
-- Reassigned: CMP-15/ARC-21 → spec 3 (run spec §12 updated); `ERR_JUDGE_UNSET` mapping, env/consent docs, `Untrusted` marking → spec 5.
+- Reassigned: CMP-15/ARC-21 → spec 3 (run spec §12 updated); `ERR_JUDGE_UNSET` mapping, env/consent docs, `Untrusted` marking, manual deletion of a sidecar-less run, `WorkTree` object writes (scratch index writes loose blobs into `.git/objects`; gc reclaims) → spec 5 docs.
+- `JUDGE_EFFORT: {required: true}` in both shipped workflows (plan §6 shipped `{default: medium}`; design §17 wants no hardcoded effort).
+- C20's `ERR_EXEC_UNSUPPORTED` at Parse time is `ERR_WORKFLOW_INVALID{exec_kind_mismatch}`; the runtime code keeps C20's name (spec 4).
+- Fork obligation restated: the child's sidecar is the **child's** resolved workflow bytes (whose sha equals the child's `WorkflowHash`), copied from the parent only when unchanged; forks rebaseline `tree` on the child (spec 3 r2).
+- `nothing_found`/`nothing_confirmed` gates added (9 built-ins; C23 said 7): iteration-0 clean exits that refuse once any bug is known.
+- Convergence evaluated whenever the terminal gate fails (loops are always bounded by their predicate); `not` classes `custom`; `all_fixed` placement rule.
+- `WorkTree` disables `core.excludesFile`/`core.attributesFile`; residual: `.git/info/exclude` and clean filters remain agent-writable (spec 5 enforcing caveat).
+- Consent, `--accept-workflow-change`, `--mock-ai`, `--calibration`, and the `RepoMode` override are agent-satisfiable (run spec §1.5 "process guarantees for a cooperating agent"); `RepoMode` override is tighten-only.
+- `workflows/` embed package (`Names()`, `Read(name)`) is owned here; implemented in M0.
 
 ## 9. `run` amendments owned here (implemented; run spec §11 lists them)
 - `AllowedCmd{Name, Argv, FileHashes, TimeoutMS int64 (omitempty), Env []string (omitempty)}`; `MaxEnv = 16`; `withinCaps` checks `Env` count and names (`MaxShort`).
diff --git a/docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md b/docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
index 9c86fe0..175bb76 100644
--- a/docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
+++ b/docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md
@@ -1,6 +1,6 @@
 # metareview 0.9.0 — spec 4: guardrails, judge, kinds, and mock AI
 
-> **Status:** DRAFT r3 (2026-08-27). Fourth of the five split artifacts (ownership ledger: run spec §12). Owns plan
+> **Status:** r4 — BUILD BASELINE after ESCALATION (2026-08-27). Attempt 3 (`mrv-20260827-070456908813000-…`) ended NEEDS_REVISION on 4/8 lenses with every blocker mechanical; the chain is ESCALATED and a human must accept this r4 (applied provisionally per the run-spec precedent). r3 note: Fourth of the five split artifacts (ownership ledger: run spec §12). Owns plan
 > r3 §1.8 items 2–5, §2 (judge port), kinds/`Executor`/`Delta` producers, match-then-adjudicate composition +
 > `Bug.Verdict` vocabulary, `index` assignment, the `llm_call`/`cmd_call` producer contract, mock scenarios, and the
 > pinned harnesseval provenance of the prompts. Implements spec 2 r3's `machine.NodeKind`/`Executor`/`Registry`,
@@ -16,6 +16,16 @@
 > provenance test; token clamps; bounded reads; scenario strict decode + file-bytes hash + typed `parsed`; every
 > test row names its discriminating fixture.
 >
+> **r4 changes (attempt 3):** explicit Anthropic effort-capable model list (no globs) and a Go-owned product thinking
+> table (the reference has no `high`); default base URLs; `Bug` field population (`Confidence`, `File`, `Line`); goldens
+> capped at `MaxDesc` at init (run cap) so matched `Desc` always fits; `Rejected` shape; still-present `*bool`; `Input =
+> prompt_tokens − cached_tokens`; every counter clamped ≥ 0 at the provider boundary; 529/`overloaded_error` precedence;
+> body-read/ctx-cancel/URL classification; trailing slash; `.gitattributes -text` for prompt fixtures; `cmdexec`
+> constructors; `Registry.Executor` for host kinds; `Verdict` fields on error; `diff_truncated` = the 30000-byte cut;
+> `lenses` default 8; non-empty `issue_text`; mock cmd rows keyed by durable ordinal; `cmd` kind `Reduce` cliff check;
+> `JUDGE` model id ≤ `MaxShort`; typed `Verdict` constants; duplicate golden comments refused; single Guarded factory;
+> test rows for unknown-fields-ignored, calibration at the `Judge.Call` level, full-stdout decode, literal bodies.
+>
 > **Port spec:** `~/Developer/harnesseval/harnesseval/{judge,adjudicate,sdlc_loop,usage,model_router,effort}.py` @
 > `19ff9a8`. Slot sources: `match` `golden_comment = Golden.Comment`, `candidate = Finding.IssueText`
 > (`sdlc_loop.py:264`); `adjudicate` `candidate = Finding.IssueText`; `still-present` `golden_comment = Bug.Desc`, where
@@ -41,15 +51,19 @@ type Spec struct { Name string /* declared cmd name; the fake keys on it, the ex
 type Result struct { Stdout, Stderr []byte; ExitCode int; Duration time.Duration }
 type Runner interface { Run(ctx, Spec) (Result, error) }
 type Guarded struct { Runner Runner; Allowed []run.AllowedCmd; Dir string; RunID string; FileHash func(string) (string, error); Audit func(run.Event) error; Environ func() []string; Clock func() time.Time }
-func (g Guarded) Run(ctx, name string, stdin []byte) (converge.CmdResult, error)   // the only entry point (converge.Runner)
-func (g Guarded) Call(ctx, name string, stdin []byte, out any) error               // Run + typed decode
+func NewExecRunner() Runner                                                        // the real one; the fake is mockai's
+func (g Guarded) Run(ctx, name string, stdin []byte) (converge.CmdResult, error)   // converge.Runner
+func (g Guarded) Call(ctx, name string, stdin []byte, out any) error               // shares Run's unaudited core; ONE cmd_call per call (audited after decode)
 ```
+Spec 2's `Deps.Runner` and this package's `kind.Deps.Guarded` are the **same closure** (spec 5 builds one `Guarded` factory).
 `Run`: `name ∈ Allowed` else `ERR_CMD_NOT_ALLOWED{name}` **without audit** (the fold refuses unsanctioned names; the
 check is defense in depth — workflow validation already guarantees names); `Argv[0]` must be absolute else
-`ERR_CMD_NOT_ALLOWED{reason: relative}`; re-verify per spec 2 §2.5 (`ERR_CMD_CHANGED`); execute `Allowed[name].Argv`
+`ERR_CMD_NOT_ALLOWED{reason: relative}` (no audit); re-verify with `workflow.VerifyCmds` (`cmdexec → workflow` edge;
+`ERR_CMD_CHANGED`, no audit — pre-exec refusals are never `cmd_call`s); execute `Allowed[name].Argv`
 verbatim in `Dir` with `Timeout = time.Duration(TimeoutMS) * time.Millisecond` (0 → 60 s); environment = exactly
 {`PATH`, `HOME`, `LANG`, `TMPDIR`} ∩ set-in-`Environ()` + `MRV_RUN_ID=<RunID>` + each `Allowed[name].Env` name that is set;
-stdout/stderr read through `io.LimitReader(MaxPayload+1)` (over → `ERR_CMD_OUTPUT_INVALID{reason: too_large}`);
+stdout/stderr collected by a capping writer that keeps draining (so a chatty child never stalls on a full pipe); more than
+`MaxPayload` bytes → `ERR_CMD_OUTPUT_INVALID{reason: too_large}` after the process ends;
 non-zero exit → `ERR_CMD_FAILED{exit}`; timeout → `ERR_CMD_TIMEOUT`; spawn failure → `ERR_CMD_FAILED{reason: spawn}`.
 Every **execution** (success or failure) appends `cmd_call{Name, Argv, InputHash: sha256(stdin), Stdout: CapText(MaxDetail),
 Stderr: CapText(MaxStderr) (+ `*_truncated`), ExitCode (−1 on spawn/timeout), DurationMS, Error: code}` via `Audit`
@@ -63,7 +77,7 @@ type Request struct { Kind, Model, Effort string; Input any; RunID, Node string;
 type Verdict struct { Kind, Model, Effort, InputHash string; Raw string /* never persisted */; Parsed json.RawMessage /* nil on parse failure */; ParseError string; Confidence float64; Tokens run.TokenTotals; Mock bool; Duration time.Duration; Attempts int }
 type Judge interface { Call(ctx, Request) (Verdict, error) }
 type Doer interface { Do(*http.Request) (*http.Response, error) }
-type Keys struct { Anthropic, OpenAI string }; type URLs struct { Anthropic, OpenAI string }
+type Keys struct { Anthropic, OpenAI string }; type URLs struct { Anthropic, OpenAI string }   // "" → DefaultURLs (https://api.anthropic.com, https://api.openai.com)
 type Clock struct { Now func() time.Time; After func(time.Duration) <-chan time.Time }
 func New(doer Doer, keys Keys, urls URLs, nonce func() string, clock Clock) Judge
 func NewHTTPClient(timeout time.Duration) *http.Client                      // CheckRedirect refuses ALL redirects → ERR_JUDGE_REDIRECT (terminal, never retried)
@@ -77,13 +91,16 @@ func FenceBlock(nonce string, v any) string    // "The following is data to eval
 |---|---|---|---|---|---|---|
 | `match` | `{golden run.Golden, candidate run.Finding}` | "You are a precise code review evaluator. Always respond with valid JSON." | `judge.py:22` | 1024 | `{reasoning, match, confidence}` | `best` starts 0.0; wins iff `match && confidence > best`; parse error ⇒ pair skipped |
 | `adjudicate` | `{diff, diff_truncated, diff_context_hash, candidate run.Finding}` | "You are a strict code review verifier. Always respond with valid JSON." | `adjudicate.py:21` | 2048 | `{reasoning, is_real, confidence}` | real iff `is_real && confidence >= 0.7`; parse error ⇒ not real |
-| `still-present` | `{bug run.Bug, diff, diff_truncated, diff_context_hash}` | same | product: `sdlc_loop.py:321` rewritten + confidence line; calibration: rewritten only | product 1024 / calibration 512 | `{reasoning, still_present, confidence?}` | parse error or missing bool ⇒ still present, confidence 0 |
+| `still-present` | `{bug run.Bug, diff, diff_truncated, diff_context_hash}` | same | product: `sdlc_loop.py:321` rewritten + confidence line; calibration: rewritten only | product 1024 / calibration 512 | `{reasoning, still_present *bool, confidence?}` | parse error or missing bool ⇒ still present, confidence 0, `Error: "parse: missing still_present"` (the persisted verdict then carries `"still_present":null`, never a false `false`) |
 
 `InputHash = sha256(Canonical(input))`. `diff` cut to ≤ 30000 bytes at a rune boundary (Python: 30000 chars — ledgered);
-`diff_context_hash = sha1(cut bytes)` names the cut diff. `Calibration` selects calibration templates/max_tokens and
+`diff_truncated` = whether **this** cut shortened it (spec 2's 1 MB `Diff.Truncated` is OR-ed in); `diff_context_hash =
+sha1(cut bytes)` names the cut diff. `Model` must be ≤ `MaxShort` canonical bytes (`ERR_JUDGE_MODEL`) before any call. `Calibration` selects calibration templates/max_tokens and
 forces `Fence=false`. Effort vocabulary: `low | medium | high | xhigh`; anything else → `ERR_JUDGE_EFFORT_UNSUPPORTED{effort}`.
 
 ### 3.2 Templates, rendering, fencing, goldens
+(`testdata/fsm/judge/prompts/*` carry `-text` in `.gitattributes`; a `.python.txt` body = every byte after the first `\n`,
+no trailing newline — the literals end in `}}` — asserted by J1.)
 `RenderPrompt` = single left-to-right pass emulating `str.format`: `{{`→`{`, `}}`→`}`, `{name}`→ value (values are never
 rescanned), any other `{`/`}` or unknown name → `ERR_PROMPT_TEMPLATE`. Fenced (`adjudicate`/`still-present`, product
 mode): the `{diff}` and `{candidate}`/`{golden_comment}` slot values are replaced by `FenceBlock(nonce, value)`; the
@@ -102,22 +119,31 @@ literal must equal `python.txt` (failure = fail, not skip); absent → that laye
 ### 3.3 Parsing
 `stripFences(s)`: if `s` starts with "```" (no trimming first — parity with `model_router._strip_fences`), take
 `strings.SplitN(s, "```", 3)[1]`, strip a leading `json`, `TrimSpace`. `json.Unmarshal` into the typed verdict (strict
-types: `"match": "true"` is a parse error — ledgered vs Python's coercion); unknown fields ignored. `Parsed` = canonical
-re-encoding of the typed struct (absent `confidence` materializes as 0 — stated); `> MaxDetail` → parse error.
-Response bodies are read through `io.LimitReader(4 MB)` (over → `ERR_JUDGE_RESPONSE`).
+types: `"match": "true"` is a parse error — ledgered vs Python's coercion); **unknown fields are ignored** (never
+`DisallowUnknownFields` here — models add keys). `Parsed` = canonical re-encoding of the typed struct (absent
+`confidence` materializes as 0 — stated; still-present's missing bool stays `null`); `> MaxDetail` → parse error.
+Response bodies are read through `io.LimitReader(4 MB)` (over → `ERR_JUDGE_RESPONSE`); a body read error mid-response
+is a transport error (retried); a `ctx` cancellation during a backoff sleep returns `ctx.Err()` immediately (the sleep
+selects on `ctx.Done()`).
 
 ### 3.4 Providers
 Routing: `anthropic` for `claude*`/`anthropic/*`; `openai` for `gpt*`/`openai/*`/`glm*`/`kimi*`; else `ERR_JUDGE_MODEL`.
-Missing key → `ERR_JUDGE_KEY{provider}`. `URLs` come only from `ANTHROPIC_BASE_URL`/`OPENAI_BASE_URL` (spec 5 `RealDeps`);
-must be `https`, or `http` with hostname exactly `localhost`/`127.0.0.1`/`::1` (any port), no userinfo, no path/query/
-fragment (`ERR_JUDGE_URL`).
+Missing key → `ERR_JUDGE_KEY{provider}`. `URLs` come only from `ANTHROPIC_BASE_URL`/`OPENAI_BASE_URL` (spec 5 `RealDeps`;
+unset → `DefaultURLs`); an override must parse (`ERR_JUDGE_URL`), be `https`, or `http` with hostname exactly
+`localhost`/`127.0.0.1`/`::1` (any port), no userinfo, and a path of `""` or `/` (stripped); query/fragment refused.
+Overrides are agent-satisfiable and unstamped (ledger): spec 5 lists them with the other agent-satisfiable knobs and
+`--agent-prompt` says so.
 - **Anthropic** `POST {base}/v1/messages`: `model`, `system`, `messages:[{user}]`, `max_tokens`; `temperature: 0` for ids
   containing `opus-4-5`/`sonnet-4-5`. Effort: **calibration** → `thinking: {type: "disabled"}` (the reference's `medium`
-  body); **product** → ids matching `claude-opus-4-5*`, `claude-*-4-6*`, `claude-*-5*` (effort-capable) send
-  `output_config: {effort}` (no beta header); other ids: `low`/`medium` → `thinking: disabled`, `high` → `thinking:
-  {type: "enabled", budget_tokens: 8192}`, `xhigh` → `budget_tokens: 32768`, with `max_tokens += budget` and no
-  `temperature` (reference `effort.py:20-32`). A 400 mentioning `output_config`/`effort`/`thinking` →
-  `ERR_JUDGE_EFFORT_UNSUPPORTED{model}`. Headers `x-api-key`, `anthropic-version: 2023-06-01`. Text = concatenation of
+  body; calibration requires `Effort == medium` on every provider, else `ERR_JUDGE_EFFORT_UNSUPPORTED{reason: calibration}`);
+  **product** → an explicit family table (prefix match on the id after an optional `anthropic/`): effort-capable =
+  `claude-opus-4-5`, `claude-opus-4-6`, `claude-sonnet-4-6`, `claude-opus-4-7`, `claude-opus-4-8`, `claude-opus-5`,
+  `claude-sonnet-5`, `claude-fable-5`, `claude-mythos-5` → `output_config: {effort}` (no beta header, no `thinking`);
+  legacy-thinking = `claude-sonnet-4-5`, `claude-haiku-4-5`, `claude-3-` → `low`/`medium`: `thinking: disabled`;
+  `high`: `thinking: {type: "enabled", budget_tokens: 8192}`; `xhigh`: `budget_tokens: 32768`, with `max_tokens +=
+  budget` and `temperature: 1` (this product table is Go's own — the reference has no `high` and sizes `xhigh` from
+  `max_tokens`; ledgered); any other `claude*` id → `ERR_JUDGE_MODEL{reason: unknown_family}`. A 400 mentioning
+  `output_config`/`effort`/`thinking` → `ERR_JUDGE_EFFORT_UNSUPPORTED{model}`. Headers `x-api-key`, `anthropic-version: 2023-06-01`. Text = concatenation of
   `content[].text` (`type == "text"`; ledger: reference took the first block). Tokens from `usage.{input_tokens,
   cache_read_input_tokens, cache_creation_input_tokens, output_tokens}`.
 - **OpenAI-compatible** `POST {base}/v1/chat/completions`: `model`, `messages`, `max_completion_tokens` (= cap; `glm*`/`kimi*`
@@ -128,11 +154,15 @@ fragment (`ERR_JUDGE_URL`).
   `completion_tokens_details.reasoning_tokens` (`Reasoning`, 0 if absent), `Output = max(0, completion_tokens − Reasoning)`.
 No text / non-JSON body / missing `usage` → `ERR_JUDGE_RESPONSE{detail}` (tokens of earlier attempts kept). **Retry** (≤ 5
 attempts): on 429, 5xx (incl. 529), transport errors other than `ERR_JUDGE_REDIRECT`, or a non-2xx JSON body with
-`error.type == "overloaded_error"`; never on a 2xx body. Sleeps via `clock.After`: 429/`overloaded_error` →
-`min(10·3^a, 120)` s (10, 30, 90, 120); others → `2^a` s (1, 2, 4, 8). Exhausted → `ERR_JUDGE_HTTP{status}` /
+`error.type == "overloaded_error"`; never on a 2xx body. Sleeps via `clock.After` (select with `ctx.Done()`):
+429 or any status whose body is `overloaded_error` (incl. 529) → `min(10·3^a, 120)` s (10, 30, 90, 120); other
+retryable → `2^a` s (1, 2, 4, 8). Exhausted → `ERR_JUDGE_HTTP{status}` /
 `ERR_JUDGE_TRANSPORT`; other statuses → `ERR_JUDGE_HTTP` immediately. 180 s per attempt via `context.WithTimeout`
 (`Clock.Now` based deadline). `Verdict.Tokens` sums every attempt; `Attempts` counts them.
 
+On error `Judge.Call` returns a `Verdict` whose `InputHash`, `Tokens` (earlier attempts), `Duration`, and `Attempts`
+are valid alongside the error; executors use them for the `llm_call`.
+
 ### 3.5 MockJudge
 `NewMock(Script)`: key `(kind, node, iter, index)`; row `Raw` goes through the real parser (so `Parsed` bytes match real
 runs); `Error` non-empty → that `ERR_*` is returned; unscripted → `ERR_MOCK_UNSCRIPTED{key}`; `ExpectModel`/
@@ -153,15 +183,21 @@ Executors `Audit` immediately after each `Judge.Call` returns; a non-parse error
 ## 4. `kind`
 ### 4.1 Common
 ```go
-type Deps struct { Judge judge.Judge; Guarded func(allowed []run.AllowedCmd, workDir, runID string, audit func(run.Event) error) cmdexec.Guarded; Clock func() time.Time; Mock bool }
-func New(d Deps) *Registry     // Registry.Mock() == d.Mock; the CLI passes MockJudge + the scenario Runner when --mock-ai / snap.Mock
+type Deps struct { Judge judge.Judge; Guarded func(allowed []run.AllowedCmd, workDir, runID string, audit func(run.Event) error) converge.Runner /* the same closure as machine.Deps.Runner; Call = cmdexec.Call(runner, …) */; Mock bool }
+func New(d Deps) (*Registry, error)   // Registry.Mock() == d.Mock; New refuses Mock:true with a non-*judge.MockJudge and Mock:false with one (ERR_MOCK_MISMATCH)
+// Bug.Verdict constants: run.VerdictMatched = "matched", run.VerdictRealButUngold = "real_but_ungold", run.VerdictHallucination = "hallucination" (typed in run; Decode validates the set for every kind incl. cmd)
+// Registry.Executor(name) for host-only kinds returns (nil, false).
 ```
-`Info()`: `review-lenses {subagent, [inline subagent], ValidateParams: lenses ∈ 1..8}`, `match-then-adjudicate {fork, [fork]}`,
+`Info()`: `review-lenses {subagent, [inline subagent], ValidateParams: lenses absent (→ 8) or an integer 1..8}`, `match-then-adjudicate {fork, [fork]}`,
 `agent-edit {inline, [inline subagent]}`, `still-present {fork, [fork]}`, `cmd {fork, [fork]}`. `Instructions` returns
 `Text` (untrusted values only inside `FenceBlock`s), `Input` (`base_sha`, `head_sha`, `iteration`, `diff_truncated`, +
 untrusted keys), `Untrusted`, documentation `OutputSchema`. `Decode` (used by `Record` and by executors on their own
-output): `DisallowUnknownFields`; lists ≤ `MaxDeltaList`; `IssueText` ≤ `MaxText`, `Desc` ≤ `MaxDesc`, `Summary` ≤
-`MaxShort`; `len(Canonical(output)) ≤ MaxPayload − 128` (envelope margin) → else `ERR_NODE_OUTPUT_INVALID{reason}`.
+output): `DisallowUnknownFields`; lists ≤ `MaxDeltaList`; `IssueText` non-empty and ≤ `MaxText`, `Desc` ≤ `MaxDesc`,
+`Summary` ≤ `MaxShort`, every other string field (`File`, `Severity`, `Category`, `Source`, `ID`, `Verdict`, `Commit`)
+≤ `MaxShort`, `Verdict` ∈ the constant set; `len(Canonical(output)) ≤ MaxPayload − 128` (envelope margin) → else
+`ERR_NODE_OUTPUT_INVALID{reason}`. Goldens are capped at `MaxDesc` at init (spec 2 §5.3 step 4, `ERR_GOLDENS_INVALID`),
+so a matched bug's `Desc = Golden.Comment` always fits; duplicate golden comments are refused there too (IDs are
+`BugID(comment)`).
 Effective bounds (ledger): ~120 full-`Desc` bugs or ~60 full-`IssueText` findings per output.
 
 | kind | Instructions → host | Decode | Reduce |
@@ -170,18 +206,21 @@ Effective bounds (ledger): ~120 full-`Desc` bugs or ~60 full-`IssueText` finding
 | `match-then-adjudicate` | `ERR_EXEC_UNSUPPORTED` | `{Confirmed, Rejected}` (`Rejected` `Desc` ≤ `MaxShort`) | `Confirmed`; fails `ERR_TOO_MANY_BUGS` when `|AllFound ∪ Confirmed| > MaxDeltaList` |
 | `agent-edit` | fix each bug in `input.unfixed_bugs` (= `AllFound` minus fixed statuses, fenced), commit, no push/amend; return `{"commit","summary"}` | `{Commit ^[0-9a-f]{7,40}$, Summary}` | `Commit` |
 | `still-present` | `ERR_EXEC_UNSUPPORTED` | `{Status}` | `Status` |
-| `cmd` | `ERR_EXEC_UNSUPPORTED` | `run.Delta` | as decoded |
+| `cmd` | `ERR_EXEC_UNSUPPORTED` | `run.Delta` (same caps) | as decoded; `ERR_TOO_MANY_BUGS` when `|AllFound ∪ Confirmed| > MaxDeltaList` |
 
 ### 4.2 `match-then-adjudicate` executor
 Input: `snap.Findings`, `snap.Goldens`, `diff`. Candidates are **deduplicated by `IssueText`** (first occurrence kept —
 Python keys by text). Calls are numbered from `StartIndex`.
 1. If goldens: for `g` (outer) × `c` (inner): `match` call — every pair, serially. Per golden: `best = 0.0`; candidate
    `c` becomes the provisional winner iff `match && confidence > best` and is marked *seen* (Python `candidate_matched`);
-   the final winner gets `Verdict: matched`, `GoldenIdx`, `Desc = Golden.Comment`, `ID = BugID(Golden.Comment)`. A
-   candidate may win several goldens (one `Bug` per golden). Superseded provisional winners stay *seen*: neither
+   the final winner gets `Verdict: matched`, `GoldenIdx`, `Desc = Golden.Comment`, `ID = BugID(Golden.Comment)`,
+   `Confidence` = the winning match confidence, `File`/`Line` = the candidate's (location only; its text never
+   propagates). A candidate may win several goldens (one `Bug` per golden). Superseded provisional winners stay *seen*: neither
    confirmed nor adjudicated (reference bookkeeping — ledgered).
 2. Every candidate never *seen*, in order → `adjudicate` call (indexes continue); real → `Verdict: real_but_ungold`,
-   `Desc = CapText(IssueText, MaxDesc)`, `ID = BugID(IssueText)`; not real → `Rejected{Verdict: hallucination}`.
+   `Desc = CapText(IssueText, MaxDesc)` (ledger: the reference passes the full text; `MaxText` candidates are cut at 2 KB),
+   `ID = BugID(IssueText)`, `Confidence` = adjudicate confidence, `File`/`Line` = the candidate's; not real →
+   `Rejected{Verdict: hallucination, Desc: CapText(IssueText, MaxShort), ID: BugID(IssueText), Confidence, File, Line}`.
 3. Output `{Confirmed (golden order then candidate order), Rejected}`; self-`Decode` before returning
    (`ERR_NODE_OUTPUT_INVALID` → executor error). No goldens → step 2 only.
 
@@ -200,10 +239,11 @@ calls:
   - {kind: adjudicate, node: adjudicate, iter: 0, index: 0, raw: '{"reasoning":"...","is_real":true,"confidence":0.9}', tokens: {input: 10, output: 5, cache_read: 0, cache_create: 0, reasoning: 0}, expect_model: gpt-5.2, expect_input_hash: "…"}
   - {kind: match, node: adjudicate, iter: 1, index: 3, error: ERR_JUDGE_HTTP}
 cmds:
-  - {name: notify, stdout: '{"stop": false, "reason": ""}', stderr: "", exit: 0, repeat: true}
+  - {name: notify, call: 0, stdout: '{"stop": false, "reason": ""}', stderr: "", exit: 0}
+  - {name: notify, call: 1, stdout: '{"stop": true, "reason": "plateau"}', exit: 0, repeat: true}
 ```
 `Load(dir) (*Scenario, error)` (own yaml-tagged wire structs, `KnownFields(true)`); `Scenario.Hash()` = sha256 of the
-scenario **file bytes**; `Scenario.Script() judge.Script`; `Scenario.Runner() cmdexec.Runner` (matches `Spec.Name`; rows
+`judge.yaml` **file bytes** (the only file in the directory that is read); `Scenario.Script() judge.Script`; `Scenario.Runner() cmdexec.Runner` (matches `Spec.Name`; rows
 consumed in order unless `repeat`; unscripted → `ERR_MOCK_UNSCRIPTED{name}`; executes nothing).
 
 ## 6. Vars — `JUDGE`/`JUDGE_EFFORT` required (at HEAD); unset → spec 2 `ERR_VAR_UNSET`; spec 5 maps to `ERR_JUDGE_UNSET`.
@@ -219,9 +259,9 @@ consumed in order unless `repeat`; unscripted → `ERR_MOCK_UNSCRIPTED{name}`; e
 ## 8. Tests (100% each; TDD; discriminating fixtures)
 | pkg | rows |
 |---|---|
-| cmdexec | X1 real runner through `Guarded` with a helper binary (`-test.run=TestHelperProcess --` in the pinned argv) printing `os.Args`/`os.Environ()` as JSON: `; rm -rf x`, `$HOME`, `*`, embedded space verbatim; env set equals the derived expected set (injected `Environ` containing `SECRET_TOKEN`, `PATH`, `HOME`, and a declared `TOKEN`; parent `t.Setenv("SECRET_TOKEN")`; `SECRET_TOKEN` absent, `MRV_RUN_ID` present, declared-but-unset name absent); dir; stdin; exit codes; timeout: grandchild `sleep 30`, `elapsed ∈ [Timeout, Timeout+WaitDelay+1s]`, `ERR_CMD_TIMEOUT`, grandchild gone; `TimeoutMS 1500` → fake sees `1500ms` (literal), default → `60s`, positive row (2000 ms, child 200 ms). X2 `Guarded.Run`: not-allowed → error and **no** audit; relative `argv[0]` refused; mismatch/missing/appeared; pinned argv executed (fake sees `Allowed.Argv`); failed, spawn failure, success; `cmd_call` fields incl. `InputHash` literal, truncation flags, `Error` on decode failure (`Call`); audit error propagates; stdout over `MaxPayload` → `too_large` |
-| judge | J1 goldens: `.python.txt` sha literals; rewrite == constant for all four templates (unconditional); sibling layer; `.plain.golden`/`.fenced.golden`; `match` fenced == unfenced; `RenderPrompt` rows: `{{`/`}}`/`{candidate}` inside values, lone `}` and unknown `{slot}` in a template → `ERR_PROMPT_TEMPLATE`. J2 `stripFences`: no fence, ```json, multi-fence, trailing text, lone fence, **leading whitespace before the fence → parse error**, prose before fence → parse error. J3 parsers: booleans present/missing/non-JSON/string-typed; still-present both fail-close triggers (confidence 0); adjudicate 0.7/0.6999/`is_real:false`+0.99; `Parsed` over `MaxDetail`; absent confidence → 0. J4 request shapes via recording `Doer`: table effort `{low, medium, high, xhigh, bogus}` × `{gpt-5.2, glm-4, kimi-k2, claude-opus-4-5, claude-sonnet-4-5, claude-opus-5}` × calibration `{true, false}` asserting literal `reasoning_effort`/`output_config`/`thinking`/`temperature`/`max_tokens`(+budget)/`max_completion_tokens` or `ERR_JUDGE_EFFORT_UNSUPPORTED`; no beta header; still-present `max_tokens` 512/1024 per mode on both providers; token accounting with four distinct nonzero values per provider incl. `cached_tokens`, missing `completion_tokens_details` → `Output = completion_tokens`, `reasoning > completion` → 0; multi-block and empty content; missing `usage`; effort 400; body over 4 MB. J5 retry with injected `After`: `[10,30,90,120]` for 429×4, `[10,30,90,120]` for `overloaded_error`×4, `[1,2,4,8]` for 5xx×4 and 529×4 and transport×4, mixed `429,500,429 → [10,2,90]`, 5xx×5 → `ERR_JUDGE_HTTP` after 4 sleeps, transport×5 → `ERR_JUDGE_TRANSPORT`, 200 body containing "overloaded" **not** retried, 400 immediate, `Attempts`/summed `Tokens`, per-attempt deadline ≈ `Now+180s`. J6 URLs (ports, `LOCALHOST`, path/query rejected, userinfo, other hosts) + `NewHTTPClient` same/cross-host redirect → `ERR_JUDGE_REDIRECT`, `Attempts == 1`, zero sleeps; routing table; missing key per provider. J7 mock: scripted, unscripted, `expect_*` literals, near-miss keys, `Error` rows, `Raw` through the real parser, `Calls()` incl. errored; nonce uniqueness. J8 fixture manifest: no `sk-ant-`, `sk-proj-`, `sk-`, `Bearer `; dummy key literal pinned; `ParseError` is the decoder message only. J9 `InputHash` literal per kind; `diff_context_hash` literals for 29999/30000/30001 + rune straddle |
-| kind | K1 `Decode` accept/reject per kind incl. commit 6/7/40/41/uppercase, `Summary` at cap/+1, lists at 256/257, `IssueText`/`Desc` at cap/+1, canonical at `MaxPayload−128`/+1, `cmd` Delta caps (desc, status, commit), unknown field per kind. K2 `Reduce` incl. `ERR_TOO_MANY_BUGS` at 257 (accept at 256). K3 composition with `MockJudge` at `iter: 2`, `StartIndex: 5`: 2 goldens × 3 findings → indexes `[5..10]` then adjudicate `11,12` for the two never-seen; equal-confidence tie → first; `confidence 0` never matches; one candidate wins both goldens (two `Bug`s, `Desc` = each golden's comment); superseded provisional winner is neither confirmed nor adjudicated (1×2: 0.5 then 0.9 → adjudicate indexes `[]`); duplicate `IssueText` collapsed; parse error on one pair → skipped, `llm_call.Error` set; no goldens; `Rejected`; HTTP error aborts after audit; output over cap → error. K4 still-present order, `Iter` propagated, 256 ok. K5 `Instructions`: raw untrusted value absent outside fences; `lenses` `ValidateParams` 0/1/8/9; `unfixed_bugs` = unfixed subset; schema shows the commit regex. K6 cmd kind via fake Runner: payload literal (vars hashed, no node outputs), delta decoded; `Instructions` → `ERR_EXEC_UNSUPPORTED` ×3. K7 `Info()` table; `New(...).Mock()`; `llm_call` events per §3.6 (success/parse/HTTP) with `Index` from `StartIndex` |
+| cmdexec | X1 real runner through `Guarded` with a helper binary (`-test.run=TestHelperProcess --` in the pinned argv) printing `os.Args`/`os.Environ()` as JSON: `; rm -rf x`, `$HOME`, `*`, embedded space verbatim; env set equals the derived expected set (injected `Environ` containing `SECRET_TOKEN`, `PATH`, `HOME`, and a declared `TOKEN`; parent `t.Setenv("SECRET_TOKEN")`; `SECRET_TOKEN` absent, `MRV_RUN_ID` present, declared-but-unset name absent); dir; stdin; exit codes; timeout: grandchild `sleep 30`, `elapsed ∈ [Timeout, Timeout+WaitDelay+1s]`, `ERR_CMD_TIMEOUT`, grandchild gone; `TimeoutMS 1500` → fake sees `1500ms` (literal), default → `60s`, positive row (2000 ms, child 200 ms). X2 `Guarded.Run`: not-allowed → error and **no** audit; relative `argv[0]` refused (no audit); `ERR_CMD_CHANGED` refused (no audit); literal expected env `{PATH=…, HOME=…, MRV_RUN_ID=<id>, TOKEN=…}` (no `LANG`/`TMPDIR` when unset); stdout at exactly `MaxPayload` accepted, over → `too_large`, stderr over cap → `too_large`; `Call` decodes a valid JSON stdout of `MaxDetail+1` bytes (audited copy truncated, `Error == ""`); timeout/spawn rows assert `exit_code == -1` and the `Error` literal; `Spec.Ordinal` = prior `cmd_call` count; mismatch/missing/appeared; pinned argv executed (fake sees `Allowed.Argv`); failed, spawn failure, success; `cmd_call` fields incl. `InputHash` literal, truncation flags, `Error` on decode failure (`Call`); audit error propagates; stdout over `MaxPayload` → `too_large` |
+| judge | J0 authority: every expected request body in J4 is a hand-written literal JSON fragment per cell (kind × provider × effort × mode), incl. `anthropic-version: 2023-06-01`, message roles, `match` 1024 / `adjudicate` 2048; an unambiguous legacy id (`claude-sonnet-4-5`) pins the `thinking` bodies (`high`: `budget_tokens: 8192`, `max_tokens: 1024+8192`, `temperature: 1`; `xhigh`: 32768); `claude-3-7-sonnet-latest` legacy; `claude-opus-4-7` effort-capable; unknown `claude-zeta` → `ERR_JUDGE_MODEL`; calibration with `Effort != medium` → `ERR_JUDGE_EFFORT_UNSUPPORTED` on both providers. J1 goldens: `.python.txt` sha literals; rewrite == constant for all four templates (unconditional); sibling layer; `.plain.golden`/`.fenced.golden`; `match` fenced == unfenced; `RenderPrompt` rows: `{{`/`}}`/`{candidate}` inside values, lone `}` and unknown `{slot}` in a template → `ERR_PROMPT_TEMPLATE`. J2 `stripFences`: no fence, ```json, multi-fence, trailing text, lone fence, **leading whitespace before the fence → parse error**, prose before fence → parse error. J3 parsers: booleans present/missing/non-JSON/string-typed; still-present both fail-close triggers (confidence 0); adjudicate 0.7/0.6999/`is_real:false`+0.99; `Parsed` over `MaxDetail`; absent confidence → 0. J4 request shapes via recording `Doer`: table effort `{low, medium, high, xhigh, bogus}` × `{gpt-5.2, glm-4, kimi-k2, claude-opus-4-5, claude-sonnet-4-5, claude-opus-5}` × calibration `{true, false}` asserting literal `reasoning_effort`/`output_config`/`thinking`/`temperature`/`max_tokens`(+budget)/`max_completion_tokens` or `ERR_JUDGE_EFFORT_UNSUPPORTED`; no beta header; still-present `max_tokens` 512/1024 per mode on both providers; token accounting with four distinct nonzero values per provider incl. `cached_tokens`, missing `completion_tokens_details` → `Output = completion_tokens`, `reasoning > completion` → 0; multi-block and empty content; missing `usage`; effort 400; body over 4 MB. J5 retry with injected `After`: `[10,30,90,120]` for 429×4, `[10,30,90,120]` for `overloaded_error`×4, `[1,2,4,8]` for 5xx×4 (plain body) and transport×4 and body-read-error×4, `[10,30,90,120]` for 529+`overloaded_error`×4, mixed `429,500,429 → [10,2,90]`, 5xx×5 → `ERR_JUDGE_HTTP` after 4 sleeps, transport×5 → `ERR_JUDGE_TRANSPORT`, 200 body containing "overloaded" **not** retried, 400 immediate, `Attempts`/summed `Tokens`, per-attempt deadline ≈ `Now+180s`. J6 URLs (unset → `DefaultURLs`; ports, `LOCALHOST`, trailing `/` stripped, `/v1` path/query rejected, `http://[::1` unparsable, userinfo, other hosts) + `NewHTTPClient` same/cross-host redirect → `ERR_JUDGE_REDIRECT`, `Attempts == 1`, zero sleeps; routing table; missing key per provider. J7 mock: scripted, unscripted, `expect_*` literals, near-miss keys, `Error` rows, `Raw` through the real parser, `Calls()` incl. errored; nonce uniqueness. J8 fixture manifest: no `sk-ant-`, `sk-proj-`, `sk-`, `Bearer `; dummy key literal pinned; `ParseError` is the decoder message only. J9 `InputHash` literal per kind; `diff_context_hash` literals for 29999/30000/30001 + rune straddle |
+| kind | K1 `Decode` accept/reject per kind incl. commit 6/7/40/41/uppercase, `Summary` at cap/+1, lists at 256/257, `IssueText`/`Desc` at cap/+1, canonical at `MaxPayload−128`/+1, `cmd` Delta caps (desc, status, commit), unknown field per kind. K2 `Reduce` incl. `ERR_TOO_MANY_BUGS` at 257 (accept at 256). K3 composition with `MockJudge` at `iter: 2`, `StartIndex: 5` (script rows carry `expect_input_hash` so index ↔ (g outer, c inner) is pinned; `ID` literals `BugID(Golden.Comment)`/`BugID(IssueText)`; `{match:false, confidence:0.99}` never wins; zero candidates → 0 calls and `{Confirmed:[],Rejected:[]}`): 2 goldens × 3 findings → indexes `[5..10]` then adjudicate `11,12` for the two never-seen; equal-confidence tie → first; `confidence 0` never matches; one candidate wins both goldens (two `Bug`s, `Desc` = each golden's comment); superseded provisional winner is neither confirmed nor adjudicated (1×2: 0.5 then 0.9 → adjudicate indexes `[]`); duplicate `IssueText` collapsed; parse error on one pair → skipped, `llm_call.Error` set; no goldens; `Rejected`; HTTP error aborts after audit; output over cap → error. K4 still-present order, `Iter` propagated, 256 ok. K5 `Instructions`: raw untrusted value absent outside fences; `lenses` `ValidateParams` 0/1/8/9; `unfixed_bugs` = unfixed subset; schema shows the commit regex. K6 cmd kind via fake Runner: payload literal (vars hashed, no node outputs), delta decoded; `Instructions` → `ERR_EXEC_UNSUPPORTED` ×3. K7 `Info()` table; `New(...).Mock()`; `llm_call` events per §3.6 (success/parse/HTTP) with `Index` from `StartIndex` |
 | mockai | S1 load errors: unknown key, duplicate key, bad tokens; `Hash()` = sha256 of file bytes (literal), changes on a comment edit; S2 `Script()` conversion incl. `error` rows; S3 runner: ordered rows, `repeat`, unscripted, matches `Spec.Name` not argv |
 
 ## 9. Ledger
@@ -236,4 +276,12 @@ consumed in order unless `repeat`; unscripted → `ERR_MOCK_UNSCRIPTED{name}`; e
 - Kind output shapes frozen under run `SchemaVersion 1`; additive `omitempty` fields do not bump; incompatible changes bump and need a per-kind migrate hook (run follow-up). No prompt identity in `llm_call` (follow-up: fold the template sha into `InputHash` in a later schema).
 - Serial fan-out; `AllFound` cliff refused at adjudicate `Reduce`.
 - `effort.py` added to the port list; provenance vendored (`python.txt`) so CI checks unconditionally.
-- Agent-satisfiable flags (`--allow-custom-cmds`, `--accept-workflow-change`, `--mock-ai`, `--calibration`) documented as such in spec 5's trust boundary; `fsm judge --no-fence` is redundant with calibration runs (spec 5 drops it).
+- Agent-satisfiable knobs (`--allow-custom-cmds`, `--accept-workflow-change`, `--mock-ai`/`MOCK_AI`, `--calibration`, `ANTHROPIC_BASE_URL`/`OPENAI_BASE_URL`, `RepoMode` override) documented as such in spec 5's trust boundary; `fsm judge --no-fence` is redundant with calibration runs (spec 5 drops it); consent depth is argv bytes (PATH-resolved children and HOME dotfiles are the operator's), and `cmd_call` persists capped stdout/stderr (a script echoing a pass-through secret lands it in the audit) — spec 5 docs.
+- Product-mode Anthropic thinking table is Go's own (the reference has no `high` level, sizes `xhigh` from `max_tokens`, and sets `temperature: 1` with thinking); `high` is an addition on every provider; calibration requires `medium`.
+- No goldens → every finding adjudicated (the reference short-circuits to `confirmed: []`); dedup by text changes the call count vs the reference; `Input = prompt_tokens − cached_tokens` (the reference ignores cache on the API path); the reference's glm/kimi one-shot retry on empty/unparseable JSON is not ported (our 5-attempt ladder covers transport; empty content is `ERR_JUDGE_RESPONSE`).
+- `real_but_ungold` `Desc` is cut at `MaxDesc` (2 KB) where the reference passed full text — ledgered parity deviation.
+- Freeze policy: kind output shapes are frozen under run `SchemaVersion 1`; a newer binary must not add fields to persisted shapes without a bump (older binaries decode with `DisallowUnknownFields` and would report `ERR_NODE_OUTPUT_INVALID`/`ERR_COPY_INVALID`); `Parsed` bytes are stable only within a schema version (spec 3 `Diff` compares decision fields + confidence, not `reasoning`).
+- Shipped scenario inventory (plan §4.3: sdlc-loop `{happy, cumulative-convergence, no-findings, no-confirmed, dirty-tree, judge-swap-iter0, judge-swap-frozen, overflow-iterations, overflow-budget, cmd-guardrails, injection}`, review-loop ×5) is authored with the black-box suite (spec 5 §5); a `match-parity` fixture (arrays → exact TP/FP/FN vs `score_from_matches`) is added to K3.
+- Resolved elsewhere (reassignment): SEC-11/SEC-25 → spec 2 §2.5; SEC-14 → spec 3 §2 step 4; SEC-28 → spec 5 `converge --check`; INT-11/FIN-5 → spec 2 §5.3 step 2; INT-22/SEC-26 → spec 5 docs. The `cmd` kind is the realization of design Appendix A's user-defined kind (name fixed, output `run.Delta`; C16 retired JSON-Schema).
+- Overflow handler is audited twice (`cmd_call` by the runner, `overflow_handler` by the machine) — accepted, spec 2 §8.
+- `NEEDS_INPUT` keys are `unfixed_bugs`, `findings_so_far`, `base_sha`, `head_sha`, `iteration`, `diff_truncated`; rubric `rubrics/task-done-review-rubric.md` + the eight lens names from `skills/review-artifact/SKILL.md` step 4 (plan §3.4 wrote `confirmed_bugs`/`findings`/artifact rubric) — spec 5 adopts these.
diff --git a/internal/fsm/converge/converge.go b/internal/fsm/converge/converge.go
index 4e203fd..c37f440 100644
--- a/internal/fsm/converge/converge.go
+++ b/internal/fsm/converge/converge.go
@@ -82,6 +82,13 @@ func Parse(node *yaml.Node, runner Runner) (Predicate, error) {
 	return parse(node, runner, nil, true, 0)
 }
 
+// MustParse binds a tree that already passed Validate (workflow.Parse ran
+// it); the error path of parse cannot recur on validated input.
+func MustParse(node *yaml.Node, runner Runner) Predicate {
+	p, _ := parse(node, runner, nil, true, 0)
+	return p
+}
+
 func bad(detail string) error {
 	return errs.E(CodeBadConvergence, detail, "detail", detail)
 }
diff --git a/internal/fsm/converge/converge_test.go b/internal/fsm/converge/converge_test.go
index aa843a9..58fbf6d 100644
--- a/internal/fsm/converge/converge_test.go
+++ b/internal/fsm/converge/converge_test.go
@@ -288,6 +288,9 @@ func TestC4ValidateAndParseErrors(t *testing.T) {
 	if err := Validate(multi, nil); !errs.Is(err, CodeBadConvergence) {
 		t.Fatal("empty document")
 	}
+	if p := MustParse(node(t, "{cmd: anything}"), &fakeRunner{}); p == nil || p.Name() != "cmd:anything" {
+		t.Fatal("MustParse binds validated input")
+	}
 	// Parse (cmdNames nil) accepts any cmd name — workflow.Parse validated it earlier.
 	if _, err := Parse(node(t, "{cmd: anything}"), &fakeRunner{}); err != nil {
 		t.Fatal(err)
diff --git a/internal/fsm/machine/machine.go b/internal/fsm/machine/machine.go
new file mode 100644
index 0000000..0869197
--- /dev/null
+++ b/internal/fsm/machine/machine.go
@@ -0,0 +1,947 @@
+package machine
+
+import (
+	"bytes"
+	"context"
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"path/filepath"
+	"regexp"
+	"sort"
+	"strings"
+
+	"github.com/dsifry/metareview/internal/fsm/converge"
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/gate"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/fsm/workflow"
+)
+
+// Machine drives one run. It caches nothing across calls except the last
+// View: every Advance/Record re-reads and re-verifies the log under the lock.
+type Machine struct {
+	deps  Deps
+	runID string
+	view  View
+	torn  bool
+}
+
+// session is the per-call working state (spec 2 §5.3b).
+type session struct {
+	m        *Machine
+	ctx      context.Context
+	st       run.FoldState
+	log      run.Log
+	w        *workflow.Workflow // resolved
+	pred     converge.Predicate
+	runner   converge.Runner
+	git      gate.Git
+	warns    []string
+	auditErr error // the first store error seen through the audit closure
+	unlock   func()
+}
+
+var recordName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
+
+// RunID returns the run this machine drives.
+func (m *Machine) RunID() string { return m.runID }
+
+// View returns the read model captured at the end of the last call.
+func (m *Machine) View() View { return m.view }
+
+// ---------------------------------------------------------------- Init
+
+// Init creates a run (spec 2 §5.3).
+func Init(ctx context.Context, deps Deps, o InitOptions) (*Machine, error) {
+	// 1. load + parse + resolve
+	var raw []byte
+	var err error
+	if strings.Contains(o.Workflow, "/") || strings.HasSuffix(o.Workflow, ".yaml") {
+		raw, err = deps.ReadFile(o.Workflow)
+		if err != nil {
+			return nil, errs.E(CodeWorkflowNotFound, err.Error(), "workflow", o.Workflow)
+		}
+	} else {
+		raw, err = deps.Workflows(o.Workflow)
+		if err != nil {
+			return nil, errs.E(CodeWorkflowNotFound, err.Error(), "workflow", o.Workflow)
+		}
+	}
+	if len(raw) > MaxWorkflowBytes {
+		return nil, errs.E(CodeWorkflowTooLarge, fmt.Sprintf("workflow is %d bytes (max %d)", len(raw), MaxWorkflowBytes), "workflow", o.Workflow)
+	}
+	parsed, err := workflow.Parse(raw, workflow.Options{Kinds: deps.Kinds.Info()})
+	if err != nil {
+		return nil, err
+	}
+	switch o.RepoMode {
+	case "":
+	case "enforcing":
+		parsed.RepoMode = "enforcing"
+	default:
+		return nil, errs.E(CodeBadRepoMode, "repo mode override must be empty or enforcing (tighten-only)", "got", o.RepoMode)
+	}
+	w, vars, err := parsed.Resolve(o.Vars, o.Calibration)
+	if err != nil {
+		return nil, err
+	}
+	// 2. commands + consent
+	allowed, sha, err := workflow.ResolveCmds(w, o.WorkDir, deps.LookPath, deps.FileHash)
+	if err != nil {
+		return nil, err
+	}
+	if len(allowed) > 0 && o.AllowCustomCmds != sha {
+		return nil, errs.E(CodeCmdsNotAllowed, consentList(allowed, o.WorkDir), "sha", sha)
+	}
+	// 3. git
+	g := deps.Git(o.WorkDir)
+	common, err := g.CommonDir(ctx)
+	if err != nil {
+		return nil, err
+	}
+	rootCommon, err := deps.Git(o.RepoRoot).CommonDir(ctx)
+	if err != nil {
+		return nil, err
+	}
+	if common != rootCommon {
+		return nil, errs.E(CodeWorkdirForeign, "work dir is not a worktree of the repository", "work_dir", o.WorkDir, "repo_root", o.RepoRoot)
+	}
+	head, err := g.Head(ctx)
+	if err != nil {
+		return nil, err
+	}
+	base := o.Base
+	if base == "" {
+		base = "HEAD"
+	}
+	baseSHA, err := g.RevParse(ctx, base)
+	if err != nil {
+		return nil, err
+	}
+	_, porcelain, err := g.Status(ctx)
+	if err != nil {
+		return nil, err
+	}
+	wt, err := g.WorkTree(ctx)
+	if err != nil {
+		return nil, err
+	}
+	// 4. goldens
+	goldens := []run.Golden{}
+	if o.GoldensPath != "" {
+		goldens, err = readGoldens(deps, o.GoldensPath)
+		if err != nil {
+			return nil, err
+		}
+	}
+	// 5. mock
+	mock := ""
+	if o.MockDir != "" {
+		dir := o.MockDir
+		if !filepath.IsAbs(dir) {
+			dir = filepath.Join(o.RepoRoot, dir)
+		}
+		h, err := deps.MockLoad(dir)
+		if err != nil {
+			return nil, errs.E(CodeMockInvalid, err.Error(), "dir", o.MockDir)
+		}
+		rel := strings.TrimPrefix(strings.TrimPrefix(filepath.Clean(dir), filepath.Clean(o.RepoRoot)), string(filepath.Separator))
+		mock = rel + "#" + h[:16]
+	}
+	if deps.Kinds.Mock() != (mock != "") {
+		return nil, errs.E(CodeMockMismatch, "the kind registry's mock mode does not match --mock-ai")
+	}
+	// 6. create, sidecar, first events
+	now := deps.Clock()
+	runID := o.RunID
+	if runID == "" {
+		runID = run.RunID(w.Name, now.Time)
+	}
+	var initialKind run.Kind
+	if n := w.NodeFor(w.Initial); n != nil {
+		initialKind = run.Kind(n.Kind)
+	}
+	if allowed == nil {
+		allowed = []run.AllowedCmd{}
+	}
+	initData := run.InitData{
+		RunID: runID, CreatedAt: now, Workflow: w.Name, WorkflowHash: w.Hash, Vars: vars, Calibration: o.Calibration,
+		Mock: mock, RepoMode: w.RepoMode, AllowedCmds: allowed, CmdsSHA256: sha, RepoRoot: o.RepoRoot, WorkDir: o.WorkDir,
+		BaseSHA: baseSHA, Head: head, InitialState: w.Initial, InitialKind: initialKind, Goldens: goldens, Lineage: []string{},
+	}
+	m := &Machine{deps: deps, runID: runID}
+	first := run.Event{SchemaVersion: run.SchemaVersion, At: now, Type: run.TypeInit, Data: run.MarshalCanonical(initData)}
+	st, err := deps.Store.Create(runID, first)
+	if err != nil {
+		return nil, err
+	}
+	if err := deps.Sidecar.Write(runID, SidecarWorkflow, raw); err != nil {
+		return nil, err
+	}
+	unlock, err := deps.Store.Lock(runID)
+	if err != nil {
+		return nil, err
+	}
+	defer unlock()
+	sess := &session{m: m, ctx: ctx, st: st, w: w, git: g}
+	status, truncated := run.CapDetail(porcelain)
+	if err := sess.append(run.TypeTree, run.TreeData{Head: head, TreeHash: gate.TreeHash(head, wt), Status: status, StatusTruncated: truncated}, ""); err != nil {
+		return nil, err
+	}
+	for _, warning := range w.Warnings {
+		if err := sess.warn(WarnWorkflow, warning); err != nil {
+			return nil, err
+		}
+	}
+	m.view = sess.viewOf()
+	return m, nil
+}
+
+// consentList renders the human-readable command list for ERR_CMDS_NOT_ALLOWED.
+func consentList(allowed []run.AllowedCmd, workDir string) string {
+	var b strings.Builder
+	b.WriteString("commands this workflow will run (consent with --allow-custom-cmds <sha256>):\n")
+	for _, c := range allowed {
+		var pinned, unpinned []string
+		for _, a := range c.Argv {
+			p := a
+			if !filepath.IsAbs(p) {
+				p = filepath.Join(workDir, a)
+			}
+			if h, ok := c.FileHashes[p]; ok {
+				pinned = append(pinned, fmt.Sprintf("%s=%s", p, h[:min(12, len(h))]))
+			} else {
+				unpinned = append(unpinned, a)
+			}
+		}
+		fmt.Fprintf(&b, "  %s: argv=%q timeout=%dms env=%v\n    pinned: %s\n    unpinned: %s\n", c.Name, c.Argv, c.TimeoutMS, c.Env, strings.Join(pinned, ", "), strings.Join(unpinned, ", "))
+	}
+	return b.String()
+}
+
+func readGoldens(deps Deps, path string) ([]run.Golden, error) {
+	raw, err := deps.ReadFile(path)
+	if err != nil {
+		return nil, errs.E(CodeGoldensInvalid, err.Error(), "path", path)
+	}
+	if len(raw) > MaxGoldensBytes {
+		return nil, errs.E(CodeGoldensInvalid, "goldens file too large", "path", path)
+	}
+	var goldens []run.Golden
+	dec := json.NewDecoder(bytes.NewReader(raw))
+	dec.DisallowUnknownFields()
+	if err := dec.Decode(&goldens); err != nil {
+		return nil, errs.E(CodeGoldensInvalid, err.Error(), "path", path)
+	}
+	if len(goldens) > run.MaxGoldens {
+		return nil, errs.E(CodeGoldensInvalid, fmt.Sprintf("more than %d goldens", run.MaxGoldens), "path", path)
+	}
+	seen := map[string]bool{}
+	for i, g := range goldens {
+		if g.Comment == "" {
+			return nil, errs.E(CodeGoldensInvalid, fmt.Sprintf("golden %d has an empty comment", i), "path", path)
+		}
+		if _, truncated := run.CapText(g.Comment, run.MaxDesc); truncated {
+			return nil, errs.E(CodeGoldensInvalid, fmt.Sprintf("golden %d exceeds %d bytes", i, run.MaxDesc), "path", path)
+		}
+		if seen[g.Comment] {
+			return nil, errs.E(CodeGoldensInvalid, fmt.Sprintf("golden %d duplicates an earlier comment", i), "path", path)
+		}
+		seen[g.Comment] = true
+	}
+	if goldens == nil {
+		goldens = []run.Golden{}
+	}
+	return goldens, nil
+}
+
+// ---------------------------------------------------------------- Open / load
+
+// Open loads an existing run (spec 2 §5.3b).
+func Open(ctx context.Context, deps Deps, runID string, o OpenOptions) (*Machine, error) {
+	m := &Machine{deps: deps, runID: runID}
+	sess, err := m.load(ctx, o.Repair)
+	if err != nil {
+		return nil, err
+	}
+	sess.unlock()
+	m.view = sess.viewOf()
+	return m, nil
+}
+
+// load performs §5.3b steps 1–2 and returns a locked session.
+func (m *Machine) load(ctx context.Context, repair bool) (*session, error) {
+	deps := m.deps
+	unlock, err := deps.Store.Lock(m.runID)
+	if err != nil {
+		return nil, err
+	}
+	sess := &session{m: m, ctx: ctx, unlock: unlock}
+	ok := false
+	defer func() {
+		if !ok {
+			unlock()
+		}
+	}()
+	log, _, err := deps.Store.EventsWithLines(m.runID)
+	if err != nil {
+		return nil, err
+	}
+	if repair {
+		if err := deps.Store.RepairTail(m.runID); err != nil {
+			return nil, err
+		}
+		dropped := len(log.Torn.Bytes)
+		log, _, err = deps.Store.EventsWithLines(m.runID)
+		if err != nil {
+			if isStoreCode(err, run.CodeRunNotFound) {
+				return nil, errs.E(run.CodeRunNotFound, "run removed; torn bytes in runs/.torn/", "run", m.runID)
+			}
+			return nil, err
+		}
+		st, err := run.FoldFull(log.Events)
+		if err != nil {
+			return nil, err
+		}
+		st.ChainHead = log.Head
+		sess.st, sess.log = st, log
+		if err := sess.warn(WarnAuditTornLineDropped, fmt.Sprintf("%d bytes dropped after seq %d from audit.jsonl", dropped, st.Seq)); err != nil {
+			return nil, err
+		}
+		log, _, err = deps.Store.EventsWithLines(m.runID)
+		if err != nil {
+			return nil, err
+		}
+	}
+	m.torn = log.Torn != nil
+	st, err := run.FoldFull(log.Events)
+	if err != nil {
+		return nil, err
+	}
+	st.ChainHead = log.Head
+	sess.st, sess.log = st, log
+	snap := st.Snapshot
+	raw, err := deps.Sidecar.Read(m.runID, SidecarWorkflow)
+	if err != nil {
+		return nil, err
+	}
+	sum := sha256.Sum256(raw)
+	if got := hex.EncodeToString(sum[:]); got != snap.WorkflowHash {
+		return nil, errs.E(CodeWorkflowChanged, "workflow sidecar does not match the run", "expected", snap.WorkflowHash, "got", got)
+	}
+	parsed, err := workflow.Parse(raw, workflow.Options{Kinds: deps.Kinds.Info()})
+	if err != nil {
+		return nil, err
+	}
+	parsed.RepoMode = snap.RepoMode
+	w, _, err := parsed.Resolve(snap.Vars, false)
+	if err != nil {
+		return nil, err
+	}
+	sess.w = w
+	if err := workflow.VerifyCmds(snap.AllowedCmds, snap.WorkDir, deps.FileHash); err != nil {
+		return nil, err
+	}
+	if snap.Mock != "" {
+		i := strings.LastIndex(snap.Mock, "#")
+		rel, want := snap.Mock[:i], snap.Mock[i+1:]
+		h, err := deps.MockLoad(filepath.Join(snap.RepoRoot, rel))
+		if err != nil || h[:16] != want {
+			return nil, errs.E(CodeMockMismatch, "mock scenario changed or missing", "mock", snap.Mock)
+		}
+	}
+	if deps.Kinds.Mock() != (snap.Mock != "") {
+		return nil, errs.E(CodeMockMismatch, "the kind registry's mock mode does not match the run")
+	}
+	sess.git = deps.Git(snap.WorkDir)
+	sess.runner = deps.Runner(snap.AllowedCmds, snap.WorkDir, m.runID, sess.audit)
+	if w.Convergence != nil {
+		sess.pred = converge.MustParse(w.Convergence, sess.runner) // validated by workflow.Parse
+	}
+	ok = true
+	return sess, nil
+}
+
+func isStoreCode(err error, code string) bool {
+	var se *run.StoreError
+	return errors.As(err, &se) && se.Code == code
+}
+
+// ---------------------------------------------------------------- session helpers
+
+// stamp builds an event with the machine's stamps (spec 2 §5.4 tail).
+func (s *session) stamp(typ string, data any, node string) run.Event {
+	snap := s.st.Snapshot
+	return run.Event{
+		SchemaVersion: run.SchemaVersion, At: s.m.deps.Clock(), Type: typ, State: snap.State, Iter: snap.Iteration,
+		Node: node, Mock: snap.Mock != "", Data: run.MarshalCanonical(data),
+	}
+}
+
+// append stores an event and rebinds the session state.
+func (s *session) append(typ string, data any, node string) error {
+	return s.appendEvent(s.stamp(typ, data, node))
+}
+
+func (s *session) appendEvent(ev run.Event) error {
+	st, err := s.m.deps.Store.Append(s.m.runID, s.st, ev)
+	if err != nil {
+		return err
+	}
+	s.st = st
+	return nil
+}
+
+// audit is the closure handed to executors and runners: it stamps and appends.
+func (s *session) audit(ev run.Event) error {
+	snap := s.st.Snapshot
+	ev.SchemaVersion = run.SchemaVersion
+	ev.At = s.m.deps.Clock()
+	ev.State = snap.State
+	ev.Iter = snap.Iteration
+	ev.Mock = snap.Mock != ""
+	if ev.Type == run.TypeLLMCall {
+		if n := s.w.NodeFor(snap.State); n != nil {
+			ev.Node = n.Name
+		}
+	} else {
+		ev.Node = ""
+	}
+	if err := s.appendEvent(ev); err != nil {
+		if s.auditErr == nil {
+			s.auditErr = err
+		}
+		return err
+	}
+	return nil
+}
+
+func (s *session) warn(code, detail string) error {
+	d, _ := run.CapText(detail, run.MaxText)
+	if err := s.append(run.TypeWarn, run.WarnData{Code: code, Detail: d}, ""); err != nil {
+		return err
+	}
+	s.warns = append(s.warns, code+": "+d)
+	return nil
+}
+
+func (s *session) gateEvent(name string, gerr *run.GateError) error {
+	return s.append(run.TypeGate, run.GateData{Name: name, Passed: gerr == nil, Error: gerr}, "")
+}
+
+func pseudoGate(name, code, detail string) *run.GateError {
+	d, truncated := run.CapDetail(detail)
+	return &run.GateError{Code: code, Gate: name, Detail: d, DetailTruncated: truncated}
+}
+
+// viewOf computes the read model.
+func (s *session) viewOf() View {
+	snap := s.st.Snapshot
+	v := View{RunID: s.m.runID, Workflow: snap.Workflow, Snapshot: snap, NextAction: NextAdvance, Torn: s.m.torn}
+	if snap.Outcome != "" {
+		v.NextAction = NextNone
+	}
+	events := s.log.Events
+	if log, err := s.m.deps.Store.Events(s.m.runID); err == nil {
+		events = log.Events
+	}
+	if n := s.w.NodeFor(snap.State); n != nil {
+		k := run.Key(n.Name, snap.Iteration)
+		_, has := snap.NodeOutputs[k]
+		v.Node = &NodeView{Name: n.Name, Kind: n.Kind, Exec: n.Exec, HasOutput: has, Applied: snap.Applied[k]}
+		if snap.Outcome == "" && n.Exec != "fork" && !has && hasNeedsInput(events, n.Name, snap.Iteration) {
+			v.NextAction = NextRecord
+		}
+	}
+	if snap.Outcome == run.OutcomeFailed {
+		v.FailedGate = lastFailedGate(events)
+	}
+	return v
+}
+
+// lastFailedGate finds the last gate{passed:false} before the failed transition.
+func lastFailedGate(events []run.Event) *run.GateData {
+	var last *run.GateData
+	for _, ev := range events {
+		if ev.Type != run.TypeGate {
+			continue
+		}
+		var gd run.GateData
+		if json.Unmarshal(ev.Data, &gd) == nil && !gd.Passed {
+			g := gd
+			last = &g
+		}
+	}
+	return last
+}
+
+// ---------------------------------------------------------------- Advance
+
+// Advance runs one step (spec 2 §5.4).
+func (m *Machine) Advance(ctx context.Context) (AdvanceResult, error) {
+	sess, err := m.load(ctx, false)
+	if err != nil {
+		return AdvanceResult{}, err
+	}
+	defer sess.unlock()
+	defer func() { m.view = sess.viewOf() }()
+	if m.torn {
+		return AdvanceResult{}, errs.E(run.CodeAuditTorn, "audit.jsonl has a torn tail; open with --repair", "run", m.runID)
+	}
+	return sess.advance()
+}
+
+func (s *session) advance() (AdvanceResult, error) {
+	snap := s.st.Snapshot
+	w := s.w
+	// 2. terminal
+	if snap.Outcome != "" {
+		if snap.Outcome == run.OutcomeOverflow && w.OnOverflow != "" && !snap.OverflowHandled {
+			return s.finish(run.TransitionData{To: snap.State, Outcome: snap.Outcome})
+		}
+		if err := s.terminal(); err != nil {
+			return AdvanceResult{}, err
+		}
+		return AdvanceResult{}, errs.E(CodeRunTerminal, "run is terminal", "outcome", string(snap.Outcome))
+	}
+	// 4. tree
+	head, err := s.git.Head(s.ctx)
+	if err != nil {
+		return AdvanceResult{}, err
+	}
+	_, porcelain, err := s.git.Status(s.ctx)
+	if err != nil {
+		return AdvanceResult{}, err
+	}
+	wt, err := s.git.WorkTree(s.ctx)
+	if err != nil {
+		return AdvanceResult{}, err
+	}
+	h := gate.TreeHash(head, wt)
+	node := w.NodeFor(snap.State)
+	status, truncated := run.CapDetail(porcelain)
+	tree := run.TreeData{Head: head, TreeHash: h, Status: status, StatusTruncated: truncated}
+	switch {
+	case snap.TreeHash == "":
+		if err := s.append(run.TypeTree, tree, ""); err != nil {
+			return AdvanceResult{}, err
+		}
+	case h != snap.TreeHash && (node == nil || node.Kind != "agent-edit"):
+		if snap.RepoMode == "enforcing" {
+			ge := pseudoGate(GateRepoMode, CodeUnsanctionedEdit, "working tree changed outside an agent-edit node:\n"+porcelain)
+			if err := s.gateEvent(GateRepoMode, ge); err != nil {
+				return AdvanceResult{}, err
+			}
+			return s.fail(ge, head)
+		}
+		if err := s.warn(WarnUnsanctionedEdit, porcelain); err != nil {
+			return AdvanceResult{}, err
+		}
+		if err := s.append(run.TypeTree, tree, ""); err != nil {
+			return AdvanceResult{}, err
+		}
+	case h != snap.TreeHash:
+		if err := s.append(run.TypeTree, tree, ""); err != nil {
+			return AdvanceResult{}, err
+		}
+	}
+	// 5. node
+	if node != nil {
+		res, done, err := s.runNode(node, head)
+		if done || err != nil {
+			return res, err
+		}
+	}
+	// 6. transitions
+	return s.transitions(head)
+}
+
+// runNode executes or requests the current node's work and applies its delta.
+// done reports that advance must return res.
+func (s *session) runNode(node *workflow.Node, head string) (AdvanceResult, bool, error) {
+	snap := s.st.Snapshot
+	kind, _ := s.m.deps.Kinds.Kind(node.Kind)
+	k := run.Key(node.Name, snap.Iteration)
+	if _, has := snap.NodeOutputs[k]; !has {
+		text, truncated, err := s.git.Diff(s.ctx, snap.BaseSHA, head, MaxDiffBytes)
+		if err != nil {
+			return AdvanceResult{}, true, err
+		}
+		diff := Diff{Text: text, Truncated: truncated}
+		if node.Exec == "fork" {
+			ex, _ := s.m.deps.Kinds.Executor(node.Kind)
+			out, err := ex.Execute(s.ctx, ExecInput{Snap: snap, Node: node, Diff: diff, StartIndex: s.st.NextIndex(k), Audit: s.audit})
+			if err != nil {
+				if s.ctx.Err() != nil {
+					return AdvanceResult{}, true, err
+				}
+				if s.auditErr != nil {
+					return AdvanceResult{}, true, s.auditErr
+				}
+				ge := pseudoGate(GateExecutor, CodeExecutorFailed, err.Error())
+				if gerr := s.gateEvent(GateExecutor, ge); gerr != nil {
+					return AdvanceResult{}, true, gerr
+				}
+				res, ferr := s.fail(ge, head)
+				return res, true, ferr
+			}
+			if err := s.append(run.TypeNodeOutput, run.NodeOutputData{Output: out}, node.Name); err != nil {
+				return AdvanceResult{}, true, err
+			}
+			snap = s.st.Snapshot
+		} else {
+			ins, err := kind.Instructions(snap, node, diff, s.m.deps.Nonce())
+			if err != nil {
+				return AdvanceResult{}, true, errs.Wrap(errs.E(CodeInstructionsFailed, err.Error(), "node", node.Name), err)
+			}
+			if !s.hasNeedsInput(node.Name, snap.Iteration) {
+				if err := s.append(run.TypeNeedsInput, run.EmptyData{}, node.Name); err != nil {
+					return AdvanceResult{}, true, err
+				}
+			}
+			ni := &NeedsInput{Node: node.Name, Kind: node.Kind, Exec: node.Exec, Model: node.Model, Effort: node.Effort, Instructions: ins,
+				Record: fmt.Sprintf("metareview fsm record node-output --run %s --node %s --data <file>", s.m.runID, node.Name)}
+			return AdvanceResult{Status: StatusNeedsInput, From: snap.State, NeedsInput: ni, Warnings: s.warns, Untrusted: untrusted(nil, s.warns, ""), ExitCode: 3, RunID: s.m.runID}, true, nil
+		}
+	}
+	if !snap.Applied[k] {
+		out, err := kind.Decode(snap.NodeOutputs[k])
+		var delta run.Delta
+		if err == nil {
+			delta, err = kind.Reduce(snap, out)
+		}
+		if err == nil {
+			err = s.append(run.TypeDeltaApplied, run.DeltaAppliedData{Delta: delta, OutputHash: run.OutputHash(snap.NodeOutputs[k])}, node.Name)
+			if err != nil && !isStoreCode(err, run.CodeAppendRejected) {
+				return AdvanceResult{}, true, err
+			}
+		}
+		if err != nil {
+			ge := pseudoGate(GateNodeOutput, CodeNodeOutputInvalid, err.Error())
+			if gerr := s.gateEvent(GateNodeOutput, ge); gerr != nil {
+				return AdvanceResult{}, true, gerr
+			}
+			res, ferr := s.fail(ge, head)
+			return res, true, ferr
+		}
+	}
+	return AdvanceResult{}, false, nil
+}
+
+func (s *session) hasNeedsInput(node string, iter int) bool {
+	return hasNeedsInput(s.log.Events, node, iter)
+}
+
+func hasNeedsInput(events []run.Event, node string, iter int) bool {
+	for _, ev := range events {
+		if ev.Type == run.TypeNeedsInput && ev.Node == node && ev.Iter == iter {
+			return true
+		}
+	}
+	return false
+}
+
+// transitions evaluates gates (spec 2 §5.4 step 6) and finishes.
+func (s *session) transitions(head string) (AdvanceResult, error) {
+	snap := s.st.Snapshot
+	w := s.w
+	var chosen *workflow.Transition
+	var failures []*run.GateError
+	eval := func(t workflow.Transition) (bool, error) {
+		g, _ := gate.Builtin(t.Gate)
+		gerr := g(s.ctx, s.st.Snapshot, s.git)
+		if err := s.gateEvent(t.Gate, gerr); err != nil {
+			return false, err
+		}
+		if gerr == nil {
+			tt := t
+			chosen = &tt
+			return true, nil
+		}
+		failures = append(failures, gerr)
+		return false, nil
+	}
+	tt := w.TerminalFor(snap.State)
+	if tt != nil {
+		if _, err := eval(*tt); err != nil {
+			return AdvanceResult{}, err
+		}
+		if chosen == nil {
+			r, err := s.pred.Evaluate(s.ctx, s.st.Snapshot)
+			if err != nil && s.auditErr != nil {
+				return AdvanceResult{}, s.auditErr // the atom's cmd_call could not be stored: abort, not a gate failure
+			}
+			if err != nil || (r.Class == run.OutcomeFixed && r.Atom != "all_fixed") {
+				detail := "convergence evaluation failed"
+				reason := "error"
+				if err != nil {
+					detail = err.Error()
+				} else {
+					reason = "fixed_class"
+					detail = "a convergence atom classed a stop as fixed"
+				}
+				ge := pseudoGate(GateConverge, CodeConvergeFailed, detail)
+				ge.Detail = reason + ": " + ge.Detail
+				if gerr := s.gateEvent(GateConverge, ge); gerr != nil {
+					return AdvanceResult{}, gerr
+				}
+				return s.fail(ge, head)
+			}
+			reason, _ := run.CapText(r.Reason, run.MaxText)
+			if err := s.append(run.TypeConverge, run.ConvergeData{Atom: r.Atom, Class: r.Class, Stop: r.Stop, Reason: reason}, ""); err != nil {
+				return AdvanceResult{}, err
+			}
+			if r.Stop {
+				chosen = &workflow.Transition{From: snap.State, To: tt.To, Gate: r.Atom, Outcome: r.Class}
+				return s.finish(s.transitionData(*chosen, head), r.Atom+": "+reason)
+			}
+		}
+		if chosen == nil {
+			for _, t := range w.Outgoing(snap.State) {
+				if t == *tt {
+					continue
+				}
+				if ok, err := eval(t); err != nil || ok {
+					if err != nil {
+						return AdvanceResult{}, err
+					}
+					break
+				}
+			}
+		}
+	} else {
+		for _, t := range w.Outgoing(snap.State) {
+			if ok, err := eval(t); err != nil || ok {
+				if err != nil {
+					return AdvanceResult{}, err
+				}
+				break
+			}
+		}
+	}
+	if chosen == nil {
+		return s.fail(failures[0], head)
+	}
+	return s.finish(s.transitionData(*chosen, head))
+}
+
+func (s *session) transitionData(t workflow.Transition, head string) run.TransitionData {
+	td := run.TransitionData{From: t.From, To: t.To, Gate: t.Gate, Outcome: t.Outcome, Loop: t.Loop, Head: head}
+	if n := s.w.NodeFor(t.To); n != nil {
+		td.ToKind = run.Kind(n.Kind)
+	}
+	return td
+}
+
+// fail appends the failed transition and finishes with GATE_FAILED.
+func (s *session) fail(first *run.GateError, head string) (AdvanceResult, error) {
+	snap := s.st.Snapshot
+	td := run.TransitionData{From: snap.State, To: workflow.FailedState, Gate: first.Gate, Outcome: run.OutcomeFailed, Head: head}
+	if err := s.append(run.TypeTransition, td, ""); err != nil {
+		return AdvanceResult{}, err
+	}
+	if err := s.terminal(); err != nil {
+		return AdvanceResult{}, err
+	}
+	gd := &run.GateData{Name: first.Gate, Passed: false, Error: first}
+	return AdvanceResult{Status: StatusGateFailed, From: snap.State, To: workflow.FailedState, Gate: gd, Outcome: run.OutcomeFailed,
+		Warnings: s.warns, Untrusted: untrusted(gd, s.warns, ""), ExitCode: 1, RunID: s.m.runID}, nil
+}
+
+// finish appends the transition (unless resuming), runs the overflow handler,
+// calls Terminal, and maps the outcome (spec 2 §5.7).
+func (s *session) finish(td run.TransitionData, stopReason ...string) (AdvanceResult, error) {
+	snap := s.st.Snapshot
+	resuming := snap.Outcome != ""
+	if !resuming {
+		ev := s.stamp(run.TypeTransition, td, "")
+		if td.Loop {
+			ev.Iter = snap.Iteration + 1
+		}
+		if err := s.appendEvent(ev); err != nil {
+			return AdvanceResult{}, err
+		}
+	}
+	if td.Outcome == run.OutcomeOverflow && s.w.OnOverflow != "" && !s.st.Snapshot.OverflowHandled {
+		if err := s.overflowHandler(); err != nil {
+			return AdvanceResult{}, err
+		}
+	}
+	res := AdvanceResult{Status: StatusAdvanced, From: td.From, To: td.To, Outcome: td.Outcome, Warnings: s.warns, RunID: s.m.runID}
+	if len(stopReason) > 0 {
+		res.StopReason = stopReason[0]
+	}
+	if resuming {
+		res.From, res.StopReason = snap.State, snap.StopReason
+	}
+	switch td.Outcome {
+	case "":
+	case run.OutcomeFixed, run.OutcomeClean:
+		res.Status = StatusDone
+	case run.OutcomeReviewed:
+		res.Status, res.ExitCode = StatusDone, 1
+	default:
+		res.Status, res.ExitCode = StatusStopped, 1
+	}
+	if td.Outcome != "" {
+		if err := s.terminal(); err != nil {
+			return AdvanceResult{}, err
+		}
+	}
+	res.Untrusted = untrusted(nil, s.warns, res.StopReason)
+	return res, nil
+}
+
+func (s *session) overflowHandler() error {
+	snap := s.st.Snapshot
+	name := s.w.OnOverflow
+	payload := converge.Payload(snap)
+	sum := sha256.Sum256(payload)
+	res, err := s.runner.Run(s.ctx, name, payload)
+	if s.auditErr != nil {
+		return s.auditErr // the runner's own cmd_call could not be stored: abort, the handler is retried on resume
+	}
+	var argv []string
+	for _, c := range snap.AllowedCmds {
+		if c.Name == name {
+			argv = c.Argv
+		}
+	}
+	stdout, so := run.CapText(string(res.Stdout), run.MaxDetail)
+	stderr, se := run.CapText(string(res.Stderr), run.MaxStderr)
+	data := run.OverflowHandlerData{Name: name, Argv: argv, InputHash: hex.EncodeToString(sum[:]), Stdout: stdout, Stderr: stderr,
+		StdoutTruncated: so, StderrTruncated: se, ExitCode: res.ExitCode, DurationMS: res.Duration.Milliseconds()}
+	if err != nil {
+		data.ExitCode = -1
+		data.Error = errs.Code(err)
+		if data.Error == "" {
+			data.Error = "ERR_CMD_FAILED"
+		}
+	}
+	if aerr := s.append(run.TypeOverflowHandler, data, ""); aerr != nil {
+		return aerr
+	}
+	if err != nil || res.ExitCode != 0 {
+		detail := fmt.Sprintf("on_overflow %s exited %d", name, res.ExitCode)
+		if err != nil {
+			detail = "on_overflow " + name + ": " + err.Error()
+		}
+		return s.warn(WarnOverflowHandlerFailed, detail)
+	}
+	return nil
+}
+
+func (s *session) terminal() error {
+	if s.m.deps.Terminal == nil {
+		return nil
+	}
+	return s.m.deps.Terminal(s.ctx, s.viewOf())
+}
+
+func untrusted(gd *run.GateData, warns []string, stop string) []string {
+	var out []string
+	if gd != nil && gd.Error != nil && gd.Error.Detail != "" {
+		out = append(out, "gate.detail")
+	}
+	if len(warns) > 0 {
+		out = append(out, "warnings")
+	}
+	if stop != "" {
+		out = append(out, "stop_reason")
+	}
+	return out
+}
+
+// ---------------------------------------------------------------- Record
+
+// Record appends host-supplied events (spec 2 §5.5).
+func (m *Machine) Record(ctx context.Context, o RecordOptions) (RecordResult, error) {
+	sess, err := m.load(ctx, false)
+	if err != nil {
+		return RecordResult{}, err
+	}
+	defer sess.unlock()
+	defer func() { m.view = sess.viewOf() }()
+	if m.torn {
+		return RecordResult{}, errs.E(run.CodeAuditTorn, "audit.jsonl has a torn tail; open with --repair", "run", m.runID)
+	}
+	snap := sess.st.Snapshot
+	switch o.Kind {
+	case RecordNodeOutput:
+		if snap.Outcome != "" {
+			return RecordResult{}, errs.E(CodeRunTerminal, "run is terminal", "outcome", string(snap.Outcome))
+		}
+		node := sess.w.NodeFor(snap.State)
+		if node == nil || node.Name != o.Node {
+			return RecordResult{}, errs.E(CodeNodeMismatch, "the current state's node is not "+o.Node, "state", string(snap.State), "node", o.Node)
+		}
+		if node.Exec == "fork" {
+			return RecordResult{}, errs.E(CodeNodeNotHost, "node "+node.Name+" is executed by the binary, not the host", "node", node.Name)
+		}
+		k := run.Key(node.Name, snap.Iteration)
+		if snap.Applied[k] {
+			return RecordResult{}, errs.E(CodeNodeOutputApplied, "output for "+k+" is already applied", "key", k)
+		}
+		if _, has := snap.NodeOutputs[k]; has && !o.Replace {
+			return RecordResult{}, errs.E(CodeNodeOutputExists, "output for "+k+" exists (use --replace)", "key", k)
+		}
+		kind, _ := m.deps.Kinds.Kind(node.Kind)
+		if _, err := kind.Decode(o.Data); err != nil {
+			return RecordResult{}, errs.Wrap(errs.E(CodeNodeOutputInvalid, err.Error(), "key", k), err)
+		}
+		canon, err := run.Canonical(o.Data)
+		if err != nil {
+			return RecordResult{}, errs.E(CodeNodeOutputInvalid, err.Error(), "key", k)
+		}
+		if err := sess.append(run.TypeNodeOutput, run.NodeOutputData{Output: canon}, node.Name); err != nil {
+			return RecordResult{}, err
+		}
+		return RecordResult{Seq: sess.st.Seq, Type: run.TypeNodeOutput, Key: k}, nil
+	case RecordTokens:
+		var tok run.TokenTotals
+		dec := json.NewDecoder(bytes.NewReader(o.Data))
+		dec.DisallowUnknownFields()
+		if err := dec.Decode(&tok); err != nil || tok.Negative() || tok.TooLarge() {
+			return RecordResult{}, errs.E(CodeRecordTokens, "tokens must be a {input, cache_read, cache_create, output, reasoning} object of non-negative counters")
+		}
+		if err := sess.append(run.TypeTokens, tok, ""); err != nil {
+			return RecordResult{}, err
+		}
+		return RecordResult{Seq: sess.st.Seq, Type: run.TypeTokens}, nil
+	case RecordEvent:
+		switch {
+		case !recordName.MatchString(o.Name):
+			return RecordResult{}, errs.E(CodeRecordName, "record names match ^[a-z][a-z0-9_-]{0,63}$", "reason", "syntax", "name", o.Name)
+		case isEventType(o.Name):
+			return RecordResult{}, errs.E(CodeRecordName, o.Name+" is a run event type", "reason", "event_type", "name", o.Name)
+		case strings.HasPrefix(o.Name, "mrv_"):
+			return RecordResult{}, errs.E(CodeRecordName, "mrv_* names are reserved for the machine", "reason", "reserved", "name", o.Name)
+		}
+		canon, err := run.Canonical(o.Data)
+		if err != nil {
+			return RecordResult{}, errs.E(CodeRecordTooLarge, "record data must be valid JSON", "name", o.Name)
+		}
+		if len(canon) > run.MaxPayload-128 {
+			return RecordResult{}, errs.E(CodeRecordTooLarge, fmt.Sprintf("record data exceeds %d bytes", run.MaxPayload-128), "name", o.Name)
+		}
+		if err := sess.append(run.TypeRecord, run.RecordData{Name: o.Name, Data: canon}, ""); err != nil {
+			return RecordResult{}, err
+		}
+		return RecordResult{Seq: sess.st.Seq, Type: run.TypeRecord, Key: o.Name}, nil
+	}
+	return RecordResult{}, errs.E(CodeRecordName, "record kind must be node-output, tokens, or event", "reason", "kind", "name", o.Kind)
+}
+
+func isEventType(name string) bool {
+	i := sort.SearchStrings(sortedEventTypes, name)
+	return i < len(sortedEventTypes) && sortedEventTypes[i] == name
+}
+
+var sortedEventTypes = func() []string {
+	s := append([]string{}, run.EventTypes...)
+	sort.Strings(s)
+	return s
+}()
diff --git a/internal/fsm/machine/sidecar.go b/internal/fsm/machine/sidecar.go
new file mode 100644
index 0000000..db8c629
--- /dev/null
+++ b/internal/fsm/machine/sidecar.go
@@ -0,0 +1,194 @@
+package machine
+
+import (
+	"errors"
+	"io"
+	"io/fs"
+	"os"
+	"path/filepath"
+	"regexp"
+	"sort"
+	"strings"
+	"sync"
+	"syscall"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+var sidecarName = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)
+
+// ValidSidecarName reports whether name may be stored beside audit.jsonl.
+func ValidSidecarName(name string) bool {
+	return sidecarName.MatchString(name) && !strings.HasPrefix(name, "audit.") && name != "lock"
+}
+
+func sidecarErr(reason, detail string) error {
+	return errs.E(CodeSidecar, detail, "reason", reason)
+}
+
+func checkSidecarArgs(runID, name string) error {
+	if err := run.ValidateRunID(runID); err != nil {
+		return sidecarErr("path", "invalid run id")
+	}
+	if !ValidSidecarName(name) {
+		return sidecarErr("name", "invalid sidecar name "+name)
+	}
+	return nil
+}
+
+// FSSidecar stores sidecars under <root>/.metareview/runs/<id>/. Open is
+// the file seam (nil → os.OpenFile); tests inject failing files.
+type FSSidecar struct {
+	Root string
+	Open func(path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error)
+}
+
+func (f FSSidecar) path(runID, name string) string {
+	return filepath.Join(f.Root, ".metareview", "runs", runID, name)
+}
+
+func (f FSSidecar) open(path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
+	if f.Open != nil {
+		return f.Open(path, flag, perm)
+	}
+	return os.OpenFile(path, flag, perm)
+}
+
+// Write creates the file exclusively (0600, O_NOFOLLOW); the run directory
+// must already exist (run.Create makes it).
+func (f FSSidecar) Write(runID, name string, b []byte) error {
+	if err := checkSidecarArgs(runID, name); err != nil {
+		return err
+	}
+	fh, err := f.open(f.path(runID, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
+	if err != nil {
+		if errors.Is(err, fs.ErrExist) {
+			return sidecarErr("exists", name+" already exists")
+		}
+		return sidecarErr("path", err.Error())
+	}
+	_, werr := fh.Write(b)
+	if err := errors.Join(werr, fh.Close()); err != nil {
+		return sidecarErr("path", err.Error())
+	}
+	return nil
+}
+
+// Read returns at most run.MaxPayload bytes.
+func (f FSSidecar) Read(runID, name string) ([]byte, error) {
+	if err := checkSidecarArgs(runID, name); err != nil {
+		return nil, err
+	}
+	fh, err := f.open(f.path(runID, name), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
+	if err != nil {
+		if errors.Is(err, fs.ErrNotExist) {
+			return nil, sidecarErr("missing", name+" is missing")
+		}
+		return nil, sidecarErr("path", err.Error())
+	}
+	defer fh.Close()
+	b, err := io.ReadAll(io.LimitReader(fh, run.MaxPayload+1))
+	if err != nil {
+		return nil, sidecarErr("path", err.Error())
+	}
+	if len(b) > run.MaxPayload {
+		return nil, sidecarErr("too_large", name+" exceeds MaxPayload")
+	}
+	return b, nil
+}
+
+// List names the sidecars of a run (never audit.* or lock).
+func (f FSSidecar) List(runID string) ([]string, error) {
+	if err := run.ValidateRunID(runID); err != nil {
+		return nil, sidecarErr("path", "invalid run id")
+	}
+	entries, err := os.ReadDir(filepath.Dir(f.path(runID, "x")))
+	if err != nil {
+		return nil, sidecarErr("missing", err.Error())
+	}
+	var out []string
+	for _, e := range entries {
+		if e.Type().IsRegular() && ValidSidecarName(e.Name()) {
+			out = append(out, e.Name())
+		}
+	}
+	sort.Strings(out)
+	return out, nil
+}
+
+// MemSidecar is the in-memory Sidecar for tests.
+type MemSidecar struct {
+	mu    sync.Mutex
+	files map[string][]byte
+}
+
+func (m


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

