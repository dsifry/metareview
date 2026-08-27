# metareview context: docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md

Run ID: `mrv-20260827-154745525699000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

## Target

- Path: `docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md`
- Repository mode: `metaswarm-extension`
- Git branch: `pr-ready-shard-results`
- Git head: `0562529`

## Artifact Excerpt

```markdown
# metareview 0.8.3 — sharded review results (r6)

Status: r6 = r5 (the simplification pass) plus the fixes from a three-lens verification of r5 (§13).
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
- `sourceDi
```

## Service Inventory

No service inventory found.

## Knowledge Facts

No Beads knowledge facts found.

## Suggested Reviewers

- feasibility
- completeness
- scope/alignment
- architecture
- intent preservation
