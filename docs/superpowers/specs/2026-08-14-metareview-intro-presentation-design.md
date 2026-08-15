# metareview Intro Presentation — Design Spec

**Status:** built. Deliverable: `docs/presentations/metareview-intro.html`
(single self-contained reveal.js file, ~198K, works offline — reveal.js inlined).

## Goal

A beautiful, single-file reveal.js deck (16 slides) that introduces `metareview` to a
developer who is fluent in agentic coding but may be new to metaswarm. They should leave
understanding what metareview does, how it works, how it grew out of metaswarm, and — from
real, sourced evidence — the bugs it has caught and the outcomes it produces.

- **Audience:** practitioners / devs (no metaswarm knowledge assumed).
- **Aesthetic:** terminal / technical — dark canvas, monospace identity, verdict chips,
  review-card and run-chain signatures. Multi-hue *semantic* palette (not one accent).
- **Evidence policy:** verbatim + quantified, conservative and descriptive. Every quote and
  stat is real and sourced; estimates are explicitly labeled; no causal claims the data
  (solo-dev repos, no control period) can't support.
- **Narrative:** factory → the review bottleneck → benefits + proof → what/how → evidence →
  learning → origin & fit → adopt.
- **Author footer (close slide):** David Sifry · david@sifry.com · github.com/dsifry/metareview · MIT.

## Final slide order (16)

1. **Title** — terminal boot: a `pr-ready` gate resolves to `PASS`. The `PASS` is an
   illustrative boot animation, not a record of a live gate run.
2. **First, the factory** — metaswarm's 11-phase build pipeline (Research → Plan →
   Plan Validation → Design Review Gate → Decompose → Dependency Check → Orchestrated
   Execution → Final Review → PR Creation → PR Shepherd → Close+Learn), review phases
   highlighted; plain-language "new to metaswarm?" intro; bridge line to metareview.
   (Sourced from metaswarm `docs/index.html` "The Pipeline".)
3. **The bottleneck moved** — framework-agnostic problem: agentic coding outpaces review;
   "please review this" doesn't scale. Before/after cards.
4. **Why metareview · what it catches** — 62 completions blocked (48 fixed) · ~120–240 hrs
   debugging deferred (est.) · ~$8.6k–17k recovered (est., $150k/yr ≈ $72/hr) · six bug
   classes. Caveat: hours/dollars are illustrative estimates.
5. **Outcomes** — 80% first-pass · 2% of 251 PRs drew changes-requested · +87,908-line PR
   merged clean · per-gate first-pass (epic 100% / task-done 86% / artifact 64% / pr-ready 59%).
   Caveat: descriptive, not a controlled trial.
6. **Proof at scale** — 1,705 sessions · sessions-by-repo bars · 5 repos · 2,911 fix-loop
   sessions · 73% hit a blocking verdict.
7. **What it is** — named gates · deterministic blockers · local-first vs hosted.
8. **The seven gates** — command reference; fractal loop.
9. **How one review runs** — the deterministic pipeline + real package names.
10. **Verdicts & blockers** — five verdicts; BLOCKER/ADVISORY/FOLLOW_UP; evidence receipts.
11. **Real catches I** — security & bounded context (eval, TODO-in-done, truncated-diff ESCALATED).
12. **Real catches II** — cross-model / cross-repo / caught-its-own-design-gap (2×2, fragments).
13. **The learning loop** — post-merge learning; verbatim accepted learning w/ provenance;
    discard discipline (PRs #3/#4 honestly yielded zero).
14. **Where it came from & where it fits** — metaswarm explained for newcomers; lineage
    (design/plan-review-gate → metareview); standalone or alongside metaswarm/Beads/Superpowers;
    thesis quote.
15. **How to get started** — quickstart terminal (install · task-done · pr-ready · --previous-run).
16. **Close** — meta-frame PASS review card for the deck itself; recap; author/MIT/GitHub block.

Eyebrow markers: title = `MRV`, content slides `01`–`14`, close = `✓`.

## Corrected evidence base (final numbers)

All numbers mined from this machine (`~/.codex/sessions/**`, `~/.claude/projects/**`,
each repo's `.metareview/runs.jsonl` + `findings.jsonl`, `docs/metareview/**`, `docs/specs/**`,
README, and live `gh` queries as `dsifry`). Two research corrections were applied.

**Reproducibility / auditability:** the ledger- and `gh`-derived figures (62 blocked, 48
fixed, 80.3% first-pass, per-repo PR stats) are reproducible from committed state — each
repo's `.metareview/*.jsonl` plus `gh pr`/`gh api` queries. The session-*scale* figures
(1,610 sessions, 2,911 fix-loops, 73%) are derived from machine-local session transcripts
under `~/.codex` and `~/.claude`, which are private and not committable; those are therefore
directional (file-presence counts), and the deck labels them as such.

- **"Issues caught" = 62** completions blocked (NEEDS_REVISION + ESCALATED) across the
  committed run ledgers of four repos (metareview, metafactory, theguide, warmstart-tng);
  62 high-severity blocking findings logged, **48 fixed**. This is a conservative *floor* —
  transient ledgers undercount (theguide has 7 local runs vs 827 sessions). The earlier
  one-repo "8" and the session-log "278 ESCALATED" were dropped: the latter is inflated by
  context re-printing and is not a defensible count.
- **Review cycles = 80.3% first-pass** across 315 reviewed runs; only 2 ever escalated.
  First-pass by gate: epic-ready 100% · task-done 85.7% · artifact 63.6% · pr-ready 58.5%.

Outcomes / PR complexity (via `gh`, all four repos are solo-dev):
- metafactory: 251 merged PRs, only **2%** ever drew a CHANGES_REQUESTED.
- Standout clean merges: warmstart-tng #140 (+87,908 / 204 files), metafactory #130
  (+40,182 / 89), #146 (+34,054 / 194), theguide #11 (+137,576 / 374).
- Median gated PR is substantial: metareview +7,640 / 27 files; theguide +5,528 / 18.

Value estimate (explicitly illustrative, not measured):
- 62 catches × 2–4 hrs each ≈ 120–240 hrs; at $150k/yr ≈ $72/hr → ≈ $8.6k–17k.

Scale (file-presence counts, directional; per-gate run-occurrence counts dropped as inflated):
- **1,610** Codex sessions across the five repos ran a gate (2026-05-26 → 2026-08-11):
  theguide 827, metafactory 558, metareview 173, warmstart-tng 51, repofortify 1 (sum = 1,610;
  the deck hero and the per-repo bars use this same total, so they reconcile). **2,911** used
  --previous-run. A blocking verdict appears in 73% of recorded sessions.

Verbatim catches used on slides (all sourced in-deck via card captions):
- `eval(input)` critical + `// TODO: add tests later` high — task-done-issue-2-wu3.
- 1.78 MB truncated diff → "task closure cannot be trusted" → ESCALATED — codex session 2026-08-02.
- "Codex's metareview missed it, my independent gate run caught it" — metafactory memory.
- "two unresolved high findings … in a stale worktree … nothing downstream ever saw them" — metafactory session.
- validation false-negative across 3 PRs → filed `dsifry/metareview#1`.
- `MRV-S2-BLOCKER-001` — metareview caught a gap in its own design — codex session 2026-05-26.

Origin thesis (verbatim): "What metaswarm brought to the software development process,
metareview should bring to the review process." — `~/.codex/history.jsonl`.

## Honesty caveats encoded in the deck

- No causal "fewer cycles vs. a baseline" claim — no ungated control; outcomes are descriptive.
- No "fewer *human* review rounds" claim — across ~830 merged PRs, `gh` review data shows no
  review submitted by a non-author human account (only bot reviewers + the author's own
  comments appear). Low human-round counts reflect team size, not metareview.
- Cross-repo CHANGES_REQUESTED rates are confounded (different bot-reviewer mixes); only the
  within-metafactory 2% fact is used.
- 62 is a floor; hours & dollars are labeled estimates.

## Visual system

- Tokens: bg `#0a0d12`, panel `#121824`; ink `#e9eef4`, muted `#8b98a9`. Semantic palette:
  PASS `#3fb950` · ADVISORY/NEEDS_REVISION `#d29922` · BLOCKER/ESCALATED `#f85149` ·
  structural cyan `#4fd0e0` · learning violet `#a371f7`.
- Type: system mono stack (`SF Mono`/`JetBrains Mono`) for identity/findings, system sans for
  prose — all system stacks so the file stays offline.
- Signatures: verdict chips, review cards (severity bar + sourced caption), the run-chain rail,
  and the meta-frame (deck bookended as a review of itself). Charts are single-hue magnitude
  bars, direct-labeled (per the dataviz skill; hover omitted for the projection medium).

## Build / maintenance

- Source of truth is the template + slide partials in the session scratchpad; the final file
  is produced by splicing inlined `reveal.css` + `reveal.js` into the template
  (`/*__REVEAL_CSS__*/`, `/*__REVEAL_JS__*/` placeholders).
- Reviewed slide-by-slide in a headless browser at 1280×720 during construction.
