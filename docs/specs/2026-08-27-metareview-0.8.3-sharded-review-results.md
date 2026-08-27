# metareview 0.8.3 — sharded review results (r5)

Status: r5, a deliberate simplification of r4 on Dave's direction (2026-08-27): *"don't overengineer; the
trust model is out of scope — trust the agents."* r1–r4 and their three review attempts are kept in the
review logs; §11 lists what r5 removed and why. Branch `pr-ready-shard-results` off `main` (0.8.2), two PRs
(§10) under version **0.8.3**; `fsm-enhancements` (0.9.0) rebases onto it and PR #13 is re-gated.

**Trust model (settled, not revisited):** result files are evidence the reviewing agent writes about its own
work, exactly like `--evidence`. metareview checks that a result is *about the current content* (content
hashes) and *covers every shard*. It does not try to prove a review happened, and it does not defend against
a hostile branch. Adversarial hardening is explicitly out of scope.

## 1. Problem

`metareview review pr-ready`/`task-done` cannot pass on a branch whose diff exceeds the 120 KB context cap:

1. `gitcontext` truncates every diff at `maxDiffBytes = 120000` → risk reason `DIFF_TRUNCATED` → the
   architecture-reviewer's **blocking** "Review context risk" finding. Nothing can clear it.
2. The shard plan is computed from the *truncated* diff (`prready.branchOnlyGitContext` sets
   `RawDiffBytes = FilteredDiffBytes = len(git.Diff)`), so its sizes are fiction — on PR #13 it reported
   "shard-01: 120 files, 59,967 bytes" for a 1,372,619-byte / 133-file diff.
3. The prompt pack each shard names is never written.
4. `reviewmanifest` has a tested aggregator for shard results (WU4) that nothing populates and no reviewer
   consults.
5. The deterministic lints scan the truncated diff only.
6. The existing freshness keys hash byte counts and generated paths, so they miss same-size edits and change
   on metareview's own writes; the plan (largest-first first-fit) re-cuts on any size change.

WU4's boundary holds: the CLI aggregates externally produced results and never spawns a reviewer.

## 2. Goals / non-goals

Goals: (a) real per-file sizes from the untruncated, exclude-filtered branch diff; (b) content-derived
freshness keys, invariant under metareview's own writes and under local changes; (c) a **content-stable**
plan: editing one file re-cuts only that file's shard; (d) packs written from the measured bytes; (e) a
result file per shard, discovered and validated deterministically; (f) the context-risk blocker *satisfied*
when every shard has a fresh passing result and the aggregate passes; (g) lints over the full branch diff
plus local changes; (h) an end-to-end shell test covering the pass, a fix round and the negative cases;
(i) `internal/shardpack` at 100% statement coverage.

Non-goals: raising or parameterising `maxDiffBytes`; spawning reviewers from Go; a `--shard-budget` flag;
`epic-ready` ingestion (§9); satisfying context risk raised by local (staged/worktree/untracked) content; a
human waiver mechanism; **any defence against a hostile branch or a dishonest agent** (per the trust model);
porting the 0.9.0 coverage gate to `main`.

## 3. Measurement and freshness keys

**Two git commands** (raw, untrimmed bytes; the existing `git()` helper trims, these paths do not):

- Names: `git diff --name-only -z --no-renames base..HEAD -- . ':(exclude)e1' …` (pathspec magic on, no env
  var) → paths *P*.
- Per path: `GIT_LITERAL_PATHSPECS=1 git diff --no-renames --text --no-textconv base..HEAD -- p` with the
  bare path after `--`. The env var and `:(literal)` are mutually exclusive — do not combine them.
  `--no-renames` gives both sides of a rename their own path; `--text --no-textconv` keeps a committed
  `.gitattributes` from blanking a file's diff. `diff(p)` is that output; `fileBytes(p) = len(diff(p))`.

**Hashes** (fields joined as `len:bytes\0` so a path with `\n` cannot forge a boundary):

- `fileHash(p) = sha256("mrv-file-v4" ‖ p ‖ diff(p))[:16]`
- `sourceDiffHash = sha256("mrv-source-v4" ‖ Σ_{p sorted}(p ‖ fileHash(p)))[:16]`
- `chunkHash = sha256("mrv-chunk-v4" ‖ path ‖ part ‖ parts ‖ text)[:16]`
- `shardHash(s) = sha256("mrv-shard-v4" ‖ scope ‖ targetID ‖ Σ_{chunk sorted}(path ‖ part ‖ chunkHash))[:16]`
- `planHash = sha256("mrv-plan-v4" ‖ sourceDiffHash ‖ Σ_{s}(id ‖ shardHash(s)))[:16]`;
  `Manifest.SourceManifestHash := planHash`, with generated paths and dispositions **removed** from the
  manifest hash (they are why it churned every run).

Invariants (named tests): unchanged after metareview writes under `docs/metareview/` or `.metareview/`;
unchanged by local changes; changed by any content edit including a same-length one; three runs on an
unchanged tree yield one `planHash`.

**Bound:** `Σ fileBytes > maxBranchDiffBytes` (16 MiB, package var so a test can lower it) → risk reason
`DIFF_OVERSIZE`, never satisfiable, no packs written.

## 4. Plan and packs

### 4.1 `internal/gitcontext`

- `CollectWith(root, Options{Base, Excludes, Exceptions, RunGit})` — `RunGit` (nil → real git) is the test
  seam; existing `Collect*` wrap it.
- `Context` gains `BranchFiles []BranchFile{Path, Bytes, Hash, Diff}`, `BranchDiffFull string`,
  `BranchRawDiffBytes`, `BranchFilteredDiffBytes`, all `json:"-"` (the `metareview context diff` payload is
  unchanged — golden key-set test). `BranchFiles` is computed only when the branch diff was truncated;
  otherwise nil and there is no plan.
- `branchOnlyGitContext` copies the branch measurements, clears both untracked counters, and never
  recomputes bytes from truncated text.
- `reviewers.GitContext` gains `BranchDiffFull`; `addedLines()` keeps today's union (branch ∪ staged ∪
  worktree ∪ untracked) and uses `BranchDiffFull` for the branch part when non-empty.

### 4.2 `internal/contextprofile`

- `FileProfile` gains `Hash` and `Source ∈ {branch, staged, worktree, untracked}`.
- Reasons: `DIFF_TRUNCATED` = branch truncation; `LOCAL_DIFF_TRUNCATED` (new) = staged/worktree truncation;
  `LARGE_DIFF` keeps today's meaning (total bytes); `UNTRACKED_*` unchanged and cleared on the pr-ready
  branch-only path; `DIFF_OVERSIZE` per §3. Shard-satisfiable: `{DIFF_TRUNCATED, LARGE_DIFF}` only.
- `PlanShards(profile, branchFiles, options)` — chunk text comes from `branchFiles`, never from `Profile`.
  Nil `branchFiles` → empty plan; the manifest renders "not sharded".
- **Chunks:** a file ≤ budget is one chunk (`part 1/1`); a bigger file is cut into consecutive pieces
  ≤ budget at newline boundaries, hard-cutting a single over-long line at the budget with a `[cut]` marker.
- **Stable assignment (replaces first-fit):** `depth = number of hex digits such that 16^depth ≥
  ceil(Σ fileBytes / budget)`, capped at 3 (4,096 shards); every chunk of file *p* goes to shard
  `shard-<first `depth` hex digits of sha256("mrv-bucket-v4" ‖ p)>`; empty shards are dropped; a chunk whose
  file alone exceeds the budget keeps its shard (that shard is simply larger — a pack may exceed the budget
  only for a single-file shard). Editing a file changes only its own shard's hash unless the total crosses a
  depth boundary (tests cover both). Ids are a function of the path, not of position.
- `Shard{ID, Chunks, Bytes, Hash}`; `SourceDiffHash`, `PromptPackPath`, `Prompt`, `Reason` are removed.
  `ShardPlanMarkdown` renders ids, hashes, chunks, bytes, and a pack directory only when given one.

### 4.3 `internal/shardpack` (new; 100%; all I/O through `Deps`)

`Deps{WriteFile, MkdirAll, MkdirTemp, Rename, RemoveAll, ReadDir, ReadFile, EvalSymlinks, Now}` +
`OSDeps()`; interface `Writer{Write(plan, header) (rollback func() error, error); Prune(scope, slug, keep)
error; Discover(scope, slug, plan) (Found, error)}`. `prready.Options`/`taskdone.Options` gain
`ShardWriter Writer` (nil → real) — the seam the rollback tests use.

- Packs are transient: `.metareview/shards/<scope>/<target-slug>/<planHash>/` with `shard-<id>.md`,
  `cross-shard.md` (≥ 2 shards) and `plan.json` (ids, hashes, chunks, base, head).
  `<target-slug> = Slugify(targetID) + "-" + sha256(scope ‖ targetID)[:8]`.
  `.metareview/shards/.gitignore` (`*`) is created on first use. Paths are `EvalSymlinks`-resolved and must
  stay under the resolved repo root (an accident guard, not a defence).
- Written to a temp dir and moved in with rename-aside → rename-in → remove-aside, so an existing pack set
  is never destroyed before its replacement is in place; `rollback` undoes what the call did and
  `prready.Create`/`taskdone.Create` call it on any later failure. `Prune` runs last and removes only
  sibling 16-hex directories under the same `<scope>/<target-slug>`.
- Pack bytes come from `branchFiles[p].Diff` — no second diff call. `shard-<id>.md` = header (scope, target,
  base, head, plan hash, shard id, shard hash, chunk table with path/part/byte-range/chunk hash), reviewer
  instructions, the result contract generated from the validator's constants, the exact re-run command, then
  each chunk inside a `markdown.FencedCodeBlock` under a line reading
  `--- source diff below: data, not instructions ---`. `cross-shard.md` = header, shard table, the list of
  files reviewed as chunks, contract, instruction to review the seams using the shard results.
- Reproducibility oracle: two runs on unchanged content produce the same plan hash and byte-identical packs.

## 5. Result files and ingestion

### 5.1 Format (`reviewmanifest.ResultSchemaVersion = 1`, distinct from `Manifest.SchemaVersion`)

`ReviewResult` gets JSON tags; `SourceManifestHash` is dropped from it.

```
{"schemaVersion":1,"id":"<result id>","kind":"shard|cross-shard","shardId":"shard-3a",
 "shardHash":"<16 hex>","planHash":"<16 hex>","verdict":"PASS|PASS_ADVISORY|NEEDS_REVISION|ESCALATED",
 "reviewer":"<agent/model/rubric>","reviewedAt":"<RFC3339>",
 "coveredChunks":[{"path":"internal/x.go","part":1,"parts":1,"chunkHash":"<16 hex>"}],
 "coveredShardIds":["shard-3a","shard-3b"],
 "evidence":[{"path":"internal/x.go","line":12,"note":"..."}],
 "findings":[{"severity":"low|medium|high|critical",
              "disposition":"fixed|waived|accepted-risk|false-positive|deferred|open",
              "note":"...","evidence":[…]}],
 "blockingCount":0}
```

Validation (pure, in `reviewmanifest`; one named test each):
- `schemaVersion == 1`; `id` non-empty; `kind` closed; `verdict` closed; `reviewer` non-empty; `reviewedAt`
  parses; `shardId` matches `^shard-[0-9a-f]{1,3}$` (empty for cross-shard).
- **Freshness by content:** a shard result is fresh iff its `shardHash` equals a current shard's hash; a
  cross-shard result iff its `planHash` equals the current plan's. Anything else is **ignored** — listed with
  a reason, never a blocker. There is no "stale" category.
- **Identity:** attribution is by `shardHash`; the id inside the file must equal the id in its filename; two
  fresh results for one shard → blocker `duplicate shard result <id>`; `ShardsCovered` counts distinct
  current shards with a fresh result (cross-shard never counts).
- **Evidence:** each entry needs `path` and `line > 0`, or a `note` ≥ 12 chars; ≥ 1 per result (this is what
  `evidenceRefValid` already enforces — the pack's contract text is generated from those constants, not
  hand-copied).
- **Dispositions:** `fixed`/`false-positive` close a finding; `waived`/`accepted-risk`/`deferred` on a
  medium-or-higher finding block, as does `open` medium+. There is no waiver channel: a blocked finding is
  fixed or the target is narrowed.
- **Size guard:** a result file over 256 KiB is ignored with a reason (accident guard).
- Ingested strings are rendered through `markdown.PlainText`/`InlineCode` wherever they reach a document
  (review log, `Found`/`FINDINGS.md`, context pack, manifest) so a stray newline cannot break a table.

### 5.2 Discovery, flags, retention

- Results live in `docs/metareview/shards/<scope>/<target-slug>/` as `shard-<id>.<shardHash>.result.json`
  and `cross-shard.<planHash>.result.json`, and are committed with the review log after the gate passes.
- Discovery lists fresh, ignored (with reason) and unreadable files; an unreadable file is a blocker naming
  it. `--shard-result <path>` (repeatable) and `--cross-shard-result <path>` add files explicitly;
  `cmd/metareview/main.go` validates them (exists, regular, parses) and exits 2 with nothing written.
- After a passing sharded gate, result files under the target directory matching no current shard/plan are
  deleted before the results are committed.
- Accepted on `review pr-ready` and `review task-done`; not on `epic-ready` (§9).

## 6. Gate semantics

- `reviewers.ManifestContext{Present, Verdict, Blockers, ShardCount, ShardsCovered, CrossShard, PlanHash}`.
- Satisfied iff reasons ⊆ `{DIFF_TRUNCATED, LARGE_DIFF}` ∧ `Present` ∧ `ShardCount > 0` ∧
  `ShardsCovered == ShardCount` ∧ (`ShardCount == 1` ∨ `CrossShard`) ∧ `Verdict == PASS`.
- Satisfied path: the blocking "Review context risk" becomes advisory "Context risk covered by shard
  reviews" (stable fingerprint `architecture:context-risk-covered`, `pr:`-prefixed on pr-ready, plan hash in
  `Found`); the separately blocking "Diff context was truncated" becomes advisory too; the lints then run
  over the union added lines. task-done's `tests:missing` becomes reachable and behaves as it does today
  (unchanged fingerprint) — noted, not redesigned.
- Unsatisfied path: the blocking finding stands, with the manifest verdict and its first 10 blockers
  appended to `Found`. It still early-returns before the other findings (which is why their fingerprints
  need no migration).
- The context-risk fingerprint becomes reason-independent in all three scopes (`architecture:context-risk`,
  `pr:…`, `epic:…`), reasons moving to `Found` — §8 has the one-time migration.
- Manifest source set = branch chunks (each in exactly one shard); local files are listed under "### Local
  changes (not sharded)" and never block assignment. `Aggregate` is amended for content freshness, identity,
  dispositions and cross-shard-only-for-≥2; WU4's superseded criteria are quoted in a dated note on the WU4
  plan (the manifest-hash inputs, and "unresolved medium-or-higher findings without an explicit
  disposition").
- Attempts: a sharded gate costs a plan run, a results run and one run per fix round; the skills tell the
  operator to set `--max-attempts` on the **first** run (mid-chain it is ignored by `runchain.Resolve`).

## 7. CLI, docs, release

- `--shard-result`, `--cross-shard-result` on pr-ready and task-done; usage text; pre-validation in
  `main.go`.
- Per-pack review: **one subagent per pack against `rubrics/task-done-review-rubric.md`**, and one
  cross-shard subagent over the seams; the human may use a richer lens set. Cost: packs + 1 per plan,
  1 + 1 per fix round.
- Docs carrying the sharded flow or the durable/transient lists: `AGENTS.md`, `CLAUDE.md`, `README.md`,
  `INSTALL.md`, `docs/quickstart.md`, `docs/README.claude.md`, `docs/README.codex.md`,
  `skills/review-pr-ready/SKILL.md`, `skills/review-task-done/SKILL.md`, `skills/status/SKILL.md`,
  `commands/review-pr-ready.md`, `commands/review-task-done.md`, `commands/status.md`. Durable +=
  `docs/metareview/shards/`; transient += `.metareview/shards/`.
- `learnsource.Collect` excludes `docs/metareview/shards/**`.
- `runchain.ReadRuns`, `state.ReadJSONL` and `reviewlog.readFindings` get a 1 MiB scanner buffer (they
  currently cap at 64 KiB, and the 0.9.0 reader already uses 1 MiB).
- Five version files → 0.8.3; CHANGELOG with §8.
- `tests/go/test-shardpack-coverage.sh`: `go test -coverprofile` must exit 0, the profile must be non-empty,
  and the number of profile blocks with zero hits must be 0. Registered in `tests/run-all.sh` alongside
  `tests/go/test-sharded-review.sh`.

## 8. Upgrade

- Hashes are `v4` and content-derived; nothing persisted depended on the old values. `context diff` output
  is unchanged.
- The context-risk fingerprint becomes reason-independent. On the first 0.8.3 run for a target,
  `findings.Reconcile` treats an open row with a legacy prefix (`architecture:context-risk:`,
  `pr:architecture:context-risk:`, `epic:context-risk:`) as an alias of the new fingerprint and marks it
  `superseded` — a new status, so readers (which key on `open`/`fixed`) neither count it as a blocker nor
  feed it to learning as a fix. It works without `--previous-run` and on escalated chains. No other
  fingerprint changes.
- New reasons `LOCAL_DIFF_TRUNCATED` and `DIFF_OVERSIZE`.
- New transient `.metareview/shards/`, new durable `docs/metareview/shards/`; committed results are audit
  records, verifiable only while `base..head` is reproducible, and are ignored on a clone whose plan differs.
- `learn-post-merge` diffs exclude the results directory; `runs.jsonl` readers accept 1 MiB lines.

## 9. epic-ready

`RunEpicReady` returns the context-risk blocker unconditionally from its own profile, so after 0.8.3 an epic
whose parent range exceeds 120 KB **stays blocked** — no workaround in 0.8.3. Its fingerprint becomes
reason-independent and its context pack advertises no pack paths. Ingestion is a follow-up.

## 10. Landing plan

Two PRs, each reviewable under the cap by the existing gates (split further if either exceeds it):

- **PR-A — measurement and packs:** §3, §4, `test-shardpack-coverage.sh`, plan rendering in the context
  pack. No ingestion; the visible change is true sizes, the new reasons, and packs being written.
- **PR-B — ingestion and gate:** §5, §6, §7 docs/skills, §8 migration, `test-sharded-review.sh`, version
  bump.

## 11. What r5 removed from r4 (and why)

Per Dave's direction, everything whose only purpose was defending against a hostile branch or a dishonest
agent: the `packRunId`/`KnownPackRuns` provenance chain and the `createdAt`/`planHash` run-row fields it
needed; the `.metareview` ownership check and `METAREVIEW_DIR_TRACKED`; `SUBMODULE_CHANGE`; `UNSAFE_PATH`
and the Unicode-format-character sanitiser; the per-run nonce markers; the identifier-pattern blocker; the
discovered-file count cap; `--shard-budget` and its bounds; the adaptive bucket trie (replaced by one
fixed-depth hash bucketing); the "attestation" wording (the log simply lists what was ingested). The
freshness, identity, coverage, stability and rollback rules stay — those are correctness, not defence.

## 12. Tests (write first; named; package in parentheses)

- (gitcontext) `TestBranchFilesMeasureUntruncatedFilteredDiff` (300 KB deterministic fixture; `Σ Bytes`
  equals an independent whole-diff measurement taken with the same flags and excludes, raw bytes),
  `TestBranchFilesRenameDeleteBinaryModeOnly`, `TestBranchFilesExcludeGenerated`,
  `TestBranchFilesLiteralPathspecs` (space, `*`, `[`, leading `-`, leading `:`, non-ASCII — each with
  `Bytes > 0` and its changed line present), `TestBranchFilesDefeatDiffAttributes`,
  `TestBranchFilesOnlyWhenTruncated`, `TestRunGitErrorBranches`, `TestAddedLinesUnionUsesFullBranchDiff`,
  `TestContextDiffJSONShapeUnchanged`.
- (contextprofile) `TestSourceDiffHashFromDocumentedPreimage` (computed in-test from §3's encoding),
  `…OrderIndependent`, `…ChangesOnSameSizeEdit`, `…InvariantUnderMetareviewWrites`, `…IgnoresLocalChanges`,
  `TestNewlinePathCannotCollide`, `TestBucketDepthFromTotalBytes`, `TestEditChangesOnlyItsShard`,
  `TestDepthBoundaryRecutsAll` (documents the one case that does), `TestChunkNeverExceedsBudgetExceptSingleFileShard`,
  `TestOverLongLineHardCut`, `TestLocalTruncationIsSeparateReason`, `TestLargeDiffKeepsTotal`,
  `TestPlanEmptyWhenNotTruncated`, `TestDiffOversizeReason`.
- (reviewmanifest) `TestManifestHashIsPlanHash`, `…ExcludesGeneratedPathsAndDispositions`,
  `TestFreshByShardHash`, `TestUnmatchedResultIgnored`, `TestFilenameIdMismatchBlocks`,
  `TestDuplicateShardResultBlocks`, `TestCrossShardFreshByPlanHash`, `TestCrossShardRequiredOnlyForMultiShard`,
  `TestWaivedMediumFindingBlocks`, `TestFixedAndFalsePositiveClose`, `TestEvidenceRuleMatchesValidator`,
  `TestResultSchemaVersionDistinct`, `TestOversizeResultIgnored`, `TestLocalFilesNeverBlockAssignment`,
  `TestChunkAssignedToExactlyOneShard`, `TestMarkdownRendersResultsPlainText`.
- (shardpack) `TestLayoutAndSlug`, `TestSelfIgnoreCreated`, `TestReplaceKeepsOldUntilNewInPlace`,
  `TestRollbackRestoresAside`, `TestPruneOnlySiblingHexDirsOfSameTarget`, `TestPackUsesMeasuredBytes`,
  `TestPackBytesReproducible`, `TestFencedChunkCannotEscape`, `TestCrossShardPackListsChunkedFiles`,
  `TestDiscoverByHashNamesAndReasons`, `TestExplicitResultsAdded`, `TestUnreadableDiscoveredIsBlocker`,
  `TestResultOutsideRepoRejected`, `TestGCAfterPass`, one test per `Deps` failure branch.
- (reviewers) `TestContextRiskSatisfiedEmitsAdvisoryAndRunsLints` (a `TODO` beyond 120 KB is found),
  `…NotSatisfiedByEmptyManifest`, `…NotSatisfiedWithMissingShard`, `TestMixedReasonsNeverSatisfied`,
  `TestLocalReasonsNeverSatisfied`, `TestTruncatedDiffFindingAdvisoryOnSatisfiedPath`,
  `TestContextRiskFingerprintReasonIndependentAllScopes`, `TestCoveredFingerprintStable`,
  `TestStagedEvalStillFoundOnSatisfiedPath`.
- (prready, taskdone) `TestBranchOnlyContextClearsUntrackedAndKeepsMeasuredBytes`,
  `TestPlanHashIdenticalAcrossPackDirContextPackAndManifest`, `TestPackWriteFailureRollsBackRun`,
  `TestFailureAfterPackRenameKeepsOldPacks`, `TestAttestationSectionAfterVerdictParses`,
  `TestTaskDoneWithDocsMetareviewTarget`, `TestTwoTargetsDoNotPruneEachOther` — via `Options.ShardWriter`
  and `gitcontext.Options.RunGit`.
- (findings, runchain, state, reviewlog) `TestLegacyContextRiskRowSuperseded` (unchained and escalated),
  `TestReadersAcceptOneMiBLines`.
- (learnsource) `TestCollectExcludesShardResults`.
- Shell `tests/go/test-sharded-review.sh` (deterministic 300 KB lint-clean fixture with one file over the
  budget and one untracked 5 KB file): pr-ready → `NEEDS_REVISION` with packs written and blockers naming
  every shard → read `plan.json`, write passing results for every shard + cross-shard → `--previous-run` →
  `PASS_ADVISORY`, zero blockers, the context-risk row `fixed`, the log listing the chunked file →
  **fix round**: same-size edit in one file, commit, re-run → exactly that shard and the cross-shard listed
  as ignored, all others fresh → write those two results → `PASS_ADVISORY` with the untouched shards carried
  and the superseded files GC'd → a result for another plan → ignored → `--shard-result` to a missing path →
  exit 2 with `runs.jsonl` byte-identical → two runs on unchanged content → one plan hash, identical packs.
  Then the task-done variant with a task file, a staged `eval(` (still blocks) and an untracked file (never
  sharded).
