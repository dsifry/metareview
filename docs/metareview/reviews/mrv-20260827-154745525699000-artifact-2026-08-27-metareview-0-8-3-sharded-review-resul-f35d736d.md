# metareview: artifact review

Run ID: `mrv-20260827-154745525699000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

Target: `docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md`

Context pack: `docs/metareview/context/mrv-20260827-154745525699000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d-context.md`

Execution mode: `parallel-subagents` (three lenses only)

Previous run: `mrv-20260827-154737583099000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

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
| Feasibility | PASS_ADVISORY | 0 | 0 | r6 review; 4 findings |
| Completeness | NEEDS_REVISION | 3 | 0 | r6 review; 5 findings |
| Testing-quality | NEEDS_REVISION | 1 | 0 | r6 review; 6 findings |
| Scope and alignment | NOT_RUN | 0 | 0 | not dispatched — targeted three-lens pass (see notes) |
| Architecture | NOT_RUN | 0 | 0 | not dispatched — targeted three-lens pass (see notes) |
| Intent preservation | NOT_RUN | 0 | 0 | not dispatched — targeted three-lens pass (see notes) |
| Security | NOT_RUN | 0 | 0 | not dispatched — targeted three-lens pass (see notes) |
| Data-migration | NOT_RUN | 0 | 0 | not dispatched — targeted three-lens pass (see notes) |

## Orchestrator Notes (not findings)

Review of **r6** (`0562529`) with three lens subagents (Feasibility, Completeness, Testing-quality — Claude Opus 5, parallel), chained to the retroactive r5 record. Scope was deliberately narrow on Dave's instruction not to overengineer, after he ruled the trust model out of scope; Scope, Architecture, Intent, Security and Data-migration were **not dispatched**, so this run is weaker evidence than the eight-lens attempts on r1-r3 and does not satisfy the artifact-review gate rule on its own.

Aggregate NEEDS_REVISION (Feasibility PASS_ADVISORY; Completeness and Testing-quality NEEDS_REVISION). Feasibility reproduced §4.2 end to end on the real PR #13 diff: the two-step assignment yields 46 shards with a maximum of exactly 60,000 bytes (none over budget), and perturbing eight files changed only their own buckets — the sizing regression that r5's check found is closed. The remaining blockers are all 'the spec must choose between two readings' rather than design faults: whether `coveredChunks`/`coveredShardIds` are validated or dropped; what epic-ready renders after the `PlanShards` signature change; whether `Manifest.SourcePaths` stays plain paths; and one test fixture (`TestShardNeverExceedsBudget`) that would pass with the fix unimplemented.

## Findings

## Blocking Findings

- (r6, Completeness) [P1] [75] [blocking] coveredChunks/coveredShardIds appear in the §5.1 wire format with no validation rule, and §4.2's Shard.Paths consumer list omits manifest.go:264 where shard.Paths is passed as requiredPaths to reviewResultBlockers (the "does not cover"/"covers unknown" blockers at :305-320, and crossShardBlockers at :269-280) — unclear whether the check is dropped (subsumed by shardHash) or re-expressed over chunks; §12 has no test either way — fix: state which, and add the named test.
- (r6, Completeness) [P2] [75] [blocking] The PlanShards/ShardPlanMarkdown signature change strands epicready (review.go:500 calls ShardPlanMarkdown with GroupBy:"path"); with no branchFiles it silently renders "not sharded", a behaviour README:59/INSTALL:128/quickstart:62 advertise; §4.2 never gives the new signatures nor says what becomes of ShardOptions.GroupBy / groupedFiles / shardGroupKey / shardReason and TestPlanShardsHonorsDomainGrouping (shards_test.go:49) — fix: state epic-ready renders "not sharded"; give the signatures; remove GroupBy and its test explicitly.
- (r6, Completeness) [P2] [75] [blocking] §6 redefines Manifest.SourcePaths as branch chunks without saying how a chunk is represented, and ignores the other consumers: pathDispositionBlockers (manifest.go:188 both-source-and-disposition; :200 disposition required for every generated path) and Markdown "### Source Paths" (:157). If entries become `path#part` keys the :188 check silently stops matching — fix: say SourcePaths stays plain paths with a separate chunk set for assignment, or re-express both consumers.
- (r6, Testing-quality) [P1] [75] [blocking] TestShardNeverExceedsBudget has no fixture constraint → passes with §4.2 step 2 unimplemented (same vacuity class r5 flagged); it is §13's sole verification that shards no longer exceed the cap — fix: fixture must put a bucket over the lowered MaxBytesPerShard; assert a `-2` sub-shard id exists and every shard Bytes <= budget.

## Advisory Findings

- (r6, Feasibility) [P2] [80] [advisory] §5.1 defines coveredChunks/coveredShardIds with no validation rule, while the code they replace enforces exact bidirectional coverage (manifest.go:264,312-331); the hashes already commit to both sets — fix: delete both fields, or state they must equal the shard's chunk list / plan's shard list exactly in both directions.
- (r6, Feasibility) [P3] [95] [advisory] §7's "roughly 35-40 packs" understated: running §4.2 exactly on main…fsm-enhancements gives need=23, bits=5, all 32 buckets non-empty, 9 splitting into 14 sub-shards → 46 shards, max exactly 60,000 B (zero over budget), min 413 B, mean 29,840 B — fix: "~46 packs (roughly 2× need, the cost of hash bucketing)".
- (r6, Feasibility) [P3] [60] [advisory] §6 "placed after the `## Verdict` token" read literally breaks parsing (reviewlog.go:113-114 takes nextNonEmpty after the heading) — say "after the verdict value line that follows `## Verdict`".
- (r6, Completeness) [P3] [75] [advisory] §4.2 step 2 sorts "its own path-sorted chunk list" with no tie-break between parts of one path, and the Chunk shape is only implied by plan.json — fix: sort by (path, part); name the Chunk struct once in §4.2.
- (r6, Completeness) [P3] [50] [advisory] §5.2's GC is not scoped to the result filename patterns and sits against §8's "audit records" claim — fix: restrict deletion to the two patterns; note in §8 that superseded records are pruned by design.
- (r6, Testing-quality) [P2] [65] [advisory] TestChunkNeverExceedsBudget contradicts §4.2: if the `[cut]` marker is inside the chunk text a hard-cut chunk is budget+len(marker), and the marker also breaks the byteStart/byteEnd → source mapping plan.json promises — fix: state the marker is rendered in the pack outside the hashed/measured chunk text (or cut at budget-len(marker)).
- (r6, Testing-quality) [P2] [70] [advisory] Pack "exact re-run command" would carry --previous-run <run-id>, and run ids are timestamp-derived — the last per-run string in a pack after Deps lost its clock; breaks both reproducibility assertions — fix: the pack's re-run command is run-id-free (base/target flags only).
- (r6, Testing-quality) [P2] [60] [advisory] TestOSDepsRoundTripOnDisk never reaches OSDeps().ReadFile (a fresh TempDir has no result files) → the 100% gate stays red — fix: the round trip writes a real result file (plus one unreadable/oversize) so Discover exercises ReadFile.
- (r6, Testing-quality) [P2] [55] [advisory] reviewlog.parseMarkdown harvests `mrvf-…` from every line (reviewlog.go:46,116), so a reviewer string or --shard-result path in the §6 listing contaminates Summary.FindingIDs (consumed by learnsource:57, prready/evidence.go:45,104); PlainText/InlineCode don't neutralise it — fix: fixture puts an mrvf--shaped token in reviewer and asserts parsed FindingIDs unchanged.
- (r6, Testing-quality) [P3] [55] [advisory] TestBucketBitsFromTotalBytes states no oracle → mirrors the implementation — fix: pin a small table (need 1→0, 2→1, 3→2, 4→2, 5→3, cap 12).

## Follow-up Findings

- (r6, Feasibility) [P3] [90] [follow-up] Shard.Paths consumer list incomplete: also manifest.go:264 (coverage args), :369 (SourceManifestHash preimage), :387 (canonicalShardPlan), contextprofile/shards.go:70,75,162 and profile.go:120 — the two manifest sites are behaviour, not renames.

## Warnings

Five of the eight required lenses were not dispatched (see Orchestrator Notes); this review is evidence, not a passing gate.

## Human Decision

2026-08-27 — Dave Sifry accepted **r7** (`db2280d`) as the implementation baseline. r7 closes every blocker from this run (§14 of the spec maps each one); it was not itself re-reviewed, and the recorded aggregate for r6 stays `NEEDS_REVISION` as returned. Rationale recorded at the time: blocker counts fell 35 → 41 → 33 → 6 → 4 across the chain, Feasibility reproduced the plan on the real PR #13 diff and returned PASS_ADVISORY, and the remaining risk is in code rather than in the spec — so the task-done gate on PR-A is the next real filter. Trust model and adversarial hardening were ruled out of scope by the human earlier in the same session.

