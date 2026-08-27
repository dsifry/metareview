# metareview 0.9.0 — spec 2: workflow, gates, convergence, and the machine core

> **Status:** DRAFT r2 (2026-08-27). Second of the five split 0.9.0 artifacts (ownership ledger: run spec §12).
> Builds on `internal/fsm/run` (r4, implemented, 100%). Owns plan r3 §1.1 (layout), §1.3 gate/converge/workflow
> types, §1.5 `Advance`, `Record`, §1.6 outcomes (reproduced in §5.7), §6 shipped workflows (embedded in `workflows/`),
> the canonical `cmds_sha256` preimage, the `PrevUnfixed == nil` rule, `tree` cadence + `TreeHash` preimage +
> `UNSANCTIONED_EDIT`, the repair-`warn` emission, and the `Status` full-coverage input contract for verify.
>
> **r2 changes** (review `mrv-20260827-062655783147000-…fsm-core-a0b8592f`, 8 lenses, all NEEDS_REVISION): single
> top-level `cmds:` declaration referenced by name (§2.2); `failed` reserved (§2.3); `duplicate_transition` keyed on
> `(from, gate)`; loop-safety reasons; `RevParse` + base resolution; workflow sidecar so `Open` never depends on the
> embedded bytes; `snap` rebound after every append; enforcing edits and executor/decode failures leave `gate` evidence;
> `needs_input` appended once per key; `View`/`RecordResult`/outcome table declared; shared `errs.Error`; `Repair`
> API; kind-info supplied to `Parse` (default exec); every test row names its discriminating fixture.
>
> **Scope rule:** what the deterministic core *decides*: how a YAML becomes a `Workflow`, what each gate and atom
> returns, and exactly which `run` events `Init`/`Advance`/`Record` append. Kinds' prompts/executors are spec 4, forks/
> diff/export/runs.jsonl are spec 3, the CLI is spec 5. Every LLM or shell effect is behind an interface declared here.

---

## 1. Packages and dependency direction

```
internal/fsm/errs       Error{Code, Detail string; Fields map[string]string}; Is(err, code); E(code, detail, kv...)
internal/fsm/workflow   YAML → Workflow; validation; var resolution; cmd resolution + cmds_sha256; WorkflowHash
internal/fsm/gate       Git interface (exec-backed via an exec seam + Fake); 7 gates; TreeHash
internal/fsm/converge   AllFixed; atoms; any/all/not; Parse; CmdResult/Runner interface
internal/fsm/machine    Deps; Init/Open/Advance/Record/View; node interfaces (implemented by spec 4's kinds); Sidecar
```
`errs` ← all. `run` ← all. `converge` ← `run` only; `gate` ← `converge` (`all_fixed`/`bugs_remain` call `converge.AllFixed`). `machine` ← `workflow`, `gate`, `converge`. `machine`
imports no kinds/judge/cmdexec package: it consumes §5.1. `workflows` (embed) is used by the CLI (spec 5) to build
`Deps.Workflows`; `machine` does not import it. Only external dependency: `gopkg.in/yaml.v3` (first external module
dependency of the repo; `go.sum` ships — spec 5).

`errs.Error` is the one error type for every `ERR_*` in specs 2–5: `Code` (the `ERR_*`), `Detail` (human text, capped
by the emitter), `Fields` (structured: `reason`, `name`, `path`, `expected`, `got`, …). `errors.As`-compatible; `Error()`
= `code: detail`.

## 2. `workflow`

### 2.1 Types
```go
type VarSpec struct { Default string; Required bool }
type CmdDecl struct { Name string; Argv []string /* resolved argv after Resolve */; Timeout time.Duration /* default 60s */; Env []string /* extra env names passed through; default none */ }
type Node struct { Name string; Kind string; Exec string /* inline|subagent|fork; filled from KindInfo.DefaultExec when omitted */; Model, Effort string; Params map[string]any; Cmd string /* cmd kind: name in Cmds */ }
type Transition struct { From, To run.State; Gate string; Outcome run.Outcome; Loop bool }
type KindInfo struct { DefaultExec string; AllowedExec []string; IsLLM bool }
type Options struct { Kinds map[string]KindInfo /* from Registry.Info(); nil → the five built-ins with spec 4's table */ }
type Workflow struct {
    Name string; Version int; Vars map[string]VarSpec; States []run.State; Initial run.State
    Transitions []Transition          // declaration order
    Nodes map[run.State]*Node         // states without a node have no entry
    Cmds map[string]*CmdDecl          // top-level cmds: declaration; the only place argv is written
    Convergence *yaml.Node            // parsed by converge.Parse (structure validated at Parse via converge.Validate)
    RepoMode string                   // advisory|enforcing (default advisory)
    OnOverflow string                 // cmd name or ""
    Hash string                       // sha256 of the raw file bytes (WorkflowHash)
    Refs map[run.State][]string       // $VARs referenced by each node (model/effort/params/cmd argv) computed at Parse, before resolution
    CmdRefs map[string][]string       // $VARs referenced by each cmd's argv
}
func Parse(raw []byte, opts Options) (*Workflow, error)                                      // structure + static validation; $VAR unresolved
func (w *Workflow) Resolve(vars map[string]string, calibration bool) (*Workflow, map[string]string, error)  // substitutes $VAR everywhere; returns a resolved copy and the effective vars
func (w *Workflow) NodeFor(s run.State) *Node; func (w *Workflow) Outgoing(s run.State) []Transition
func (w *Workflow) IsTerminal(s run.State) bool                                              // no outgoing transitions (failed included)
func (w *Workflow) LoopTransition() *Transition                                              // the one with Loop, or nil
func (w *Workflow) TerminalFor(s run.State) (run.State, run.Outcome)                         // the loop-carrying state's outcome-bearing terminal transition (validated: loop_terminal)
func (w *Workflow) VarsReferencedBy(s run.State) []string                                    // Refs[s] ∪ CmdRefs of the node's cmd; sorted (spec 3's freeze rule; works on resolved copies too)
```

### 2.2 YAML schema (both forms accepted)
```yaml
workflow: sdlc-loop
version: 1
vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5} }
states: [discover, adjudicate, fix, verify, done, failed]      # states[0] is initial; failed is reserved (§2.3)
cmds:                                                          # optional; the only place argv appears
  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }
nodes:
  discover:   { kind: review-lenses, exec: subagent, model: $REVIEWER, lenses: 8 }
  adjudicate: { kind: match-then-adjudicate, exec: fork, model: $JUDGE, effort: $JUDGE_EFFORT }
  custom:     { kind: cmd, cmd: notify }                       # cmd kinds reference a declared name
transitions:                                                   # list form (shipped)
  - { from: discover, to: adjudicate, gate: findings_nonempty }
  - { from: verify, to: done, gate: all_fixed, outcome: fixed }
  - { from: verify, to: discover, gate: bugs_remain, loop: true }
convergence: { any: [ {no_fixation_progress: true}, {max_iterations: 5}, {budget: {tokens: 4000000}}, {cmd: notify} ] }
repo_mode: advisory
on_overflow: notify
```
Mapping form (design spec §5): `transitions: {"discover→adjudicate": {gate: …}, "*→failed": {on: gate_error}}` — parsed via
`yaml.Node` to keep order; the `*→failed` entry is accepted and ignored (the implicit rule). `cmds.<name>.timeout` seconds
(default 60, max 3600); `env` names must match `^[A-Z_][A-Z0-9_]*$` and may not be `PATH`/`HOME` duplicates; a node's
`cmd:` and `on_overflow:` and every `{cmd: <name>}` atom reference `cmds` by name. `workflow:` and `version: 1` required.
Both shipped YAMLs are updated to this schema in M1 (they carry no `cmds`, so the only change is none — they already use
the list form; W1 pins them).

### 2.3 Static validation (Parse) — every failure is `ERR_WORKFLOW_INVALID` with `Fields{reason, at}`:
| reason | rule |
|---|---|
| `missing_name`, `bad_version` | `workflow` non-empty; `version == 1` |
| `unknown_state` | transition `from`/`to`, node keys not in `states` |
| `no_initial` | `states` empty |
| `failed_reserved` | `failed` must be declared in `states`, must have no node and no outgoing transitions (it is the implicit `*→failed` target) |
| `unreachable_state` | a non-initial state other than `failed` with no incoming transition |
| `duplicate_transition` | two transitions with the same `(from, gate)` (review-loop's two `adjudicate→done` rows differ by gate and are legal) |
| `terminal_without_outcome` / `outcome_on_nonterminal` / `bad_outcome` | a transition into a terminal state must carry an `outcome` ∈ `run.Outcomes`; one into a non-terminal must not |
| `loop_count` | more than one `loop: true` |
| `loop_not_cycle` | the loop transition's `to` must reach `from` through non-loop transitions |
| `loop_terminal` | the loop-carrying `from` must have exactly one non-loop transition into a terminal state carrying an outcome (its `TerminalFor`) |
| `missing_convergence` | `loop: true` present but no `convergence:` |
| `cycle_without_loop` | a cycle among non-loop transitions |
| `node_without_kind`, `unknown_kind`, `unknown_exec`, `exec_kind_mismatch` | kind ∈ `Options.Kinds`; exec ∈ `KindInfo.AllowedExec` (omitted → `DefaultExec`) |
| `terminal_with_node` | terminal states carry no node |
| `unknown_gate` | gate ∉ `gate.Names()` |
| `bad_cmd`, `duplicate_cmd`, `unknown_cmd`, `cmd_without_kind` | `argv` non-empty list of strings; names unique + `^[a-z][a-z0-9_-]{0,31}$`; every reference (node `cmd`, `on_overflow`, atoms) names a declared cmd; a `cmd:` on a non-`cmd` kind or a `cmd` kind without `cmd:` |
| `bad_convergence` | `converge.Validate(node, cmdNames)` failed (`Fields.detail` carries converge's reason) |
| `bad_repo_mode` | not `advisory`/`enforcing` |
| `bad_var` | var name not `^[A-Z_][A-Z0-9_]*$`, or `required` with a `default` |

Parse never refuses a `cmd` atom on consent grounds: consent is `Init` step 2 (§5.3), after the list is known.

### 2.4 Resolution
`$NAME` tokens in `model`, `effort`, string params, and every `cmds.*.argv` element are substituted from `vars` (CLI) over
`Default`s. `Required` without a value → `ERR_VAR_UNSET{name}` (spec 5 maps `JUDGE`/`JUDGE_EFFORT` to `ERR_JUDGE_UNSET`,
exit 2). Unknown `$X` → `ERR_VAR_UNKNOWN{name}`. `calibration=true` pins `JUDGE=gpt-5.2`, `JUDGE_EFFORT=medium` and refuses
**caller-supplied** values for either (`ERR_CALIBRATION_PINNED`); `Open` re-resolves from the stored effective vars with
`calibration=false` (the stored vars already carry the pins).

### 2.5 Commands and `cmds_sha256`
`ResolveCmds(w *Workflow /* resolved */, workDir string, lookPath func(string) (string, error), hash func(path string) (string, error)) ([]run.AllowedCmd, string, error)`:
for every `CmdDecl` (sorted by name): `argv[0]` → absolute path via `lookPath` (missing → `ERR_CMD_NOT_FOUND{name}`), written
back into `AllowedCmd.Argv[0]` (**the runner executes `AllowedCmd.Argv` verbatim; the workflow's argv is never re-read**);
every argv element naming an existing regular file (absolute, or relative to `workDir`) is sha256-hashed into `FileHashes`
(absolute path → hash; `FileHashes` is always a non-nil map). `AllowedCmd{Name, Argv, FileHashes, TimeoutMS, Env}`.
**Preimage:** `run.Canonical(json of []AllowedCmd sorted by name)` → `sha256` hex. `VerifyCmds(allowed, workDir, hash)`
recomputes every pinned hash → `ERR_CMD_CHANGED{path, reason: mismatch|missing}`, **and** re-scans argv elements: one that
now resolves to a regular file but has no `FileHashes` entry → `ERR_CMD_CHANGED{path, reason: appeared}` (SEC-25).

## 3. `gate`
```go
type Git interface {
    Head(ctx) (string, error)
    RevParse(ctx, ref string) (string, error)              // git rev-parse --verify --end-of-options <ref>^{commit}; ERR_GIT_REF if ref starts with '-' or has control chars; unknown → ERR_GIT{detail}
    IsAncestor(ctx, a, b string) (bool, error)             // git merge-base --is-ancestor; exit 1 → false,nil; other → ERR_GIT
    CommitCount(ctx, from, to string) (int, error)         // git rev-list --count from..to
    Status(ctx) (clean bool, porcelain string, err error)  // git status --porcelain=v2 --untracked-files=all
    Diff(ctx, from, to string, max int) (diff string, truncated bool, err error)   // git diff from..to; cut at a rune boundary ≤ max bytes
    WorkingDiff(ctx, max int) (string, bool, error)        // git diff HEAD
}
func NewExec(dir string, x Exec) Git      // Exec func(ctx, dir string, args ...string) (stdout, stderr []byte, code int, err error); the real one wraps exec.CommandContext with GIT_TERMINAL_PROMPT=0
type Fake struct{ … }                     // scripted answers + call log
func TreeHash(head, porcelain string) string   // sha256(head + "\n" + porcelain)
```
SHA arguments (`a`, `b`, `from`, `to`) are validated `^[0-9a-f]{7,40}$|^HEAD$` → `ERR_GIT_REF`; all args follow
`--end-of-options`. The `Exec` seam makes every error branch of the real implementation reachable (100% gate).

Gates — `type Gate func(ctx, run.Snapshot, Git) *run.GateError`; `Names()` and `Builtin()`:

| gate | pass | error code |
|---|---|---|
| `findings_nonempty` / `findings_empty` | `len(Findings) > 0` / `== 0` | `ERR_NO_FINDINGS` / `ERR_FINDINGS_PRESENT` |
| `confirmed_nonempty` / `confirmed_empty` | `len(Confirmed) > 0` / `== 0` | `ERR_NO_CONFIRMED` / `ERR_CONFIRMED_PRESENT` |
| `commit_exists` | `FixEntryHead == ""` → `ERR_GATE_INAPPLICABLE`; else `CommitCount(FixEntryHead, HEAD) > 0 && clean` | `ERR_NO_COMMIT`, `Detail` = porcelain + `WorkingDiff(64 KB)` via `run.CapDetail` |
| `all_fixed` / `bugs_remain` | `converge.AllFixed(snap)` / `!AllFixed` | `ERR_BUGS_REMAIN` / `ERR_ALL_FIXED` |

Git failures inside a gate return `ERR_GIT` as the gate error (never a Go error): the audit shows them.

## 4. `converge`
```go
func AllFixed(s run.Snapshot) bool          // len(AllFound) > 0 && Unfixed == 0   (nothing found is NOT fixed; review-loop uses findings_empty)
type CmdResult struct { Stdout, Stderr []byte; ExitCode int; Duration time.Duration }
type Runner interface { Run(ctx, name string, stdin []byte) (CmdResult, error) }   // spec 4's cmdexec.Guarded: name-only, argv pinned, guarded + audited
type Predicate interface { Name() string; Class() run.Outcome; Evaluate(ctx, run.Snapshot) (stop bool, reason string, err error) }
func Validate(node *yaml.Node, cmdNames []string) error       // structural; used by workflow.Parse (bad_convergence)
func Parse(node *yaml.Node, runner Runner) (Predicate, error) // same validation + bind
```
Atoms: `all_fixed` (class `fixed`; `Evaluate` returns `AllFixed(snap)` — kept for user workflows; `Advance` decides `fixed`
via the gate first, §5.4 step 6), `no_fixation_progress` (class `stalled`: `PrevUnfixed != nil && Unfixed >= *PrevUnfixed`;
**nil ⇒ false**, so the first boundary never stalls), `max_iterations: N` (class `overflow`: stop iff `Iteration+1 >= N`,
evaluated at the loop boundary before the loop transition; `Iteration` is 0-based, so `N: 5` runs iterations 0–4 and stops
at `Iteration == 4`), `budget: {tokens: N}` (class `overflow`: `Tokens.Total() >= N`), `{cmd: <name>}` (class `custom`:
`Runner.Run(name, snapshot JSON per spec 4 §4.4's redacted payload)`; stdout must decode (`DisallowUnknownFields`) to
`{"stop": bool, "reason": string}` else `ERR_CMD_OUTPUT_INVALID`; non-zero exit → `ERR_CMD_FAILED`; the Runner audits the
`cmd_call`). Compose: `any: [...]`, `all: [...]`, `not: <atom>`. `any` stops with the first firing atom's name/class; `all`
stops only if all fire (name = names joined by `+`, class = the first's); `not` inverts stop (class = inner's). Errors from
any atom abort evaluation (returned to `Advance` → §5.4 step 6a).

## 5. `machine`

### 5.1 Consumed interfaces (implemented by spec 4)
```go
type Instructions struct { Text string /* fenced per spec 4 §3.2: untrusted values appear only inside nonce-fenced JSON blocks */; Input map[string]any; Untrusted []string; OutputSchema json.RawMessage }
type NodeKind interface {
    Name() string; Info() workflow.KindInfo
    Instructions(snap run.Snapshot, node *workflow.Node, diff Diff, nonce string) (Instructions, error)
    Decode(raw json.RawMessage) (any, error)                       // typed, DisallowUnknownFields, caps (spec 4 §4.1)
    Reduce(snap run.Snapshot, out any) (run.Delta, error)
}
type Diff struct { Text string; Truncated bool }                   // Git.Diff(BaseSHA, HEAD, MaxDiffBytes=1<<20)
type Executor interface { Execute(ctx, snap run.Snapshot, node *workflow.Node, diff Diff, audit func(run.Event) error) (any, error) }
type Registry interface { Kind(name string) (NodeKind, bool); Executor(name string) (Executor, bool); Info() map[string]workflow.KindInfo }
type Clock func() run.Time
type Sidecar interface { Write(runID, name string, b []byte) error; Read(runID, name string) ([]byte, error) }   // FS: <root>/.metareview/runs/<id>/<name> (0600, O_NOFOLLOW); Mem for tests
```
`audit` appends immediately (durable even if `Execute` later fails); it returns the store's error so the executor stops
on `ERR_AUDIT_FULL`. The executor assigns `LLMCallData.Index`/`CmdCallData` order from 0 per `Execute`; the machine stamps
everything else (§5.4 tail). `Execute` is never retried in-run: failure → step 8 with pseudo-gate `executor`.

### 5.2 Deps and API
```go
type Deps struct { Store run.RunStore; Sidecar Sidecar; Kinds Registry; Git func(workDir string) gate.Git; Runner func(allowed []run.AllowedCmd, workDir, runID string, audit func(run.Event) error) converge.Runner; Clock Clock; LookPath func(string) (string, error); FileHash func(string) (string, error); Workflows func(name string) ([]byte, error); ReadFile func(string) ([]byte, error); Nonce func() string; MockLoad func(dir string) (mockHash string, err error) }
type InitOptions struct { Workflow string /* name, or path containing '/' or ending .yaml */; RunID string /* "" → run.RunID(name, Clock()) */; Vars map[string]string; Base string /* default HEAD */; RepoMode string /* "" → workflow's */; AllowCustomCmds string; Calibration bool; MockDir string; GoldensPath string; WorkDir, RepoRoot string }
type OpenOptions struct { Repair bool }
func Init(ctx, Deps, InitOptions) (*Machine, error)
func Open(ctx, Deps, runID string, OpenOptions) (*Machine, error)   // folds; verifies sidecar hash, cmds, mock; Repair → RepairTail + warn
func (m *Machine) Advance(ctx) (AdvanceResult, error)
func (m *Machine) Record(ctx, RecordOptions) (RecordResult, error)  // RecordOptions{Kind: node-output|tokens|event; Node string; Data json.RawMessage; Replace bool; Name string}
func (m *Machine) View() View
type AdvanceResult struct { Status string /* ADVANCED|NEEDS_INPUT|DONE|STOPPED|GATE_FAILED */; From, To run.State; Gate *run.GateData /* GATE_FAILED: the FIRST failing gate */; Outcome run.Outcome; StopReason string; NeedsInput *NeedsInput; Warnings []string; ExitCode int; RunID string }
type NeedsInput struct { Node string; Kind, Exec, Model, Effort string; Instructions Instructions; Record string /* "metareview fsm record node-output --run <id> --node <n> --data <file>" */ }
type RecordResult struct { Seq int64; Type run.EventType; Key string }
type View struct { RunID, Workflow string; Snapshot run.Snapshot; Node *NodeView /* nil when the state has no node */; NextAction string /* advance|record|none */; Torn bool }
type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
```
`NextAction`: `none` when terminal; `record` when the current node is host-executed and `NodeOutputs[key]` is absent;
else `advance`.

### 5.3 `Init` — the events it appends
1. Load YAML: name → `Deps.Workflows` (`ERR_WORKFLOW_NOT_FOUND`); path → `Deps.ReadFile`. `workflow.Parse(raw, Options{Kinds: Kinds.Info()})`; `Resolve(vars, calibration)`.
2. `ResolveCmds`; if `len(Cmds) > 0 && AllowCustomCmds != cmds_sha256` → `ERR_CMDS_NOT_ALLOWED{sha}` with `Detail` = the printed list (name, argv, file hashes, timeout, env). No cmds → no flag needed.
3. Git in `WorkDir`: `Head` → `Head`; `RevParse(Base or "HEAD")` → `BaseSHA` (full sha; `ERR_GIT_REF`/`ERR_GIT`); `Status` → `TreeHash`.
4. Goldens: `ReadFile(GoldensPath)` → JSON array of `run.Golden`, ≤ `MaxGoldens`; failure → `ERR_GOLDENS_INVALID{path}`.
5. Mock: `MockDir != ""` → `MockLoad(dir)` → `Mock = dir + "#" + hash[:16]` (content-pinned; `ERR_MOCK_INVALID` on load failure).
6. `runID` ← `RunID` or `run.RunID(w.Name, Clock().Time)`; `Sidecar.Write(runID, "workflow.yaml", raw)`; `run.Create(runID, init)` with `InitData{RunID, CreatedAt: Clock(), Workflow: w.Name, WorkflowHash: w.Hash, Vars: effective, Calibration, Mock, RepoMode, AllowedCmds, CmdsSHA256, RepoRoot, WorkDir, BaseSHA, Head, InitialState: w.Initial, InitialKind, Goldens, Lineage: []}`; then under the lock append `tree{Head, TreeHash, Status: porcelain capped}`.
7. Returns the machine; `View().NextAction == "advance"`.
Every event `Init` appends is stamped per §5.4's tail (`Mock` true on mock runs, `State = Initial`, `Iter = 0`).

### 5.4 `Advance`
```
1  Lock; log ← EventsWithLines; log.Torn → ERR_AUDIT_TORN{seq, bytes} (Open(Repair) is the only repair path: it calls
   RepairTail then appends warn{AUDIT_TORN_LINE_DROPPED, Detail: "<n> bytes dropped after seq <s> from audit.jsonl"});
   st ← FoldFull(log.Events); snap ← st.Snapshot. Every "append" below is Store.Append(runID, st, ev) and REBINDS
   st, snap to the returned FoldState (gates evaluate the post-delta snapshot). Any store error aborts Advance with that
   error (ERR_AUDIT_FULL included; nothing is rolled back — appends are durable).
2  snap.Outcome != "" → ERR_RUN_TERMINAL
3  integrity: sha256(Sidecar.Read("workflow.yaml")) == snap.WorkflowHash else ERR_WORKFLOW_CHANGED; VerifyCmds → ERR_CMD_CHANGED;
   MockLoad(dir) hash vs snap.Mock else ERR_MOCK_MISMATCH. Reparse + re-resolve from the sidecar and stored vars.
4  head, porcelain ← Git; h ← TreeHash; node ← w.NodeFor(state)
   if snap.TreeHash != "" && h != snap.TreeHash && (node == nil || node.Kind != agent-edit):
       advisory  → append warn{UNSANCTIONED_EDIT, Detail: porcelain via CapText(MaxText)}
       enforcing → append tree{head, h, porcelain}; append gate{Name: "repo_mode", Passed: false, Error: ERR_UNSANCTIONED_EDIT{Detail: porcelain capped}}; → step 8
   if h != snap.TreeHash: append tree{head, h, porcelain capped}         (only on change; Init wrote the first one)
5  if node != nil:
       k ← Key(node, iter); diff ← Git.Diff(BaseSHA, head, MaxDiffBytes)
       if NodeOutputs[k] absent:
           exec == fork: out, err ← Executor.Execute(snap, node, diff, audit); err → append gate{Name: "executor", Passed: false,
                         Error: ERR_EXECUTOR_FAILED{Detail: err}} → step 8; else append node_output{Canonical(out)}
           else: if the last event with Key k is not needs_input: append needs_input{Node}   (once per key)
                 return NEEDS_INPUT (exit 3) with Instructions(snap, node, diff, Nonce()); nothing else appended
       if !Applied[k]: out ← kind.Decode(output); delta ← kind.Reduce(snap, out); on error → append gate{Name: "node_output",
                       Passed: false, Error: ERR_NODE_OUTPUT_INVALID{Detail}} → step 8; else append delta_applied{delta, OutputHash(output)}
6  chosen, first ← nil, nil
   for t in w.Outgoing(state):
       if t.Loop:   (loop boundary)
           tt, outcome ← w.TerminalFor(state)
           if AllFixed(snap): chosen ← the transition into tt (it carries gate all_fixed or findings_empty; the gate is
                              evaluated and appended like any other and must pass) ; break
           stop, reason, err ← Convergence.Evaluate(snap); err → append converge{Atom: name, Class, Stop: false, Reason: err}
                              → treat as first ??= gate error ERR_CONVERGE_FAILED and continue
           append converge{Atom, Class, Stop, Reason}
           if stop: chosen ← synthetic {From: state, To: tt, Gate: atom.Name(), Outcome: atom.Class()}; break
           err ← gate(t.Gate)(snap); append gate{…}; pass → chosen ← t; break on pass; else first ??= err
       else:        err ← gate(t.Gate)(snap); append gate{t.Gate, passed, err}; pass → chosen ← t; break; else first ??= err
7  (each gate evaluation is appended at the moment it is evaluated, in order)
8  chosen == nil: append transition{From: state, To: "failed", Outcome: failed, Head: head}; return GATE_FAILED with
   Gate = the FIRST failing GateData (repo_mode/executor/node_output pseudo-gates included), exit 1
9  else append transition{From, To, Gate, Outcome: chosen.Outcome, Loop, ToKind: kind of To's node, Head: head}
   if Outcome == overflow && OnOverflow != "" && !OverflowHandled: Runner.Run(OnOverflow, payload) → append overflow_handler{…}
   (Runner audits cmd_call; failure → also warn{OVERFLOW_HANDLER_FAILED}); return per §5.7
```
**Stamps** (every event appended by `Init`, `Advance`, `Record`, and through `audit`): `At ← Clock()`, `State ← current
state` (`From` for transitions), `Iter ← current iteration` (`N+1` on the loop transition), `Mock ← snap.Mock != ""`,
`Node ← node.Name` for node-scoped events (`needs_input`, `node_output`, `delta_applied`, `llm_call`, `cmd_call`). Run's
`Apply` re-checks all of them.

### 5.5 `Record` (takes the run lock)
- `node-output`: run not terminal (`ERR_RUN_TERMINAL`); the state has a node and `Node == node.Name` (`ERR_NODE_MISMATCH`);
  node exec is `inline|subagent` (`ERR_NODE_NOT_HOST`); `Applied[k]` false (`ERR_NODE_OUTPUT_APPLIED`); `NodeOutputs[k]`
  absent unless `Replace` (`ERR_NODE_OUTPUT_EXISTS`); `kind.Decode(data)` must succeed (`ERR_NODE_OUTPUT_INVALID`, nothing
  appended); append `node_output{Canonical(data)}`. The delta is applied on the next `Advance`.
- `tokens`: decode `run.TokenTotals` (`DisallowUnknownFields`) → append `tokens`.
- `event`: `Name` must match `^[a-z][a-z0-9_-]{0,63}$`, must not be a run event type, and must not start with `mrv_`
  (reserved for the machine) → `ERR_RECORD_NAME`; append `record{name, data}` (`data` ≤ `MaxPayload`).

### 5.6 `Status` input contract (for spec 4's `still-present`)
A verify output must carry a status for **every** bug in `AllFound` (the machine passes `AllFound` in `Instructions.Input`
and in the executor's snapshot); `Reduce` returns exactly that set or `run` rejects with `status_incomplete`.

### 5.7 Outcomes → status → exit code (plan §1.6, owned here)
| outcome | `AdvanceResult.Status` | exit | notes |
|---|---|---|---|
| `""` (non-terminal transition) | `ADVANCED` | 0 | |
| host node awaiting output | `NEEDS_INPUT` | 3 | |
| `fixed`, `clean` | `DONE` | 0 | |
| `reviewed` | `DONE` | 1 | findings remain by design |
| `stalled`, `overflow`, `custom` | `STOPPED` | 1 | `StopReason` = converge reason |
| `failed` | `GATE_FAILED` | 1 | `Gate` set; spec 5 adds `resume_hint` |

## 6. Errors (all `errs.Error`)
`ERR_WORKFLOW_INVALID{reason, at}`, `ERR_WORKFLOW_NOT_FOUND`, `ERR_VAR_UNSET{name}`, `ERR_VAR_UNKNOWN{name}`,
`ERR_CALIBRATION_PINNED{name}`, `ERR_CMDS_NOT_ALLOWED{sha}`, `ERR_CMD_NOT_FOUND{name}`, `ERR_CMD_CHANGED{path, reason}`,
`ERR_GIT{detail}`, `ERR_GIT_REF{ref}`, `ERR_GOLDENS_INVALID{path}`, `ERR_MOCK_INVALID{dir}`, `ERR_RUN_TERMINAL`,
`ERR_WORKFLOW_CHANGED{expected, got}`, `ERR_MOCK_MISMATCH`, `ERR_NODE_MISMATCH`, `ERR_NODE_NOT_HOST`,
`ERR_NODE_OUTPUT_APPLIED`, `ERR_NODE_OUTPUT_EXISTS`, `ERR_NODE_OUTPUT_INVALID`, `ERR_RECORD_NAME`, `ERR_UNSANCTIONED_EDIT`,
`ERR_EXECUTOR_FAILED`, `ERR_CONVERGE_FAILED`, `ERR_GATE_INAPPLICABLE`, gate codes (§3), `ERR_CMD_OUTPUT_INVALID`,
`ERR_CMD_FAILED`, `ERR_CMD_TIMEOUT` (surfaced from the Runner), and `run`'s store codes pass through unchanged.
Warn codes: `UNSANCTIONED_EDIT`, `AUDIT_TORN_LINE_DROPPED`, `OVERFLOW_HANDLER_FAILED`.

## 7. Tests (each package 100% statements; TDD; every row names its discriminating fixture)

| pkg | rows |
|---|---|
| errs | E1 `Error()` format, `Fields` copy, `Is`, `errors.As` through wrapping |
| workflow | W1 parse both shipped YAMLs + the §2.2 example + mapping form (order preserved, `*→failed` ignored); assert `Refs`/`CmdRefs` and `Hash` literals. W2 one fixture per reason in §2.3 (each fixture differs from a valid base by one edit; assert `reason` and `at`). W3 `Resolve`: defaults, required, unknown `$X` inside argv, calibration pin vs caller value, `Open`-style re-resolve of pinned vars succeeds. W4 `ResolveCmds` with fake lookPath/hash: `argv[0]` rewritten absolute; file-hash closure over `["bash","./s.sh"]` and an absolute path; non-nil empty `FileHashes`; preimage pinned to a literal sha of a committed fixture; negative pin (edit one byte → different sha); `ERR_CMD_NOT_FOUND`; `VerifyCmds` mismatch / missing / **appeared**. W5 `VarsReferencedBy` (node + its cmd, sorted, on a resolved copy), `Outgoing` order, `IsTerminal` incl. `failed`, `LoopTransition`, `TerminalFor` |
| gate | G1 each gate pass/fail with exact code; `commit_exists`: no fix entry → `ERR_GATE_INAPPLICABLE`; commits+clean → pass; commits+dirty → `ERR_NO_COMMIT` with capped porcelain+diff; 0 commits → `ERR_NO_COMMIT`; `Git` error → `ERR_GIT`. G2 real `NewExec` over temp repos: head, `RevParse` of branch/`HEAD~1`/unknown/`-bad`, ancestor true/false (exit 1)/error, count, status incl. untracked, diff + truncation at a rune boundary (fixture with a multibyte char straddling `max`), working diff, ref regex, `--end-of-options` observed via the Exec seam's call log; every error branch via a scripted Exec. G3 `TreeHash` literal pin. G4 shared contract suite run against `Fake` and `NewExec` |
| converge | C1 `AllFixed` (empty AllFound → false; Unfixed 0 with AllFound → true). C2 atoms: `no_fixation_progress` nil → false, `Unfixed == Prev` → stop, `Unfixed < Prev` → continue; `max_iterations: 5` stops at `Iteration == 4` and not at 3; `budget` at N−1/N; cmd atom: stdin bytes equal the payload, valid/invalid JSON, unknown field, non-zero exit → `ERR_CMD_FAILED`, runner error propagates. C3 `any`/`all`/`not`: name/class propagation (`all` name join, first class), error abort mid-list (later atoms not evaluated: fake counts). C4 `Validate` rejects unknown cmd name, unknown atom, non-integer N, empty `any`; `Parse` binds names to the Runner |
| machine | M1 `Init` event sequence (`init`+`tree`) with every `InitData` field asserted against a golden; embedded and path workflows; sidecar written; `ERR_WORKFLOW_NOT_FOUND`; goldens ok/invalid/over cap; cmds consent (list + sha in Detail, wrong sha, no cmds → no flag); base resolution via `RevParse` (`--base main` → full sha); mock hash pinned. M2 `Advance` happy paths on both shipped workflows with a fake Registry (host nodes via NEEDS_INPUT+Record; fork nodes via a fake Executor emitting `llm_call`): exact event sequences and payload fields asserted via goldens; `needs_input` appended once across two consecutive advances; `View.NextAction` at each step. M3 gate failure → `failed`; two-failing-gates fixture asserts `Gate` is the first; `ERR_GATE_INAPPLICABLE` path; executor error → `executor` pseudo-gate + partial `llm_call`s kept; decode error → `node_output` pseudo-gate. M4 loop: cumulative regression (iter 3 fixes its own bug, 7 remain: assert loop taken, `AllFound == 8`, `Unfixed == 7`, statuses for all 8, outcome not `fixed`); `stalled` via nil-then-plateau; `overflow` via `max_iterations` and `budget` (judge tokens **and** `record tokens`); `custom` via cmd atom; overflow handler once + failure warn; converge error → `ERR_CONVERGE_FAILED`. M5 fixture: identical porcelain, only head varies → advisory warn vs enforcing `repo_mode` gate + tree; agent-edit state exempt; out-of-node commit detected; `tree` appended only on change (count events). M6 `Record` refusals (each code incl. state-without-node, reserved `mrv_` prefix), `Replace`, `ERR_NODE_OUTPUT_INVALID` leaves `Events` byte-identical. M7 `Open` integrity (`ERR_WORKFLOW_CHANGED` via sidecar edit; embedded bytes changed but sidecar intact → opens), `ERR_CMD_CHANGED`, `ERR_MOCK_MISMATCH` via scenario edit, torn → `ERR_AUDIT_TORN`, `Repair` → warn Detail literal + fold ok. M8 every event's `At` equals the injected clock's sequence, `State/Iter/Mock/Node` per §5.4 tail, and `SnapshotEqualIgnoringSeq(machine state, Fold(Events))` after each step. M9 §5.7 table: one row per outcome asserting Status + ExitCode; `ERR_AUDIT_FULL` surfaced with `MaxEvents` small |

Fakes live in each package (`gate.Fake`, `machine/fakes_test.go` registry/executor/runner/clock/sidecar). No scenario files here.

## 8. Ledger (design/plan deviations decided in this spec)
- `cmds:` is a single top-level declaration referenced by name from nodes, `on_overflow`, and atoms (design §16 wrote argv inline; the plan's second parser and unnamed atoms made the consent sha unstable — SEC-23/CMP). `cmds.<name>.env` is the per-command env pass-through (spec 4 §2 owns the runner's env rules).
- `failed` is reserved: declared, node-less, no outgoing edges, exempt from `unreachable_state`.
- `duplicate_transition` keys on `(from, gate)`; loop safety: `loop_terminal`, `missing_convergence`, `cycle_without_loop`.
- `all_fixed` atom kept; `Advance` decides `fixed` via the gate first (C3).
- `needs_input` is appended once per node key (plan appended on every call; MaxEvents budget).
- `tree` at `Init` and on every hash change (not every advance; MaxEvents budget). Design §10 satisfied by porcelain status.
- `commit_exists` is `FixEntryHead..HEAD` (design §8 wrote `base..HEAD`) with `ERR_GATE_INAPPLICABLE` before the first fix (SCP3-5).
- `Open` verifies the run's own `workflow.yaml` sidecar, never the embedded bytes: binary upgrades and edits to shipped workflows do not orphan in-flight runs (feasibility/data-migration finding). Forks re-copy the sidecar (spec 3).
- Enforcing `UNSANCTIONED_EDIT`, executor failure, and decode failure all leave a `gate{passed:false}` record under the pseudo-gate names `repo_mode`, `executor`, `node_output` (restores plan step 7; ARC-18/FEA-N6).
- `match` calls at `max_tokens 1024` are parity with `sdlc_loop.py:274` (spec 4 §9).
- Reassigned: CMP-15/ARC-21 → spec 3; `ERR_JUDGE_UNSET` mapping → spec 5; env allow-list docs → spec 5.
- Accepted: `bugs_remain = !AllFixed` reads "nothing found" as bugs remaining; shipped loops guard with `confirmed_nonempty`, user loops get a Parse-time warning (not an error) when a loop-carrying state has no `findings_*`/`confirmed_*` guard upstream — recorded in `Warnings` of `Parse`'s result (`workflow.Parse` returns warnings via `w.Warnings []string`).
