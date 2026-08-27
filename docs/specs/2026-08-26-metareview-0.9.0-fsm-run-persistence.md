# metareview 0.9.0 — `internal/fsm/run`: event-sourced run persistence

> **Status:** DRAFT for review — the first of the split 0.9.0 build artifacts. Parent plan:
> [`2026-08-26-metareview-0.9.0-build-plan.md`](2026-08-26-metareview-0.9.0-build-plan.md) (revision 3,
> escalated on 2026-08-26 as too large to converge in one document). This spec covers **only** the `run`
> package: wire types, the event log, the fold that derives a snapshot, the store/lock contract, and the
> trust boundary. It is the contract that `workflow`, `gate`, `converge`, `judge`, `kind`, `machine`, and
> `cli` will build against; those get their own specs after this one is locked.
>
> **Scope rule:** this document says what `run` *stores and derives*. It does not say when the machine
> appends an event, how a gate is evaluated, how a fork chooses a checkpoint, or what the CLI prints.
> Where a later spec needs a field, the field is defined here; where a rule belongs to the machine, this
> document names the invariant `run` enforces and nothing more.
>
> **Resolves (from review `mrv-20260827-014544…`):** ARC3-1/2/3/4/6/10, DM3-2/4, CMP3-2/4/5/6, SEC-21/22
> (run part), FIN-1/2 (run part)/6/7, TQ3-1/9.

---

## 1. Principles

1. **`audit.jsonl` is the only authority.** There is no `state.json`. Every command derives the snapshot
   by folding the log. Read-only commands write nothing.
2. **`Fold` is pure and total over `run` types.** It takes events and returns a snapshot or an error. It
   imports no other `fsm` package. Anything the machine "sets" must be carried by an event payload or be
   derivable from earlier events, or it does not exist.
3. **Append-only, hash-chained, versioned.** Events are never rewritten. Each event carries the hash of the
   previous line. A version or type the reader does not know is an error, never a skip.
4. **Fail closed.** A log that violates an invariant folds to `ERR_AUDIT_INVALID`; the run is unusable
   until an operator inspects it. No "best effort" snapshots.
5. **Trust boundary (stated, not solved).** The run directory is `0700` and files are `0600`, and the hash
   chain detects accidental corruption and casual edits. It does **not** defend against the same-user
   process that runs the FSM — the host agent can rewrite the whole chain. The FSM's guarantees are
   *process* guarantees for a cooperating agent: "you cannot transition fix→verify without a commit" holds
   for an agent that drives the FSM through the CLI, not for one that edits `.metareview/`. This is the
   same class of limit as spec §1 ("does not guarantee the adjudication was correct") and is documented in
   the user-facing docs with the same prominence as the advisory/enforcing caveat.

---

## 2. Wire types

All persisted types carry snake_case JSON tags. `json:"…,omitempty"` only where marked. Sizes are enforced
at append time (`ERR_EVENT_TOO_LARGE`), never silently truncated, except `GateError.Detail`, which is
truncated to 64 KB with `detail_truncated: true`.

```go
package run

const SchemaVersion = 1

type State string       // any string the workflow declares; run treats "done" and "failed" as terminal only via HasOutgoing (§4.7)
type Outcome string     // fixed | clean | reviewed | stalled | overflow | custom | failed
type ExecMode string    // inline | subagent | fork

type Finding struct {
    IssueText string `json:"issue_text"`          // ≤ 4 KB
    File      string `json:"file,omitempty"`
    Line      int    `json:"line,omitempty"`
    Severity  string `json:"severity,omitempty"`
    Category  string `json:"category,omitempty"`
    Source    string `json:"source,omitempty"`
}
type Golden struct { Comment string `json:"comment"`; Severity string `json:"severity,omitempty"`; Category string `json:"category,omitempty"` }
type Bug struct {
    ID         string  `json:"id"`                  // BugID(issue_text) = hex(sha1(issue_text))[:12]
    Desc       string  `json:"desc"`                // ≤ 2 KB
    File       string  `json:"file,omitempty"`
    Line       int     `json:"line,omitempty"`
    Verdict    string  `json:"verdict"`             // "matched" | "real_but_ungold"
    Confidence float64 `json:"confidence"`
    GoldenIdx  *int    `json:"golden_idx,omitempty"`
}
type BugStatus struct { ID string `json:"id"`; StillPresent bool `json:"still_present"`; Confidence float64 `json:"confidence"` }
type TokenTotals struct {
    Input int64 `json:"input"`; CacheRead int64 `json:"cache_read"`; CacheCreate int64 `json:"cache_create"`
    Output int64 `json:"output"`; Reasoning int64 `json:"reasoning"`
}
func (t TokenTotals) Add(u TokenTotals) TokenTotals
func (t TokenTotals) Total() int64

type GateError struct {
    Code            string `json:"code"`
    Gate            string `json:"gate"`
    Detail          string `json:"detail,omitempty"`            // ≤ 64 KB in the audit
    DetailTruncated bool   `json:"detail_truncated,omitempty"`
}
func (e *GateError) Error() string

type AllowedCmd struct {
    Name       string            `json:"name"`
    Argv       []string          `json:"argv"`                   // argv[0] absolute
    FileHashes map[string]string `json:"file_hashes"`            // absolute path → hex sha256; serialized with sorted keys
}

// Delta is what a node's Reduce produced. Defined here because Fold applies it.
// Delta carries NO tokens: token accounting comes only from llm_call and tokens events (§4.4).
type Delta struct {
    Findings  []Finding   `json:"findings,omitempty"`
    Confirmed []Bug       `json:"confirmed,omitempty"`
    Status    []BugStatus `json:"status,omitempty"`
    Commit    string      `json:"commit,omitempty"`               // informational; gates read git
}
```

### 2.1 Snapshot (derived; never persisted by `run`)

```go
type Snapshot struct {
    SchemaVersion int               `json:"schemaVersion"`
    RunID         string            `json:"run_id"`
    ParentRunID   string            `json:"parent_run_id,omitempty"`
    ForkedAtSeq   int64             `json:"forked_at_seq,omitempty"`   // parent seq the prefix was copied through
    CreatedAt     time.Time         `json:"created_at"`
    Seq           int64             `json:"seq"`                        // last event folded
    Workflow      string            `json:"workflow"`
    WorkflowHash  string            `json:"workflow_hash"`
    Vars          map[string]string `json:"vars"`
    Calibration   bool              `json:"calibration"`
    Mock          string            `json:"mock,omitempty"`
    RepoMode      string            `json:"repo_mode"`
    AllowedCmds   []AllowedCmd      `json:"allowed_cmds,omitempty"`
    CmdsSHA256    string            `json:"cmds_sha256,omitempty"`
    RepoRoot      string            `json:"repo_root"`                  // where .metareview/runs lives
    WorkDir       string            `json:"work_dir"`                   // where git commands run; may differ on a fork
    State         State             `json:"state"`
    Outcome       Outcome           `json:"outcome,omitempty"`
    Iteration     int               `json:"iteration"`
    BaseSHA       string            `json:"base_sha"`
    Head          string            `json:"head"`                       // HEAD after the last transition (or at init)
    FixEntryHead  string            `json:"fix_entry_head,omitempty"`
    TreeHash      string            `json:"tree_hash,omitempty"`
    Goldens       []Golden          `json:"goldens,omitempty"`
    Findings      []Finding         `json:"findings"`
    Confirmed     []Bug             `json:"confirmed"`
    AllFound      []Bug             `json:"all_found"`                  // cumulative union by Bug.ID, first-seen order
    Status        []BugStatus       `json:"status"`                     // last applied Status; covers AllFound (§4.3)
    Unfixed       int               `json:"unfixed"`
    PrevUnfixed   *int              `json:"prev_unfixed"`               // nil until the first loop transition
    Tokens        TokenTotals       `json:"tokens"`
    NodeOutputs   map[string]json.RawMessage `json:"node_outputs"`     // "node@iter" → last recorded output
    Applied       map[string]bool   `json:"applied"`                    // "node@iter" → delta applied
    NodesRun      []string          `json:"nodes_run"`                  // "node@iter" in first-run order (for downstream freeze rules)
    LastError     *GateError        `json:"last_error,omitempty"`
    StopReason    string            `json:"stop_reason,omitempty"`
    OverflowHandled bool            `json:"overflow_handled"`
    Warnings      []string          `json:"warnings"`                   // codes of warn events, in order
}
func (s Snapshot) Clone() Snapshot        // deep copy: every slice, map, pointer, and RawMessage is fresh
func Key(node string, iter int) string    // fmt.Sprintf("%s@%d", node, iter)
```

---

## 3. Events

```go
type Event struct {
    SchemaVersion int             `json:"schemaVersion"`
    Seq           int64           `json:"seq"`                   // 1-based, contiguous
    Prev          string          `json:"prev"`                  // hex sha256 of the previous line's bytes (without '\n'); "" for seq 1
    At            time.Time       `json:"at"`                    // UTC, RFC3339Nano
    Type          string          `json:"type"`
    State         State           `json:"state,omitempty"`       // snapshot.State when the event was appended
    Iter          int             `json:"iter"`                  // snapshot.Iteration when appended (post-increment for loop transitions, §4.6)
    Node          string          `json:"node,omitempty"`        // for node-scoped events
    Mock          bool            `json:"mock,omitempty"`
    Origin        *Origin         `json:"origin,omitempty"`      // set on events copied from a parent run by a fork
    Data          json.RawMessage `json:"data"`                  // ≤ 1 MB; typed per Type below
}
type Origin struct { RunID string `json:"run_id"`; Seq int64 `json:"seq"` }
```

Copied events are **not wrapped**: a fork appends the parent's events verbatim with new `Seq`/`Prev` and
`Origin` set. `Fold` treats them exactly like native events. (No `replay` type.)

### 3.1 Event types and typed payloads (`Data`)

| `Type` | payload | node-scoped | notes |
|---|---|---|---|
| `init` | `InitData{Workflow, WorkflowHash, Vars, Calibration, Mock, RepoMode, AllowedCmds, CmdsSHA256, RepoRoot, WorkDir, BaseSHA, Head, Goldens, ParentRunID, ForkedAtSeq}` | no | exactly one, at seq 1 |
| `needs_input` | `{}` | yes | informational |
| `node_output` | `{"output": RawMessage}` | yes | the host- or executor-supplied output, already validated by the machine |
| `delta_applied` | `Delta` | yes | the machine appends this after `Reduce`; Fold applies it (§4.3) |
| `llm_call` | `{kind, model, effort, index, input_hash, verdict RawMessage, confidence, tokens TokenTotals, duration_ms, error string}` | yes | `index` = call ordinal within `node@iter` (0-based) |
| `cmd_call` | `{name, argv, input_hash, stdout, stderr, exit_code, duration_ms, error}` | optional | stdout ≤ 64 KB, stderr ≤ 8 KB |
| `gate` | `{name, passed bool, error *GateError}` | no | one per gate evaluation |
| `converge` | `{atom, class Outcome, stop bool, reason}` | no | |
| `transition` | `TransitionData{From, To State; Gate string; Outcome Outcome; Loop bool; ToKind string; Head, TreeHash string}` | no | the checkpoint event (§4.6) |
| `tokens` | `TokenTotals` | no | host-reported; additive |
| `record` | `{name string, data RawMessage}` | no | user events; `Type` is always `record` (name lives in data) |
| `warn` | `{code, detail}` | no | |
| `overflow_handler` | `{exit_code, duration_ms, error}` | no | |
| `fork` | `{child_run_id, at_seq}` | no | appended to the **parent** |

Payload structs are Go types in `run` and are decoded with `DisallowUnknownFields`. `Fold` decodes every
payload; a payload that fails to decode is `ERR_AUDIT_INVALID` (with seq and type in the message).

---

## 4. `Fold`

```go
func Fold(events []Event) (Snapshot, error)
```

Total, pure, deterministic: same slice → byte-identical `json.Marshal(snapshot)`. Errors (all fail closed):

| code | when |
|---|---|
| `ERR_AUDIT_EMPTY` | no events |
| `ERR_AUDIT_VERSION` | any `Event.SchemaVersion != SchemaVersion` |
| `ERR_AUDIT_INVALID` | first event not `init`; a later `init`; `Seq` not contiguous from 1; `Prev` mismatch; unknown `Type`; undecodable payload; or any invariant in §4.3–4.7 |

### 4.1 `init`
Sets every configuration field listed in `InitData`; `State` ← the workflow's initial state is **not**
known to `run`, so `InitData` also carries `InitialState State`. `Head` ← `InitData.Head`. `Iteration` ← 0.
`PrevUnfixed` ← nil. Maps/slices initialized empty (never nil in the snapshot JSON).

### 4.2 `node_output`
Key `k = Key(ev.Node, ev.Iter)`. **Invariant:** `Applied[k]` must be false, else `ERR_AUDIT_INVALID`
("output after delta_applied"). Sets `NodeOutputs[k]` ← payload (last one before the delta wins — this is
the only legal use of `--replace`). Appends `k` to `NodesRun` if absent.

### 4.3 `delta_applied`
Key `k`. **Invariants:** `NodeOutputs[k]` must exist; `Applied[k]` must be false. Then:
- `Findings` ← `Delta.Findings` if non-nil (replace).
- `Confirmed` ← `Delta.Confirmed` if non-nil (replace); each `Bug` whose `ID` is not in `AllFound` is appended to `AllFound` (first-seen order; existing entries unchanged).
- `Status` ← `Delta.Status` if non-nil (replace). **Invariant:** the set of `Status[i].ID` equals the set of
  `AllFound[i].ID` exactly (no missing, no extra, no duplicates) else `ERR_AUDIT_INVALID` ("status incomplete").
  `Unfixed` ← count of `StillPresent == true`.
- `Applied[k]` ← true.
(`Delta.Commit` is retained only in the event; nothing in the snapshot derives from it.)

### 4.4 `llm_call`, `tokens`
`Tokens` ← `Tokens.Add(payload tokens)`. These are the **only** two sources of `Tokens`.

### 4.5 `gate`, `converge`, `warn`, `record`, `needs_input`, `cmd_call`, `overflow_handler`, `fork`
- `gate` with `passed == false`: `LastError` ← `error`. `gate` with `passed == true`: no change.
- `warn`: append `code` to `Warnings`.
- `overflow_handler`: `OverflowHandled` ← true (regardless of exit code).
- `converge` with `stop == true`: `StopReason` ← `atom`.
- `record`, `needs_input`, `cmd_call`, `fork`: no snapshot change (audit only). `fork` is legal at any point
  after `init` and does not make the run terminal.

### 4.6 `transition`
- `State` ← `To`; `Head` ← `Head`; `TreeHash` ← `TreeHash`; `LastError` ← nil.
- If `Loop`: `Iteration` ← `Iteration + 1`; `v := Unfixed; PrevUnfixed ← &v` (fresh value); `Findings` ← empty;
  `Confirmed` ← empty. (`AllFound`, `Status`, `Unfixed`, `NodeOutputs`, `Applied`, `NodesRun` are kept.)
  **Invariant:** the event's `Iter` equals the post-increment `Iteration`, else `ERR_AUDIT_INVALID`.
- If `ToKind == "agent-edit"`: `FixEntryHead` ← `Head`. (The machine resolves `ToKind` from the workflow
  when it appends; `run` never consults a workflow.)
- If `Outcome != ""`: `Outcome` ← `Outcome`. **Invariant:** an `Outcome`-bearing transition must be the last
  non-`fork` event in the log (terminal), else `ERR_AUDIT_INVALID`.
- If `To` is `"failed"`: `Outcome` ← `failed` (even if the payload omits it).

### 4.7 Terminal detection
`run` does not know the workflow graph. `Snapshot.Outcome != ""` **is** the terminal condition for `run`;
the machine additionally refuses to advance a terminal snapshot (`ERR_RUN_TERMINAL`, machine spec).

---

## 5. Store

```go
type RunSummary struct { RunID string; Workflow string; CreatedAt time.Time; State State; Outcome Outcome; ParentRunID string; Mock bool }

type RunStore interface {
    Create(first Event) error                              // first.Type == "init"; creates <root>/.metareview/runs/<run_id>/ (0700) and audit.jsonl (0600)
    Append(runID string, ev Event) (seq int64, err error)  // caller must hold the lock; assigns Seq, Prev, At? — no: At is set by the machine's Clock; store assigns Seq and Prev only
    Events(runID string) ([]Event, error)                  // whole log; see torn-line rule
    List() ([]RunSummary, error)                           // newest CreatedAt first; summary from init + last transition
    Lock(runID string) (unlock func(), err error)
    Root() string
}
func NewJSONLStore(root string) RunStore     // root = repo root; runs live at <root>/.metareview/runs/
func NewMemStore() RunStore                  // holds serialized line bytes; Events() decodes them — behaves exactly like JSONL
func ValidateRunID(id string) error          // ^mrv-[A-Za-z0-9-]{8,200}$
```

Rules:
- **Root is fixed at init.** `RepoRoot` (from `git rev-parse --show-toplevel` of the init cwd, resolved by the
  machine) is where the store lives; a fork's `--work-dir` changes `WorkDir` only. A run's directory never
  moves.
- **Write path.** `Append` serializes the event (`json.Marshal`, no indentation), computes `Prev` from the
  current last line, writes `line + "\n"` with `O_APPEND`, and `fsync`s. `Append` refuses (`ERR_RUN_LOCKED`)
  unless the lock is held by the calling process.
- **Torn-line rule.** `Events()` reads all lines; if the final line does not end in `\n` or does not decode, it
  is **not** returned and the reader reports it via `TornTail bool` on the store (`Events` returns the clean
  prefix). The next `Append` under the lock **truncates the file to the end of the last complete line** before
  writing, then appends a `warn{code: "AUDIT_TORN_LINE_DROPPED"}` event followed by the requested event. A
  torn line earlier than the tail is `ERR_AUDIT_INVALID` (the chain is broken).
- **Hash chain.** `Prev` is `hex(sha256(previous line bytes))`; `Events()` verifies the chain and returns
  `ERR_AUDIT_INVALID` on mismatch.
- **Lock.** `<run dir>/lock` created with `O_CREAT|O_EXCL|O_WRONLY|O_NOFOLLOW`, mode `0600`, containing the
  pid. If it exists: read the pid; if `kill(pid, 0)` returns `ESRCH`, remove the stale file and retry once;
  otherwise `ERR_RUN_LOCKED`. Pid reuse is an accepted residual (the directory is `0700`). `unlock` removes the
  file. The lock is process-scoped, not goroutine-scoped.
- **Size caps.** `Event.Data` ≤ 1 MB → `ERR_EVENT_TOO_LARGE` (append refused, nothing written).
- **No cache.** There is no `state.json`. Commands that need the snapshot call `Fold(Events())`.

---

## 6. Run IDs

`RunID(workflow string, at time.Time) string` = `state.RunID("fsm-"+workflow, workflow, at)` (the existing
`mrv-<stamp>-<scope>-<target>-<hash>` shape) — so ids sort by creation time and validate under `ValidateRunID`.
A fork's child gets a fresh id; `ParentRunID`/`ForkedAtSeq` live in its `init`.

---

## 7. Sequencing contract for writers (what the machine must respect)

`run` enforces these at fold time; the machine spec must produce logs that satisfy them:

1. `init` first; `Seq` contiguous; `Prev` chain intact; `SchemaVersion` constant.
2. Per `node@iter`: zero or more `node_output` events, then at most one `delta_applied`, and no `node_output` after it.
3. `delta_applied.Status` covers `AllFound` exactly.
4. A `Loop` transition's `Iter` is post-increment; every other event's `Iter` equals the current `Iteration`.
5. An `Outcome`-bearing transition is the last non-`fork` event.
6. `ToKind` is supplied by the machine on every transition (empty string when the target state has no node).

---

## 8. Tests (all in `internal/fsm/run`, 100% statement coverage, no other `fsm` package imported)

| # | test | discriminates |
|---|---|---|
| R1 | fold table: one test per event type × rule row in §4 (init, node_output, delta_applied incl. union/first-seen/replace-vs-nil, llm_call+tokens additivity, gate fail/pass, converge stop, warn, overflow_handler, transition non-loop/loop/agent-edit/outcome/failed) | every derivation rule |
| R2 | invariants: each `ERR_AUDIT_*` row (empty, version, non-init first, second init, seq gap, prev mismatch, unknown type, bad payload, output-after-delta, delta-without-output, second delta, status incomplete/extra/duplicate, loop iter mismatch, outcome not last) | fail-closed behavior, exact code + seq in message |
| R3 | determinism: `Fold(e)` twice → byte-equal marshaled snapshots; fold via JSONL store vs mem store → byte-equal | store independence |
| R4 | property: for a valid log, any single-event deletion, duplication, or swap of two adjacent events either yields `ERR_AUDIT_INVALID` or an identical snapshot (never a silently different one) | chain + invariants catch reordering |
| R5 | `Clone`: mutate every slice/map/pointer/RawMessage of the clone; original unchanged (reflect-based deep-equal before/after) | shallow copy |
| R6 | `PrevUnfixed`: value copy — after a loop transition, later `delta_applied` changing `Unfixed` leaves `*PrevUnfixed` unchanged; nil before the first loop transition; survives a JSONL round trip as `null` vs number | aliasing, nil-vs-0 |
| R7 | store: create/append/events round trip; `Prev` chain; fsync path; `O_APPEND` concurrency (two processes → second gets `ERR_RUN_LOCKED`); stale lock recovered; lock refuses symlink; `ERR_EVENT_TOO_LARGE`; dir/file modes `0700`/`0600` | store contract |
| R8 | torn tail: truncate mid-line → `Events()` returns prefix + `TornTail`; next `Append` repairs (file ends with warn + new event; chain valid); torn non-tail → `ERR_AUDIT_INVALID` | repair rule |
| R9 | `List()` ordering and summary fields from init + last transition; `ValidateRunID` accept/reject table (path separators, `..`, length) | id safety |
| R10 | `Origin`-bearing copied events fold identically to natives (a log with `Origin` set on its prefix equals the same log without) | fork transparency |
| R11 | size caps: `Finding.IssueText` > 4 KB, `Bug.Desc` > 2 KB rejected at `delta_applied` decode; `GateError.Detail` truncated at 64 KB with flag | caps enforced, not silent |

Mock/fixture policy: no scenario files here — `run` tests build events in Go. Fixtures for the other specs
(`testdata/fsm/…`) will be produced by helpers exported from `run` (`run/runtest` package: `Builder` for logs).

---

## 9. What the next specs must define (out of scope here, listed so nothing falls between)

- **workflow spec:** ordered transitions with `loop`/`outcome` keys, `loop` count rule per workflow shape (a
  loop-less workflow is valid), `TerminalFor`, exec/kind pairing, var resolution, cmd resolution + canonical
  `cmds_sha256` preimage (sorted JSON of `[{name, argv, file_hashes}]`).
- **machine spec:** `Advance`/`Record`/`Fork` algorithms as producers of §7-conforming logs; `--replace`
  refused after `delta_applied` (`ERR_NODE_OUTPUT_APPLIED`); freeze rule on fork defined over `NodesRun` ∩
  nodes whose YAML references `$K` (covers host nodes); checkpoint = the `transition` event whose `To == S`
  and `Iter == N`, or seq 1 for the initial state; git preconditions using `TransitionData.Head`;
  `ERR_RUN_TERMINAL`; runs.jsonl record; export redaction over **every** event type including `Origin`
  copies; trust-boundary paragraph in the docs.
- **judge/kind spec:** `index` assignment per kind, `RenderPrompt` goldens with pinned harnesseval sha,
  `stripFences` guarded on the ``` prefix, `still-present` `max_tokens` 512 (matching the Python) unless a
  C-row says otherwise, nonce from `crypto/rand`.
- **cli spec:** envelope, exit codes, `fsm state` = `Fold(Events())`, `diff` alignment by `(node, iter, index)`
  with one-sided rows.
