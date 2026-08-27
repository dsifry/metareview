# metareview: artifact review

Run ID: `mrv-20260827-150408336424000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

Target: `docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md`

Context pack: `docs/metareview/context/mrv-20260827-150408336424000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260827-144839455264000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

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
| Feasibility | NEEDS_REVISION | 4 | 0 | attempt 3 on r3; 8 findings |
| Completeness | NEEDS_REVISION | 4 | 0 | attempt 3 on r3; 6 findings |
| Scope and alignment | NEEDS_REVISION | 3 | 0 | attempt 3 on r3; 5 findings |
| Architecture | NEEDS_REVISION | 7 | 0 | attempt 3 on r3; 8 findings |
| Intent preservation | NEEDS_REVISION | 3 | 0 | attempt 3 on r3; 6 findings |
| Security | NEEDS_REVISION | 2 | 0 | attempt 3 on r3; 7 findings |
| Testing-quality | NEEDS_REVISION | 7 | 0 | attempt 3 on r3; 8 findings |
| Data-migration | NEEDS_REVISION | 3 | 0 | attempt 3 on r3; 4 findings |

## Orchestrator Notes (not findings)

Attempt 3 reviewed r3 with eight parallel lens subagents (Claude Opus 5); execution mode `parallel-subagents`; chained to attempt 2; this is the last attempt of the chain. Aggregate NEEDS_REVISION: all eight lenses NEEDS_REVISION. The lenses confirmed the attempt-2 blockers they re-checked as closed (literal pathspecs, `.gitattributes` defeat, `superseded` status, attestation heading, counting domain, coverage-gate scope) and raised new blockers concentrated on plan stability (lexical first-fit re-cuts every later shard), the identity rule, the ownership check (icase/gitlink bypass, verified), the chain-independent carried-result clause, the stale/ignored contradiction, pack reproducibility, and the run-row data path. The chain is exhausted. Every attempt-3 blocker is addressed in r4 (§14 of the spec maps each to its change: content-stable bucket-trie plan with prefix ids, hash-only attribution, icase/any-mode ownership check as a never-satisfiable reason, chain-only KnownPackRuns, no stale category, rename-aside replacement, hard chunk cuts, createdAt on the run row with shard hashes in plan.json, two-PR landing plan). **Recommendation: accept r4 as the implementation baseline, subject to the human's explicit decision; the recorded aggregate is NEEDS_REVISION as returned.**

## Findings

## Blocking Findings

- (attempt 3, Feasibility) [P1] [90] [blocking] Lexical first-fit re-cuts every shard from the changed file onward (simulated: 1-byte growth re-cut 3/3 shards); with MaxAttempts from the root run a 23-shard plan escalates on the first fix round — fix: content-stable assignment (path-bucketed / hash-partitioned) or re-plan only the changed shard; restate per-round cost.
- (attempt 3, Feasibility) [P1] [88] [blocking] §5.1 identity rule contradicts §3 carry-forward: a carried result whose id moved is a blocker (+ duplicate on the same hash) — fix: attribute strictly by shardHash; id check = file id equals filename id; current id may differ.
- (attempt 3, Feasibility) [P2] [90] [blocking] Pack bytes not reproducible (run id + per-run nonce) vs "pure function"/"packs identical" — fix: exclude run id/nonce from the identity claim or derive the nonce deterministically.
- (attempt 3, Feasibility) [P2] [80] [blocking] Chunking at newline boundaries cannot bound a chunk when one line exceeds the budget — fix: hard byte cut with a marker; state placement.
- (attempt 3, Completeness) [P1] [90] [blocking] Reason taxonomy incomplete: UNTRACKED_OMITTED / UNTRACKED_TRUNCATED never mentioned; branchOnlyGitContext clears UntrackedOmittedCount but not UntrackedTruncatedCount → any untracked file > 4,000 B makes the satisfied path unreachable on pr-ready (the author's own tree has an untracked 13,786 B doc) — fix: state their handling on the branch gate (clear on branch-only path / reclassify as LOCAL_*), add an untracked >4 KB file to the shell fixture.
- (attempt 3, Completeness) [P1] [85] [blocking] "LARGE_DIFF uses the branch measurement" silently drops the combined-size signal (today branch+staged+worktree+untracked); 80 KB branch + 70 KB staged → no reason, clean pass over 150 KB — fix: keep LARGE_DIFF on the total and add a branch-scoped reason for planning, or add LOCAL_LARGE_DIFF; state in §11.
- (attempt 3, Completeness) [P2] [90] [blocking] KnownPackRuns.At has no data path (runchain.Record lacks createdAt) — fix: name createdAt in §7; treatment of rows without it.
- (attempt 3, Completeness) [P2] [75] [blocking] <target-slug> never defined; state's slug helper truncates to 48 chars with no hash → collisions (feature/a vs feature-a) prune each other's packs; §7/§8 spell it <target> — fix: define normatively (Slugify + sha256[:8]); one spelling.
- (attempt 3, Scope and alignment) [P1] [90] [blocking] Per-pack review work undefined: no lens set exists for pr-ready/task-done (only the artifact review has lenses); "standard lens set" per pack is not executable and §9 prices one dispatch per pack (24×8 = ~192 under the only reading) — fix: name the lens/rubric set and per-pack aggregation rule in §7 (or "one subagent per pack against the pr-ready/task-done rubric"); restate §9 cost as packs × lenses.
- (attempt 3, Scope and alignment) [P2] [80] [blocking] Ownership check hard-fails on tracked runs.jsonl/findings.jsonl, contradicting CLAUDE.md:42 / INSTALL.md:130 ("unless the repository explicitly changes that contract") — fix: amend the contract in §7 docs or degrade to a never-satisfiable risk reason.
- (attempt 3, Scope and alignment) [P2] [75] [blocking] No landing plan for 0.8.3's own PR: largest change in repo history, > 120 KB, must shard-review itself with ungated code — fix: landing section splitting delivery (PR-A measurement+packs under the cap; PR-B ingestion+gate+migration+docs), or state how 0.8.3 self-shards.
- (attempt 3, Architecture) [P1] [85] [blocking] Identity rule contradicts hash-based freshness: "matched by hash regardless of id" vs "shardId must equal current id" → renumbering after a deletion makes carried results permanent blockers (+ duplicate on re-review); nothing prunes docs/metareview/shards — fix: bind identity to hash, demote shardId/NN to metadata (drop the id-equality blocker); specify supersession/removal of superseded result files.
- (attempt 3, Architecture) [P1] [78] [blocking] v3 chain has no producer: PlanShards(profile, options) has no diff text; FileProfile has Hash/Source but no Diff — fix: state PlanShards' new signature (profile, branchFiles []gitcontext.BranchFile, options); chunk text never copied into Profile.
- (attempt 3, Architecture) [P1] [72] [blocking] Domain of definition for non-risk runs unstated: BranchFiles nil on every small run (DefaultLargeDiffBytes == maxDiffBytes → second disjunct dead) but the manifest/planHash is built unconditionally — fix: define hashes when BranchFiles is nil (compute or absent; gate manifest render on a plan existing); drop the dead disjunct.
- (attempt 3, Architecture) [P2] [88] [blocking] Replace = RemoveAll then rename destroys existing packs on crash; "pure function" false (nonce + run id) so "packs identical" oracle unfalsifiable — fix: rename-aside → rename-in → remove; deterministic nonce from planHash+shard id or restate the oracle.
- (attempt 3, Architecture) [P2] [85] [blocking] gitcontext.runGit unexported → unreachable from prready/taskdone tests; second injection style; global not t.Parallel-safe — fix: Deps-style injection for gitcontext (Options.RunGit threaded from callers).
- (attempt 3, Architecture) [P2] [82] [blocking] KnownPackRuns[id].At has no data path: runchain.Record has no createdAt (writers persist it, reader drops it) — fix: add createdAt to runchain.Record; say which caller maps chain rows into Input.KnownPackRuns.
- (attempt 3, Architecture) [P2] [72] [blocking] No retention for docs/metareview/shards; 65th file blocks; two re-plans exceed 64 on PR #13 — fix: supersession/GC (keep results matching a current shard/plan hash; prune after pass) or 65th ignored with deterministic keep order.
- (attempt 3, Intent preservation) [P1] [100] [blocking] "Results committed only after pass, so HEAD does not move" is false: a fix must be committed to reach the reviewed content (freshness ignores local changes), and runchain resets ESCALATED rows whose HeadSHA differs (runchain.go:155-190) → a fresh chain at attempt 1 with zero re-review cost via carried results; only a skill sentence protects escalation — fix: make the escalation lock chain/target-scoped for sharded loops, or delete the rationale and state plainly nothing mechanically prevents a fresh chain after a fix commit.
- (attempt 3, Intent preservation) [P1] [75] [blocking] --shard-budget ≤ 120000 with "bounds one pack, never the review context" — the pack IS the lens's context (chunk + header + contract + fences); at max budget it meets/exceeds the cap; §3/§9 recommend PR #13 run there — fix: bound budget so chunk bytes + rendered overhead < maxDiffBytes; drop the 120000 recommendation.
- (attempt 3, Intent preservation) [P1] [75] [blocking] Chunking: >budget files reviewed only in fragments possibly across shards; cross-shard gets results not bytes; attestation certifies as if whole-file — fix: state the reduced guarantee in §2(g)/§5.3; render split files in the attestation; all chunks of one file to a single reviewer (or stated overlap).
- (attempt 3, Security) [P1] [88] [blocking] Ownership check bypassable: (a) case-insensitive FS — tracked `.METAREVIEW/runs.jsonl` makes `git ls-files -- .metareview/...` empty while the dir is real and readable; (b) `.metareview` tracked as a gitlink/submodule — empty ls-files, real dir with attacker content; combined with committed results and offline-computable hashes → one-run PASS_ADVISORY on an unreviewed branch (reproduced) — fix: `git ls-files -sz -- ':(icase).metareview'` refuse on ANY entry incl. mode 160000; refuse if .metareview resolves inside a nested git repo; require .metareview git-ignored.
- (attempt 3, Security) [P2] [72] [blocking] KnownPackRuns not chain-bound: the carried-result clause "any local row recording that shard hash" is unconditional → a planted/unrelated row honoured on the first run with no --previous-run — fix: carried-result clause restricted to runs in the resolved --previous-run chain; refuse ingestion when no chain row records the current plan.
- (attempt 3, Testing-quality) [P1] [100] [blocking] "packs identical" across three runs contradicts the per-run run id header and per-run nonce; §4.3 "pure function of plan and header" false — fix: invariant over the deterministic portion (normalise run-id line and nonce lines, or inject fixed Nonce/Now and run id); state in §4.3 and §8.
- (attempt 3, Testing-quality) [P1] [90] [blocking] §5.1 says hash-mismatch → blocker `stale shard result`; §5.2 says hash-mismatch → ignored `no current shard`; same condition, two outputs; the r2 "stale unreachable" blocker not closed — fix: hash-mismatch ⇒ ignored; `stale` reserved for hash-match-with-chunk-mismatch; restate shell assertions in exact strings.
- (attempt 3, Testing-quality) [P2] [85] [blocking] Shell scenario never re-reaches a passing gate after the edit (no fix round: write result for the changed shard + cross-shard, --previous-run, PASS_ADVISORY with untouched shards carried) — fix: extend the script.
- (attempt 3, Testing-quality) [P2] [75] [blocking] Zero-uncovered gate unattainable: OSDeps' real Nonce (crypto/rand error branch) untestable; no OSDeps test named — fix: `var randRead = rand.Read` seam + TestOSDepsNonceAndErrorBranch, or state excluded entry points.
- (attempt 3, Testing-quality) [P2] [75] [blocking] prready/taskdone writer seam named only in §8; §4.3 declares a concrete Writer; no field/var/interface — fix: declare the seam with its type in §4.3/§7.
- (attempt 3, Testing-quality) [P2] [75] [blocking] Independent whole-diff oracle omits --text --no-textconv --no-ext-diff while the sibling fixture has a NUL file under --text → equality false by construction — fix: identical flag set + excludes.
- (attempt 3, Testing-quality) [P2] [75] [blocking] Chunking has no rule/oracle for a single line longer than the budget; TestBudgetNeverExceedsDiffCap relates two constants only — fix: over-long-line rule; TestChunkNeverExceedsBudget.
- (attempt 3, Data-migration) [P1] [74] [blocking] §11 alias rule retires a still-open `tests:missing:` row unconditionally (prefix-keyed) even when the run early-returned at context risk and emitted no replacement → genuine blocker silently cleared; the new fingerprint shares the prefix so it would be re-superseded every run — fix: alias-supersede only when the run evaluated that class (new equivalent fingerprint present / non-early-return path); legacy discriminator = "value not exactly 16 lowercase hex"; test the early-return case.
- (attempt 3, Data-migration) [P2] [82] [blocking] KnownPackRuns.At has no source (Record lacks createdAt) → zero time makes reviewedAt ≥ At inert — fix: add createdAt/updatedAt to runchain.Record in §7 (rows missing them = "recorded no plan") or drop the comparison.
- (attempt 3, Data-migration) [P2] [76] [blocking] shardHashes uncapped map on runs.jsonl whose readers use a 64 KiB scanner (no Buffer): a legal run (16 MiB / budget 1 → >2,000 shards) writes a line > MaxScanTokenSize and ReadRuns fails forever; shardId pattern caps at 999 anyway — fix: reject budgets yielding >999 shards at arg validation; store only planHash in the row with shard hashes in plan.json; raise scanner buffers in runchain/state/reviewlog on main.

## Advisory Findings

- (attempt 3, Feasibility) [P3] [85] [advisory] KnownPackRun{PlanHash, At} has no ShardHashes field for the carry-forward rule — add it.
- (attempt 3, Feasibility) [P3] [85] [advisory] Σ fileBytes invariant fails: git() TrimSpaces output (off by one) and the whole-diff call omits --text --no-textconv --no-ext-diff (1589 vs 1581 B verified) — raw untrimmed bytes, identical flags.
- (attempt 3, Feasibility) [P3] [70] [advisory] reviewedAt (RFC3339, seconds) vs createdAt (RFC3339Nano) → same-second results ignored — truncate pack time to seconds or require fractional seconds.
- (attempt 3, Feasibility) [P3] [60] [advisory] Durable results dir has no lifecycle; superseded results accumulate and can trip duplicate — plan-scoped subdir or explicit prune rule.
- (attempt 3, Completeness) [P2] [85] [advisory] §7 docs list incomplete: docs/README.claude.md:52, docs/README.codex.md:59, skills/status/SKILL.md:27-28, commands/status.md:9 carry the durable/transient contract — add them.
- (attempt 3, Completeness) [P3] [80] [advisory] WU4 supersession cites line numbers of unnumbered bullets (33 is provenance fields, not disposition) — quote each criterion's text.
- (attempt 3, Scope and alignment) [P2] [75] [advisory] Chain budget for PR #13 unstated; DefaultMaxAttempts=3 = plan + results + one fix round, then locked until HEAD moves — state that the first run sets --max-attempts.
- (attempt 3, Architecture) [P3] [60] [advisory] AddedLines ownership inverted (gitcontext computing a reviewers value; runtime warning for a wiring defect); leftover "?" in §4.2 — keep the union derivation in reviewers, pass the untruncated branch text; delete the question.
- (attempt 3, Intent preservation) [P2] [50] [advisory] shardHash carries no scope/target/base binding; "any local row recording that shard hash" lets a result from another target/base/abandoned chain count — fold scope/target/base into the preimage; restrict vouching to same scope/target lineage.
- (attempt 3, Intent preservation) [P2] [50] [advisory] §5.4 "no waiver channel" vs fixed/false-positive closing any severity; `fixed` on a fresh result is self-contradictory (a real fix stales the shard) — block fixed/false-positive on high/critical unless the shard hash changed, or reword §5.4 as host-attested unverified.
- (attempt 3, Security) [P2] [82] [advisory] "content cannot be hidden" false for gitlinks: submodule bump = 186-byte "Subproject commit" chunk while arbitrary code enters — fix: scope the sentence; never-satisfiable `SUBMODULE_CHANGE` on mode-160000 entries.
- (attempt 3, Security) [P2] [80] [advisory] UNSAFE_PATH covers only <0x20/0x7f; PlainText strips only Cc; bidi/invisible/Zl/Zp survive in paths and all ingested strings at all sinks — fix: reject/strip Cf, Zl, Zp; NFC normalisation for path comparison.
- (attempt 3, Security) [P3] [70] [advisory] Nonce has no entropy requirement and no consumer/detector — require crypto/rand (test OSDeps uses it); state fence is the boundary, nonce defence in depth.
- (attempt 3, Testing-quality) [P3] [75] [advisory] Coverage script passes vacuously on empty profile / failed build; no oracle for --shard-budget bounds exit 2 or "no packs under DIFF_OVERSIZE" — add assertions and cases.
- (attempt 3, Data-migration) [P3] [68] [advisory] In-place findings.jsonl rewrite with no backup; FINDINGS.md also rewritten on success — copy to findings.jsonl.pre-0.8.3 before the alias pass; note the FINDINGS.md rewrite.

## Follow-up Findings

- (attempt 3, Scope and alignment) [P3] [60] [follow-up] --shard-budget optional surface; default reaches PR #13 — mark follow-up or justify.
- (attempt 3, Intent preservation) [P3] [50] [follow-up] Durable results outlive their verification path (packs pruned; base..HEAD unreproducible after rebase/merge) — record base/head SHAs; state in §11.
- (attempt 3, Security) [P3] [65] [follow-up] Ownership check is a precondition of Writer.Write only; no rule that ingestion is refused when the check didn't pass — state ordering invariant; test.
- (attempt 3, Security) [P3] [60] [follow-up] 64-file cap contradicts goal (h): 16 MiB at 60 KB → ~280 shards; branches > ~3.8 MB can never satisfy — derive cap from plan (ShardCount+1+slack) or lower the bound.

## Warnings

None.

## Human Decision

Pending: the chain is exhausted (3 attempts); r4 closes every attempt-3 blocker. Acceptance of r4 as the implementation baseline is requested from Dave Sifry.
