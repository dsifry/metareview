# metareview 0.8.3 — sharded review results (r1)

Status: draft for adversarial review. Branch `pr-ready-shard-results` off `main` (0.8.2). Ships as **0.8.3** on
`main` so that `fsm-enhancements` (0.9.0, under manual QA) can rebase onto it and be gated honestly.

## 1. Problem

`metareview review pr-ready` (and `task-done`, `epic-ready`) cannot pass on a branch whose diff exceeds the
120 KB context cap, and the tooling that was designed to make that case reviewable is half-built:

1. `internal/gitcontext` truncates every diff at `maxDiffBytes = 120000` and sets `DiffTruncated`; the
   context profile turns that into risk reason `DIFF_TRUNCATED`; the architecture-reviewer turns any
   context risk into a **blocking** "Review context risk" finding (`internal/reviewers/taskdone.go:66`,
   `prready.go` via `branchDiffFindings`, `epicready.go:54`). Nothing downstream can clear it.
2. The Context Shard Plan is computed from the **truncated** diff: `prready.branchOnlyGitContext` sets
   `RawDiffBytes = FilteredDiffBytes = len(git.Diff)` (`internal/prready/review.go:595-596`) and
   `contextprofile.filesFromGit` sizes files by scanning `git.Diff` text. On the 0.9.0 branch (≈976 KB of
   code) the plan reported "shard-01: 120 files, 59,967 bytes" — the numbers are fiction, so the plan cannot
   be used as-is. `sourceDiffHash` (the freshness key for every shard result) is derived from the same
   fiction.
3. Every shard names a prompt pack `docs/metareview/shards/<hash>-<id>.md` that is **never written**
   (`contextprofile.PlanShards` sets the path; no writer exists).
4. `reviewmanifest.Manifest` has `ShardResults` and `CrossShardResult` with a complete, tested aggregator
   (WU4, `docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md`), but no
   command populates them; the manifest always renders "missing shard result for shard-NN" and the
   manifest verdict is decorative (it is not consulted by any reviewer).
5. The deterministic lints (`eval(`, `TODO|FIXME`, inventory paths) scan `addedLines(context.Git)` — the
   truncated diff — so on a large branch they silently cover only the first 120 KB.

WU4 §Scope already set the boundary this spec keeps: *"Keep agent execution out of the Go CLI. The CLI
should model and aggregate externally produced shard results, not spawn Codex, Claude, or other
runtimes."*

## 2. Goals / non-goals

Goals: (a) true sizes and a trustworthy plan; (b) prompt packs actually written; (c) a documented, closed
result-file format the host agent writes per shard, discovered and validated deterministically; (d) the
context-risk blocker is *satisfied* — not suppressed — when the manifest aggregate passes; (e) lints cover
the whole diff; (f) an end-to-end shell test that drives a >120 KB branch from `NEEDS_REVISION` to `PASS`
with shard results and back to blocked with a stale result.

Non-goals: raising or parameterising `maxDiffBytes`; spawning reviewers from Go; changing the artifact
review flow; changing `.metareview/` state formats; `epic-ready` result ingestion (it keeps its current
blocker; it is reviewed on the parent's diff, which is the child task ranges already gated — see §9).

## 3. True sizes (`internal/gitcontext`, `internal/contextprofile`, `internal/prready`)

- `gitcontext.Context` gains `FileDiffBytes map[string]int` (branch diff, keyed by post-image path) measured
  from the **untruncated** `git diff base..HEAD` output inside `limitedGitMeasured`'s caller, using the same
  `diff --git` header walk `contextprofile.addDiffProfiles` uses today (that helper moves to `gitcontext`
  as `DiffFileBytes(text string) map[string]int`; `contextprofile` calls it). Staged, working-tree and
  untracked contributions keep their current (text-scan) measurement — they are small by construction and
  outside the branch gate.
- `contextprofile.filesFromGit` uses `git.FileDiffBytes` for the branch diff when present and falls back to
  scanning `git.Diff` only when the map is nil (older callers/tests). `RawDiffBytes`/`FilteredDiffBytes`
  are the measured values already returned by `limitedGitMeasured`; **no caller may recompute them from
  truncated text**: `prready.branchOnlyGitContext` copies the branch-measured bytes (`git.BranchRawDiffBytes`,
  `git.BranchFilteredDiffBytes`, two new fields populated in `collect`) instead of `len(git.Diff)`.
- `sourceDiffHash` therefore changes for every existing plan; that is intended (the old hashes were derived
  from truncated data) and there are no persisted results to invalidate.
- Reviewer lints: `reviewers.GitContext` gains `AddedLines []string` populated by the caller from the full
  diff (`gitcontext.AddedLines(fullDiff)`); `reviewers.addedLines` prefers it over scanning `Diff`. The
  context pack keeps rendering the truncated `Diff` (the pack is for humans/agents; the lints are not).
  `gitcontext.Context` carries the full text as `FullDiff string` (not rendered anywhere; bounded by
  `maxFullDiffBytes = 8 << 20`; beyond that `FullDiffTruncated = true` adds risk reason `DIFF_OVERSIZE`,
  which no shard result can satisfy — the branch must be split).

## 4. Prompt packs (`internal/contextprofile`, `internal/prready`, `internal/taskdone`)

When a review runs with `RiskLevel == context-risk` and a non-empty shard plan, the review writes, before
the reviewers run and regardless of verdict:

- `docs/metareview/shards/<sourceDiffHash>-<shard-id>.md` per shard: header (run id, scope, target, base,
  head, source diff hash, source manifest hash, shard id, byte count), the shard prompt (`Shard.Prompt`,
  extended with the result contract below and the exact resubmit command), then the **full diff of the
  shard's paths** obtained with `git diff base..HEAD -- <paths>` (pathspec-quoted, `--` always present).
  Byte count ≤ `MaxBytesPerShard` by construction, except a single file larger than the shard budget, which
  becomes its own shard with `Reason: "oversize-file"` and the diff truncated at `MaxBytesPerShard` with a
  trailing `[truncated: N of M bytes]` line — such a shard can only be closed by a result whose
  `findings` include a `disposition: accepted-risk` entry with a note, and the cross-shard result must cover
  it (the aggregator already enforces "has blockers" → the pack tells the reviewer this).
- `docs/metareview/shards/<sourceDiffHash>-cross-shard.md`: header, the shard list with paths and byte
  counts, the result contract, and the instruction to review integration seams across shards using the
  shard results as input.
- Packs are written atomically (temp + rename) and are idempotent for the same hash; a differing existing
  file with the same name is overwritten (the hash makes the name content-addressed for the plan; the body
  may legitimately change with run id). They are durable (`docs/metareview/` policy) and appear in the
  context pack under "Context Shard Plan" as today.
- The generated-path excludes (`generatedMetareviewPathExcludes`) already exclude `docs/metareview/**`, so
  packs and result files never enter the source diff or the manifest.

## 5. Result files and ingestion

### 5.1 Format

One JSON object per file, schema = `reviewmanifest.ReviewResult` with these JSON names (added as struct
tags; the Go types are unchanged):

```
{"schemaVersion":1,"id":"<free text, usually the review run id>","path":"<optional log path>",
 "shardId":"shard-01","verdict":"PASS|PASS_ADVISORY|NEEDS_REVISION|ESCALATED",
 "sourceManifestHash":"<from the pack header>","reviewer":"<agent/model/lens set>",
 "coveredPaths":["..."],"coveredShardIds":["shard-01","shard-02"],
 "evidence":[{"path":"internal/x.go","line":12,"note":"..."}],
 "findings":[{"severity":"low|medium|high|critical","disposition":"fixed|waived|accepted-risk|false-positive|deferred|open",
              "evidence":[{"path":"...","line":1,"note":"..."}]}],
 "blockingCount":0}
```

Rules (all already enforced by `reviewResultBlockers`; restated so the pack can print them): `schemaVersion`
must be 1; `id` or `path` required; `verdict` closed enum; `sourceManifestHash` must equal the current
manifest hash (else "stale shard result …"); `reviewer` required; at least one evidence entry with a path;
a shard result's `coveredPaths` must equal the shard's paths; a cross-shard result's `coveredShardIds` must
equal the plan's shard ids; `blockingCount > 0`, `NEEDS_REVISION`/`ESCALATED`, or a medium-or-higher finding
left `open` blocks.

### 5.2 Discovery

- Default: the review scans `docs/metareview/shards/` for `<currentSourceDiffHash>-<shard-id>.result.json`
  and `<currentSourceDiffHash>-cross-shard.result.json`. Files for other hashes are ignored (they belong to
  other plans) and listed in the context pack under "Ignored shard results" so a stale set is visible.
- Explicit: `--shard-result <path>` (repeatable) and `--cross-shard-result <path>` add files regardless of
  name; the same shard supplied twice is the existing "duplicate shard result" blocker. Explicit paths that
  do not exist or do not parse are an **error before any state is written** (exit 2, message names the file);
  a discovered file that does not parse becomes a manifest blocker `unreadable shard result <path>` (the
  review still runs, so the agent sees it in the log).
- Accepted on `review pr-ready` and `review task-done`. `epic-ready` does not accept them (§9).

### 5.3 Trust

Result files are host-written evidence, exactly like `--evidence`: the gate checks structure, freshness and
coverage, not truth. The context pack lists every ingested result (path, shard id, verdict, reviewer,
blocking count) so a human can audit what satisfied the gate. Result files are committed under
`docs/metareview/shards/` with the review log.

## 6. Gate semantics (`internal/reviewers`, `internal/prready`, `internal/taskdone`)

- `reviewers.Context`/`PRReadyContext` gain `Manifest reviewers.ManifestContext{Verdict string; Blockers
  []string; ShardCount int; ResultCount int; SourceManifestHash string; Present bool}` populated from
  `reviewmanifest.Aggregate` of the manifest built with the ingested results.
- The "Review context risk" finding rule becomes:
  - risk reasons contain only shard-satisfiable reasons (`DIFF_TRUNCATED`, `LARGE_DIFF`) **and**
    `Manifest.Present && Manifest.Verdict == PASS` → emit an **advisory** finding "Context risk covered by
    shard reviews" (`Found`: N shards, N results, manifest hash; fingerprint
    `architecture:context-risk-covered:<hash>`), and the review proceeds to the normal lints over the full
    diff.
  - otherwise the existing blocking finding stands; its `Found` now appends "Manifest: <verdict>;
    blockers: …" (first 10, then "+N more") so the next action is explicit. `UNTRACKED_OMITTED`,
    `UNTRACKED_TRUNCATED` and `DIFF_OVERSIZE` are never shard-satisfiable.
- `reviewmanifest.Aggregate` is unchanged. `Markdown` additionally renders "### Shard Results" (id, verdict,
  reviewer, blocking count, path) and "### Ignored Shard Results".
- Fingerprints of the blocking finding are unchanged so `--previous-run` reconciliation resolves it when the
  advisory replaces it.

## 7. CLI and docs

- `review pr-ready` and `review task-done`: `--shard-result <path>` (repeatable), `--cross-shard-result
  <path>`. Usage text and `docs/quickstart.md` updated. No `--max-diff-bytes`.
- `skills/review-pr-ready/SKILL.md`, `skills/review-task-done/SKILL.md`, `commands/*.md`: a "Sharded review"
  section — when the log's blocking finding is context risk and the context pack lists a shard plan: for
  each shard prompt pack dispatch the standard lens set as subagents on that pack (packs are self-contained),
  write `<hash>-<id>.result.json` with the lens aggregate, then one cross-shard subagent over all results
  and the cross-shard pack, then re-run with `--previous-run <id>`. The orchestrator discipline rules
  (0.8.2) apply per shard. State explicitly that a result file is evidence the agent writes about its own
  work and must reflect the lens verdicts as returned.
- `AGENTS.md`/`CLAUDE.md`/`README.md` durable list += `docs/metareview/shards/`. CHANGELOG 0.8.3.

## 8. Tests (write first; every bullet is a named test)

- gitcontext: `TestFileDiffBytesMeasuresUntruncatedBranchDiff` (a 300 KB branch diff: `FileDiffBytes` sums
  to `RawDiffBytes`, `DiffTruncated` true, `FullDiff` intact); `TestFullDiffOversizeAddsReason`
  (`maxFullDiffBytes` lowered via an unexported var for the test); `TestAddedLinesFromFullDiff`.
- contextprofile: `TestFilesFromGitPrefersMeasuredBytes`; `TestShardPlanUsesTrueSizesAndIsStable` (hash
  differs between truncated-text sizing and measured sizing; measured plan splits 300 KB into ≥5 shards of
  ≤60 KB); `TestOversizeFileBecomesOwnShard`.
- shard pack writer (new `internal/shardpack`): `TestWritePacksAreAtomicIdempotentAndComplete` (header
  fields, full per-path diff, contract text, cross-shard pack); `TestOversizePackTruncatesWithMarker`;
  `TestPathspecIsQuoted` (a path with a leading `-` and a space).
- result loader (`internal/shardpack` or `reviewmanifest`): discovery by hash, other-hash ignored and
  listed, explicit paths, missing explicit path → error, unreadable discovered → blocker, duplicate.
- reviewers: `TestContextRiskSatisfiedByPassingManifest` (advisory emitted, lints run over `AddedLines`),
  `TestContextRiskStaysBlockingWithManifestBlockers` (Found lists them), `TestUntrackedRiskNeverSatisfied`,
  `TestLintsScanFullDiff` (a `TODO` beyond the 120 KB boundary is found) — for task-done and pr-ready.
- prready/taskdone: `TestBranchOnlyContextKeepsMeasuredBytes`; result files rendered in the context pack.
- CLI: flag parsing, exit 2 on a missing explicit result path with nothing written.
- Shell `tests/go/test-sharded-review.sh`: repo with a 300 KB branch diff → pr-ready `NEEDS_REVISION` with
  packs written and manifest blockers listed → write passing results for every shard + cross-shard →
  `--previous-run` → `PASS_ADVISORY` (advisory "covered by shard reviews", context-risk finding reconciled)
  → append one byte to a source file, commit → old results are stale → `NEEDS_REVISION` naming the stale
  shard. Same for task-done with a task file.
- Coverage: new package `internal/shardpack` at 100%; touched legacy packages must not drop below
  `tests/coverage-floor.txt` and the floor is raised to the new values in the same PR (`--update-floor`,
  no decrease).

## 9. Out of scope, recorded

- `epic-ready` keeps the blocking finding. Its diff is the parent range whose child task ranges were gated;
  an epic on a >120 KB range can be re-run after the child task-done runs with shard results resolve, which
  this spec makes possible. Ingestion for epic-ready is a follow-up if it is ever needed.
- The shard prompt packs contain full diffs of source files, i.e. the same content as the PR; they are
  committed under `docs/metareview/shards/`. Repositories that must not duplicate source in docs can delete
  the packs after the results are written (results carry the hash, not the diff).
- Sharded review is more expensive than a single review (N shard lens runs + 1 cross-shard run); the skill
  says so and leaves the choice of dispatching the lenses on a cheaper model to the human.

## 10. Upgrade

Additive. Existing logs and state are untouched. `sourceDiffHash` values in old context packs no longer
match new plans; nothing depended on them. Nothing new to ignore; `docs/metareview/shards/` is durable.
