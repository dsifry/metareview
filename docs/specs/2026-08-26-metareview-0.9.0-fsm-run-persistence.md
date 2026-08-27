# metareview 0.9.0 — `internal/fsm/run`: event-sourced run persistence

> **Status:** REVISION 2 (2026-08-26) — re-review pending. First of the split 0.9.0 build artifacts (parent plan:
> [`2026-08-26-metareview-0.9.0-build-plan.md`](2026-08-26-metareview-0.9.0-build-plan.md), r3, escalated as too
> large). This spec covers **only** `internal/fsm/run`: wire types, the event log, `Fold`, the store/lock contract,
> and the trust boundary. Downstream specs (§10) build against it.
>
> **Scope rule:** this document says what `run` *stores and derives*. It does not say when the machine appends an
> event, how a gate is evaluated, how a fork chooses a checkpoint, or what the CLI prints. Where a later spec needs
> a field or event, it is defined here; the rule that uses it belongs to that spec.
>
> **Revision 2** answers review `mrv-20260827-050354…` (8 lenses, NEEDS_REVISION). §0 maps each blocking finding to
> its change; §11 is the amendment ledger for parent-plan contracts this spec supersedes.

---

## 0. Attempt-1 resolution map

| cluster / findings | change |
|---|---|
| RunID/CreatedAt/InitialState unsourced — RC-1, RF-1, RF-2, RA-1, RA-10 | `InitData` carries `run_id`, `created_at`, `initial_state`, `initial_kind`; `Create(runID, first)`; init sets `FixEntryHead` when `initial_kind == agent-edit` (§3.1, §4.1, §5). |
| fork copy — RDM-2, RT-4, RSEC-5, RDM-6 | Copied prefix excludes the parent's `init`; `Origin{run_id, seq, hash}`; provenance invariants (§4.8); `MockTainted` derived; `At` retained (§3, §4.8). |
| outcome-last vs post-terminal events — RI-1, RA-3 | Post-terminal allow-list: `overflow_handler`, `warn`, `record`, `tokens`, `fork`, `tree` (§4.7). |
| torn tail — RSEC-2, RA-4, RDM-3, RDM-4, RF-4, RF-5, RC-2, RT-11 | Torn ≡ final line lacks `\n` only; a complete-but-undecodable line is `ERR_AUDIT_INVALID`/`ERR_AUDIT_VERSION`; `Events()` returns `Log{Events, Torn *TornTail}`; repair is a separate `RepairTail` under the lock that sidecars the bytes; the **machine** appends the `warn` with correct `Iter`/`State`; read-only commands surface `Torn` without repairing (§5.3). |
| `Prev` ownership/preimage — RF-3, RA-5, RSEC-7, RT-2 | Chain verified only in `Events()` over raw stored line bytes; `Fold` never sees `Prev`; `Data` compacted and duplicate keys rejected at append; external-oracle fixture (§3, §5.2, R2b). |
| `Status` invariant — RA-2, RI-6 | Single rule: `Unfixed` = bugs in `AllFound` **not** marked fixed by the current `Status`; unstatused bugs count as still present (fail-closed); coverage checked only when `Status` is supplied (§4.3). |
| `FixEntryHead` on fork — RI-2, RI-5 | New `fix_baseline{head}` event (§3.1, §4.6); trust-boundary example replaced (§1.5). |
| state-name leak — RS-1, RI-3, RA-9, RF-6 | `Outcome` comes only from the payload; no `"failed"` rule; `HasOutgoing` removed (§4.6). |
| lock/CAS/symlink/modes — RSEC-4, RSEC-8, RA-6, RSEC-10 | `flock`-based lock (no stale-pid logic); `Append(…, expectedPrev)` CAS; `O_NOFOLLOW` + `Lstat` on every path component; `runs/` created `0700` and fsync'd (§5.1, §5.2). |
| migration/gitignore/retention — RDM-1, RDM-5, RDM-7, RSEC-1 | `MinReadableVersion`; older-version runs readable, not appendable; self-ignoring `.metareview/runs/.gitignore`; retention "until deleted", `List()` = fold per run with `Error` rows (§5.4, §5.5). |
| caps — RC-6, RF-7, RSEC-6, RT-5, RDM-8 | All caps enforced at **decode** inside `Fold` (`ERR_AUDIT_INVALID` reason `oversize`) and at `Append` for the whole line; truncation of `Detail` is done by the machine via `run.CapDetail`; every payload field has a cap (§2.2). |
| non-loop `Iter`, `index` — RT-7, RA-8 | Enforced for every event (§4.9). |
| `delta_applied` ↔ output — RA-7 | `delta_applied.output_hash` checked against the recorded output (§4.3). |
| tests — RT-1, RT-3, RT-6, RT-8, RT-9, RT-10, RT-12, RT-13 | §8 rewritten: prefix-fold property, re-chained mutation fuzz with expected codes, external oracle, order assertions, reflect-driven `Clone`, store conformance table, typed `FoldError`; `Builder` lives in package `run` (no `runtest`). |
| ledger/scope — RS-2, RS-3, RI-4, RC-5 | §10 ownership ledger for the five split specs; §11 amendment ledger (incl. design §10 diff snapshot → `tree` event with porcelain status). |

---

## 1. Principles

1. **`audit.jsonl` is the only authority.** No `state.json`. Every command derives the snapshot by folding the
   log. Read-only commands write nothing.
2. **`Fold` is pure and total over `run` types.** Events in, snapshot or typed error out. `run` imports no other
   `fsm` package. Anything the machine "sets" is carried by an event payload or derived from earlier events.
3. **Append-only, hash-chained, versioned.** Lines are never rewritten (the single exception is §5.3's repair of
   an unterminated final line, which preserves the removed bytes). A version the reader cannot read or a type it
   does not know is an error, never a skip.
4. **Fail closed.** A log violating an invariant folds to a typed `FoldError`; the run is unusable until inspected.
5. **Trust boundary.** The run directory is `0700`, files `0600`. The hash chain gives **integrity against
   accidental corruption only** — it is unanchored, so it provides no tamper evidence against any deliberate
   write by the same user, including the host agent. The FSM's guarantees are *process* guarantees for a
   cooperating agent that drives it through the CLI. Concretely: "an `advance` refuses a transition whose gate
   fails" holds; "the log cannot be forged" does not. The docs state this with the same prominence as the
   advisory/enforcing caveat (parent plan C18).

---

## 2. Wire types

All persisted types have snake_case JSON tags (the pre-existing `schemaVersion` key is kept for consistency
with `.metareview/*.jsonl`). Payloads decode with `DisallowUnknownFields`; duplicate keys are rejected at append
(§5.2).

```go
package run

const (
    SchemaVersion      = 1   // written by this binary
    MinReadableVersion = 1   // oldest version Fold accepts (§5.4)
)

type State string      // opaque to run
type Outcome string    // fixed | clean | reviewed | stalled | overflow | custom | failed
type Kind string       // opaque to run except the constant KindAgentEdit = "agent-edit" (§4.6)

type Finding   struct { IssueText string `json:"issue_text"`; File string `json:"file,omitempty"`; Line int `json:"line,omitempty"`; Severity string `json:"severity,omitempty"`; Category string `json:"category,omitempty"`; Source string `json:"source,omitempty"` }
type Golden    struct { Comment string `json:"comment"`; Severity string `json:"severity,omitempty"`; Category string `json:"category,omitempty"` }
type Bug       struct { ID string `json:"id"`; Desc string `json:"desc"`; File string `json:"file,omitempty"`; Line int `json:"line,omitempty"`; Verdict string `json:"verdict"`; Confidence float64 `json:"confidence"`; GoldenIdx *int `json:"golden_idx,omitempty"` }
type BugStatus struct { ID string `json:"id"`; StillPresent bool `json:"still_present"`; Confidence float64 `json:"confidence"` }
type TokenTotals struct { Input int64 `json:"input"`; CacheRead int64 `json:"cache_read"`; CacheCreate int64 `json:"cache_create"`; Output int64 `json:"output"`; Reasoning int64 `json:"reasoning"` }
func (t TokenTotals) Add(u TokenTotals) TokenTotals
func (t TokenTotals) Total() int64
func BugID(issueText string) string        // hex(sha1(issueText))[:12]

type GateError struct { Code string `json:"code"`; Gate string `json:"gate"`; Detail string `json:"detail,omitempty"`; DetailTruncated bool `json:"detail_truncated,omitempty"` }
func (e *GateError) Error() string
func CapDetail(detail string) (string, bool)   // truncates to MaxDetail bytes at a UTF-8 boundary; the machine calls this before appending

type AllowedCmd struct { Name string `json:"name"`; Argv []string `json:"argv"`; FileHashes map[string]string `json:"file_hashes"` }

// Delta is what a node's Reduce produced. Defined here because Fold applies it. No token field (§4.4).
type Delta struct { Findings []Finding `json:"findings,omitempty"`; Confirmed []Bug `json:"confirmed,omitempty"`; Status []BugStatus `json:"status,omitempty"`; Commit string `json:"commit,omitempty"` }

type FoldError struct { Code string; Seq int64; Type string; Reason string }   // Code ∈ {ERR_AUDIT_EMPTY, ERR_AUDIT_VERSION, ERR_AUDIT_INVALID}
func (e *FoldError) Error() string
```

### 2.1 Snapshot (derived; never persisted by `run`)

```go
type Snapshot struct {
    SchemaVersion int `json:"schemaVersion"`; RunID string `json:"run_id"`; ParentRunID string `json:"parent_run_id,omitempty"`; ForkedAtSeq int64 `json:"forked_at_seq,omitempty"`
    CreatedAt time.Time `json:"created_at"`; Seq int64 `json:"seq"`; LogVersion int `json:"log_version"`   // version of the events folded
    Workflow string `json:"workflow"`; WorkflowHash string `json:"workflow_hash"`; Vars map[string]string `json:"vars"`; Calibration bool `json:"calibration"`
    Mock string `json:"mock,omitempty"`; MockTainted bool `json:"mock_tainted"`                              // any event with Mock=true while Mock==""
    RepoMode string `json:"repo_mode"`; AllowedCmds []AllowedCmd `json:"allowed_cmds,omitempty"`; CmdsSHA256 string `json:"cmds_sha256,omitempty"`
    RepoRoot string `json:"repo_root"`; WorkDir string `json:"work_dir"`
    State State `json:"state"`; Outcome Outcome `json:"outcome,omitempty"`; Iteration int `json:"iteration"`
    BaseSHA string `json:"base_sha"`; Head string `json:"head"`; FixEntryHead string `json:"fix_entry_head,omitempty"`
    TreeHash string `json:"tree_hash,omitempty"`; TreeStatus string `json:"tree_status,omitempty"`              // from the last tree event (§4.5)
    Goldens []Golden `json:"goldens"`; Findings []Finding `json:"findings"`; Confirmed []Bug `json:"confirmed"`; AllFound []Bug `json:"all_found"`; Status []BugStatus `json:"status"`
    Unfixed int `json:"unfixed"`; PrevUnfixed *int `json:"prev_unfixed"`; Tokens TokenTotals `json:"tokens"`
    NodeOutputs map[string]json.RawMessage `json:"node_outputs"`; Applied map[string]bool `json:"applied"`; NodesRun []string `json:"nodes_run"`
    LastError *GateError `json:"last_error,omitempty"`; StopReason string `json:"stop_reason,omitempty"`; OverflowHandled bool `json:"overflow_handled"`
    Warnings []string `json:"warnings"`
}
func (s Snapshot) Clone() Snapshot         // deep copy of every slice, map, pointer, RawMessage (R5 is reflect-driven)
func Key(node string, iter int) string     // "node@iter"
```
Slices and maps are never nil in a folded snapshot (they marshal as `[]`/`{}`).

### 2.2 Caps (enforced at decode inside `Fold`, reason `oversize`; whole line ≤ `MaxLine` at `Append`)

| field | cap | | field | cap |
|---|---|---|---|---|
| `Finding.IssueText` | 4 KB | | `Bug.Desc` | 2 KB |
| `Golden.Comment` | 4 KB; `Goldens` ≤ 512 | | `Vars` | ≤ 64 entries, key ≤ 128 B, value ≤ 4 KB |
| `GateError.Detail` | 64 KB (`MaxDetail`; machine truncates via `CapDetail`) | | `AllowedCmd.FileHashes` | ≤ 256; `Argv` ≤ 64 × 4 KB |
| `cmd_call.stdout` / `stderr` | 64 KB / 8 KB (machine truncates; `stdout_truncated` flag) | | `llm_call.verdict`, `node_output.output`, `record.data` | 512 KB each |
| `tree.status` | 64 KB (`status_truncated` flag) | | `Event` line | `MaxLine` = 1 MB |

---

## 3. Events

```go
type Event struct {
    SchemaVersion int             `json:"schemaVersion"`
    Seq           int64           `json:"seq"`                 // 1-based, contiguous
    Prev          string          `json:"prev"`                // hex sha256 of the previous stored line's bytes (excluding '\n'); "" at seq 1
    At            time.Time       `json:"at"`                  // set by the machine's Clock; copied events keep their original At
    Type          string          `json:"type"`
    State         State           `json:"state,omitempty"`     // snapshot.State when appended (set by the machine)
    Iter          int             `json:"iter"`                // snapshot.Iteration when appended; post-increment for loop transitions (§4.9)
    Node          string          `json:"node,omitempty"`
    Mock          bool            `json:"mock,omitempty"`
    Origin        *Origin         `json:"origin,omitempty"`    // events copied from a parent run by a fork
    Data          json.RawMessage `json:"data"`                // compacted at append (§5.2)
}
type Origin struct { RunID string `json:"run_id"`; Seq int64 `json:"seq"`; Hash string `json:"hash"` }   // Hash = sha256 of the parent's stored line
```
Field ownership: the **machine** sets `SchemaVersion`, `At`, `Type`, `State`, `Iter`, `Node`, `Mock`, `Origin`,
`Data`; the **store** sets `Seq` and `Prev` (and verifies them on read). Copied events are appended verbatim (all
machine-set fields retained, including `At` and `SchemaVersion`) with the store assigning new `Seq`/`Prev` and the
machine setting `Origin`. The parent's `init` is **never** copied (§4.8).

### 3.1 Event types and payloads

| `Type` | payload (Go struct in `run`) | node-scoped | notes |
|---|---|---|---|
| `init` | `InitData{RunID, CreatedAt, Workflow, WorkflowHash, Vars, Calibration, Mock, RepoMode, AllowedCmds, CmdsSHA256, RepoRoot, WorkDir, BaseSHA, Head, InitialState State, InitialKind Kind, Goldens, ParentRunID, ForkedAtSeq}` | no | exactly one, at seq 1 |
| `tree` | `TreeData{Head, TreeHash, Status string, StatusTruncated bool}` | no | working-tree snapshot (`git status --porcelain=v2 --untracked-files=all`); appended by the machine at each advance |
| `needs_input` | `{}` | yes | informational |
| `node_output` | `NodeOutputData{Output json.RawMessage}` | yes | |
| `delta_applied` | `DeltaAppliedData{Delta; OutputHash string}` | yes | `OutputHash` = sha256 of the compacted recorded output |
| `llm_call` | `LLMCallData{Kind, Model, Effort string; Index int; InputHash string; Verdict json.RawMessage; Confidence float64; Tokens TokenTotals; DurationMS int64; Error string}` | yes | `Index` contiguous from 0 within `node@iter` |
| `cmd_call` | `CmdCallData{Name string; Argv []string; InputHash, Stdout, Stderr string; StdoutTruncated, StderrTruncated bool; ExitCode int; DurationMS int64; Error string}` | optional | |
| `gate` | `GateData{Name string; Passed bool; Error *GateError}` | no | |
| `converge` | `ConvergeData{Atom string; Class Outcome; Stop bool; Reason string}` | no | |
| `transition` | `TransitionData{From, To State; Gate string; Outcome Outcome; Loop bool; ToKind Kind; Head string}` | no | checkpoint event |
| `fix_baseline` | `FixBaselineData{Head string}` | no | sets `FixEntryHead` (used by the fork spec) |
| `tokens` | `TokenTotals` | no | host-reported; additive |
| `record` | `RecordData{Name string; Data json.RawMessage}` | no | user events; `Type` is always `record` |
| `warn` | `WarnData{Code, Detail string}` | no | |
| `overflow_handler` | `OverflowHandlerData{ExitCode int; DurationMS int64; Error string}` | no | |
| `fork` | `ForkData{ChildRunID string; AtSeq int64}` | no | appended to the parent |

---

## 4. `Fold`

```go
func Fold(events []Event) (Snapshot, error)    // error is always *FoldError
```
Total, pure, deterministic (same slice → byte-identical `json.Marshal`). `Fold` never sees line bytes and does
**not** verify `Prev` (that is `Events()`, §5.2). Errors:

| code | when |
|---|---|
| `ERR_AUDIT_EMPTY` | no events |
| `ERR_AUDIT_VERSION` | any `SchemaVersion < MinReadableVersion` or `> SchemaVersion` |
| `ERR_AUDIT_INVALID` | first event not `init`; a later `init`; `Seq` not contiguous from 1; unknown `Type`; undecodable payload; `oversize`; any invariant in §4.2–4.9 |

`Snapshot.LogVersion` = the maximum `SchemaVersion` seen (§5.4).

### 4.1 `init` — copies every `InitData` field into the snapshot; `State ← InitialState`; `Iteration ← 0`;
`PrevUnfixed ← nil`; if `InitialKind == KindAgentEdit`: `FixEntryHead ← Head`. Maps/slices initialized empty.

### 4.2 `node_output` — `k = Key(Node, Iter)`. **Invariant:** `!Applied[k]`. `NodeOutputs[k] ← Output` (a later
`node_output` before the delta replaces an earlier one). Append `k` to `NodesRun` if absent.

### 4.3 `delta_applied` — **Invariants:** `NodeOutputs[k]` exists; `!Applied[k]`; `OutputHash ==
sha256(NodeOutputs[k])`. Then: `Findings ← Delta.Findings` if non-nil; `Confirmed ← Delta.Confirmed` if non-nil
and each `Bug` whose `ID` is absent from `AllFound` is appended to `AllFound` (first-seen order); if
`Delta.Status != nil`: **invariant** its `ID`s are a subset of `AllFound` with no duplicates, then `Status ←
Delta.Status`. In all cases recompute: `Unfixed ← |{b ∈ AllFound : no s ∈ Status with s.ID == b.ID && !s.StillPresent}|`
— a bug with no status entry, or with `StillPresent`, counts as unfixed (fail closed). `Applied[k] ← true`.

### 4.4 `llm_call`, `tokens` — `Tokens ← Tokens.Add(payload tokens)`. The only two token sources.

### 4.5 `tree` — `Head ← Head`; `TreeHash ← TreeHash`; `TreeStatus ← Status`.

### 4.6 `transition` — `State ← To`; `Head ← Head`; `LastError ← nil`. If `Loop`: `Iteration++`; `v := Unfixed;
PrevUnfixed ← &v`; `Findings`, `Confirmed` ← empty (everything else kept). If `ToKind == KindAgentEdit`:
`FixEntryHead ← Head`. If `Outcome != ""`: `Outcome ← Outcome` (**run derives no outcome from any state name; the
machine supplies it, including `failed`**).
`fix_baseline` — `FixEntryHead ← Head`.

### 4.7 Terminal rule — `Outcome != ""` is terminal. After the terminal transition only these types are legal:
`overflow_handler`, `warn`, `record`, `tokens`, `tree`, `fork`; anything else → `ERR_AUDIT_INVALID` (`post_terminal`).
Other rules: `gate{Passed:false}` → `LastError ← Error`; `converge{Stop}` → `StopReason ← Atom`; `warn` → append
`Code` to `Warnings`; `overflow_handler` → `OverflowHandled ← true`; `record`, `needs_input`, `cmd_call`, `fork` →
no change.

### 4.8 Provenance invariants (fork copies) — `Origin`-bearing events, if any, must (a) be exactly the events with
`2 ≤ Seq ≤ ForkedAtSeq' + 1` where `ForkedAtSeq'` is the count of copied events (i.e. a contiguous block right
after `init`), (b) all have `Origin.RunID == ParentRunID`, (c) have strictly increasing `Origin.Seq` ≤ `ForkedAtSeq`,
(d) never be of type `init`. `ParentRunID == ""` ⇒ no `Origin` events. `Origin.Hash` is verified by the **fork
spec** at copy time against the parent's stored lines (run exposes `LineHash(line []byte)`); `Fold` only checks
it is non-empty. `MockTainted ← true` if any event has `Mock` and `Snapshot.Mock == ""`.

### 4.9 Stamp invariants (every event) — `Iter` must equal the current `Iteration` (for a `Loop` transition, the
post-increment value); `State` must equal the current `State` (for a `transition`, the `From` state); `llm_call.Index`
must be contiguous from 0 per `node@iter` in append order. Violations → `ERR_AUDIT_INVALID` (`stamp`).

---

## 5. Store

```go
type TornTail struct { Offset int64; Bytes []byte }               // an unterminated final line
type Log struct { Events []Event; Torn *TornTail; Lines [][]byte } // Lines = raw stored lines (for hashing/oracles)
type RunSummary struct { RunID, Workflow string; CreatedAt time.Time; State State; Outcome Outcome; ParentRunID string; Mock bool; MockTainted bool; Torn bool; Error string }

type RunStore interface {
    Create(runID string, first Event) error                          // first.Type == "init" and InitData.RunID == runID
    Append(runID string, ev Event, expectedPrev string) (seq int64, err error)   // CAS on the current last-line hash
    Events(runID string) (Log, error)                                // verifies chain; reports torn tail
    RepairTail(runID string, torn TornTail) error                    // under the lock; sidecars then truncates (§5.3)
    List() ([]RunSummary, error)
    Lock(runID string) (unlock func(), err error)                    // flock(LOCK_EX|LOCK_NB) on <run>/lock
    Root() string
}
func NewJSONLStore(root string) RunStore        // runs at <root>/.metareview/runs/<id>/
func NewMemStore() RunStore                     // same contract; holds serialized lines; passes the §8 conformance table
func ValidateRunID(id string) error             // ^mrv-[A-Za-z0-9-]{8,200}$ ; every store method calls it before any path join
func LineHash(line []byte) string               // hex sha256 of a stored line (no '\n')
```

### 5.1 Layout, paths, modes
`<root>/.metareview/runs/` is created `0700` with a `.gitignore` containing `*` (self-ignoring: the directory
is transient regardless of the consumer repo's `.gitignore` policy; durable copies are the fork/CLI spec's `export`).
`<root>/.metareview/runs/<id>/` is `0700`; `audit.jsonl`, `lock`, and any `audit.torn-<seq>.bin` are `0600`.
Every path component below `<root>` is `Lstat`-checked to be a real directory (no symlinks); files are opened with
`O_NOFOLLOW`. A symlink anywhere → `ERR_STORE_PATH`. After `Create`, the run directory and `runs/` are fsync'd.
`RepoRoot` is fixed at init; a fork's `--work-dir` never moves the store.

### 5.2 Write path and chain
`Append`: validate `ev.Data` (`json.Valid`, duplicate-key rejection via a streaming decoder, then `json.Compact`);
marshal the event with `Seq = last+1`, `Prev = LineHash(lastLine)`; refuse if `expectedPrev != Prev` (`ERR_AUDIT_CAS`)
or if the caller does not hold the lock (`ERR_RUN_LOCKED`; the store tracks the lock it issued per process); refuse if
`len(line) > MaxLine` (`ERR_EVENT_TOO_LARGE`; nothing written, `Seq` not consumed). Write `line + "\n"` with `O_APPEND`,
`fsync`. `Events()`: read all lines; verify `Seq` contiguity and `Prev == LineHash(previous line)` for every complete
line (`ERR_AUDIT_CHAIN` with the seq); decode each into `Event` (undecodable complete line → `ERR_AUDIT_INVALID`,
unreadable version → `ERR_AUDIT_VERSION`). The chain preimage is the **raw stored line bytes**; readers never re-marshal.

### 5.3 Torn tail
Torn ≡ the file's final bytes after the last `\n` are non-empty. Those bytes are returned as `Log.Torn` and are
not decoded, whatever they contain (a complete JSON line missing only its `\n` is still torn: it was never
durably terminated). `Events()` never modifies the file. Repair: the machine, holding the lock, calls
`RepairTail(runID, torn)`; the store verifies the file still ends with exactly `torn.Bytes` at `torn.Offset`, writes
them to `audit.torn-<nextSeq>.bin`, truncates to `torn.Offset`, fsyncs; the machine then appends
`warn{Code: "AUDIT_TORN_LINE_DROPPED", Detail: "<bytes> bytes preserved in audit.torn-<seq>.bin"}` with correct
`Iter`/`State` (it holds the fold). Read-only commands report `Torn` in their output and do not repair.

### 5.4 Versioning
`Fold` accepts events with `MinReadableVersion ≤ SchemaVersion ≤ run.SchemaVersion`. A run whose `LogVersion <
SchemaVersion` is readable (`fsm state`, `export`, `diff`) but **not appendable**: `Append` refuses with
`ERR_AUDIT_UPGRADE` until a `Migrate(Log) (Log, error)` for that version exists (none needed at v1). A log with any
event `> SchemaVersion` is `ERR_AUDIT_VERSION` (older binary, newer log). A version bump therefore never destroys a
run; it freezes it.

### 5.5 `List()` and retention
`List()` folds every conforming run directory (non-conforming names are skipped); a run that fails `Events()`/`Fold`
yields a `RunSummary` with `Error` set (never aborts the listing). Cost is O(total bytes) — acceptable at the
expected scale (tens of runs); no index in 0.9.0. Retention: logs are kept until the user deletes the directory;
no compaction.

### 5.6 Lock
`Lock` opens `<run>/lock` (`O_CREAT|O_RDWR|O_NOFOLLOW`, `0600`) and takes `flock(LOCK_EX|LOCK_NB)`; failure →
`ERR_RUN_LOCKED`. The OS releases the lock on process exit, so there is no stale-lock logic and no pid file. The lock
is per process; a second `Lock` in the same process also fails (`flock` on a second descriptor). `Append` and
`RepairTail` require it.

---

## 6. Run IDs
`RunID(workflow string, at time.Time) string` = `state.RunID("fsm-"+workflow, workflow, at)`; `InitData.RunID` carries it.

## 7. Writer contract (the machine spec must produce logs satisfying §4.2–4.9)
Summarized: `init` first with all fields; per `node@iter` outputs then one delta with matching `output_hash`;
`Status` ⊆ `AllFound`; every event stamped with the current `Iter`/`State`; `Outcome` only from the payload; post-
terminal allow-list; fork copies exclude `init`, are contiguous after the child's `init`, and carry `Origin.Hash`;
`ToKind` supplied on every transition (empty when the target has no node); `Detail`/stdout/stderr capped via `run`
helpers before append.

---

## 8. Tests (`internal/fsm/run`, 100% statements, imports no other `fsm` package; `Builder` is part of the package)

| # | test | discriminates |
|---|---|---|
| R1 | fold table: one row per rule in §4.1–4.7 incl. negative/second-instance cases (`ToKind != agent-edit` leaves `FixEntryHead`; second loop resets `PrevUnfixed` again; `LastError ← nil`; `converge.Class` ignored; `Status` nil keeps prior `Status` but `Unfixed` recomputed over grown `AllFound`; `fix_baseline`; `tree`) | every derivation |
| R1b | no-op rows: `record`, `needs_input`, `cmd_call`, `fork` each fold to a snapshot byte-equal to before (and `NodesRun` unchanged for `needs_input`) | §4.7 no-ops |
| R2 | invariants: one row per `FoldError` reason (empty, version low/high, non-init first, second init, seq gap, unknown type, bad payload, each `oversize` field, output-after-delta, delta-without-output, second delta, output-hash mismatch, status not subset/duplicate, post-terminal, provenance a–d, stamp iter/state/index) asserting `FoldError{Code, Seq, Type, Reason}` fields | fail-closed with exact typed error |
| R2b | external oracle: a committed `testdata/run/oracle.jsonl` whose `prev` values were produced by `sha256sum` outside Go; `Events()` accepts it and rejects a one-byte-modified copy with `ERR_AUDIT_CHAIN` at the right seq; `LineHash` matches the oracle values | chain preimage = raw bytes |
| R3 | order: `AllFound` first-seen, `NodesRun` first-run, `Warnings`, `Status` replacement — each with ≥ 3 elements inserted in an order that is neither sorted nor reverse-sorted; repeated folds byte-equal; fold from JSONL store vs mem store byte-equal | map-ranging bugs |
| R4 | prefix property: for every n, `Fold(events[:n])` succeeds and equals `Apply(Fold(events[:n-1]), events[n-1])` (`Apply` is the single-step function `Fold` is built on); mutation fuzz over a re-chained log (delete/duplicate/swap then recompute `Seq`/`Prev`) asserting the specific `FoldError.Reason` expected for each mutation class, or byte-equality for the no-op classes | fold is compositional; each invariant catches its mutation |
| R5 | `Clone`: driven by `reflect.VisibleFields`; fails if any slice/map/pointer/RawMessage field is not exercised; mutates in place and asserts the original unchanged, and vice versa | shallow copy |
| R6 | `PrevUnfixed`: nil before the first loop; value copy (later `Unfixed` change leaves it); JSONL round trip `null` vs number | aliasing, nil-vs-0 |
| R7 | store conformance table run against **both** stores: create/append/events round trip; `expectedPrev` CAS refusal; `Append` without `Lock` → `ERR_RUN_LOCKED`; second `Lock` → `ERR_RUN_LOCKED`; `ERR_EVENT_TOO_LARGE` leaves bytes and seq unchanged; duplicate-key and non-compact `Data` handling; `ValidateRunID` on every method. JSONL-only rows: modes `0700`/`0600`, `.gitignore` present, symlinked component → `ERR_STORE_PATH`, fsync path, `flock` released on close | store contract on both |
| R8 | torn tail: (a) mid-line truncation, (b) a complete JSON line missing `\n`, (c) a torn tail that is a valid prefix of a longer line — all → `Torn` set, `Events` unchanged file; `RepairTail` sidecars exact bytes, truncates, refuses if the tail changed; a complete-but-undecodable last line → `ERR_AUDIT_INVALID` (not torn); an unreadable-version last line → `ERR_AUDIT_VERSION` | repair rule; fail-closed on decode |
| R9 | `List()`: ordering; `Error` rows for a corrupt run; non-conforming dirs skipped; `RunID` shape table | listing never aborts |
| R10 | fork log: build parent, copy `[2..k]` with `Origin` (via `Builder.CopyFrom`), fold child; equality of the derived state with the parent's prefix fold except `RunID`/`ParentRunID`/`ForkedAtSeq`/`CreatedAt`; each provenance violation (init copied, gap, wrong parent id, non-monotonic seq, seq > ForkedAtSeq) → `ERR_AUDIT_INVALID`; `MockTainted` when a mock event is copied into a non-mock child | provenance |
| R11 | versioning: `LogVersion`; `Append` on an older-version log → `ERR_AUDIT_UPGRADE`; newer-version event → `ERR_AUDIT_VERSION` | freeze-not-destroy |
| R12 | caps: each §2.2 field one over its cap → `oversize` at the right seq; `CapDetail` truncates at a UTF-8 boundary and sets the flag; `MaxLine` at append | caps enforced with codes |
| R13 | `Builder` itself: every method exercised by R1–R12 (it is in-package and under the gate) | helper correctness |

---

## 9. Sequencing of what `run` exposes to later specs
`Fold`, `Apply`, `Builder`, `LineHash`, `CapDetail`, `BugID`, `Key`, `ValidateRunID`, `RunID`, all payload types,
`RunStore` + both implementations. Nothing else.

## 10. Ownership ledger for the split (five specs; this is #1)

| # | spec | owns (from parent plan r3) | open findings it must close |
|---|---|---|---|
| 1 | **run** (this) | §1.2, §1.4, §1.7 persistence half, R-tests | attempt-3: ARC3-1/2/3/4/6/10, DM3-2/4, CMP3-2/4/5/6 (run half), SEC-21/22 (run half), FIN-1/2 (run half)/6/7, TQ3-1/9 |
| 2 | **workflow + gate + converge** (deterministic core) | §1.3 gate/converge/workflow types, `loop`/`outcome` keys, `TerminalFor`, exec/kind pairing, `AllFixed`, atoms with `Class()`, `PrevUnfixed == nil` rule, canonical `cmds_sha256` preimage, the `Advance` algorithm as a producer of §7-conforming logs, `Record` algorithm, `ERR_NODE_OUTPUT_APPLIED`, `TreeHash` preimage + `UNSANCTIONED_EDIT` rule, gate-set registry incl. `findings_empty`/`confirmed_empty` codes | CMP3-1/3/4/5/6/7/8, ARC3-5/7/8/9, INT-16 (via `fix_baseline`), TQ3-10/11, SCP3-4/5, RI-7 |
| 3 | **fork / resume / diff / export** | `Fork` over seq positions, freeze rule over `NodesRun` ∩ nodes referencing `$K`, git preconditions using `TransitionData.Head` + `fix_baseline`, `--work-dir` validation (must be a worktree of the run repo), `Origin.Hash` verification, `diff` alignment with one-sided rows, `export` redaction over every event type incl. `Origin` copies with a manifest, runs.jsonl record (`fsmRunRecord` matching the existing writer schema, verdict mapping, parent row written at fork time), `ESCALATED` mapping | INT-9/10/13/14/18/19, FEA-N3/N4, DM3-1/3/5, DMN-1, SEC-13/22/27, FIN-6, TQ3-2/3/7, SCP3-1/2/3/8, RDM-6, RSEC-5 (fork half) |
| 4 | **guardrails + judge + kinds + mockai** | §1.8 (two-step sha, `FileHashes` closure, re-consent on fork), §2 (prompts, fencing with `crypto/rand` nonce, `--no-fence` scoped to `--calibration`, `stripFences` guarded, `still-present max_tokens` decision with a C-row, goldens with pinned harnesseval sha + committed snippets), kinds/`Executor`/`Delta` producers, match-then-adjudicate composition + `Bug.Verdict` vocabulary, `index` assignment, scenario keying, `JUDGE_EFFORT` requiredness | INT-11/12/17/20/21/22, SEC-11/12/14/23..29, FIN-3/4/5, CMP3-3, TQ3-8 |
| 5 | **CLI + tests + milestones + docs** | §3 envelope/exit codes/all subcommand shapes (`state`, `judge`, `converge`, `workflows`, `status` FSM section), `test-fsm.sh`, E1–E15 rewritten against specs 1–4, coverage gate wiring + CI, forbidden-phrase grep over the M8 file list, AGENTS/CLAUDE/README/INSTALL/quickstart amendments (exit 3, path-less exit 1, `.metareview/runs/` transient + `gitpolicy.go` reconcile, trust boundary, enforcing caveat, judge-swap limitation, spec amendments for §10.1/§14.3/§17), `package.json` files incl. `go.sum`, milestones/orchestration, version bump last | CMP3-7, TQ3-4/5/6 (done in tree), SCP3-6/7, INT-15/23, DM3-6/7/8, SEC-30, FIN-8, RDM-5 (docs half) |

## 11. Amendment ledger (parent plan r3 contracts this spec supersedes — accepted with the split decision)

| parent contract | change here |
|---|---|
| C21 `state.json` cache | removed; no cache (§1.1). |
| §1.2 `replay` wrapper event | removed; verbatim copies with `Origin{run_id, seq, hash}` (§3). |
| §1.7.4 `llm_call.vars_used` | removed; freeze rule to be defined over `NodesRun` (spec 3). |
| §1.3 `Delta.Tokens` | removed; tokens only from `llm_call`/`tokens` (§4.4). |
| C15 "user names never become `Event.Type`" | unchanged for `record <name>`; the reserved CLI subcommands `record tokens` / `record node-output` map to types `tokens` / `node_output` (spec 5 documents the mapping). |
| design §10 "working-tree diff snapshots at each advance" | satisfied as the `tree` event carrying porcelain status (file list + states, ≤ 64 KB), not a full diff (§3.1). |
| §1.5 step 8 `FixEntryHead ← head` (imperative) | derived from `transition.ToKind` / `init.InitialKind` / `fix_baseline` (§4.1, §4.6). |
| §1.5/§1.6 outcome `failed` on `→failed` | supplied by the machine in the payload; `run` knows no state names (§4.6). |
