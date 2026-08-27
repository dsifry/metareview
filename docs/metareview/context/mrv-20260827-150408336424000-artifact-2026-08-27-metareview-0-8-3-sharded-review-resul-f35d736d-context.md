# metareview context: docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md

Run ID: `mrv-20260827-150408336424000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

## Target

- Path: `docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md`
- Repository mode: `metaswarm-extension`
- Git branch: `pr-ready-shard-results`
- Git head: `ef23776`

## Artifact Excerpt

```markdown
# metareview 0.8.3 — sharded review results (r3)

Status: r3 after attempts 1 and 2 of the adversarial review (both `NEEDS_REVISION`; §13 maps every attempt-2
blocker to its r3 change). Branch `pr-ready-shard-results` off `main` (0.8.2). Ships as **0.8.3** on `main`;
`fsm-enhancements` (0.9.0, under manual QA) rebases onto it and PR #13 is re-gated.

## 1. Problem

`metareview review pr-ready` (and `task-done`, `epic-ready`) cannot pass on a branch whose diff exceeds the
120 KB context cap, and the tooling designed for that case is half-built:

1. `internal/gitcontext` truncates every diff at `maxDiffBytes = 120000` and sets `DiffTruncated`; the
   context profile turns that into risk reason `DIFF_TRUNCATED`; the architecture-reviewer turns any
   context risk into a **blocking** "Review context risk" finding (`internal/reviewers/taskdone.go:66`,
   `prready.go` via `branchDiffFindings`, `epicready.go:54`). Nothing downstream can clear it.
2. The Context Shard Plan is computed from the **truncated** diff (`prready.branchOnlyGitContext` sets
   `RawDiffBytes = FilteredDiffBytes = len(git.Diff)`, `internal/prready/review.go:595-596`;
   `contextprofile.filesFromGit` sizes files by scanning `git.Diff`). On the 0.9.0 branch the plan reported
   "shard-01: 120 files, 59,967 bytes"; the exclude-filtered diff is in fact 1,372,619 bytes over 133 files.
3. Every shard names a prompt pack `docs/metareview/shards/<hash>-<id>.md` that is never written.
4. `reviewmanifest.Manifest` has `ShardResults`/`CrossShardResult` with a tested aggregator (WU4), but
   nothing populates them and no reviewer consults the manifest verdict.
5. The deterministic lints scan the truncated diff only.
6. Both existing freshness keys are unusable: `sourceDiffHash` hashes byte counts only
   (`internal/contextprofile/shards.go:148-158`) and `sourceManifestHash` folds in generated paths that grow
   with every run (`internal/reviewmanifest/manifest.go:355-373`); the mechanism deadlocks on itself.

WU4's boundary is kept: the CLI models and aggregates externally produced shard results; it never spawns a
reviewer.

## 2. Goals / non-goals

Goals: (a) content-derived, per-shard freshness keys, invariant under metareview's own writes and under
local (staged/working-tree/untracked) changes; (b) prompt packs written from the very bytes that were
measured; (c) a closed, fully specified result-file format that the reviewing host writes per shard, bound
to the local pack-writing run through the existing `--previous-run` chain, discovered by shard identity
and validated deterministically; (d) the context-risk blocker is *satisfied* — not suppressed — when the
manifest aggregate passes, with every finding that becomes reachable on that path stated; (e) lints cover
the whole branch diff plus local changes, as today; (f) an end-to-end shell test that drives a >120 KB
branch to `PASS_ADVISORY` with zero blockers, then through a one-shard fix (only that shard's result and the
cross-shard result go stale), a same-size edit, a foreign result and a bad explicit path; (g) no file is ever
unreviewable by size: files larger than the shard budget are split into byte-range sub-shards; (h) a memory
bound — the exclude-filtered branch diff over 16 MiB (`DIFF_OVERSIZE`) is a declared, never-satisfiable
class (the branch must be split); (i) `internal/shardpack` at exactly 100% statement coverage, enforced
by a check that exists on `main`.

Non-goals: raising or parameterising `maxDiffBytes`; spawning reviewers from Go; changing the artifact
review flow; `epic-ready` ingestion (§10); satisfying context risk raised by staged, working-tree or
untracked content; a human waiver mechanism for shard findings (§5.4 says what exists instead); porting
the 0.9.0 combined coverage gate to `main` (it stays with 0.9.0 — §7 uses a minimal check instead).

## 3. Freshness keys (normative)

**Diff commands.** Let *E* be the caller's effective generated-path excludes (`generatedMetareviewPath
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
