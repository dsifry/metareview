# metareview 0.9.0 — TDD build & orchestration plan

> **Status:** READY FOR GO/NO-GO. Companion to
> [`2026-08-26-metareview-0.9.0-fsm-enhancements.md`](2026-08-26-metareview-0.9.0-fsm-enhancements.md)
> (the design spec). This document locks the interfaces, corrects the spec where it is wrong
> about the current binary, fixes the CLI contract the spec left open, and sequences the build
> so that independent packages can be written in parallel — each test-first, each under a
> hard 100% coverage gate.
>
> **Inputs:** the design spec; the pi session log that produced it
> (`harnesseval` session `2026-08-25T00-52-28…01a03667`); the harnesseval Python that is the
> port spec (`harnesseval/{judge,adjudicate,sdlc_loop,usage,model_router}.py`); the current
> Go binary on branch `fsm-enhancements` @ `57221cd`.

---

## 0. Corrections to the spec (lock these before code)

| # | Spec says | Reality / decision |
|---|---|---|
| C1 | "The Go binary already has 100% test coverage as a hard quality gate" (§18) | **Not measured, and not 100%.** No coverage gate exists in `tests/` or CI, and no `_test.go` was ever deleted — the belief came from the behavioral shell suite (`tests/go/*.sh`), which `go test -cover` cannot see. Measured 2026-08-26 on `57221cd` with **unit tests + the full shell suite** run against a `-cover`-instrumented binary (`GOFLAGS=-cover GOCOVERDIR=… bash tests/run-all.sh`): **86.3% total**; lowest `markdown` 70.0%, `learnsource` 70.8%, `contextpack` 76.1%, `knowledge` 77.8%, `tasksource` 79.2%, `cmd/metareview` 80.4%, `artifactreview` 80.4%; highest `integration` 100%, `reviewers` 97.2%, `githubcontext` 95.9%. (Unit tests alone: several packages at 0–30%.) **Decision:** M0 adds the gate that was believed to exist — combined unit+behavioral coverage via `GOFLAGS=-cover` (no script changes needed) — enforcing **100% on every `internal/fsm/...` package** and a **recorded per-package floor** (today's numbers) on legacy packages. Whether 0.9.0 also lifts legacy to 100% is §7 Q2. |
| C2 | `still-present` outputs `{still_present, confidence}` (§6) | The Python prompt returns `{reasoning, still_present}` only, and **fails closed** (parse failure ⇒ still present). **Decision:** the Go prompt asks for `confidence` too (this prompt is not part of Martian calibration, so it is safe to extend); the parser tolerates its absence (`confidence: 0`), and fail-closed is preserved and tested. |
| C3 | `bugs_remain` / `all_fixed` are listed as gates *and* `all_fixed` as a convergence atom | At the `verify` boundary the **convergence predicate** decides; `all_fixed` and `bugs_remain` are the two named outcomes of that evaluation, not separate gates. Both names stay valid in YAML (`verify→done: {gate: all_fixed}`, `verify→discover: {gate: bugs_remain}`) and resolve to the predicate result. |
| C4 | `executor: $SESSION` on the fix node (§5) | Superseded by `exec: inline`. Dropped from the shipped YAML. |
| C5 | CLI is written as `mrv fsm …` | The binary is `metareview`. All commands are `metareview fsm …`; docs mention `alias mrv=metareview` once. |
| C6 | "default workflows" (plural) ship built-ins only (§13, §16) | Only `sdlc-loop` was ever defined. **Decision (needs confirmation, §7):** ship two — `sdlc-loop` (the full loop) and `review-loop` (`discover→adjudicate→done`, read-only: the existing artifact/task-done review expressed as an FSM). |
| C7 | `advance` "returns NEEDS_INPUT for the agent to satisfy" — one sentence, no contract | Defined in §3.4 below: JSON shape, exit code 3, and the `record node-output` hand-back. |
| C8 | Exit codes and JSON output shapes: not specified | Defined in §3.5 below. |
| C9 | Token budget "source" is an open question (§15.4) | Judge calls self-report (the binary made them); the agent records session totals via `fsm record tokens`. **The Python silently drops judge tokens** (`_score_tin` is a dead key) — the Go port counts them, and a test asserts it. |
| C10 | `mrv fsm run` "optional convenience wrapper" (§12) | Not built. `fsm state` output already includes `next_action`; the skill doc is the loop skeleton. |
| C11 | Resume when `--from` is *earlier* than the current state is described only for "logic fix" and "judge swap" | Formalized: `--from S` truncates the snapshot to the last checkpoint at S-entry (replaying node outputs for states before S from `audit.jsonl`), and continues. Node outputs after S are discarded, not reused. Tested per kind (§4.4). |
| C12 | Overflow end state unspecified | Safety-stop (`max_iterations`, `budget`) ends in `done` with `stop_reason` set and `overflow: true`; `on_overflow` runs exactly once, after the transition is persisted. Normal convergence (`all_fixed`, `no_fixation_progress`) never runs it. |

Adopted verbatim from the session (not restated in the spec): *frame this as "deterministic
workflow structure, auditable/swappable LLM calls" — never "deterministic results."* All user-facing
text (help, skill, docs, CHANGELOG) follows that wording.

---

## 1. Architecture

### 1.1 Package layout (all new code under `internal/fsm/` so the coverage gate can scope to it)

```
internal/fsm/
  run/        Event, Snapshot, Finding/Bug/Verdict wire types, RunStore iface, JSONL + memory stores
  workflow/   YAML schema → Workflow; var resolution; validation; ERR_JUDGE_UNSET; cmd declare+verify
  gate/       deterministic gates + error codes; Git iface (real via exec, fake in tests)
  converge/   Predicate atoms, any/all/not, ParsePredicate, CmdPredicate
  cmdexec/    guarded shell runner: opt-in, timeout, schema-validated stdout, audited
  judge/      Judge iface; prompts (match/adjudicate/still-present/dedup); parsers; Anthropic + OpenAI-compat providers over an injected Doer; token accounting; MockJudge
  kind/       Kind iface + Registry; review-lenses, match-then-adjudicate, still-present, agent-edit; CmdKind; MockKind
  mockai/     scenario-file loader (input-hash → scripted output) used by MockJudge/MockKind
  machine/    Init / Advance / Record / Resume; NEEDS_INPUT; convergence at the loop boundary; on_overflow
  cli/        `metareview fsm …` arg parsing + JSON output + exit codes; Run(args, stdin, stdout, stderr, Deps) int
cmd/metareview/main.go   one new branch: `fsm` → fsmcli.Run(...)  (main.go stays thin; it is 0% covered today and stays out of the gate)
workflows/               shipped default workflows (sdlc-loop.yaml, review-loop.yaml)
skills/fsm/SKILL.md, commands/fsm.md, docs/fsm/driving-a-workflow.md, docs/fsm/sdlc-loop-example.md
testdata/fsm/            scenario files + fixtures (see §4.3)
tests/go/test-fsm.sh, tests/go/test-fsm-coverage.sh
```

Dependency direction: `run` ← everything; `cmdexec` ← `converge`, `kind`; `judge` ← `kind`; `gate`,
`converge`, `kind`, `workflow` ← `machine` ← `cli`. No cycles; each package is unit-testable alone.

New module dependency: `gopkg.in/yaml.v3` (fetchable; verified). The only dependency.

### 1.2 Shared types (`internal/fsm/run`) — the contract every package codes against

```go
type State string                                   // "discover" | "adjudicate" | "fix" | "verify" | "done" | "failed" (from the workflow)
type ExecMode string                                // "inline" | "subagent" | "fork"

type Finding struct {                               // review-lenses output (mirrors harnesseval Finding.to_candidate_dict)
    IssueText string `json:"issue_text"`
    File      string `json:"file,omitempty"`
    Line      int    `json:"line,omitempty"`
    Severity  string `json:"severity,omitempty"`
    Category  string `json:"category,omitempty"`
    Source    string `json:"source,omitempty"`      // e.g. "metareview-lens/security"
}

type Bug struct {                                   // a confirmed bug (adjudicate output; the fix/verify unit)
    ID         string  `json:"id"`                  // sha1(issue_text)[:12]; stable across iterations
    Desc       string  `json:"desc"`
    File       string  `json:"file,omitempty"`
    Line       int     `json:"line,omitempty"`
    Verdict    string  `json:"verdict"`             // "matched" | "real_but_ungold"
    Confidence float64 `json:"confidence"`
}

type TokenTotals struct{ Input, CacheRead, CacheCreate, Output, Reasoning int64 }   // Total() sums all five

type GateError struct {                             // every failed gate produces one; serialized into the audit log and CLI output
    Code   string `json:"code"`                     // ERR_NO_FINDINGS | ERR_NO_CONFIRMED | ERR_NO_COMMIT | ERR_CMD_* | ERR_JUDGE_UNSET | ...
    Gate   string `json:"gate"`
    Detail string `json:"detail,omitempty"`         // e.g. the uncommitted diff for ERR_NO_COMMIT
}
func (e *GateError) Error() string

type Snapshot struct {                              // state.json — the whole FSM state; deterministic given the event log
    RunID        string            `json:"run_id"`
    Workflow     string            `json:"workflow"`
    WorkflowHash string            `json:"workflow_hash"`
    Vars         map[string]string `json:"vars"`
    RepoMode     string            `json:"repo_mode"`
    State        State             `json:"state"`
    Iteration    int               `json:"iteration"`     // 0-based; increments on verify→discover
    BaseSHA      string            `json:"base_sha"`      // recorded at init (var BASE or HEAD)
    FixEntryHead string            `json:"fix_entry_head,omitempty"` // HEAD when fix was entered this iteration
    TreeHash     string            `json:"tree_hash,omitempty"`      // working-tree fingerprint at last advance (UNSANCTIONED_EDIT)
    Findings     []Finding         `json:"findings"`      // this iteration's discover output
    Confirmed    []Bug             `json:"confirmed"`     // this iteration's adjudicate output
    AllFound     []Bug             `json:"all_found"`     // CUMULATIVE union across iterations (the convergence set)
    Unfixed      int               `json:"unfixed"`       // cumulative still-present after last verify
    PrevUnfixed  int               `json:"prev_unfixed"`  // for no_fixation_progress (math.MaxInt sentinel on iter 0 → stored as -1)
    Tokens       TokenTotals       `json:"tokens"`
    NodeOutputs  map[string]json.RawMessage `json:"node_outputs"` // key "node@iter"
    LastError    *GateError        `json:"last_error,omitempty"`
    StopReason   string            `json:"stop_reason,omitempty"`   // all_fixed | no_fixation_progress | max_iterations | budget | cmd:<name>
    Overflow     bool              `json:"overflow"`
}

type Event struct {                                 // one line of audit.jsonl
    Seq       int64           `json:"seq"`
    At        time.Time       `json:"at"`
    Type      string          `json:"type"`         // init | transition | gate | llm_call | cmd_call | node_output | record | warn | converge | overflow | resume
    State     State           `json:"state,omitempty"`
    Iteration int             `json:"iter"`
    Data      json.RawMessage `json:"data"`
}

type RunStore interface {
    Init(runID string, snap Snapshot) error
    Save(snap Snapshot) error                       // atomic overwrite of state.json
    Load(runID string) (Snapshot, error)
    Append(runID string, ev Event) error            // audit.jsonl, monotonic Seq assigned by the store
    Events(runID string) ([]Event, error)
    List() ([]string, error)
}
```
Real store: `.metareview/runs/<run-id>/state.json` + `audit.jsonl` (same namespace as the existing
`.metareview/runs.jsonl`, per spec §15.3). Test store: in-memory. `Clock` (`func() time.Time`) and
`IDGen` are injected everywhere time or randomness appears; tests use fixed values.

### 1.3 Interfaces (DI seams)

```go
// gate
type Git interface {
    Head(ctx) (string, error)
    CommitCount(ctx, from, to string) (int, error)   // rev-list --count from..to
    IsClean(ctx) (bool, string, error)               // porcelain + diff for the detail
    TreeHash(ctx) (string, error)                    // hash of `git status --porcelain` + `git diff` (advisory snapshot)
    Diff(ctx, from, to string) (string, error)
}
type Gate func(ctx, *run.Snapshot, Git) *run.GateError        // nil == pass
var Builtin = map[string]Gate{"findings_nonempty":…, "confirmed_nonempty":…, "commit_exists":…, "all_fixed":…, "bugs_remain":…}

// converge
type Predicate interface { Name() string; Evaluate(run.Snapshot) (stop bool, reason string, err error) }
func Parse(node *yaml.Node, opts ParseOptions) (Predicate, error)   // opts carries the cmdexec.Runner + allow flag

// cmdexec
type Runner interface { Run(ctx, Spec) (Result, error) }        // Spec{Cmd, Args, Stdin, Timeout}; Result{Stdout, Stderr, ExitCode, Duration}
type Guarded struct { Runner; Allow bool; Audit func(run.Event) }  // Call(ctx, name, spec, outputSchema) → ERR_CMD_NOT_FOUND / ERR_CMD_FAILED / ERR_CMD_OUTPUT_INVALID / ERR_CMD_TIMEOUT

// judge
type Request struct { Kind, Model, Effort string; Input any; RunID string }
type Verdict struct { Kind string; Raw string; Parsed json.RawMessage; Confidence float64; Tokens run.TokenTotals; InputHash string; Model, Effort string }
type Judge interface { Call(ctx, Request) (Verdict, error) }
type Doer interface { Do(*http.Request) (*http.Response, error) }   // providers take a Doer; tests inject a fake with recorded bodies

// kind
type NodeConfig struct { Model, Effort string; Vars map[string]string; RunID string; Iteration int; Params map[string]any }
type Kind interface {
    Name() string
    DefaultExec() run.ExecMode
    IsLLM() bool
    InputSchema() json.RawMessage
    OutputSchema() json.RawMessage
    // Instructions renders the NEEDS_INPUT payload for inline/subagent execution (the host does the work).
    Instructions(in Input, cfg NodeConfig) (Instructions, error)
    // Run executes the node inside the binary — only meaningful for exec=fork (LLM kinds via Judge, or CmdKind).
    Run(ctx, in Input, cfg NodeConfig, j Judge) (Output, error)
    // Reduce validates a host-supplied or Run-produced output and folds it into the snapshot.
    Reduce(snap *run.Snapshot, out Output) error
}
type Registry struct{ … } ; func (r *Registry) Register(Kind) ; func (r *Registry) Get(name) (Kind, error)

// machine
type Deps struct { Store run.RunStore; Kinds *kind.Registry; Judge judge.Judge; Git gate.Git; Cmd cmdexec.Runner; Clock func() time.Time; RunID func(string, time.Time) string }
func Init(ctx, Deps, InitOptions) (*Machine, InitResult, error)          // InitOptions{WorkflowPath|Workflow, Vars, AllowCustomCmds, Calibration, RepoMode}
func Open(ctx, Deps, runID string) (*Machine, error)
func (m *Machine) Advance(ctx, AdvanceOptions) (AdvanceResult, error)     // AdvanceOptions{From State, Vars map[string]string}
func (m *Machine) Record(ctx, RecordOptions) error                        // node-output | tokens | note
func (m *Machine) State() StateView
```

### 1.4 The exec-mode split is the key simplification

* `exec: fork` — **the binary does the work.** `Advance` calls `kind.Run` (which calls `Judge`), stores the
  `node_output`, then evaluates gates. Only `fork` nodes ever make LLM calls from inside the binary.
* `exec: inline | subagent` — **the host agent does the work.** `Advance` returns `NEEDS_INPUT` with the
  kind's `Instructions` (prompt, lens list, input, output schema, and the exact `record` command to
  call). The host executes (inline: itself; subagent: parallel subagents), then runs
  `metareview fsm record node-output --node <n> --data <file>`; the kind's `Reduce` validates against
  `OutputSchema` and folds it into the snapshot. The next `Advance` proceeds to the gates.

Consequence for testing: mock-AI is only needed for `fork` kinds (`match-then-adjudicate`,
`still-present`, and `review-lenses` when overridden to `fork` for a fully-automated run). Inline/subagent
outputs in tests are just scripted `record` calls — no mock needed, and the e2e tests drive the machine
exactly the way a host agent would.

### 1.5 `Advance` algorithm (deterministic; this is what the e2e tests pin)

```
1  snap ← Store.Load(run); if --from S: snap ← rewind(S)  (§C11), append resume event
2  if snap.State ∈ {done, failed} and no --from → ERR_RUN_TERMINAL (exit 1)
3  advisory mode: h ← Git.TreeHash(); if h ≠ snap.TreeHash and State ∉ {fix} → append warn UNSANCTIONED_EDIT (never fails)
4  node ← workflow.Nodes[snap.State]
5  if node exists and NodeOutputs["node@iter"] absent:
       if node.Exec == fork:  out ← kind.Run(...); append llm_call/cmd_call + node_output; kind.Reduce(snap, out); Store.Save
       else:                  return NEEDS_INPUT{instructions} (exit 3), nothing persisted except the tree hash
6  for t in workflow.Transitions from snap.State (declaration order):
       if t.To is a loop-boundary target (verify→*): stop,reason ← convergence.Evaluate(snap); pick t by (stop ? all_fixed : bugs_remain); break
       err ← gate(t.Gate)(snap); if err == nil: pick t; break; else remember err
7  no transition passed → snap.State ← failed; snap.LastError ← err; append gate(fail)+transition; Save; exit 1 (JSON includes code + detail)
8  transition: append gate(pass)+transition; if t.To == discover: iteration++, Findings/Confirmed reset (AllFound kept), PrevUnfixed ← Unfixed
   if t.From == fix→verify: FixEntryHead cleared; if t.To == fix: FixEntryHead ← Git.Head()
   if stop: StopReason ← reason; if reason ∈ {max_iterations, budget, cmd:*}: Overflow ← true
9  Save; if Overflow: run on_overflow via cmdexec.Guarded (audited, output ignored) exactly once; exit 0
```
`commit_exists` = `CommitCount(FixEntryHead, HEAD) > 0 && IsClean()`; on fail `Detail` = the porcelain
status + uncommitted diff (what would have caught the 27-files/0-commits incident).

---

## 2. Judge primitive (`internal/fsm/judge`) — port contract

Prompts ported verbatim from harnesseval (Martian calibration parity for `match` is a hard requirement):

| kind | system | user template | max_tokens | output | rule |
|---|---|---|---|---|---|
| `match` | "You are a precise code review evaluator. Always respond with valid JSON." | `judge.py:22` JUDGE_PROMPT | 1024 (Python: 256; raised for reasoning models per `sdlc_loop.py:274`) | `{reasoning, match, confidence}` | greedy golden-outer/candidate-inner, `confidence > best` |
| `adjudicate` | "You are a strict code review verifier. Always respond with valid JSON." | `adjudicate.py:21` ADJUDICATE_PROMPT, diff `[:30000]` bytes | **2048** (load-bearing; 512 starves reasoning models → all-hallucination) | `{reasoning, is_real, confidence}` | real iff `is_real && confidence ≥ 0.7`; error ⇒ hallucination |
| `still-present` | same as adjudicate | `sdlc_loop.py:321` + `"confidence": 0.0-1.0` | 1024 | `{reasoning, still_present, confidence?}` | **fail-closed**: parse error/missing ⇒ `true` |
| `dedup` | "You are a precise code review deduplicator. Always respond with valid JSON." | `sdlc_loop.py:389` DEDUP_PROMPT (one call per file) | 4096 | `{"<idx>": cluster}` | parse failure ⇒ singleton clusters |

Parsing: `stripFences` reproduces `content.split("```")[1]` + `json` prefix strip exactly (tested against
multi-fence outputs). Diff truncation is a **byte** slice; `diff_context_hash = sha1(diff[:30000])`.

Providers (selected by model id, mirroring `effort.py:50-55`): `anthropic` for `claude*`/`anthropic/*`
(Messages API, `temperature` only for opus-4-5/sonnet-4-5); `openai-compat` for `gpt*`/`openai/*`/`glm*`/`kimi*`
(chat completions; `reasoning_effort` from effort; `max_completion_tokens`). Keys: `ANTHROPIC_API_KEY`,
`OPENAI_API_KEY`; base URL overrides `MRV_ANTHROPIC_BASE_URL`, `MRV_OPENAI_BASE_URL` (Martian proxy /
Lunaroute). Retry: 5 attempts on 429/5xx/overloaded with the Python backoff table; tests use a fake
`Doer` with recorded bodies. Token accounting per `usage.py` (Anthropic: input/cache_read/cache_creation/output;
OpenAI: `prompt_tokens`, `completion_tokens − reasoning_tokens`, `reasoning_tokens`). Every call appends an
`llm_call` event `{kind, model, effort, input_hash, verdict, confidence, tokens, duration_ms}`.

`--calibration` pins `JUDGE=gpt-5.2`, `JUDGE_EFFORT=medium` and refuses `--var JUDGE=` overrides.
Without `--calibration` and without `--var JUDGE=…` for a workflow whose `JUDGE` var is `required`,
`init` fails `ERR_JUDGE_UNSET` (spec §17 — no product default until the judge-eval reports).

---

## 3. CLI contract (`metareview fsm …`)

### 3.1 Commands

```
metareview fsm init     --workflow <name|path> [--var K=V]... [--base <ref>] [--repo-mode advisory|enforcing]
                        [--allow-custom-cmds] [--calibration] [--mock-ai <scenario-dir>]
metareview fsm state    [--run <id>]                       (default --run: newest run in .metareview/runs/)
metareview fsm advance  [--run <id>] [--from <state>] [--var K=V]... [--mock-ai <dir>]
metareview fsm record   node-output --node <n> --data <file|-> [--run <id>]
metareview fsm record   tokens --data '{"input":N,"output":N,...}'   [--run <id>]
metareview fsm record   note   --data '<json>'                        [--run <id>]
metareview fsm gate     <name> [--run <id>]                (evaluate one gate against the current snapshot; no transition)
metareview fsm judge    --kind <match|adjudicate|still-present|dedup> --model <m> [--effort <e>] --input <file> [--context <file>] [--run <id>]
metareview fsm converge [--run <id>] [--check <yaml>]       (evaluate the run's predicate, or an ad-hoc one, against the snapshot)
metareview fsm workflows                                     (list shipped + ./workflows/*.yaml with their vars)
metareview fsm --agent-prompt                                (emit the tool-usage blurb for --append-system-prompt / AGENTS.md)
```
Shipped workflow names resolve to embedded files (`workflows/*.yaml` via `embed`); a path is also accepted.
`MOCK_AI=<dir>` is equivalent to `--mock-ai`.

### 3.2 Output: JSON on stdout, one object, always

Every command prints one JSON object. Human-readable detail goes to stderr. Common envelope:
`{"ok":bool, "run_id":…, "state":…, "iteration":N, "status":"ADVANCED|NEEDS_INPUT|GATE_FAILED|CONVERGED|TERMINAL", …}`.

`advance` → `AdvanceResult`:
```json
{"ok":true,"run_id":"mrv-…","status":"ADVANCED","from":"adjudicate","to":"fix","gate":{"name":"confirmed_nonempty","passed":true},
 "iteration":0,"next_action":"advance","warnings":["UNSANCTIONED_EDIT: 2 files changed outside a fix node"]}
{"ok":false,"status":"GATE_FAILED","from":"fix","to":"failed","gate":{"name":"commit_exists","passed":false,"code":"ERR_NO_COMMIT","detail":"…diff…"},"resume_hint":"metareview fsm advance --run … --from fix"}
{"ok":true,"status":"CONVERGED","to":"done","stop_reason":"no_fixation_progress","overflow":false,"unfixed":2,"all_found":9}
```
`NEEDS_INPUT` (§3.4) is the fourth shape.

### 3.3 Exit codes (consistent with the repo's existing "0 verify / 1 follow the log / 2 usage" contract)

| code | meaning |
|---|---|
| 0 | command succeeded; for `advance`: transitioned or converged (check `status`) |
| 1 | gate failed / run terminal / ERR_* condition; JSON on stdout has `code` + `detail`; state persisted |
| 2 | usage error (unknown option, missing value, unknown workflow, ERR_JUDGE_UNSET, ERR_CMD_NOT_FOUND at init) |
| 3 | `NEEDS_INPUT` — the host agent must do the node's work and `record node-output`, then `advance` again |

### 3.4 `NEEDS_INPUT` payload

```json
{"ok":true,"status":"NEEDS_INPUT","run_id":"…","state":"discover","iteration":0,
 "node":{"name":"discover","kind":"review-lenses","exec":"subagent","model":"claude-opus-5","effort":"low"},
 "instructions":"Dispatch the 8 adversarial lenses as parallel subagents against `git diff <base>..HEAD` … Return a JSON array of findings matching output_schema.",
 "input":{"base_sha":"…","head_sha":"…","lenses":["feasibility",…],"rubric":"rubrics/artifact-review-rubric.md","confirmed_bugs":[]},
 "output_schema":{…JSON schema…},
 "record":"metareview fsm record node-output --run <id> --node discover --data <file>"}
```
`Reduce` rejects outputs that fail the schema with `ERR_NODE_OUTPUT_INVALID` (exit 1, nothing persisted).

---

## 4. Test strategy (the release gate)

### 4.1 Hard gates, wired into `tests/run-all.sh`

* `tests/coverage.sh` (new `npm test` entry point; `tests/run-all.sh` stays the plain suite): creates a
  `GOCOVERDIR`, runs `go test ./... -args -test.gocoverdir=$dir` **and** `GOFLAGS=-cover bash tests/run-all.sh`
  (which instruments every `go run` / `go build` the shell scripts perform — verified, zero script edits), merges
  with `go tool covdata percent`, then **fails unless every `internal/fsm/...` package reports 100.0%** and no
  legacy package drops below its line in `tests/coverage-floor.txt` (seeded from the 2026-08-26 measurement in
  §0 C1). Prints the per-package table so the gap is visible in CI output.
* `tests/go/test-fsm.sh`: black-box CLI tests through `go run ./cmd/metareview fsm …` with `--mock-ai`
  against a temp git repo: init → NEEDS_INPUT → record → advance … → done; gate failure exit 1; usage exit 2;
  `--agent-prompt` mentions every subcommand; `--help` lists `fsm`.
* `tests/manifest/test-skills.sh`: add `skills/fsm/SKILL.md`, `commands/fsm.md`, `workflows/sdlc-loop.yaml`,
  `workflows/review-loop.yaml`, the two docs pages; `test-manifests.sh`: `workflows/` in `package.json` files,
  plugin manifests advertise "workflow"/"fsm".

### 4.2 TDD loop per package (every subagent follows this; the orchestrator verifies the trail)

1. Write the test table for one behavior. Run `go test ./internal/fsm/<pkg>/` — must **fail to compile or fail
   the assertion for the stated reason** (record the failing output in the commit message trailer `TDD-Red:`).
2. Implement the minimum. Green.
3. `go test -cover ./internal/fsm/<pkg>/` must print 100.0% before the next behavior starts. Uncovered lines
   are a test gap, not a refactor task — write the test (error paths get explicit tests via injected failures:
   a failing `Doer`, a store that errors on `Save`, a `Git` that returns dirty, a clock, etc.).
4. Commit per behavior (small commits; `git diff --check` clean). One PR per milestone.

Anything that cannot be reached by a test is deleted, not `//nolint`-ed. No global state; no `init()`
registration (the registry is built by `kind.Builtins()` and injected).

### 4.3 Test data (`testdata/fsm/`)

* `scenarios/sdlc-loop/{happy,cumulative-convergence,no-findings,dirty-tree,judge-swap,overflow}/`:
  each a scenario dir with `judge.yaml` (kind + input-hash → scripted `{parsed, tokens}`), and
  `records/<node>@<iter>.json` (the host-supplied outputs the test "records"). **Unscripted inputs fail
  the test** (`ERR_MOCK_UNSCRIPTED`), never silently pass.
* `judge/fixtures/`: recorded provider HTTP bodies (Anthropic + OpenAI-compat, incl. a fenced JSON reply,
  a multi-fence reply, a reasoning-starved empty reply, a 429 then success) — sanitized from
  `harnesseval/runs/*/summary.json` shapes.
* `judge/match-parity/`: a small slice of Martian's shipped `candidates.json` + `dedup_groups.json` +
  `evaluations.json` for one PR, used to assert `scoreFromMatches` reproduces TP/FP/FN exactly (calibration
  parity of the greedy matcher, independent of any model).
* `workflows/`: valid + invalid YAML (unknown state, transition to undeclared state, node without kind,
  `cmd:` without `--allow-custom-cmds`, missing required var, unknown convergence atom, cyclic `not`).

### 4.4 Behavioral end-to-end tests (`internal/fsm/machine/e2e_test.go`) — all mocked, temp git repo, ms, zero tokens

| # | scenario | asserts |
|---|---|---|
| E1 | happy path | discover→adjudicate→fix→verify→done; every transition/gate/llm_call/node_output event present with the right fields; final snapshot; `status: CONVERGED, stop_reason: all_fixed`; `on_overflow` not run |
| E2 | **cumulative convergence regression** | 3 iterations; iter 3 fixes its own 1 bug while 7 cumulative remain → **no** `all_fixed`; loops; stops on `no_fixation_progress` at the right iteration; `AllFound` is the union; `PrevUnfixed` bookkeeping exact |
| E3 | gate failure + resume | fix leaves the tree dirty → `ERR_NO_COMMIT` with the diff in `Detail`; state `failed`; test commits; `advance --from verify` → discover/adjudicate outputs **replayed from audit.jsonl** (judge mock records zero new calls for them); run completes |
| E4 | JUDGE swap via resume (§17) | run to done with `JUDGE=gpt-5.2`; `advance --from adjudicate --var JUDGE=claude-opus-5`; discover not re-run; adjudicate/verify re-run with the new model; the two audit logs differ **only** in llm_call model/verdict rows (diff helper asserts the set of differing fields) |
| E5 | overflow | `max_iterations: 2` fires → `done`, `overflow: true`, `on_overflow` cmd ran once with the final snapshot on stdin and was audited; E1/E2 never run it; `budget` variant with judge-reported tokens crossing the threshold |
| E6 | ERR_NO_FINDINGS / ERR_NO_CONFIRMED | discover returns `[]` → `failed` with the code; adjudicate confirms none → same |
| E7 | advisory vs enforcing | advisory: out-of-node edit → `warn UNSANCTIONED_EDIT`, gate still passes on a real commit; enforcing: same edit → `ERR_UNSANCTIONED_EDIT` |
| E8 | cmd guardrails | workflow with `cmd:` atom: init without `--allow-custom-cmds` → exit 2; missing script → `ERR_CMD_NOT_FOUND`; slow script → `ERR_CMD_TIMEOUT`; bad JSON → `ERR_CMD_OUTPUT_INVALID`; nonzero → `ERR_CMD_FAILED`; every call audited |
| E9 | idempotency per kind | for each of the 4 kinds: resume `--from` its entry state re-runs only it; upstream outputs byte-identical to the first run |
| E10 | judge tokens counted | `budget` atom sees judge-reported tokens (the Python `_score_tin` bug, inverted into a test) |

### 4.5 Smoke (manual, not CI)

`go test -tags smoke ./internal/fsm/judge/ -run TestSmoke` with real keys: one `match`, one `adjudicate`,
one `still-present` call per provider, asserting only shape + non-zero tokens. Run before tagging 0.9.0.

---

## 5. Milestones & orchestration

Interfaces in §1.2–1.3 are the contract; they land first so package work can fan out. Each milestone is a
PR to `main` from a branch off `fsm-enhancements` (no version numbers in branch names); each PR must pass
`bash tests/run-all.sh` (which now includes the 100% gate) and the repo's own `review pr-ready` gate.

| M | deliverable | depends on | parallel? |
|---|---|---|---|
| **M0 spine** (orchestrator, inline) | `go get gopkg.in/yaml.v3`; `internal/fsm/run` types + `RunStore` (JSONL + memory) at 100%; `tests/go/test-fsm-coverage.sh` + `coverage-floor.txt` wired into `run-all.sh`; `cmd/metareview` `fsm` branch → `fsmcli.Run` stub (`--help`, exit 2 on unknown) | — | no — everything codes against it |
| **M1 workflow** | YAML parse/validate/resolve; `ERR_JUDGE_UNSET`; `--calibration`; cmd declare+verify; embedded shipped workflows loader | M0 | ✅ with M2–M4 |
| **M2 gate + git** | 5 gates; `Git` real impl over `exec` (tested in temp repos) + fake; `TreeHash` advisory snapshot | M0 | ✅ |
| **M3 cmdexec + converge** | guarded runner (timeout/schema/audit); predicate atoms + compose + `Parse` + `CmdPredicate` | M0 | ✅ |
| **M4 judge** | prompts, parsers, thresholds, fail-closed, providers over `Doer`, retry/backoff, token accounting, `MockJudge` + `mockai` loader, match-parity fixture test | M0 | ✅ |
| **M5 kinds** | `Kind` registry; 4 built-ins (`Instructions`/`Run`/`Reduce`), `CmdKind`, `MockKind`; JSON-schema validation of outputs | M0, M4 | after M4 |
| **M6 machine** | `Init/Open/Advance/Record/State`; NEEDS_INPUT; loop boundary + convergence; overflow; E1, E2, E5, E6, E7, E8, E10 | M1–M5 | no |
| **M7 resume** | `--from` rewind + replay; E3, E4, E9 | M6 | no |
| **M8 CLI + product** | full `fsm` surface, JSON envelope, exit codes, `--agent-prompt`, `workflows`; `tests/go/test-fsm.sh`; `workflows/sdlc-loop.yaml` + `review-loop.yaml`; `skills/fsm/SKILL.md`, `commands/fsm.md`; docs pages; README/quickstart/AGENTS/CLAUDE updates; manifests + manifest tests; CHANGELOG; version → 0.9.0 in all five places | M7 | docs/skill can start at M6 |
| **M9 release** | smoke test with real keys; `review pr-ready`; tag | M8 | — |

**Orchestration:** M0 is written inline by the orchestrator (it *is* the contract). M1–M4 fan out to four
subagents in isolated worktrees, each with the §1 contract, its package's section of this doc, the relevant
harnesseval digest, and the TDD loop of §4.2 as its instructions; each returns a PR-ready branch. The
orchestrator reviews each for the TDD trail (red → green commits) and the 100% gate before merging. M5–M7 are
sequential (they integrate everything) and stay with the orchestrator or a single subagent. M8 splits
product docs/skill/workflows from CLI code. Estimated Go: ~5–6k lines incl. tests; the deterministic
core (M2, M3, M6, M7) is where the value is — the spec's own advice ("build the spine, then generalize")
is honored by putting `kind` after `judge`/`gate`/`converge`.

**Before M0:** run `metareview review artifact` on the design spec *and* this plan (CLAUDE.md lifecycle
rule: review the artifact before implementing it). Findings from that review amend §0 before code starts.

---

## 6. Shipped default workflows (built-ins only; no `cmd:` in loop logic)

`workflows/sdlc-loop.yaml` — the spec's §5 YAML with C3/C4 applied and `JUDGE: {required: true}`; convergence
`any: [all_fixed, no_fixation_progress, {max_iterations: 5}, {budget: {tokens: 4000000}}]`; `repo_mode: advisory`;
`on_overflow` present but commented out as the documented example.

`workflows/review-loop.yaml` (proposed, §7 Q1) —
```yaml
workflow: review-loop
version: 1
vars: {REVIEWER: {default: claude-opus-5}, JUDGE: {required: true}, REV_EFFORT: {default: low}, JUDGE_EFFORT: {default: medium}}
states: [discover, adjudicate, done, failed]
transitions:
  discover→adjudicate: {gate: findings_nonempty}
  adjudicate→done:     {gate: confirmed_nonempty}
  "*→failed":          {on: gate_error}
nodes:
  discover:   {kind: review-lenses, exec: subagent, lenses: 8, model: $REVIEWER, effort: $REV_EFFORT}
  adjudicate: {kind: match-then-adjudicate, exec: fork, model: $JUDGE, effort: $JUDGE_EFFORT}
repo_mode: advisory
```
This is the existing one-shot review (`review artifact` / `task-done` lens dispatch) with an adjudication pass,
expressed as an FSM: read-only, no `agent-edit`, so the 8 lenses stay critics. It also gives the judge-eval a
cheap re-runnable harness (`--from adjudicate` with a different `JUDGE`).

---

## 7. Decisions (resolved 2026-08-26 with Dave)

1. **Second default workflow:** ship `review-loop` alongside `sdlc-loop` (§6).
2. **Coverage gate:** 0.9.0 enforces 100% on `internal/fsm/...` plus the recorded per-package floor on legacy
   packages; a follow-up `coverage-to-100` branch lifts legacy packages to 100% after 0.9.0 so the gate becomes flat.
3. **Judge providers:** Anthropic + OpenAI-compatible only (glm/kimi/Martian proxy via base-URL override).
4. **Version:** stays `0.8.2` during the build; bumps to `0.9.0` (all five manifest places) only after the full
   suite and coverage gate pass and the release is ready to tag (last step of M8, before M9 smoke + tag).
