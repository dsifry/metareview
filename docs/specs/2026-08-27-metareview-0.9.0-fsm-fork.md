# metareview 0.9.0 — spec 3: fork/resume, diff, export, and the runs.jsonl record

> **Status:** DRAFT r2 (2026-08-27). Third of the five split artifacts (ownership ledger: run spec §12 row 3, plus
> CMP-15/ARC-21 reassigned from spec 2). Owns plan r3 §1.7's fork half, `Origin` verification incl. version, `Lineage`,
> the var-freeze rule, git preconditions + `tree`→`fix_baseline`, `--work-dir` validation, re-validation of copied
> outputs, `fsm diff`, `fsm export` (redaction, sidecars, manifest), the `fsmRunRecord` in `.metareview/runs.jsonl`
> with the verdict map, and the `ESCALATED` mapping. Builds on the implemented `run`, `machine` (r4), `kind` (r5).
>
> **r2 changes** (handoffs received from the spec 2/4 attempt-4 logs): the child gets its **own** `workflow.yaml`
> sidecar (the bytes it was parsed from; parent's copy unless `AcceptWorkflowChange`); every fork rebaselines `tree`
> (not only agent-edit); `Deps.Terminal` is the runs.jsonl recorder — idempotent by run id, invoked by the machine on
> every terminal advance (incl. `failed` and the 9b resume) — and lives in this spec's `record.go`; `custom` in the
> verdict map; `Sidecar.List` drives export; `Export` copies `workflow.yaml`, keeps every `node_output` (the snapshot
> must re-fold; `output_hash` is checked) but redacts `Rejected` descs inside `match-then-adjudicate` outputs; `Diff`
> compares decision fields + confidence, never `reasoning`; `Untrusted`/`error.detail` marking is spec 5's; the fork
> re-consents commands in the child's `WorkDir` through `workflow.ResolveCmds` and freezes vars via
> `VarsReferencedBy` on the parent's **resolved** workflow; `ERR_RUN_NOT_FOUND{detail}` passthrough; `ValidateRunID`;
> `Git.CommonDir` (implemented in `gate`) for the same-repository check; ordinal continuity (copied `cmd_call`s count).
>
> **Scope rule:** what `Fork`, `Diff`, `Export`, and the runs-record *produce*. The CLI shapes are spec 5's.

---

## 1. Package
```
internal/fsm/machine   fork.go (Fork, VerifyOrigin), diff.go (Diff), export.go (Export), record.go (Terminal recorder)
```
No new packages. `record.go` imports `internal/runchain` + `internal/state` (existing writers) — `machine` already
imports nothing from `kind`; the recorder is wired by the CLI as `Deps.Terminal = machine.RecordTerminal(repoRoot)`.

## 2. `Fork`
```go
type ForkOptions struct { From run.State; AtIter *int; Vars map[string]string; WorkDir string; AcceptWorkflowChange bool; AllowCustomCmds string; WorkflowBytes []byte /* only with AcceptWorkflowChange: the new workflow (CLI reads name/path) */ }
func (m *Machine) Fork(ctx, ForkOptions) (*Machine, ForkResult, error)   // ForkResult{ChildRunID string; ForkedAtSeq int64; Copied int; Rebaselined bool; CmdsSHA256 string}
```
Algorithm (parent P is `m`, opened via `load`; P is appended only the final `fork` event):
1. **Checkpoint.** `seq` ← the `transition` event in P with `To == From` and (if `AtIter` set) `Iter == *AtIter`,
   latest otherwise; if `From == P.Initial` and (`AtIter` nil or 0) → `seq = 1`. None → `ERR_CHECKPOINT_NOT_FOUND{from,
   at_iter}`. `From` must be a state of P's workflow (`ERR_CHECKPOINT_NOT_FOUND{reason: unknown_state}`). A terminal P is
   allowed (judge-swap on a finished run).
2. **Workflow bytes.** `raw ← P's sidecar` (already verified by `load`). If `WorkflowBytes != nil`: `AcceptWorkflowChange`
   must be set (`ERR_WORKFLOW_CHANGED{expected: P.WorkflowHash, got: sha256(WorkflowBytes)}` otherwise) and `raw ←
   WorkflowBytes`; `Parse(raw, Kinds.Info())` (`ERR_WORKFLOW_INVALID` passes through). `WorkflowHash ← sha256(raw)`.
3. **Vars.** Effective vars = P's stored vars overlaid by `Vars` (undeclared → `ERR_VAR_UNKNOWN`). For each overridden `K`:
   frozen if `K ∈ w.VarsReferencedBy(s)` for any state `s` whose node ran in the copied prefix (`NodesRun` of
   `Fold(P[1..seq])`, on P's **resolved** workflow) → `ERR_VAR_FROZEN{name, state}`. Calibration runs refuse
   `JUDGE`/`JUDGE_EFFORT` (`ERR_CALIBRATION_PINNED`). `Resolve(effective, false)`.
4. **Work dir.** `WorkDir` (default P's) must be absolute and `Git(WorkDir).CommonDir == Git(P.RepoRoot).CommonDir`
   (`ERR_WORKDIR_FOREIGN`).
5. **Commands.** `ResolveCmds(w, WorkDir, LookPath, FileHash)`; if `len > 0 && sha != P.CmdsSHA256` → require
   `AllowCustomCmds == sha` (`ERR_CMDS_NOT_ALLOWED{sha}` with the printed list, exactly as `Init`).
6. **Git precondition.** `ch` = checkpoint head (`TransitionData.Head` of the checkpoint event; `InitData.Head` for seq 1).
   If the node at `From` is `agent-edit`: `IsAncestor(ch, HEAD)`; else `HEAD == ch`. Violation → `ERR_TREE_NOT_AT_CHECKPOINT{expected: ch, got: HEAD}`
   with the hint `git worktree add <dir> <ch>` in `Detail`.
7. **Copy.** Child id `run.RunID(w.Name, Clock().Time)`; `init` = P's `InitData` with `RunID`, `CreatedAt`, `ParentRunID: P`,
   `Lineage: P.Lineage + P`, `ForkedAtSeq: seq`, `WorkDir`, `Vars` (effective), `AllowedCmds`/`CmdsSHA256` (new),
   `WorkflowHash` (new), `Head` (child HEAD), `Mock` (P's; a mock run forks as a mock run — the registry must agree). `run.Create`;
   `Sidecar.Write(child, "workflow.yaml", raw)`; then, under the child's lock, for each P event `2..seq`: **re-validate**
   `node_output` payloads with the child workflow's kind `Decode` (`ERR_COPY_INVALID{seq}` — refuse the fork; the child run
   directory is left for `fsm export`/manual deletion and reported in `Detail`); append verbatim with `Origin{RunID: P,
   Seq, Version: P.Log.Version, Hash: LineHash(P line)}` (bytes from `EventsWithLines`). Copied events keep their `At`,
   `State`, `Iter`, `Node`, `Mock`. Copied `cmd_call`s count toward the child's ordinals (they are in its log).
8. **Rebaseline (every fork).** Append `tree{Head: HEAD, TreeHash: TreeHash(HEAD, WorkTree), Status}` stamped with the
   checkpoint's state/iter; if the node at `From` is `agent-edit`, also `fix_baseline{Head: HEAD}`.
9. **Parent.** Append `fork{ChildRunID, AtSeq: seq}` to P (P's lock); then `Deps.Terminal(P view)` if P has no runs.jsonl row yet
   (a `forked` row so children can chain — §6).
10. Return the child machine (`Open`ed) — its first `Advance` re-runs the node at `From` from `StartIndex` (no copied
    `llm_call` for that key exists: the prefix ends at the transition **into** `From`) and evaluates `From`'s gates.
The judge-swap limitation (spec 5 docs): on `sdlc-loop` `--var JUDGE=…` is accepted only at `--at-iter 0` (adjudicate ran at every later iteration).

## 3. `VerifyOrigin`
`VerifyOrigin(ctx, store, child Log) []OriginCheck` — for each `Origin`-bearing event, read the parent's `EventsWithLines`
(version-selected file), compare `LineHash`; `{Seq, OK bool, Reason: ok|parent_missing|hash_mismatch}`. Used by `fsm diff`
and `fsm export` (reported, never fatal).

## 4. `Diff`
```go
func Diff(a, b run.Log) (Report, error)
type Report struct { A, B string; SameWorkflow bool; CommonPrefixSeq int64; Outcomes [2]run.Outcome; Calls []CallRow; Transitions []TransRow }
type CallRow struct { Node string; Iter, Index int; Kind string; A, B *CallSide; Same bool }   // CallSide{Model, Effort, InputHash string; Decision *bool; Confidence float64; Error string}
type TransRow struct { SeqA, SeqB int64; To run.State; Gate string; Outcome run.Outcome; Same bool }
```
`CallSide.Decision` = the kind's decision field (`match`/`is_real`/`still_present`; nil when the verdict is `null` or the
bool is absent). `Same` = both present ∧ `InputHash` equal ∧ `Decision` equal ∧ `Confidence` equal — `reasoning` never
participates. Rows align by `(node, iter, index)` for `llm_call`s and by ordinal for transitions; different `Workflow`
names → `ERR_DIFF_INCOMPATIBLE`. `CommonPrefixSeq` = the largest `n` such that events `1..n` of both logs have identical
`Data` (excluding `init`). Output is deterministic (sorted rows).

## 5. `Export`
```go
type ExportOptions struct { Out string; IncludeVars bool; MaxBytes int64 /* default 5 MB */ }
func Export(ctx, deps Deps, runID string, opts) (Manifest, error)
type Manifest struct { SchemaVersion int; RunID, Workflow string; ExportedAt run.Time; SourceHead string; Redacted []string; IncludeVars bool; Events int; Sidecars []string; OriginChecks []OriginCheck; Bytes int64; Chain string /* "redacted" */ }
```
Writes `<Out>/{manifest.json, audit.jsonl, snapshot.json, workflow.yaml}` plus copies of every sidecar from
`Sidecar.List` and of `audit.torn-*.bin`. **Redaction** (every event, copied ones included): `gate.error.detail` →
`detail_summary {files, insertions, deletions}` (`detail` emptied, `detail_truncated` kept); `cmd_call`/`overflow_handler`
`stdout`/`stderr` emptied with `*_truncated: true`; `llm_call.verdict` kept (typed JSON); `node_output.output` kept for every
kind **except** that `match-then-adjudicate` outputs have `rejected[].desc` emptied and `review-lenses` outputs keep only
`file`/`line`/`issue_text` (so `output_hash` no longer matches — the exported `snapshot.json` is the **pre-redaction** fold
written by the exporter, and the manifest says `chain: "redacted"`); `record.data` kept; `tree.status` reduced to the file
list; `init.vars` values replaced by `sha256:` unless `IncludeVars`; `init.allowed_cmds[].argv` elements equal to a var
value replaced likewise; `repo_root`/`work_dir` made relative to the repo root. Over `MaxBytes` → `ERR_EXPORT_TOO_LARGE`.
Default `Out` = `docs/metareview/fsm/<run-id>/`.

## 6. `.metareview/runs.jsonl` record (C27)
`RecordTerminal(repoRoot string) func(ctx, View) error` — the `Deps.Terminal` implementation. Idempotent: a row whose `id`
already exists is not written again. Shape matches the existing writers (`internal/artifactreview/review.go:22-41`):
```json
{"schemaVersion":1,"id":"<run>","scope":"fsm-<workflow>","target":{"type":"fsm","id":"<workflow>@<base_sha[:12]>"},
 "status":"complete|forked","verdict":"PASS|NEEDS_REVISION|ESCALATED","previousRunId":"<parent>","attemptNumber":N,"maxAttempts":3,
 "headSha":"…","baseSha":"…","createdAt":"…","updatedAt":"…","repoRoot":"…","executionMode":"fsm","contextPackPath":"","reviewLogPath":"",
 "mock":false,"outcome":"fixed","fsmRunDir":".metareview/runs/<run>/"}
```
Verdict map: `fixed|clean` → PASS; `reviewed|stalled|failed|custom` → NEEDS_REVISION; `overflow` → **ESCALATED**. A
`forked` row (written by `Fork` step 9 when P has none) carries `outcome: ""`, `verdict: NEEDS_REVISION`. `attemptNumber` =
1 + parent's; `maxAttempts` 3 (`runchain.Resolve` applies the same-target escalation rule to forks of an escalated run).
Spec 5 makes sure `.metareview/` is ignored in the target repo (it already self-ignores `runs/`).

## 7. Errors
`ERR_CHECKPOINT_NOT_FOUND{from, at_iter, reason?}`, `ERR_WORKFLOW_CHANGED`, `ERR_VAR_FROZEN{name, state}`, `ERR_CALIBRATION_PINNED`,
`ERR_CMDS_NOT_ALLOWED`, `ERR_WORKDIR_FOREIGN`, `ERR_TREE_NOT_AT_CHECKPOINT{expected, got}`, `ERR_COPY_INVALID{seq}`,
`ERR_DIFF_INCOMPATIBLE`, `ERR_EXPORT_TOO_LARGE`, plus spec 2's passthroughs.

## 8. Tests (100%; TDD; fakes from `machine`'s harness)
| # | rows |
|---|---|
| F1 | fork from each state of a happy sdlc run: checkpoint selection (latest / `--at-iter` / initial → seq 1); copied prefix equals parent `[2..seq]` byte-for-byte modulo `Origin` (hashes match `EventsWithLines`); child folds; child sidecar bytes == parent's; `tree` (+ `fix_baseline` at fix) appended; `fork` event on parent; parent otherwise byte-identical; unknown `From` |
| F2 | `ERR_NO_COMMIT` recovery: fail at fix, fork `--from fix`, negative control (no commit → `ERR_NO_COMMIT` again), commit → `commit_exists` passes, discover/adjudicate not re-run (fake executor call counts; copied `llm_call`s carry `Origin`); child `StartIndex` for `fix`'s successor is 0 |
| F3 | judge swap: review-loop `--from adjudicate --var JUDGE=b` re-runs adjudicate only with model `b` (MockJudge `Calls()`); sdlc-loop at iter 0 ok, at iter 1 `ERR_VAR_FROZEN{JUDGE, adjudicate}`; `--var REVIEWER` frozen after discover ran; calibration refusal; undeclared var |
| F4 | git preconditions: exact head for non-edit states, ancestor rule for agent-edit; `ERR_TREE_NOT_AT_CHECKPOINT` with hint; `--work-dir` worktree accepted, foreign dir refused, relative refused; child `TreeHash` == its worktree's, enforcing child advances cleanly on a pristine checkout |
| F5 | workflow change: refused / accepted with re-Decode (a kind whose `Decode` tightened → `ERR_COPY_INVALID{seq}`); cmd set change requires the new sha (list printed); mock run forks as mock, registry mismatch refused |
| F6 | `VerifyOrigin` ok / parent_missing / hash_mismatch |
| F7 | `Diff`: parent vs child rows, one-sided rows, `Same` ignores `reasoning` (two verdicts differing only in reasoning → `Same`), differs on confidence, incompatible workflows, determinism (two calls equal) |
| F8 | `Export`: every redaction with seeded markers (detail, stdout/stderr, vars, argv-from-vars, paths, tree status, rejected descs, review-lenses fields), sidecars incl. `workflow.yaml` and torn files copied, manifest fields, `MaxBytes` refusal, `IncludeVars`, `chain: "redacted"`, snapshot equals the pre-redaction fold |
| F9 | runs.jsonl record: shape vs existing writers (decode with `runchain.ReadRuns`), verdict map for all 7 outcomes, idempotent on a second call, `forked` row on fork, `attemptNumber`, `--previous-run` resolution via `runchain.Resolve` |
