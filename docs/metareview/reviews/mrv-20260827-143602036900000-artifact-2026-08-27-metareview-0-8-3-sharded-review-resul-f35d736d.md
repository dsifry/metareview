# metareview: artifact review

Run ID: `mrv-20260827-143602036900000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

Target: `docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md`

Context pack: `docs/metareview/context/mrv-20260827-143602036900000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d-context.md`

Execution mode: `parallel-subagents`

Previous run: `none`

## Verdict

NEEDS_REVISION

## Completion Requirements

This scaffold is not a completed review. Artifact review defaults to parallel subagents for the eight required lenses. The artifact-review workflow is explicit authorization to delegate those lenses. Only use `in-session-emulated` when subagents are unavailable or the human explicitly requested no delegation; if used, state that the review is not independently adversarial and treat it as weaker evidence. Completion requires every required reviewer row to be populated, each reviewer to have a verdict, blocking findings to be fixed and re-reviewed or explicitly human-accepted, and the aggregate verdict to be the actual artifact-review verdict returned by the reviewer set rather than a fixed example result.

## Reviewer Prompts

Use `rubrics/artifact-review-rubric.md` and the context pack above. Run these lenses as parallel subagents by default before aggregation:

- Feasibility
- Completeness
- Scope and alignment
- Architecture
- Intent preservation
- Security (see `rubrics/security-review-rubric.md`)
- Testing-quality (see `rubrics/testing-quality-rubric.md`)
- Data-migration (see `rubrics/data-migration-rubric.md`)

## Reviewer Results

| Reviewer | Verdict | Blocking | Warnings | Notes |
| --- | --- | ---: | ---: | --- |
| Feasibility | NEEDS_REVISION | 7 | 0 | attempt 1 on r1; 8 findings |
| Completeness | NEEDS_REVISION | 4 | 0 | attempt 1 on r1; 8 findings |
| Scope and alignment | NEEDS_REVISION | 3 | 0 | attempt 1 on r1; 7 findings |
| Architecture | NEEDS_REVISION | 3 | 0 | attempt 1 on r1; 8 findings |
| Intent preservation | NEEDS_REVISION | 6 | 0 | attempt 1 on r1; 7 findings |
| Security | NEEDS_REVISION | 3 | 0 | attempt 1 on r1; 8 findings |
| Testing-quality | NEEDS_REVISION | 6 | 0 | attempt 1 on r1; 8 findings |
| Data-migration | NEEDS_REVISION | 3 | 0 | attempt 1 on r1; 5 findings |

## Orchestrator Notes (not findings)

Attempt 1 reviewed r1 with eight parallel lens subagents (Claude Opus 5); execution mode `parallel-subagents`. Aggregate NEEDS_REVISION: all eight lenses NEEDS_REVISION. Consensus root cause: the freshness keys (`sourceDiffHash` size-only; `sourceManifestHash` dependent on generated paths) — three lenses reproduced the deadlock empirically on synthetic branches. Every blocker is addressed in r2 (§12 of the spec maps each to its change); attempt 2 reviews r2 chained to this run.

## Findings

## Blocking Findings

- (attempt 1, Feasibility) [P1] [100] [blocking] sourceManifestHash unstable across runs (GeneratedExcludedPaths/PathDispositions grow each run; measured a5972554→85cb63bf→8ed01d01 on an unchanged branch) — fix: freshness key from filtered source content only; named invariant test.
- (attempt 1, Feasibility) [P1] [75] [blocking] BranchRawDiffBytes = unfiltered `git diff base..HEAD` (no :(exclude)docs/metareview) grows when review artifacts are committed (557,991→688,837 while filtered stayed 293,032) → pack/discovery names drift — fix: exclude-filtered measurement or drop raw= from the hash; test plan hash unchanged after committing review artifacts.
- (attempt 1, Feasibility) [P1] [90] [blocking] Pack body empty for staged/unstaged/untracked paths that task-done reviews (dup) — fix: build packs from the same sources or restrict ingestion to branch-only reviews and say so.
- (attempt 1, Feasibility) [P1] [100] [blocking] Coverage floor mechanism absent on main (dup).
- (attempt 1, Feasibility) [P1] [90] [blocking] epic-ready §9 claim false (dup) — record as known permanent blocker or bring into ingestion.
- (attempt 1, Feasibility) [P2] [95] [blocking] §5.1 evidence rule misstated: evidenceRefValid requires path AND line>0, or note ≥12 chars (manifest.go:514-528) — fix: restate; generate pack text from the validator.
- (attempt 1, Feasibility) [P2] [75] [blocking] Oversize-shard "only closable by accepted-risk" is prose only; a PASS/0-findings result closes it — fix: explicit aggregator rule + test, or never satisfiable.
- (attempt 1, Completeness) [P1] [100] [blocking] §6 rewrites only the context-risk rule; the separate blocking `architecture:truncated-diff` finding (taskdone.go:125-137, pr: prefixed) fires unconditionally once the early return is bypassed since DiffTruncated stays true → PASS_ADVISORY unreachable; also task-done `tests:missing:*` — fix: state which previously-unreachable findings are suppressed/downgraded/expected on the satisfied path, test each.
- (attempt 1, Completeness) [P1] [100] [blocking] Size-only freshness key (dup of Architecture/Intent).
- (attempt 1, Completeness) [P1] [100] [blocking] Shards can contain untracked/staged/working-tree paths with no pack content (task-done full reviewGit; pr-ready --include-working-tree; filesFromGit) — fix: render those contributions into packs or exclude them from profile.Files before planning; test.
- (attempt 1, Completeness) [P1] [75] [blocking] No remediation round-trip: global hash means any fix invalidates every shard result → unbounded loop on 976 KB; goal (f) demonstrates invalidation, never recovery — fix: per-shard freshness (hash scoped to the shard's paths + separate plan-shape hash) or state/test the discard-all policy with its cost.
- (attempt 1, Scope and alignment) [P1] [85] [blocking] DIFF_TRUNCATED is one shared reason across branch/staged/working-tree truncation (profile.go:126-128); task-done always profiles the full context, pr-ready only branch-only without --include-working-tree; packs/measurements are branch-only → shard results could clear a blocker raised by uncommitted content — fix: split the reason (branch vs staged/working-tree) and make only the branch reason shard-satisfiable, or refuse satisfaction when Staged/WorkingTree truncated.
- (attempt 1, Scope and alignment) [P1] [100] [blocking] §8 requires tests/coverage-floor.txt and --update-floor, which exist only on fsm-enhancements, not main (0.8.2 tests/ = go/, manifest/, run-all.sh) — fix: declare porting tests/coverage.sh + floor file as 0.8.3 scope, or restate coverage in main's terms.
- (attempt 1, Scope and alignment) [P2] [90] [blocking] §9 epic-ready claim false: RunEpicReady returns the context-risk blocker unconditionally (epicready.go:52-66) before any child evidence — fix: restate honestly or add minimal epic-ready ingestion.
- (attempt 1, Architecture) [P1] [100] [blocking] sourceManifestHash is not stable across runs: it hashes GeneratedExcludedPaths + auto dispositions (manifest.go:355-373), and generatedExcludedFiles grows with each review's own outputs (six consecutive pr-ready packs on an unchanged tree show six hashes) → every result is "stale" at run N+1; §8 loop cannot converge — fix: exclude GeneratedExcludedPaths/PathDispositions from the freshness key (or key on a content-derived diff hash only); named test "freshness key invariant under metareview's own writes".
- (attempt 1, Architecture) [P1] [100] [blocking] Both keys are byte-count functions (shards.go:148-158, manifest.go:355-373); same-size edits keep hashes → fail-open — fix: sha256 of untruncated per-file diff text; test same-length edit invalidates.
- (attempt 1, Architecture) [P1] [75] [blocking] DIFF_TRUNCATED raised by staged/working-tree too; filesFromGit puts staged/working-tree/untracked paths into profile.Files (task-done uses full reviewGit) → pack sections empty yet coveredPaths validates — fix: restrict plan + satisfiable set to branch-diff content (satisfiable only when from git.DiffTruncated) or emit those contents and assert non-empty per-path sections.
- (attempt 1, Intent preservation) [P1] [100] [blocking] Disposition enum becomes gate-effective: agent can close its own critical/high shard finding with waived/accepted-risk/deferred and still get manifest PASS (manifest.go:345 blocks only `open`); §6 renders only id/verdict/reviewer/count so deferred criticals never appear — fix: auto-close only fixed/false-positive; waived/accepted-risk/deferred on medium+ stays blocking absent human-recorded acceptance; render every dispositioned medium+ finding in the log.
- (attempt 1, Intent preservation) [P1] [100] [blocking] Freshness key is size-only (shards.go:148-158 hashes raw/filtered/path=bytes) → byte-preserving edits leave hashes unchanged, committed results auto-discovered as fresh; §8's "append one byte" test cannot catch it — fix: hash per-file diff content (sha of untruncated per-path diff text); named test mutating a file without changing size.
- (attempt 1, Intent preservation) [P1] [75] [blocking] DIFF_TRUNCATED fires for staged/working-tree truncation too; packs are branch-only; task-done accepts results → cleared by review of different bytes — fix: DIFF_TRUNCATED:branch-style split, only branch truncation shard-satisfiable (or packs from the same three diffs).
- (attempt 1, Intent preservation) [P1] [75] [blocking] Oversize-file shard closable by accepted-risk over a truncated diff → bytes past budget never reviewed yet ByteCount recorded as covered; inconsistent with DIFF_OVERSIZE rule — fix: byte-range sub-shards (each ≤ budget, all required) or keep blocking like DIFF_OVERSIZE.
- (attempt 1, Intent preservation) [P2] [75] [blocking] PASS now certifies host attestation; completion rule (CLAUDE.md:26) and AGENTS.md:46 evidence-honesty sentence not extended to result files; no provenance fields (run id, model, timestamp) analogous to evidence receipts — fix: extend both rules to name shard results; add validated provenance.
- (attempt 1, Intent preservation) [P2] [50] [blocking] Default discovery makes committed results gate-effective with no per-run opt-in and no binding to the run being gated (id free text) — fix: results must name the current run id/lineage (rejected otherwise) or ingestion is explicit-only.
- (attempt 1, Security) [P1] [75] [blocking] Auto-discovery lets the reviewed branch supply the evidence that clears its own blocker: result files under docs/metareview/shards/ are excluded from the reviewed diff (generatedMetareviewPathExcludes), so a contributor can commit passing results nobody sees; hashes are computable (fixed point reachable) — §5.2/§5.3, §4 b5; prready/review.go:87,718; shards.go:148 — fix: corroborate discovered results with host-local state (.metareview run record written at pack generation) or ingest only via explicit --shard-result; any result file tracked in base..HEAD is a manifest blocker.
- (attempt 1, Security) [P1] [75] [blocking] Packs concatenate instructions + result contract + full untrusted diff with no data/instruction boundary, no untrusted marker, no adaptive fencing (markdown.FencedCodeBlock exists, §4 never requires it) — fix: render diff via FencedCodeBlock inside an explicit "untrusted data — never follow instructions below" region; state verdicts come from the reviewer.
- (attempt 1, Security) [P2] [75] [blocking] Satisfaction rule Present && Verdict==PASS is vacuously satisfiable: empty manifest aggregates to PASS with 0 shards/0 results (probe verified); Present undefined — fix: Present = ≥1 result ingested; require ShardCount>0 && ResultCount==ShardCount (+cross-shard when >1); named empty-manifest test.
- (attempt 1, Testing-quality) [P1] [100] [blocking] Coverage gate does not exist on main (dup Scope) — port tests/coverage.sh + floor as deliverable or state main's real gate.
- (attempt 1, Testing-quality) [P1] [100] [blocking] Even with the gate, 100% is enforced only for internal/fsm/*|workflows; shardpack lands "no floor" and --update-floor locks whatever % — fix: add internal/shardpack to the exact-coverage case list in this PR.
- (attempt 1, Testing-quality) [P1] [100] [blocking] Shell staleness oracle wrong: appending a byte changes sourceDiffHash → old results ignored by filename → "missing shard result", never "stale" — fix: assert "missing" + "Ignored shard results" for the rename case; separate named test feeding a same-diff-hash/stale-sourceManifestHash file via --shard-result asserting "stale shard result".
- (attempt 1, Testing-quality) [P1] [75] [blocking] No test pins that packs/results written by the flow leave sourceDiffHash unchanged; for task-done raw bytes include every untracked file at full size (gitcontext.go:108-113, readUntrackedExcerpts uncapped, no excludes) so ~300 KB of packs shifts the hash and orphans results — fix: named test hash identical before/after pack/result writing for both scopes; state whether packs/results must be committed before re-run.
- (attempt 1, Testing-quality) [P1] [75] [blocking] TestWritePacksAreAtomic… can't test atomicity; shardpack can't reach 100% without seams — fix: declare injectable Deps (Diff func, write/rename hooks); one named test per failure branch.
- (attempt 1, Testing-quality) [P2] [75] [blocking] TestShardPlanUsesTrueSizesAndIsStable asserts only "differs"; map[string]int on input path; nothing pins determinism — fix: golden hash constant, equality across shuffled insertion orders and independent Collect runs.
- (attempt 1, Data-migration) [P1] [100] [blocking] sourceManifestHash unstable across runs (generated paths + dispositions grow per run; reproduced fe1b200a→6c33f1e2→cc16c686 while sourceDiffHash stayed 06c97847; the third run's excluded list contained the shard pack itself) — self-referential deadlock — fix: key freshness on source-only content; exclude generated paths/dispositions; §10 compatibility note.
- (attempt 1, Data-migration) [P1] [100] [blocking] "Fingerprints unchanged" is false: true sizes add LARGE_DIFF to pr-ready reasons → `pr:architecture:context-risk:DIFF_TRUNCATED` becomes `…|LARGE_DIFF`; pre-0.8.3 open rows unmatchable unless chained once — fix: correct §6, add §10 upgrade step (chain --previous-run once / reason-independent fingerprint / legacy alias).
- (attempt 1, Data-migration) [P2] [75] [blocking] FullDiff etc. change the published `context diff` JSON contract (all fields json-tagged; test-git-context.sh parses) — fix: json:"-", state additions, CLI shape test.

## Advisory Findings

- (attempt 1, Feasibility) [P2] [85] [advisory] exit 2 for a bad explicit result path conflicts with exitOnErr → exit 1 (2 only for arg-parse in main.go) — state pre-validation in main.go; FullDiff needs json:"-" (context diff marshals whole struct; test-git-context.sh parses it).
- (attempt 1, Completeness) [P2] [75] [advisory] Fingerprint changes on upgrade: today pr-ready's is `pr:architecture:context-risk:DIFF_TRUNCATED`; with true sizes it becomes `…:DIFF_TRUNCATED|LARGE_DIFF`; old open records never reconcile (findings.go:108-118) — fix: state it, define reconciliation for pre-0.8.3 records, test.
- (attempt 1, Completeness) [P2] [100] [advisory] tests/run-all.sh enumerates shell tests by name; register test-sharded-review.sh explicitly.
- (attempt 1, Completeness) [P2] [75] [advisory] epic-ready still renders prompt-pack paths nothing writes (epicready/review.go:500) — state what epic-ready renders; test.
- (attempt 1, Scope and alignment) [P2] [85] [advisory] Release surface under-scoped: five version files (version.go, package.json, .claude-plugin/plugin.json, marketplace.json, .codex-plugin/plugin.json) missing from §7.
- (attempt 1, Scope and alignment) [P2] [65] [advisory] DIFF_OVERSIZE / maxFullDiffBytes = 8 MB is a new hard-block class no goal asks for and sits against the non-goal on caps — add rationale to goals or drop it and measure per shard from git.
- (attempt 1, Scope and alignment) [P3] [85] [advisory] Goal (f) says PASS; mechanism yields PASS_ADVISORY — restate.
- (attempt 1, Architecture) [P2] [100] [advisory] FullDiff (≤8 MB) in gitcontext.Context changes a serialized contract: `metareview context diff` marshals the whole Context (main.go:104-120), learnsource embeds it — fix: json:"-" or return alongside.
- (attempt 1, Architecture) [P2] [75] [advisory] File identity from strings.Fields(header)[3] (profile.go:190-196) breaks on spaces/quoted names → wrong sizes, bogus paths, empty pack sections — fix: source paths from `git diff --name-only -z`; test with space + non-ASCII.
- (attempt 1, Architecture) [P2] [75] [advisory] Pack writes outside the all-or-nothing snapshot/restore (prready/review.go:148-232); PlanShards invoked from multiple call sites with inlined options; "content-addressed name but body changes with run id" self-contradictory — fix: compute plan once in Create, write packs inside the snapshot block, drop per-run values from bodies.
- (attempt 1, Architecture) [P2] [75] [advisory] Advisory fingerprint embeds the per-run hash → each re-run leaves the prior record open forever (findings.go:100-133) — fix: stable fingerprint `architecture:context-risk-covered` (or pr: prefixed), hash in Found.
- (attempt 1, Architecture) [P2] [75] [advisory] AddedLines caller-populated with silent fallback to truncated Diff; three hand-built GitContext callers — fix: derive in gitcontext, single non-optional field, or fail loudly when DiffTruncated && AddedLines empty.
- (attempt 1, Intent preservation) [P2] [50] [advisory] §9 sanctions deleting packs after results, erasing the record of what was reviewed — fix: per-path content hash in each result, or packs retained for the branch's life.
- (attempt 1, Security) [P2] [75] [advisory] No read cap / field cap / file-count cap on result ingestion (readEvidence caps at 12000) — fix: cap file 256 KB, cap count, truncate strings before render; tests.
- (attempt 1, Security) [P2] [50] [advisory] Untrusted result strings (reviewer, note, path) and ignored-file names rendered without PlainText/InlineCode → header injection into parsed sections — fix: pass through markdown.PlainText/InlineCode; state in §6.
- (attempt 1, Security) [P2] [50] [advisory] No containment/symlink check for docs/metareview/shards on write or read paths (safeJoin, EvalSymlinks patterns exist) — fix: EvalSymlinks, real dir under root, reject non-regular/symlinked result files; test.
- (attempt 1, Testing-quality) [P2] [75] [advisory] Several §5–§6 behaviours unnamed (loader cases, context-pack rendering, CLI, reviewmanifest.Markdown changes); loader package undecided (swaps 100% for 90.5 floor) — fix: name every test, fix loader package, add render tests.
- (attempt 1, Testing-quality) [P2] [75] [advisory] Negative cases under-covered: oversize-only-closable-by-accepted-risk has no enforcement point or test; mixed shardable+non-shardable reason set untested; exit-2 oracle unstated — fix: tests for mixed reasons, DIFF_OVERSIZE stays blocking e2e, oversize rule enforced not prose, exit-2 oracle = runs.jsonl byte-identical + no new run dir.
- (attempt 1, Data-migration) [P2] [75] [advisory] learnsource.Collect passes no generated excludes → committed packs (full diffs) enter the post-merge learning diff — fix: pass generatedMetareviewPathExcludes (or exclude docs/metareview/shards/**); note in §10.

## Follow-up Findings

- (attempt 1, Completeness) [P3] [75] [follow-up] Pack lifecycle: outside rollback; no pruning of superseded hashes; a copy of the branch per iteration — define snapshot participation and pruning.
- (attempt 1, Scope and alignment) [P3] [60] [follow-up] Cross-shard pack/result unconditional vs WU4 single-shard passes without cross-shard — require only when ≥2 shards.
- (attempt 1, Security) [P3] [50] [follow-up] Pathspec handling: git magic pathspecs/globs (*, ?, [, leading :) and C-quoted names; use GIT_LITERAL_PATHSPECS=1 / :(literal), -z parsing; assert pack byte count == planned ByteCount.
- (attempt 1, Security) [P3] [50] [follow-up] Full diffs committed under docs/metareview/shards give secrets a second permanent copy excluded from later lints — make retention explicit (gitignore-by-default or delete-after-results step in the skill).
- (attempt 1, Data-migration) [P3] [50] [follow-up] §10 "additive / untouched" false; list changed keys with one-time actions.

## Warnings

None.
