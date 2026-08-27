# metareview 0.9.0 — spec 2: workflow, gates, convergence, and the machine core

> **Status:** DRAFT r1 (2026-08-27). Second of the five split 0.9.0 artifacts (ownership ledger: run spec §12).
> Builds on `internal/fsm/run` (r4, implemented, 100%). Owns plan r3 §1.1 (layout), §1.3 gate/converge/workflow
> types, §1.5 `Advance`, `Record`, §1.6 outcomes, §6 shipped workflows (already embedded in `workflows/`), the
> canonical `cmds_sha256` preimage, the `PrevUnfixed == nil` rule, `tree` cadence + `TreeHash` preimage +
> `UNSANCTIONED_EDIT`, the repair-`warn` emission, and the `Status` full-coverage input contract for verify.
>
> **Scope rule:** what the deterministic core *decides*: how a YAML becomes a `Workflow`, what each gate and
> atom returns, and exactly which `run` events `Init`/`Advance`/`Record` append. It does not define kinds'
> prompts/executors (spec 4), forks/diff/export/runs.jsonl (spec 3), or the CLI (spec 5). Every LLM or shell
> effect is behind an interface this spec declares and spec 4 implements.

---

## 1. Packages and dependency direction

```
internal/fsm/workflow   YAML → Workflow; validation; var resolution; cmd resolution + cmds_sha256; WorkflowHash
internal/fsm/gate       Git interface (exec-backed + fake); 7 gates
internal/fsm/converge   AllFixed; atoms; any/all/not; Parse; CmdPredicate over a Runner interface
internal/fsm/machine    Deps; Init/Open/Advance/Record/View; node interfaces (implemented by spec 4's kinds)
```
`run` ← all; `converge` ← `gate` (`AllFixed`); `workflow`, `gate`, `converge` ← `machine`. `machine` imports no
kinds/judge/cmdexec package: it consumes the interfaces in §5.1. `workflows` (embed) ← `machine` (name resolution).
Only external dependency: `gopkg.in/yaml.v3`.

## 2. `workflow`

### 2.1 Types
```go
type VarSpec struct { Default string; Required bool }
type Node struct { Name string; Kind string; Exec string /* inline|subagent|fork */; Model, Effort string /* after $VAR substitution */; Params map[string]any /* e.g. lenses: 8 */; Cmd []string /* CmdKind only, resolved argv */ }
type Transition struct { From, To run.State; Gate string; Outcome run.Outcome; Loop bool }
type Workflow struct {
    Name string; Version int; Vars map[string]VarSpec; States []run.State; Initial run.State
    Transitions []Transition          // declaration order
    Nodes map[run.State]*Node         // states without a node have no entry
    Convergence *yaml.Node            // parsed by converge.Parse
    RepoMode string                   // advisory|enforcing
    OnOverflow *Cmd                   // {Argv []string; Timeout time.Duration}
    Hash string                       // sha256 of the raw file bytes (WorkflowHash)
    Cmds []CmdRef                     // every cmd: {Name string; Argv []string} incl. atoms and on_overflow, in declaration order
}
func Parse(raw []byte) (*Workflow, error)                                          // structure + static validation; $VAR unresolved
func (w *Workflow) Resolve(vars map[string]string, calibration bool) (*Workflow, map[string]string, error)  // substitutes $VAR everywhere; returns the resolved workflow and the effective vars
func (w *Workflow) NodeFor(s run.State) *Node; func (w *Workflow) Outgoing(s run.State) []Transition
func (w *Workflow) IsTerminal(s run.State) bool                                    // no outgoing transitions
func (w *Workflow) LoopTransition() *Transition                                    // the one with Loop, or nil
func (w *Workflow) VarsReferencedBy(s run.State) []string                          // $VARs in that node's model/effort/params/cmd (spec 3's freeze rule)
```

### 2.2 YAML schema (both forms accepted)
List form (shipped): `transitions: [{from, to, gate, outcome?, loop?}]`. Mapping form (design spec §5):
`transitions: {"discover→adjudicate": {gate: …}, "*→failed": {on: gate_error}}` — parsed via `yaml.Node` to keep order;
the `*→failed` entry is accepted and ignored (the implicit rule). `nodes.<state>: {kind, exec?, model?, effort?, <params>…, cmd?}`;
`vars.<NAME>: {default?, required?}`; `convergence:` any predicate tree (§4); `repo_mode:`; `on_overflow: {cmd: [...], timeout?: N}`.
`workflow:` and `version: 1` required.

### 2.3 Static validation (Parse) — error codes are `ERR_WORKFLOW_INVALID` with a `Reason`:
`missing_name`, `bad_version`, `unknown_state` (transition/node names not in `states`), `no_initial` (`states[0]` is the
initial state), `unreachable_state`, `duplicate_transition` (same from/to), `terminal_without_outcome` (a transition into a
terminal state lacks `outcome`), `outcome_on_nonterminal`, `bad_outcome` (not in `run.Outcomes`), `loop_count` (more than one
`loop: true`), `loop_not_cycle` (`loop` transition's `to` must already be reachable from `from`... simplified: `to` must be an
ancestor of `from`), `node_without_kind`, `unknown_exec`, `exec_kind_mismatch` (`review-lenses`/`agent-edit` may not be `fork`;
`match-then-adjudicate`/`still-present` must be `fork`; `cmd` kinds must be `fork`), `terminal_with_node`, `unknown_gate`,
`bad_convergence` (from `converge.Parse`), `bad_repo_mode`, `bad_cmd` (non-list argv, empty). Gate names are validated against
`gate.Names()`. Known kinds are validated against a list supplied by the caller (`Parse` takes `Options{Kinds []string}`;
default = the four built-ins + `cmd`).

### 2.4 Resolution
`$NAME` tokens in `model`, `effort`, string params, `cmd` argv, `on_overflow.cmd`, and convergence cmd atoms are substituted
from `vars` (CLI) over `Default`s. `Required` without a value → `ERR_VAR_UNSET{Name}` (for `JUDGE` the CLI reports
`ERR_JUDGE_UNSET`). Unknown `$X` → `ERR_VAR_UNKNOWN`. `--calibration` pins `JUDGE=gpt-5.2`, `JUDGE_EFFORT=medium` and refuses
explicit values for either (`ERR_CALIBRATION_PINNED`).

### 2.5 Commands and `cmds_sha256`
`ResolveCmds(w, workDir, lookPath func(string) (string, error), hash func(path string) (string, error)) ([]run.AllowedCmd, string, error)`:
for every `CmdRef`, `argv[0]` is resolved to an absolute path (`lookPath`; missing → `ERR_CMD_NOT_FOUND{Name}`), every argv
element that names an existing regular file (absolute, or relative to `workDir`) is sha256-hashed into `FileHashes`
(absolute path → hash). **Preimage** of `cmds_sha256`: `Canonical` JSON of `[{name, argv, file_hashes}]` sorted by name, with
`file_hashes` keys sorted (Go maps marshal sorted) — `sha256` hex of those bytes. Re-verification (`VerifyCmds(allowed, hash)`)
recomputes each file hash → `ERR_CMD_CHANGED{Path}` on mismatch or a now-missing file.

## 3. `gate`
```go
type Git interface {
    Head(ctx) (string, error)
    IsAncestor(ctx, a, b string) (bool, error)            // git merge-base --is-ancestor a b
    CommitCount(ctx, from, to string) (int, error)        // git rev-list --count from..to
    Status(ctx) (clean bool, porcelain string, err error) // git status --porcelain=v2 --untracked-files=all
    Diff(ctx, from, to string) (string, error)            // git diff from..to  (BaseSHA..HEAD for kinds)
    WorkingDiff(ctx) (string, error)                      // git diff HEAD (for ERR_NO_COMMIT detail)
}
```
Refs are validated `^[0-9a-f]{7,40}$|^HEAD$` before use and passed after `--end-of-options`; violation → `ERR_GIT_REF`. Real
implementation shells out (`exec.CommandContext`, `Dir` = work dir, env `GIT_TERMINAL_PROMPT=0`); tests use temp repos and a
`Fake`. `TreeHash(head, porcelain) = sha256(head + "\n" + porcelain)`.

Gates — `type Gate func(ctx, run.Snapshot, Git) *run.GateError`; `Names()` and `Builtin()`:

| gate | pass | error code |
|---|---|---|
| `findings_nonempty` / `findings_empty` | `len(Findings) > 0` / `== 0` | `ERR_NO_FINDINGS` / `ERR_FINDINGS_PRESENT` |
| `confirmed_nonempty` / `confirmed_empty` | `len(Confirmed) > 0` / `== 0` | `ERR_NO_CONFIRMED` / `ERR_CONFIRMED_PRESENT` |
| `commit_exists` | `FixEntryHead != ""` else `ERR_GATE_INAPPLICABLE`; `CommitCount(FixEntryHead, HEAD) > 0 && clean` | `ERR_NO_COMMIT` with `Detail` = porcelain + `WorkingDiff` via `run.CapDetail` |
| `all_fixed` / `bugs_remain` | `converge.AllFixed(snap)` / `!` | `ERR_BUGS_REMAIN` / `ERR_ALL_FIXED` |

## 4. `converge`
```go
func AllFixed(s run.Snapshot) bool          // len(AllFound) > 0 && Unfixed == 0   (nothing found is NOT fixed; review-loop uses findings_empty)
type Predicate interface { Name() string; Class() run.Outcome; Evaluate(run.Snapshot) (stop bool, reason string, err error) }
type Runner interface { Run(ctx, name string, argv []string, stdin []byte, timeout time.Duration) (stdout []byte, exitCode int, err error) }   // spec 4's cmdexec.Guarded satisfies it
func Parse(node *yaml.Node, opts ParseOptions) (Predicate, error)   // ParseOptions{Runner; AllowCmds bool}
```
Atoms: `all_fixed` (class `fixed`; never stops a loop in `Advance` — `Advance` checks `AllFixed` first), `no_fixation_progress`
(class `stalled`: `PrevUnfixed != nil && Unfixed >= *PrevUnfixed`; **nil ⇒ false**, so the first boundary never stalls),
`max_iterations: N` (class `overflow`: `Iteration+1 >= N` evaluated *before* the loop transition, i.e. stop when the next
iteration would be the N-th... precisely: stop iff `Iteration+1 >= N`), `budget: {tokens: N}` (class `overflow`:
`Tokens.Total() >= N`), `{cmd: [argv...], timeout?}` (class `custom`: runs with the snapshot JSON on stdin; stdout must decode to
`{"stop": bool, "reason": string}` else the atom returns `ERR_CMD_OUTPUT_INVALID`; refused at parse when `!AllowCmds`).
Compose: `any: [...]`, `all: [...]`, `not: <atom>`. `any` stops with the first firing atom's name/class; `all` stops only if all
fire (name = joined, class = the first's); `not` inverts stop (class = inner's). Errors from any atom abort evaluation.

## 5. `machine`

### 5.1 Consumed interfaces (implemented by spec 4)
```go
type Instructions struct { Text string; Input map[string]any; Untrusted []string; OutputSchema json.RawMessage }
type NodeKind interface {
    Name() string; IsLLM() bool; AllowedExec() []string
    Instructions(snap run.Snapshot, node *workflow.Node, diff string) (Instructions, error)
    Decode(raw json.RawMessage) (any, error)                       // typed, DisallowUnknownFields
    Reduce(snap run.Snapshot, out any) (run.Delta, error)
}
type Executor interface { Execute(ctx, snap run.Snapshot, node *workflow.Node, diff string, audit func(run.Event)) (any, error) }   // fork kinds: emits llm_call/cmd_call events via audit
type Registry interface { Kind(name string) (NodeKind, bool); Executor(name string) (Executor, bool) }
type Clock func() run.Time
```

### 5.2 Deps and API
```go
type Deps struct { Store run.RunStore; Kinds Registry; Git func(workDir string) gate.Git; Runner converge.Runner; Clock Clock; LookPath func(string) (string, error); FileHash func(string) (string, error); Workflows func(name string) ([]byte, error) }
type InitOptions struct { Workflow string /* name or path */; Vars map[string]string; Base string; RepoMode string; AllowCustomCmds string /* sha256 */; Calibration bool; MockDir string; GoldensPath string; WorkDir, RepoRoot string }
func Init(ctx, Deps, InitOptions) (*Machine, error)
func Open(ctx, Deps, runID string) (*Machine, error)        // folds; verifies WorkflowHash, cmds, mock (ERR_WORKFLOW_CHANGED, ERR_CMD_CHANGED, ERR_MOCK_MISMATCH)
func (m *Machine) Advance(ctx) (AdvanceResult, error)
func (m *Machine) Record(ctx, RecordOptions) (RecordResult, error)   // RecordOptions{Kind: node-output|tokens|event; Node string; Data json.RawMessage; Replace bool; Name string}
func (m *Machine) View() View                                        // Snapshot + workflow name + next action
type AdvanceResult struct { Status string /* ADVANCED|NEEDS_INPUT|DONE|STOPPED|GATE_FAILED */; From, To run.State; Gate *run.GateData; Outcome run.Outcome; StopReason string; NeedsInput *NeedsInput; Warnings []string; ExitCode int }
type NeedsInput struct { Node string; Kind, Exec, Model, Effort string; Instructions Instructions; Record string }
```

### 5.3 `Init` — the events it appends
1. Load YAML (embedded name via `Deps.Workflows`, or a path containing `/` or ending `.yaml`); `workflow.Parse`; `Resolve(vars, calibration)`.
2. `ResolveCmds`; if any cmd exists and `AllowCustomCmds != cmds_sha256` → `ERR_CMDS_NOT_ALLOWED` carrying the printed list and the sha (exit 2 in the CLI). No cmds → no flag needed.
3. Git in `WorkDir`: `Head` → `Head`; `Base` (default `HEAD`) → `BaseSHA`; `Status` → tree hash.
4. Goldens: read `GoldensPath` (JSON array of `run.Golden`) if given.
5. `run.Create(runID, init)` with `InitData{…, InitialState: w.Initial, InitialKind: kind of the initial node or "", Mock: MockDir, AllowedCmds, CmdsSHA256, WorkflowHash: w.Hash, RepoMode, Lineage: []}`; then under the lock append `tree{Head, TreeHash, Status}`.
6. Returns the machine; `View().NextAction == "advance"`.

### 5.4 `Advance` — producer of §7-conforming logs (plan r3 §1.5 restated over `run`)
```
1  lock; st ← FoldFull(Events); ChainHead ← Log.Head; Torn → ERR_AUDIT_TORN (the CLI prints the repair hint; a
   `--repair` flag calls RepairTail then appends warn{AUDIT_TORN_LINE_DROPPED})
2  snap.Outcome != "" → ERR_RUN_TERMINAL
3  integrity: WorkflowHash / cmds (VerifyCmds) / MockDir vs snap.Mock
4  head, porcelain ← Git; h ← TreeHash(head, porcelain); node ← w.NodeFor(state)
   if snap.TreeHash != "" && h != snap.TreeHash && (node == nil || node.Kind != agent-edit):
       advisory → append warn{UNSANCTIONED_EDIT, detail: porcelain (capped)}; enforcing → gate error ERR_UNSANCTIONED_EDIT (→ step 8 as a failure of the pseudo-gate "repo_mode")
   append tree{head, h, porcelain capped} (every advance; this is the §10 working-tree snapshot)
5  if node != nil:
       k ← Key(node, iter)
       if NodeOutputs[k] absent:
           fork: executor.Execute(...) with audit appending llm_call/cmd_call; append node_output{Canonical(out)}
                 (a fork kind's Execute is given the snapshot, node, and Diff(BaseSHA..HEAD))
           else: return NEEDS_INPUT (exit 3) with Instructions(snap, node, diff); nothing else appended
       if !Applied[k]: delta ← kind.Reduce(snap, kind.Decode(output)); on error → the GateError ERR_NODE_OUTPUT_INVALID (→ 8);
                       append delta_applied{delta, OutputHash(output)}
6  transitions ← w.Outgoing(state) in order; chosen, first ← nil, nil
   for t in transitions:
       if t.Loop: (loop boundary) — first evaluate the terminal sibling: if AllFixed(snap) and a sibling t' into a terminal
                  state with gate all_fixed exists → chosen ← t'; else stop, atom ← Convergence.Evaluate(snap); append converge;
                  if stop: chosen ← synthetic {To: terminal target of the sibling with outcome, Gate: atom.Name(), Outcome: atom.Class()}
                  (`TerminalFor`: the unique terminal state that the loop-carrying state has an outcome-bearing transition to;
                  validated at Parse as `loop_terminal`); else: gate(t.Gate) → chosen ← t on pass
       else: err ← gate(t.Gate)(snap); pass → chosen ← t; else first ??= err
       break on chosen
7  every gate evaluation appends gate{name, passed, error}
8  chosen == nil: append transition{From: state, To: "failed", Outcome: failed, Head: head} → GATE_FAILED, exit 1, resume_hint (spec 3)
9  else append transition{From, To, Gate, Outcome: chosen.Outcome, Loop, ToKind: kind of To's node, Head: head}
   if Outcome == overflow && OnOverflow != nil && !OverflowHandled: Runner.Run(on_overflow, snapshot JSON) → append overflow_handler{...}
   (failure → also warn{OVERFLOW_HANDLER_FAILED}); exit code per §1.6 of the run spec's outcome table
```
Every appended event is stamped by the machine: `At ← Clock()`, `State ← current state` (`From` for transitions), `Iter ←`
current iteration (new iteration for the loop transition), `Mock ← snap.Mock != ""`, `Node` for node-scoped events.

### 5.5 `Record`
- `node-output`: run not terminal (`ERR_RUN_TERMINAL`); `Node == w.NodeFor(state).Name` (`ERR_NODE_MISMATCH`); node exec is
  `inline|subagent` (`ERR_NODE_NOT_HOST`); `Applied[k]` false (`ERR_NODE_OUTPUT_APPLIED`); `NodeOutputs[k]` absent unless
  `Replace` (`ERR_NODE_OUTPUT_EXISTS`); `kind.Decode(data)` must succeed (`ERR_NODE_OUTPUT_INVALID`, nothing appended);
  append `node_output`. (The delta is applied on the next `Advance`, step 5.)
- `tokens`: decode `run.TokenTotals` → append `tokens`.
- `event`: any other name → append `record{name, data}`; `Name` must not collide with a run event type (`ERR_RECORD_NAME`).

### 5.6 `Status` input contract (for spec 4's `still-present`)
A verify output must carry a status for **every** bug in `AllFound` (the machine passes `AllFound` in `Instructions.Input`
and in the executor's snapshot); `Reduce` returns exactly that set or `run` rejects with `status_incomplete`.

## 6. Errors (machine + workflow + gate; all as `*Error{Code, Detail}` implementing `error`)
`ERR_WORKFLOW_INVALID`, `ERR_WORKFLOW_NOT_FOUND`, `ERR_VAR_UNSET`, `ERR_JUDGE_UNSET`, `ERR_VAR_UNKNOWN`, `ERR_CALIBRATION_PINNED`,
`ERR_CMDS_NOT_ALLOWED`, `ERR_CMD_NOT_FOUND`, `ERR_CMD_CHANGED`, `ERR_GIT`, `ERR_GIT_REF`, `ERR_RUN_TERMINAL`, `ERR_WORKFLOW_CHANGED`,
`ERR_MOCK_MISMATCH`, `ERR_NODE_MISMATCH`, `ERR_NODE_NOT_HOST`, `ERR_NODE_OUTPUT_APPLIED`, `ERR_NODE_OUTPUT_EXISTS`,
`ERR_NODE_OUTPUT_INVALID`, `ERR_RECORD_NAME`, `ERR_UNSANCTIONED_EDIT`, `ERR_GATE_INAPPLICABLE`, gate codes (§3),
`ERR_CMD_OUTPUT_INVALID`, `ERR_CMD_FAILED`, `ERR_CMD_TIMEOUT` (surfaced from the Runner).

## 7. Tests (each package 100% statements; TDD)

| pkg | rows |
|---|---|
| workflow | W1 parse both shipped YAMLs + mapping form (order preserved, `*→failed` ignored); W2 one row per `Reason` in §2.3; W3 `Resolve` defaults/required/unknown/calibration incl. `JUDGE`; W4 `ResolveCmds` with fake lookPath/hash: absolute pin, file-hash closure over argv elements (incl. `["bash","./s.sh"]`), preimage pinned to a committed sha of a fixture, `VerifyCmds` change/missing; W5 `VarsReferencedBy`, `Outgoing` order, `IsTerminal`, `LoopTransition`, `Hash` |
| gate | G1 each gate pass/fail with exact code + detail (`commit_exists`: no fix entry → inapplicable; commits+clean → pass; commits+dirty → `ERR_NO_COMMIT` with capped porcelain+diff; no commits → `ERR_NO_COMMIT`); G2 real `Git` in temp repos (head, ancestor, count, status incl. untracked, diff, working diff, ref validation, `--end-of-options`); G3 `TreeHash` pin; G4 `Fake` conformance |
| converge | C1 `AllFixed` (empty AllFound → false); C2 each atom incl. `no_fixation_progress` nil-`PrevUnfixed` → false and `Unfixed >= Prev` boundary; `max_iterations` boundary; `budget` boundary; C3 `any`/`all`/`not` semantics + class/name propagation + error abort; C4 cmd atom: refused without AllowCmds, stdin = snapshot JSON, valid/invalid/nonzero/timeout via a fake Runner; C5 parse errors |
| machine | M1 `Init` event sequence (init + tree) with every InitData field, embedded and path workflows, `ERR_WORKFLOW_NOT_FOUND`, goldens, cmds consent (list + sha printed, wrong sha, no cmds → no flag); M2 `Advance` happy paths on both shipped workflows with a fake Registry (host nodes via NEEDS_INPUT + Record, fork nodes via a fake Executor emitting llm_call): exact event sequences asserted; M3 gate failure → `failed` + GATE_FAILED; M4 loop: cumulative convergence regression (iter 3 fixes its own bug, 7 remain → no `fixed`), `stalled` via nil-then-plateau, `overflow` via max_iterations and budget (judge tokens **and** `record tokens`), `custom` via cmd atom, overflow handler once + failure warn; M5 advisory `UNSANCTIONED_EDIT` warn vs enforcing gate error; edits during agent-edit exempt; out-of-node commit detected; M6 `Record` refusals (each code) and `Replace`; `ERR_NODE_OUTPUT_INVALID` leaves the log unchanged; M7 `Open` integrity (`ERR_WORKFLOW_CHANGED`, `ERR_CMD_CHANGED`, `ERR_MOCK_MISMATCH`, `ERR_RUN_TERMINAL`, torn); M8 every event appended carries correct stamps (fold succeeds after each step; `SnapshotEqualIgnoringSeq` of the machine's state vs `Fold(Events)`); M9 exit codes per outcome |

Fakes live in each package (`gate.Fake`, `machine/fakes_test.go` registry/executor/runner/clock). No scenario files here.

## 8. Ledger
- `all_fixed` atom: kept as a name but `Advance` decides `fixed` via the gate, never via the predicate (design spec §9 listed it as an atom; C3 of the plan).
- `TerminalFor`: validated at parse (`loop_terminal`).
- `Init` appends a `tree` event; `Advance` appends one every call (design §10 satisfied by porcelain status).
- `UNSANCTIONED_EDIT` compares `TreeHash(head, porcelain)`; an out-of-node *commit* changes `head` and is detected.
