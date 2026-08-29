# metareview context: docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md

Run ID: `mrv-20260827-062655870118000-artifact-2026-08-27-metareview-0-9-0-fsm-judge-kinds-33d63bfb`

## Target

- Path: `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
- Repository mode: `metaswarm-extension`
- Git branch: `fsm-enhancements`
- Git head: `bf2ef96`

## Artifact Excerpt

````markdown
# metareview 0.9.0 — spec 4: guardrails, judge, kinds, and mock AI

> **Status:** DRAFT r1 (2026-08-27). Fourth of the five split artifacts (ownership ledger: run spec §12). Owns plan
> r3 §1.8 (escape-hatch guardrails), §2 (judge port), kinds/`Executor`/`Delta` producers, match-then-adjudicate
> composition + `Bug.Verdict` vocabulary, `index` assignment, mock scenarios, `JUDGE_EFFORT` requiredness, and the
> pinned harnesseval provenance of the prompts. Implements the `machine.NodeKind`/`Executor`/`Registry` and
> `converge.Runner` interfaces declared in spec 2.
>
> **Port spec:** `~/Developer/harnesseval/harnesseval/{judge,adjudicate,sdlc_loop,usage,model_router}.py` @ `19ff9a8`
> (sibling repo, not vendored). The verbatim prompt texts are committed as fixtures with that sha in a header line.

---

## 1. Packages
```
internal/fsm/cmdexec   Runner (exec-backed + fake); Guarded (allow-list, hash re-verify, timeout, typed decode, audit)
internal/fsm/judge     Judge iface; prompts + fencing; parsers; providers; retry; tokens; MockJudge
internal/fsm/kind      NodeKind/Executor implementations: review-lenses, match-then-adjudicate, still-present, agent-edit, cmd; Registry
internal/fsm/mockai    scenario files for MockJudge and the fake Runner
```
`run` ← all; `workflow` ← `kind` (node params); `machine` interfaces ← `kind` (compile-time assertion only via a small
`machine` import — allowed: `machine` does not import `kind`). `cmdexec` ← `kind` (CmdKind), `converge` (Runner satisfied).

## 2. `cmdexec`
```go
type Runner interface { Run(ctx, Spec) (Result, error) }                       // Spec{Argv []string; Dir string; Stdin []byte; Timeout time.Duration}; Result{Stdout, Stderr []byte; ExitCode int; Duration time.Duration}
type Guarded struct { Runner Runner; Allowed []run.AllowedCmd; FileHash func(string) (string, error); Audit func(run.Event) }
func (g Guarded) Call(ctx, name string, stdin []byte, out any) *Error
func (g Guarded) Run(ctx, name string, argv []string, stdin []byte, timeout time.Duration) ([]byte, int, error)   // converge.Runner
```
`Call`: `name ∈ Allowed` (`ERR_CMD_NOT_ALLOWED`); for every `FileHashes` entry re-hash → `ERR_CMD_CHANGED`; run with `Timeout`
(default 60 s; `ERR_CMD_TIMEOUT`); non-zero → `ERR_CMD_FAILED`; stdout → `json.Unmarshal` into `out` with
`DisallowUnknownFields` → `ERR_CMD_OUTPUT_INVALID`; every call (including failures) appends `cmd_call{name, argv, input_hash =
sha256(stdin), stdout/stderr capped via CapText with flags, exit_code, duration_ms, error}` through `Audit`. The exec runner
never uses a shell: `exec.CommandContext(ctx, argv[0], argv[1:]...)` with `Dir`, an empty-ish environment (`PATH`, `HOME`,
`LANG`, `TMPDIR`, `MRV_RUN_ID`), and the context deadline for the timeout.

## 3. `judge`
```go
type Request struct { Kind, Model, Effort string; Input any; RunID, Node string; Index int; Fence bool }
type Verdict struct { Kind, Model, Effort, InputHash, Raw string; Parsed json.RawMessage; Confidence float64; Tokens run.TokenTotals; Mock bool; Duration time.Duration }
type Judge interface { Call(ctx, Request) (Verdict, error) }
type Doer interface { Do(*http.Request) (*http.Response, error) }
func New(doer Doer, keys Keys, urls URLs, nonce func() string, clock func() time.Time) Judge   // real
type MockJudge struct{ … }; func (m *MockJudge) Calls() []Request
func RenderPrompt(kind string, in any, fence bool, nonce string) (system, user string, err error)
```
### 3.1 Kinds and inputs
| kind | input | system | user template source | max_tokens | output | rule |
|---|---|---|---|---|---|---|
| `match` | `{golden Golden, candidate Finding}` | "You are a precise code review evaluator. Always respond with valid JSON." | `judge.py:22` JUDGE_PROMPT | 1024 | `{reasoning, match, confidence}` | `match && confidence > best` (greedy; ties keep the first) |
| `adjudicate` | `{diff string, candidate Finding}` | "You are a strict code review verifier. Always respond with valid J
````

## Service Inventory

No service inventory found.

## Knowledge Facts

No Beads knowledge facts found.

## Suggested Reviewers

- feasibility
- completeness
- scope/alignment
- architecture
- intent preservation
