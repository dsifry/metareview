# metareview 0.9.0 — `internal/fsm/run`: event-sourced run persistence

> **Status:** REVISION 4 (2026-08-26) — **build baseline for M0.** First of the five split 0.9.0 build artifacts
> (parent plan: [`2026-08-26-metareview-0.9.0-build-plan.md`](2026-08-26-metareview-0.9.0-build-plan.md), r3,
> escalated; Dave chose "split the target"). Three artifact-review attempts (`mrv-20260827-05{0354,1457,2714}…`)
> converged to sentence-level residue; attempt 3 recorded ESCALATE per the chain contract and Dave chose
> "proceed": r4 applies the remaining edits (§0) and M0 implements this spec test-first, with the code gate
> (`review task-done`) as the next review.
>
> **Scope rule:** this document says what `run` stores and derives, and what a writer must produce for `run` to
> accept it (widened from r2, recorded in §11). It does not say when the machine appends an event, how a gate is
> evaluated, how a fork chooses a checkpoint, or what the CLI prints.

---

## 0. Attempt-3 resolution map (all mechanical)

| findings | change |
|---|---|
| RA3-1, FIN3-2, FIN3-3, RC3-1 | `FoldState.ChainHead` is a distinct field (the audit chain head); `Snapshot.Head` stays the git HEAD; `Append` sets `ChainHead` itself after writing (§4, §5.2). |
| RA3-2, RA3-3, FIN3-4, FIN3-6 | `FoldFull(events) (FoldState, error)` exported; `Apply` is copy-on-write over the maps it touches and never deep-copies `NodeOutputs` values; `Append` returns the new `FoldState`; on any `Append` error the caller's state is unchanged (§4, §5.2). |
| RA3-4, RC3-5 | `Create(runID, st0, first)` has parity with `Append` (canonicalize, `Apply` from the zero state, `MaxLine`, fsync line + dirs, returns `(FoldState, error)`); `ERR_RUN_EXISTS` is race-free via `O_EXCL` on the run directory's `audit.jsonl` (§5.2). |
| RA3-5, RI3-1, RT3-1 | A fork into an agent-edit state appends `tree{Head: current HEAD}` **immediately before** `fix_baseline{Head: same}`; `fix_baseline` is legal only when the previous event is a `tree` with the same `Head` (`fix_baseline_order`); R10's oracle excludes `FixEntryHead` and adds the fork→`tree`→`fix_baseline` row (§4.6, §7, R10). |
| FIN3-1, RC3-8, RT3-9 | `Canonical(raw)` = encode `json.RawMessage(raw)` with an `Encoder` (`SetEscapeHTML(false)`), strip the trailing `\n`, after `json.Valid` + duplicate-key rejection at every depth; key order preserved, numbers re-emitted as Go does (verified idempotent); R2b pins a non-canonical input and idempotence (§2.3). |
| RA3-6, FIN3-5, RDM3-1, RSEC3-2, RC3-2, RT3-3, RT3-4 | Aggregate caps: `MaxPayload` = 256 KB canonical for **every** payload (`init` included); per-field caps reduced to fit (`Goldens` ≤ 64, `AllowedCmds` ≤ 16, `Argv` ≤ 32, `FileHashes` ≤ 64, per-delta lists ≤ 256); every string field capped by name or by `MaxShort`; all caps measured on canonical bytes of the field — `CapDetail`/`CapText` cap the **canonical** encoding (§2.3). |
| RSEC3-1, RDM3-5, RA3-8 | `RepairTail(runID)` re-derives the tail from the file under the lock (no caller-supplied bytes); offset 0 writes the sidecar **before** removing the directory; the lock file is unlinked last (§5.3). |
| RDM3-4, RI3-4, RT3-10 | `MaxEvents` applies to non-terminal node events only; `transition`, `warn`, `fork`, `overflow_handler`, `cmd_call` are exempt; both stores take `MaxEvents` as a constructor option (§2.3, §5). |
| RDM3-2, RDM3-3, RDM3-6, RI3-5 | Migration: `Migrate` writes `audit.v<N>.jsonl` with a fresh chain; `Append`/`RepairTail` target the highest version file the binary understands; `Origin` gains `Version int`; verification reads the parent's file of that version; fork-of-a-fork: `Origin` always names the **immediate** parent (copies overwrite `Origin`), and the child's `init` carries `Lineage []string` (ancestor ids); deleting an ancestor leaves `Origin.Hash` unverifiable — reported, not fatal (§3, §4.8, §5.4). |
| RC3-4, RT3-5 | Reason→Code table (§2.4). |
| RC3-3, RC3-10 | §9 export list complete; `Builder` API declared (§8). |
| RC3-6 | Node-scoped events require non-empty `Node` (`node_scope`); `gate{Passed:true}` is a no-op; `gate{Passed:false, Error:nil}` is `bad_payload` (§4.7). |
| RC3-7, FIN3-8, RA3-7 | Loop transition: the event's `Iter` is the **new** iteration (N+1); every other event carries the current iteration; `mock_stamp` exempts `Origin`-bearing events (§4.8, §4.9, §7). |
| RT3-2, RT3-6, RT3-8 | R1 rows: a node re-run in iteration 2 with interleaved keys; `SnapshotEqualIgnoringSeq` mirror rows; per-type post-terminal negatives (§8). |
| RSEC3-4, RSEC3-5, RSEC3-6, RSEC3-7 | `cmd_call.Name`/`overflow_handler.Name` must be in `AllowedCmds` (`unsanctioned_cmd`); `OverflowHandlerData` gains `Stdout`/`Stderr` + flags; §12 row 3 lists every secret carrier for export redaction; `ForkedAtSeq` > copied events is a spec-3 precondition + R10 row (§3.1, §4.7, §12). |
| RS3-1..8, RC3-9 | §11 rows for lock/torn reversal/`Unfixed` redefinition/scope widening; §12 rebuilt as a partition incl. carried partials; `MaxEvents`, local-FS-only, retention recorded as product constraints for spec 5's docs (§11, §12). |
| RDM3-7, RDM3-8 | Unlocked reads are advisory (§5.5); `.gitignore` written atomically with content exactly `*\n` (§5.1); sidecar `EEXIST` → `ERR_STORE_PATH` (§2.4). |

---

## 1. Principles

1. **`audit.jsonl` is the only authority.** No cache. Every command folds the log.
2. **`Fold` is pure and total over `run` types**; `Fold = reduce(Apply)`; `run` imports no other `fsm` package.
3. **Append-only, hash-chained, versioned, validated before write.** A line is written only after `Apply` accepts
   it; a log produced through `Create`/`Append` is always foldable. The single byte-removing operation is §5.3's
   repair of an unterminated tail, which preserves the removed bytes in a sidecar.
4. **Fail closed.** Every invariant violation is a typed error.
5. **Trust boundary.** Directory `0700`, files `0600`. The hash chain gives integrity against accidental corruption
   only — unanchored, so no tamper evidence against any deliberate write by the same user, including the host
   agent. The FSM's guarantees are process guarantees for a cooperating agent driving it through the CLI. Local
   filesystems only. (Docs: spec 5, with the prominence of the advisory/enforcing caveat.)

---

## 2. Wire types

Snake_case JSON tags (`schemaVersion` kept). Payloads decode with `DisallowUnknownFields`. Every serialization goes
through `Canonical` (§2.3).

```go
package run

const SchemaVersion = 1

type State string; type Outcome string; type Kind string
const ( OutcomeFixed Outcome = "fixed"; OutcomeClean = "clean"; OutcomeReviewed = "reviewed"; OutcomeStalled = "stalled"; OutcomeOverflow = "overflow"; OutcomeCustom = "custom"; OutcomeFailed = "failed" )
var Outcomes = []Outcome{…}                 // membership check
const KindAgentEdit Kind = "agent-edit"

type Finding   struct { IssueText string `json:"issue_text"`; File string `json:"file,omitempty"`; Line int `json:"line,omitempty"`; Severity string `json:"severity,omitempty"`; Category string `json:"category,omitempty"`; Source string `json:"source,omitempty"` }
type Golden    struct { Comment string `json:"comment"`; Severity string `json:"severity,omitempty"`; Category string `json:"category,omitempty"` }
type Bug       struct { ID string `json:"id"`; Desc string `json:"desc"`; File string `json:"file,omitempty"`; Line int `json:"line,omitempty"`; Verdict string `json:"verdict"`; Confidence float64 `json:"confidence"`; GoldenIdx *int `json:"golden_idx,omitempty"` }
type BugStatus struct { ID string `json:"id"`; StillPresent bool `json:"still_present"`; Confidence float64 `json:"confidence"` }
type TokenTotals struct { Input int64 `json:"input"`; CacheRead int64 `json:"cache_read"`; CacheCreate int64 `json:"cache_create"`; Output int64 `json:"output"`; Reasoning int64 `json:"reasoning"` }
func (t TokenTotals) Add(u TokenTotals) TokenTotals; func (t TokenTotals) Total() int64
func BugID(issueText string) string        // hex(sha1)[:12]; BugID("x") == "11f6ad8ec52a"
func Key(node string, iter int) string     // node + "@" + itoa(iter)

type GateError struct { Code string `json:"code"`; Gate string `json:"gate"`; Detail string `json:"detail,omitempty"`; DetailTruncated bool `json:"detail_truncated,omitempty"` }
func (e *GateError) Error() string
func CapDetail(s string) (string, bool)               // = CapText(s, MaxDetail)
func CapText(s string, max int) (string, bool)        // truncates so that len(Canonical(json string of s)) ≤ max, at a UTF-8 boundary; flag = truncated

type AllowedCmd struct { Name string `json:"name"`; Argv []string `json:"argv"`; FileHashes map[string]string `json:"file_hashes"` }
type Delta struct { Findings []Finding `json:"findings,omitempty"`; Confirmed []Bug `json:"confirmed,omitempty"`; Status []BugStatus `json:"status,omitempty"`; Commit string `json:"commit,omitempty"` }
```

### 2.1 Snapshot (derived)

```go
type Snapshot struct {
    SchemaVersion int `json:"schemaVersion"`; RunID string `json:"run_id"`; ParentRunID string `json:"parent_run_id,omitempty"`; Lineage []string `json:"lineage"`; ForkedAtSeq int64 `json:"forked_at_seq,omitempty"`
    CreatedAt Time `json:"created_at"`; Seq int64 `json:"seq"`
    Workflow string `json:"workflow"`; WorkflowHash string `json:"workflow_hash"`; Vars map[string]string `json:"vars"`; Calibration bool `json:"calibration"`
    Mock string `json:"mock,omitempty"`; MockTainted bool `json:"mock_tainted"`
    RepoMode string `json:"repo_mode"`; AllowedCmds []AllowedCmd `json:"allowed_cmds"`; CmdsSHA256 string `json:"cmds_sha256,omitempty"`
    RepoRoot string `json:"repo_root"`; WorkDir string `json:"work_dir"`
    State State `json:"state"`; StateKind Kind `json:"state_kind,omitempty"`; Outcome Outcome `json:"outcome,omitempty"`; Iteration int `json:"iteration"`
    BaseSHA string `json:"base_sha"`; Head string `json:"head"`; FixEntryHead string `json:"fix_entry_head,omitempty"`
    TreeHash string `json:"tree_hash,omitempty"`; TreeStatus string `json:"tree_status,omitempty"`
    Goldens []Golden `json:"goldens"`; Findings []Finding `json:"findings"`; Confirmed []Bug `json:"confirmed"`; AllFound []Bug `json:"all_found"`; Status []BugStatus `json:"status"`
    Unfixed int `json:"unfixed"`; PrevUnfixed *int `json:"prev_unfixed"`; Tokens TokenTotals `json:"tokens"`
    NodeOutputs map[string]json.RawMessage `json:"node_outputs"`; Applied map[string]bool `json:"applied"`; NodesRun []string `json:"nodes_run"`
    LastError *GateError `json:"last_error,omitempty"`; StopReason string `json:"stop_reason,omitempty"`; OverflowHandled bool `json:"overflow_handled"`
    Warnings []string `json:"warnings"`
}
func (s Snapshot) Clone() Snapshot                     // deep copy (slice elements, map values, pointer targets, RawMessage bytes)
func SnapshotEqualIgnoringSeq(a, b Snapshot) bool     // Canonical(marshal(a with Seq=0)) == Canonical(marshal(b with Seq=0))
```
Slices/maps never nil after `Fold`. Marshaling a `Snapshot` uses the same encoder as events (escaping off).

### 2.2 Time — `type Time struct{ time.Time }`; marshals `UTC().Format(RFC3339Nano)`; unmarshals only `Z`-suffixed
RFC3339(Nano); zero value in `Event.At`/`InitData.CreatedAt` → `stamp`/`init_stamp`. Copies keep the parent's `At`.

### 2.3 Canonical bytes and caps

```go
func Canonical(raw []byte) ([]byte, error)   // json.Valid → duplicate-key rejection at every depth (Decoder.Token walk) → Encoder{SetEscapeHTML(false)}.Encode(json.RawMessage) minus '\n'
func OutputHash(raw []byte) string           // hex sha256 of Canonical(raw)
func LineHash(line []byte) string            // hex sha256 of a stored line (no '\n')
```
`Canonical` is idempotent; key order preserved; whitespace removed; `<>&` and U+2028/9 left as literal UTF-8.
All caps are measured on canonical bytes and enforced in `Apply` (reason `oversize`, naming the field):

| cap | value | applies to |
|---|---|---|
| `MaxShort` | 1 KB | every string not listed below (names, codes, gates, atoms, reasons, errors, `Type`, `Node`, `State`, `Kind`, ids, `Commit`, paths, hashes, `Model`, `Effort`, `Verdict` strings, severities, categories, sources, `Vars`/`FileHashes` keys, `Class`) |
| `MaxText` | 4 KB | `Finding.IssueText`, `Golden.Comment`, `Vars` values, `Argv` elements, `WarnData.Detail`, `ConvergeData.Reason` |
| `MaxDesc` | 2 KB | `Bug.Desc` |
| `MaxDetail` | 64 KB | `GateError.Detail`, `cmd_call.Stdout`, `overflow_handler.Stdout`, `tree.Status` (writer truncates via `CapText`, flag set) |
| `MaxStderr` | 8 KB | `cmd_call.Stderr`, `overflow_handler.Stderr` |
| counts | `Vars` ≤ 64; `Goldens` ≤ 64; `AllowedCmds` ≤ 16; `Argv` ≤ 32; `FileHashes` ≤ 64; `Findings`/`Confirmed`/`Status` per delta ≤ 256; `Warnings` ≤ 1024 |
| `MaxPayload` | 256 KB | canonical `Data` of **every** event (sufficient: worst-case `init` ≈ 16×(32×4 KB+64×2 KB) ≈ 4 MB is prevented by this cap, so a cap-legal-but-huge `init` is refused at `Create` with `oversize` — writers must stay within `MaxPayload`, and `node_output.output`/`llm_call.verdict`/`record.data` are single fields under it) |
| `MaxLine` | 1 MB | the stored line; always satisfiable given `MaxPayload` + the `MaxShort` envelope caps, so no separate line check is implemented (`ERR_EVENT_TOO_LARGE` is reserved) |
| `MaxEvents` | 100 000 default, constructor option | counts only `needs_input`, `node_output`, `delta_applied`, `llm_call`, `tokens`, `record`, `tree`; `transition`, `warn`, `fork`, `overflow_handler`, `cmd_call`, `fix_baseline` are exempt (`ERR_AUDIT_FULL`) |

### 2.4 Errors

```go
type FoldError  struct { Code, Reason string; Seq int64; Type string }
type StoreError struct { Code string; Seq int64; Detail string; Cause error }
```
| `Reason` | `Code` | | `Reason` | `Code` |
|---|---|---|---|---|
| `empty` | `ERR_AUDIT_EMPTY` | | `output_after_delta`, `delta_without_output`, `second_delta`, `output_hash` | `ERR_AUDIT_INVALID` |
| `version` | `ERR_AUDIT_VERSION` | | `status_not_subset`, `status_incomplete`, `status_duplicate` | `ERR_AUDIT_INVALID` |
| `first_not_init`, `second_init`, `seq_gap`, `unknown_type`, `bad_payload`, `oversize` | `ERR_AUDIT_INVALID` | | `post_terminal`, `provenance`, `stamp`, `init_stamp`, `mock_stamp`, `node_scope` | `ERR_AUDIT_INVALID` |
| `fix_baseline_head`, `fix_baseline_kind`, `fix_baseline_order`, `unsanctioned_cmd`, `bad_outcome` | `ERR_AUDIT_INVALID` | | | |

`StoreError` codes: `ERR_STORE_PATH`, `ERR_RUN_EXISTS`, `ERR_RUN_NOT_FOUND`, `ERR_RUN_LOCKED`, `ERR_AUDIT_CHAIN`,
`ERR_AUDIT_CAS`, `ERR_AUDIT_TORN`, `ERR_AUDIT_TAIL_CHANGED`, `ERR_AUDIT_NOT_TORN`, `ERR_EVENT_TOO_LARGE`,
`ERR_AUDIT_FULL`, `ERR_APPEND_REJECTED` (`Cause` is the `*FoldError`).

---

## 3. Events

```go
type Event struct {
    SchemaVersion int `json:"schemaVersion"`; Seq int64 `json:"seq"`; Prev string `json:"prev"`; At Time `json:"at"`; Type string `json:"type"`
    State State `json:"state,omitempty"`; Iter int `json:"iter"`; Node string `json:"node,omitempty"`; Mock bool `json:"mock,omitempty"`
    Origin *Origin `json:"origin,omitempty"`; Data json.RawMessage `json:"data"`
}
type Origin struct { RunID string `json:"run_id"`; Seq int64 `json:"seq"`; Version int `json:"version"`; Hash string `json:"hash"` }   // immediate parent; Hash = LineHash of that parent's stored line in audit file Version
```
Ownership: the machine sets `SchemaVersion`, `At`, `Type`, `State`, `Iter`, `Node`, `Mock`, `Data`, and (on copies)
`Origin`; the store sets `Seq`, `Prev`. Copies retain the other machine-set fields verbatim; a copy of a copy gets a
fresh `Origin` naming the immediate parent.

### 3.1 Types and payloads (snake_case tags on every struct)

| `Type` | payload | node-scoped | notes |
|---|---|---|---|
| `init` | `InitData{RunID, CreatedAt Time, Workflow, WorkflowHash, Vars, Calibration, Mock, RepoMode, AllowedCmds, CmdsSHA256, RepoRoot, WorkDir, BaseSHA, Head, InitialState State, InitialKind Kind, Goldens, ParentRunID, Lineage []string, ForkedAtSeq}` | no | seq 1 only |
| `tree` | `TreeData{Head, TreeHash, Status string; StatusTruncated bool}` | no | carrier (cadence: spec 2) |
| `needs_input` | `{}` | yes | |
| `node_output` | `NodeOutputData{Output json.RawMessage}` | yes | |
| `delta_applied` | `DeltaAppliedData{Delta; OutputHash string}` | yes | |
| `llm_call` | `LLMCallData{Kind, Model, Effort string; Index int; InputHash string; Verdict json.RawMessage; Confidence float64; Tokens TokenTotals; DurationMS int64; Error string}` | yes | |
| `cmd_call` | `CmdCallData{Name string; Argv []string; InputHash, Stdout, Stderr string; StdoutTruncated, StderrTruncated bool; ExitCode int; DurationMS int64; Error string}` | no | `Name ∈ AllowedCmds` |
| `gate` | `GateData{Name string; Passed bool; Error *GateError}` | no | `Passed:false` requires `Error` |
| `converge` | `ConvergeData{Atom string; Class Outcome; Stop bool; Reason string}` | no | |
| `transition` | `TransitionData{From, To State; Gate string; Outcome Outcome; Loop bool; ToKind Kind; Head string}` | no | |
| `fix_baseline` | `FixBaselineData{Head string}` | no | §4.6 |
| `tokens` | `TokenTotals` | no | |
| `record` | `RecordData{Name string; Data json.RawMessage}` | no | |
| `warn` | `WarnData{Code, Detail string}` | no | |
| `overflow_handler` | `OverflowHandlerData{Name string; Argv []string; InputHash, Stdout, Stderr string; StdoutTruncated, StderrTruncated bool; ExitCode int; DurationMS int64; Error string}` | no | `Name ∈ AllowedCmds` |
| `fork` | `ForkData{ChildRunID string; AtSeq int64}` | no | parent only |

---

## 4. `Apply`, `Fold`, `FoldFull`

```go
type FoldState struct {
    Snapshot
    ChainHead string                 // LineHash of the last stored line; "" before any line; maintained by the store (§5.2), NOT by Apply
    indexes    map[string]int        // node@iter → next llm_call index
    originOpen bool
}
func Apply(st FoldState, ev Event) (FoldState, error)   // pure: copy-on-write of every map/slice it modifies; never copies NodeOutputs values; st untouched on error
func Fold(events []Event) (Snapshot, error)
func FoldFull(events []Event) (FoldState, error)        // reduce(Apply) from the zero state; ChainHead left "" (the store fills it)
```
Deterministic (same events → identical `Canonical(marshal(snapshot))`). Sequence errors: `empty`; `version`
(`SchemaVersion != run.SchemaVersion`); `first_not_init`; `second_init`; `seq_gap` (`ev.Seq != st.Seq+1`);
`unknown_type`; `bad_payload`; `oversize`; `bad_outcome`. After every accepted event `Snapshot.Seq ← ev.Seq`.

- **4.1 `init`** — copies every `InitData` field; `SchemaVersion ← run.SchemaVersion`; `State ← InitialState`; `StateKind ← InitialKind`;
  `Iteration ← 0`; `PrevUnfixed ← nil`; if `InitialKind == KindAgentEdit && ParentRunID == ""`: `FixEntryHead ← Head`. Maps/slices empty.
- **4.2 `node_output`** — `Node != ""` (`node_scope`); `k = Key(Node, Iter)`; `!Applied[k]` (`output_after_delta`); `NodeOutputs[k] ←
  Canonical(Output)`; append `Node` to `NodesRun` if absent (`NodesRun` records nodes that produced output; a node with only `llm_call`s is not listed).
- **4.3 `delta_applied`** — `node_scope`; `NodeOutputs[k]` exists (`delta_without_output`); `!Applied[k]` (`second_delta`); `OutputHash ==
  OutputHash(NodeOutputs[k])` (`output_hash`). `Findings ← Delta.Findings` if non-nil; `Confirmed ← Delta.Confirmed` if non-nil with
  new IDs appended to `AllFound` (first-seen); if `Delta.Status != nil`: its IDs must equal the set of `AllFound` IDs exactly
  (`status_not_subset` / `status_duplicate` / `status_incomplete`), then `Status ← Delta.Status`. Always: `Unfixed ← |{b ∈ AllFound :
  no s ∈ Status with s.ID == b.ID && !s.StillPresent}|`. `Applied[k] ← true`.
- **4.4 `llm_call`** — `node_scope`; `Index == indexes[k]` (`stamp`); `indexes[k]++`; `Tokens += Tokens`. **`tokens`** — `Tokens += payload`.
- **4.5 `tree`** — `Head`, `TreeHash`, `TreeStatus` ← payload.
- **4.6 `transition`** — `State ← To`; `StateKind ← ToKind`; `Head ← Head`; `LastError ← nil`; if `Loop`: `Iteration++`, `v := Unfixed;
  PrevUnfixed ← &v`, `Findings`/`Confirmed` ← empty; if `ToKind == KindAgentEdit && ev.Origin == nil`: `FixEntryHead ← Head`; if
  `Outcome != ""`: must be in `Outcomes` (`bad_outcome`), `Outcome ← Outcome`. **`fix_baseline`** — `StateKind == KindAgentEdit`
  (`fix_baseline_kind`); the immediately preceding event is a `tree` with the same `Head` (`fix_baseline_order`); `payload.Head ==
  Snapshot.Head` (`fix_baseline_head`); `FixEntryHead ← Head`.
- **4.7 Terminal & misc** — `Outcome != ""` is terminal; afterwards only `overflow_handler`, `cmd_call`, `warn`, `record`, `tokens`,
  `tree`, `fork` (`post_terminal`). `gate{Passed:true}` → no-op; `gate{Passed:false}` requires `Error` (`bad_payload`) and sets
  `LastError`; `converge{Stop}` → `StopReason ← Atom`; `warn` → append `Code`; `overflow_handler` → `OverflowHandled ← true`;
  `cmd_call`/`overflow_handler` `Name` must be in `AllowedCmds` (`unsanctioned_cmd`); `record`, `needs_input` (`node_scope`), `fork` → no change.
- **4.8 Provenance** — if `ParentRunID != ""`: events at seqs `2..ForkedAtSeq` carry `Origin{RunID == ParentRunID, Seq == ev.Seq,
  Version ≥ 1, Hash != ""}` and `Type != init`; no `Origin` outside that range; `ParentRunID == ""` ⇒ no `Origin` (`provenance`).
  `Lineage` = `InitData.Lineage` (spec 3 sets it to parent's lineage + parent). `MockTainted ← true` on any `Mock` event when
  `Snapshot.Mock == ""`. Producer invariant: when `Snapshot.Mock != ""`, every non-`init`, non-`Origin` event carries `Mock: true` (`mock_stamp`).
- **4.9 Stamps** — `init`: `State == ""`, `Iter == 0`, `At` non-zero (`init_stamp`). Others: `Iter == Iteration` except a `Loop` transition,
  whose `Iter == Iteration+1`; `State == State` (a `transition` carries `From`); `At` non-zero (`stamp`).

---

## 5. Store

```go
type TornTail struct { Offset int64; Bytes []byte }
type Log struct { Events []Event; Head string; Torn *TornTail; Version int }
type RunSummary struct { RunID, Workflow string; CreatedAt Time; State State; Outcome Outcome; ParentRunID string; Mock string; MockTainted, Torn bool; Sidecars int; Error string }
type Options struct { MaxEvents int }   // zero → default

type RunStore interface {
    Create(runID string, first Event) (FoldState, error)                  // §5.2; ERR_RUN_EXISTS
    Append(runID string, st FoldState, ev Event) (FoldState, error)       // §5.2; returns the advanced state with ChainHead set
    Events(runID string) (Log, error)                                     // ERR_RUN_NOT_FOUND; chain verified; Log.Head = ChainHead
    EventsWithLines(runID string) (Log, [][]byte, error)
    RepairTail(runID string) error                                        // §5.3; ERR_AUDIT_NOT_TORN if the file is clean
    List() ([]RunSummary, error)
    Lock(runID string) (unlock func(), err error)
    Root() string
}
func NewJSONLStore(root string, opts Options) RunStore; func NewMemStore(opts Options) RunStore
func ValidateRunID(id string) error; func RunID(workflow string, at time.Time) string   // = state.RunID("fsm-"+workflow, workflow, at)
```

**5.1 Layout** — `<root>/.metareview/runs/` `0700`; `Create` always ensures `runs/.gitignore` == `"*\n"` (written via temp+rename).
`<id>/` `0700`; `audit.jsonl`, `lock`, `audit.torn-*.bin` `0600`. Every method validates `runID` first. Every component below
`<root>` is `Lstat`-checked (no symlinks; best-effort against on-disk symlinks, not a concurrent local attacker) and files open
`O_NOFOLLOW` (`ERR_STORE_PATH`). `runs/` and `<id>/` fsync'd after `Create`.

**5.2 Write path** — `Create(runID, first)`: `first.Type == init` and `InitData.RunID == runID` else `ERR_APPEND_REJECTED{bad_payload}`;
`ev.Data ← Canonical`; `Seq ← 1`, `Prev ← ""`; `Apply(zero, ev)`; `MaxLine`; create `<id>/` and open `audit.jsonl` with
`O_CREAT|O_EXCL` (`ERR_RUN_EXISTS`); write, fsync line and dirs; return state with `ChainHead`. `Append(runID, st, ev)`: lock
held by this process (`ERR_RUN_LOCKED`); tail: torn → `ERR_AUDIT_TORN`; `LineHash(last) != st.ChainHead || lastSeq != st.Seq`
→ `ERR_AUDIT_CAS`; `Data ← Canonical`; `Seq ← st.Seq+1`; `Prev ← st.ChainHead`; `Apply` (`ERR_APPEND_REJECTED`); `MaxLine`;
`MaxEvents` (counted types only); write + fsync; return `Apply` result with `ChainHead ← LineHash(line)`. On any error the
caller's `st` is unchanged. `Events()`: verify `Seq` contiguity and `Prev` over raw lines (`ERR_AUDIT_CHAIN` with seq;
undecodable complete line → `ERR_AUDIT_CHAIN{"undecodable"}`); readers never re-marshal.

**5.3 Torn tail** — torn ≡ non-empty bytes after the last `\n`. `Events()` returns them as `Torn`, unmodified; `Append` refuses.
`RepairTail(runID)` (lock): re-derive the tail from the file (`ERR_AUDIT_NOT_TORN` if none); create `audit.torn-<nextSeq>-<unixnano>.bin`
`O_CREAT|O_EXCL` (`EEXIST` → `ERR_STORE_PATH`), write, fsync file + dir; if `Offset == 0`: remove `audit.jsonl` then the run
directory except the sidecar is first moved to `<root>/.metareview/runs/.torn/<id>-<unixnano>.bin`, and `lock` is unlinked last;
else truncate through the open descriptor to `Offset`, fsync. The machine appends `warn{AUDIT_TORN_LINE_DROPPED, "<n> bytes in <file>"}`
with correct stamps. Unlocked reads may observe transient `Torn`; act only under the lock.

**5.4 Versioning** — 0.9.0 reads/writes `SchemaVersion == 1` (`version` otherwise). Future contract: `Migrate(v, Log)` writes
`audit.v<N>.jsonl` beside the original with a fresh chain (original retained); `Append`/`RepairTail`/`Events` target the
highest-version file the binary understands (`Log.Version`); `Origin.Version` names the parent file a copy was taken from and
verification (spec 3) reads that file; an older binary finding only newer files reports `version`.

**5.5 `List()`** — folds every conforming run dir (missing `runs/` → empty); errors → summary with `Error` (never aborts);
torn → prefix-derived + `Torn`; order `CreatedAt` desc, `RunID` asc; O(total bytes) accepted. Retention: until the user deletes
the directory (product constraint, spec 5 docs).

**5.6 Lock** — `flock(LOCK_EX|LOCK_NB)` on `<id>/lock` (`O_CREAT|O_RDWR|O_NOFOLLOW`, `0600`, **fresh fd per `Lock`**); failure
→ `ERR_RUN_LOCKED`; released by `unlock`/exit; `Append`, `Create`(no — `Create` needs no lock: `O_EXCL`), `RepairTail` require it.

---

## 6. Run IDs — `RunID(workflow, at)`; carried in `InitData.RunID`.

## 7. Writer contract (spec 2 = `Advance`/`Record`, spec 3 = `Fork`)
Logs must satisfy §4: `init` first (`State ""`, `Iter 0`); stamp every event with the current iteration (loop transitions with
the new one) and current state; `Mock: true` on every native event of a mock run; per `node@iter` outputs then one delta with
`OutputHash(NodeOutputs[k])`; supplied `Status` covers `AllFound` exactly (every verify statuses every bug ever found);
`Outcome` only from the payload (incl. `failed`); post-terminal allow-list; `cmd_call`/`overflow_handler` names from `AllowedCmds`;
fork copies are exactly parent seqs `2..ForkedAtSeq` with `Origin{parent, seq, version, hash}` and `ForkedAtSeq` equal to the
number of copied events + 1; a fork into an agent-edit state appends `tree{Head: HEAD}` then `fix_baseline{Head: HEAD}`; `ToKind`
on every transition; truncate via `CapDetail`/`CapText`; hash with `OutputHash`.

---

## 8. Tests (`internal/fsm/run`, 100% statements, no other `fsm` import)

```go
type Builder struct{ … }                                   // in-package test helper (under the gate)
func NewBuilder(runID string) *Builder                      // literal stamps; every field overridable
func (b *Builder) Init(d InitData, o ...Override) Event; func (b *Builder) Event(typ string, data any, o ...Override) Event
func (b *Builder) Events() []Event                          // Seq/Prev derived by literal counters unless overridden; never calls Apply
func (b *Builder) Copy(parent []Event, from, to int64, childID string) []Event   // fork copy per §7 (child init + Origin copies)
```
| # | test | discriminates |
|---|---|---|
| R1 | fold table: every rule in §4.1–4.9 with negative/second-instance rows incl. `transition{To:"failed",Outcome:""}` non-terminal; `Unfixed` 3-bug fixture → 2; `Status` after `AllFound` grew → `status_incomplete`; copied agent-edit transition leaves `FixEntryHead` empty; fork→`tree`→`fix_baseline` sets it; `fix_baseline` without preceding `tree` / head mismatch / wrong kind; per-type post-terminal accept **and** reject rows; a node re-run at iteration 2 with interleaved keys (`NodeOutputs`/`Applied`/`indexes` keyed by `node@iter`); loop transition `Iter == N+1` accepted, `N` rejected; `gate{true}` no-op; `unsanctioned_cmd`; `node_scope` | every derivation |
| R1b | no-op rows via `SnapshotEqualIgnoringSeq`; plus its own mirror rows (true only when `Seq` differs; false for each other field) | no-ops and the oracle |
| R2 | one row per `Reason` asserting `{Code, Reason, Seq, Type}` from the §2.4 table | typed fail-closed |
| R2b | `testdata/oracle.jsonl` + `oracle.sha256` generated by `testdata/oracle.sh` from `oracle.template.jsonl` (`shasum -a 256`/`sha256sum`), with `oracle.events.jsonl` as the non-canonical source: accept; one-byte edit → `ERR_AUDIT_CHAIN`; `LineHash`; `Canonical` on a non-canonical input (whitespace, `<`, U+2028) equals the oracle and is idempotent | chain + canonical form |
| R3 | order rows (`AllFound`, `NodesRun`, `Warnings`, `Status`, ≥3 elements); repeat folds byte-equal; JSONL vs mem byte-equal | ordering |
| R4 | per-prefix goldens (`testdata/golden-log.jsonl`, `golden-snapshots.jsonl`; regression-only, authority = R1/R2b/R13; regenerated only with `FSM_RUN_UPDATE_GOLDEN=1`); mutation table {delete, duplicate, swap} × 16 types with enumerated outcomes | compositional |
| R5 | recursive reflect `Clone` walk; mutate in place both directions | copy depth |
| R6 | `PrevUnfixed` nil/copy/second loop/JSONL round trip | aliasing |
| R7 | conformance over both stores (`Options{MaxEvents: 5}` to reach `ERR_AUDIT_FULL` and its exemptions): `Create` refusals + `ERR_RUN_EXISTS`; `ERR_RUN_NOT_FOUND`; CAS on `ChainHead`/`Seq`; no lock; second `Lock`; `ERR_APPEND_REJECTED` leaves file/state unchanged; `ERR_EVENT_TOO_LARGE`; dup keys at depth 3; canonical storage; `Root()`; `EventsWithLines`. JSONL-only: modes, `.gitignore` exact content re-ensured, symlink → `ERR_STORE_PATH`, fsync path, flock release, fresh fd | store contract |
| R8 | torn (a)(b)(c) → `Torn`, `Append` → `ERR_AUDIT_TORN`; `RepairTail` sidecar name/`O_EXCL`/lock/`ERR_AUDIT_NOT_TORN`; offset-0 → sidecar moved to `.torn/`, dir removed; undecodable complete last line → `ERR_AUDIT_CHAIN` | repair |
| R9 | `List()` order, `Error` rows, missing `runs/`, `Torn`/`Sidecars`, id table | listing |
| R10 | `Builder.Copy` child folds equal to parent prefix except id/parent/lineage/forked/created/`FixEntryHead`; each provenance violation; `ForkedAtSeq` > copies (accepted by `Fold`, documented spec-3 precondition); `MockTainted` both ways; `mock_stamp` exempting copies | provenance |
| R11 | `SchemaVersion != 1` → `version` | exact match |
| R12 | every cap: at-cap accepted + one-over rejected, using escape-expanding content; `CapText` exactly-max unchanged/flag false | boundaries |
| R13 | value pins: `BugID("x")`, `Key`, `Add` (five distinct), `Total`, every `Reason`/`Code` string, `Outcomes` | constants |
| R14 | `Time` marshal/unmarshal/zero | time |

## 9. Exports — `Fold`, `FoldFull`, `Apply`, `FoldState`, `Builder`+methods, `Canonical`, `OutputHash`, `LineHash`, `CapDetail`,
`CapText`, `BugID`, `Key`, `ValidateRunID`, `RunID`, `SnapshotEqualIgnoringSeq`, `Time`, all wire/payload/store types
(`Event`, `Origin`, `Snapshot`, `Log`, `TornTail`, `RunSummary`, `Options`, `InitData`…), constants (`SchemaVersion`, `Max*`,
`Outcome*`, `Outcomes`, `KindAgentEdit`, reason/code strings), `FoldError`, `StoreError`, `RunStore`, `NewJSONLStore`, `NewMemStore`.

## 10. (reserved)

## 11. Amendment ledger (parent plan r3 → this spec; accepted with the split/proceed decisions)

| parent contract | change |
|---|---|
| C21 `state.json` | removed. |
| §1.2 `replay` | verbatim copies + `Origin{run_id, seq, version, hash}`; `Lineage` in `init`. |
| §1.7.4 `vars_used` | removed; spec 3 freeze rule over `NodesRun`. |
| §1.3 `Delta.Tokens` | removed. |
| C15 record types | `record tokens`/`record node-output` → types `tokens`/`node_output`. |
| design §10 diff snapshots | `tree` event with porcelain status. |
| §1.5 step 8 `FixEntryHead` | derived from non-copied `ToKind`/`InitialKind`, or `tree`+`fix_baseline`. |
| §1.5/1.6 outcome from `→failed` | payload-supplied. |
| §1.2 `Fold(events, kinds)` re-Decode | `Fold` type-blind; spec 3 re-validates copies. |
| §1.4 `Unfixed = count(StillPresent)` | unstatused bugs count as unfixed (fail closed); spec 2's gates/atoms read this definition. |
| §1.2 `ExecMode` | moved to `kind`. |
| §1.2 `ValidateRunID` | `{8,200}`. |
| §1.2 `Detail` full in audit | ≤ 64 KB + flag. |
| §1.2 `transition.tree_hash` | `tree` event. |
| §1.2 `Lock` pid file | `flock`; no stale-pid logic; local FS only (product constraint → spec 5 docs). |
| §1.2/§1.7 torn tail dropped by `Events()` | fail-closed refusal + `RepairTail` with sidecar. |
| §1.2 `List()` order | `CreatedAt` desc, id asc. |
| scope rule | widened in r3 to include the writer contract (§7). |
| new product constraints | `MaxEvents` (`ERR_AUDIT_FULL`), local-FS-only, retention-until-deleted → spec 5 docs. |
| four-spec split | five specs (§12). |

## 12. Ownership ledger (partition)

| # | spec | owns | findings (attempt-3 build-plan blockers + carried partials) |
|---|---|---|---|
| 1 | **run** | plan §1.2, §1.4, persistence half of §1.7 | ARC3-1/2/3/4/6/10, DM3-2/4, CMP3-2/4/6, SEC-21, FIN-1/6/7, TQ3-1/9, CMP3-8 (run codes), ARC-17/18, TQ-2 |
| 2 | **workflow + gate + converge + machine core** | plan §1.1, §1.3 gate/converge/workflow types, §1.5 `Advance`/`Record`, §1.6, §6 + `workflows` pkg, `cmds_sha256` preimage, `PrevUnfixed == nil` rule, `tree` cadence + `TreeHash` preimage + `UNSANCTIONED_EDIT`, repair-`warn` emission, `Status` full-coverage input contract for verify | CMP3-1/5/7(machine)/9, ARC3-5/7/8/9, INT-16, TQ3-10/11, SCP3-4/5, SEC-23, RI2-3, CMP-15/17/19, ARC-20/21, TQ-7/9 |
| 3 | **fork / resume / diff / export / runs.jsonl** | plan §1.7 fork half, `Origin` verification incl. version, `Lineage`, freeze rule, git preconditions + `tree`→`fix_baseline`, `--work-dir`, copy re-validation, `ForkedAtSeq` precondition, `diff`, `export` (redact `Vars`, `cmd_call`/`overflow_handler` streams, `llm_call.Verdict`, `record.data`, `node_output`, `tree.status`; include sidecars; manifest), `fsmRunRecord` + verdict map + `ESCALATED` | INT-9/10/13/14/18/19, FEA-N3/N4, DM3-1/3/5, DMN-1/6, SEC-13/22/27, FIN-2, TQ3-2/3/7, SCP3-1/2/3/8, CMP3-10, SEC-7/16, DM-2 |
| 4 | **guardrails + judge + kinds + mockai** | plan §1.8, §2, kinds/`Executor`/`Delta` producers, match-then-adjudicate + `Bug.Verdict`, `index`, scenarios, `JUDGE_EFFORT`, pinned harnesseval sha | INT-11/12/17/20/21/22, SEC-11/12/14/24/25/26/28/29, FIN-3/4/5, CMP3-3, TQ3-8, TQ-8 |
| 5 | **CLI + tests + milestones + docs** | plan §3, §4, §5, §0/§7 ledgers, M8 docs (exit 3, path-less exit 1, `.metareview/runs/` transient + `gitpolicy.go`, trust boundary, enforcing caveat, judge-swap limitation, spec amendments §10.1/§14.3/§17, torn-repair UX, `ERR_AUDIT_FULL`, local-FS-only, retention), `package.json` + `go.sum`, forbidden-phrase grep, CI (`test.yml`) | CMP3-7(CLI), TQ3-4/5/6/12, SCP3-6/7, INT-15/23, DM3-6/7/8, SEC-30, FIN-8, CMP-18, TQ-12, DMN-9 |
