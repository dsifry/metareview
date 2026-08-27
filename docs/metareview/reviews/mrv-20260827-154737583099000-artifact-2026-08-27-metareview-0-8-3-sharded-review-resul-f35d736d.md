# metareview: artifact review

Run ID: `mrv-20260827-154737583099000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

Target: `docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md`

Context pack: `docs/metareview/context/mrv-20260827-154737583099000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d-context.md`

Execution mode: `parallel-subagents` (three lenses only)

Previous run: `mrv-20260827-150408336424000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

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
| Feasibility | NEEDS_REVISION | 2 | 0 | r5 verification; 3 findings |
| Completeness | NEEDS_REVISION | 4 | 0 | r5 verification; 6 findings |
| Testing-quality | NEEDS_REVISION | 0 | 0 | r5 verification; 6 findings |
| Scope and alignment | NOT_RUN | 0 | 0 | not dispatched — targeted three-lens pass (see notes) |
| Architecture | NOT_RUN | 0 | 0 | not dispatched — targeted three-lens pass (see notes) |
| Intent preservation | NOT_RUN | 0 | 0 | not dispatched — targeted three-lens pass (see notes) |
| Security | NOT_RUN | 0 | 0 | not dispatched — targeted three-lens pass (see notes) |
| Data-migration | NOT_RUN | 0 | 0 | not dispatched — targeted three-lens pass (see notes) |

## Orchestrator Notes (not findings)

**Retroactive record.** This run documents the three-lens verification of **r5** (commit `732cf87`) that was executed before r6 existed. The lenses (Feasibility, Completeness, Testing-quality — Claude Opus 5, parallel subagents) ran directly rather than against a scaffold, so no review log was created at the time; Dave asked for the gap in the audit trail to be closed, and this is that record. Because the scaffold was created afterwards, its context pack reflects r6, not the r5 text the lenses reviewed — r5 is `git show 732cf87:docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md`.

Scope of the pass was deliberately narrow (three lenses, not eight) on Dave's instruction not to overengineer, after he ruled the trust model out of scope; Scope, Architecture, Intent, Security and Data-migration were **not dispatched**, so this run is weaker evidence than the eight-lens attempts 1-3 and is recorded as such. Aggregate NEEDS_REVISION: 3/3 lenses. The decisive finding was a regression the simplification introduced — fixed hex-depth bucketing produced 108 shards where ~23 were wanted, and left two shards above the 120 KB cap the feature exists to escape. All findings are closed in r6 (`0562529`), whose §13 maps each one; r6 is reviewed separately in run `mrv-20260827-154745525699000`.

## Findings

## Blocking Findings

- (r5 verification, Feasibility) [P1] [100] [blocking] Fixed hex-depth bucketing over-shards ~5x on the real distribution: ceil(1372619/60000)=23 → smallest depth with 16^depth ≥ 23 is 2 (256 buckets); 133 files land in 108 non-empty shards (87 single-file, mean 12,709 B = 21% of budget), so one plan run costs ~109 subagents and 109 result files, all of which §6 requires fresh — fix: bucket on the first ceil(log2(need)) BITS of the hash (32 buckets here, mean 42,894 B; ids still path-only and still stable).
- (r5 verification, Feasibility) [P1] [100] [blocking] Pure hash bucketing puts no upper bound on a shard, so packs can exceed the 120 KB cap the feature exists to escape: at depth 2 shards `c7` = 80,997 B (2 files) and `b7` = 68,498 B (3 files); with 32 buckets, 9 shards exceed the budget and 2 exceed 120,000 B (max 159,464 B) — fix: split an over-budget bucket deterministically over its own sorted chunk list into shard-<hex>-2, -3, … (r4's trie solved sizing, not hostile-branch defence).
- (r5 verification, Completeness) [P1] [85] [blocking] §4.2 has no acceptance criterion for shard size or count and its stated invariant is false: a multi-file bucket can hold many times the budget, contradicting "a pack may exceed the budget only for a single-file shard"; on PR #13's numbers depth=2 yields ~100+ mostly single-file packs where ~23 are needed (~5x the subagent count §7 prices) — fix: correct the sentence and add a stated bound plus named tests for max shard bytes and shard count.
- (r5 verification, Completeness) [P1] [95] [blocking] "budget" is undefined in r5 — used five times but r4's defining paragraph (DefaultMaxBytesPerShard = 60000, bounds, --shard-budget) was deleted wholesale — fix: state the budget is the fixed contextprofile.DefaultMaxBytesPerShard = 60000, not configurable in 0.8.3.
- (r5 verification, Completeness) [P1] [85] [blocking] The review log's rendering of ingested results is unspecified after the attestation section was deleted, while §12 still requires TestAttestationSectionAfterVerdictParses and the shell test asserts "the log listing the chunked file"; placement is load-bearing (a section between ## Verdict and the token breaks verdict parsing) — fix: name the section, state it renders after the verdict token, list its rows, or delete the test name.
- (r5 verification, Completeness) [P1] [80] [blocking] The satisfied path is unreachable on task-done when local content raises an untracked reason, and §12's own task-done fixture (5 KB untracked file) triggers exactly that — fix: state the expected verdict and the operator consequence, or keep the fixture file ≤ 4,000 bytes.

## Advisory Findings

- (r5 verification, Feasibility) [P3] [100] [advisory] Shard id has `shard-` baked in but is re-prefixed in filenames (`shard-<id>.md` → `shard-shard-3a.md`), and §5.1 carries both `id` and `shardId` without saying which the filename encodes — fix: Shard.ID is bare hex, shardId = "shard-"+ID, filename encodes shardId.
- (r5 verification, Completeness) [P2] [70] [advisory] plan.json ("ids, hashes, chunks, base, head") is too thin for the shell test, which must write conforming results and cannot compute <target-slug> — fix: enumerate plan.json's fields incl. per-chunk hashes and the resolved results directory.
- (r5 verification, Completeness) [P2] [75] [advisory] §7's 1 MiB scanner list omits internal/findings/findings.go:381, also a default 64 KiB scanner, though §12 groups TestReadersAcceptOneMiBLines under (findings, …) — fix: add findings.readJSONL.

## Follow-up Findings

- (r5 verification, Testing-quality) [P1] [88] TestAttestationSectionAfterVerdictParses names a section §11 deleted and §5/§6 never define — rename to TestReviewLogListsIngestedAndIgnoredResults; add one sentence to §6 defining the section (per-shard id/hash/verdict; ignored with reason).
- (r5 verification, Testing-quality) [P1] [80] TestChunkNeverExceedsBudgetExceptSingleFileShard conflates chunk vs shard: §4.2 says chunks are always ≤ budget (over-long line hard-cut AT budget); the SHARD may exceed for a single-file shard — split into TestChunkNeverExceedsBudget and TestShardMayExceedBudgetOnlyForSingleFileShard.
- (r5 verification, Testing-quality) [P1] [72] 100% unreachable: OSDeps() wrapper bodies are their own blocks that stub-Deps tests never enter — add TestOSDepsRoundTripOnDisk (Write → Discover → Prune through OSDeps in t.TempDir()).
- (r5 verification, Testing-quality) [P2] [74] "independent whole-diff measurement with the same flags" is unimplementable (magic vs env-var forms are exclusive) → test would mirror production — state the oracle concretely: one `git diff --no-renames --text --no-textconv base..HEAD -- . ':(exclude)…'`, raw bytes, len(out) == Σ fileBytes, no per-path loop in the test.
- (r5 verification, Testing-quality) [P2] [70] Same-size-edit oracles vacuous: fixture may put all files in one bucket; shell round has no determinate edit and no exact counts — pin fixture paths with distinct bucket prefixes + lowered MaxBytesPerShard; fixed literal substitution; assert fresh = N-1, ignored = 2 with reasons.
- (r5 verification, Testing-quality) [P2] [62] TestPackBytesReproducible may be false: Deps still has Now though no header field uses it — drop Now or state where it is used and exclude from pack bytes; assert byte-equality of every pack file and plan.json.

## Warnings

Five of the eight required lenses were not dispatched (see Orchestrator Notes); this review is incomplete by the artifact-review gate rule and is recorded as evidence, not as a passing gate.
