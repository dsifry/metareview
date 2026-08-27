# metareview 0.9.0 — spec 3: fork/resume, diff, export, and the runs.jsonl record

> **Status:** DRAFT r3 (2026-08-27). Third of the five split artifacts (ownership ledger: run spec §12 row 3, plus
> CMP-15/ARC-21 reassigned from spec 2). Owns plan r3 §1.7's fork half, `Origin` verification incl. version, `Lineage`,
> the var-freeze rule, git preconditions + `tree`→`fix_baseline`, `--work-dir` validation, re-validation of copied
> events, `fsm diff`, `fsm export` (redaction, sidecars, manifest), the `fsmRunRecord` in `.metareview/runs.jsonl`
> with the verdict map, and the `ESCALATED` mapping. Builds on the implemented `run`, `machine` (r4), `kind` (r5).
>
> **r3 changes** (attempt-1 review of r2, all eight lenses): the `forked` runs.jsonl row is **gone** — rows are written
> only at terminal (C27 honoured), `attemptNumber` is derived from `Lineage`, and `ESCALATED` is produced by the recorder
> itself (§6) and enforced by `Fork` through `Deps.Escalated` (no `runchain.Resolve` call anywhere); `snapshot.json` is a
> **redacted projection** and the export is declared evidence, not a foldable log (`audit.redacted.jsonl`, per-line
> `redacted` markers, original `seq`/`prev` bytes kept); the redaction table is per event type and per field, and var
> values are replaced by `$NAME` wherever argv appears; `HEAD == ch` for **every** state (the ancestor allowance is
> dropped: "fork, then commit"); `AcceptWorkflowChange` carries a prefix-compatibility invariant (`ERR_WORKFLOW_INCOMPATIBLE`);
> the child log is built and folded **in memory** before `run.Create` (`ERR_COPY_INVALID{seq, reason}` never leaves a
> directory behind); P is locked and torn-checked first; `ERR_FORK_INCOMPLETE`; terminal `From` refused; the
> freeze rule covers convergence/overflow `cmd`s; `VerifyOrigin` reads the parent once and checks content;
> `Diff` separates `DecisionSame`/`ConfidenceSame`, aligns judge rows by input hash, and takes the decision accessor from
> `kind`; `RecordTerminal` and `Export` move out of `machine` into `internal/fsm/record` and `internal/fsm/export`
> (FS seams; destination safety); the runs.jsonl row gains `workflowHash`/`workflowSource`; a ledger (§9).
>
> **Scope rule:** what `Fork`, `Diff`, `Export`, and the runs-record *produce*. The CLI shapes are spec 5's.

---

## 1. Packages
```
internal/fsm/machine   fork.go (Fork, VerifyOrigin), diff.go (Diff)
internal/fsm/record    record.go (Terminal recorder, Escalated)      — imports internal/runchain + internal/state
internal/fsm/export    export.go (Export, Redact*)                   — imports run, machine (types only)
```
`machine` keeps its charter (it decides; effects sit behind `Deps`). Two new fields on `machine.Deps`, both optional:
`Escalated func(ctx, runID string) (bool, error)` (nil → never escalated) and — unchanged — `Terminal`. The CLI wires
`Deps.Terminal = record.Terminal(root, clock)` and `Deps.Escalated = record.Escalated(root)`. `tests/coverage.sh` already
gates every `internal/fsm/*` package at exactly 100%.

## 2. `Fork`
```go
type ForkOptions struct { From run.State; AtIter *int; Vars map[string]string; WorkDir string; AcceptWorkflowChange bool; AllowCustomCmds string; WorkflowBytes []byte /* only with AcceptWorkflowChange */ }
type ForkResult struct { ChildRunID string; ForkedAtSeq int64; Copied int; CmdsSHA256 string; DroppedVars []string }
func (m *Machine) Fork(ctx, ForkOptions) (*Machine, ForkResult, error)
```
Algorithm. P is `m` (opened via `load`). **P's lock is held for steps 0–9**; the checkpoint, the copied lines and the
`fork` stamps all come from one `EventsWithLines` read taken under that lock.
0. **Preconditions.** `ctx.Err()` is returned as-is at every step. P torn → `ERR_AUDIT_TORN` (as `Advance`/`Record`).
   `Deps.Escalated(P.RunID)` true → `ERR_RUN_ESCALATED{run}` (the completion rule: a human must narrow, split, or redesign).
1. **Checkpoint.** `From` must be a state of P's workflow (`ERR_CHECKPOINT_NOT_FOUND{from, reason: unknown_state}`) and
   not terminal (`reason: terminal_state`). `seq` ← the `transition` event with `To == From` and, if `AtIter` set,
   `Iter == *AtIter`; latest such transition when `AtIter` is nil. If none: `From == P.Initial` and (`AtIter` nil or
   `*AtIter == 0`) → `seq = 1` (a restart: nothing is copied); otherwise `ERR_CHECKPOINT_NOT_FOUND{from, at_iter}`.
   (On a looping workflow `--from <initial>` without `--at-iter` therefore means the latest re-entry, and `--at-iter 0`
   means the restart.) A terminal P is allowed (judge swap on a finished run).
2. **Workflow bytes.** `raw ← P's sidecar` (verified by `load`). If `WorkflowBytes != nil`: `AcceptWorkflowChange` must be
   set (`ERR_WORKFLOW_CHANGED{expected: P.WorkflowHash, got: sha256(WorkflowBytes)}`), `len ≤ MaxWorkflowBytes`
   (`ERR_WORKFLOW_TOO_LARGE`), `raw ← WorkflowBytes`. `w ← workflow.Parse(raw, workflow.Options{Kinds: Kinds.Info()})`
   (`ERR_WORKFLOW_INVALID` passes through). `WorkflowHash ← sha256(raw)`.
   **Compatibility invariant** (only when the bytes changed; violation → `ERR_WORKFLOW_INCOMPATIBLE{reason, state?}`):
   `w.Name == P.Workflow` (`reason: name`); `w.Initial == P's initial` (`initial`); every state stamped on a copied event
   `2..seq` and `From` itself exists in `w` with the same node **name** and **kind** (`state`, with the state); every
   `cmd` name referenced by a copied `cmd_call`/`overflow_handler` is still declared (`cmd`). Other differences (gates,
   convergence, models, effort, prompts, added states) are accepted — that is what the flag consents to.
3. **Vars.** P's stored vars filtered to `w`'s declared set (`DroppedVars` = the removed names; only possible with a
   workflow change) overlaid by `Vars` (undeclared → `ERR_VAR_UNKNOWN`). For each overridden `K`: **frozen** if
   `K ∈ w.VarsReferencedBy(s)` for any state `s` whose node ∈ `NodesRun` of `Fold(P[1..seq])`, or `K ∈ w.CmdRefs[c]` for
   any `cmd` `c` named by a copied `cmd_call`/`overflow_handler` — evaluated on P's **resolved** workflow (its `Refs`/`CmdRefs`
   survive `Resolve`) → `ERR_VAR_FROZEN{name, state}` with `state` = the first frozen state in `w.States` order (a `cmd`
   reference reports the state whose convergence ran it, the overflow handler reports P's state at the time).
   `Resolve(effective, P.Calibration)` — a calibration parent refuses `JUDGE`/`JUDGE_EFFORT` overrides through the
   existing `CodeCalibrationPinned` (`ERR_CALIBRATION_PINNED`).
4. **Work dir.** `WorkDir` (default P's) must be absolute (`ERR_WORKDIR_FOREIGN{reason: relative}`) and
   `Git(WorkDir).CommonDir == Git(P.RepoRoot).CommonDir` (`ERR_WORKDIR_FOREIGN{reason: other_repo}`).
5. **Commands.** `ResolveCmds(w, WorkDir, LookPath, FileHash)`; if `len > 0 && sha != P.CmdsSHA256` → require
   `AllowCustomCmds == sha` (`ERR_CMDS_NOT_ALLOWED{sha}` with the printed list, exactly as `Init`). Copied `cmd_call`s are
   validated against the **new** `AllowedCmds` by the in-memory fold (step 7), so a renamed command is caught as
   `ERR_WORKFLOW_INCOMPATIBLE{cmd}` (step 2) before consent is asked.
6. **Git precondition (every state).** `ch` = checkpoint head (`TransitionData.Head` of the checkpoint event;
   `InitData.Head` for `seq = 1`). `HEAD(WorkDir) == ch`, else `ERR_TREE_NOT_AT_CHECKPOINT{expected: ch, got: HEAD}` with
   `Detail` = `git worktree add <dir> <ch>` hint. The fork is the node's starting line: **fork first, then commit** — a
   commit made before the fork would sit below the child's `fix_baseline` and be invisible to `commit_exists` (spec 5's
   recovery recipe says so).
7. **Build the child in memory.** Child id `run.RunID(w.Name, Clock().Time)`; `init` = P's `InitData` with `RunID`,
   `CreatedAt`, `ParentRunID: P`, `Lineage: P.Lineage + P`, `ForkedAtSeq: seq`, `WorkDir`, `Vars` (effective),
   `AllowedCmds`/`CmdsSHA256` (step 5), `WorkflowHash` (step 2), `Head: ch`, `Mock` (P's — a mock run forks as a mock run;
   the registry must agree, `ERR_MOCK_MISMATCH`). Then P's events `2..seq` verbatim (same `Data` bytes, `At`, `State`,
   `Iter`, `Node`, `Mock`) each with `Origin{RunID: P, Seq, Version: P.Log.Version, Hash: LineHash(P line)}`; then
   `tree{Head: ch, TreeHash: TreeHash(ch, WorkTree), Status}` stamped with the checkpoint's state/iter; then, if the node at
   `From` is `agent-edit`, `fix_baseline{Head: ch}`. `run.FoldFull` over this sequence **before any I/O**: a `*FoldError`
   at copied seq `n` → `ERR_COPY_INVALID{seq: n, reason}` (this is where a tightened kind `Decode`, an `output_hash`
   mismatch, a `cmd` no longer sanctioned, or `MaxEvents` are caught); nothing has been created. Copied `cmd_call`s count
   toward the child's ordinals (they are in its log).
8. **Write the child.** `run.Create(init)`; `Sidecar.Write(child, "workflow.yaml", raw)`; under the child's lock append the
   remaining events in order. A crash inside this step leaves an **incomplete fork**: a child whose `Seq ≤ ForkedAtSeq`
   (no rebaseline `tree`). `Open` on such a run returns `ERR_FORK_INCOMPLETE{run, parent}`; `List` reports it in
   `RunSummary.Error`; spec 5 documents manual deletion. `init.parent_run_id`/`lineage` are authoritative; P's `fork` event
   (step 9) is advisory.
9. **Parent.** Append `fork{ChildRunID, AtSeq: seq}` to P (still under P's lock). No runs.jsonl row is written here (§6).
10. Return the child machine (`Open`ed): its first `Advance` re-runs the node at `From` from `StartIndex` (no copied
    `llm_call` for that key exists — the prefix ends at the transition **into** `From`) and evaluates `From`'s gates. A
    fork from a state after `fix` in the same iteration has `FixEntryHead == ""` (copied transitions never set it);
    `commit_exists` then fails closed with `ERR_GATE_INAPPLICABLE` — only `fix → verify` uses it in the shipped workflows.

The judge-swap limitation (spec 5 docs): on `sdlc-loop` `--var JUDGE=…` is accepted only at `--at-iter 0` (adjudicate and
verify both reference `$JUDGE` and ran at every later iteration).

## 3. `VerifyOrigin`
`VerifyOrigin(ctx, store, child run.Log) []OriginCheck` — reads the parent's `EventsWithLines` **once** (indexed by seq)
and, for each `Origin`-bearing child event, reports `{Seq, OK bool, Reason}` with `Reason ∈ ok | parent_missing |
parent_unreadable (any store error incl. ERR_AUDIT_CHAIN/TORN) | version_unavailable (Origin.Version != parent
Log.Version) | hash_mismatch (LineHash) | content_mismatch (Data, At, State, Iter, Node, Mock differ from the parent
event)`. Used by `fsm diff` and `fsm export` (reported, never fatal).

## 4. `Diff`
```go
type DecisionFunc func(kind string, verdict json.RawMessage) *bool      // kind.Decision implements it (spec 4's vocabulary stays in kind)
func Diff(a, b run.Log, decide DecisionFunc) (Report, error)
type Report struct { A, B string; SameWorkflow bool /* WorkflowHash equal */; CommonPrefixSeq int64; Outcomes [2]run.Outcome; Calls []CallRow; Transitions []TransRow }
type CallRow struct { Node string; Iter int; Kind, InputHash string; A, B *CallSide; DecisionSame, ConfidenceSame, Same bool }
type CallSide struct { Index int; Model, Effort string; Decision *bool; Confidence float64; Error string }
type TransRow struct { SeqA, SeqB int64; To run.State; Gate string; Outcome run.Outcome; Same bool }
```
Judge rows align by `(node, iter, kind, input_hash)` — not by ordinal, because `match-then-adjudicate` indexes depend on
the seen set and a different judge changes it; each side reports its own `Index`. `Decision` = `decide(kind, verdict)`
(nil when the verdict is `null`, the bool is absent, or the kind has no decision field). `DecisionSame` = both present ∧
`Decision` equal; `ConfidenceSame` = both present ∧ `Confidence` equal; `Same = DecisionSame ∧ ConfidenceSame`.
`Same` additionally requires both `Error == ""` (an errored call is never `Same`). `reasoning` never participates. Transitions align by ordinal; `TransRow.Same` = both present ∧ `To`, `Gate`, `Outcome` equal. Different `Workflow` names → `ERR_DIFF_INCOMPATIBLE`.
`CommonPrefixSeq` = the largest `n` such that events `2..n` of both logs have identical `Data`. Output is deterministic
(rows sorted by `(node, iter, kind, input_hash)`; transitions by ordinal).

## 5. `Export`
```go
type ExportOptions struct { Out string; IncludeVars bool; MaxBytes int64 /* default 5 MB */ }
type FS interface { MkdirAll(dir string, perm os.FileMode) error; Create(path string) (io.WriteCloser, error) /* O_CREATE|O_EXCL|O_WRONLY|O_NOFOLLOW 0600 */; Lstat(path string) (os.FileInfo, error); ReadDir(dir string) ([]os.DirEntry, error) }
func Export(ctx, deps Deps /* Store, Sidecar, FS, Clock */, runID string, opts ExportOptions) (Manifest, error)
type Manifest struct { SchemaVersion int /* 1 */; RunID, Workflow, WorkflowHash string; ExportedAt run.Time; SourceHead string /* snapshot Head */; IncludeVars bool; Events int; Redacted []int64 /* seqs with a redacted marker */; Sidecars []string; TornFiles []TornFile /* {Name, SHA256, Bytes} — listed, never copied */; OriginChecks []OriginCheck; Bytes int64; Chain string /* "redacted" */ }
```
**Contract.** The export is *evidence for a reader*, not a foldable log: redacted lines keep their original `seq` and
`prev` bytes (nothing is re-chained), gain a top-level `"redacted": ["field", …]` array, and therefore no longer
hash-verify; unredacted lines are byte-identical to the source. `Chain` is always `"redacted"`.

**Files.** `<Out>/{manifest.json, audit.redacted.jsonl, snapshot.json, workflow.yaml}` plus a copy of every sidecar in
`Sidecar.List` (all names pass `ValidSidecarName`). `audit.torn-*.bin` are **not** copied (raw, never-redacted bytes):
`RunStore.TornFiles(runID) ([]TornFile, error)` lists them for the manifest (run spec §11 ledger).

**Redaction table** (applies to every event, copied ones included; the snapshot mirrors it):
| event / field | rule |
|---|---|
| `init.vars` values | `sha256:<hex>` unless `IncludeVars` |
| `init.allowed_cmds[].argv`, `init.allowed_cmds[].file_hashes` keys, `cmd_call.argv`, `overflow_handler.argv` | every occurrence of a var value (`len ≥ 4`) replaced by `$<NAME>` unless `IncludeVars`; then paths relativised (below) |
| `init.repo_root`, `init.work_dir`, `init.mock` | relative to the repo root when inside it, else `<outside>` |
| `init.goldens[].comment` | emptied |
| `gate.error.detail` | `commit_exists`: replaced by `detail_summary {files, insertions, deletions}`; every other gate: emptied; `detail_truncated` kept |
| `cmd_call`/`overflow_handler` `stdout`/`stderr` | emptied, `*_truncated: true` |
| `converge.reason`, `warn.detail` | emptied |
| `llm_call.verdict` | kept (typed JSON incl. `reasoning` — the §17 audit needs it) |
| `llm_call.error` | class prefix only (text before the first `: `), `error_truncated: true` |
| `node_output.output` — `review-lenses` | findings keep `id, file, line, severity, category, source, issue_text` (no `desc`) |
| `node_output.output` — `match-then-adjudicate` | `rejected[].desc` emptied (spec 4 handoff); `confirmed` kept |
| `node_output.output` — other kinds; `delta_applied` | kept (`delta_applied.findings[].desc` emptied like above) |
| `record.data` | kept verbatim (host-supplied, durable, **unredacted** — spec 5 documents it) |
| `tree.status` | reduced to the sorted file list |
| `transition`, `tokens`, `fork`, `fix_baseline`, `llm_call` other fields | kept |
| `workflow.yaml` | copied verbatim (var **defaults** are workflow text; secrets travel as declared `env` names, never as vars) |

`snapshot.json` = `Redact(FoldFull(source log))`: the same rules applied to `Vars`, `AllowedCmds`, `RepoRoot`/`WorkDir`,
`Mock`, `TreeStatus`, `LastError.Detail` (emptied), and `NodeOutputs` (per kind as above); `SchemaVersion` kept.

**Destination safety.** `Out` default = `<repo root>/docs/metareview/fsm/<run-id>/`. Every path component under `Out`
is `Lstat`-checked (symlink → `ERR_EXPORT_DEST{reason: symlink}`); an existing non-empty `Out` → `ERR_EXPORT_DEST{reason:
not_empty}`; directories 0700; files created `O_EXCL|O_NOFOLLOW` 0600. The whole bundle is built in memory and
`MaxBytes` is checked **before the first write** (`ERR_EXPORT_TOO_LARGE{bytes, max}` writes nothing). `--include-vars`
output must never be committed (spec 5 docs); exports are one-way (`audit.redacted.jsonl` cannot be copied back).

## 6. `.metareview/runs.jsonl` record (C27)
`record.Terminal(repoRoot string, clock func() run.Time) func(ctx, machine.View) error` — the `Deps.Terminal`
implementation, called by the machine on every terminal advance (incl. `failed` and the resume path). **One row per run,
appended at terminal only** (C27). Idempotent: if a row with this `id` exists, nothing is appended (a run reaches terminal
once; the resume path re-calls with the same view). Shape follows the `taskdone`/`prready` writers
(`internal/taskdone/review.go:40-60`), decoded by `runchain.ReadRuns` unchanged; new keys are additive (`schemaVersion` stays 1):
```json
{"schemaVersion":1,"id":"<run>","scope":"fsm-<workflow>","target":{"type":"fsm","id":"<workflow>@<base_sha[:12]>"},
 "status":"complete","verdict":"PASS|NEEDS_REVISION|ESCALATED","executionMode":"fsm","previousRunId":"<parent or omitted>",
 "attemptNumber":N,"maxAttempts":3,"baseSha":"…","headSha":"…","createdAt":"<RFC3339Nano>","updatedAt":"<RFC3339Nano>",
 "repoRoot":"…","contextPackPath":"","reviewLogPath":"","mock":false,"outcome":"fixed","fsmRunDir":".metareview/runs/<run>/",
 "workflowHash":"<sha256>","workflowSource":"embedded|path","escalationReason":""}
```
`attemptNumber = len(Lineage) + 1` (root = 1; derived from the run's own `init`, no parent row needed); `previousRunId` =
`ParentRunID` (omitted for roots — the existing decoder accepts both); `mock` = `Snapshot.Mock != ""` (a mock row never
satisfies a gate — spec 5 states it in `status`/docs); `workflowSource` from the init event (spec 5 adds it to `InitData`
via spec 2's ledger: `embedded` when `--workflow <name>` resolved an embedded workflow, else `path`); `createdAt` =
`Snapshot.CreatedAt`, `updatedAt` = `clock()`. `scope`/`target` are derived from the run's **own** `init` (a fork carries
its parent's `Workflow` by the §2 invariant and its `BaseSHA` by copy, so a lineage shares one target).

**Verdict map:** `fixed|clean` → PASS; `reviewed|stalled|failed|overflow|custom` → NEEDS_REVISION (plan §1.6 honoured —
`overflow` is a safety stop, and raising the budget or swapping the judge in a fork is its prescribed recovery); a
non-PASS outcome with `attemptNumber ≥ maxAttempts (3)` → **ESCALATED** with `escalationReason:
"attempt N of a fork lineage ended <outcome>"`. That is the same-target rule for FSM runs: three attempts on one lineage
without a PASS stops retries. `record.Escalated(root) func(ctx, runID) (bool, error)` reads `runs.jsonl` and reports
whether `runID`'s row is ESCALATED; `Fork` refuses such a parent (§2 step 0). `runchain.Resolve` is **not** involved
(no `--previous-run` exists for FSM runs). FSM rows have `scope: fsm-*`/`target.type: fsm` and never match a review
scope, so they cannot satisfy or block a review gate. `metareview status` reads `.metareview/runs/` via `Store.List()`,
not this file (spec 5 owns the section).

## 7. Errors
`ERR_RUN_ESCALATED{run}`, `ERR_CHECKPOINT_NOT_FOUND{from, at_iter?, reason?: unknown_state|terminal_state}`,
`ERR_WORKFLOW_CHANGED{expected, got}`, `ERR_WORKFLOW_TOO_LARGE`, `ERR_WORKFLOW_INCOMPATIBLE{reason: name|initial|state|cmd, state?}`,
`ERR_VAR_UNKNOWN`, `ERR_VAR_FROZEN{name, state}`, `ERR_CALIBRATION_PINNED`, `ERR_CMDS_NOT_ALLOWED{sha}`,
`ERR_WORKDIR_FOREIGN{reason}`, `ERR_TREE_NOT_AT_CHECKPOINT{expected, got}`, `ERR_COPY_INVALID{seq, reason}`,
`ERR_FORK_INCOMPLETE{run, parent}`, `ERR_MOCK_MISMATCH`, `ERR_DIFF_INCOMPATIBLE`, `ERR_EXPORT_DEST{reason}`,
`ERR_EXPORT_TOO_LARGE{bytes, max}`, `ERR_RUN_NOT_FOUND{detail}` (via `ValidateRunID`/store on every run argument of `Fork`,
`Diff`, `Export`), `ERR_AUDIT_TORN`, plus spec 2's passthroughs. All are `errs.Error` (store/fold errors are wrapped with
their `Code`, `Seq`/`Reason` in `Fields`).

## 8. Tests (100%; TDD; fakes from `machine`'s harness; `record`/`export` get their own harness with a `MemFS`)
| # | rows |
|---|---|
| F1 | fork from each non-terminal state of a happy sdlc run, plus a **two-iteration** parent asserting `ForkedAtSeq` literally for `--from discover` (nil → the iter-1 re-entry), `--at-iter 0` (→ seq 1), `--at-iter 1`, `--from adjudicate` (nil → iter 1), `--at-iter 2` → `ERR_CHECKPOINT_NOT_FOUND{from, at_iter: 2}`; copied prefix equals parent `[2..seq]` byte-for-byte modulo `Origin` (hashes match `EventsWithLines`); the child `InitData` asserted literally (`ParentRunID`, `Lineage`, `ForkedAtSeq`, `WorkDir`, `Vars`, `CmdsSHA256`, `WorkflowHash`, `Head == ch`, `Mock`); child folds; child sidecar bytes == parent's; `tree` (+ `fix_baseline` at fix) appended; `fork` event on parent; parent otherwise byte-identical; unknown `From`; terminal `From`; fork of a fork (`Lineage` grows, `Origin.RunID` = immediate parent); bad run id → `ERR_RUN_NOT_FOUND` |
| F2 | `ERR_NO_COMMIT` recovery: fail at fix, fork `--from fix`, negative control (no commit → `ERR_NO_COMMIT` again), commit → `commit_exists` passes, discover/adjudicate not re-run (fake executor call counts; copied `llm_call`s carry `Origin`); child `StartIndex` for `fix`'s successor is 0; commit-before-fork → `ERR_TREE_NOT_AT_CHECKPOINT` (the documented order) |
| F3 | judge swap: review-loop `--from adjudicate --var JUDGE=b` re-runs adjudicate only with model `b` (MockJudge `Calls()`); sdlc-loop at iter 0 ok, at iter 1 `ERR_VAR_FROZEN{JUDGE, adjudicate}` (first in `States` order although verify also ran); `--var REVIEWER` frozen after discover ran; a var referenced only by a convergence `cmd` that ran is frozen, one referenced by a `cmd` that never ran is not; calibration refusal; undeclared var |
| F4 | git preconditions: exact head for every state; `ERR_TREE_NOT_AT_CHECKPOINT` with hint; `--work-dir` worktree accepted, foreign dir refused, relative refused; child `TreeHash`/`Status` == its worktree's (the per-dir git fake reports values distinct from P's dir, so a rebaseline computed from P's `WorkDir` fails); mock parent: child `Open` re-verifies the scenario through `MockLoad`; enforcing child advances cleanly on a pristine checkout; fork after `fix` in the same iteration → `commit_exists` `ERR_GATE_INAPPLICABLE` |
| F5 | workflow change: refused without the flag; `ERR_WORKFLOW_TOO_LARGE`; invalid bytes → `ERR_WORKFLOW_INVALID`; incompatible (renamed workflow / changed initial / removed copied state / changed kind / removed `cmd`) → `ERR_WORKFLOW_INCOMPATIBLE{reason}` with **no child directory**; accepted with a tightened `Decode` → `ERR_COPY_INVALID{seq, reason}` with no child directory; dropped var reported; cmd set change requires the new sha (list printed); mock run forks as mock, registry mismatch refused; torn parent refused before any I/O; escalated parent → `ERR_RUN_ESCALATED` |
| F6 | `VerifyOrigin`: ok / parent_missing / parent_unreadable / version_unavailable / hash_mismatch / content_mismatch (edited copy with intact `Origin`); parent read once (counting store fake) |
| F7 | `Diff`: parent vs child rows (`CommonPrefixSeq == ForkedAtSeq` — computed over `Data`, so `Origin`/`Seq` do not shorten it), one-sided rows, alignment by input hash when indexes differ, one case **per kind** (`match`/`is_real`/`still_present` — a decoder that only reads `match` fails), `DecisionSame` without `ConfidenceSame`, `Same` ignores `reasoning`, nil vs `false` not `Same` and nil vs nil `DecisionSame`, errored call never `Same`, `SameWorkflow == false` after `AcceptWorkflowChange`, `TransRow.Same` on a diverging outcome, incompatible workflows, determinism (two calls equal) |
| F8 | `Export`: every row of the redaction table with seeded markers — including a var value inside a `file_hashes` key and inside an argv element (`--token=<v>`) — each marker absent from **every byte under `<Out>/`** (manifest included) and present with `IncludeVars` only where the table says so; positive assertion that the redacted snapshot keeps `Head`, `Iteration`, `Outcome`, `NodeOutputs` keys and the kept finding fields; per-line `redacted` arrays and `Manifest.Redacted`, unredacted lines byte-identical, original `seq`/`prev` kept, sidecars incl. `workflow.yaml` copied, torn files listed not copied, manifest fields literal, `MaxBytes` refusal writes nothing, symlinked component / non-empty dir refused, `O_EXCL` creation (fake `FS` records flags), `chain: "redacted"` |
| F9 | runs.jsonl record: golden JSON line per outcome decoded with `DisallowUnknownFields` into a struct listing exactly §6's keys (a missing or extra key fails), root (`previousRunId` absent, `attemptNumber` 1) and grandchild (3); also decodes with `runchain.ReadRuns`; verdict map for all 7 outcomes; ESCALATED at attempt 3 non-PASS with `escalationReason`, never at PASS; idempotent on a second call (resume path); `mock: true`; `workflowHash`/`workflowSource`; `Escalated` true/false/missing file; injected clock |
| F10 | refusal postconditions + failure sweeps: after **every** refusal in steps 0–7 P is byte-identical and no child directory exists; the harness's failure seams (`Sidecar.Write`, `run.Create`, child/parent `Lock`, each `Append` of a copied/`tree`/`fix_baseline`/`fork` event, `Git.Head/CommonDir/WorkTree/Status`, `Escalated`, `MockLoad`, export `FS.MkdirAll/Create`, recorder read/write) each pass their error through unchanged; ordinal continuity (`RunnerDeps.CmdCalls` on the child counts copied `cmd_call`s) |
| F11 | incomplete fork: crafted child with `Seq == ForkedAtSeq` → `Open` `ERR_FORK_INCOMPLETE`, `List` error summary; crash injection after `Create` (failing store) leaves the child incomplete and P without `fork` |

## 9. Ledger (deviations from plan r3 / the design, and handoffs)
| item | decision |
|---|---|
| plan §1.9 "row appended once, at terminal" (C27) | honoured in r3; r2's `forked` row removed |
| plan §1.9 `maxAttempts: 0` | 3, so that ESCALATED is producible; `attemptNumber = len(Lineage)+1` |
| plan §1.6 `overflow → NEEDS_REVISION` | honoured; ESCALATED = third non-PASS attempt of a lineage (this spec's "ESCALATED mapping") |
| plan §1.7 FEA-N4 ancestor rule for agent-edit | dropped: `HEAD == ch` everywhere; "fork, then commit" |
| plan §1.7.4 freeze on `cmd:` references (FEA-N3/INT-10) | restored: convergence/overflow `cmd`s that ran freeze their vars |
| plan §1.7.5 `ERR_AUDIT_INVALID` on copy | `ERR_COPY_INVALID{seq, reason}` from an in-memory fold, before any I/O |
| plan §3.5 export files / `llm_call.raw` dropped | files: manifest, `audit.redacted.jsonl`, `snapshot.json` (redacted), `workflow.yaml`, sidecars; `llm_call.error` reduced to its class (raw text lives only there, spec 4 r5) |
| plan §3.5 `Diff` `verdict` field | `Decision *bool` + `Confidence` via `kind.Decision`; `DecisionSame`/`ConfidenceSame` |
| spec 2 §8 `tree` baseline when `TreeHash == ""` | every fork writes its own `tree`; spec 2's branch stays as defence-in-depth for hand-built logs |
| run spec §7 "rebaseline on agent-edit forks" | every fork rebaselines `tree`; `fix_baseline` only for agent-edit `From` |
| run spec §12 row 3 redaction carriers (`llm_call.Verdict`, `record.data`, `node_output`) | verdict kept, `record.data` kept (documented unredacted), `node_output` trimmed per kind |
| `machine` charter | `RecordTerminal`/`Export` moved to `record`/`export`; `Deps.Escalated` added |
| run spec §11 (handoff) | `RunStore.TornFiles(runID)`; `InitData.WorkflowSource`; `ERR_FORK_INCOMPLETE` on `Open`; `RunSummary.Error` for incomplete forks |
| spec 5 (handoff) | `Deps.Escalated`/`Deps.Terminal` wiring, `DecisionFunc = kind.Decision`, "fork then commit" recipe, `--include-vars` never committed, `record.data` unredacted, manual deletion of an incomplete fork, `mock` rows never satisfy gates, `status` reads `Store.List()` |
| SEC-13/SEC-7/SEC-22 | closed by §5 (snapshot redacted; var values replaced everywhere argv appears) |
| ARC-21 | target id `<workflow>@<base_sha[:12]>`; `status` section is spec 5's (from `Store.List()`) |
