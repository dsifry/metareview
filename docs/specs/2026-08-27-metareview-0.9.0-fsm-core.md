# metareview 0.9.0 — spec 2: workflow, gates, convergence, and the machine core

> **Status:** r4 — BUILD BASELINE after ESCALATION (2026-08-27). Attempt 3 (`mrv-20260827-070456832125000-…`) ended NEEDS_REVISION on 5/8 lenses with every blocker mechanical; per the same-target rule the chain is ESCALATED and a human must accept this r4 (Dave's "proceed" precedent for the run spec is applied provisionally so the unattended build can continue). r3 note: Second of the five split 0.9.0 artifacts (ownership ledger: run spec §12).
> Builds on `internal/fsm/run` (r4 + the §9 amendments below, implemented, 100%). Owns plan r3 §1.1 (layout), §1.3
> gate/converge/workflow types, §1.5 `Advance`, `Record`, §1.6 outcomes (§5.7), §6 shipped workflows, the canonical
> `cmds_sha256` preimage, the `PrevUnfixed == nil` rule, `tree` cadence + `TreeHash` preimage + `UNSANCTIONED_EDIT`,
> the repair-`warn` emission, and the `Status` full-coverage input contract for verify.
>
> **r3 changes** (review `mrv-20260827-064453390399000-…`, attempt 2): the `run` amendments are now owned and
> implemented (§9: `AllowedCmd.TimeoutMS/Env` + `MaxEnv`, `tokens_negative`, `FoldState.NextIndex`); `init` carries no
> stamps; `ChainHead ← log.Head`; `Create` before the sidecar (`O_EXCL`); loop boundary is order-independent and
> gate-first by construction; converge errors are a `converge` pseudo-gate; enforcing edits append the gate before the
> `tree`; `on_overflow` is crash-resumable; `TreeHash` is content-aware (`Git.WorkTree`); executors receive
> `StartIndex` and return validated JSON; `Kinds` required (no duplicate table); `Predicate.Evaluate` returns a
> `Result`; `converge.Payload` is the payload home; atom grammar declared; `$NAME` grammar declared; `bad_state`,
> `bad_env`, `bad_params`, `missing_kinds` reasons; negative tokens refused; `Deps.Terminal` hook; every test row
> names its discriminating fixture and the authority is hand-written, goldens regression-only.
>
> **r4 changes (attempt 3, all mechanical; code already reflects them):** `all_fixed` legal only at the top level or as a
> direct child of a top-level `any`, `not` always classes `custom`, tree depth ≤ 4 / width ≤ 32 (`converge`); the loop
> boundary evaluates convergence whenever the terminal gate fails (no `!AllFixed` guard); `sdlc-loop`'s clean exits use
> the new `nothing_found`/`nothing_confirmed` gates (iteration-0 only; a later discovery miss is `GATE_FAILED`, visible);
> `Deps.Terminal` is idempotent and runs on every terminal advance (incl. `failed`, incl. resume); `tree` baseline when
> `TreeHash == ""`; enforcing edits append the gate and **no** tree; `first` = pseudo-gate else evaluation order; `Open`
> algorithm (§5.3b); `tree.Status` capped; `Workflow.Hash` preimage; `initial_terminal`, `unknown_var`, `bad_yaml` reasons
> in order; `RepoMode` override tighten-only; token counters capped per record (`tokens_too_large`); `GIT_*` scrubbed by
> prefix and excludes/attributes disabled for `WorkTree`; `Snapshot.Clone` keeps `TimeoutMS/Env`; `CmdsSHA256` uses
> `run.MarshalCanonical`; `Sidecar.Read` capped + `O_NOFOLLOW` + `ValidateRunID`; `Untrusted` includes `error.detail`;
> ctx cancellation is returned, never a pseudo-gate; sidecar obligation for forks restated (child's own bytes).
>
> **Open for Dave — D1 (`AllFixed` needs a non-empty `AllFound`):**
>
> | option | loop state reached with nothing ever found | discover finds nothing at iteration 0 | discover misses remaining bugs at iteration ≥ 1 |
> |---|---|---|---|
> | **A (current, r4)** `AllFixed = len(AllFound) > 0 && Unfixed == 0`; `nothing_found`/`nothing_confirmed` clean exits | convergence runs → `overflow`/`stalled`, exit 1 (shipped loop cannot reach this: verify needs `confirmed_nonempty`) | `clean`, exit 0 (sdlc-loop) / `GATE_FAILED` if the workflow has no clean exit (Parse warns) | `findings_nonempty` fails → `GATE_FAILED`, exit 1 — visible, fork to retry |
> | **B (design-literal)** `AllFixed = Unfixed == 0` | `fixed`, exit 0 with nothing found | same as A | same as A |
>
> **Decided 2026-08-27 by Dave: keep A.** (B remains documented here for the record.)
>
> **Scope rule:** what the deterministic core *decides*: how a YAML becomes a `Workflow`, what each gate and atom
> returns, and exactly which `run` events `Init`/`Advance`/`Record` append. Kinds' prompts/executors are spec 4, forks/
> diff/export/runs.jsonl are spec 3, the CLI is spec 5. Every LLM or shell effect is behind an interface declared here.

---

## 1. Packages and dependency direction

```
internal/fsm/errs       Error{Code, Detail, Fields}; E, Wrap, Is, Code, As                       (implemented)
internal/fsm/converge   AllFixed; Result; Predicate; Validate/Parse; atoms; Payload; CmdResult/Runner  (implemented)
internal/fsm/gate       Git (exec seam + Fake); ValidSHA/ValidRef; 7 gates; TreeHash; Cut          (implemented)
internal/fsm/workflow   YAML → Workflow; validation; $VAR resolution; ResolveCmds/VerifyCmds + cmds_sha256
internal/fsm/machine    Deps; Init/Open/Advance/Record/View; consumed interfaces; Sidecar (FS + Mem)
```
Edges: `errs` ← all; `run` ← all; `converge` ← `run`; `gate` ← `converge`; `workflow` ← `gate` (`Names()`), `converge`
(`Validate`); `machine` ← `workflow`, `gate`, `converge`. `machine` imports no kinds/judge/cmdexec/workflows package —
the CLI (spec 5) wires them through `Deps`. No cycles (`kind` → `machine`, `workflow`, never the reverse). External:
`gopkg.in/yaml.v3` (`go.sum` ships).

## 2. `workflow`

### 2.1 Types
```go
type VarSpec struct { Default string; Required bool }
type CmdDecl struct { Name string; Argv []string; Timeout time.Duration /* default 60s; 1s..3600s */; Env []string }
type Node struct { Name string; Kind string; Exec string /* inline|subagent|fork; from KindInfo.DefaultExec when omitted */; Model, Effort string; Params map[string]any; Cmd string }
type Transition struct { From, To run.State; Gate string; Outcome run.Outcome; Loop bool }
type KindInfo struct { DefaultExec string; AllowedExec []string; ValidateParams func(map[string]any) error /* nil → any */ }
type Options struct { Kinds map[string]KindInfo /* required (missing_kinds); the CLI passes Registry.Info() */ }
type Workflow struct {
    Name string; Version int; Vars map[string]VarSpec; States []run.State; Initial run.State
    Transitions []Transition; Nodes map[run.State]*Node; Cmds map[string]*CmdDecl
    Convergence *yaml.Node; RepoMode string; OnOverflow string; Hash string /* hex sha256 of the raw bytes given to Parse */
    Refs map[run.State][]string; CmdRefs map[string][]string   // $VARs per node / per cmd, computed at Parse (pre-resolution)
    Warnings []string                                          // non-fatal Parse observations (§2.3 end)
}
func Parse(raw []byte, opts Options) (*Workflow, error)
func (w *Workflow) Resolve(vars map[string]string, calibration bool) (*Workflow, map[string]string, error)
func (w *Workflow) NodeFor(s run.State) *Node; Outgoing(s) []Transition; IsTerminal(s) bool; LoopTransition() *Transition
func (w *Workflow) TerminalFor(s run.State) *Transition            // the loop-carrying state's outcome-bearing terminal transition (nil elsewhere)
func (w *Workflow) VarsReferencedBy(s run.State) []string          // Refs[s] ∪ CmdRefs[node.Cmd], sorted unique; valid on resolved copies
```

### 2.2 YAML schema
```yaml
workflow: sdlc-loop
version: 1
vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5} }
states: [discover, adjudicate, fix, verify, done, failed]   # states[0] initial; failed reserved
cmds:                                                      # optional; the only place argv appears
  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }
nodes:
  discover:   { kind: review-lenses, exec: subagent, model: $REVIEWER, lenses: 8 }
  adjudicate: { kind: match-then-adjudicate, exec: fork, model: $JUDGE, effort: $JUDGE_EFFORT }
  fix:        { kind: agent-edit }
  verify:     { kind: still-present, model: $JUDGE, effort: $JUDGE_EFFORT }
transitions:                                               # list form (shipped)
  - { from: discover, to: done, gate: findings_empty, outcome: clean }
  - { from: discover, to: adjudicate, gate: findings_nonempty }
  - { from: adjudicate, to: done, gate: confirmed_empty, outcome: clean }
  - { from: adjudicate, to: fix, gate: confirmed_nonempty }
  - { from: fix, to: verify, gate: commit_exists }
  - { from: verify, to: done, gate: all_fixed, outcome: fixed }
  - { from: verify, to: discover, gate: bugs_remain, loop: true }
convergence: { any: [ no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}, {cmd: notify} ] }
repo_mode: advisory
on_overflow: notify
```
This example parses (W1 pins it). Mapping-form transitions (design §5): `transitions: {"discover→adjudicate": {gate: …},
"*→failed": {on: gate_error}}` — keys split on `→` or `->`; order preserved via `yaml.Node`; the `*→failed` entry is
accepted and ignored. `nodes.<state>`: `kind` + optional `exec`, `model`, `effort`, `cmd`; every other key is a param.
`cmds.<name>`: `argv` (non-empty list of non-empty strings), `timeout` seconds (integer 1..3600, default 60), `env`
(names). Node `cmd:`, `on_overflow:`, and `{cmd: <name>}` atoms reference `cmds` by name. Unknown top-level keys →
`unknown_key`. `$NAME` grammar: `\$([A-Z_][A-Z0-9_]*)` (longest match; `$JUDGE_EFFORT` is one token); `$$` is a literal
`$`; any other `$` (`$1`, `${X}`, a trailing `$`) is left literal. Substitution covers `model`, `effort`, top-level string
params and strings inside list params (nested maps are not walked), and every argv element. Shipped YAMLs: `sdlc-loop` gained `discover→done findings_empty (clean)` and
`adjudicate→done confirmed_empty (clean)` rows and the r2-form escape-hatch comment (ledger); `review-loop` unchanged.

### 2.3 Static validation (Parse) — `ERR_WORKFLOW_INVALID{reason, at}`, first failure in this order
| reason | rule |
|---|---|
| `missing_kinds` | `Options.Kinds` nil/empty |
| `bad_yaml` | the document does not decode (malformed YAML, duplicate mapping keys, non-mapping root); `at = document` |
| `unknown_key` | unknown key at the top level, inside a `cmds.<name>` or transition entry, or a reserved node key with a non-string value |
| `missing_name`, `bad_version` | `workflow` non-empty; `version == 1` |
| `no_initial`, `bad_state` | `states` non-empty; each `^[a-z][a-z0-9_-]{0,31}$`, unique, `judge` reserved (spec 5's `fsm judge` node) |
| `bad_var` | var name not `^[A-Z_][A-Z0-9_]*$`; `required` with `default`; more than `MaxVars` |
| `bad_cmd` | argv empty / non-string element / empty element; `timeout` non-integer or outside 1..3600; name not `^[a-z][a-z0-9_-]{0,31}$`; more than `MaxAllowedCmds`; argv longer than `MaxArgv` |
| `bad_env` | name not `^[A-Z_][A-Z0-9_]*$`, duplicate, more than `MaxEnv`, or reserved: `PATH HOME LANG TMPDIR`, `MRV_*`, `LD_*`, `DYLD_*`, `GIT_*`, `BASH_ENV ENV SHELLOPTS PS4 IFS CDPATH GLOBIGNORE PROMPT_COMMAND`, `PYTHONPATH PYTHONSTARTUP PYTHONHOME`, `NODE_OPTIONS NODE_PATH`, `PERL5OPT PERL5LIB`, `RUBYOPT RUBYLIB`, `JAVA_TOOL_OPTIONS` (best-effort denylist; the consent list shows env **names**) |
| `duplicate_cmd` | two `cmds` keys with the same name (detected on the `yaml.Node`) |
| `unknown_state` | transition `from`/`to` or node key not in `states` |
| `failed_reserved` | `failed` must be declared, has no node, appears in no transition |
| `node_without_kind`, `unknown_kind`, `unknown_exec`, `exec_kind_mismatch` | kind ∈ `Kinds`; exec ∈ {inline, subagent, fork} and ∈ `AllowedExec` (omitted → `DefaultExec`) |
| `bad_params` | `KindInfo.ValidateParams(params)` failed (`Fields.detail`) |
| `cmd_without_kind`, `unknown_cmd` | `cmd:` on a non-`cmd` kind / `cmd` kind without `cmd:`; any reference (node, `on_overflow`, atom) to an undeclared name |
| `initial_terminal` | the initial state has no outgoing transition |
| `terminal_with_node` | a state with no outgoing transition carries a node |
| `unknown_gate` | gate ∉ `gate.Names()` |
| `duplicate_transition` | same `(from, gate)` twice |
| `terminal_without_outcome` / `outcome_on_nonterminal` / `bad_outcome` | into a terminal state ⇒ `outcome` ∈ `run.Outcomes` minus `failed`; into a non-terminal ⇒ none |
| `unreachable_state` | non-initial state other than `failed` with no incoming transition |
| `loop_count` | more than one `loop: true` |
| `loop_not_cycle` | the loop's `to` must reach `from` via non-loop transitions |
| `loop_terminal` | the loop's `from` must have exactly one non-loop transition into a terminal state carrying an outcome |
| `missing_convergence` | `loop: true` without `convergence:` |
| `cycle_without_loop` | a cycle among non-loop transitions |
| `bad_convergence` | `converge.Validate(node, cmdNames)` failed (`Fields.detail`) |
| `bad_repo_mode` | not `advisory`/`enforcing` |
| `unknown_var` | a `$X` in model/effort/params/argv names an undeclared var (`at` = the node or cmd) |

Parse never refuses on consent grounds. **Warnings** (`w.Warnings`): `loop_without_clean_exit` when a loop exists and no
transition carries outcome `clean` (D1). `Init` appends `warn{WORKFLOW_WARNING, Detail}` per warning.

### 2.4 Resolution
`$NAME` tokens in `model`, `effort`, string params (top-level strings and strings inside lists), and every `cmds.*.argv`
element are substituted from `vars` (caller) over `Default`s; caller names not declared → `ERR_VAR_UNKNOWN{name}`;
`$X` referencing an undeclared var → `ERR_VAR_UNKNOWN{name}` (detected at Parse as well: reason `unknown_var`);
`Required` without a value → `ERR_VAR_UNSET{name}` (spec 5 maps `JUDGE`/`JUDGE_EFFORT` → `ERR_JUDGE_UNSET`, exit 2).
`calibration=true` pins `JUDGE=gpt-5.2`, `JUDGE_EFFORT=medium` **for declared vars only** and refuses caller-supplied
values for either (`ERR_CALIBRATION_PINNED{name}`). `Open` re-resolves from the stored effective vars with
`calibration=false`. The resolved copy carries `Refs`/`CmdRefs`/`Warnings` unchanged.

### 2.5 Commands and `cmds_sha256`
`ResolveCmds(w /* resolved */, workDir /* absolute */, lookPath, hash) ([]run.AllowedCmd, string, error)`: for every
`CmdDecl` in name order: `argv[0]` → if it contains `/`, `Join(workDir, argv[0])` (or as-is when absolute) and then
`lookPath` on that path (design §16.2: exists **and** executable, fail fast), else `lookPath(name)`; the result must be
absolute (`ERR_CMD_NOT_FOUND{name}` otherwise), written into `AllowedCmd.Argv[0]`; every
argv element (including the rewritten `argv[0]`) that names an existing regular file (absolute, or relative to
`workDir`) is sha256-hashed into `FileHashes` (absolute path → hash; always a non-nil map). `AllowedCmd{Name, Argv,
FileHashes, TimeoutMS, Env}` (`Env` nil when none — `omitempty` keeps the preimage stable). **Preimage:**
`run.MarshalCanonical([]AllowedCmd sorted by name)` (escape-off encoder, the one every fsm struct→JSON path uses) → sha256
hex; independent of declaration order (W4). `hash(path)` returns an error for missing or non-regular files (that is how
"names an existing regular file" is decided; `workflow.FileSHA256` is the real one). The runner
executes `AllowedCmd.Argv` verbatim. `VerifyCmds(allowed /* = snap.AllowedCmds, never re-resolved */, workDir, hash)`:
each pinned hash → `ERR_CMD_CHANGED{path, reason: mismatch|missing}`; an argv element that now resolves to a regular file
without a `FileHashes` entry → `ERR_CMD_CHANGED{path, reason: appeared}`. Ledger: absolute paths make the sha per-machine
and per-worktree; forks to another worktree re-consent (spec 3). Vars are configuration, not secrets: substituted values
appear in argv/consent lists in clear; secrets travel only via `env` pass-through names (values never persisted).

## 3. `gate` (implemented)
```go
type Git interface {
    Head(ctx) (string, error); RevParse(ctx, ref) (string, error)          // rev-parse --verify --quiet --end-of-options <ref>^{commit}; ValidRef: non-empty, no leading '-', no control/space
    IsAncestor(ctx, a, b) (bool, error); CommitCount(ctx, from, to) (int, error)
    Status(ctx) (clean bool, porcelain string, err error)                   // --porcelain=v2 --untracked-files=all
    Diff(ctx, from, to string, max int) (string, bool, error); WorkingDiff(ctx, max int) (string, bool, error)   // --no-ext-diff --no-textconv; Cut at a rune boundary
    CommonDir(ctx) (string, error)                                          // rev-parse --path-format=absolute --git-common-dir
    WorkTree(ctx) (string, error)                                           // git add -A into a scratch index (GIT_INDEX_FILE in $TMPDIR) + write-tree: content hash incl. untracked, excl. ignored
}
type Exec func(ctx, dir string, env []string, args ...string) (stdout, stderr []byte, code int, err error)   // RealExec: -c core.fsmonitor=false, GIT_TERMINAL_PROMPT=0, LC_ALL=C, GIT_* overrides scrubbed
func NewExec(dir string, x Exec) Git; type Fake struct{…}                 // Fake answers are keyed by arguments (Counts["from..to"], Ancestors["a b"], Refs[ref], Diffs["from..to"|"HEAD"]) and log calls
func TreeHash(head, workTree string) string                                // sha256(head + "\n" + workTree); pinned literal in G3
func ValidSHA(s string) bool  // ^[0-9a-f]{7,40}$|^HEAD$ — every sha-taking method checks it (ERR_GIT_REF)
```
Exit ≥ 2 → `ERR_GIT{op, exit}` with stderr; exit 1 is an answer only for `IsAncestor`. Gates:

| gate | pass | error |
|---|---|---|
| `findings_nonempty` / `findings_empty` | `len(Findings) > 0` / `== 0` | `ERR_NO_FINDINGS` / `ERR_FINDINGS_PRESENT` |
| `confirmed_nonempty` / `confirmed_empty` | `len(Confirmed) > 0` / `== 0` | `ERR_NO_CONFIRMED` / `ERR_CONFIRMED_PRESENT` |
| `commit_exists` | `FixEntryHead == ""` → `ERR_GATE_INAPPLICABLE`; `CommitCount(FixEntryHead, Head) > 0 && clean` | `ERR_NO_COMMIT`, Detail = count + porcelain + `WorkingDiff(MaxDetail)` via `CapDetail`; any Git failure → `ERR_GIT` gate error (evidence in the audit; recovery by fork — ledgered) |
| `all_fixed` / `bugs_remain` | `converge.AllFixed` / `!` | `ERR_BUGS_REMAIN` / `ERR_ALL_FIXED` |

## 4. `converge` (implemented)
```go
func AllFixed(s run.Snapshot) bool        // len(AllFound) > 0 && Unfixed == 0
type Result struct { Stop bool; Atom string; Class run.Outcome; Reason string }
type Predicate interface { Name() string; Class() run.Outcome; Evaluate(ctx, run.Snapshot) (Result, error) }
type CmdResult struct { Stdout, Stderr []byte; ExitCode int; Duration time.Duration }
type Runner interface { Run(ctx, name string, stdin []byte) (CmdResult, error) }     // spec 4's cmdexec.Guarded (name-only; pinned argv; audited)
func Validate(node *yaml.Node, cmdNames []string) error; func Parse(node *yaml.Node, r Runner) (Predicate, error)
func Payload(s run.Snapshot) []byte      // canonical snapshot with Vars → "sha256:<hex>" and NodeOutputs omitted — the stdin of every cmd atom / on_overflow / cmd kind
```
**Grammar:** an atom is either a bare scalar `all_fixed` | `no_fixation_progress`, or a one-key mapping: `{all_fixed:
true}`, `{no_fixation_progress: true}`, `{max_iterations: N>0}`, `{budget: {tokens: N>0}}`, `{cmd: <name>}`; composites
`{any: [..]}`, `{all: [..]}` (1..`MaxAtoms`=32 children), `{not: <predicate>}`; nesting depth ≤ `MaxDepth`=4. `all_fixed`
is legal only at the root or as a direct child of a root-level `any` (a give-up can never carry class `fixed` through
`all`/`not` — plan C3); `not` always classes `custom`. `Validate(node, cmdNames)`: `workflow.Parse` passes the declared
names (an empty non-nil slice when none), so an unknown atom name is `bad_convergence`. Anything else →
`ERR_BAD_CONVERGENCE{detail}` (Parse reason `bad_convergence`). The cmd atom's `reason` field is optional. Atoms: `all_fixed` (class `fixed`), `no_fixation_progress` (class `stalled`: `PrevUnfixed != nil
&& Unfixed >= *PrevUnfixed`; nil ⇒ false), `max_iterations: N` (class `overflow`: stop iff `Iteration+1 >= N`; `N: 5`
stops at `Iteration == 4`), `budget` (class `overflow`: `Tokens.Total() >= N`), `cmd` (class `custom`; stdout must be
exactly `{"stop": bool, "reason": string}` → `ERR_CMD_OUTPUT_INVALID`; non-zero exit → `ERR_CMD_FAILED{exit}`). `any`
returns the first firing child's `Result`; `all` fires only when every child fires (`Atom` = names joined by `+`, class
= first child's); `not` inverts. Errors abort at the first failing child (later atoms not evaluated).

## 5. `machine`

### 5.1 Consumed interfaces (implemented by spec 4)
```go
type Instructions struct { Text string; Input map[string]any; Untrusted []string; OutputSchema json.RawMessage }
type Diff struct { Text string; Truncated bool }                                  // Git.Diff(BaseSHA, head, MaxDiffBytes = 1<<20)
type ExecInput struct { Snap run.Snapshot; Node *workflow.Node; Diff Diff; StartIndex int /* st.NextIndex(key) */; Audit func(run.Event) error }
type NodeKind interface {
    Name() string; Info() workflow.KindInfo
    Instructions(snap run.Snapshot, node *workflow.Node, diff Diff, nonce string) (Instructions, error)
    Decode(raw json.RawMessage) (any, error)      // typed, DisallowUnknownFields, caps incl. len(Canonical) ≤ MaxPayload − 128 (spec 4 §4.1)
    Reduce(snap run.Snapshot, out any) (run.Delta, error)
}
type Executor interface { Execute(ctx, ExecInput) (json.RawMessage, error) }     // returns output already accepted by the kind's Decode
type Registry interface { Kind(name) (NodeKind, bool); Executor(name) (Executor, bool); Info() map[string]workflow.KindInfo; Mock() bool }
type Clock func() run.Time
type Sidecar interface {
    Write(runID, name string, b []byte) error      // O_CREAT|O_EXCL|O_NOFOLLOW 0600 in the run's 0700 dir; ERR_SIDECAR{reason: exists|path|name}
    Read(runID, name string) ([]byte, error)       // O_NOFOLLOW; ≤ MaxPayload else ERR_SIDECAR{reason: too_large}; ERR_SIDECAR{reason: missing}
    List(runID string) ([]string, error)           // names (spec 3 Export/Fork)
}
// Sidecar names: ^[a-z][a-z0-9._-]{0,63}$, never `audit.*` or `lock`; runID must pass run.ValidateRunID. FS impl under <root>/.metareview/runs/<id>/; Mem for tests.
```
`Audit` appends immediately (durable) and rebinds the machine's state; it returns store errors so the executor stops.
Executors number `llm_call.Index` from `StartIndex` (the fold's next index for the key), so an interrupted execution
resumes with a continuing index and its earlier spend stays audited. `Execute` is never retried by the machine inside
one `Advance`; a failure → `executor` pseudo-gate (§5.4 step 5), except `ctx.Err() != nil` (interrupt), which `Advance`
returns unchanged so the next `Advance` resumes from `StartIndex`.

### 5.2 Deps and API
```go
type Deps struct {
    Store run.RunStore; Sidecar Sidecar; Kinds Registry; Git func(workDir string) gate.Git
    Runner func(allowed []run.AllowedCmd, workDir, runID string, audit func(run.Event) error) converge.Runner
    Clock Clock; LookPath func(string) (string, error); FileHash func(string) (string, error)
    Workflows func(name string) ([]byte, error); ReadFile func(string) ([]byte, error); Nonce func() string
    MockLoad func(dir string) (hash string, err error); Terminal func(ctx, View) error   // spec 3's runs.jsonl record; nil → no-op
}
type InitOptions struct { Workflow string; RunID string; Vars map[string]string; Base string; RepoMode string; AllowCustomCmds string; Calibration bool; MockDir string; GoldensPath string; WorkDir, RepoRoot string }
type OpenOptions struct { Repair bool }
// Typed string sets: Status ∈ {ADVANCED, NEEDS_INPUT, DONE, STOPPED, GATE_FAILED}; NextAction ∈ {advance, record, none}; RecordOptions.Kind ∈ {node-output, tokens, event} — exported constants.
func Init(ctx, Deps, InitOptions) (*Machine, error); func Open(ctx, Deps, runID string, OpenOptions) (*Machine, error)
func (m *Machine) Advance(ctx) (AdvanceResult, error); func (m *Machine) Record(ctx, RecordOptions) (RecordResult, error); func (m *Machine) View() View
type AdvanceResult struct { Status string; From, To run.State; Gate *run.GateData /* first failing */; Outcome run.Outcome; StopReason string; NeedsInput *NeedsInput; Warnings []string /* warn events appended by this call */; Untrusted []string /* "gate.detail","warnings","stop_reason","error.detail" when non-empty — every Detail may carry repo/third-party bytes */; ExitCode int; RunID string }
type NeedsInput struct { Node string; Kind, Exec, Model, Effort string; Instructions Instructions; Record string }
type RecordOptions struct { Kind string /* node-output|tokens|event */; Node string; Data json.RawMessage; Replace bool; Name string }
type RecordResult struct { Seq int64; Type run.EventType; Key string }
type View struct { RunID, Workflow string; Snapshot run.Snapshot; Node *NodeView; NextAction string /* advance|record|none */; Torn bool; FailedGate *run.GateData /* last gate{passed:false} before a failed transition */ }
type NodeView struct { Name, Kind, Exec string; HasOutput, Applied bool }
```

### 5.3 `Init`
1. Load YAML: name → `Deps.Workflows` (`ERR_WORKFLOW_NOT_FOUND`); path (`/` or `.yaml`) → `ReadFile`, ≤ 256 KB
   (`ERR_WORKFLOW_TOO_LARGE`). `Parse(raw, Options{Kinds: Kinds.Info()})`; `Resolve(vars, calibration)`.
   `RepoMode` override must be `""` or `enforcing` (tighten-only; `advisory` over a workflow's `enforcing` → `ERR_BAD_REPO_MODE`).
   Git failures during `Init`/`Advance` outside gates are returned as the `ERR_GIT`/`ERR_GIT_REF` error unchanged (retryable);
   only gates convert Git failures into gate errors.
2. `ResolveCmds`; `len(Cmds) > 0 && AllowCustomCmds != sha` → `ERR_CMDS_NOT_ALLOWED{sha}`, Detail = the printed list
   (name, argv with pinned/unpinned elements marked, file hashes, timeout, env **names**).
3. Git in `WorkDir`: `CommonDir` must equal `RepoRoot`'s (`ERR_WORKDIR_FOREIGN`); `Head`; `RevParse(Base||"HEAD")` →
   `BaseSHA`; `Status` + `WorkTree` → `TreeHash`.
4. Goldens: `ReadFile` ≤ 512 KB, JSON array of `run.Golden` with `DisallowUnknownFields`, ≤ `MaxGoldens` → else
   `ERR_GOLDENS_INVALID{path}`.
5. Mock: `MockDir` (made relative to `RepoRoot`) → `MockLoad` → `Mock = rel + "#" + hash[:16]` (`ERR_MOCK_INVALID`);
   `Kinds.Mock()` must equal `MockDir != ""` (`ERR_MOCK_MISMATCH`).
6. `runID` ← `RunID` or `run.RunID(w.Name, Clock().Time)`; **`run.Create` first**, then `Sidecar.Write(runID,
   "workflow.yaml", raw)` (failure → the error; the run exists without a sidecar and `Open` reports `ERR_SIDECAR`); then
   under the lock append `tree{Head, TreeHash, Status: CapDetail(porcelain)}` and one `warn{WORKFLOW_WARNING}` per
   `w.Warnings`. A crash between `Create` and the sidecar write leaves a run that `Open` reports as `ERR_SIDECAR{missing}`;
   spec 5 documents manual deletion of `.metareview/runs/<id>/` (no automatic repair).
   `init` carries no `State`/`Iter`/`Node` stamps (run's `init_stamp` rule); `Mock` stamp on every later event.
7. Return; `View().NextAction == "advance"`.

### 5.3b `Open(ctx, deps, runID, opts)`
1. `Store.Lock(runID)` for the duration. `log ← EventsWithLines` (`ERR_RUN_NOT_FOUND` passes through). If `log.Torn`: with
   `opts.Repair` → `RepairTail` (offset 0 → run removed → `ERR_RUN_NOT_FOUND{detail: "run removed; torn bytes in
   runs/.torn/"}`), reload, then append `warn{AUDIT_TORN_LINE_DROPPED, Detail: "<n> bytes dropped after seq <s> from
   audit.jsonl"}`; without `Repair` → the machine opens with `View.Torn = true` and every `Advance`/`Record` returns
   `ERR_AUDIT_TORN`. `opts.Repair` on a clean log → `ERR_AUDIT_NOT_TORN` passes through.
2. `st ← FoldFull`, `st.ChainHead ← log.Head`; `raw ← Sidecar.Read("workflow.yaml")`; `sha256(raw) == snap.WorkflowHash`
   else `ERR_WORKFLOW_CHANGED{expected, got}`; `Parse(raw, Options{Kinds: Kinds.Info()})` (a vocabulary change in a newer
   binary surfaces here as `ERR_WORKFLOW_INVALID` — the "older-binary run" signal; `list`/`export` still work);
   `Resolve(snap.Vars, false)`; `VerifyCmds(snap.AllowedCmds, WorkDir, FileHash)`; `Mock`: split on the **last** `#`, resolve
   `rel` against `snap.RepoRoot`, `MockLoad` hash vs stored, `Kinds.Mock() == (snap.Mock != "")`, else `ERR_MOCK_MISMATCH`;
   build `runner ← Deps.Runner(snap.AllowedCmds, WorkDir, runID, audit)` and, when `w.Convergence != nil`, `pred ←
   converge.Parse(w.Convergence, runner)`.
`Advance` and `Record` re-run steps 1–2 under their own lock (no cached state is trusted across calls).

### 5.4 `Advance`
```
1  §5.3b steps 1–2 (lock, fold, ChainHead, sidecar/cmds/mock integrity, runner, predicate). "append X" ≡ st, snap ←
   Store.Append(runID, st, X) (rebind; the Audit closure is the same path). Any store error aborts with that error;
   nothing is rolled back. Emitter caps: ConvergeData.Reason / warn.Detail CapText(MaxText); ConvergeData.Atom ≤ MaxShort
   (guaranteed by MaxDepth/MaxAtoms); GateError.Detail and tree.Status CapDetail (+ flags).
2  snap.Outcome != "": if Outcome == overflow && w.OnOverflow != "" && !OverflowHandled → step 9b; else Deps.Terminal(ctx,
   View()) (idempotent — spec 3 dedups by run id) then ERR_RUN_TERMINAL.
3  (folded into step 1)
4  head ← Git.Head; porcelain ← Status; wt ← WorkTree; h ← TreeHash(head, wt); node ← w.NodeFor(state); mode ← snap.RepoMode
   if snap.TreeHash == "": append tree{head, h, porcelain}                         (baseline: forks from the initial state, Init crash)
   else if h != snap.TreeHash && (node == nil || node.Kind != agent-edit):
       advisory  → append warn{UNSANCTIONED_EDIT, Detail: porcelain}; append tree{head, h, porcelain}
       enforcing → append gate{Name:"repo_mode", Passed:false, Error:{ERR_UNSANCTIONED_EDIT, Detail: porcelain}} → step 8   (NO tree: a crash re-detects)
   else if h != snap.TreeHash: append tree{head, h, porcelain}
5  if node != nil:
       k ← Key(node, iter); diff ← Git.Diff(BaseSHA, head, MaxDiffBytes)   (only when needed: output absent, or fork)
       if NodeOutputs[k] absent:
           fork: out, err ← Executor.Execute(ExecInput{snap, node, diff, st.NextIndex(k), audit})
                 ctx.Err() != nil → return err (resumable); other err → append gate{Name:"executor", Passed:false, Error:{ERR_EXECUTOR_FAILED, Detail: CapDetail(err)}} → step 8
                 append node_output{out}
           host: ins, err ← kind.Instructions(snap, node, diff, Nonce()); err → ERR_INSTRUCTIONS_FAILED (nothing further appended)
                 if no needs_input event exists for k: append needs_input
                 return NEEDS_INPUT (exit 3)
       if !Applied[k]: out ← Decode(output); delta ← Reduce(snap, out); error → append gate{Name:"node_output", Passed:false,
                       Error:{ERR_NODE_OUTPUT_INVALID, Detail}} → step 8; else append delta_applied{delta, OutputHash(output)}
                       (a store rejection of delta_applied is treated the same way: node_output pseudo-gate → step 8)
6  chosen ← nil; failures ← [] (GateData in evaluation order)
   if w.TerminalFor(state) != nil:                                            (loop boundary — order-independent)
       tt ← w.TerminalFor(state); err ← gate(tt.Gate)(snap); append gate; pass → chosen ← tt else failures += err
       if chosen == nil:
           r, err ← pred.Evaluate(snap)                                        (always when tt failed — bounded loops)
           err → append gate{Name:"converge", Passed:false, Error:{ERR_CONVERGE_FAILED, Detail}} → step 8
           r.Class == fixed → same, ERR_CONVERGE_FAILED{reason: fixed_class}   (defense in depth over converge's placement rule)
           append converge{Atom: r.Atom, Class: r.Class, Stop: r.Stop, Reason}
           if r.Stop: chosen ← synthetic {From: state, To: tt.To, Gate: r.Atom, Outcome: r.Class}
       if chosen == nil: for t in Outgoing(state) except tt: err ← gate(t.Gate)(snap); append gate; pass → chosen ← t; break; else failures += err
   else: for t in Outgoing(state): err ← gate(t.Gate)(snap); append gate; pass → chosen ← t; break; else failures += err
7  (every gate evaluation is appended when evaluated, in order)
8  chosen == nil: first ← the pseudo-gate that sent us here, else failures[0]; append transition{From: state, To: failed, Gate:
   first.Gate, Outcome: failed, Head: head}; Deps.Terminal(ctx, View()); return GATE_FAILED with Gate = first, exit 1
9  append transition{From, To, Gate, Outcome, Loop, ToKind, Head}
9b if Outcome == overflow && OnOverflow != "" && !OverflowHandled: res, err ← runner.Run(OnOverflow, converge.Payload(snap));
   append overflow_handler{Name, Argv: snap.AllowedCmds[name].Argv, InputHash: sha256(payload), Stdout/Stderr: CapText(MaxDetail/MaxStderr)+flags,
   ExitCode (−1 when err), DurationMS, Error: code}; err or exit≠0 → also warn{OVERFLOW_HANDLER_FAILED}   (at-least-once: a crash between the
   runner's cmd_call and this append re-runs the command)
   if Outcome != "": Deps.Terminal(ctx, View()) (its error is returned; the transition is already durable)
   return per §5.7 — StopReason = "<Atom>: <Reason>" (the fold keeps only Atom in Snapshot.StopReason)
```
**Stamps** on every event appended by `Init`/`Advance`/`Record`/`Audit`: `At ← Clock()`, `State ← current` (`From` for
transitions), `Iter ← current` (`N+1` on the loop transition), `Mock ← snap.Mock != ""`, `Node ← node.Name` for
node-scoped events (`needs_input`, `node_output`, `delta_applied`, `llm_call`); `cmd_call`/`overflow_handler` carry no
`Node`. A non-mock run driven by a mock registry is refused at step 3, so `MockTainted` stays false on real runs (M8).

### 5.5 `Record` (takes the run lock; `tokens`/`event` allowed on terminal runs, `node-output` not)
- `node-output`: not terminal (`ERR_RUN_TERMINAL`); state has a node and `Node == node.Name` (`ERR_NODE_MISMATCH`); exec
  `inline|subagent` (`ERR_NODE_NOT_HOST`); `!Applied[k]` (`ERR_NODE_OUTPUT_APPLIED`); `NodeOutputs[k]` absent unless
  `Replace` (`ERR_NODE_OUTPUT_EXISTS`); `Decode` ok (`ERR_NODE_OUTPUT_INVALID`, nothing appended); append `node_output`.
- `tokens`: `run.TokenTotals`, `DisallowUnknownFields`, no negative field → else `ERR_RECORD_TOKENS`; append `tokens`.
- `event`: `Name` ~ `^[a-z][a-z0-9_-]{0,63}$`, not a run event type, not `mrv_*` → else `ERR_RECORD_NAME{reason:
  syntax|event_type|reserved}`; `Data ≤ MaxPayload − 128` (`ERR_RECORD_TOO_LARGE`); append `record{name, data}`.

### 5.6 `Status` input contract — unchanged (every bug in `AllFound`; `Reduce` returns exactly that set).

### 5.7 Outcomes → status → exit
| outcome | Status | exit |
|---|---|---|
| `""` | `ADVANCED` | 0 |
| host node awaiting output | `NEEDS_INPUT` | 3 |
| `fixed`, `clean` | `DONE` | 0 |
| `reviewed` | `DONE` | 1 |
| `stalled`, `overflow`, `custom` | `STOPPED` (`StopReason` = converge Reason) | 1 |
| `failed` | `GATE_FAILED` (`Gate` set) | 1 |

## 6. Errors (`errs.Error`) and warn codes
`ERR_WORKFLOW_INVALID{reason, at}`, `ERR_WORKFLOW_NOT_FOUND`, `ERR_WORKFLOW_TOO_LARGE`, `ERR_VAR_UNSET{name}`,
`ERR_VAR_UNKNOWN{name}`, `ERR_CALIBRATION_PINNED{name}`, `ERR_CMDS_NOT_ALLOWED{sha}`, `ERR_CMD_NOT_FOUND{name}`,
`ERR_CMD_CHANGED{path, reason}`, `ERR_GIT{op}`, `ERR_GIT_REF{ref}`, `ERR_WORKDIR_FOREIGN`, `ERR_GOLDENS_INVALID{path}`,
`ERR_MOCK_INVALID{dir}`, `ERR_MOCK_MISMATCH`, `ERR_BAD_REPO_MODE`, `ERR_SIDECAR{reason}`, `ERR_RUN_TERMINAL`,
`ERR_WORKFLOW_CHANGED{expected, got}`, `ERR_NODE_MISMATCH`, `ERR_NODE_NOT_HOST`, `ERR_NODE_OUTPUT_APPLIED`,
`ERR_NODE_OUTPUT_EXISTS`, `ERR_NODE_OUTPUT_INVALID`, `ERR_INSTRUCTIONS_FAILED`, `ERR_RECORD_NAME{reason}`,
`ERR_RECORD_TOKENS`, `ERR_RECORD_TOO_LARGE`, `ERR_UNSANCTIONED_EDIT`, `ERR_EXECUTOR_FAILED`, `ERR_CONVERGE_FAILED`,
`ERR_GATE_INAPPLICABLE`, gate codes (§3), `ERR_CMD_OUTPUT_INVALID`, `ERR_CMD_FAILED`, `ERR_CMD_TIMEOUT`,
`ERR_CMD_NOT_ALLOWED` (Runner), and `run` store codes unchanged. Warn codes: `UNSANCTIONED_EDIT`,
`AUDIT_TORN_LINE_DROPPED` (Detail literal: `"<n> bytes dropped after seq <s> from audit.jsonl"` — the run spec defers to
this), `OVERFLOW_HANDLER_FAILED`, `WORKFLOW_WARNING`. `Open(Repair)` when `RepairTail` removed the run (offset 0) returns
`ERR_RUN_NOT_FOUND{detail: "run removed; torn bytes in runs/.torn/"}` and appends nothing.

## 7. Tests (100% statements; TDD). Authority = hand-written expectations and literal pins; goldens are regression-only
behind `FSM_MACHINE_UPDATE_GOLDEN=1` with the run package's "drift ≠ regenerate" comment.

| pkg | rows |
|---|---|
| workflow | W0 order row: a document with an unknown top-level key **and** `version: 2` → `unknown_key` (first failure in table order). W1 both shipped YAMLs + the §2.2 example + a mapping-form twin of review-loop (order preserved, `*→failed` ignored, `->` and `→`): assert `Transitions`, `Nodes` (exec defaulted for `fix`/`verify`), `Cmds` (`Timeout 30s`, default `60s`), `Refs`/`CmdRefs` literals, `Hash` literal, `Warnings` empty; W2 one fixture per reason **and per sub-rule** (each one edit from a valid base): `bad_cmd` ×8 (argv empty / non-string / empty element / timeout non-integer / 0 / 3601 / name regex / > MaxAllowedCmds / > MaxArgv), `bad_env` one per reserved literal and prefix + regex + duplicate + count, `bad_var` ×3, `bad_state` ×3 (incl. `judge`), `failed_reserved` ×3 (undeclared / has node / in a transition), `duplicate_transition` (same `(from, gate)`, different `to`), `loop_terminal` ×2, `unknown_cmd` ×3, `cmd_without_kind` ×2, `unknown_var` ×3 (node, cmd, list param), `bad_outcome` ×2 (`great`, `failed`), `bad_yaml` ×2 (malformed, duplicate key), `bad_version` (`one`), `initial_terminal`, `bad_params`, `missing_kinds`; acceptance boundaries: timeout 1 and 3600, 32-char state, `MaxVars`/`MaxEnv`/`MaxArgv`/`MaxAllowedCmds` at cap; assert `reason` + `at`; W3 `Resolve`: `$JUDGE`/`$JUDGE_EFFORT` prefix pair → `Model=="a"`, `Effort=="b"`; caller value beats `Default`; list params substituted, non-string list elements and nested maps untouched (literal asserts), `$1`/`${X}` left literal; `Refs`/`CmdRefs` from a list param; `$$` literal; `$JUDGEX` → `ERR_VAR_UNKNOWN`; calibration pins asserted as literals; caller `FOO` → `ERR_VAR_UNKNOWN`; required unset; calibration refuses caller `JUDGE` **and** `JUDGE_EFFORT`; calibration on a workflow without `JUDGE` var is a no-op; re-resolve of stored pinned vars succeeds; argv substitution; W4 `ResolveCmds`: fake lookPath/hash; `argv[0]` rewritten absolute (`bash` → `/bin/bash`, `./s.sh` → `<workDir>/s.sh`), relative lookPath result → `ERR_CMD_NOT_FOUND`; closure over `["bash","./s.sh"]` + absolute path; `/bin/bash` itself appears in `FileHashes`; non-nil empty map; **hand-authored preimage** (`testdata/cmds-preimage.json` + `.sha256` from `shasum`) with two cmds declared out of order, `TimeoutMS 1500`, `Env` set; one-byte edit → different sha; `VerifyCmds` mismatch/missing/appeared; a directory argv element is not hashed; **no re-resolution**: after pinning `/bin/bash`, point `lookPath` elsewhere and edit the pinned file → `ERR_CMD_CHANGED{/bin/bash, mismatch}`, and the inverse (pinned intact, lookPath moved) → no error; non-absolute `workDir` refused; W5 `VarsReferencedBy` (node ∪ cmd, sorted, resolved copy), `Outgoing`, `IsTerminal` incl. `failed`, `LoopTransition`, `TerminalFor`, `loop_without_clean_exit` warning |
| machine | M0 fakes: `gate.FailingFake{Fake; FailAt string}` (exported from `gate`, fails exactly one method) drives per-call-site `ERR_GIT{op}` rows for every Git call in `Init`/`Advance`; a counting store fails `Lock`, `EventsWithLines`, or append #N; seam row: `Fake.Diffs["<base>..<head>"] = "D"` with a small `MaxDiffBytes` → the fake kind/executor observe `diff.Text == "D"`, `Truncated == true`, `nonce == "n1"`. M1 `Init`: hand-written expected sequence `[init(no stamps), tree(State=initial, Iter 0), warn?]` with every `InitData` field asserted literally (embedded + path workflows; `workflow.yaml` sidecar bytes == raw; `Create` before sidecar observed via a fake store/sidecar call log; `ERR_RUN_EXISTS` leaves the victim's sidecar intact); `ERR_WORKFLOW_NOT_FOUND`, `ERR_WORKFLOW_TOO_LARGE`; goldens ok/unknown field/over cap/over bytes; consent list as a hand-written literal (pinned/unpinned marks, env **names** only, no process env values) + sha in Detail, wrong sha, no cmds; `ERR_MOCK_INVALID`; `RepoMode` override `enforcing` accepted / `advisory` over `enforcing` refused; `RevParse` base (`main` → sha); `ERR_WORKDIR_FOREIGN`; `ERR_BAD_REPO_MODE`; mock hash pinned + `Kinds.Mock()` mismatch; unknown `--var`; M2 `Advance` on both shipped workflows with a fake Registry: hand-written expected event-type sequences per path (`review-loop` clean/reviewed; `sdlc-loop` clean at discover, clean at adjudicate, fixed after 1 iteration, loop once then fixed), literal asserts on transition fields, `needs_input` once across `advance, record tokens, advance` and again at `discover@1`, `View.NextAction` per step; goldens regression-only; M3 gate failure: two failing gates → `Gate` is the first in evaluation order and `transition.Gate` names it; loop-boundary variant (tt and the loop gate both fail → tt named); two passing gates → the first taken; `ERR_INSTRUCTIONS_FAILED` (needs_input already present stays, nothing further); ctx cancelled during `Execute` → error returned, no pseudo-gate, next `Advance` resumes with `StartIndex`; `ERR_GATE_INAPPLICABLE`; executor error → `executor` pseudo-gate with earlier `llm_call`s kept and `StartIndex` honoured on the next fork (interrupted-execution fixture: pre-seeded `llm_call` index 0, executor asserts `StartIndex == 1`); decode error / Reduce error / rejected `delta_applied` (status subset from a fake cmd kind) → `node_output` pseudo-gate; M4 loop: cumulative regression (iter 3 fixes its own bug, 7 remain: loop taken, `AllFound == 8`, `Unfixed == 7`, all 8 statuses, not `fixed`); **gate-first**: `max_iterations: 1` with all bugs fixed at verify → `fixed`, zero `converge` events; negative control one bug left → `converge{max_iterations}` → `overflow`; `stalled` via nil-then-plateau and via regression (`Prev 3 → Unfixed 5`); `budget` via `llm_call` tokens and via `record tokens`; `custom` via cmd atom; converge error → `converge` pseudo-gate and no loop taken; `fixed_class` guard via a fake predicate; user workflow whose terminal gate is `confirmed_empty` (not `all_fixed`) with all bugs fixed and findings present → convergence evaluated (bounded) — the `max_iterations` stop fires; emitter caps: a 5 KB cmd-atom reason → `converge.Reason` and `StopReason` capped; overflow handler once, `overflow_handler` fields literal, failure warn, **not** run for `stalled`/`custom`, resumed after a crash (fixture: terminal overflow run without handler → `Advance` runs it, then `ERR_RUN_TERMINAL`); M5 tree: identical porcelain + different `WorkTree` → advisory warn (+ tree) vs enforcing `repo_mode` gate with **no** tree (a second `Advance` re-detects); porcelain ≈ 5 KB → warn Detail capped at `MaxText`, tree Status intact; 70 KB porcelain → `tree.Status` capped with flag; agent-edit exempt; baseline `tree` appended when `TreeHash == ""` (fork-from-initial fixture); `tree` only on change (count); M6 `Record` refusals per code and sub-reason (`syntax`, `event_type` (`transition`), `reserved`), `ERR_RECORD_TOKENS` on unknown field and on `-1`, `Replace`, terminal `tokens` allowed, `ERR_NODE_OUTPUT_INVALID` leaves `Events` byte-identical; M7 `Open`: `ERR_WORKFLOW_CHANGED` via sidecar edit; embedded bytes replaced by a workflow with different transitions while the sidecar is intact → `Advance` follows the **sidecar's** transitions; `ERR_CMD_CHANGED`; `ERR_MOCK_MISMATCH` via scenario edit and via registry mismatch; torn → `ERR_AUDIT_TORN`; `Repair` → warn Detail literal + fold ok; `Repair` at offset 0 → `ERR_RUN_NOT_FOUND`; `ERR_SIDECAR{missing}`; M8 stamps: every event's `At` equals the injected clock sequence, `State/Iter/Mock/Node` per §5.4 tail (`cmd_call` has no Node), non-mock runs never carry `Mock: true` (`MockTainted == false`), mock runs carry it on every non-init event; M9 §5.7 table incl. `StopReason` ("atom: reason"), `Untrusted` list, `Deps.Terminal` called for every terminal outcome incl. `failed`, again on a later `Advance` of a terminal run (idempotency is spec 3's), and on the 9b resume path; `Terminal` error returned with the transition durable; `ERR_AUDIT_FULL` surfaced; a counting store that fails append #N for every N of the happy sequence returns the error unchanged; FS `Sidecar`: symlink refused, exists refused, mode 0600, missing run → `ERR_SIDECAR` |

## 8. Ledger
- `cmds:` single top-level declaration referenced by name; per-cmd `timeout`/`env` are consent-covered (design §16 inline argv retired).
- `failed` reserved; `duplicate_transition` on `(from, gate)`; loop safety reasons; `bad_state` (`judge` reserved for spec 5); `bad_env` reserved names.
- Loop boundary is order-independent: `TerminalFor` gate first, convergence only when `!AllFixed`, then the loop gate and remaining transitions (C3 gate-first, made structural).
- Converge errors are the `converge` pseudo-gate; enforcing edits, executor and decode failures are `repo_mode`/`executor`/`node_output` pseudo-gates; the failed transition names the first failing gate.
- `needs_input` once per key; `tree` at `Init` and on change (content-aware `WorkTree`; agent-edit states may emit one per advance while the agent edits — accepted).
- `commit_exists` = `FixEntryHead..HEAD` + `ERR_GATE_INAPPLICABLE` (SCP3-5); Git failures inside gates are gate errors (recovery by fork) — accepted.
- `Open` verifies the run's `workflow.yaml` sidecar (written after `Create`, `O_EXCL`); forks copy the parent's sidecar (spec 3 r2 obligation; also `Export` includes it).
- `ERR_RECORD_NAME` narrows locked C15 (reserved names refused; plan E13's `record transition` row becomes an `ERR_RECORD_NAME` row in spec 5).
- `machine` does not import `workflows` (plan §1.1 had the edge); the CLI passes `Deps.Workflows`.
- `Options.Kinds` is required — no second copy of the kind table.
- Overflow handler audited twice by design (`cmd_call` by the runner + `overflow_handler` by the machine); `on_overflow` resumable after a crash.
- Design §9's atom params (`window: 2`) dropped: cmd atoms take no params (the command reads the payload).
- Vars are configuration, not secrets (argv/consent lists show them); secrets use `env` pass-through.
- `sdlc-loop` gained `clean` exits at discover and adjudicate (D1 mitigation); comment shows the r2 escape-hatch form.
- Reassigned: CMP-15/ARC-21 → spec 3 (run spec §12 updated); `ERR_JUDGE_UNSET` mapping, env/consent docs, `Untrusted` marking, manual deletion of a sidecar-less run, `WorkTree` object writes (scratch index writes loose blobs into `.git/objects`; gc reclaims) → spec 5 docs.
- `JUDGE_EFFORT: {required: true}` in both shipped workflows (plan §6 shipped `{default: medium}`; design §17 wants no hardcoded effort).
- C20's `ERR_EXEC_UNSUPPORTED` at Parse time is `ERR_WORKFLOW_INVALID{exec_kind_mismatch}`; the runtime code keeps C20's name (spec 4).
- Fork obligation restated: the child's sidecar is the **child's** resolved workflow bytes (whose sha equals the child's `WorkflowHash`), copied from the parent only when unchanged; forks rebaseline `tree` on the child (spec 3 r2).
- `nothing_found`/`nothing_confirmed` gates added (9 built-ins; C23 said 7): iteration-0 clean exits that refuse once any bug is known.
- Convergence evaluated whenever the terminal gate fails (loops are always bounded by their predicate); `not` classes `custom`; `all_fixed` placement rule.
- `WorkTree` disables `core.excludesFile`/`core.attributesFile`; residual: `.git/info/exclude` and clean filters remain agent-writable (spec 5 enforcing caveat).
- Consent, `--accept-workflow-change`, `--mock-ai`, `--calibration`, and the `RepoMode` override are agent-satisfiable (run spec §1.5 "process guarantees for a cooperating agent"); `RepoMode` override is tighten-only.
- `workflows/` embed package (`Names()`, `Read(name)`) is owned here; implemented in M0.

## 9. `run` amendments owned here (implemented; run spec §11 lists them)
- `AllowedCmd{Name, Argv, FileHashes, TimeoutMS int64 (omitempty), Env []string (omitempty)}`; `MaxEnv = 16`; `withinCaps` checks `Env` count and names (`MaxShort`).
- `tokens`/`llm_call` with any negative counter → `FoldError{Reason: tokens_negative}`; `TokenTotals.Negative()`.
- `FoldState.NextIndex(key) int` exported for `ExecInput.StartIndex`.
- `run.MarshalCanonical` exported.
- Repair-warn Detail literal is this spec's (§6).
