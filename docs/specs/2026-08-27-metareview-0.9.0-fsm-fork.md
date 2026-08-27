# metareview 0.9.0 — spec 3: fork/resume, diff, export, and the runs.jsonl record

> **Status:** DRAFT r1 (2026-08-27). Third of the five split artifacts (ownership ledger: run spec §12 row 3). Owns
> plan r3 §1.7's fork half, `Origin` verification incl. version, `Lineage`, the var-freeze rule, git preconditions +
> `tree`→`fix_baseline`, `--work-dir` validation, re-validation of copied outputs, `fsm diff`, `fsm export` (redaction,
> sidecars, manifest), the `fsmRunRecord` in `.metareview/runs.jsonl` with the verdict map, and the `ESCALATED`
> mapping. Builds on `run` (implemented) and spec 2's `machine`.
>
> **Scope rule:** what `Fork`, `Diff`, `Export`, and the runs-record *produce*. The CLI shapes are spec 5's.

---

## 1. Package
```
internal/fsm/machine   Fork (this spec), plus Diff and Export in sub-files; runs.jsonl record appended by Advance at terminal
```
No new packages: fork/diff/export operate on `run` logs and the machine's `Deps`. `runchain`/`state` (existing) are used for
the record.

## 2. `Fork`
```go
type ForkOptions struct { From run.State; AtIter *int; Vars map[string]string; WorkDir string; AcceptWorkflowChange bool; AllowCustomCmds string }
func (m *Machine) Fork(ctx, ForkOptions) (*Machine, ForkResult, error)   // ForkResult{ChildRunID string; ForkedAtSeq int64; Replayed int; Rebaselined bool}
```
Algorithm (parent P is `m`, opened and folded; P is never appended except the final `fork` event):
1. **Checkpoint.** `seq` ← the `transition` event in P with `To == From` and (if `AtIter` set) `Iter == *AtIter`, latest
   otherwise; if `From == P.Initial` and (`AtIter` nil or 0) → `seq = 1`. None → `ERR_CHECKPOINT_NOT_FOUND`. `P.Outcome != ""`
   is allowed (forking a finished run is the judge-swap case).
2. **Workflow.** Reload the workflow (same name/path as P's init; the CLI passes it). If `Hash != P.WorkflowHash` →
   `ERR_WORKFLOW_CHANGED` unless `AcceptWorkflowChange`.
3. **Vars.** Effective vars = P's vars overlaid by `Vars`. For each overridden `K`: frozen if `K ∈ VarsReferencedBy(s)` for any
   state `s` whose node ran in the copied prefix (`NodesRun` of `Fold(P[1..seq])`), or if any `cmd` argv/atom references `$K` →
   `ERR_VAR_FROZEN{K}`. Calibration runs refuse `JUDGE`/`JUDGE_EFFORT` (`ERR_CALIBRATION_PINNED`). Re-`Resolve` the workflow.
4. **Commands.** `ResolveCmds` again in the child's `WorkDir`; if the resulting `cmds_sha256 != P.CmdsSHA256` → require
   `AllowCustomCmds == new sha` (`ERR_CMDS_NOT_ALLOWED` with the printed list).
5. **Work dir.** `WorkDir` (default P's) must be a git worktree of the same repository: `git rev-parse --git-common-dir`
   resolved equals P's (`ERR_WORKDIR_FOREIGN`). Git precondition: `ch` = checkpoint head (`TransitionData.Head` of the
   checkpoint event, or `InitData.Head` for seq 1). If the node at `From` is `agent-edit`: `IsAncestor(ch, HEAD)`; else
   `HEAD == ch`. Violation → `ERR_TREE_NOT_AT_CHECKPOINT{Expected: ch, Got: HEAD}` with the hint `git worktree add <dir> <ch>`.
6. **Copy.** Child id `run.RunID(workflow, now)`; `init` = P's `InitData` with `RunID`, `CreatedAt`, `ParentRunID: P`,
   `Lineage: P.Lineage + P`, `ForkedAtSeq: seq`, `WorkDir`, `Vars` (effective), `CmdsSHA256`/`AllowedCmds` (new),
   `WorkflowHash` (new), `Head` (child HEAD), `Mock` (P's). `run.Create`. Then, under the child's lock, for each P event
   `2..seq`: **re-validate** `node_output` payloads with the (new) workflow's kind `Decode` (`ERR_COPY_INVALID` if a copied
   output no longer decodes — refuse the fork); append verbatim with `Origin{RunID: P, Seq, Version: P.Log.Version,
   Hash: LineHash(P line)}` (the line bytes from `EventsWithLines`). Copied events keep their `At`, `State`, `Iter`, `Mock`.
7. **Rebaseline.** If the node at `From` is `agent-edit`: append `tree{Head: HEAD, TreeHash, Status}` then
   `fix_baseline{Head: HEAD}` (stamped with the checkpoint's state/iter).
8. **Parent.** Append `fork{ChildRunID, AtSeq: seq}` to P (P's lock).
9. Return the child machine (`Open`ed) — its first `Advance` re-runs the node at `From` (NEEDS_INPUT or fork execute) and
   then evaluates `From`'s exit gates.
The judge-swap limitation is documented (spec 5): on `sdlc-loop` `--var JUDGE=…` is only accepted at `--at-iter 0`.

## 3. `VerifyOrigin`
`VerifyOrigin(ctx, store, child Log) []OriginCheck` — for each `Origin`-bearing event, read the parent's `EventsWithLines`
(version-selected file), compare `LineHash`; results `{Seq, OK bool, Reason: ok|parent_missing|hash_mismatch}`. Used by
`fsm diff` and `fsm export` (reported, never fatal).

## 4. `Diff`
```go
func Diff(a, b run.Log) (Report, error)
type Report struct { A, B string; SameWorkflow bool; CommonPrefixSeq int64; Outcomes [2]run.Outcome; Calls []CallRow; Transitions []TransRow }
type CallRow struct { Node string; Iter, Index int; Kind string; A, B *CallSide /* nil when absent on that side */; Same bool }   // CallSide{Model, InputHash string; Verdict json.RawMessage; Confidence float64}
type TransRow struct { SeqA, SeqB int64; To run.State; Gate string; Outcome run.Outcome; Same bool }
```
Rows are aligned by `(node, iter, index)` for `llm_call`s and by ordinal for transitions; different `Workflow` names →
`ERR_DIFF_INCOMPATIBLE`. `CommonPrefixSeq` = the largest `n` such that events `1..n` of both logs have identical `Data`
(excluding `init`) — the fork point for parent/child pairs. Output is deterministic (sorted rows).

## 5. `Export`
```go
type ExportOptions struct { Out string /* dir */; IncludeVars bool; MaxBytes int64 /* default 5 MB */ }
func Export(ctx, store run.RunStore, runID string, opts) (Manifest, error)
type Manifest struct { SchemaVersion int; RunID, Workflow string; ExportedAt run.Time; SourceHead string; Redacted []string; IncludeVars bool; Events int; Sidecars []string; OriginChecks []OriginCheck; Bytes int64 }
```
Writes `<Out>/{manifest.json, audit.jsonl, snapshot.json}` plus copies of `audit.torn-*.bin`. **Redaction** (applied to every
event, including copied ones): `gate.error.detail` → summary `{files: [...], insertions, deletions}` in a new field
`detail_summary` (`detail` emptied, `detail_truncated` kept); `cmd_call`/`overflow_handler` `stdout`/`stderr` emptied with
`*_truncated: true`; `llm_call.verdict` kept (parsed JSON, no raw text) — `Raw` never reaches the audit anyway;
`node_output.output` for `review-lenses`/`agent-edit` kept (the product), `record.data` kept; `tree.status` reduced to
the file list without untracked contents; `init.vars` values replaced by `sha256` unless `IncludeVars`; `repo_root`/`work_dir`
made relative to the repo root; `snapshot.json` = `Fold` of the redacted log. Over `MaxBytes` → `ERR_EXPORT_TOO_LARGE`. The
exported audit is **not** chain-valid (redaction rewrites lines) and says so in the manifest (`chain: "redacted"`).
Default `Out` = `docs/metareview/fsm/<run-id>/`.

## 6. `.metareview/runs.jsonl` record (C27)
Appended by `Advance` when a run becomes terminal (and by `Fork` for the parent if the parent has no row yet — a row with
`status: "forked"` so children can chain). Shape matches the existing writers (`internal/artifactreview/review.go:22-41`):
```json
{"schemaVersion":1,"id":"<run>","scope":"fsm-<workflow>","target":{"type":"fsm","id":"<workflow>@<base_sha[:12]>"},
 "status":"complete|forked","verdict":"PASS|NEEDS_REVISION|ESCALATED","previousRunId":"<parent>","attemptNumber":N,"maxAttempts":3,
 "headSha":"…","baseSha":"…","createdAt":"…","updatedAt":"…","repoRoot":"…","executionMode":"fsm","contextPackPath":"","reviewLogPath":"",
 "mock":false,"outcome":"fixed","fsmRunDir":".metareview/runs/<run>/"}
```
Verdict map: `fixed|clean` → PASS; `reviewed|stalled|failed` → NEEDS_REVISION; `overflow` → **ESCALATED** (the safety stop is
the "stop retrying" case; SCP3-1). `attemptNumber` = 1 + parent's (parent row guaranteed by the fork step). `maxAttempts` 3
(the existing default; `runchain.Resolve` then applies the same-target escalation rule to forks of an escalated run).

## 7. Errors
`ERR_CHECKPOINT_NOT_FOUND`, `ERR_WORKFLOW_CHANGED`, `ERR_VAR_FROZEN`, `ERR_CALIBRATION_PINNED`, `ERR_CMDS_NOT_ALLOWED`,
`ERR_WORKDIR_FOREIGN`, `ERR_TREE_NOT_AT_CHECKPOINT`, `ERR_COPY_INVALID`, `ERR_DIFF_INCOMPATIBLE`, `ERR_EXPORT_TOO_LARGE`.

## 8. Tests (100%; TDD; fakes from spec 2)
| # | rows |
|---|---|
| F1 | fork from each state of a happy sdlc run (temp git repo per test): checkpoint selection (latest / `--at-iter` / initial), copied prefix equals parent `[2..seq]` with `Origin` hashes matching `EventsWithLines`, child folds, `fork` event on parent, parent otherwise byte-identical |
| F2 | `ERR_NO_COMMIT` recovery: fail at fix, fork `--from fix`, negative control (no commit → `ERR_NO_COMMIT` again), commit → `commit_exists` passes, discover/adjudicate not re-run (fake executor/registry call counts, `replay`-free: copied events carry `Origin`) |
| F3 | judge swap: review-loop `--from adjudicate --var JUDGE=b` re-runs adjudicate only with model `b` (MockJudge `Calls()`); sdlc-loop at iter 0 ok, at iter 1 `ERR_VAR_FROZEN`; `--var REVIEWER` frozen after discover ran; calibration refusal |
| F4 | git preconditions: exact head for non-edit states, ancestor rule for agent-edit; `ERR_TREE_NOT_AT_CHECKPOINT` with hint; `--work-dir` worktree accepted, foreign dir refused; `tree`+`fix_baseline` appended and `FixEntryHead` == child HEAD |
| F5 | workflow change: refused / accepted with re-Decode; `ERR_COPY_INVALID`; cmd set change requires new sha |
| F6 | `VerifyOrigin` ok / parent_missing / hash_mismatch |
| F7 | `Diff`: parent vs child rows, one-sided rows, incompatible workflows, determinism |
| F8 | `Export`: every redaction with seeded markers (detail, stdout/stderr, vars, paths, tree status), sidecars copied, manifest fields, `MaxBytes` refusal, `IncludeVars` |
| F9 | runs.jsonl record: shape vs existing writers (decode with `runchain.ReadRuns`), verdict map for all 7 outcomes, parent row on fork, `attemptNumber`, `--previous-run` resolution via `runchain.Resolve` |
