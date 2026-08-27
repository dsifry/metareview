# metareview 0.9.0 — `internal/fsm/run`: event-sourced run persistence

> **Status:** REVISION 3 (2026-08-26) — re-review pending (attempt 3 of the chain). First of the five split
> 0.9.0 build artifacts (parent plan: [`2026-08-26-metareview-0.9.0-build-plan.md`](2026-08-26-metareview-0.9.0-build-plan.md),
> r3, escalated as too large; Dave chose "split the target" on 2026-08-26). This spec covers **only** `internal/fsm/run`.
>
> **Scope rule:** this document says what `run` *stores and derives*, and what a writer must produce. It does not say
> when the machine appends an event, how a gate is evaluated, how a fork chooses a checkpoint, or what the CLI prints.
>
> **Revision 3** answers review `mrv-20260827-051457…` (attempt 2). §0 maps each blocking finding to its change.
> §11 is the amendment ledger; §12 the ownership ledger.

---

## 0. Attempt-2 resolution map

| cluster / findings | change |
|---|---|
| validate-before-write, `Apply`, fold state — RA2-1, RA2-3, RF2-2, RF2-4, RT2-4, RT2-5 | `FoldState` (§4) carries the accumulators `Snapshot` lacks; `Apply(FoldState, Event) (FoldState, error)` declared; `Fold = reduce(Apply)`; `Append(runID, st FoldState, ev)` runs `Apply` **before** writing and refuses on any fold error (`ERR_APPEND_REJECTED`), returns the new head hash; `Snapshot.Seq` rule stated; no-op rows compare `Snapshot` minus `Seq` (§4, §5.2, §8). |
| HTML escaping / canonical bytes — RF2-1, RSEC2-9, RSEC2-5 | `Canonical(raw)` (validate, reject duplicate keys at every depth, compact, **`SetEscapeHTML(false)`**) is the only serialization path; `OutputHash = sha256(Canonical(output))`; stored bytes are exactly the canonical bytes; caps measure canonical bytes (§2.3, §5.2). |
| fork provenance — RA2-2, RDM2-4, RC2-2, RSEC2-3 | `ForkedAtSeq` = parent seq of the last copied event; copied block is exactly child seqs `2..ForkedAtSeq` with `Origin.Seq == child.Seq` (contiguous from parent 2) (§4.8). |
| `fix_baseline` fail-closed — RI2-1 | `FixEntryHead` is never set from an `Origin`-bearing `transition`/`init`; `fix_baseline.Head` must equal `Snapshot.Head` and `StateKind == agent-edit` (§4.6). |
| `Status` coverage — RI2-2, RC2-3(ii), RS2-1 | Restored: when `Delta.Status` is supplied it must cover `AllFound` exactly once (`status_incomplete`); `Unfixed` stays fail-closed between deltas (§4.3). |
| store errors, `Seq`/`SchemaVersion`, `init` base case, `NodesRun` — RC2-1, RC2-4, RC2-8, RF2-5, RA2-6, RA2-9 | `StoreError{Code, Seq, Detail}` with enumerated codes (§2.4); `Snapshot.Seq`/`SchemaVersion` rules (§4.1, §4.10); `init` stamp base case (§4.9); `NodesRun` holds node **names** (§2.1). |
| versioning — RF2-3, RDM2-1, RDM2-2, RI2-4 | v1 = exact match; the v2 contract is `Migrate` writing `audit.v2.jsonl` beside the original (never in place; originals and children's `Origin.Hash` untouched); no unreachable branch at v1 (§5.4). |
| torn tail — RSEC2-1, RSEC2-4, RDM2-3, RA2-8, RC2-5 | `Append` refuses a torn log (`ERR_AUDIT_TORN`); `RepairTail`: `O_EXCL` sidecar `audit.torn-<seq>-<unixnano>.bin`, fsync file + dir, then truncate + fsync; a run whose durable prefix is empty is deleted by `RepairTail` (it never existed); `RunSummary.Sidecars` (§5.3). |
| time encoding — RDM2-5 | `run.Time` marshals as UTC RFC3339Nano; zero `At` is an error; copied events keep the parent's `At` (non-monotonic across the seam, stated) (§2.2). |
| caps — RT2-7, RC2-7, RDM2-6 | Every string field capped (`MaxShort` for names/codes/paths); payload caps 256 KB so canonical bytes always fit `MaxLine`; `Goldens`/`AllowedCmds` counts capped; one test row at-cap and one over per cap (§2.3, R12). |
| `MockTainted`, allow-list — RSEC2-2, RI2-3 | Producer contract: every non-`init` event of a mock run carries `Mock: true` (invariant); `cmd_call` added to the post-terminal allow-list; mirrored test rows (§4.7, §4.8, §7). |
| tests — RT2-1..12 | §8 rewritten (see rows). `Builder` uses literal stamps with per-field overrides; per-prefix golden snapshots; mutation-class table; recursive `Clone` walk; store refusal rows; `List` order; value pins; `Reason`/`Code` constants. |
| ledgers/scope — RS2-2..8, RC2-9, RC2-10 | §11 extended (ExecMode, ValidateRunID, Detail, tree_hash, List order, Decoder re-Decode rehomed, five-spec refinement); §12 is a partition; `tree` cadence and CLI wording moved to §7/§12 (`tree` here is only a carrier). |
| `Log.Lines` memory — RF2-7, RSEC2-6 | `Events()` returns events + `Head`; `EventsWithLines()` opt-in; per-run `MaxEvents` (§2.3, §5). |

---

## 1. Principles

1. **`audit.jsonl` is the only authority.** No cache. Every command derives the snapshot by folding the log.
2. **`Fold` is pure and total over `run` types**; `Fold = reduce(Apply)`. `run` imports no other `fsm` package.
3. **Append-only, hash-chained, versioned, validated before write.** A line is written only after `Apply` accepts
   the event against the current fold state, so a log produced through `Append` is always foldable. The single
   byte-removing operation is §5.3's repair of an unterminated tail, which preserves the removed bytes.
4. **Fail closed.** Any invariant violation is a typed error; no best-effort snapshots.
5. **Trust boundary.** Directory `0700`, files `0600`. The hash chain provides **integrity against accidental
   corruption only** — it is unanchored, so it gives no tamper evidence against any deliberate write by the same
   user, including the host agent. The FSM's guarantees are *process* guarantees for a cooperating agent that
   drives it through the CLI ("an `advance` refuses a transition whose gate fails" holds; "the log cannot be
   forged" does not). Stated in the docs with the prominence of the advisory/enforcing caveat (plan C18). Local
   filesystems only (`flock`, `O_APPEND` atomicity); network filesystems are unsupported.

---

## 2. Wire types

All persisted types have snake_case JSON tags (`schemaVersion` kept for consistency with `.metareview/*.jsonl`).
Payloads decode with `DisallowUnknownFields`. Every serialization goes through `Canonical` (§2.3).

```go
package run

const SchemaVersion = 1

type State string; type Outcome string; type Kind string
const ( OutcomeFixed Outcome = "fixed"; OutcomeClean = "clean"; OutcomeReviewed = "reviewed"; OutcomeStalled = "stalled"; OutcomeOverflow = "overflow"; OutcomeCustom = "custom"; OutcomeFailed = "failed" )
const KindAgentEdit Kind = "agent-edit"     // the only kind run interprets

type Finding   struct { IssueText string `json:"issue_text"`; File string `json:"file,omitempty"`; Line int `json:"line,omitempty"`; Severity string `json:"severity,omitempty"`; Category string `json:"category,omitempty"`; Source string `json:"source,omitempty"` }
type Golden    struct { Comment string `json:"comment"`; Severity string `json:"severity,omitempty"`; Category string `json:"category,omitempty"` }
type Bug       struct { ID string `json:"id"`; Desc string `json:"desc"`; File string `json:"file,omitempty"`; Line int `json:"line,omitempty"`; Verdict string `json:"verdict"`; Confidence float64 `json:"confidence"`; GoldenIdx *int `json:"golden_idx,omitempty"` }
type BugStatus struct { ID string `json:"id"`; StillPresent bool `json:"still_present"`; Confidence float64 `json:"confidence"` }
type TokenTotals struct { Input int64 `json:"input"`; CacheRead int64 `json:"cache_read"`; CacheCreate int64 `json:"cache_create"`; Output int64 `json:"output"`; Reasoning int64 `json:"reasoning"` }
func (t TokenTotals) Add(u TokenTotals) TokenTotals; func (t TokenTotals) Total() int64
func BugID(issueText string) string        // hex(sha1(issueText))[:12]
func Key(node string, iter int) string     // node + "@" + strconv.Itoa(iter)

type GateError struct { Code string `json:"code"`; Gate string `json:"gate"`; Detail string `json:"detail,omitempty"`; DetailTruncated bool `json:"detail_truncated,omitempty"` }
func (e *GateError) Error() string
func CapDetail(detail string) (string, bool)          // MaxDetail, UTF-8 boundary
func CapText(s string, max int) (string, bool)        // generic helper for cmd stdout/stderr, tree status

type AllowedCmd struct { Name string `json:"name"`; Argv []string `json:"argv"`; FileHashes map[string]string `json:"file_hashes"` }
type Delta struct { Findings []Finding `json:"findings,omitempty"`; Confirmed []Bug `json:"confirmed,omitempty"`; Status []BugStatus `json:"status,omitempty"`; Commit string `json:"commit,omitempty"` }
```

### 2.1 Snapshot (derived)

```go
type Snapshot struct {
    SchemaVersion int `json:"schemaVersion"`     // == run.SchemaVersion
    RunID string `json:"run_id"`; ParentRunID string `json:"parent_run_id,omitempty"`; ForkedAtSeq int64 `json:"forked_at_seq,omitempty"`
    CreatedAt Time `json:"created_at"`; Seq int64 `json:"seq"`                                    // Seq = seq of the last event folded
    Workflow string `json:"workflow"`; WorkflowHash string `json:"workflow_hash"`; Vars map[string]string `json:"vars"`; Calibration bool `json:"calibration"`
    Mock string `json:"mock,omitempty"`; MockTainted bool `json:"mock_tainted"`
    RepoMode string `json:"repo_mode"`; AllowedCmds []AllowedCmd `json:"allowed_cmds"`; CmdsSHA256 string `json:"cmds_sha256,omitempty"`
    RepoRoot string `json:"repo_root"`; WorkDir string `json:"work_dir"`
    State State `json:"state"`; StateKind Kind `json:"state_kind,omitempty"`; Outcome Outcome `json:"outcome,omitempty"`; Iteration int `json:"iteration"`
    BaseSHA string `json:"base_sha"`; Head string `json:"head"`; FixEntryHead string `json:"fix_entry_head,omitempty"`
    TreeHash string `json:"tree_hash,omitempty"`; TreeStatus string `json:"tree_status,omitempty"`
    Goldens []Golden `json:"goldens"`; Findings []Finding `json:"findings"`; Confirmed []Bug `json:"confirmed"`; AllFound []Bug `json:"all_found"`; Status []BugStatus `json:"status"`
    Unfixed int `json:"unfixed"`; PrevUnfixed *int `json:"prev_unfixed"`; Tokens TokenTotals `json:"tokens"`
    NodeOutputs map[string]json.RawMessage `json:"node_outputs"`; Applied map[string]bool `json:"applied"`; NodesRun []string `json:"nodes_run"`   // node NAMES, first-run order, unique
    LastError *GateError `json:"last_error,omitempty"`; StopReason string `json:"stop_reason,omitempty"`; OverflowHandled bool `json:"overflow_handled"`
    Warnings []string `json:"warnings"`
}
func (s Snapshot) Clone() Snapshot                     // deep copy through every slice element, map value, pointer target, RawMessage
func SnapshotEqualIgnoringSeq(a, b Snapshot) bool     // used by no-op test rows
```
Slices and maps are never nil after `Fold`.

### 2.2 Time
`type Time struct{ time.Time }` marshals as `t.UTC().Format(time.RFC3339Nano)` and unmarshals only that form
(offset must be `Z`); a zero `Time` in `Event.At` or `InitData.CreatedAt` is `stamp` error. Copied events keep the
parent's `At`; `At` is therefore non-monotonic across a fork seam and must not be differenced across it.

### 2.3 Canonical bytes and caps

```go
func Canonical(raw []byte) ([]byte, error)   // json.Valid; rejects duplicate keys at every depth; json.Compact; encoder with SetEscapeHTML(false)
func OutputHash(raw []byte) string           // hex sha256 of Canonical(raw)
func LineHash(line []byte) string            // hex sha256 of a stored line (no '\n')
```
Every `Event` and every payload is serialized with an encoder that has `SetEscapeHTML(false)`; stored `Data` is
`Canonical(Data)`. Caps are measured on canonical bytes and enforced in `Apply` (reason `oversize`, naming the
field), hence both at `Append` (validate-before-write) and at `Fold`:

| field | cap | field | cap |
|---|---|---|---|
| `MaxShort` (every `Name`, `Code`, `Gate`, `Atom`, `Reason`, `Error`, `Type`, `Node`, `State`, `Kind`, `ID`, `Commit`, `RepoRoot`, `WorkDir`, `Workflow`, hashes) | 1 KB | `Finding.IssueText`, `Golden.Comment`, `Vars` value, `Argv` element, `WarnData.Detail` | 4 KB |
| `Bug.Desc` | 2 KB | `Vars` ≤ 64 entries; `Goldens` ≤ 128; `AllowedCmds` ≤ 32; `Argv` ≤ 64; `FileHashes` ≤ 256; `Findings`/`Confirmed`/`Status` per delta ≤ 512 | counts |
| `GateError.Detail`, `cmd_call.stdout`, `tree.status` | 64 KB (`MaxDetail`; writer truncates via `CapDetail`/`CapText`, flag set) | `cmd_call.stderr` | 8 KB |
| `node_output.output`, `llm_call.verdict`, `record.data` | 256 KB canonical | `Event` line | `MaxLine` = 1 MB (always satisfiable given the above) |
| `Warnings` | ≤ 1024 entries | events per run | `MaxEvents` = 100 000 (`ERR_AUDIT_FULL` at append) |

### 2.4 Errors

```go
type FoldError  struct { Code, Reason string; Seq int64; Type string }   // Code ∈ {ERR_AUDIT_EMPTY, ERR_AUDIT_VERSION, ERR_AUDIT_INVALID}
type StoreError struct { Code string; Seq int64; Detail string; Cause error }
// StoreError codes: ERR_STORE_PATH, ERR_RUN_EXISTS, ERR_RUN_NOT_FOUND, ERR_RUN_LOCKED, ERR_AUDIT_CHAIN, ERR_AUDIT_CAS,
//                   ERR_AUDIT_TORN, ERR_AUDIT_TAIL_CHANGED, ERR_EVENT_TOO_LARGE, ERR_AUDIT_FULL, ERR_APPEND_REJECTED (wraps a *FoldError)
// FoldError reasons (constants): empty, version, first_not_init, second_init, seq_gap, unknown_type, bad_payload, oversize,
//   output_after_delta, delta_without_output, second_delta, output_hash, status_not_subset, status_incomplete, status_duplicate,
//   post_terminal, provenance, stamp, mock_stamp, fix_baseline_head, fix_baseline_kind, init_stamp
```

---

## 3. Events

```go
type Event struct {
    SchemaVersion int `json:"schemaVersion"`; Seq int64 `json:"seq"`; Prev string `json:"prev"`; At Time `json:"at"`; Type string `json:"type"`
    State State `json:"state,omitempty"`; Iter int `json:"iter"`; Node string `json:"node,omitempty"`; Mock bool `json:"mock,omitempty"`
    Origin *Origin `json:"origin,omitempty"`; Data json.RawMessage `json:"data"`
}
type Origin struct { RunID string `json:"run_id"`; Seq int64 `json:"seq"`; Hash string `json:"hash"` }   // Hash = LineHash of the parent's stored line
```
Ownership: the **machine** sets `SchemaVersion`, `At`, `Type`, `State`, `Iter`, `Node`, `Mock`, `Origin`, `Data`; the
**store** sets `Seq` and `Prev`. Copies retain every machine-set field (incl. `At`, `SchemaVersion`, `Mock`).

### 3.1 Event types and payloads (all payload structs carry snake_case tags)

| `Type` | payload | node-scoped | notes |
|---|---|---|---|
| `init` | `InitData{RunID, CreatedAt Time, Workflow, WorkflowHash, Vars, Calibration, Mock, RepoMode, AllowedCmds, CmdsSHA256, RepoRoot, WorkDir, BaseSHA, Head, InitialState State, InitialKind Kind, Goldens, ParentRunID, ForkedAtSeq}` | no | exactly one, seq 1 |
| `tree` | `TreeData{Head, TreeHash, Status string; StatusTruncated bool}` | no | working-tree snapshot carrier (cadence: spec 2) |
| `needs_input` | `{}` | yes | |
| `node_output` | `NodeOutputData{Output json.RawMessage}` | yes | |
| `delta_applied` | `DeltaAppliedData{Delta; OutputHash string}` | yes | `OutputHash = OutputHash(NodeOutputs[k])` |
| `llm_call` | `LLMCallData{Kind, Model, Effort string; Index int; InputHash string; Verdict json.RawMessage; Confidence float64; Tokens TokenTotals; DurationMS int64; Error string}` | yes | `Index` contiguous from 0 per `node@iter` |
| `cmd_call` | `CmdCallData{Name string; Argv []string; InputHash, Stdout, Stderr string; StdoutTruncated, StderrTruncated bool; ExitCode int; DurationMS int64; Error string}` | optional | |
| `gate` | `GateData{Name string; Passed bool; Error *GateError}` | no | |
| `converge` | `ConvergeData{Atom string; Class Outcome; Stop bool; Reason string}` | no | |
| `transition` | `TransitionData{From, To State; Gate string; Outcome Outcome; Loop bool; ToKind Kind; Head string}` | no | checkpoint |
| `fix_baseline` | `FixBaselineData{Head string}` | no | §4.6 |
| `tokens` | `TokenTotals` | no | additive |
| `record` | `RecordData{Name string; Data json.RawMessage}` | no | `Type` always `record` |
| `warn` | `WarnData{Code, Detail string}` | no | |
| `overflow_handler` | `OverflowHandlerData{Name string; Argv []string; InputHash string; ExitCode int; DurationMS int64; Error string}` | no | |
| `fork` | `ForkData{ChildRunID string; AtSeq int64}` | no | parent only |

---

## 4. `Apply` and `Fold`

```go
type FoldState struct {
    Snapshot
    indexes   map[string]int   // node@iter → next llm_call index
    originOpen bool            // still inside the copied block
    lastOrigin int64
}
func Apply(st FoldState, ev Event) (FoldState, error)   // pure; error is *FoldError; st is not mutated
func Fold(events []Event) (Snapshot, error)             // reduce(Apply) from the zero state; returns Snapshot only
```
Deterministic: same events → byte-identical `Canonical(json.Marshal(snapshot))`. Sequence errors: `empty`;
`version` (any `SchemaVersion != run.SchemaVersion`); `first_not_init`; `second_init`; `seq_gap` (must be `st.Seq+1`);
`unknown_type`; `bad_payload`; `oversize`. After every accepted event: `Snapshot.Seq ← ev.Seq`.

### 4.1 `init` — copies every `InitData` field; `SchemaVersion ← run.SchemaVersion`; `State ← InitialState`;
`StateKind ← InitialKind`; `Iteration ← 0`; `PrevUnfixed ← nil`; if `InitialKind == KindAgentEdit && ParentRunID == ""`:
`FixEntryHead ← Head`. Maps/slices initialized empty.

### 4.2 `node_output` — `k = Key(Node, Iter)`; `!Applied[k]` else `output_after_delta`. `NodeOutputs[k] ← Canonical(Output)`.
Append `Node` to `NodesRun` if absent.

### 4.3 `delta_applied` — `NodeOutputs[k]` exists (`delta_without_output`); `!Applied[k]` (`second_delta`);
`OutputHash == OutputHash(NodeOutputs[k])` (`output_hash`). Then `Findings ← Delta.Findings` if non-nil;
`Confirmed ← Delta.Confirmed` if non-nil, and each `Bug` with an `ID` absent from `AllFound` is appended (first-seen
order). If `Delta.Status != nil`: the multiset of `Status[i].ID` must equal the set of `AllFound[i].ID` **exactly**
(`status_not_subset` for an unknown id, `status_duplicate`, `status_incomplete` for a missing one); then `Status ←
Delta.Status`. Always recompute `Unfixed ← |{b ∈ AllFound : no s ∈ Status with s.ID == b.ID && !s.StillPresent}|`
(an unstatused bug counts as unfixed — fail closed until the next full `Status`). `Applied[k] ← true`.

### 4.4 `llm_call` — `Index` must equal `indexes[k]` (`stamp`); `indexes[k]++`; `Tokens ← Tokens.Add(payload.Tokens)`.
`tokens` — `Tokens ← Tokens.Add(payload)`. These are the only two token sources.

### 4.5 `tree` — `Head`, `TreeHash`, `TreeStatus` ← payload.

### 4.6 `transition` — `State ← To`; `StateKind ← ToKind`; `Head ← Head`; `LastError ← nil`. If `Loop`: `Iteration++`;
`v := Unfixed; PrevUnfixed ← &v`; `Findings`, `Confirmed` ← empty. If `ToKind == KindAgentEdit && ev.Origin == nil`:
`FixEntryHead ← Head` (a copied transition never sets it — a fork into an agent-edit state must append `fix_baseline`).
If `Outcome != ""`: `Outcome ← Outcome` (must be one of the constants; run derives no outcome from state names).
`fix_baseline` — requires `StateKind == KindAgentEdit` (`fix_baseline_kind`) and `payload.Head == Snapshot.Head`
(`fix_baseline_head`); `FixEntryHead ← Head`. It does not change `Snapshot.Head`.

### 4.7 Terminal rule — `Outcome != ""` is terminal. After it only `overflow_handler`, `cmd_call`, `warn`, `record`,
`tokens`, `tree`, `fork` are legal (`post_terminal`). Other rules: `gate{Passed:false}` → `LastError ← Error`;
`converge{Stop}` → `StopReason ← Atom`; `warn` → append `Code`; `overflow_handler` → `OverflowHandled ← true`;
`record`, `needs_input`, `cmd_call`, `fork` → no change except `Seq`.

### 4.8 Provenance — if `ParentRunID != ""` the events at child seqs `2..ForkedAtSeq` must all carry `Origin` with
`Origin.RunID == ParentRunID`, `Origin.Seq == ev.Seq`, non-empty `Hash`, and `Type != init`; no event outside that range
carries `Origin`; `ParentRunID == ""` ⇒ no `Origin` anywhere (`provenance`). (`ForkedAtSeq` is thus both the parent seq
of the last copied event and the child seq of the last copied event, because copies start at seq 2 in both logs.)
`MockTainted ← true` on any `Mock` event when `Snapshot.Mock == ""`. **Producer invariant:** when `Snapshot.Mock != ""`,
every non-`init` event must carry `Mock: true` (`mock_stamp`).

### 4.9 Stamps — `init`: `State == ""`, `Iter == 0`, `At` non-zero (`init_stamp`). Every later event: `Iter == Iteration`
(post-increment for a `Loop` transition), `State == State` (for a `transition`, `From`), `At` non-zero (`stamp`).

### 4.10 Terminal note — `Seq` and `SchemaVersion` are the only fields set outside §4.1–4.9 (see §4 preamble and §4.1).

---

## 5. Store

```go
type TornTail struct { Offset int64; Bytes []byte }
type Log struct { Events []Event; Head string; Torn *TornTail }          // Head = LineHash of the last complete line
type RunSummary struct { RunID, Workflow string; CreatedAt Time; State State; Outcome Outcome; ParentRunID string; Mock bool; MockTainted bool; Torn bool; Sidecars int; Error string }

type RunStore interface {
    Create(runID string, first Event) error                                      // ERR_RUN_EXISTS; first must be init with InitData.RunID == runID (ERR_APPEND_REJECTED otherwise)
    Append(runID string, st FoldState, ev Event) (seq int64, head string, err error)   // §5.2
    Events(runID string) (Log, error)                                            // ERR_RUN_NOT_FOUND; chain verified
    EventsWithLines(runID string) (Log, [][]byte, error)                         // raw lines for oracles / Origin.Hash verification
    RepairTail(runID string, torn TornTail) error                                // §5.3
    List() ([]RunSummary, error)                                                 // §5.5
    Lock(runID string) (unlock func(), err error)                                // §5.6
    Root() string
}
func NewJSONLStore(root string) RunStore; func NewMemStore() RunStore; func ValidateRunID(id string) error   // ^mrv-[A-Za-z0-9-]{8,200}$
```

### 5.1 Layout, paths, modes
`<root>/.metareview/runs/` is created `0700`; `Create` **always** ensures `runs/.gitignore` contains `*` (the directory
is transient regardless of the consumer's ignore policy; durable copies are spec 3's `export`). `<root>/.metareview/runs/<id>/`
is `0700`; `audit.jsonl`, `lock`, `audit.torn-*.bin` are `0600`. Every store method validates `runID` before any path
join. Every path component below `<root>` is `Lstat`-checked to be a real directory and files are opened with
`O_NOFOLLOW`; a symlink → `ERR_STORE_PATH`. This check is best-effort against symlinks present on disk (e.g. committed
to the repo), not against a concurrent local attacker (out of the threat model, §1.5). After `Create`, `runs/` and the
run directory are fsync'd. `RepoRoot` is fixed at init; a fork's `--work-dir` never moves the store.

### 5.2 Write path and chain
`Append(runID, st, ev)`: (1) `ValidateRunID`; (2) lock held by this process else `ERR_RUN_LOCKED`; (3) read the tail:
torn → `ERR_AUDIT_TORN`; `LineHash(lastLine) != st.Head` or `st.Seq != lastSeq` → `ERR_AUDIT_CAS`; (4) `ev.Data ←
Canonical(ev.Data)`; `ev.Seq ← st.Seq+1`; `ev.Prev ← st.Head` (or `""` at seq 1); (5) `Apply(st, ev)` → on error
`ERR_APPEND_REJECTED{Cause}`; (6) `line ← Canonical(marshal(ev))`; `len(line) > MaxLine` → `ERR_EVENT_TOO_LARGE`;
`st.Seq+1 > MaxEvents` → `ERR_AUDIT_FULL`; (7) write `line+"\n"` with `O_APPEND`, `fsync`; return `(ev.Seq, LineHash(line))`.
`FoldState` carries `Head` (`FoldState.Head string`, set by `Events()` consumers via `Log.Head`; `Apply` updates it to
`LineHash` only inside `Append`). `Events()`: read all lines; for each complete line verify `Seq == prev+1` and
`Prev == LineHash(previous line)` (`ERR_AUDIT_CHAIN` at that seq); decode each line into `Event` (a complete line that is
not valid JSON or not an `Event` envelope is `ERR_AUDIT_CHAIN` with `Detail: "undecodable"`; payload validity is `Fold`'s
job). The chain preimage is the raw stored line bytes; readers never re-marshal.

### 5.3 Torn tail
Torn ≡ non-empty bytes after the last `\n` (a complete JSON line missing only `\n` is torn). `Events()` returns them as
`Log.Torn`, never modifies the file, and `Append` refuses until repaired. `RepairTail(runID, torn)` (lock required):
verify the file still ends with exactly `torn.Bytes` at `torn.Offset` (`ERR_AUDIT_TAIL_CHANGED`); create
`audit.torn-<nextSeq>-<unixnano>.bin` with `O_CREAT|O_EXCL`, write, fsync file, fsync dir; truncate through the open
audit descriptor to `torn.Offset`; fsync. If `torn.Offset == 0` (no durable `init`), the run directory is removed
instead — the run never existed. The **machine** then appends `warn{AUDIT_TORN_LINE_DROPPED, "<n> bytes in <file>"}`
with correct stamps (it holds the fold). `RunSummary.Sidecars` counts `audit.torn-*.bin` files; spec 3's `export`
includes them.

### 5.4 Versioning
0.9.0 reads and writes `SchemaVersion == 1` only; any other value is `version`. The contract for a future bump: the
new binary ships `Migrate(v int, in Log) (Log, error)` and writes `audit.v<N>.jsonl` **beside** `audit.jsonl` (original
retained, never rewritten, so children's `Origin.Hash` stay valid against the original); readers prefer the highest
`audit.v<N>.jsonl` they understand; an older binary that finds only newer files reports `version`. No code for this
ships at v1 (no unreachable branch).

### 5.5 `List()`
Folds every conforming run directory (non-conforming names skipped; missing `runs/` → empty list, no error). A run
whose `Events()`/`Fold` fails yields a summary with `Error` set (listing never aborts); a torn run's summary is
prefix-derived with `Torn: true`. Order: `CreatedAt` descending, then `RunID` ascending. Cost O(total bytes); accepted
at expected scale (tens of runs). Retention: until the user deletes the directory.

### 5.6 Lock
`flock(LOCK_EX|LOCK_NB)` on `<run>/lock` opened `O_CREAT|O_RDWR|O_NOFOLLOW` `0600`; failure → `ERR_RUN_LOCKED`. Released
by `unlock` or process exit; no pid file. A second `Lock` in the same process fails. `Append` and `RepairTail` require it.

---

## 6. Run IDs — `RunID(workflow, at) = state.RunID("fsm-"+workflow, workflow, at)`; carried in `InitData.RunID`.

## 7. Writer contract (spec 2 = `Advance`/`Record`, spec 3 = `Fork`)
Produce logs that `Apply` accepts: `init` first with all fields (`State == ""`, `Iter == 0`); stamp every event with the
current `Iter`/`State`/non-zero `At`; `Mock: true` on every event of a mock run; per `node@iter` outputs then one delta
with `OutputHash(NodeOutputs[k])`; supplied `Status` covers `AllFound` exactly; `Outcome` only from the payload (incl.
`failed`); post-terminal allow-list; `ToKind` on every transition (`""` when the target has no node); fork copies are
exactly parent seqs `2..ForkedAtSeq` with `Origin{parent, seq, LineHash(parent line)}` and a fork into an agent-edit
state is followed by `fix_baseline{Head: current HEAD}`; the machine appends `tree` events (spec 2 says when) and
truncates `Detail`/stdout/stderr/status via `CapDetail`/`CapText`; hash outputs with `OutputHash` (never `sha256` of
un-canonical bytes).

---

## 8. Tests (`internal/fsm/run`, 100% statements, no other `fsm` import; `Builder` is in-package)

`Builder` constructs events with **literal** `Seq`/`Prev`/`Iter`/`State`/`At` supplied by the test (defaults derive
them, and every field has an override so invalid logs can be built); it never calls `Apply`. Golden fixtures live in
`internal/fsm/run/testdata/`.

| # | test | discriminates |
|---|---|---|
| R1 | fold table: one row per rule in §4.1–4.9 with negative/second-instance cases; explicit rows: `transition{To:"failed", Outcome:""}` → `Outcome == ""`, non-terminal; `Unfixed` with one unstatused, one `StillPresent:true`, one `false` bug → 2; `Status` supplied after `AllFound` grew → `status_incomplete`; copied agent-edit transition leaves `FixEntryHead` empty; `fix_baseline` head mismatch / wrong kind; `cmd_call` after terminal accepted, `transition` after terminal rejected (one row per allow-listed type) | every derivation and its negation |
| R1b | no-op rows (`record`, `needs_input`, `cmd_call`, `fork`): `SnapshotEqualIgnoringSeq` before/after; `NodesRun` unchanged for `needs_input` | §4.7 no-ops |
| R2 | one row per `FoldError.Reason` constant asserting `{Code, Reason, Seq, Type}` | fail-closed, typed |
| R2b | external oracle: `testdata/oracle.jsonl` with `prev` produced by `sha256sum` (regeneration script `testdata/oracle.sh` committed; goldens never auto-regenerated); `Events()` accepts it, rejects a one-byte edit with `ERR_AUDIT_CHAIN` at the right seq; `LineHash`/`Canonical` match the oracle; a `<>&` payload round-trips unescaped | chain preimage, no HTML escaping |
| R3 | order: `AllFound`, `NodesRun`, `Warnings`, `Status` with ≥ 3 elements in non-sorted order; repeated folds byte-equal; JSONL vs mem byte-equal | ordering bugs |
| R4 | per-prefix goldens: `testdata/golden-log.jsonl` + `golden-snapshots.jsonl` (one expected snapshot per prefix, hand-reviewed once, never regenerated); mutation table: for each class {delete/duplicate/swap} × {each event type} the expected outcome is enumerated (`seq_gap` before re-chain; after re-chain: the specific reason, or "valid, snapshot differs in fields F", or "no-op") — asserted per class | compositional + each invariant catches its mutation |
| R5 | `Clone`: reflect walk recursing into slice elements, map values, pointer targets, RawMessage bytes; mutate in place; both directions | shallow/partial copy |
| R6 | `PrevUnfixed`: nil before first loop; value copy; second loop; JSONL round trip `null` vs number | aliasing, nil-vs-0 |
| R7 | store conformance over both stores: create/append/events; `Create` refusals (non-init, id mismatch, `ERR_RUN_EXISTS`); `ERR_RUN_NOT_FOUND`; CAS (`Head`, `Seq`); `Append` without lock; second `Lock`; `ERR_APPEND_REJECTED` leaves bytes/seq unchanged; `ERR_EVENT_TOO_LARGE`; `ERR_AUDIT_FULL`; duplicate keys at depth 3 rejected; `Data` stored canonical; `Root()`. JSONL-only: modes, `.gitignore` content `*` re-ensured on second `Create`, symlink → `ERR_STORE_PATH`, fsync path, `flock` released on unlock/exit | store contract |
| R8 | torn: (a) mid-line, (b) complete line missing `\n`, (c) valid prefix of a longer line → `Torn`, file unchanged, `Append` → `ERR_AUDIT_TORN`; `RepairTail` writes sidecar with the exact name pattern + `O_EXCL`, truncates, refuses `ERR_AUDIT_TAIL_CHANGED`, requires lock; offset 0 → directory removed; complete-but-undecodable last line → `ERR_AUDIT_CHAIN` | repair rule |
| R9 | `List()`: order (`CreatedAt` desc, id asc), `Error` rows, missing `runs/` → empty, `Torn`/`Sidecars`, id shape table | listing |
| R10 | fork: `Builder.Copy(parent, 2..k)` produces the child; fold equals parent prefix fold except id/parent/forked/created; violations (init copied, gap, wrong parent, `Origin.Seq != Seq`, Origin outside range, Origin with no parent) → `provenance`; `MockTainted` true when a mock event lands in a non-mock child and **false** for a mock run; `mock_stamp` for a mock run with an unmarked event | provenance both ways |
| R11 | version: any `SchemaVersion != 1` → `version` at the right seq | exact match |
| R12 | caps: for every cap in §2.3 one at-cap accepted row and one over rejected (`oversize` naming the field); `CapDetail`/`CapText` UTF-8 boundary + flag | off-by-one, coverage |
| R13 | value pins: `BugID("x") == "11f6ad8ec52a"`, `Key("n",2) == "n@2"`, `Add` with five distinct values, `Total`, every `Reason`/`Code` constant string | constants the gate cannot see |
| R14 | `Time`: marshals `Z` nanos from a non-UTC input; rejects offsets; zero `At` → `stamp`/`init_stamp` | time contract |

---

## 9. Exports — `Fold`, `Apply`, `FoldState`, `Builder`, `Canonical`, `OutputHash`, `LineHash`, `CapDetail`, `CapText`, `BugID`,
`Key`, `ValidateRunID`, `RunID`, `SnapshotEqualIgnoringSeq`, `Time`, all wire/payload types, `FoldError`, `StoreError`, `RunStore` + both stores. Nothing else.

## 10. Reserved (numbering kept stable for reviewers).

## 11. Amendment ledger (parent plan r3 contracts superseded here; accepted with the split decision)

| parent contract | change |
|---|---|
| C21 `state.json` cache | removed. |
| §1.2 `replay` wrapper | verbatim copies with `Origin{run_id, seq, hash}`. |
| §1.7.4 `llm_call.vars_used` | removed; spec 3 defines the freeze rule over `NodesRun` (node names). |
| §1.3 `Delta.Tokens` | removed. |
| C15 record types | `record tokens`/`record node-output` map to types `tokens`/`node_output` (spec 5 documents). |
| design §10 diff snapshots | `tree` event with porcelain status (≤ 64 KB), not a full diff. |
| §1.5 step 8 `FixEntryHead ← head` | derived from `ToKind`/`InitialKind` (non-copied only) and `fix_baseline`. |
| §1.5/§1.6 outcome from `→failed` | payload-supplied. |
| §1.2 `Fold(events, kinds Decoder)` + re-Decode invariant | `Fold` is type-blind; **spec 3** re-validates every copied `node_output` against the (new) workflow's kinds before copying (`ERR_AUDIT_INVALID` if it no longer fits). |
| §1.4 exact `Status` coverage | kept (restored in r3 after r2 relaxed it). |
| §1.2 `run.ExecMode` | moved to `kind`. |
| §1.2 `ValidateRunID ^mrv-[A-Za-z0-9-]+$` | narrowed to `{8,200}` (no existing FSM ids to break). |
| §1.2 `Detail` "full in audit" | ≤ 64 KB with `detail_truncated`. |
| §1.2 `transition.tree_hash` | moved to the `tree` event. |
| §1.2 `List()` order | `CreatedAt` desc, id asc (restated). |
| four-spec split | refined to five (workflow+gate+converge separated from fork/resume) — recorded here for Dave's acceptance. |

## 12. Ownership ledger (partition; each attempt-3 finding and parent section appears exactly once)

| # | spec | owns | attempt-3 findings |
|---|---|---|---|
| 1 | **run** (this) | plan §1.2, §1.4, persistence half of §1.7, R-tests | ARC3-1/2/3/4/6/10, DM3-2/4, CMP3-2/4/6, SEC-21, FIN-1/2 (persistence: fields exist), FIN-6/7, TQ3-1/9 |
| 2 | **workflow + gate + converge + machine core** | plan §1.1 layout/dependency direction, §1.3 gate/converge/workflow types, §1.5 `Advance`, `Record` (incl. `ERR_NODE_OUTPUT_APPLIED`), `Fork` prerequisites it must emit (`tree` cadence, `TreeHash` preimage, `UNSANCTIONED_EDIT`), §1.6, §6 shipped workflows + `workflows` package, canonical `cmds_sha256` preimage, `PrevUnfixed == nil` rule | CMP3-1/5/7 (machine half), ARC3-5/7/8/9, INT-16 (uses `fix_baseline`), TQ3-10/11, SCP3-4/5, SEC-23, RI2-3 (uses `overflow_handler` fields) |
| 3 | **fork / resume / diff / export / runs.jsonl** | plan §1.7 fork half, `Origin.Hash` verification, freeze rule, git preconditions, `--work-dir` validation, re-validation of copied outputs, `diff`, `export` (redaction incl. `tree.status`, sidecars, manifest), `fsmRunRecord` + verdict mapping + parent row at fork time, `ESCALATED` mapping | INT-9/10/13/14/18/19, FEA-N3/N4, DM3-1/3/5, DMN-1, SEC-13/22/27, TQ3-2/3/7, SCP3-1/2/3/8, FIN-2 (freeze half) |
| 4 | **guardrails + judge + kinds + mockai** | plan §1.8, §2, kinds/`Executor`/`Delta` producers, match-then-adjudicate composition + `Bug.Verdict` vocabulary, `index` assignment, scenarios, `JUDGE_EFFORT` requiredness, pinned harnesseval sha | INT-11/12/17/20/21/22, SEC-11/12/14/24/25/26/28/29, FIN-3/4/5, CMP3-3, TQ3-8 |
| 5 | **CLI + tests + milestones + docs** | plan §3, §4, §5, M8 docs (exit 3, path-less exit 1, `.metareview/runs/` transient + `gitpolicy.go` reconcile, trust boundary, enforcing caveat, judge-swap limitation, spec amendments §10.1/§14.3/§17, `ERR_AUDIT_TORN` / repair UX), `package.json` files incl. `go.sum`, forbidden-phrase grep, CI | CMP3-7 (CLI half), TQ3-4/5/6, SCP3-6/7, INT-15/23, DM3-6/7/8, SEC-30, FIN-8, RDM-5 |
