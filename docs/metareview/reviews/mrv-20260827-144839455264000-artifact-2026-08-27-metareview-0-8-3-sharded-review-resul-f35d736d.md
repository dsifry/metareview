# metareview: artifact review

Run ID: `mrv-20260827-144839455264000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

Target: `docs/specs/2026-08-27-metareview-0.8.3-sharded-review-results.md`

Context pack: `docs/metareview/context/mrv-20260827-144839455264000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d-context.md`

Execution mode: `parallel-subagents`

Previous run: `mrv-20260827-143602036900000-artifact-2026-08-27-metareview-0-8-3-sharded-review-resul-f35d736d`

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
| Feasibility | NEEDS_REVISION | 7 | 0 | attempt 2 on r2; 8 findings |
| Completeness | NEEDS_REVISION | 6 | 0 | attempt 2 on r2; 8 findings |
| Scope and alignment | NEEDS_REVISION | 3 | 0 | attempt 2 on r2; 7 findings |
| Architecture | NEEDS_REVISION | 5 | 0 | attempt 2 on r2; 8 findings |
| Intent preservation | NEEDS_REVISION | 4 | 0 | attempt 2 on r2; 7 findings |
| Security | NEEDS_REVISION | 6 | 0 | attempt 2 on r2; 7 findings |
| Testing-quality | NEEDS_REVISION | 6 | 0 | attempt 2 on r2; 8 findings |
| Data-migration | NEEDS_REVISION | 4 | 0 | attempt 2 on r2; 8 findings |

## Orchestrator Notes (not findings)

Attempt 2 reviewed r2 with eight parallel lens subagents (Claude Opus 5); execution mode `parallel-subagents`; chained to attempt 1. Aggregate NEEDS_REVISION: all eight lenses NEEDS_REVISION. Several blockers were verified empirically by the lenses in scratch repositories (pathspec triple yields empty diffs; `.gitattributes -diff` hides content; tracked `.metareview/runs.jsonl` survives checkout; rename accounting; `os.Rename` onto a non-empty dir). Every blocker is addressed in r3 (§13 of the spec maps each to its change); attempt 3 reviews r3 chained to this run and is the last attempt of the chain.

## Findings

## Blocking Findings

- (attempt 2, Feasibility) [P1] [100] [blocking] Pathspec triple no-op (dup Security): literal env + :(literal) → empty diffs; env disables :(exclude) — fix: env scoped to per-path diff with a bare path; name list without env using :(exclude).
- (attempt 2, Feasibility) [P1] [100] [blocking] Per-shard freshness cannot survive discovery: planHash folds in sourceDiffHash → any fix renames every result's expected filename → all "Ignored" → ResultCount==ShardCount fails; §8 "others fresh" unreachable — fix: name results by shard identity (`shard-NN.<shardHash>.result.json`, `cross-shard.<planHash>.result.json`) or re-key unchanged results by shardHash.
- (attempt 2, Feasibility) [P1] [100] [blocking] Branch-only planning collides with sourceAssignmentBlockers ("<path> is not assigned to a primary shard" for every profile file, manifest.go:209-232) → task-done / --include-working-tree unsatisfiable with any dirty/untracked file — fix: amend sourceAssignmentBlockers to require assignment only for branch-source paths; define Source for a path both branch-changed and locally dirty.
- (attempt 2, Feasibility) [P1] [75] [blocking] Rename detection: per-path diff of a renamed file is whole-file (14,677 B vs 99 B in the full diff); old path never listed; sum invariant fails; inflates DIFF_OVERSIZE — fix: --no-renames (-M0) for name list and per-path diffs; define measurement against the rename-free diff.
- (attempt 2, Feasibility) [P2] [100] [blocking] Atomic rename onto an existing non-empty <planHash> dir fails (verified darwin); the unchanged-content re-run scenarios re-write the same hash — fix: remove-then-rename / tmp swap; MkdirTemp under .metareview (no EXDEV).
- (attempt 2, Feasibility) [P2] [75] [blocking] "Inside the snapshot block" doesn't roll back a directory (restoreSnapshots rewrites five files; removeEmptyDirs); prune irreversible — fix: register pack dir for RemoveAll on failure; prune only after writes succeed.
- (attempt 2, Feasibility) [P2] [75] [blocking] AddedLines defined as branch-only; today addedLines unions branch+staged+worktree+untracked (reviewers/taskdone.go:250-259) → task-done stops linting uncommitted work — fix: AddedLines = untruncated branch ∪ staged ∪ worktree ∪ untracked; test staged eval( found on satisfied path.
- (attempt 2, Completeness) [P1] [100] [blocking] Branch-only planning vs sourceAssignmentBlockers (dup) — fix: SourcePaths/assignment branch-only or explicit out-of-scope disposition for local paths; test.
- (attempt 2, Completeness) [P1] [100] [blocking] Renames undefined; sum invariant false (83 vs 715 B); deleted side of a rename in no pack — fix: pin --no-renames (or pair via --name-status -z); restate invariant; rename/copy/binary fixture test.
- (attempt 2, Completeness) [P1] [90] [blocking] AddedLines population for non-branch sources unspecified (dup) — define union per caller; staged-only TODO test.
- (attempt 2, Completeness) [P1] [75] [blocking] Provenance channel unspecified end-to-end: no field/JSON key; writer structs (prready/review.go:47-72 runRecord, taskdone) and reader runchain.Record not listed; reviewmanifest pure — fix: name run-row fields in writers and runchain.Record and the input carrying known pack-run ids to the validator.
- (attempt 2, Completeness) [P2] [75] [blocking] Result rendering specified for the context pack (reviewmanifest.Markdown) not the review log; the certification sentence "in its verdict section" would corrupt reviewlog.parseMarkdown (first non-empty line after ## Verdict) — fix: name the log section/heading/placement (strictly after the verdict token); log-render + reviewlog-parse test.
- (attempt 2, Completeness) [P2] [75] [blocking] Prune "per target" over a layout with no target; sibling-dir deletion outside snapshot; task-done vs in-flight pr-ready deletes each other's packs — fix: scope+target in the pack dir (and result names) or repo-wide prune only after the new plan is durably written; two-target test.
- (attempt 2, Scope and alignment) [P1] [90] [blocking] Goal unreachable on PR #13: `internal/fsm/machine/machine_test.go` is 86,974 bytes filtered diff > DefaultMaxBytesPerShard 60,000 → oversize-file shard, always blocks; only remedy "split the change" — fix: parameterise the shard budget (distinct from maxDiffBytes) and/or byte-range sub-shards; state the budget PR #13 runs with.
- (attempt 2, Scope and alignment) [P2] [85] [blocking] r2 silently supersedes WU4 criteria (hash inputs 29; explicit disposition unblocks 33/34/54) — fix: name superseded criteria in §11/§12; dated supersession note in the WU4 plan.
- (attempt 2, Scope and alignment) [P2] [80] [blocking] Goal (g) ports 0.9.0's coverage gate + 28-line floor into a point release (main has no internal/fsm/workflows; the script takes its legacy-only path) — fix: split the gate port into its own PR; in 0.8.3 enforce shardpack 100% with a minimal check in run-all.sh.
- (attempt 2, Architecture) [P1] [75] [blocking] planHash-scoped discovery cancels per-shard freshness (dup Feasibility) — fix: shard-scoped discovery keyed by shardHash; define carry-forward (which run id a carried result may name; only changed shards + cross-shard re-run).
- (attempt 2, Architecture) [P1] [75] [blocking] Branch-only planning vs sourceAssignmentBlockers (dup) — fix: manifest SourcePaths/assignment restricted to branch-source files; non-branch files as an explicit disposition class; test with staged/untracked present.
- (attempt 2, Architecture) [P1] [75] [blocking] AddedLines branch-only drops lint coverage of staged/worktree/untracked on every task-done run (dup) — fix: union; test worktree eval( still blocks.
- (attempt 2, Architecture) [P2] [100] [blocking] Snapshot block doesn't roll back directories; prune unrecoverable (dup) — fix: explicit shardpack rollback (record created/pruned dirs, undo on failure); prune after writes succeed.
- (attempt 2, Architecture) [P2] [75] [blocking] Goal (b) contradicted: pack writer re-derives diffs via Deps.Diff instead of the measured bytes; 2×N git spawns per review on the hot path, sharded or not — fix: thread measured per-path bytes from gitcontext into the writer; compute BranchFiles lazily only at context risk.
- (attempt 2, Intent preservation) [P1] [75] [blocking] planHash-keyed discovery cancels per-shard freshness (dup); provenance compounds it: a carried result names a run that recorded the OLD planHash — spec never says which plan hash is compared — fix: shardHash-keyed filenames; state normatively whether a result whose packRunId recorded a superseded planHash passes provenance.
- (attempt 2, Intent preservation) [P1] [75] [blocking] "Human accepts by editing the review log's human-decision section" mechanism does not exist: nothing reads review logs back; findings.jsonl Status is machine-set only (findings.go:101-146,242) — fix: name the real acceptance mechanism or scope adding one into 0.8.3.
- (attempt 2, Intent preservation) [P1] [75] [blocking] §6 --max-attempts advice hands the escalation ceiling to the agent and doesn't work: with --previous-run, runchain.Resolve takes MaxAttempts from the root run (runchain.go:78-83); committing results each round changes HEAD, which already clears ESCALATED rows for the branch target (:154-192) — fix: ceiling set by a human on the root run; mid-chain flag ignored; the sharded loop stops and reports rather than opening a new chain.
- (attempt 2, Intent preservation) [P2] [75] [blocking] §5.3 wording claims a verified fact ("every shard was reviewed") but nothing checks a review happened (reviewer non-empty; reviewedAt parses) — fix: word as attestation; add reviewedAt ≥ pack run timestamp.
- (attempt 2, Security) [P1] [88] [blocking] packRunId forgeable: .metareview/runs.jsonl is read from the worktree with no ownership check (runchain.go:94-123); a tracked file survives .gitignore (verified `git add -f`); planHash deterministic → hostile branch commits a fabricated run row + results → first pr-ready run PASS_ADVISORY — fix: bind packRunId to the run id generated in memory (never re-read from disk) and refuse the run if `git ls-files -- .metareview` is non-empty or .metareview is not an untracked real directory.
- (attempt 2, Security) [P1] [92] [blocking] GIT_LITERAL_PATHSPECS=1 + :(literal) + :(exclude) are mutually exclusive; verified: with the env var, :(exclude) fails open (generated paths re-enter the hash) and ':(literal)p' returns nothing (every shard empty → vacuous PASS) — fix: exclude-filtered listing runs without the env var using :(exclude) magic; each per-path diff runs with GIT_LITERAL_PATHSPECS=1 and a bare path after --; tests for a generated path absent and a `*` path resolving.
- (attempt 2, Security) [P1] [80] [blocking] Committed `.gitattributes` `path -diff` yields "Binary files differ" stubs while hashes validate (verified; --text defeats it; -c core.attributesFile does not) — fix: per-path diff with --text --no-textconv; any path still reporting binary is a never-satisfiable shard reason like oversize-file.
- (attempt 2, Security) [P2] [82] [blocking] Delimiter spoofable: filenames may contain newlines (verified via -z) and paths render outside the fence in the pack header, cross-shard table, manifest ### Shards, context pack; no closing delimiter/nonce — fix: PlainText+InlineCode for every path/hash outside a fence; matching close delimiter with a per-run nonce; newline/backtick/pipe filename test.
- (attempt 2, Security) [P2] [80] [blocking] Ingested strings reach unsanitized sinks: aggregator blocker text (shardId unconstrained/untruncated) → finding Found → review log (`"- Found: "+record.Found` at prready/review.go:914, taskdone/review.go:532), FINDINGS.md, findings.jsonl; PlainText doesn't escape `|` in table cells — fix: closed pattern for shardId; truncate + PlainText every ingested string at every sink; escape `|`; name all four sinks with a test each.
- (attempt 2, Security) [P2] [72] [blocking] Containment resolves only repoRoot, not `.metareview` itself; `.gitignore` `.metareview/*` doesn't match `.metareview`, so a committed symlink at .metareview redirects packs/prune/runs.jsonl outside the repo (verified) — fix: EvalSymlinks on the full path and each parent; refuse symlinked/non-dir/tracked .metareview; prune only direct children matching 16-hex.
- (attempt 2, Testing-quality) [P1] [90] [blocking] Shell staleness oracle still unreachable (planHash filenames → all "ignored", never stale/fresh); "stale shard result" unreachable via default path — fix: shardHash-keyed filenames (or match by shardId, validate hashes inside); assert fresh/stale/ignored separately.
- (attempt 2, Testing-quality) [P1] [85] [blocking] sum(BranchFiles.Bytes)==BranchFilteredDiffBytes is tautology or false (renames: 813 vs 12,619 B; old path never in coveredPaths); fixture avoids rename/delete/binary/mode-only — fix: independent measurement, --no-renames stated, TestBranchFilesRenameDeleteBinaryModeOnly.
- (attempt 2, Testing-quality) [P1] [80] [blocking] ResultCount==ShardCount counting domain undefined: no shardId uniqueness/filename match/unknown-id rule; cross-shard counted? two copies of shard-01 via --shard-result satisfy a 2-shard plan — fix: distinct covered shard ids == plan set (cross-shard separate); TestDuplicateShardIdRejected, TestResultForUnknownShardIdRejected, TestCrossShardResultOnSingleShardPlan.
- (attempt 2, Testing-quality) [P2] [85] [blocking] TestBranchFilesLiteralPathspecs is presence-only; with the §3 recipe every diff(p) is empty and it still passes — fix: assert fileBytes(p)>0 and expected changed line; single pathspec mechanism.
- (attempt 2, Testing-quality) [P2] [80] [blocking] Coverage claim not enforceable as stated: three coordinated sites (case pattern, REQUIRED go-list, floor-exclusion awk); on main REQUIRED resolves to nothing → legacy-only path — fix: name all three; REQUIRED must list ./internal/shardpack.
- (attempt 2, Testing-quality) [P2] [75] [blocking] gitcontext has no seam (exec.Command direct); prready/taskdone gain pack-write/run-row code with no injectable I/O; floors can only rise → PR blocked by its own gate — fix: git-runner seam for per-path diffs; seams for pack write; one test per error branch.
- (attempt 2, Data-migration) [P1] [85] [blocking] Committed results bound to transient machine-local runs.jsonl → on any other clone/CI every committed result is the blocker "unknown pack run" — fix: ignored-not-blocking when the run is unknown, or keep results local; say which in §5.2/§7/§11.
- (attempt 2, Data-migration) [P1] [85] [blocking] §11 migration not executable: Reconcile fixes only rows in the previous-run chain/reset set; runchain refuses --previous-run on an escalated chain and a fresh chain at the same head; unchained pre-0.8.3 rows never reconcile — fix: escalated/unchained recipes or a legacy-fingerprint alias reconciled on first 0.8.3 run; test per state.
- (attempt 2, Data-migration) [P2] [80] [blocking] Result format "ReviewResult + provenance" but the struct has no JSON tags, lacks reviewedAt/shardHash/planHash/packRunId/fileHashes/note, still carries SourceManifestHash; schemaVersion 2 collides with the shared SchemaVersion=1 — fix: explicit tagged wire schema; say whether Manifest stays 1.
- (attempt 2, Data-migration) [P2] [70] [blocking] Reason split silently changes epic-ready fingerprints (epic:context-risk:<reasons>) → orphaned open row + duplicate — fix: reason-independent epic fingerprint (or alias); name in §11.

## Advisory Findings

- (attempt 2, Completeness) [P2] [75] [advisory] task-done targeting a docs/metareview/** path uses CollectWithExcludesExcept (exact-list minus target) → B, shard set and reviewed diff diverge — state the exclude set per caller; test.
- (attempt 2, Completeness) [P2] [50] [advisory] .metareview/shards/ self-ignore asserted not specified (main has no runs/ precedent; only learning.EnsureLearningGitPolicy writes ignores) — specify content, writer, creation point, prune preserves it.
- (attempt 2, Scope and alignment) [P2] [75] [advisory] Real target: filtered main…fsm-enhancements = 1,372,619 bytes over 133 files → ~23 shards / ~24 dispatches per round; lexical first-fit re-cuts later shards on any size change — record measured cost in §9; path-bucketed cut policy so one fix stales one shard.
- (attempt 2, Scope and alignment) [P3] [90] [advisory] §3 claims DIFF_OVERSIZE is listed in §2 goals; it isn't — add the line.
- (attempt 2, Scope and alignment) [P3] [60] [advisory] schemaVersion 2 vs shared reviewmanifest.SchemaVersion=1 used for both Manifest and results — name a distinct ResultSchemaVersion=2; Manifest stays 1.
- (attempt 2, Architecture) [P2] [75] [advisory] Provenance ownership unstated: reviewmanifest is pure (imports contextprofile only) yet §5.1 puts packRunId validation there; run-row has four writer structs + one reader; none named for planHash — fix: caller injects KnownPackRuns/PackRunPlanHash into reviewmanifest.Input; name every run-row struct gaining the field.
- (attempt 2, Architecture) [P3] [50] [advisory] v2 hash inputs lack length prefixes/NUL delimiters; `paths` join undefined; newline in path forges entries — fix: length-prefix or NUL-delimit; newline collision test.
- (attempt 2, Intent preservation) [P2] [60] [advisory] fixed/false-positive remain unverified self-declared closures of critical findings — render such closures of high/critical in a blocking-adjacent section.
- (attempt 2, Intent preservation) [P2] [50] [advisory] MaxBytesPerShard (60,000) not tied to maxDiffBytes (120,000); nothing forbids a shard budget above the cap — invariant MaxBytesPerShard ≤ maxDiffBytes with a test.
- (attempt 2, Testing-quality) [P2] [75] [advisory] TestPackWriteFailureRollsBackRun tests the direction that can't leave debris; failure after rename leaves populated dir — fix: test failing the review-markdown write after packs; extend restore to the dir.
- (attempt 2, Testing-quality) [P3] [70] [advisory] Unfalsifiable/mislocated oracles: TestPlanComputedOnceAndThreaded (no seam to count), golden constant captured from impl, cmd/metareview has no Go tests (exit-2 → shell suite), DIFF_OVERSIZE "no packs written" untested with no lowerable bound — fix as stated.
- (attempt 2, Data-migration) [P2] [80] [advisory] §11 omits the runs.jsonl row change (planHash/shardHashes; four duplicated writer structs; 0.9.0 adds a fifth) — specify keys, fail-closed semantics, schemaVersion stays 1.
- (attempt 2, Data-migration) [P2] [75] [advisory] Ported gate doesn't enforce shardpack 100% (case arm only for packages present; REQUIRED lists fsm/workflows; floor awk) — name all three sites.
- (attempt 2, Data-migration) [P3] [75] [advisory] learnsource excludes broader than stated (all .metareview/** and docs/metareview/**; rawDiffBytes semantics flip) — narrow to docs/metareview/shards/** or note in §11/CHANGELOG.

## Follow-up Findings

- (attempt 2, Feasibility) [P3] [75] [follow-up] Provenance binds durable results to transient runs.jsonl → PASS reproducible only on the writing machine; fresh checkout/CI permanently "unknown pack run" — record consequence or provide a durable non-forgeable receipt.
- (attempt 2, Scope and alignment) [P3] [70] [follow-up] Durable committed results are lineage-bound to local runs.jsonl → on other clones only "unknown pack run" — state audit-only; second machine regenerates.
- (attempt 2, Architecture) [P3] [50] [follow-up] tests:missing:* becomes reachable on sharded task-done with a fingerprint embedding every changed path → churning, never reconciles — fix: state behaviour, stable (hashed) fingerprint, test.
- (attempt 2, Intent preservation) [P3] [55] [follow-up] §11 one-time reconcile marks the renamed-fingerprint row `fixed` (false remediation feeding learning, learning/candidates.go:200-210) — distinct status/rationale or learning ignores them.
- (attempt 2, Security) [P3] [55] [follow-up] Caps under-specified: 65th result blocker or drop? discovery order; array length caps; pre-validation/re-open TOCTOU — fix: exceeding 64 is a blocker; lexical order; cap arrays; re-check at read time.
- (attempt 2, Data-migration) [P3] [70] [follow-up] Stale plan fields (Shard.SourceDiffHash, PromptPackPath) and largest-first cut order contradict §3 — state removal/reorder.

## Warnings

None.
