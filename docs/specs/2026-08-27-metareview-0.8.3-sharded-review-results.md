# metareview 0.8.3 — sharded review results (r7)

Status: r7 = r6 plus the fixes from its recorded three-lens review (§14). r6 = r5 plus the fixes from a
three-lens verification of r5 (§13).
r5 was written on Dave's direction (2026-08-27): *"don't overengineer; the
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
- `PlanShards(profile Profile, branchFiles []gitcontext.BranchFile, options ShardOptions) (ShardPlan, error)`
  — chunk text comes from `branchFiles`, never from `Profile`. Nil `branchFiles` → empty plan; the manifest
  renders "not sharded". `ShardPlanMarkdown(plan ShardPlan, packDir string) string` renders a pack directory
  only when `packDir` is non-empty. `ShardOptions.GroupBy` and its machinery (`groupedFiles`,
  `shardGroupKey`, `shardReason`) are **removed** along with `TestPlanShardsHonorsDomainGrouping`
  (`shards_test.go:49`): hash bucketing replaces grouping, and no caller passed anything but `"path"`.
- **Budget:** the fixed `contextprofile.DefaultMaxBytesPerShard = 60000`. It is not configurable in 0.8.3
  (tests lower it through the existing `ShardOptions.MaxBytesPerShard`).
- **Chunks:** `Chunk{Path string; Part, Parts int; ByteStart, ByteEnd int; Hash string}` — a byte range of
  `diff(p)`. A file ≤ budget is one chunk (`part 1/1`); a bigger file is cut into consecutive pieces
  ≤ budget at newline boundaries, hard-cutting a single over-long line at the budget. Every chunk is
  ≤ budget, without exception: the `[cut]` marker is rendered by the pack writer **outside** the chunk
  text, so it never enters the hashed or measured bytes and `ByteStart`/`ByteEnd` keep mapping to source.
- **Stable assignment (replaces first-fit), in two steps:**
  1. *Bucket by path.* `need = ceil(Σ fileBytes / budget)`; `bits = max(0, ceil(log2(need)))`, capped at 12.
     Every chunk of file *p* goes to the bucket named by the first `bits` bits of
     `sha256("mrv-bucket-v4" ‖ p)`, rendered as `ceil(bits/4)` lowercase hex digits (`bits = 0` → one
     bucket, id `0`). Empty buckets are dropped. Bits, not hex digits: hex granularity jumps 16 → 256 and
     over-shards badly (measured on PR #13: 108 shards where ~23 were wanted).
  2. *Split an over-budget bucket.* A bucket whose bytes exceed the budget is cut over its own chunk list
     sorted by `(Path, Part)`, first-fit, into sub-shards `<hex>-2`, `<hex>-3`, … (the first keeps the bare `<hex>`).
     Since chunks are ≤ budget by construction, **every shard is ≤ budget**. A split re-cuts only that
     bucket.
  Editing a file changes only its own bucket's shards, unless `Σ fileBytes` crosses a `bits` boundary, which
  re-cuts everything (a named test documents that case). Ids are a function of the path, never of position.
- `Shard{ID, Chunks, Bytes, Hash}` where `ID` is the bare bucket name (`3a`, `3a-2`); the wire and filename
  form is `shardId = "shard-" + ID`. `SourceDiffHash`, `PromptPackPath`, `Prompt`, `Reason` are removed, and
  `Shard.Paths` becomes `Shard.Chunks` — `reviewmanifest.Markdown` and `sourceAssignmentBlockers`
  read `Paths` today and are updated with it, as do `manifest.go:264` (coverage arguments — see §5.1, where
  the coverage rule is deleted), `manifest.go:369` (the `SourceManifestHash` preimage — now the plan hash),
  `manifest.go:387` (`canonicalShardPlan`), `contextprofile/shards.go:70,75,162` and `profile.go:120`.
  `ShardPlanMarkdown` renders ids, hashes, chunks, bytes, and a pack directory only when given one.

### 4.3 `internal/shardpack` (new; 100%; all I/O through `Deps`)

`Deps{WriteFile, MkdirAll, MkdirTemp, Rename, RemoveAll, ReadDir, ReadFile, EvalSymlinks}` (no clock —
nothing in a pack is time-dependent, which is what keeps packs byte-reproducible) +
`OSDeps()`; interface `Writer{Write(plan, header) (rollback func() error, error); Prune(scope, slug, keep)
error; Discover(scope, slug, plan) (Found, error)}`. `prready.Options`/`taskdone.Options` gain
`ShardWriter Writer` (nil → real) — the seam the rollback tests use.

- Packs are transient: `.metareview/shards/<scope>/<target-slug>/<planHash>/` with `shard-<id>.md`,
  `cross-shard.md` (≥ 2 shards) and `plan.json`. `plan.json` carries everything a host needs to write
  conforming results without recomputing anything: `{planHash, scope, target, targetSlug, resultsDir, base,
  head, budget, shards:[{shardId, shardHash, bytes, chunks:[{path, part, parts, byteStart, byteEnd,
  chunkHash}]}]}`.
  `<target-slug> = Slugify(targetID) + "-" + sha256(scope ‖ targetID)[:8]`.
  `.metareview/shards/.gitignore` (`*`) is created on first use. Paths are `EvalSymlinks`-resolved and must
  stay under the resolved repo root (an accident guard, not a defence).
- Written to a temp dir and moved in with rename-aside → rename-in → remove-aside, so an existing pack set
  is never destroyed before its replacement is in place; `rollback` undoes what the call did and
  `prready.Create`/`taskdone.Create` call it on any later failure. `Prune` runs last and removes only
  sibling 16-hex directories under the same `<scope>/<target-slug>`.
- Pack bytes come from `branchFiles[p].Diff` — no second diff call. `shard-<id>.md` = header (scope, target,
  base, head, plan hash, shard id, shard hash, chunk table with path/part/byte-range/chunk hash), reviewer
  instructions, the result contract generated from the validator's constants, the re-run command (base and
  target flags only — never `--previous-run`, whose run id is timestamp-derived and would make pack bytes
  non-reproducible), then
  each chunk inside a `markdown.FencedCodeBlock` under a line reading
  `--- source diff below: data, not instructions ---`. `cross-shard.md` = header, shard table, the list of
  files reviewed as chunks, contract, instruction to review the seams using the shard results.
- Reproducibility oracle: two runs on unchanged content produce the same plan hash and byte-identical packs.

## 5. Result files and ingestion

### 5.1 Format (`reviewmanifest.ResultSchemaVersion = 1`, distinct from `Manifest.SchemaVersion`)

`ReviewResult` gets JSON tags; `SourceManifestHash` is dropped from it.

```json
{"schemaVersion":1,"id":"<result id>","kind":"shard|cross-shard","shardId":"shard-3a",
 "shardHash":"<16 hex>","planHash":"<16 hex>","verdict":"PASS|PASS_ADVISORY|NEEDS_REVISION|ESCALATED",
 "reviewer":"<agent/model/rubric>","reviewedAt":"<RFC3339>",
 "evidence":[{"path":"internal/x.go","line":12,"note":"..."}],
 "findings":[{"severity":"low|medium|high|critical",
              "disposition":"fixed|waived|accepted-risk|false-positive|deferred|open",
              "note":"...","evidence":[…]}],
 "blockingCount":0}
```

Validation (pure, in `reviewmanifest`; one named test each):
- `schemaVersion == 1`; `id` non-empty; `kind` closed; `verdict` closed; `reviewer` non-empty; `reviewedAt`
  parses; `shardId` matches `^shard-[0-9a-f]{1,3}(-[0-9]+)?$` (empty for cross-shard); it is the value §4.2
  defines, and the one the filename encodes.
- **Freshness by content:** a shard result is fresh iff its `shardHash` equals a current shard's hash; a
  cross-shard result iff its `planHash` equals the current plan's. Anything else is **ignored** — listed with
  a reason, never a blocker. There is no "stale" category.
- **Identity:** attribution is by `shardHash`; the id inside the file must equal the id in its filename; two
  fresh results for one shard → blocker `duplicate shard result <id>`; `ShardsCovered` counts distinct
  current shards with a fresh result (cross-shard never counts).
- **Coverage is the hash.** `coveredChunks`/`coveredShardIds` are **removed** from the result format, and the
  per-result coverage blockers they fed (`manifest.go:264` `requiredPaths`, and `crossShardBlockers`'
  shard-id equality at `:269-280`) are deleted with them: `shardHash` already commits to the exact chunk set
  and `planHash` to the exact shard set, so the lists could only restate or contradict the hashes. Named
  test: a result whose hash matches is accepted with no coverage list, and `ShardsCovered == ShardCount` is
  what the gate checks.
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
- After a passing sharded gate, files under the target directory whose names match
  `shard-<id>.<hash>.result.json` or `cross-shard.<hash>.result.json` but no current shard/plan are deleted
  before the results are committed; nothing else in the directory is touched.
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
- **Review log rendering.** When results were ingested, the log gets a `## Sharded Review` section placed
  **after the verdict value line** that follows `## Verdict` (so `reviewlog.parseMarkdown`, which takes the
  first non-empty line after the heading, still reads the verdict token — named test). It lists the fresh results (shard id, shard hash, verdict,
  reviewer, blocking count, file), the ignored files with their reason, any unreadable file, and the files
  reviewed as chunks (path, parts, shard ids). Strings come from §5.1's sanitising.
- Unsatisfied path: the blocking finding stands, with the manifest verdict and its first 10 blockers
  appended to `Found`. It still early-returns before the other findings (which is why their fingerprints
  need no migration).
- The context-risk fingerprint becomes reason-independent in all three scopes (`architecture:context-risk`,
  `pr:…`, `epic:…`), reasons moving to `Found` — §8 has the one-time migration.
- Manifest source set: `Manifest.SourcePaths` stays a list of **plain branch paths** (so
  `pathDispositionBlockers` at `manifest.go:188,200` and the "### Source Paths" rendering at `:157` are
  unchanged); the chunk set lives beside it and is what `sourceAssignmentBlockers` checks — every chunk in
  exactly one shard. Local files are listed under "### Local changes (not sharded)" and never block
  assignment. `Aggregate` is amended for content freshness, identity,
  dispositions and cross-shard-only-for-≥2; WU4's superseded criteria are quoted in a dated note on the WU4
  plan (the manifest-hash inputs, and "unresolved medium-or-higher findings without an explicit
  disposition").
- **Local content on task-done.** task-done profiles the full context, so an untracked file over 4,000
  bytes (`UNTRACKED_TRUNCATED`) or staged/worktree truncation puts a non-satisfiable reason in the set and
  the sharded gate cannot pass. That is intended — those bytes are in no pack — and the skills tell the
  operator to commit or remove such files first. pr-ready's branch-only path clears the untracked counters,
  so it is unaffected.
- Attempts: a sharded gate costs a plan run, a results run and one run per fix round; the skills tell the
  operator to set `--max-attempts` on the **first** run (mid-chain it is ignored by `runchain.Resolve`).

## 7. CLI, docs, release

- `--shard-result`, `--cross-shard-result` on pr-ready and task-done; usage text; pre-validation in
  `main.go`.
- Per-pack review: **one subagent per pack against `rubrics/task-done-review-rubric.md`**, and one
  cross-shard subagent over the seams; the human may use a richer lens set. Cost: packs + 1 per plan,
  1 + 1 per fix round. Measured on PR #13 (1,372,619 bytes / 133 files): `need = 23` → `bits = 5` → 32
  buckets, some of which split → **46 shard packs plus one cross-shard pack** (measured against the
  implementation, not estimated): total exactly 1,372,619 bytes, min 1,185, max 60,000, mean 29,840, none
  over budget. Roughly 2 × `need` — the price of hash bucketing, paid for plan stability.
- Docs carrying the sharded flow or the durable/transient lists: `AGENTS.md`, `CLAUDE.md`, `README.md`,
  `INSTALL.md`, `docs/quickstart.md`, `docs/README.claude.md`, `docs/README.codex.md`,
  `skills/review-pr-ready/SKILL.md`, `skills/review-task-done/SKILL.md`, `skills/status/SKILL.md`,
  `commands/review-pr-ready.md`, `commands/review-task-done.md`, `commands/status.md`. Durable +=
  `docs/metareview/shards/`; transient += `.metareview/shards/`.
- `learnsource.Collect` excludes `docs/metareview/shards/**`.
- `runchain.ReadRuns`, `state.ReadJSONL`, `reviewlog.readFindings` and `findings.readJSONL` get a 1 MiB
  scanner buffer (they
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
  feed it to learning as a fix; `fixedInRunID` is left empty, since a non-empty value also reads as a fix. It works without `--previous-run` and on escalated chains. No other
  fingerprint changes.
- New reasons `LOCAL_DIFF_TRUNCATED` and `DIFF_OVERSIZE`.
- New transient `.metareview/shards/`, new durable `docs/metareview/shards/`; committed results are audit
  records, verifiable only while `base..head` is reproducible, and are ignored on a clone whose plan differs.
  Superseded result files are pruned by design (§5.2), so the audit record covers the passing plan rather
  than every plan the branch passed through. Two consequences, plainly: CI cannot produce results itself (it has no reviewing agent), so a sharded gate
  is run by the operator's agent; and moving the `--base` ref changes every hash, so all committed results
  for that target become ignored and the loop is re-run.
- `learn-post-merge` diffs exclude the results directory; `runs.jsonl` readers accept 1 MiB lines.

## 9. epic-ready

`RunEpicReady` returns the context-risk blocker unconditionally from its own profile, so after 0.8.3 an epic
whose parent range exceeds 120 KB **stays blocked** — no workaround in 0.8.3. Its fingerprint becomes
reason-independent, and because epic-ready has no `branchFiles`, `ShardPlanMarkdown` renders **"not
sharded"** where it renders a Context Shard Plan today (`epicready/review.go:500`). That is a visible
behaviour change: the plan it printed was computed from the truncated diff and its numbers were fiction, so
nothing real is lost, but `README.md:59`, `INSTALL.md:128` and `docs/quickstart.md:62` advertise shard
planning for epic-ready and are corrected in §7's docs pass. Ingestion is a follow-up.

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
discovered-file count cap; `--shard-budget` and its bounds; the "attestation" wording (§6 defines a plain
`## Sharded Review` listing instead). The freshness, identity, coverage, stability and rollback rules stay —
those are correctness, not defence.

One r5 deletion went too far, and r6 restores it in simplified form: r4's adaptive bucket splitting existed
to bound **pack size**, not to defend anything, and dropping it let a bucket exceed the 120 KB cap this
feature exists to escape (measured on PR #13: two shards over 120,000 bytes). §4.2 step 2 puts back a
deterministic first-fit split *inside* an over-budget bucket — local, so plan stability is preserved.

## 12. Tests (write first; named; package in parentheses)

- (gitcontext) `TestBranchFilesMeasureUntruncatedFilteredDiff` — 300 KB deterministic fixture; the oracle is
  one `git diff --no-renames --text --no-textconv base..HEAD -- . ':(exclude)…'` invocation (pathspec-magic
  form, no env var, raw untrimmed bytes) whose `len(out)` must equal `Σ fileBytes`; the test must not loop
  per path, or it merely mirrors production,
  `TestBranchFilesRenameDeleteBinaryModeOnly`, `TestBranchFilesExcludeGenerated`,
  `TestBranchFilesLiteralPathspecs` (space, `*`, `[`, leading `-`, leading `:`, non-ASCII — each with
  `Bytes > 0` and its changed line present), `TestBranchFilesDefeatDiffAttributes`,
  `TestBranchFilesOnlyWhenTruncated`, `TestRunGitErrorBranches`, `TestAddedLinesUnionUsesFullBranchDiff`,
  `TestContextDiffJSONShapeUnchanged`.
- (contextprofile) `TestSourceDiffHashFromDocumentedPreimage` (computed in-test from §3's encoding),
  `…OrderIndependent`, `…ChangesOnSameSizeEdit`, `…InvariantUnderMetareviewWrites`, `…IgnoresLocalChanges`,
  `TestNewlinePathCannotCollide`, `TestBucketBitsFromTotalBytes` (a pinned table — need 1→bits 0, 2→1,
  3→2, 4→2, 5→3, cap 12 — never recomputed in the test), `TestEditChangesOnlyItsShard` and
  `TestOverBudgetBucketSplitsLocally` (both on a fixture whose paths are pinned so `sha256("mrv-bucket-v4" ‖ p)`
  puts them in known-distinct buckets, with `MaxBytesPerShard` lowered — a fixture landing every file in one
  bucket makes them vacuous), `TestBitsBoundaryRecutsAll` (documents the one case that re-cuts everything),
  `TestChunkNeverExceedsBudget` (incl. the hard-cut over-long line, with the `[cut]` marker
  outside the measured text), `TestShardNeverExceedsBudget` — its fixture **must** put at least one bucket
  over the lowered `MaxBytesPerShard`, and it asserts both that a `-2` sub-shard id exists and that every
  shard's `Bytes ≤ budget`, so it cannot pass with step 2 unimplemented,
  `TestOverLongLineHardCut`, `TestLocalTruncationIsSeparateReason`, `TestLargeDiffKeepsTotal`,
  `TestPlanEmptyWhenNotTruncated`, `TestDiffOversizeReason`.
- (reviewmanifest) `TestManifestHashIsPlanHash`, `…ExcludesGeneratedPathsAndDispositions`,
  `TestFreshByShardHash`, `TestResultNeedsNoCoverageList`, `TestUnmatchedResultIgnored`, `TestFilenameIdMismatchBlocks`,
  `TestDuplicateShardResultBlocks`, `TestCrossShardFreshByPlanHash`, `TestCrossShardRequiredOnlyForMultiShard`,
  `TestWaivedMediumFindingBlocks`, `TestFixedAndFalsePositiveClose`, `TestEvidenceRuleMatchesValidator`,
  `TestResultSchemaVersionDistinct`, `TestOversizeResultIgnored`, `TestLocalFilesNeverBlockAssignment`,
  `TestChunkAssignedToExactlyOneShard`, `TestMarkdownRendersResultsPlainText`.
- (shardpack) `TestLayoutAndSlug`, `TestSelfIgnoreCreated`, `TestReplaceKeepsOldUntilNewInPlace`,
  `TestRollbackRestoresAside`, `TestPruneOnlySiblingHexDirsOfSameTarget`, `TestPackUsesMeasuredBytes`,
  `TestPackBytesReproducible` (byte-identical `shard-*.md`, `cross-shard.md` and `plan.json` across two
  runs), `TestOSDepsRoundTripOnDisk` (through `OSDeps()` in `t.TempDir()`: Write, then place a real
  result file and one unreadable file under the results dir so `Discover` exercises `ReadFile`, then Prune —
  stub-`Deps` tests never enter the real wrapper bodies and the 100% gate would otherwise stay red),
  `TestFencedChunkCannotEscape`, `TestCrossShardPackListsChunkedFiles`,
  `TestDiscoverByHashNamesAndReasons`, `TestExplicitResultsAdded`, `TestUnreadableDiscoveredIsBlocker`,
  `TestResultOutsideRepoRejected`, `TestGCAfterPass`, one test per `Deps` failure branch.
- (reviewers) `TestContextRiskSatisfiedEmitsAdvisoryAndRunsLints` (a `TODO` beyond 120 KB is found),
  `…NotSatisfiedByEmptyManifest`, `…NotSatisfiedWithMissingShard`, `TestMixedReasonsNeverSatisfied`,
  `TestLocalReasonsNeverSatisfied`, `TestTruncatedDiffFindingAdvisoryOnSatisfiedPath`,
  `TestContextRiskFingerprintReasonIndependentAllScopes`, `TestCoveredFingerprintStable`,
  `TestStagedEvalStillFoundOnSatisfiedPath`.
- (prready, taskdone) `TestBranchOnlyContextClearsUntrackedAndKeepsMeasuredBytes`,
  `TestPlanHashIdenticalAcrossPackDirContextPackAndManifest`, `TestPackWriteFailureRollsBackRun`,
  `TestFailureAfterPackRenameKeepsOldPacks`,
  `TestReviewLogListsIngestedAndIgnoredResults` (the §6 section, rendered after the verdict value
  line and parsed back through `reviewlog.Discover`; the fixture's `reviewer` string carries an
  `mrvf-`-shaped token and the test asserts the parsed `FindingIDs` are unchanged — `reviewlog.go:116`
  harvests that pattern from every line),
  `TestTaskDoneWithDocsMetareviewTarget`, `TestTwoTargetsDoNotPruneEachOther` — via `Options.ShardWriter`
  and `gitcontext.Options.RunGit`.
- (findings, runchain, state, reviewlog) `TestLegacyContextRiskRowSuperseded` (unchained and escalated),
  `TestReadersAcceptOneMiBLines`.
- (learnsource) `TestCollectExcludesShardResults`.
- Shell `tests/go/test-sharded-review.sh` (deterministic 300 KB lint-clean fixture with one file over the
  budget and one untracked 5 KB file): pr-ready → `NEEDS_REVISION` with packs written and blockers naming
  every shard → read `plan.json`, write passing results for every shard + cross-shard → `--previous-run` →
  `PASS_ADVISORY`, zero blockers, the context-risk row `fixed`, the log listing the chunked file →
  **fix round**: a determinate same-size edit (a fixed literal substitution, `AAAA` → `BBBB`, at a known
  line), commit, re-run → assert exact counts: fresh = N-1, ignored = 2 (that shard and the cross-shard,
  each with its reason) → write those two results → `PASS_ADVISORY` with the untouched shards carried and
  the two superseded files GC'd → a result for another plan → ignored → `--shard-result` to a missing path →
  exit 2 with `runs.jsonl` byte-identical → two runs on unchanged content → one plan hash, identical packs.
  Then the task-done variant with a task file, a staged `eval(` (still blocks) and an untracked file kept
  **under 4,000 bytes** (above that it raises `UNTRACKED_TRUNCATED`, never satisfiable — §6).

## 13. r5 verification (three lenses) → r6

| Finding (lens) | r6 |
|---|---|
| Hex-depth bucketing over-shards ~5× (108 shards where ~23 wanted) (Feasibility, Completeness) | §4.2 step 1 buckets on `bits`, not hex digits |
| Pure hash bucketing leaves shards over the 120 KB cap (measured: two) (Feasibility, Completeness) | §4.2 step 2: deterministic first-fit split inside an over-budget bucket; `TestShardNeverExceedsBudget` |
| "budget" undefined after r5 deleted the paragraph (Completeness) | §4.2: fixed `DefaultMaxBytesPerShard = 60000`, not configurable |
| `shard-` prefix doubled in filenames; `id` vs `shardId` ambiguous (Feasibility) | §4.2 `Shard.ID` is bare; `shardId = "shard-" + ID`; the filename encodes `shardId` |
| Review-log results section undefined but still tested (Completeness, Testing) | §6 `## Sharded Review`, after the verdict token; test renamed |
| task-done fixture cannot reach PASS (untracked > 4 KB) (Completeness) | §6 states the consequence; §12 fixture stays under 4,000 bytes |
| `TestChunkNeverExceedsBudgetExceptSingleFileShard` conflates chunk and shard (Testing) | split into chunk and shard tests |
| 100% unreachable: `OSDeps()` bodies uncovered (Testing) | §12 `TestOSDepsRoundTripOnDisk` |
| Whole-diff oracle unimplementable as "same flags" (Testing) | §12 states the exact single invocation and forbids a per-path loop |
| Same-size-edit oracles vacuous (Testing) | §12 pinned bucket-distinct fixture; exact fresh/ignored counts in the shell round |
| `Deps.Now` unused but breaks reproducibility if used (Testing) | dropped from `Deps` |
| `plan.json` too thin for the shell test (Completeness) | §4.3 enumerates its fields incl. `resultsDir` and per-chunk hashes |
| `findings.readJSONL` missing from the scanner list (Completeness) | §7 adds it |
| `Shard.Paths` consumers not noted (Completeness) | §4.2 names `reviewmanifest.Markdown` and `sourceAssignmentBlockers` |
| `superseded` safe only if `fixedInRunID` stays empty (Completeness) | §8 says so |
| CI / moved-base consequences half-stated (Completeness) | §8 states both plainly |

## 14. r6 review (three lenses, run `mrv-20260827-154745525699000`) → r7

Feasibility returned **PASS_ADVISORY**, having reproduced §4.2 on the real PR #13 diff (46 shards, max
exactly 60,000 bytes, no leakage across buckets when eight files were perturbed). Completeness and
Testing-quality returned `NEEDS_REVISION`; every blocker was a choice the spec had left open rather than a
design fault.

| Finding (lens) | r7 |
|---|---|
| `coveredChunks`/`coveredShardIds` unvalidated; `manifest.go:264` coverage blockers unmentioned (Completeness, Feasibility) | both fields **deleted**, and the coverage blockers they fed deleted with them — the hashes already commit to those sets; `TestResultNeedsNoCoverageList` |
| `PlanShards` signature strands epic-ready; `GroupBy` machinery unaddressed (Completeness) | §4.2 gives both signatures and removes `GroupBy` + its test; §9 states epic-ready renders "not sharded" and §7's docs pass corrects README/INSTALL/quickstart |
| `Manifest.SourcePaths` redefinition would break `pathDispositionBlockers` (Completeness) | §6: `SourcePaths` stays plain branch paths; the chunk set sits beside it for assignment only |
| `TestShardNeverExceedsBudget` passes with step 2 unimplemented (Testing) | §12: fixture must exceed the budget; asserts a `-2` sub-shard exists and every shard ≤ budget |
| `[cut]` marker inside chunk text breaks "every chunk ≤ budget" and the byte mapping (Testing) | §4.2: marker rendered outside the hashed/measured text; `Chunk` struct named |
| Pack re-run command would carry a timestamp-derived run id (Testing) | §4.3: base and target flags only, never `--previous-run` |
| `TestOSDepsRoundTripOnDisk` never reaches `OSDeps().ReadFile` (Testing) | §12: the round trip writes a real and an unreadable result file first |
| `mrvf-` tokens in rendered strings contaminate `Summary.FindingIDs` (Testing) | §12: fixture plants such a token and asserts `FindingIDs` unchanged |
| `TestBucketBitsFromTotalBytes` mirrors the implementation (Testing) | §12: pinned expectation table |
| Pack count understated (Feasibility) | §7: measured 46, ~2 × `need` |
| `Shard.Paths` consumer list incomplete (Feasibility) | §4.2 names all of them |
| §6 "after the `## Verdict` token" read literally breaks parsing (Feasibility) | §6: "after the verdict value line" |
| Split tie-break between parts of one path undefined; `Chunk` shape only implied (Completeness) | §4.2: sorted by `(Path, Part)`; `Chunk` struct named |
| GC not scoped to result filename patterns (Completeness) | §5.2 scopes it; §8 notes superseded records are pruned by design |
