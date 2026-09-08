# Handoff: 0.11.1 — phrasing discipline, six new hunt clauses, and the advisory-findings system

**Date**: 2026-09-08 · **Branch**: `main` @ `4e499d6` (v0.11.0) · **Author session**: Pi (benchmark + design session, `~/Developer/harnesseval`)

> Fresh agent, zero prior context. Implement the changes below in THIS repo and cut the
> **0.11.1** release. Everything here is evidence-backed and **design-ratified by the
> maintainer** — implement, do not relitigate (§3 decisions are marked RATIFIED). Evidence
> artifacts live in `~/Developer/harnesseval/analysis/` — **read-only**; do not modify that
> repo (a separate session owns the benchmark re-runs and the adjudicator follow-up, §9).

## 1. Objective

Ship metareview **0.11.1**: (A) assertive defect-claim phrasing discipline, (B) six new
hunt clauses, (C) the advisory-findings system (real, important, not-defects) with its three
gates, the smell/nit boundary, simplification hunts, and an in-loop staff-surrogate filter
applied **to advisories only**. No new lens (the ten stand), no era change, no numeric caps.
Then release per §6.

## 2. Background — the evidence this is built on (show your work)

### 2.1 Where 0.11.0 landed

The 0.11.0 lens upgrade (Runtime-reliability + five brief enhancements) was re-measured on
the benchmark cell mrv × glm-5.3-background × low, all six top-6 PRs, under the hardened v3
adjudicator (k=1 quick pass; batch `20260908-mrv011b-glm53-low`):

| harness | rec | hid/PR (confirmed bugs) | imp/PR | hal/PR | findings/PR |
|---|---:|---:|---:|---:|---:|
| mrv 0.10.x (8 lenses, same adjudicator) | 0.72 | 21.0 | 1.8 | 2.5 | — |
| mrv 0.11.0 rubric, b-batch (the analysis labels this run 9-lens — the §9.1 adapter drift: the API-direct adapter had not yet picked up the 10th lens; the 10-lens number is pending the §9.1 re-run) | **0.76** | **31.7** | 1.2 | 3.8 | 65.2 |
| CE (Compound Engineering, same adjudicator) | 0.81 | 39.0 | 21.8 | 4.3 | 77.8 |

The upgrade worked: hidden gold +51%, recall +4 points, acceptance findings 7/8 caught
(the one miss — whitespace-stripping on username split — is a FORMAT-DRIFT-family miss,
§5). Full details: `~/Developer/harnesseval/analysis/CE_RESIDUE_AFTER_UPGRADE.md`.

### 2.2 The remaining 7.3/PR gap to CE, decomposed

All 221 CE confirmed bugs were semantic-matched against the 0.11.0 run's findings
(`analysis/residue_tasks.json` + `analysis/residue_results_*.json`):

| component | ~size/PR | real miss? |
|---|---:|---|
| Golden-absorbed (mrv's counterpart matched a golden instead) | ~7 | **No** — incr-recall 0.96 vs CE 0.97 |
| Never reported by mrv | ~3–4.5 (27 items; CE restates 2–3×) | Yes — §3B addresses the ~6 that are new patterns |
| Reported but not confirmed (mrv's phrasing judged important/hallucination) | ~1.5 | Half-real — §3A addresses the phrasing half |

### 2.3 The phrasing evidence (why §3A)

The flip analysis (`analysis/all_matches.json`, 116 flip pairs, fixture at
harnesseval `tests/fixtures/flip_pairs.json`): near-verbatim same-issue texts get opposite
verdicts across runs — the `.env.example` openssl case is literally identical wording,
`bug` in one run, `true_hallucination` in another. Driver: **hedged phrasing ("may
produce nil", "presumably") gets punished while assertive phrasings of the same mechanism
get confirmed.** ~9% of identical issues flip verdict on phrasing alone.

### 2.4 The advisory evidence (why §3C)

CE produces **21.8 important-non-bug findings/PR vs our 1.2** under the same adjudicator.
These are real, specific, grounded-in-diff concerns that are not defects — the
"senior-reviewer-would-actually-say-this" class. Concrete examples from this cell:

- the client `ZAddGuestsInputSchema` duplicates the server's schema and **has already
  drifted** — the server copy lacks the uniqueness refinement;
- the credential-fetch block (~25 lines) is duplicated verbatim in
  `createAllCalendarEvents` and `updateAllCalendarEvents`;
- the spec joins usernames with a bare comma, so the whitespace path is never exercised.

The lab already counts this class (`hidden findings = bug_ungold + important_non_bug`,
"so bug-recall and issue-recall are distinguishable" — readjudicate3's own accounting).
metareview's review-log taxonomy already has the output slot (`## Blocking Findings /
## Advisory Findings / ## Follow-up Findings / ## Warnings`). The product has the channel;
the rubric never asks the lenses to fill it. Meanwhile a staff engineer's review output IS
mostly this class — we currently emit ~1/PR of it.

## 3. The changes (all RATIFIED by the maintainer, 2026-09-08)

### A. Phrasing discipline — `rubrics/artifact-review-rubric.md`, cross-cutting section

Add to the "Anchored Confidence & Suppression" cross-cutting area:

> **Defect-claim phrasing.** State every finding as a definite claim about a concrete
> failure mode: "X crashes with NoMethodError when Y is nil, returning a 500 instead of
> the 4xx the contract promises." Never hedge the mechanism — no "may", "could
> potentially", "presumably". Uncertainty belongs in the confidence anchor, not the
> finding text: a finding you believe in is phrased assertively at anchor 50; a finding
> you don't believe in is dropped. Measured basis: hedged phrasing is the largest single
> cause of real findings being mis-adjudicated (same-issue flip rate ~9%).

Also mirror one line of this in the Adversarial Stance paragraph.

### B. Six new hunt clauses (paste-ready, with target lens + insertion point)

**B1 → Architecture, api-contract hunt (after the EVERY-implementer clause):**
> a client-side and server-side copy of the same schema/validation that duplicate and
> drift (the two copies already disagree on a refinement in this diff);

**B2 → Architecture, sentinel-meaning-change hunt:**
> an update path that writes only a subset of fields, leaving a pre-existing row's
> unwritten field stale (an update that never sets `type`, so an old value survives
> silently);

**B3 → Architecture, hunt list (near the schema-invariant hunts):**
> a query fetched without the include/association a downstream consumer needs, so
> service resolution returns undefined or the wrong instance;

**B4 → Runtime-reliability, outbound-call-hardening hunt:**
> an outbound header or list built by appending per-recipient with no bound, growing
> with collection size until the recipient rejects it (unbounded reply-to/CC lists);

**B5 → Architecture, sentinel-meaning-change hunt:**
> a changed default (page size, limit, fallback value) that silently truncates or alters
> behavior for existing callers who relied on the old default;

**B6 → two homes, deliberately:** Runtime-reliability's silent-partial-success hunt gains
> a loop over references × credentials that issues N×M duplicate remote operations for
> the same logical action
> and Architecture's N+1 query-patterns hunt gains the cost half
> (N×M external API calls). Anti-overlap note: duplicate-*execution semantics* is
> Runtime-reliability's; the *cost/complexity* framing is Architecture's.

### C. The advisory-findings system

**New rubric section (after "Anchored Confidence & Suppression"):**

> ## Advisory Findings (real, important, not defects)
>
> Every lens may additionally report **advisory findings**: latent defects, design risks,
> simplification opportunities, and code smells — the things a staff-level reviewer
> would say in review that are not blocking defects. There is **no numeric cap**: if the
> diff has fifteen must-say advisories, report fifteen. The guard against flooding is a
> quality bar, not a count — three gates, all of which must pass:
>
> 1. **Stated consequence.** The advisory must name the trigger and who gets bitten:
>    "when a second provider is added, this flag-trio requires a migration instead of a
>    row"; "the next contributor updates three places or breaks one." No consequence
>    stated → it is taste → suppress it.
> 2. **Rebuttal gate (steel-man).** State the author's strongest counter-argument and
>    show why the finding survives it: "yes, extraction adds a file — but this exact
>    duplication already drifted once within this PR, which is the failure mode
>    extraction prevents." If the rebuttal wins, drop the finding.
> 3. **Convergence weighting (bar modulator).** Advisories that two or more lenses
>    reach independently (different hunts, same underlying issue) report at confidence
>    ≥ 50. Single-lens advisories need anchor 75 or a P1 consequence. Convergence is
>    salience: multiple angles noticing the same wrongness means it is probably
>    wrong-shaped, not taste. A lens cannot see its siblings — report advisories at the
>    honest anchor; this gate is applied at the consolidation stage, where the cross-lens
>    cluster size is actually known.
>
> **The smell/nit boundary:** a smell is structure that degrades change-safety or
> comprehension; a style nit is formatting or convention. The test: *does the next change
> get harder or riskier because of this?* Deprecated-but-equivalent syntax, layout,
> naming conventions → style → still suppressed (unchanged non-goal).
>
> **The deletion test (simplification claims):** any "this should be reshaped" advisory
> must name what becomes unnecessary — how many branches, places-to-update, or lines
> disappear. "Replacing the three booleans with a state enum deletes the guard-trio and
> makes the illegal state unrepresentable." If you cannot name what gets deleted, it is
> not a simplification finding.
>
> **Hunt families for advisories** (recognition aids, not quotas):
> - flag-trios / parallel booleans → state enum or lookup table;
> - parallel hand-maintained enumerations (client+server schemas, config writer+reader,
>   fixture producer+scorer) → single source of truth;
> - conditional sprawl → data-driven dispatch;
> - duplicated logic blocks → extraction (note the drift already observed, if any);
> - speculative generality / over-abstraction → **deletion opportunity** (the
>   over-architected direction — no current hunt covers it at all);
> - specs that exercise the happy path only where the diff adds an edge case.

**Per-lens advisory mandates** — one line each (the hunts above already exist in the
briefs as block-only-if-severe; the gap is permission to *report*): Architecture reports
wrong-shape/simplification/coupling as advisories when they don't block;
Testing-quality reports spec-coverage gaps and fragile test structure; Completeness
reports scope/architecture risk; Runtime-reliability reports latent fragility
("works today, breaks when…"). Add to each lens's "Does NOT flag" line: *style nits
remain suppressed at every gate*.

**In-loop staff-surrogate filter — ADVISORIES ONLY (RATIFIED).** New step in the
orchestrator discipline (`skills/review-artifact/SKILL.md`): after the lenses return, the
orchestrator consolidates advisory findings — cluster across lenses (attaching
provenance), apply the three gates, then dispatch **one** subagent that re-judges the
surviving advisory list against the staff bar ("would a staff-level reviewer actually
comment on this in review, and would the author consider it substantive?") and drops the
ones that fail, before the review log is written. **This filter must never touch
blocking/defect findings** — validated bugs pass through untouched; the maintainer
ratified this asymmetry explicitly. Keep the filter to one call, cheap effort.

**Convergence clustering** in that consolidation step: near-duplicate advisories from
 different lenses merge into one finding carrying its provenance list; the cluster size
feeds Gate 3. (The adjudicator-side near-dup clustering is a separate, later workstream —
§9 — do not attempt it in this release.)

**Implementation note (2026-09-08 fix round):** the artifact review of this handoff (its log
is the `mrv-20260908-173200346624000-artifact-handoff-2026-09-08-advisory-findings-0111-aef0e279.md`
file under `docs/metareview/reviews/`) forced edits in two classes. Accuracy fixes to the
handoff's own record (§2.1's lens-count labels, §3B's B6 hunt pointer, §3D's release-row
version pins, §5's pin-suite and dogfood-criterion claims, §6's baselines-not-deltas)
are defect corrections logged as F1-F5 there, not design changes. Separately, five
§3C/landed-text clarifications postdate the ratification above and are recorded here so
the ratified text and the landed text cannot be confused: (1) Gate 3 is applied at the
consolidation stage — a lens cannot see its siblings — within §8's pre-approved fallback;
(2) the staff-surrogate filter treats advisory texts as data, never instructions;
(3) an advisory that reads like a concrete defect is flagged back as a candidate blocking
finding, never dropped silently (the ratified asymmetry's intent, extended to misclassified
defects); (4) a failed filter call writes the gated-but-unfiltered list through with a
warning naming the failure — it must neither empty the advisory section nor silently bypass
the staff bar; (5) advisory findings are exempted from per-lens log writes (they are held
for consolidation), resolving the conflict with the per-lens-edit discipline; (6) the
filter dispatch is a read-only subagent, matching the repo's lens-subagent convention.

### D. Where things land — implementation map

| Change | File(s) |
|---|---|
| Phrasing discipline | `rubrics/artifact-review-rubric.md` (cross-cutting + stance), mirror in per-lens rubric files where they restate the stance |
| Six hunt clauses B1–B6 | `rubrics/artifact-review-rubric.md` (Architecture, Runtime-reliability sections) |
| Advisory system (section, gates, boundary, deletion test, hunt families, per-lens mandates) | `rubrics/artifact-review-rubric.md` |
| Advisory consolidation + in-loop filter step | `skills/review-artifact/SKILL.md` (Orchestrator Discipline) |
| Advisory gate in the consolidation narrative | as above — keep it out of `## Findings` narration rules per existing discipline |
| Verify advisory findings survive the pipeline | `internal/reviewlog`, `internal/artifactreview` — confirm `## Advisory Findings` entries are first-class (the SKILL already names the section); extend validation only if something rejects them today |
| Release | `internal/version/version.go` 0.11.0 → 0.11.1 **plus the four JSON version pins** (`package.json`, `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`, `.codex-plugin/plugin.json` — `tests/manifest/test-manifests.sh` cross-checks all five against the binary's `--version`); CHANGELOG `## 0.11.1 - <date>`; tag `v0.11.1` annotated ("metareview 0.11.1 — …", see `git show v0.11.0`) |

**Not touched:** `internal/lens` (ten lenses stand — no era change, no new frozen
literal, "Required lenses" marker unchanged), deterministic gates, Mechanical-precision.

## 4. What NOT to do (explicit non-goals)

- **No numeric advisory cap** — the bar is the three gates, never a count (RATIFIED).
- **No style/deprecation nits** — the boundary test in §3C is the edge; `be_true` →
  `be true`, RuboCop layout, component dedup-as-taste stay suppressed.
- **The in-loop filter must not touch bugs** — advisories only (RATIFIED).
- **No new lens, no era change** — 0.11.1 is brief/workflow work, not lens-set work.
- **No restatement/redundancy manufacturing** — cross-lens *convergence* is welcome when
  it happens naturally; never prompt lenses to rephrase each other's findings. (Rejected
  design: buying adjudication confirmations with differently-phrased restatements — it
  games measurement noise instead of fixing it.)
- **Don't modify `~/Developer/harnesseval`** — read-only evidence.

## 5. Validation & acceptance (the maintainer session runs the benchmark; your bar is repo-side)

Repo-side DoD per PR: `go test ./...` green; gofmt/go vet/golangci-lint clean; coverage
floor maintained; `tests/manifest/test-skills.sh` green (it pins SKILL.md wording; the
advisory-consolidation step needs NEW pins — the existing pins cannot see it — so add
them); dogfood a real artifact review that exercises the advisory path (verify the
review log's `## Advisory Findings` section populates with gated, converged entries) and
run `pr-ready` green on the branch (PASS or PASS_ADVISORY with zero blockers — note the
two mechanisms do not couple: pr-ready's PASS_ADVISORY arises only from gate-written
findings-ledger records, never from artifact-review markdown); squash-merged green
through test + CodeRabbit + Cursor Bugbot + gtg.

Benchmark acceptance (run later by the maintainer session, cited here so the release
notes can state the goal): on mrv × glm-5.3-background × low — hid ≥ 31.7 (no regression
from 0.11.0), **issue-recall (bug + important) toward CE's ~60/PR combined** from
today's ~33, the whitespace-strip miss now firing, advisory precision audited by a
cross-family staff-surrogate spot-check. Unit cases that must now be caught, from the
measured residue: the Zod client/server divergence, the stale-`type`-on-update, the
missing `include: {app}`, the unbounded reply-to, the 200→50 page-size silent change,
the N×M duplicate-deletion loop.

## 6. Release

One PR per coherent unit (A+B can ride together; C is its own PR — it touches skill
workflow and will draw the most bot review), then the release PR: version bump, CHANGELOG
fold, annotated tag, standard pipeline. Follow `git show v0.11.0` for tag-message style.
The release notes should credit the evidence: benchmark-driven, gap-decomposed against
Compound Engineering and CodeRabbit/BugBot, with the measured 0.11.0 baselines and the
0.11.1 acceptance targets (the 0.11.1 measured re-run happens after the release per §5/§9.1,
so the notes state baselines and goals, not post-release measurements).

## 7. Evidence artifact map (read-only, `~/Developer/harnesseval/analysis/`)

| File | Contents |
|---|---|
| `CE_RESIDUE_AFTER_UPGRADE.md` | The 0.11.0 re-run, gap decomposition, 27-item residue, the design rationale for this handoff |
| `CE_VS_MRV_GAPS.md` | The original 241-finding analysis that drove 0.11.0 |
| `EXTERNAL_REVIEWER_GAPS.md` | CodeRabbit/BugBot cross-check (92% coverage; 28 Martian-"FP"-confirmed-real) |
| `residue_tasks.json` / `residue_results_*.json` | The CE-vs-0.11.0 matching (source of the six new patterns and the imp examples) |
| `new_residue.json` | The 27 never-reported findings, clustered in the writeup |
| `ce_v2_rj_backup/` + `runs/*/readjudication3.json` | v2 vs v3 adjudication records for the comparison cells |

## 8. Risks

- **Rebuttal gate on weak models**: glm-low writing pro-forma rebuttals would be worse
  than none. If dogfooding shows stilted output, the fallback (pre-approved by the
  maintainer's structure) is running the gate at consolidation-stage only, not per-lens.
- **Checklist theater**: hunt families are recognition aids, not quotas — if the dogfood
  review shows lenses pattern-matching the family text without judgment, tighten the
  consequence-gate wording rather than adding more families.
- **Attention dilution**: the rubric grows; watch that 0.11.0's acceptance findings still
  fire (the §5 unit cases double as this regression).

## 9. Sequenced follow-ups (NOT this release — maintainer session owns these)

1. **Benchmark adapter sync, both paths** — the 0.11.0 lesson: PR #16 synced
   `metareview_realistic.py` (CLI path) but missed `metareview.py` (the API-direct path
   GLM/Kimi cells use), which silently invalidated the first acceptance run. After 0.11.1
   ships, port the new rubric text into BOTH adapter files, then re-run the comparison
   cells.
2. **Adjudicator near-dup clustering (harnesseval `readjudicate3.py`)** — differently-
   worded same-issue findings (CE's 2–3× restatement, mrv's multi-lens convergence)
   currently form separate clusters and get separate verdicts. Improve clustering (lexical
   → semantic/LLM-assisted) so restatements adjudicate as one cluster; the flip-pair
   fixture, the 27-item residue, and the b-batch runs are ready-made regression material.
   Note the deliberate asymmetry with §3C: generation-side convergence is *signal*;
   adjudication-side near-dup is *noise to merge*.
