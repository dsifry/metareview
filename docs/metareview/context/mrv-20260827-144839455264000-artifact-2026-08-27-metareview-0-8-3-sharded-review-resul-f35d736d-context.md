# metareview context: docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md

Run ID: `mrv-20260827-144839455264000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

## Target

- Path: `docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md`
- Repository mode: `metaswarm-extension`
- Git branch: `pr-ready-shard-results`
- Git head: `a0c925d`

## Artifact Excerpt

```markdown
# metareview 0.8.3 — sharded review results (r2)

Status: r2 after attempt 1 of the adversarial review (all eight lenses `NEEDS_REVISION`; every blocker is
addressed below and the change is named in §12). Branch `pr-ready-shard-results` off `main` (0.8.2). Ships
as **0.8.3** on `main`; `fsm-enhancements` (0.9.0, under manual QA) rebases onto it and PR #13 is re-gated.

## 1. Problem

`metareview review pr-ready` (and `task-done`, `epic-ready`) cannot pass on a branch whose diff exceeds the
120 KB context cap, and the tooling designed for that case is half-built:

1. `internal/gitcontext` truncates every diff at `maxDiffBytes = 120000` and sets `DiffTruncated`; the
   context profile turns that into risk reason `DIFF_TRUNCATED`; the architecture-reviewer turns any
   context risk into a **blocking** "Review context risk" finding (`internal/reviewers/taskdone.go:66`,
   `prready.go` via `branchDiffFindings`, `epicready.go:54`). Nothing downstream can clear it.
2. The Context Shard Plan is computed from the **truncated** diff: `prready.branchOnlyGitContext` sets
   `RawDiffBytes = FilteredDiffBytes = len(git.Diff)` (`internal/prready/review.go:595-596`) and
   `contextprofile.filesFromGit` sizes files by scanning `git.Diff` text. On the 0.9.0 branch (≈976 KB of
   code) the plan reported "shard-01: 120 files, 59,967 bytes".
3. Every shard names a prompt pack `docs/metareview/shards/<hash>-<id>.md` that is never written.
4. `reviewmanifest.Manifest` has `ShardResults`/`CrossShardResult` with a tested aggregator (WU4,
   `docs/superpowers/plans/2026-07-05-issue-2-wu4-review-manifest-shard-aggregation.md`), but nothing
   populates them and no reviewer consults the manifest verdict.
5. The deterministic lints (`eval(`, `TODO|FIXME`, inventory paths) scan the truncated diff only.
6. **Both freshness keys are unusable as designed** (attempt 1, verified empirically by three lenses):
   `sourceDiffHash` hashes byte *counts* only (`internal/contextprofile/shards.go:148-158`), so a
   same-size edit keeps it; and `sourceManifestHash` folds in `GeneratedExcludedPaths` + dispositions
   (`internal/reviewmanifest/manifest.go:355-373`), which grow with every review's own outputs — three
   consecutive `pr-ready` runs on an unchanged branch produced three manifest hashes. Any result written
   against run N is "stale" at run N+1: the mechanism deadlocks on itself. The unfiltered `raw=` byte count
   in `sourceDiffHash` drifts the same way once review artifacts are committed.

WU4's boundary is kept: *"Keep agent execution out of the Go CLI. The CLI should model and aggregate
externally produced shard results, not spawn Codex, Claude, or other runtimes."*

## 2. Goals / non-goals

Goals: (a) content-derived, per-shard freshness keys that are invariant under metareview's own writes;
(b) prompt packs actually written, from the same bytes that were measured; (c) a closed result-file
format the reviewing host writes per shard, bound to the local run that produced the pack, discovered and
validated deterministically; (d) the context-risk blocker is *satisfied* — not suppressed — when the
manifest aggregate passes, and no other finding that was unreachable behind it re-blocks the satisfied
path unexpectedly; (e) lints cover the whole branch diff; (f) an end-to-end shell test that drives a
>120 KB branch from `NEEDS_REVISION` to `PASS_ADVISORY` (zero blockers) with shard results, then through a
one-shard fix (only that shard's result goes stale), a same-size edit (stale), and a foreign result file
(ignored); (g) the coverage gate from the 0.9.0 branch ported to `main` so (a)–(f) are enforced.

Non-goals: raising or parameterising `maxDiffBytes`; spawning reviewers from Go; changing the artifact
review flow; `epic-ready` ingestion (§10); satisfying context risk raised by staged, working-tree or
untracked content (§6).

## 3. Freshness keys (normative; everything else depends on this section)

Let *B* be the exclude-filtered branch diff: 
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
