# metareview 0.9.0 — spec 3: fork/resume, diff, export, and the runs.jsonl record

> **Status:** DRAFT r5 (2026-08-27; r4 + the attempt-3 in-attempt fixes listed at the end of this header). Third of the five split artifacts (ownership ledger: run spec §12 row 3, plus
> CMP-15/ARC-21 reassigned from spec 2). Owns plan r3 §1.7's fork half, `Origin` verification incl. version, `Lineage`,
> the var-freeze rule, git preconditions + `tree`→`fix_baseline`, `--work-dir` validation, re-validation of copied
> events, `fsm diff`, `fsm export` (redaction, sidecars, manifest), the `fsmRunRecord` in `.metareview/runs.jsonl`
> with the verdict map, and the `ESCALATED` mapping. Builds on the implemented `run`, `machine` (r4), `kind` (r5).
>
> **r4 changes** (attempt-2 review of r3): this spec now **owns** the small sibling-package amendments it needs, listed
> in §1 and mirrored into the owners' ledgers (`InitData.WorkflowSource`, `RunStore.TornFiles`/`MaxEvents`,
> `run.Counted`, `ERR_FORK_INCOMPLETE` in `machine.load`/`run.summarize`, `kind.Decision`); escalation is computed from
> P's own hash-chained snapshot (`Deps.Escalated` dropped) and stated honestly as a **lineage-depth** rule (ancestor
> forks and fresh `init`s are human resets — ledgered against the review gates' per-target rule); runs.jsonl `status`
> uses the existing vocabulary (`passed|needs-revision|escalated`); golden comments are declared not sensitive and the
> redaction table is corrected to the real field names (`Finding` has no `id`/`desc`), enumerates every event/field, and
> fails closed for unknown kinds; `converge.reason` is emptied only for `cmd` atoms; `snapshot.json` mirror lists every
> field; `ERR_FORK_INCOMPLETE` covers the agent-edit crash window and is checked before the sidecar read; the child's
> lock is released before step 10; the fork's own `tree`/`fix_baseline` carry spec 2's stamps; `ch` is the checkpoint's
> head (discriminating fixture); calibration parents strip pinned names before `Resolve`; the freeze rule is evaluated on
> P's resolved workflow only; substitution order and path rules defined; `FS.OpenFile(path, flag, perm)`; `--include-vars`
> refuses the default `Out`; `Manifest.ChainHead`/`Records`; `Diff` compares the kind's **effective** decision;
> `CommonPrefixSeq` over `(Type, Node, Data)`; test rows for every gap named by the Testing lens.
>
> **r5 (attempt-3 fixes, in-attempt):** `record` owns its append (torn tail preserved to `runs.jsonl.torn-<ts>` and
> truncated to the last newline under an advisory lock before any write — never glued; a pre-existing row for the id
> whose `headSha`/`workflowHash` differ is an error, not a silent skip); `Snapshot.WorkflowSource` (fold copies it
> from `init`) so the recorder can see it; `machine.Decision{Raw, Effective}` lives in `machine` (`kind` imports
> `machine`, so the type cannot live in `kind`); `Effective == Raw` for `match` (relative argmax, no per-call threshold);
> the copied-`node_output` re-`Decode` is an explicit machine-side pass (the fold has no kinds); `rejected[].desc` is
> kept; `CommonPrefixSeq` tuple includes `Origin` presence; `Copied` defined; path root = `export.Deps.RepoRoot`,
> `$HOME` via `Deps.Home`; the exported `init` payload carries `exported: true` so a copied-back bundle fails closed even
> for an init-only run; `cmd` output shape corrected, `cmd_call.error` listed, the dead `llm_call.error` rule dropped;
> additive-v1-field exception ledgered; `ERR_RUN_ESCALATED` is for a human (spec 5 agent-prompt sentence) and the
> non-terminal attempt-3 case is named; the review-only row keys are dropped explicitly; ledger rows for
> `detail_summary`, the narrowed `cmd:` freeze, `--from <initial>` unset, the added `Diff` fields; test rows for every
> gap the attempt-3 lenses named.
>
> **Scope rule:** what `Fork`, `Diff`, `Export`, and the runs-record *produce*. The CLI shapes are spec 5's.

---

## 1. Packages and owned amendments
```
internal/fsm/machine   fork.go (Fork, VerifyOrigin), diff.go (DiffRuns); load: ERR_FORK_INCOMPLETE check
internal/fsm/record    record.go (Terminal, Exists)                — imports machine (View), internal/runchain (Record type); its own writer, no internal/state
internal/fsm/export    export.go (Export, redaction)               — imports run, machine (types), workflow (parse sidecar)
internal/fsm/kind      decision.go (Decision)                      — spec 4's package; this spec owns the addition (returns machine.Decision)
internal/fsm/run       InitData.WorkflowSource; RunStore.TornFiles/MaxEvents; Counted; summarize incomplete-fork rule
```
`machine` keeps its charter (it decides; effects sit behind `Deps`). No new `Deps` field: escalation is read from P's
own snapshot (§6). The CLI wires `Deps.Terminal = record.Terminal(root, clock)`. `tests/coverage.sh` gates every
`internal/fsm/*` package at exactly 100%.

**Owned amendments** (each mirrored as a row in the owner's ledger — run spec §11, spec 2 §8, spec 4 §9):
- `run.InitData.WorkflowSource string` (`json:"workflow_source,omitempty"`, `withinCaps` ≤ `MaxShort`, vocabulary
  `embedded|path|""`; `""` = written before the field existed and is reported as `"unknown"` by the recorder) and
  `run.Snapshot.WorkflowSource` (the fold copies it from `init`; `Clone` copies it) so `View`-only consumers see it.
  Accepted exception to run spec §5.4: an additive v1 `init` field before the first tagged release that contains
  `internal/fsm/run` (v0.8.2 has none); after that release, any new persisted field bumps `SchemaVersion`;
  `machine.InitOptions.WorkflowSource` stamps it (spec 5 sets `embedded` when `--workflow <name>` resolved through
  `Deps.Workflows`, else `path`).
- `run.RunStore` gains `TornFiles(runID) ([]TornFile{Name, SHA256 string; Bytes int64}, error)` (the `audit.torn-`
  prefix shared with `List`) and `MaxEvents() int`; `run.Counted(eventType string) bool` exports `countedType`.
- `run.summarize` and `machine.load` apply the incomplete-fork rule (§2 step 8) — in `load` immediately after
  `FoldFull`, **before** `Sidecar.Read`, so a child missing its sidecar reports `ERR_FORK_INCOMPLETE`, not `ERR_SIDECAR`.
- `machine.Decision{Raw, Effective *bool}` (declared in `machine/types.go` — `kind` imports `machine`, so the type
  cannot live in `kind`) and `kind.Decision(kind string, verdict json.RawMessage) machine.Decision`: `Raw` = the
  kind's decision field (`match` for `judge.KindMatch`, `is_real` for `KindAdjudicate`, `still_present` for
  `KindStillPresent`; nil when the verdict is `null`, the bool is absent/`null`, or the kind is unknown); `Effective` =
  the kind's per-call rule as `kind` applies it: `is_real ∧ confidence ≥ kind.AdjudicateThreshold (0.7)` for
  adjudicate; `still_present` for still-present; **`Raw`** for match (the match rule is a relative argmax across
  candidates — there is no per-call threshold, so a single verdict has no effective decision beyond its own bool); nil
  whenever `Raw` is nil.

## 2. `Fork`
```go
type ForkOptions struct { From run.State; AtIter *int; Vars map[string]string; WorkDir string; AcceptWorkflowChange bool; AllowCustomCmds string; WorkflowBytes []byte /* only with AcceptWorkflowChange */ }
type ForkResult struct { ChildRunID string; ForkedAtSeq int64; Copied int /* parent events copied = seq-1; 0 on a restart */; CmdsSHA256 string; DroppedVars []string }
func (m *Machine) Fork(ctx, ForkOptions) (*Machine, ForkResult, error)
```
Algorithm. P is `m` (opened via `load`). **P's lock is held for steps 0–9**; the checkpoint, the copied lines and the
`fork` stamps all come from one `EventsWithLines` read taken under that lock.
0. **Preconditions.** `ctx.Err()` is returned as-is at every step. P torn → `ERR_AUDIT_TORN` (as `Advance`/`Record`).
   P escalated — `attempt(P) ≥ MaxAttempts (3)` ∧ P terminal ∧ `verdict(P.Outcome) != PASS`, all from P's snapshot
   (`attempt = len(Lineage)+1`) — → `ERR_RUN_ESCALATED{run, attempt}` (a human must narrow, split, or redesign).
1. **Checkpoint.** `From` must be a state of P's workflow (`ERR_CHECKPOINT_NOT_FOUND{from, reason: unknown_state}`) and
   not terminal (`reason: terminal_state`). Search first: `seq` ← the `transition` event with `To == From` and, if
   `AtIter` set, `Iter == *AtIter`; the latest such transition when `AtIter` is nil. Only if none matches: `From ==
   P.Initial` and (`AtIter` nil or `*AtIter == 0`) → `seq = 1` (a restart: nothing is copied); otherwise
   `ERR_CHECKPOINT_NOT_FOUND{from, at_iter}`. (So `--from adjudicate --at-iter 0` is the iter-0 transition, never a
   restart; on a looping workflow `--from <initial>` without `--at-iter` is the latest re-entry and `--at-iter 0` the
   restart.) A terminal P is allowed (judge swap on a finished run).
2. **Workflow bytes.** `raw ← P's sidecar` (verified by `load`), `source ← P.WorkflowSource`. If `WorkflowBytes != nil`:
   `AcceptWorkflowChange` must be set (`ERR_WORKFLOW_CHANGED{expected: P.WorkflowHash, got: sha256(WorkflowBytes)}`),
   `len ≤ MaxWorkflowBytes` (`ERR_WORKFLOW_TOO_LARGE`), `raw ← WorkflowBytes`, `source ← "path"`.
   `w ← workflow.Parse(raw, workflow.Options{Kinds: Kinds.Info()})` (`ERR_WORKFLOW_INVALID` passes through).
   `WorkflowHash ← sha256(raw)`. **Compatibility invariant** (only when the bytes changed; violation →
   `ERR_WORKFLOW_INCOMPATIBLE{reason, state?}`): `w.Name == P.Workflow` (`reason: name`); `w.Initial == P's initial`
   (`initial`); every state stamped on a copied event `2..seq` and `From` itself exists in `w` (same state key — `Node.Name` is always the state key) with the same
   **kind** and **exec** (`state`, with the state); every `cmd` name referenced by a copied
   `cmd_call`/`overflow_handler` is still declared (`cmd`). Everything else (gates, convergence, models, effort,
   params, added or not-yet-run states, unreferenced cmds) may differ — that is what the flag consents to.
3. **Vars.** P's stored vars (resolved values — a changed workflow's new defaults therefore never apply in a fork;
   ledger) filtered to `w`'s declared set (`DroppedVars` = the removed names) overlaid by `Vars` (undeclared →
   `ERR_VAR_UNKNOWN`). For each overridden `K`: **frozen** if `K ∈ Pw.VarsReferencedBy(s)` for any state `s` whose node
   ∈ `NodesRun` of `Fold(P[1..seq])`, or `K ∈ Pw.CmdRefs[c]` for any `cmd` `c` named by a copied `cmd_call` /
   `overflow_handler` — where `Pw` is P's **resolved** workflow (the one that ran; its `Refs`/`CmdRefs` survive
   `Resolve`) → `ERR_VAR_FROZEN{name, state}` with `state` = the first frozen state in `Pw.States` order (a convergence
   `cmd` reports the state whose convergence ran it, i.e. the `converge` event's `State` stamp; the overflow handler
   reports the `overflow_handler` event's `State` stamp). Calibration parent: remove `JUDGE`/`JUDGE_EFFORT` from
   `effective` before `Resolve` (it re-pins them); an override of either → `ERR_CALIBRATION_PINNED`. Then
   `Resolve(effective, P.Calibration)`.
4. **Work dir.** `WorkDir` (default P's) must be absolute (`ERR_WORKDIR_FOREIGN{reason: relative}`) and
   `Git(WorkDir).CommonDir == Git(P.RepoRoot).CommonDir` (`ERR_WORKDIR_FOREIGN{reason: other_repo}`).
5. **Commands.** `ResolveCmds(w, WorkDir, LookPath, FileHash)`; if `len > 0 && sha != P.CmdsSHA256` → require
   `AllowCustomCmds == sha` (`ERR_CMDS_NOT_ALLOWED{sha}` with the printed list and `Fields["cmds_json"]`, exactly as
   `Init`).
6. **Git precondition (every state).** `ch` = the **checkpoint's** head — `TransitionData.Head` of the checkpoint
   transition, or `InitData.Head` for `seq = 1` — never P's latest head. `HEAD(WorkDir) == ch`, else
   `ERR_TREE_NOT_AT_CHECKPOINT{expected: ch, got: HEAD}` with `Detail` = "fork first, then commit: `git worktree add
   <dir> <ch>`, fork with `--work-dir <dir>`, then commit (or `git cherry-pick`) there". The fork is the node's starting
   line; a commit made before the fork would sit below the child's `fix_baseline` and be invisible to `commit_exists`
   (spec 5's `resume_hint` and recovery recipe say the same).
7. **Build the child in memory.** Child id `run.RunID(w.Name, Clock().Time)`; `init` = P's `InitData` with `RunID`,
   `CreatedAt`, `ParentRunID: P`, `Lineage: P.Lineage + P`, `ForkedAtSeq: seq`, `WorkDir`, `Vars` (effective),
   `AllowedCmds`/`CmdsSHA256` (step 5), `WorkflowHash`, `WorkflowSource: source` (step 2), `Head: ch`, `Mock` (P's — a
   mock run forks as a mock run; the registry must agree, `ERR_MOCK_MISMATCH`). Then P's events `2..seq` verbatim (same
   `Data` bytes, `At`, `State`, `Iter`, `Node`, `Mock`) each with `Origin{RunID: P, Seq, Version: P.Log.Version, Hash:
   LineHash(P line)}`; then `tree{Head: ch, TreeHash: TreeHash(ch, WorkTree), Status}`; then, if the node at `From` is
   `agent-edit`, `fix_baseline{Head: ch}` — both stamped per spec 2 §5.4 (`At: Clock()`, `State`/`Iter` of the
   checkpoint, `Mock: P.Mock != ""`). Validate **before any I/O**, three passes: (a) an explicit machine-side re-`Decode` of every copied `node_output`
   with `Kinds.Kind(w.NodeFor(State).Kind).Decode(output)` (the fold has no kinds) → `ERR_COPY_INVALID{seq, reason:
   decode}`; (b) `run.FoldFull` over the sequence with provisional `Seq`s (a `*FoldError` at seq `n` →
   `ERR_COPY_INVALID{seq: n, reason}` — `output_hash` mismatch, un-sanctioned `cmd`, mock stamp, provenance); (c)
   `count(run.Counted) ≤ Store.MaxEvents()` (→ `ERR_COPY_INVALID{seq, reason: max_events}`); nothing has been created. Copied `cmd_call`s count toward the child's ordinals (they are in its log).
8. **Write the child.** `run.Create(init)`; `Sidecar.Write(child, "workflow.yaml", raw)`; under the child's lock append
   the remaining events in order; **release the child's lock**. A crash inside this step leaves an **incomplete fork**:
   `ParentRunID != "" ∧ (Seq ≤ ForkedAtSeq ∨ (Seq == ForkedAtSeq+1 ∧ StateKind == agent-edit))` — no rebaseline `tree`,
   or a `tree` without the `fix_baseline` the agent-edit checkpoint requires. `machine.load` (after `FoldFull`, before the
   sidecar read) returns `ERR_FORK_INCOMPLETE{run, parent}`; `run.summarize` sets `RunSummary.Error` so `List` reports it;
   spec 5 documents manual deletion. `init.parent_run_id`/`lineage` are authoritative; P's `fork` event (step 9) is advisory.
9. **Parent.** Append `fork{ChildRunID, AtSeq: seq}` to P (still under P's lock). No runs.jsonl row is written here (§6).
10. Return the child machine (`Open`ed — P's lock released, the child's re-taken by `load`): its first `Advance` re-runs
    the node at `From` from `StartIndex` (no copied `llm_call` for that key exists — the prefix ends at the transition
    **into** `From`) and evaluates `From`'s gates. A fork from a state after `fix` in the same iteration has
    `FixEntryHead == ""` (copied transitions never set it); `commit_exists` then fails closed with
    `ERR_GATE_INAPPLICABLE` — only `fix → verify` uses it in the shipped workflows.

The judge-swap limitation (spec 5 docs): on `sdlc-loop` `--var JUDGE=…` is accepted only at `--at-iter 0` (adjudicate and
verify both reference `$JUDGE` and ran at every later iteration).

## 3. `VerifyOrigin`
`VerifyOrigin(ctx, store, child run.Log) []OriginCheck` — reads the parent's `EventsWithLines` **once** (indexed by seq)
and, for each `Origin`-bearing child event, reports `{Seq, OK bool, Reason}` with `Reason` = the first failing check in
this order: `parent_missing` (`ERR_RUN_NOT_FOUND`) | `parent_unreadable` (any other store error) | `version_unavailable`
(`Origin.Version != parent Log.Version`) | `hash_mismatch` (`LineHash`) | `content_mismatch` (`Data`, `At`, `State`,
`Iter`, `Node`, `Mock` differ) | `ok`. Used by `fsm diff` and `fsm export` (reported, never fatal).

## 4. `Diff`
```go
type Decision struct { Raw, Effective *bool }   // declared in machine; produced by kind.Decision
func DiffRuns(a, b run.Log, decide func(kind string, verdict json.RawMessage) Decision) (Report, error)   // named DiffRuns: machine.Diff is the git-diff input type; the CLI passes kind.Decision
type Report struct { A, B string; SameWorkflow bool /* WorkflowHash equal */; CommonPrefixSeq int64; Outcomes [2]run.Outcome; Calls []CallRow; Transitions []TransRow }
type CallRow struct { Node string; Iter int; Kind, InputHash string; A, B *CallSide; RawSame, DecisionSame, ConfidenceSame, Same bool }
type CallSide struct { Index int; Model, Effort string; Raw, Effective *bool; Confidence float64; Error string }
type TransRow struct { SeqA, SeqB int64; To run.State; Gate string; Outcome run.Outcome; Same bool }
```
Judge rows align by `(node, iter, kind, input_hash)` — not by ordinal, because `match-then-adjudicate` indexes depend on
the seen set and a different judge changes it; each side reports its own `Index`. "Both present" below means both
`CallSide`s exist. `RawSame` = both present ∧ `Raw` equal (nil == nil); `DecisionSame` = both present ∧ `Effective`
equal (nil == nil) — this is the §17 signal (`true/0.9` vs `true/0.6` on adjudicate is a flip); `ConfidenceSame` = both
present ∧ `Confidence` equal; `Same = DecisionSame ∧ ConfidenceSame ∧ both Error == ""` (errored calls are never `Same`,
even with identical `Error`). `reasoning` never participates. Transitions align by ordinal; `TransRow.Same` = both
present ∧ `To`, `Gate`, `Outcome` equal. Different `Workflow` names → `ERR_DIFF_INCOMPATIBLE`. `CommonPrefixSeq` = the
largest `n` such that events `2..n` of both logs have identical `(Type, Node, At, Data)` — `Origin`, `Seq` and `Prev`
are ignored (copies carry `Origin` and re-chained `Prev`; a root parent's events carry neither) — so a parent/child
pair yields exactly `ForkedAtSeq`: the child's own `tree` has a fresh `Clock()` `At` and never extends it; `1` when
they diverge at seq 2.
Output is deterministic (rows sorted by `(node, iter, kind, input_hash)`; transitions by ordinal). Spec 4 follow-up
(ledger): a `Subject` on `llm_call` (bug id / golden index) would let a row name *which* finding flipped; until then the
reader joins `input_hash` to `node_output` ids.

## 5. `Export`
```go
type Deps struct { Store run.RunStore; Sidecar machine.Sidecar; Kinds machine.Registry; FS FS; Clock machine.Clock; RepoRoot string /* the path root for relativisation */; Home string /* the `~` prefix; "" disables */ }
type FS interface { MkdirAll(dir string, perm os.FileMode) error; OpenFile(path string, flag int, perm os.FileMode) (io.WriteCloser, error); Lstat(path string) (os.FileInfo, error); ReadDir(dir string) ([]os.DirEntry, error) }
type ExportOptions struct { Out string; IncludeVars bool; MaxBytes int64 /* default 5 MB */ }
func Export(ctx, deps Deps, runID string, opts ExportOptions) (Manifest, error)
type Manifest struct { SchemaVersion int /* 1 */; RunID, Workflow, WorkflowHash, WorkflowSource string; ExportedAt run.Time; SourceHead string /* snapshot Head */; ChainHead string /* source Log.Head */; IncludeVars bool; Events int; Redacted []int64 /* seqs carrying a marker */; Records []int64 /* seqs of record events (unredacted) */; Sidecars []string; TornFiles []run.TornFile /* listed, never copied */; OriginChecks []OriginCheck; Bytes int64 /* sum of all written files */; Chain string /* "redacted" */ }
```
**Contract.** The export is *evidence for a reader*, not a foldable log: redacted lines keep their original `seq` and
`prev` bytes (nothing is re-chained), gain a top-level `"redacted": ["field", …]` array, and therefore no longer
hash-verify; unredacted lines are byte-identical to the source and verify pairwise (`LineHash(line n) == line n+1.prev`);
`ChainHead` anchors the tail. Redacted payloads carry fields no `run` type has (`detail_summary`, `error_truncated`,
`file_hashes` as a list) — readers treat `data` as generic JSON. `Chain` is always `"redacted"`.

**Files.** `<Out>/{manifest.json, audit.redacted.jsonl, snapshot.json, workflow.yaml}` plus a copy of every sidecar in
`Sidecar.List`. `audit.torn-*.bin` are **not** copied (raw, never-redacted bytes): `Store.TornFiles(runID)` lists them.

**Kinds.** The node→kind map comes from parsing the exported `workflow.yaml` with `Kinds.Info()` (the same parse `load`
does); per-kind rules are enumerated below and an unknown kind **fails closed** (`output` replaced by `{}`, field
`output` in the marker).

**Redaction table** (every event type in `run.EventTypes`; a field not named is kept):
| event / field | rule |
|---|---|
| `init.vars` values | `sha256:<hex>` unless `IncludeVars` (a confirmation oracle for low-entropy values; what it hides is CLI-supplied values that reach only cmd argv — `model`/`effort` params are cleartext in `llm_call`/`workflow.yaml` anyway) |
| `init.allowed_cmds[].argv`, `cmd_call.argv`, `overflow_handler.argv`, `init.allowed_cmds[].file_hashes` keys | unless `IncludeVars`: every occurrence of a var value with `len ≥ 4` replaced by `$<NAME>`, longest value first, single pass per element (values shorter than 4 bytes are exported verbatim — stated); then **paths**: inside `Deps.RepoRoot` → relative to it; else verbatim except a `Deps.Home` prefix → `~`. `file_hashes` becomes a sorted list `[{path, sha256}]` (collisions after substitution keep both entries) |
| `init.repo_root`, `init.work_dir` | relative to `Deps.RepoRoot` when inside it, else `<outside>` (`init.mock` is already repo-relative) |
| `init` (always) | gains `exported: true` — `InitData` decodes with `DisallowUnknownFields`, so a copied-back bundle fails closed at seq 1 even when nothing else was redacted |
| `init.goldens[].comment` | **kept** — golden comments are the calibration answer key by design and appear as `Bug.Desc` of matched bugs; they are not treated as secrets (ledger) |
| `gate.error.detail` | `commit_exists`: replaced by `detail_summary {files: [paths from the porcelain block], truncated: detail_truncated}`; every other gate: emptied; `detail_truncated` kept |
| `cmd_call`/`overflow_handler` `stdout`/`stderr` | emptied, `*_truncated: true` |
| `converge.reason` | emptied when the stopping atom is a `cmd` atom (`atom` starts with `cmd:`); built-in atom reasons (iteration/budget/no-progress numbers) kept |
| `warn.detail` | emptied |
| `llm_call.verdict` | kept (typed JSON incl. `reasoning` — the §17 audit needs it) |
| `llm_call.error`, `cmd_call.error`, `overflow_handler.error` | kept — all three are bare error codes already (`kind.go` stores `errs.Code`; `cmdexec` likewise); raw model text lives only in the source log's `llm_call.verdict`/`ParseError` path per spec 4 |
| `node_output.output` — `review-lenses` | kept whole (`run.Finding` = `issue_text, file, line, severity, category, source`; `issue_text` is the finding) |
| `node_output.output` — `match-then-adjudicate` | kept whole (`rejected[].desc` equals a `review-lenses` `issue_text` that is exported anyway; emptying it hid nothing and cost the §17 reader the false-negative cell — spec 4's handoff is retired, ledger) |
| `node_output.output` — `agent-edit`, `still-present`, `cmd` | kept (`{commit}`; `{status…}`; a `run.Delta` for `cmd` — the same finding/bug text class as `review-lenses`, kept under the same policy) |
| `node_output.output` — unknown kind | `{}` (fail closed) |
| `delta_applied` | kept |
| `record.data` | kept verbatim (host-supplied, durable, **unredacted**; seqs listed in `Manifest.Records`; spec 5 documents it) |
| `tree.status` | reduced to the sorted list of paths (`[]string`) |
| `init` other fields, `transition`, `tokens`, `fork`, `fix_baseline`, `needs_input`, `gate.name/passed`, `converge.atom/class/stop`, `llm_call` other fields | kept |
| `workflow.yaml` | copied verbatim (var **defaults** are workflow text; secrets travel as declared `env` names, never as vars) |

`snapshot.json` = `Redact(FoldFull(source log))` with the same rules applied to every `run.Snapshot` field: `Vars`,
`AllowedCmds` (argv + `FileHashes`), `RepoRoot`/`WorkDir`, `TreeStatus` (path list), `LastError.Detail` (emptied),
`StopReason` (emptied when it names a `cmd` atom), `NodeOutputs` (per kind as above), `Goldens`/`Confirmed`/`AllFound`/
`Findings` kept; plus a top-level `"redacted": true` marker and `SchemaVersion` kept.

**Destination safety.** `Out` default = `<RepoRoot>/docs/metareview/fsm/<run-id>/`. With `IncludeVars` the default is
refused (`ERR_EXPORT_DEST{reason: include_vars_default}` — cleartext var values never land in the committed tree by
default; the manifest records `include_vars: true`). Every path component from the filesystem root down to and including
`Out` is `Lstat`-checked (symlink → `ERR_EXPORT_DEST{reason: symlink}` — so `/tmp` on macOS needs `/private/tmp`;
`ENOENT` on trailing not-yet-existing components is tolerated — they are created by `MkdirAll`);
an existing non-empty `Out` → `ERR_EXPORT_DEST{reason: not_empty}`; directories `MkdirAll(…, 0700)`; files
`OpenFile(path, O_WRONLY|O_CREATE|O_EXCL|O_NOFOLLOW, 0600)` (the fake `FS` asserts flag and perm literally). The
`Lstat`→`OpenFile` window on intermediate directories is an accepted local-single-user TOCTOU (stated). The whole bundle
is built in memory and `MaxBytes` is checked **before the first write** (`ERR_EXPORT_TOO_LARGE{bytes, max}` writes
nothing). Exports are one-way: `audit.redacted.jsonl` cannot be copied back — seq 1 carries `exported: true` (refused by the payload decoder) and, for any run that advanced, seq 2's `prev` no longer matches. A reflection test pins that every `run.Snapshot` field name appears in the snapshot redaction map (fail closed on new fields).

## 6. `.metareview/runs.jsonl` record (C27)
`record.Terminal(repoRoot string, clock func() run.Time) func(ctx, machine.View) error` — the `Deps.Terminal`
implementation, called by the machine on every terminal advance (incl. `failed` and the resume path). **One row per run,
appended at terminal only** (C27). Idempotent **by `id`** (never by target/scope): if a row with this `id` exists, nothing
is appended. `record.Exists(root, id) (bool, error)` is the same lookup (spec 5 uses it for `init --run-id`). **Reading and writing** are `record`'s own (it does not use `state.AppendJSONL`): under an advisory `flock` on
`runs.jsonl`, read; a final line without a trailing `\n` that **decodes** as a JSON object is a complete row (0.8.x
readers accept it — editors drop final newlines): a `\n` is written first and nothing is moved; an **undecodable**
unterminated fragment (a torn append) is **preserved** to `.metareview/runs/.torn/runs.jsonl-<unixnano>` (the
self-ignored `.torn/` directory) and the file is truncated to its last `\n` (mirroring `RepairTail`) before anything is
appended; blank lines are skipped exactly as `runchain.ReadRuns` skips them; any other undecodable
line → `ERR_RUNS_JSONL{line, reason: malformed}` and nothing is written; a pre-existing row with this `id` whose
`headSha` or `workflowHash` differ from the view → `ERR_RUNS_JSONL{line, reason: id_conflict}` (a planted row cannot
silently suppress the real one); then the row + `\n` is appended with `O_APPEND` and fsynced. A `Terminal` error leaves
the transition durable and the resume path retries; `Exists` uses the same reader (read-only, no repair). The row drops
the review-only keys of the existing writers (`reviewers`, `findingIds`, `sourceRefs`, `gateEffect`, `*FindingCount`) —
stated so the golden pins it.
Shape follows the `taskdone`/`prready` writers (`internal/taskdone/review.go:40-60`) and is decoded by
`runchain.ReadRuns` unchanged; new keys are additive (`schemaVersion` stays 1):
```json
{"schemaVersion":1,"id":"<run>","scope":"fsm-<workflow>","target":{"type":"fsm","id":"<workflow>@<base_sha[:12]>"},
 "status":"passed|needs-revision|escalated","verdict":"PASS|NEEDS_REVISION|ESCALATED","executionMode":"fsm",
 "previousRunId":"<parent, omitted for roots>","attemptNumber":N,"maxAttempts":3,"baseSha":"…","headSha":"<Snapshot.Head>",
 "createdAt":"<RFC3339Nano>","updatedAt":"<RFC3339Nano>","repoRoot":"…","contextPackPath":"","reviewLogPath":"",
 "mock":false,"outcome":"fixed","fsmRunDir":".metareview/runs/<run>/","workflowHash":"<sha256>",
 "workflowSource":"embedded|path|unknown","escalationReason":""}
```
`status` uses the existing vocabulary (`passed`↔PASS, `needs-revision`↔NEEDS_REVISION, `escalated`↔ESCALATED).
`attemptNumber = len(Lineage) + 1` (root = 1; from the run's own `init`, no parent row needed); `previousRunId` =
`ParentRunID` — **informational** for FSM rows (the parent may have no row yet; nothing walks FSM chains through
`runchain`); `mock` = `Snapshot.Mock != "" || MockTainted`; `target.id` slices `base_sha` to its first 12 characters or
the whole value when shorter (never panics); `createdAt` = `Snapshot.CreatedAt`, `updatedAt` = `clock()`. `scope`/`target`
derive from the run's **own** `init` (a fork shares its parent's `Workflow` by the §2 invariant and its `BaseSHA` by copy).

**Verdict map:** `fixed|clean` → PASS; `reviewed|stalled|failed|overflow|custom` → NEEDS_REVISION (plan §1.6 honoured);
a non-PASS outcome with `attemptNumber ≥ maxAttempts (3)` → **ESCALATED** with `escalationReason: "attempt N of a fork
lineage ended <outcome>"`. **This is a lineage-depth rule, not the review gates' per-target rule** (ledger): it stops the
third non-PASS attempt on one branch (`Fork` §2 step 0 refuses the escalated leaf using its own chained snapshot — a
deleted or edited runs.jsonl row cannot bypass it); forking an earlier ancestor, a fresh `fsm init` on the same
`<workflow>@<base>`, or forking a **non-terminal** attempt-3 parent (abandoned or parked at `needs_input`; its
children are refused once they end non-PASS) is a deliberate **human reset** and is allowed — and `ERR_RUN_ESCALATED`
is for a human: spec 5's agent prompt says the agent must not fork an ancestor or re-`init` on its own after it (same
family as the consent rules). Rationale: FSM runs on one base are legitimately
re-run with different vars/workflows (the §17 judge experiments), and `--var`/workflow changes are not "the same target"
in the sense the review gates mean; the design's per-target block would forbid exactly the recovery it prescribes for
`overflow`. `runchain.Resolve` is **not** involved (no `--previous-run` for FSM runs). FSM rows have `scope: fsm-*` /
`target.type: fsm` and never match a review scope, so they cannot satisfy or block a review gate; a `mock: true` row never
satisfies any gate (spec 5 states it). `metareview status` reads `.metareview/runs/` via `Store.List()`, not this file.

## 7. Errors
`ERR_RUN_ESCALATED{run, attempt}`, `ERR_CHECKPOINT_NOT_FOUND{from, at_iter?, reason?: unknown_state|terminal_state}`,
`ERR_WORKFLOW_CHANGED{expected, got}`, `ERR_WORKFLOW_TOO_LARGE`, `ERR_WORKFLOW_INCOMPATIBLE{reason: name|initial|state|cmd, state?}`,
`ERR_VAR_UNKNOWN`, `ERR_VAR_FROZEN{name, state}`, `ERR_CALIBRATION_PINNED`, `ERR_CMDS_NOT_ALLOWED{sha}`,
`ERR_WORKDIR_FOREIGN{reason}`, `ERR_TREE_NOT_AT_CHECKPOINT{expected, got}`, `ERR_COPY_INVALID{seq, reason}`,
`ERR_FORK_INCOMPLETE{run, parent}`, `ERR_MOCK_MISMATCH`, `ERR_DIFF_INCOMPATIBLE`, `ERR_EXPORT_DEST{reason:
symlink|not_empty|include_vars_default}`, `ERR_EXPORT_TOO_LARGE{bytes, max}`, `ERR_RUNS_JSONL{line, reason: malformed|id_conflict}`,
`ERR_RUN_NOT_FOUND{detail}` (via `ValidateRunID`/store on every run argument of `Fork`, `Diff`, `Export`),
`ERR_AUDIT_TORN`, plus spec 2's passthroughs. All are `errs.Error` (store/fold errors are wrapped with their `Code` and
`Seq`/`Reason` in `Fields`).

## 8. Tests (100%; TDD; `machine`'s harness for fork/diff — with a per-run lock-failure seam (`failLockRun`) and a per-dir
git fake whose `HeadSHA` is mutated mid-run; `export` and `record` get their own harnesses: `MemFS` recording
`OpenFile` flags/perm; `record` on a temp dir with `.metareview` as a regular file (`ENOTDIR`), `runs.jsonl` as a
directory, a malformed middle line, and a torn final line)
| # | rows |
|---|---|
| F1 | fork from each non-terminal state of a happy sdlc run, plus a **two-iteration** parent whose head advanced at iter-0 `fix` (`h0` → `h1`), asserting `ForkedAtSeq` literally for `--from discover` (nil → the iter-1 re-entry), `--at-iter 0` (→ seq 1, `expected == InitData.Head`), `--at-iter 1`, `--from adjudicate` (nil → iter 1), `--from adjudicate --at-iter 0` (→ the iter-0 transition, `Copied > 0`, discover executor not called on the child's first advance — **not** a restart), `--at-iter 2` → `ERR_CHECKPOINT_NOT_FOUND{from, at_iter: 2}`; `ch` discrimination: `--from adjudicate --at-iter 0` with the worktree at `h0` accepted (child `Head`/`tree.Head == h0`), at `h1` refused with `expected: h0`; copied prefix equals parent `[2..seq]` byte-for-byte modulo `origin` and `prev` (`Origin.Hash` matches `EventsWithLines`); the child `InitData` asserted literally (`ParentRunID`, `Lineage`, `ForkedAtSeq`, `WorkDir`, `Vars`, `CmdsSHA256`, `WorkflowHash`, `WorkflowSource`, `Head == ch`, `Mock`); `ForkResult` literal (`Copied`, `CmdsSHA256`); child folds; child sidecar bytes == parent's; `tree` (+ `fix_baseline` at fix) appended with `State`/`Iter`/`Mock` stamps; `fork` event on parent; parent otherwise byte-identical; unknown `From`; terminal `From`; fork of a fork (`Lineage` grows, `Origin.RunID` = immediate parent); bad run id → `ERR_RUN_NOT_FOUND` for `Fork`, `Diff`, `Export` |
| F2 | `ERR_NO_COMMIT` recovery: fail at fix, fork `--from fix`, negative control (no commit → `ERR_NO_COMMIT` again), commit → `commit_exists` passes, discover/adjudicate not re-run (fake executor call counts; copied `llm_call`s carry `Origin`); child `StartIndex` for `fix`'s successor is 0; commit-before-fork → `ERR_TREE_NOT_AT_CHECKPOINT` whose `Detail` contains the worktree recipe |
| F3 | judge swap on a **completed** parent; freeze rule evaluated on `Pw` **not** `w`: `AcceptWorkflowChange` with bytes that drop `$JUDGE` from adjudicate, `--from verify --at-iter 0 --var JUDGE=b` → still `ERR_VAR_FROZEN{JUDGE, adjudicate}`; bytes that newly reference `$X` in a state that ran, `--var X=…` → accepted; review-loop `--from adjudicate --var JUDGE=b` re-runs adjudicate only with model `b` (MockJudge `Calls()`); sdlc-loop at iter 0 ok, at iter 1 `ERR_VAR_FROZEN{JUDGE, adjudicate}` (first in `States` order although verify also ran); `--var REVIEWER` frozen after discover ran; a var referenced only by a **convergence** `cmd` atom that ran → frozen with `state` = the `converge` event's state; one referenced by a convergence `cmd` that never ran → not frozen; a var referenced by the **overflow handler** of an overflowed parent → `ERR_VAR_FROZEN{X, <state at overflow>}`; calibration parent: no override → fork succeeds (pins re-applied), override → `ERR_CALIBRATION_PINNED`; undeclared var |
| F4 | git preconditions: exact head for every state; `--work-dir` worktree accepted, foreign dir refused, relative refused; child `TreeHash`/`Status` == its worktree's (the per-dir git fake reports values distinct from P's dir); mock parent: child `Open` re-verifies the scenario through `MockLoad`; enforcing child advances cleanly on a pristine checkout; fork after `fix` in the same iteration → `commit_exists` `ERR_GATE_INAPPLICABLE` |
| F5 | workflow change: refused without the flag; `ERR_WORKFLOW_TOO_LARGE`; invalid bytes → `ERR_WORKFLOW_INVALID`; incompatible (renamed workflow / changed initial / removed copied state / changed kind / changed exec / `From` removed or renamed with an unchanged prefix / removed referenced `cmd`) → `ERR_WORKFLOW_INCOMPATIBLE{reason}` with **no child directory**; **positive controls**: removal of a not-yet-run state, removal of an unreferenced `cmd`, an added state, changed gates/model/params — all accepted with a child created and `WorkflowSource: path`; accepted with a tightened `Decode` → `ERR_COPY_INVALID{seq, reason: decode}` with no child directory; `output_hash` mismatch, un-sanctioned `cmd`, and `max_events` (store with a small `MaxEvents`) likewise, with the boundary control `MaxEvents` == the child's exact **counted** total accepted while total events exceed it; dropped var reported; cmd set change requires the new sha (list printed); mock run forks as mock, registry mismatch refused; torn parent refused before any I/O; escalated parent (attempt-3 non-PASS leaf) → `ERR_RUN_ESCALATED{attempt: 3}`, an **attempt-4** non-PASS leaf (child of a PASS attempt-3 leaf) → `ERR_RUN_ESCALATED{attempt: 4}`, its non-escalated parent still forkable, a PASS attempt-3 leaf forkable, a **non-terminal** attempt-3 parent (`Outcome == ""`) forkable |
| F6 | `VerifyOrigin`: ok / parent_missing / parent_unreadable / version_unavailable / hash_mismatch / content_mismatch (edited copy with intact `Origin`); precedence with several faults at once; parent read once (counting store fake) |
| F7 | `Diff`: parent vs child rows (`CommonPrefixSeq == ForkedAtSeq` for a fork of a **root** parent and of a fork — `Origin`/`Prev` ignored — even when P's next event is a `tree` with identical `Data` (its `At` differs); `1` when seq 2 differs), one-sided rows, alignment by input hash when indexes differ, one case **per kind** (`match`/`is_real`/`still_present` — a decoder that only reads `match` fails), `Effective` vs `Raw` (`true/0.9` vs `true/0.6` on adjudicate → `RawSame` ∧ ¬`DecisionSame`; `true/0.9` vs `true/0.8` → `RawSame` ∧ `DecisionSame` ∧ ¬`ConfidenceSame` ∧ ¬`Same`, asserted as a literal `CallRow`), match `Effective == Raw`, literal row order on out-of-order insertion differing only in `kind` then only in `input_hash`, nil vs `false` not `DecisionSame`, nil vs nil `DecisionSame`, identical errors not `Same`, `Same` ignores `reasoning`, `SameWorkflow == false` after `AcceptWorkflowChange`, `TransRow.Same` on a diverging outcome, incompatible workflows, determinism (two calls equal) |
| F8 | `Export`: every row of the redaction table with seeded markers (fixture var values differ from workflow defaults and no marker sits in `record.data` — stated constraints) — including a var value inside a `file_hashes` key, inside an argv element (`--token=<v>`), a 3-byte value that must **not** be replaced, nested values (longest first), a `$HOME`-prefixed argv path → `~`, an `llm_call.error` without `: `, a `cmd:` vs built-in `converge.reason`, an unknown-kind `node_output` → `{}` — a var value that is itself an in-repo absolute path (vars first → `$TARGET`), two `file_hashes` keys that collide after substitution (both entries kept), an **intermediate** symlinked component — each marker absent from **every byte under `<Out>/`** (manifest included; no marker in any sidecar either) and present with `IncludeVars` only where the table says so; **positive payload assertions**: `confirmed`/`rejected` intact on a `match-then-adjudicate` line, `detail_summary.files` == the seeded porcelain paths, `tree.status` == the seeded path list, a redacted argv asserted as the full slice (`["tool","--token=$TOKEN","src/x"]`), the exported `init` carries `exported: true`; copy-back: a bundle copied into `.metareview/runs/<id>/` → `Open`/`FoldFull` `bad_payload` at seq 1 (init-only run; `Store.Events` alone passes — payloads decode in `Apply`) / `Store.Events` `ERR_AUDIT_CHAIN` at seq 2 (advanced run); positive assertion that the redacted snapshot keeps `Head`, `Iteration`, `Outcome`, `Goldens`, `NodeOutputs` keys, `Findings`, and carries `redacted: true`; per-line `redacted` arrays and `Manifest.Redacted`/`Records`; unredacted lines byte-identical and pairwise-verifiable, `ChainHead` == source head, original `seq`/`prev` kept; sidecars incl. `workflow.yaml` copied; torn files listed not copied; manifest fields literal; `MaxBytes` refusal writes nothing; symlinked component / non-empty dir / `IncludeVars` + default `Out` refused; `OpenFile` flags `O_WRONLY|O_CREATE|O_EXCL|O_NOFOLLOW` and perm 0600, `MkdirAll` 0700 asserted by the fake; `chain: "redacted"` |
| F9 | runs.jsonl record: a legacy file whose last row is complete but newline-less (an `ESCALATED` review row) → `Terminal` writes `\n` then the row, nothing moved, `runchain.ReadRuns` still sees the review row; a blank line in a legacy file is skipped; **byte-for-byte golden line** per outcome (injected clock) for a root (`previousRunId` absent, `attemptNumber` 1) and a grandchild (3), also decoded with `DisallowUnknownFields` into the §6 key set and with `runchain.ReadRuns`; verdict/status map for all 7 outcomes; ESCALATED at attempt 3 non-PASS with `escalationReason`, never at PASS; idempotent by id (second call with the same view → one row) **and** parent row present + child terminal → two rows (never deduped by target); `mock: true` incl. `MockTainted`; `workflowHash`/`workflowSource` incl. `unknown` for a legacy init; short `base_sha`; `Exists` true/false/missing file; **torn tail write path**: torn final line → `Terminal(view)` → fragment preserved to `runs/.torn/runs.jsonl-*`, exactly one new decodable row, `Exists(id)` true, `runchain.ReadRuns` succeeds; a malformed **terminated** final line and a malformed middle line → `ERR_RUNS_JSONL{malformed}`, nothing written; a planted row with the id but a different `headSha` → `ERR_RUNS_JSONL{id_conflict}`; idempotency when the matching row is not the last line (parent row, child row, second `Terminal` for the parent → still one parent row); `ENOTDIR`/directory-as-file write errors passed through; `run` package rows: `jsonlStore.TornFiles` literal `{Name, SHA256, Bytes}` for a real `audit.torn-*` file (mem store: empty), `MaxEvents()` echoes `Options`, `Counted` true/false per type |
| F10 | refusal postconditions + failure sweeps: after **every** refusal in steps 0–7 **and every step-8 seam failure** P is byte-identical (the `fork` event is appended only in step 9) and, for steps 0–7, no child directory exists; the failure seams (`Sidecar.Write`, `run.Create`, child `Lock` via `failLockRun`, parent `Lock`, each `Append` of a copied/`tree`/`fix_baseline`/`fork` event, `Git.Head/CommonDir/WorkTree/Status`, `MockLoad`, export `FS.MkdirAll/OpenFile/Lstat`, recorder read/write) each pass their error through unchanged; after each injected append failure in step 8 the child `Open`s as `ERR_FORK_INCOMPLETE` (or, for a failure at the `fork` append, opens complete with P lacking the `fork` event); ordinal continuity (`RunnerDeps.CmdCalls` on the child counts copied `cmd_call`s) |
| F11 | incomplete fork: crafted children with `Seq == ForkedAtSeq`, with a `tree` but no `fix_baseline` at an agent-edit checkpoint, and with no sidecar → `Open` `ERR_FORK_INCOMPLETE` (checked before the sidecar read), `List` error summary; a non-agent-edit checkpoint with `Seq == ForkedAtSeq+1` opens normally |

## 9. Ledger (deviations from plan r3 / the design, and handoffs)
| item | decision |
|---|---|
| plan §1.9 "row appended once, at terminal" (C27) | honoured; r2's `forked` row removed |
| plan §1.9 `maxAttempts: 0`; "chaining via `--previous-run` within the FSM scope" | 3, so that ESCALATED is producible; `attemptNumber = len(Lineage)+1`; no `--previous-run` for FSM runs (lineage is in the log) |
| plan §1.6 `overflow → NEEDS_REVISION` | honoured; ESCALATED = third non-PASS attempt of a lineage |
| design "ESCALATED blocks the same scope and target" (per-target, `escalatedForTarget`) | **lineage-depth** rule for FSM runs; ancestor forks and same-base `init` are human resets (rationale §6); enforcement from the chained snapshot, not from runs.jsonl |
| existing writers' `status` vocabulary | reused (`passed|needs-revision|escalated`) |
| plan §1.7 FEA-N4 ancestor rule for agent-edit | dropped: `HEAD == ch` everywhere; "fork, then commit" (design §10.1's commit-first order is replaced; the hint carries the worktree recipe) |
| plan §1.7.4 freeze on `cmd:` references (FEA-N3/INT-10) | restored: convergence/overflow `cmd`s that ran freeze their vars |
| plan §1.7.5 `ERR_AUDIT_INVALID` on copy | `ERR_COPY_INVALID{seq, reason}` from an in-memory fold + count, before any I/O |
| stored vars are resolved values | a changed workflow's new defaults never apply in a fork (stated §2 step 3) |
| plan §3.5 export files / `llm_call.raw` dropped | files: manifest, `audit.redacted.jsonl`, `snapshot.json` (redacted), `workflow.yaml`, sidecars; `llm_call.error` kept (already a bare code) |
| plan §3.5 `Diff` `verdict` field / `(node, iter, index)` alignment | `Raw`/`Effective` via `kind.Decision`; alignment by `(node, iter, kind, input_hash)` |
| golden comments | not sensitive (the calibration answer key by design); kept everywhere |
| spec 4 §9 handoff "`Rejected` desc redacted on export" | retired: `rejected[].desc` equals an exported `issue_text`; kept |
| run spec §5.4 "older binary reports `version`" | accepted exception for `InitData.WorkflowSource` (no tagged release contains `run` yet); thereafter every persisted-field addition bumps `SchemaVersion` |
| `state.AppendJSONL` | not used by `record`: torn-tail repair + `flock` + fsync + id-conflict check are `record`'s own writer |
| plan §3.5 gate `detail` → `{files, insertions, deletions}` for all gates | `detail_summary {files, truncated}` for `commit_exists` only (the stored detail is porcelain + diff text, no diffstat); other gates emptied |
| plan §1.7.4 "or any `cmd:` references K" | **narrowed** to cmds that actually ran in the copied prefix (a never-run cmd has no history to invalidate) |
| plan §0.2 CMP-13/FEA-N2 `--from <initial>` → seq 1 | `--at-iter 0` → seq 1; **unset** → the latest re-entry on a looping workflow (plan §1.7.1's "default: latest") |
| plan §3.5 `Diff` shape | adds `SameWorkflow`, `TransRow.Outcome`, `RawSame`/`ConfidenceSame`, per-side `Index`; `Decision` type lives in `machine` |
| `record.data` | kept, documented unredacted, seqs in `Manifest.Records` |
| spec 2 §8 `tree` baseline when `TreeHash == ""` | every fork writes its own `tree`; spec 2's branch stays as defence-in-depth |
| run spec §7 "rebaseline on agent-edit forks" | every fork rebaselines `tree`; `fix_baseline` only for agent-edit `From` |
| run spec §12 row 3 redaction carriers | verdict kept, `record.data` kept, `node_output` kept whole for known kinds (`{}` for unknown) |
| `machine` charter | `Terminal`/`Export` live in `record`/`export`; no new `Deps` field |
| **owned amendments** (mirrored in run spec §11, spec 2 §8, spec 4 §9) | `InitData.WorkflowSource` + `Snapshot.WorkflowSource` (+ `InitOptions.WorkflowSource`), `machine.Decision` type, `RunStore.TornFiles`/`MaxEvents`, `run.Counted`, `TornFile`, incomplete-fork rule in `load`/`summarize`, `RunSummary.Error`, `kind.Decision` |
| spec 4 (follow-up, not blocking) | `Subject` on `llm_call` for finding-level diffs |
| spec 5 (handoff) | `Deps.Terminal = record.Terminal(root, clock)`, `record.Exists` for `--run-id`, `kind.Decision` passed to `Diff`, `export.Deps` wiring, "fork then commit" recipe in `resume_hint`, the `ERR_RUN_ESCALATED`-is-for-a-human agent-prompt sentence, `export.Deps.Home`, `--include-vars` needs an explicit `--out`, `record.data` unredacted, manual deletion of an incomplete fork, `mock` rows never satisfy gates, `status` reads `Store.List()`, `WorkflowSource` set at `init` |
| SEC-13/SEC-7/SEC-22 | closed by §5 (snapshot redacted; var values replaced everywhere argv appears; default `Out` never carries cleartext vars) |
| ARC-21 | target id `<workflow>@<base_sha[:12]>` (length-guarded); `status` section is spec 5's (from `Store.List()`) |
