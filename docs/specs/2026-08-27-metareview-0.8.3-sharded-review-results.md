# metareview 0.8.3 — sharded review results (r4)

Status: r4, written after attempt 3 of the adversarial review chain (attempts 1–3 all `NEEDS_REVISION`; the
chain is exhausted). Every attempt-3 blocker is closed below and mapped in §14; the aggregate stays as
returned and the artifact awaits explicit human acceptance. Branch `pr-ready-shard-results` off `main`
(0.8.2). Delivered as two PRs to `main` (§12) under version **0.8.3**; `fsm-enhancements` (0.9.0, under
manual QA) rebases onto it and PR #13 is re-gated.

## 1. Problem

`metareview review pr-ready` (and `task-done`, `epic-ready`) cannot pass on a branch whose diff exceeds the
120 KB context cap, and the tooling designed for that case is half-built:

1. `internal/gitcontext` truncates every diff at `maxDiffBytes = 120000` and sets `DiffTruncated`; the
   context profile turns that into risk reason `DIFF_TRUNCATED`; the architecture-reviewer turns any
   context risk into a **blocking** "Review context risk" finding. Nothing downstream can clear it.
2. The Context Shard Plan is computed from the truncated diff (`prready.branchOnlyGitContext` sets
   `RawDiffBytes = FilteredDiffBytes = len(git.Diff)`); on the 0.9.0 branch it reported "shard-01: 120 files,
   59,967 bytes" while the exclude-filtered diff is 1,372,619 bytes over 133 files.
3. Every shard names a prompt pack that is never written.
4. `reviewmanifest` has `ShardResults`/`CrossShardResult` with a tested aggregator (WU4), but nothing
   populates them and no reviewer consults the manifest verdict.
5. The deterministic lints scan the truncated diff only.
6. The existing freshness keys hash byte counts and generated paths and deadlock on metareview's own
   writes; the shard plan (global largest-first first-fit) is unstable under any size change.

WU4's boundary is kept: the CLI models and aggregates externally produced shard results; it never spawns a
reviewer.

## 2. Goals / non-goals

Goals: (a) content-derived freshness keys, invariant under metareview's own writes and under local
(staged/working-tree/untracked) changes; (b) a **content-stable** shard plan: a change to one file affects
only the shard containing it; (c) packs written from the very bytes that were measured; (d) a closed
result-file format written by the reviewing host per shard, bound to the local pack-writing run through the
`--previous-run` chain, discovered by shard identity and validated deterministically; (e) the context-risk
blocker is *satisfied* — not suppressed — when the manifest aggregate passes, with every finding that becomes
reachable on that path stated; (f) lints cover the whole branch diff plus local changes, as today; (g) an
end-to-end shell test that drives a >120 KB branch to `PASS_ADVISORY` with zero blockers, through a fix
round in which only the changed shard and the cross-shard review are redone, a same-size edit, a foreign
result and a bad explicit path; (h) files larger than the shard budget are reviewed as byte-range chunks —
**a chunk-level review, a weaker guarantee than a whole-file review, stated in the attestation** (§5.3);
(i) declared never-satisfiable classes: `DIFF_OVERSIZE` (exclude-filtered branch diff > 16 MiB),
`TOO_MANY_SHARDS` (> 999), `UNSAFE_PATH`, `SUBMODULE_CHANGE`, `METAREVIEW_DIR_TRACKED`, and every local reason;
(j) `internal/shardpack` at exactly 100% statement coverage, enforced by a check that exists on `main`.

Non-goals: raising or parameterising `maxDiffBytes`; spawning reviewers from Go; changing the artifact
review flow; `epic-ready` ingestion (§10); satisfying context risk raised by local content; a human waiver
mechanism for shard findings (§5.4); porting the 0.9.0 combined coverage gate to `main`; a chain-scoped
escalation lock (§6 states the gap plainly).

## 3. Freshness keys (normative)

**Diff commands.** *E* = the caller's effective generated-path excludes (for a `task-done` targeting a
`docs/metareview/` path, the exact-list-minus-target form `CollectWithExcludesExcept` computes). Two
invocations, deliberately different, both returning **raw, untrimmed** bytes (the existing `git()` helper
trims; these paths do not):

- Name list, pathspec magic **on**, no env var:
  `git diff --name-only -z --no-renames base..HEAD -- . ':(exclude)e1' …` → paths *P* (split on NUL). A
  mode-`160000` entry (`git diff --raw --no-renames -z` is consulted for modes) adds `SUBMODULE_CHANGE`:
  a submodule bump is a 186-byte "Subproject commit" line that hides arbitrary code; never satisfiable.
- Per-path diff, pathspec magic **off**: for each `p ∈ P`,
  `GIT_LITERAL_PATHSPECS=1 git diff --no-renames --text --no-textconv --no-ext-diff base..HEAD -- p` with the
  bare path after `--` (the env var and `:(literal)` are mutually exclusive — verified). `--no-renames` gives
  both sides of a rename their own path; `--text --no-textconv --no-ext-diff` defeats `.gitattributes`
  `-diff`/`diff=`/`textconv`/`binary` and external diff drivers (verified by the attempt-3 Security lens for
  every trick it could commit). `diff(p)` is that output; `fileBytes(p) = len(diff(p))`. The independent
  whole-diff measurement used by tests runs with the identical flag set and excludes.

**Encoding.** Every hash input is a sequence of fields, each `len(field)` in decimal, `:`, the bytes, `\0`.

- `fileHash(p) = sha256("mrv-file-v4" ‖ p ‖ diff(p))[:16]`.
- `sourceDiffHash = sha256("mrv-source-v4" ‖ Σ_{p sorted}(p ‖ fileHash(p)))[:16]`.
- Chunks (§4.2): `chunkHash = sha256("mrv-chunk-v4" ‖ path ‖ part ‖ parts ‖ text)[:16]`.
- `shardHash(s) = sha256("mrv-shard-v4" ‖ scope ‖ targetID ‖ Σ_{chunk ∈ s sorted}(path ‖ part ‖
  chunkHash))[:16]` — scope and target are folded in so a result produced for another target can never
  count here.
- `planHash = sha256("mrv-plan-v4" ‖ sourceDiffHash ‖ Σ_{s}(id ‖ shardHash(s)))[:16]`;
  `reviewmanifest.Manifest.SourceManifestHash := planHash`, generated paths and dispositions excluded.

Invariants (named tests, §8): unchanged after metareview writes anything under `docs/metareview/` or
`.metareview/`; unchanged by local changes; changed by any content edit incl. same-length; a path with `\n`
cannot forge a field; three runs on an unchanged tree → one `planHash`.

**Freshness.** A shard result is **fresh** iff its `shardHash` equals a current shard's hash (attribution is
by hash; ids are metadata — §4.2 makes them stable anyway). A cross-shard result is fresh iff its `planHash`
equals the current plan's. A result matching no current shard/plan is **ignored** (listed with a reason,
never a blocker). There is no "stale" category.

**Bounds.** `Σ fileBytes > maxBranchDiffBytes` (16 MiB, overridable package var) → `DIFF_OVERSIZE`, no packs
written. Paths containing C0/C1 controls, `0x7f`, or Unicode categories Cf/Zl/Zp (bidi, zero-width, line/
paragraph separators) → `UNSAFE_PATH`; path comparison is byte-exact after NFC normalisation. More than 999
shards → `TOO_MANY_SHARDS` (the id space and the run-row scanner both bound it).

**Shard budget.** `DefaultMaxBytesPerShard = 60000`; `--shard-budget <bytes>` is deferred to PR-B (§12) and,
when it lands, is bounded `1 ≤ budget ≤ 100000` at arg validation (exit 2): the pack is the reviewer's
context, and pack overhead (header ≤ 4 KiB, contract ≤ 8 KiB, fences) plus 100000 stays under `maxDiffBytes`.
No recommendation to run PR #13 above the default.

## 4. Measurement, plan, packs

### 4.1 `internal/gitcontext`

- New entry point `CollectWith(root string, opts Options) (Context, error)` with
  `Options{Base string; Excludes, Exceptions []string; RunGit func(root string, env []string, args ...string)
  ([]byte, error)}` (nil → real git); the existing `Collect*` functions wrap it. This is the injection seam
  for every caller's tests (no unexported globals across package boundaries).
- `Context` gains `BranchFiles []BranchFile{Path, Bytes int, Hash string, Diff string}`,
  `BranchDiffFull string` (concatenation of `diff(p)` in *P* order), `BranchRawDiffBytes`,
  `BranchFilteredDiffBytes`, `Submodules []string`, all `json:"-"` (golden key-set test keeps the
  `metareview context diff` shape). `BranchFiles` is computed iff the truncated branch diff was truncated
  (`DiffTruncated`); otherwise nil, and everything downstream that needs a plan reports "not sharded"
  (§4.2). (The former "or exceeds `DefaultLargeDiffBytes`" clause was dead — both constants are 120000 — and
  is dropped.)
- `branchOnlyGitContext` (pr-ready) copies the branch measurements, clears **both** untracked counters and
  never recomputes bytes from truncated text (test).
- Lints: `reviewers.GitContext` gains `BranchDiffFull string`; `reviewers.addedLines()` keeps its union
  derivation (branch ∪ staged ∪ working-tree ∪ untracked) and uses `BranchDiffFull` for the branch part when
  non-empty, else `Diff` as today. No runtime warning finding; each caller has a test asserting the field is
  populated when `DiffTruncated` (a staged `eval(` and a working-tree `TODO` are still found on the satisfied
  path).

### 4.2 `internal/contextprofile`

- `FileProfile` gains `Hash`, `Source ∈ {branch, staged, worktree, untracked}`; a branch-changed file that is
  also locally dirty has `Source: branch` and its local bytes are listed as local, never sharded.
- Risk reasons: `DIFF_TRUNCATED` = branch diff truncated; `LOCAL_DIFF_TRUNCATED` = staged/working-tree
  truncation; `LARGE_DIFF` **keeps today's total** (branch + local); `LOCAL_LARGE_DIFF` (new) = local bytes
  alone > `DefaultLargeDiffBytes`; `UNTRACKED_OMITTED`/`UNTRACKED_TRUNCATED` unchanged (local; cleared on the
  pr-ready branch-only path); plus `DIFF_OVERSIZE`, `TOO_MANY_SHARDS`, `UNSAFE_PATH`, `SUBMODULE_CHANGE`,
  `METAREVIEW_DIR_TRACKED`. Shard-satisfiable set: `{DIFF_TRUNCATED, LARGE_DIFF}`; every other reason is never
  satisfiable, so `LARGE_DIFF` caused by local bytes is satisfiable only when the local bytes are themselves
  within the cap (else `LOCAL_LARGE_DIFF` is also present and blocks). §11 records the new reasons.
- `PlanShards(profile Profile, branchFiles []gitcontext.BranchFile, options ShardOptions) (ShardPlan, error)`:
  chunk text is read from `branchFiles` and never copied into `Profile`. With nil `branchFiles` the plan is
  empty and the manifest renders "not sharded (branch diff within cap)" with no plan hash.
- **Chunks.** A branch file with `fileBytes ≤ budget` is one chunk `part 1/1`. A larger file is cut into
  consecutive chunks ≤ budget at newline boundaries; a single line longer than the budget is hard-cut at the
  budget with a `[cut]` marker line in the pack (so a chunk never exceeds the budget — test with a 200 KB
  single-line fixture).
- **Content-stable plan (replaces first-fit).** Chunks are assigned to *buckets* by a prefix trie over
  `key = sha256("mrv-bucket-v4" ‖ path)` (all chunks of one file share a key and therefore a bucket):
  start with one bucket (prefix `""`); while a bucket's bytes exceed the budget and it holds chunks of more
  than one file, split it into 16 children by the next hex nibble of `key`; a bucket that holds only one
  file's chunks and still exceeds the budget becomes one shard per chunk (`part k/n`). Shard id =
  `shard-<prefix>` (`shard-` for the root, `shard-3`, `shard-3a`, …; per-chunk shards `shard-3a-p2`), so ids
  are a function of the path key, never positional: editing, growing or deleting a file changes only the
  bucket that contains it (its split depth may change, creating or merging children under the same prefix;
  every other shard keeps its id and hash — tests: same-size edit stales exactly one shard; a growth that
  forces a split stales one shard and creates its children; a deletion merges at most one subtree). Empty
  buckets are dropped. `Shard{ID, Chunks []Chunk, Bytes, Hash}`; `SourceDiffHash`, `PromptPackPath`,
  `Prompt`, `Reason` are removed. `Manifest.SourcePaths` becomes a chunk list.
- `ShardPlanMarkdown` renders ids, hashes, chunks and bytes, and a pack directory only when given one
  (epic-ready passes none).

### 4.3 `internal/shardpack` (new; exactly 100%; all I/O via `Deps`)

`Deps{WriteFile, MkdirAll, MkdirTemp, Rename, RemoveAll, ReadDir, ReadFile, Lstat, EvalSymlinks, Now,
RandRead func([]byte) (int, error)}`; `OSDeps()` uses `crypto/rand.Read`, covered through a package-level
`var randRead = rand.Read` used only by `OSDeps` and its test. Interface `Writer{Write(plan, header)
(Rollback func() error, error); Prune(scope, slug, keep string) error; Discover(...)}`; `prready.Options`/
`taskdone.Options` gain `ShardWriter Writer` (nil → `shardpack.New(OSDeps())`) — the seam the §8 rollback
tests use.

- **Ownership check** (before any write or ingestion; failure adds `METAREVIEW_DIR_TRACKED`, never
  satisfiable, and ingestion is refused for the run — no exit 1): `git ls-files -sz -- ':(icase).metareview'`
  must return nothing (any mode, incl. `160000`; case-insensitive filesystems are the development platform);
  `.metareview` must be a real directory (`Lstat`), not inside a nested repository (`git rev-parse
  --show-toplevel` from inside it equals the root), and `git check-ignore -q .metareview/runs.jsonl` must
  succeed. Ordering invariant: discovery/ingestion run only after this check passed in the same run (test).
  `CLAUDE.md`/`INSTALL.md`/`README.md` are amended: `.metareview/runs.jsonl`, `findings.jsonl` and
  `shards/` **must** stay untracked (the "unless the repository changes that contract" clause no longer
  covers them; `knowledge/` and `learning-runs.jsonl` are unaffected).
- **Layout**: `.metareview/shards/<scope>/<target-slug>/<planHash>/` with `shard-<id>.md`, `cross-shard.md`
  (≥ 2 shards), `plan.json` (ids, hashes, chunks, budget, writing run id, base, head).
  `<target-slug> = Slugify(targetID) + "-" + sha256(scope ‖ targetID)[:8]` (`state.Slugify`, then the hash
  suffix so long or similar targets cannot collide). Every component is `EvalSymlinks`-resolved and must stay
  under the resolved root. `.metareview/shards/.gitignore` (`*\n`) is created on first use and never pruned.
- **Replace and rollback**: packs are written to `MkdirTemp` under `.metareview/shards/` and moved in with
  rename-aside → rename-in → remove-aside (an existing `<planHash>` is never destroyed before its replacement
  is in place). `Rollback` removes what this call created and restores an aside if one exists;
  `prready.Create`/`taskdone.Create` call it on any later failure (test: a failure after the pack rename
  leaves the pre-existing pack dir intact and no new one). `Prune` runs last, only under the same
  `<scope>/<target-slug>`, only sibling directories named as 16 lowercase hex.
- **Reproducibility**: pack bytes are a function of the plan, the header and a per-run nonce; the oracle is
  "same plan hash, same chunk hashes, and byte-identical packs after normalising the writing-run-id header
  line and the marker lines" (three runs on unchanged content).
- **Pack content** is built from `branchFiles[p].Diff` — no second diff call. `shard-<id>.md`: header (scope,
  target, base, head, writing run id, plan hash, shard id, shard hash, budget, chunk table: path, part,
  byte range, chunk hash), instructions, the result contract generated from the validator's constants, the
  exact re-run command, then per chunk `<!-- mrv-untrusted-begin <nonce> -->`, the chunk through
  `markdown.FencedCodeBlock`, `<!-- mrv-untrusted-end <nonce> -->`. The fence is the boundary; the nonce is
  defence in depth (16 hex from `RandRead`) and the pack tells the reviewer to ignore any marker whose nonce
  differs from the header's. Every path/hash outside a fence passes `markdown.InlineCode(markdown.PlainText(·))`;
  `UNSAFE_PATH` guarantees the rest is renderable.
- `cross-shard.md`: header, shard table, the list of files reviewed as chunks (explicit seams), contract,
  instruction to review integration seams with the shard results as input.

## 5. Result files and ingestion

### 5.1 Wire format (closed; `reviewmanifest.ResultSchemaVersion = 1`, distinct from `Manifest.SchemaVersion`)

`ReviewResult` gets explicit JSON tags; `SourceManifestHash` is removed from it:

```
{"schemaVersion":1,"id":"<result id>","kind":"shard|cross-shard","shardId":"shard-3a",
 "shardHash":"<16 hex>","planHash":"<16 hex>","packRunId":"<run id from the pack header>",
 "base":"<sha>","head":"<sha>",
 "verdict":"PASS|PASS_ADVISORY|NEEDS_REVISION|ESCALATED","reviewer":"<agent/model/rubric>",
 "reviewedAt":"<RFC3339>",
 "coveredChunks":[{"path":"internal/x.go","part":1,"parts":1,"chunkHash":"<16 hex>"}],
 "coveredShardIds":["shard-3a","shard-3b"],
 "evidence":[{"path":"internal/x.go","line":12,"note":"..."}],
 "findings":[{"severity":"low|medium|high|critical",
              "disposition":"fixed|waived|accepted-risk|false-positive|deferred|open",
              "note":"...","evidence":[{"path":"...","line":1,"note":"..."}]}],
 "blockingCount":0}
```

Validation (pure, in `reviewmanifest`; the caller injects `Input.KnownPackRuns map[runID]KnownPackRun{PlanHash
string; ShardHashes map[string]string; At time.Time}`). Each rule is a named test:
- `schemaVersion == 1`; `id` non-empty; `kind` closed; `shardId` matches `^shard-[0-9a-f]{0,3}(-p[0-9]+)?$`
  (cross-shard: empty); `verdict` closed; `reviewer` non-empty; `reviewedAt` parses.
- Freshness (§3): shard → `shardHash` equals a current shard's hash; cross-shard → `planHash` equals the
  current plan's; otherwise **ignored** (`no current shard` / `no current plan`).
- Identity: attribution is by `shardHash`; the `shardId` inside the file must equal the id in its filename
  (else blocker `shard result <file> id mismatch`) and, for a fresh result, equals the current id by
  construction (§4.2); two fresh results for the same current shard → blocker `duplicate shard result
  shard-<id>`; `ShardsCovered` = number of distinct current shards with a fresh result; cross-shard results
  never count toward it.
- Provenance: `packRunId ∈ KnownPackRuns` **and** (`KnownPackRuns[id].PlanHash == planHash` **or**
  `KnownPackRuns[id].ShardHashes` contains this `shardHash`) **and** `reviewedAt ≥ KnownPackRuns[id].At`
  truncated to whole seconds. `KnownPackRuns` is built **only** from the resolved `--previous-run` chain
  (§5.2); a run row supplies `planHash` and `createdAt`, and its `ShardHashes` are read from that plan's
  `plan.json` (pruned → treated as absent). Otherwise ignored: `unknown pack run` / `run recorded no plan` /
  `reviewed before pack`. Rationale: run ids are unpredictable and the chain is chosen by the reviewer;
  together with the ownership check, a branch cannot ship results that count.
- Evidence: every entry has `path` **and** `line > 0`, or a `note` ≥ 12 chars; ≥ 1 entry per result.
- Dispositions: `fixed`/`false-positive` close a finding (host-attested, unverified — §5.4); `waived`/
  `accepted-risk`/`deferred` on medium+ block; `open` medium+ blocks. Every dispositioned medium+ finding is
  rendered; `fixed`/`false-positive` closures of high/critical are rendered under "### Self-closed
  high/critical shard findings".
- Caps and sanitation: file ≤ 256 KiB; per target at most `2 × (ShardCount + 1)` discovered files are read
  in lexical order, the rest ignored (`over cap`) — never a blocker; arrays ≤ 1,000; `reviewer`/`note`/`id`
  truncated to 512 bytes; strings containing `mrvf-` or `mrv-` + digit → blocker `contains an identifier
  pattern`; every ingested string passes `markdown.PlainText` (which is extended to strip Cf/Zl/Zp) and
  `InlineCode` for ids/paths, `|` escaped, at every sink: review log, `Found`/`FINDINGS.md`, context pack,
  manifest markdown.

### 5.2 Discovery, explicit paths, chain, retention

- Location `docs/metareview/shards/<scope>/<target-slug>/`, files `shard-<id>.<shardHash>.result.json` and
  `cross-shard.<planHash>.result.json`. Results are committed **after** the sharded gate passes, with the
  review log; a fix that must reach the reviewed content is a normal commit (§6 states the escalation
  consequence plainly).
- Default discovery lists fresh, ignored (with reason) and unreadable files; unreadable → blocker `unreadable
  shard result <file>`. Files are `EvalSymlinks`-resolved regular files under the root, read once.
- Explicit `--shard-result` (repeatable) / `--cross-shard-result`: validated in `main.go` (exists, regular,
  ≤ cap, parses) → exit 2, nothing written (shell oracle: `runs.jsonl` byte-identical, no new files).
- Chain: the ingesting run passes `--previous-run <pack-writing run>`; `KnownPackRuns` = the chain's rows.
  Other clones have no rows → results ignored, loop re-run there (audit-only elsewhere; §11).
- Retention (GC): after a passing sharded gate, before the results are committed, the run deletes result
  files under the target directory that match no current shard/plan (test); nothing else touches the
  directory.
- Accepted on `review pr-ready` and `review task-done`; not on `epic-ready` (§10).

### 5.3 Trust and what a sharded pass certifies

The gate checks structure, freshness, identity, coverage and provenance — never truth. The review log's
`## Sharded Review Attestation` section (a separate heading; `## Verdict`'s first non-empty line stays the
verdict token — parse test) states: *"The host attests that every shard below was reviewed against the
recorded chunk hashes and that the manifest aggregate passed; metareview verified freshness, identity,
coverage and provenance, not the review itself. Files listed as chunked were reviewed part by part, not as a
whole."* Then: the results table (shard id, hash, verdict, reviewer, reviewed-at, blocking count,
dispositioned medium+ findings, file), the chunked-files list (path, parts, shard ids), the ignored list and
the self-closed list. `AGENTS.md`'s evidence-honesty sentence and the completion rule are extended to name
shard results. Result files record `base`/`head`; §11 notes that hash verification is possible only while
that range is reproducible.

### 5.4 No waiver channel (recorded)

There is no human-acceptance mechanism for task-done/pr-ready findings and 0.8.3 adds none: a blocked
medium+ shard finding is fixed or the target narrowed. `fixed`/`false-positive` closures are host
attestations, unverified by the gate, and are rendered visibly for high/critical.

## 6. Gate semantics

- `Manifest ManifestContext{Present, Verdict, Blockers, ShardCount, ShardsCovered, CrossShard, PlanHash}`.
  Satisfied iff reasons ⊆ `{DIFF_TRUNCATED, LARGE_DIFF}` ∧ `Present` ∧ `ShardCount > 0` ∧
  `ShardsCovered == ShardCount` ∧ (`ShardCount == 1` ∨ `CrossShard`) ∧ `Verdict == PASS`. Tests: empty
  manifest, missing shard, duplicate, copied file, cross-shard on a single shard, mixed reasons, each
  never-satisfiable reason.
- Satisfied path: blocking "Review context risk" → advisory "Context risk covered by shard reviews"
  (fingerprint `architecture:context-risk-covered`, `pr:` prefixed on pr-ready; plan hash in `Found`); lints
  run over the union; "Diff context was truncated" → advisory; task-done's `tests:missing` becomes reachable
  with fingerprint `tests:missing:<sha256 of the sorted path set>[:16]` (paths stay in `Found`). The
  unsatisfied path still early-returns before those findings (stated because `findings.Reconcile` cannot
  reclassify an open record under the same fingerprint).
- Unsatisfied path: the blocking finding stands; `Found` appends the manifest verdict and first 10 blockers
  through `PlainText`.
- Fingerprints reason-independent in all three scopes (`architecture:context-risk`, `pr:…`, `epic:…`).
- Manifest source set = branch chunks; local files rendered under "### Local changes (not sharded)"; a chunk is
  assigned to exactly one shard.
- `reviewmanifest.Aggregate` amended; WU4's criteria are superseded by quoted text: *"a deterministic
  `sourceManifestHash` computed from schema version, source paths, path dispositions, shard IDs, shard paths,
  shard byte counts, and the underlying WU3 source diff hash"*; *"result provenance … source manifest hash"*;
  *"unresolved medium-or-higher findings without an explicit disposition"* (shard and cross-shard). The WU4
  plan gets a dated note quoting them.
- Escalation, stated plainly: the attempt ceiling is human-set on the root run (`--max-attempts` on a
  chained run is ignored by `runchain.Resolve`); `ESCALATED` rows are head-scoped, so a fix commit lets a
  fresh chain start at attempt 1 — nothing mechanical prevents it; the skill forbids it and a chain-scoped
  lock is a follow-up. A sharded gate costs plan run + results run + one run per fix round; the skill tells
  the operator to set `--max-attempts` on the **first** run to cover the expected rounds (PR #13: 6).
- `DefaultMaxBytesPerShard + overhead ≤ maxDiffBytes` asserted by a test.

## 7. CLI, docs, release surface

- `review pr-ready`/`task-done`: `--shard-result`, `--cross-shard-result` (PR-B); `--shard-budget` deferred
  (§12). Usage text; `main.go` pre-validation.
- Per-pack review, defined: **one subagent per pack against `rubrics/task-done-review-rubric.md`** (the
  code-review rubric both gates share), returning verdict, findings with `file:line`, dispositions; the
  cross-shard subagent uses the same rubric over the seams list. The human may substitute a richer lens set;
  the default cost is packs + 1 per plan and 1 + 1 per fix round.
- `skills/review-pr-ready/SKILL.md`, `skills/review-task-done/SKILL.md`, `skills/status/SKILL.md`,
  `commands/review-pr-ready.md`, `commands/review-task-done.md`, `commands/status.md`,
  `docs/README.claude.md`, `docs/README.codex.md`, `docs/quickstart.md`, `AGENTS.md`, `CLAUDE.md`,
  `README.md`, `INSTALL.md`: sharded-review section; durable += `docs/metareview/shards/`; transient +=
  `.metareview/shards/`; the must-stay-untracked amendment (§4.3); commit-after-pass rule; `--max-attempts`
  advice; "do not open a new chain to dodge escalation".
- `learnsource.Collect` excludes `docs/metareview/shards/**` only.
- Run rows: `prready`/`taskdone` `runRecord` writers and `runchain.Record` gain `planHash` (`omitempty`)
  and `runchain.Record` gains `createdAt` (already written by every writer); shard hashes live in
  `plan.json`, not the row (row size stays bounded); `schemaVersion` stays 1. `runchain.ReadRuns`,
  `state.ReadJSONL` and `reviewlog.readFindings` get a 1 MiB scanner buffer (matching the 0.9.0 reader).
- Release: five version files → 0.8.3; CHANGELOG with §11.
- Coverage: `tests/go/test-shardpack-coverage.sh` — `go test -coverprofile` must exit 0, the profile must be
  non-empty, and the count of profile blocks with zero hits must be 0; registered in `tests/run-all.sh` with
  `tests/go/test-sharded-review.sh`.

## 8. Tests (write first; named)

- (gitcontext) `TestBranchFilesMeasureUntruncatedFilteredDiff` (raw bytes; `Σ Bytes == BranchFilteredDiffBytes`
  measured independently with identical flags/excludes), `TestBranchFilesRenameDeleteBinaryModeOnly`,
  `TestBranchFilesExcludeGenerated`, `TestBranchFilesLiteralPathspecs` (content asserted per exotic path),
  `TestBranchFilesDefeatDiffAttributes`, `TestSubmoduleChangeReason`, `TestBranchFilesOnlyWhenTruncated`,
  `TestRunGitErrorBranchesViaOptions`, `TestAddedLinesUnionUsesFullBranchDiff`, `TestContextDiffJSONShapeUnchanged`,
  `TestUnsafePathReasonIncludesFormatChars`.
- (contextprofile) `TestSourceDiffHashFromDocumentedPreimage`, `…OrderIndependent`, `…ChangesOnSameSizeEdit`,
  `…InvariantUnderMetareviewWrites`, `…IgnoresLocalChanges`, `TestNewlinePathCannotCollide`,
  `TestBucketPlanIsContentStable` (same-size edit → one shard changes), `TestGrowthSplitsOnlyItsBucket`,
  `TestDeletionMergesOnlyItsSubtree`, `TestChunkNeverExceedsBudget` (200 KB single line),
  `TestChunksOfOneFileShareBucket`, `TestTooManyShardsReason`, `TestLocalReasonsSplit`, `TestLargeDiffTotalKept`,
  `TestPlanEmptyWhenNotTruncated`, `TestDiffOversizeReason`, `TestBudgetPlusOverheadUnderCap`.
- (reviewmanifest) `TestManifestHashIsPlanHash`, `…ExcludesGeneratedPathsAndDispositions`,
  `TestShardResultFreshByHash`, `TestUnmatchedResultIgnoredNotBlocking`, `TestFilenameIdMismatchBlocks`,
  `TestDuplicateShardResultBlocks`, `TestCopiedResultNeverCoversAnotherShard`, `TestCrossShardFreshByPlanHash`,
  `TestCrossShardRequiredOnlyForMultiShard`, `TestUnknownPackRunIgnored`, `TestCarriedResultViaShardHashes`,
  `TestReviewedBeforePackIgnoredSecondGranularity`, `TestWaivedMediumFindingBlocks`, `TestFixedAndFalsePositiveClose`,
  `TestSelfClosedHighCriticalListed`, `TestEvidenceRuleMatchesValidator`, `TestResultSchemaVersionDistinct`,
  `TestIdentifierPatternRejected`, `TestCapsIgnoreNotBlock`, `TestLocalFilesNeverBlockAssignment`,
  `TestChunkAssignedToExactlyOneShard`, `TestMarkdownPlainTextStripsFormatCharsAndEscapesPipes`.
- (shardpack) `TestOwnershipCheckIcaseAnyModeNestedRepoIgnored`, `TestIngestionRefusedWithoutOwnership`,
  `TestSelfIgnoreCreatedAndPreserved`, `TestLayoutContainmentAndSlug`, `TestReplaceKeepsOldUntilNewInPlace`,
  `TestRollbackRestoresAside`, `TestPruneOnlySiblingHexDirsOfSameTarget`, `TestPackUsesMeasuredBytes`,
  `TestUntrustedMarkersWithNonce`, `TestOSDepsNonceUsesCryptoRandAndErrorBranch`, `TestHeaderPathsInlineCodePlainText`,
  `TestCrossShardPackListsChunkedFiles`, `TestDiscoverByHashNamesAndReasons`, `TestExplicitResultsAdded`,
  `TestUnreadableDiscoveredIsBlocker`, `TestResultTooLargeIgnored`, `TestOverCapIgnoredLexical`,
  `TestResultOutsideRepoRejected`, `TestGCAfterPass`, one test per `Deps` failure branch.
- (reviewers) `TestContextRiskSatisfiedEmitsAdvisoryAndRunsLints`, `…NotSatisfiedByEmptyManifest`,
  `…NotSatisfiedWithMissingShard`, `TestMixedReasonsNeverSatisfied`, `TestEachNeverSatisfiableReason`,
  `TestTruncatedDiffFindingAdvisoryOnSatisfiedPath`, `TestTestsMissingFingerprintStable`,
  `TestContextRiskFingerprintReasonIndependentAllScopes`, `TestCoveredFingerprintStable`,
  `TestStagedEvalStillFoundOnSatisfiedPath`.
- (prready, taskdone) `TestBranchOnlyContextClearsUntrackedAndKeepsMeasuredBytes`, `TestPlanHashIdenticalAcrossPackDirContextPackRunRowManifest`,
  `TestPackWriteFailureRollsBackRun`, `TestFailureAfterPackRenameKeepsOldPacks`, `TestRunRowRecordsPlanHashAndCreatedAt`,
  `TestAttestationSectionAfterVerdictParses`, `TestTaskDoneWithDocsMetareviewTarget`, `TestTwoTargetsDoNotPruneEachOther`,
  `TestKnownPackRunsFromChainOnly` — via `Options.ShardWriter` and `gitcontext.Options.RunGit`.
- (findings, runchain, state, reviewlog) `TestLegacyContextRiskRowSuperseded`, `TestLegacyTestsMissingSupersededOnlyWhenEvaluated`,
  `TestSupersedeBackupWritten`, `TestRecordTolerantOfPlanFields`, `TestReadersAcceptOneMiBLines`.
- (learnsource) `TestCollectExcludesShardResults`.
- Shell `tests/go/test-sharded-review.sh` (deterministic 300 KB lint-clean fixture incl. one file > budget, an
  untracked 5 KB file, no `TODO`/`eval(`): pr-ready → `NEEDS_REVISION`, packs written, blockers name every
  shard → the test reads `plan.json`, writes passing results for every shard + cross-shard →
  `--previous-run` → `PASS_ADVISORY`, zero blockers, context-risk row `fixed`, attestation lists the chunked
  file → **fix round**: same-size edit in one file, commit, re-run (`--previous-run`) → exactly that shard and
  the cross-shard listed as ignored `no current shard`/`no current plan`, all others fresh (per-category
  asserted); write a result for the changed shard + cross-shard → `--previous-run` → `PASS_ADVISORY`, untouched
  shards carried without new files, GC removed the two superseded files → foreign run id → `ignored: unknown
  pack run` → `--shard-result` to a missing path → exit 2, `runs.jsonl` byte-identical → three runs on
  unchanged content → one plan hash, packs identical after normalisation → a tracked `.METAREVIEW/x` →
  `METAREVIEW_DIR_TRACKED`, nothing ingested. Then task-done with a task file, a staged `eval(` (still blocks)
  and an untracked file (never sharded). `tests/go/test-shardpack-coverage.sh` per §7.

## 9. Cost, honestly

PR #13 (`main…fsm-enhancements`, excludes applied): 1,372,619 bytes / 133 files → ≈ 25–30 bucket shards at
the default budget (bucket splitting is coarser than first-fit; exact count is a test output), one subagent
each plus one cross-shard per plan; one shard + cross-shard per fix round. `machine_test.go` (86,974 bytes)
becomes two chunk shards.

## 10. epic-ready (recorded honestly)

`RunEpicReady` returns the context-risk blocker unconditionally from its own profile; after 0.8.3 an epic
whose parent range exceeds 120 KB **stays blocked** — no workaround. Its fingerprint becomes
reason-independent and its context pack advertises no pack paths. Ingestion is a follow-up.

## 11. Upgrade (not a no-op)

- Hashes `v4`, content-derived; nothing persisted depended on old values. `context diff` JSON unchanged.
- Fingerprints reason-independent in all scopes; `tests:missing` hashed. First 0.8.3 run for a target: copy
  `.metareview/findings.jsonl` to `.metareview/findings.jsonl.pre-0.8.3` once, then alias-supersede open rows
  with the legacy prefixes `architecture:context-risk:`, `pr:architecture:context-risk:`, `epic:context-risk:`
  → status `superseded` (readers treat unknown statuses as not-open; learning ignores them); a legacy
  `tests:missing:` row (value not exactly 16 lowercase hex) is superseded **only** when the run actually
  evaluated that class (a new `tests:missing:<hex>` fingerprint is in the current set or the non-early-return
  path ran). `docs/metareview/FINDINGS.md` is re-rendered accordingly. Works without `--previous-run` and for
  escalated chains (tests per state).
- New risk reasons: `LOCAL_DIFF_TRUNCATED`, `LOCAL_LARGE_DIFF`, `DIFF_OVERSIZE`, `TOO_MANY_SHARDS`,
  `UNSAFE_PATH`, `SUBMODULE_CHANGE`, `METAREVIEW_DIR_TRACKED`.
- `runs.jsonl` rows gain `planHash`; reader gains `createdAt`; readers accept 1 MiB lines.
- New transient `.metareview/shards/`; new durable `docs/metareview/shards/`; committed results are audit
  records verifiable only while `base..head` is reproducible; other clones ignore them.
- Contract amendment: `.metareview/runs.jsonl`, `findings.jsonl`, `shards/` must remain untracked.
- `learn-post-merge` excludes `docs/metareview/shards/**`. WU4 criteria superseded (§6).

## 12. Landing plan

Two PRs, each gated by the existing task-done/pr-ready under the cap (never self-sharded; split further if
either exceeds 120 KB):
- **PR-A** (measurement and packs): §3, §4.1, §4.2, §4.3 writer/prune/ownership, `test-shardpack-coverage.sh`,
  the context-pack rendering of the plan and pack paths. No ingestion; behaviour change limited to true
  sizes, new reasons and packs being written.
- **PR-B** (ingestion and gate): §5, §6, §7 docs/skills, §11 migration, `test-sharded-review.sh`, version
  bump 0.8.3. `--shard-budget` deferred beyond PR-B.

## 13. Attempts 1–2 → r2/r3 (kept for the record)

r2: content-derived keys, reason split, provenance, explicit satisfaction rule, dispositions closed, second
blocker downgraded, fenced untrusted region, transient packs. r3: shard-keyed results, chain-bound
provenance, byte-range chunks, literal per-path diffs with `--text`, union lints, no waiver channel,
ownership check, attestation wording, minimal coverage check.

## 14. Attempt-3 blockers → r4 changes

| Blocker (lens) | r4 |
|---|---|
| First-fit re-cuts every later shard; fix loop cannot converge (Feasibility, Intent) | §4.2 content-stable bucket trie keyed by path hash; ids from prefixes; per-bucket effect only |
| Identity rule contradicts hash freshness; carried results become blockers (Feasibility, Architecture) | §5.1 attribution by hash; id check = file vs filename only; stable ids make current id equal by construction |
| Stale vs ignored contradiction (Testing) | §3/§5.1 no "stale"; unmatched → ignored with reason; oracles restated |
| Ownership check bypassed by icase / gitlink; carried clause chain-independent (Security) | §4.3 `':(icase).metareview'` any mode, nested-repo and check-ignore checks, never-satisfiable reason + ingestion refused; §5.1 chain-only `KnownPackRuns` |
| "Packs identical" false; RemoveAll-then-rename destroys packs (Testing, Architecture, Feasibility) | §4.3 normalised oracle; rename-aside/in/remove; rollback restores aside |
| Chunk > budget on long lines; budget vs pack overhead; chunk-level guarantee unstated (Testing, Feasibility, Intent) | §4.2 hard cut; §3 budget ≤ 100000 + overhead test; §2(h)/§5.3 attestation lists chunked files |
| `KnownPackRun.At`/`ShardHashes` have no data path; row size unbounded; scanner 64 KiB (Architecture, Completeness, Data-mig, Feasibility) | §5.1 type; §7 `createdAt` on `Record`, shard hashes in `plan.json`, 1 MiB scanner, ≤ 999 shards |
| `PlanShards` has no diff producer; hashes undefined on small runs; dead disjunct (Architecture) | §4.2 signature; "not sharded" when nil; §4.1 dropped clause |
| Untracked reasons unhandled; `LARGE_DIFF` weakened (Completeness) | §4.2 taxonomy: total `LARGE_DIFF` kept, `LOCAL_LARGE_DIFF`, untracked cleared on branch-only path |
| `tests:missing` alias clears a live blocker; no backup (Data-mig) | §11 supersede only when evaluated; discriminator; backup file |
| Per-pack review undefined; cost understated (Scope) | §7 one subagent per pack against the task-done rubric; §9 restated |
| Ownership exit 1 contradicts the documented contract (Scope) | §4.3 risk reason instead of exit; contract amended in docs |
| No landing plan; `--shard-budget` optional (Scope) | §12 two PRs; budget deferred |
| Escalation rationale false (Intent) | §6 stated plainly; `--max-attempts` on the first run; lock is a follow-up |
| Unexported `runGit` seam; writer seam undeclared; OSDeps nonce uncoverable (Architecture, Testing) | §4.1 `Options.RunGit`; §4.3 `Options.ShardWriter`, `randRead` |
| `<target-slug>` undefined (Completeness) | §4.3 definition with hash suffix |
| Results retention; 65th-file blocker (Architecture, Security) | §5.2 GC after pass; cap = 2×(ShardCount+1), over cap ignored |
| Submodules; Unicode format chars; nonce entropy; ordering invariant (Security) | §3 `SUBMODULE_CHANGE`; `UNSAFE_PATH` + `PlainText` Cf/Zl/Zp; `RandRead`; §4.3 ordering |
| Docs list incomplete; WU4 line numbers; leftover question; raw-vs-trimmed bytes; second-granularity time; base/head in results; `AddedLines` ownership (various) | §7 list; §6 quoted criteria; §4.2 cleaned; §3 raw bytes; §5.1 seconds; §5.1 `base`/`head`; §4.1 union stays in `reviewers` |
