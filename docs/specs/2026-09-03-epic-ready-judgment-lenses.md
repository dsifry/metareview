# Spec: epic-ready judgment lenses (require an adjudicated review, like build B)

Status: **approved** (maintainer sign-off 2026-09-03, with the lens-independence caveat folded into choice 5),
after a 9-lens adversarial artifact review — see §"Review history". Owner: this branch
`epic-ready-judgment-lenses`. Fast-follow to
`docs/specs/2026-09-03-require-adjudicated-review.md` (build B), which listed epic-ready lenses as a non-goal /
fast-follow.

## Problem

`review epic-ready` is the **last mechanical pass in the review family.** Build B made `pr-ready`/`task-done`
require an adjudicated adversarial lens review to PASS; `epic-ready` was explicitly left out. It runs in
`ExecutionMode: deterministic-local` (`internal/epicready/review.go:192`) and its "reviewers"
(`architecture-reviewer`, `epic-integration-reviewer`, `acceptance-reviewer`, `intent-preservation-reviewer` —
`reviewerNames`, `review.go:79`) are **heuristics** in `internal/reviewers/epicready.go`:
`hasEvalContradiction` (literal "use eval" vs "no eval" string match), `violatesNoEvalIntent`,
`missingChildEvidence`, `unresolvedChildBlockers`, `missingServiceInventoryCoverage`, plus the context-risk
short-circuit. These catch a narrow, hard-coded set (the eval contradiction is a demo pattern, not a general
contradiction detector) and never invoke a model. So an epic closes — declared integration-ready — with **no
adversarial judgment** over whether the children actually cohere, whether the delivered whole preserves the
parent intent, or whether a cross-child regression slipped between the per-child reviews. ARCHITECTURE §4
already names the intended shape: a one-shot `review-loop` over the roll-up.

## Goal

Make an epic-ready `PASS` require that an **adjudicated adversarial review ran over this epic's integration
diff at this head** — reusing build B's review-evidence marker and gate reviewer, not a third mechanism —
while keeping the cheap deterministic heuristics as fast structural pre-checks (exactly as `pr-ready` kept its
structural checks under the new adversarial requirement). Provide the same labeled `in-session-emulated`
weaker-evidence escape hatch, and the same `METAREVIEW_ALLOW_MECHANICAL_PASS=1` opt-out for the migration
release. After this change, `task-done`, `pr-ready`, and `epic-ready` all block until an adjudicated review is
recorded for their exact diff — **one marker mechanism across all three scopes.**

**Maintainer requirement (sign-off caveat, 2026-09-03):** the epic review's lenses must be able to **diverge
from — and add to — the pr-ready/task-done adversarial lenses**, not move in lockstep. We need not use
different lenses on day one, but the *capability* must be in place: an epic-specific rubric and lens set that
can evolve without touching the shared review machinery. This is a first-class design constraint, resolved in
choice 5.

## The central design decision: what the marker attests vs. what the pre-checks guard

The review that first drafted this spec proposed making the adjudicated lenses review *the rendered roll-up
context pack* (child review logs + evidence + parent intent), with the integration diff as mere context. The
adversarial review of that draft found this **architecturally unsound** (findings A1/F1): build B's marker is
keyed on `base..head` currency, but the roll-up lives **outside** the diff — child review logs are excluded
from it and read out-of-band (`reviewlog.Discover`, `review.go:105`; excludes at `:456`), the evidence file is
read fresh each run (`readEvidence`, `:113`), and a Beads epic's intent is not in the diff at all. Any of
these can change while `base..head` stays fixed, so a marker keyed on `base..head` can neither *attest* nor
*track* the roll-up — and `review-loop.yaml` has no seam to feed a review node a context pack anyway; it
reviews the diff. Binding `subagent-adjudicated` (the strongest evidence mode) to a roll-up review it cannot
actually perform would launder a code-diff review as roll-up judgment.

**Resolution — split the two concerns along the grain of the two engines already in the code:**

| Concern | Guarded by | Currency |
|---|---|---|
| **Roll-up freshness** — a child flipped to NEEDS_REVISION, evidence regenerated, intent edited, a child missing evidence | the **deterministic pre-checks** (`missingChildEvidence`, `unresolvedChildBlockers`, `hasEvalContradiction`, `violatesNoEvalIntent`, `missingServiceInventoryCoverage`) | **re-read every run** — `Create` re-discovers logs/evidence/children/knowledge on each invocation (`review.go:101-117`), so these are always current by construction |
| **Integration judgment** — do the children's changes cohere as one whole; cross-child regression; acceptance-vs-intent over the delivered code | the **adjudicated adversarial review** attested by the base..head marker | **base..head** — correct, because the integration diff *is* what the review adjudicates |

This is the key insight the review surfaced: epic-ready's roll-up freshness is **already** continuously
guarded — the pre-checks re-read current state on every gate run and block independently of any marker. So the
adjudicated review does not need to attest the roll-up; it needs to add *judgment over the integrated code*,
whose natural, attestable, `review-loop`-reviewable surface is the **integration diff** (the union of the
children's changes as one diff), reviewed **with the roll-up supplied as reviewer context** (child verdicts,
parent intent). base..head currency is then exactly the right invariant, and `subagent-adjudicated` honestly
attests the review `review-loop.yaml` performs.

## The four design choices (resolved)

### 1. What surface does the adjudicated review attest? → the integration diff, read *with* the roll-up as context

The **attested** surface is the integration diff: `git.BaseSHA..git.HeadSHA` that `epicready.Create` already
computes (`review.go:196-197`) via `gitcontext.CollectWithExcludesExcept(root, options.Base, …)` — the union
of the children's changes, the divergence-point-to-tip span on the epic branch. The lenses review that diff
*as one whole* (the cross-child view no per-child task-done review has, since each saw only its own diff),
with the roll-up — child review verdicts, parent epic intent — provided as reviewer context so the judgment is
cross-child, not a re-audit of each child line. The roll-up is **context that informs** the diff review; the
roll-up's own freshness is guarded by the pre-checks (above), not by this marker. This resolves the
roll-up-vs-diff tension (F1/A1) and the "marker binds the diff, not the lens surface" security note by making
the diff the attested surface *by design*, and documenting the division of labor explicitly.

### 2. Reuse build B's require-lenses gate? → yes, verbatim, keyed on the epic integration span — with a base-consistency rule

Wire `RunEpicReady` exactly as `RunPRReady`/`RunTaskDone`:

- Add `RequireLenses bool` and `Adversarial AdversarialReviewStatus` to `reviewers.EpicReadyContext`.
- Append `adversarialReviewFindings(context.RequireLenses, context.Adversarial)` in `RunEpicReady`
  (`internal/reviewers/adversarial.go` — already scope-agnostic, `(require, status)` only, no change).
- In `epicready.Create`, set `reviewerCtx.RequireLenses = reviewstate.RequireAdjudicatedReview()` and resolve
  `reviewstate.LatestReviewEvidence(root, "epic-ready", git.BaseSHA, git.HeadSHA)` into
  `reviewerCtx.Adversarial` (Present/Verdict/Emulated), mirroring `prready/review.go:198-212`.

The recorder (`reviewstate.RecordReviewEvidence`) is already scope-agnostic — `reviewedScope: "epic-ready"`
round-trips today with no code change (`reviewevidence.go:52-98`); only the `record-lenses` CLI allow-list
rejects it (choice 4).

**Base-consistency rule (finding C3 / base-asymmetry HIGH).** The marker and gate agree *only* when the same
resolved base SHA is used on both sides. The trap: `review epic-ready` with **no** `--base` defaults to
`merge-base(HEAD, main)` (the divergence point, `gitcontext.go:329-344`), while the recorder invoked as
`record-lenses --base main` resolves `rev-parse main^{commit}` (the **tip** of main, `gitcontext.go:310`).
Once main advances past the branch point these differ, so a correctly-recorded marker would never match and
epic-ready would block **forever**. Rule, to be documented in CLAUDE.md/AGENTS.md and enforced by the recipe:
drive epic-ready with an **explicit `--base`** and pass the **identical** value to `review epic-ready`, `fsm
--workflow review-loop`, and `record-lenses --scope epic-ready`. (Ergonomic aid, optional: have `review
epic-ready` print its resolved base SHA so the agent can echo it verbatim.) A behavioral test with main
advanced past the branch point pins this (see Test plan).

gateEffect is unchanged: epic-ready is `advisory` in a plain repo, `gate` for Beads/Metaswarm
(`review.go:134-137`). A *blocking* adversarial finding still flips the verdict to a blocking verdict
regardless of gateEffect — `verdictForCounts` returns `NEEDS_REVISION`, or `ESCALATED` at the attempt cap,
whenever `counts.Blocking > 0` (`review.go:566-575`). The advisory-repo distinction only governs whether
*non-blocking* findings downgrade PASS to PASS_ADVISORY.

**Working-tree currency (findings C1 — HIGH, both reviewers).** Correcting the first draft's false claim:
`epicready.Create` folds staged + working-tree + untracked content into the reviewed surface
**unconditionally** (`reviewerContext` joins `git.Diff, git.StagedDiff, git.WorkingTreeDiff,
git.UntrackedExcerpts`, `review.go:242`; rendered at `:512`) — with no `IncludeWorkingTree` toggle, so it is
*more* exposed than pr-ready, which folds the tree only behind that flag. The base..head marker attests only
the committed span, so a dirty-tree epic-ready run must not be credited by it. Fix: resolve
`Adversarial.WorkingTreeUnattested = true` when the epic tree carries uncommitted content
(`len(git.StagedFiles)+len(git.WorkingTreeFiles)+len(git.UntrackedFiles) > 0`), letting the existing
`working-tree-unattested` block in `adversarialReviewFindings` fire (mirror `prready/review.go:203`). This
preserves build B's working-tree hardening rather than re-opening the hole. A clean tree (the normal
post-merge epic case) sets it false and passes.

### 3. Lens set → keep the heuristics as deterministic pre-checks, add the adjudicated review on top; the rubric is a deliverable

**Keep, don't replace.** The existing heuristics stay as cheap structural pre-checks — and they *are* the
roll-up-freshness guard of choice 1, so they earn their keep beyond "occasionally catch a hard-coded
contradiction." The adjudicated adversarial review is layered on as the substantive integration judgment,
mirroring how `pr-ready` kept its structural checks and added `adversarial-review-reviewer`. Recommended lens
names to pass to `--lenses`:

- **epic-integration** — do the children compose into a coherent whole? Seams, duplicated/contradictory
  abstractions, an interface one child exports that another consumes incompatibly.
- **acceptance-vs-intent** — does the delivered whole satisfy the parent epic's stated acceptance criteria and
  intent, not just each child's local acceptance?
- **cross-child-regression** — did a later child break an earlier child's behavior in a way no single-child
  task-done review could see (each reviewed only its own diff)?
- **architecture-coherence** — durable service/codepath/registry coverage and structural consistency across
  the epic.

**Rubric is an explicit deliverable (finding S1).** `rubrics/epic-ready-review-rubric.md` today encodes only
the *old* heuristic checks (contradiction / child-evidence / child-blockers / intent-drift / registry). It
must be rewritten to prompt the four cross-child-judgment lenses above, or the lenses ship in name only. The
recorded marker's `lensSet` is agent-supplied and is **audit metadata** — the gate requires the marker
present+passing, not that any particular lens name appears — so the substance lives in the rubric prompt, not
in new gate code. The heuristics are not deleted; their fingerprints keep working.

### 4. FSM vs one-shot; who drives it? → the agent drives a one-shot `review-loop` over the integration diff, bridges via `--from-run`

Unchanged from the first draft, and now fully consistent with choice 1 (the attested surface is the diff,
which is exactly what `review-loop` reviews): epic-ready is **not** a fix-loop (ARCHITECTURE §4); the agent
drives a one-shot `review-loop` (discover → adjudicate → done) over the integration diff and bridges it into a
marker. `epicready.Create` does **not** invoke the FSM — it stays a pure, model-free, hermetically-testable
deterministic function, keeping ONE bridge pattern across all three scopes.

- Full independent review: `metareview fsm --workflow review-loop --base <epic-base>` over the integration
  diff, then `record-lenses --scope epic-ready --mode subagent-adjudicated --from-run <run>` (identical
  `--base`, per the base-consistency rule). `validateFromRunDiff` already checks the run's init reviewed the
  same `base..head` and reached a passing terminal transition (`main.go:640-688`), so `subagent-adjudicated`
  works for epic-ready the moment the scope is allow-listed — no FSM change.
- Subagents unavailable: `--mode in-session-emulated` (honest, advisory) after an in-session multi-lens read.

The workflow the agent drives is the epic-specific one from choice 5 (its `discover` node applies the epic
rubric), not the raw `review-loop.yaml` (which applies the task-done rubric).

### 5. Independent, extensible lens configuration — the epic lenses do NOT move in lockstep (maintainer caveat)

**The coupling to break.** Today the FSM `review-lenses` node hard-codes what it applies:
`const Rubric = "rubrics/task-done-review-rubric.md"` (`internal/fsm/kind/kind.go:56`) and its lens personas
come from the single canonical set `var Lenses = lens.Displays()` (`kind.go:53`, from `internal/lens.All`);
the node's only param is a lens **count**, not a rubric or a set (`validateLenses`, `kind.go:311-324`;
`Instructions` interpolates `Rubric`+`Lenses[:count]`, `kind.go:340`). So driving `review-loop` for an epic
today would review the integration diff **through the task-done rubric** — the exact lockstep the maintainer
flagged: epic and task-done lenses could not diverge, and changing one would change the other.

**The seam (build it now, default unchanged).** Parameterize the rubric the `review-lenses` node applies:

- Add an optional `rubric` node param (a repo-relative path). `validateLenses` accepts `rubric` (a string)
  alongside `lenses`; `Instructions`/`lensCount`'s sibling reads `n.Params["rubric"]` when present, else falls
  back to the `Rubric` const. Workflow node params already receive var substitution
  (`internal/fsm/workflow/resolve.go:67-83`), so a workflow can set it literally or via a `$RUBRIC` var. **Default
  is the current task-done rubric, so pr-ready/task-done are byte-for-byte unchanged.**
- Add `workflows/epic-review-loop.yaml` — a `review-loop` variant whose `discover` node sets
  `rubric: rubrics/epic-ready-review-rubric.md`. This is the single, epic-specific configuration point; the
  agent drives *this* workflow for epic-ready (choice 4). `validateFromRunDiff` binds base..head + a passing
  terminal transition and is workflow-agnostic, so `--from-run` credits an epic-review-loop run exactly as it
  credits a review-loop run — no CLI change.

**Why this satisfies "not in lockstep" and "add lenses over time":**

- The *substance* (what the lenses look for) is the **rubric**, now per-workflow. epic-ready applies
  `rubrics/epic-ready-review-rubric.md`; task-done applies its own. Editing the epic rubric to add a cross-child
  concern touches neither the task-done rubric nor `lens.All`. The two evolve independently — the coupling is
  gone.
- The recorded marker's `lensSet` is **agent-supplied per scope** (`--lenses`, already scope-independent), so
  the epic lens *names* (`epic-integration`, `acceptance-vs-intent`, `cross-child-regression`,
  `architecture-coherence`) are recorded and auditable for epic-ready alone, independent of pr-ready's names.
- **Adding a lens later** is a rubric edit (+ the recommended `--lenses` list in docs), never a change to the
  shared gate (`adversarial.go`), the marker schema, or the task-done rubric.

**Deliberately deferred (the seam leaves room, we don't build it now):** dispatching *named epic-specific lens
personas* from the FSM node (today the node fans out N generic adversarial subagents that each apply the given
rubric; the persona list is still `lens.Displays()`). The rubric seam already lets epic reviews look for
different things; if we later want the node to fan out *named* epic personas rather than a count, that extends
the same param surface (a `lensSet` param naming the personas) without disturbing the default. This is called
out so "capability in place" is honest about what is built (rubric divergence + per-scope recorded lens set)
versus what the seam merely makes reachable (named-persona dispatch).

### context-risk and large-epic interaction (finding C2)

`RunEpicReady` returns early on `RiskLevel=="context-risk"` (`epicready.go:77-89`), *before* the point where
`adversarialReviewFindings` would append. Two consequences to specify:

- **Placement:** the adversarial requirement need not (and should not) be appended after the early return
  where it is unreachable. context-risk already blocks (a high-severity `architecture-reviewer` finding →
  NEEDS_REVISION), so a context-risk epic is already gated; adding a redundant adversarial block under it
  buys nothing. Specify that context-risk **pre-empts by design** — the adversarial append goes on the normal
  (non-context-risk) path, and the review renders the context-risk block as the blocker. Keep the placement
  explicit in the implementation and a test.
- **Large-epic limitation (pre-existing, documented not fixed here):** epic-ready does **not** shard (context
  pack states "Sharding is not applied to epic-ready reviews", `review.go:510`), so a genuinely oversized
  integration diff hits context-risk with no shard-remediation path — unlike pr-ready/task-done. This is a
  pre-existing gap, out of scope for this spec; note it so the "parity across three scopes" claim is honest
  (the *marker* mechanism is shared; shard remediation is not). Filed as follow-up.

## Injection points (verified in-tree)

- `internal/reviewers/epicready.go` — add `RequireLenses`/`Adversarial` to `EpicReadyContext`; append
  `adversarialReviewFindings(context.RequireLenses, context.Adversarial)` on the non-context-risk path in
  `RunEpicReady`. (No change to `internal/reviewers/adversarial.go`.)
- `internal/epicready/review.go` — in `Create`: import `internal/reviewstate`; set `reviewerCtx.RequireLenses`;
  resolve `LatestReviewEvidence(root, "epic-ready", git.BaseSHA, git.HeadSHA)` into `reviewerCtx.Adversarial`
  (mirror `prready/review.go:198-212`); set `Adversarial.WorkingTreeUnattested` from the dirty-tree test above;
  set `Adversarial.HeadSHA = git.HeadSHA`.
- `cmd/metareview/main.go` — extend the `record-lenses --scope` allow-list from `{pr-ready, task-done}` to
  include `epic-ready` (`main.go:368`), and update the **three** enumerations that will otherwise go stale:
  the error string `main.go:369`, the usage line `main.go:59`, and the doc comment on
  `ReviewEvidence.ReviewedScope` (`internal/reviewstate/reviewevidence.go:37`, currently `// the gate this
  satisfies: "pr-ready" | "task-done"`).
- `internal/fsm/kind/kind.go` (choice 5) — add an optional `rubric` string param to the `review-lenses` node:
  `validateLenses` accepts `rubric` alongside `lenses`; the `Instructions` path reads `n.Params["rubric"]` when
  set, else the existing `Rubric` const. **Default behavior byte-for-byte unchanged.** This package is in the
  require-100 set (`go list ./internal/fsm/...`), so the new branch needs a test that reaches 100%.
- `workflows/epic-review-loop.yaml` (choice 5) — new `review-loop` variant whose `discover` node sets
  `rubric: rubrics/epic-ready-review-rubric.md`. (`workflows` is in the require-100 set; a YAML file adds no
  statements, but confirm the embed/loader test lists it.)
- Review `.md` render — the `adversarial-review-reviewer` finding lands in the Blocking/Advisory findings
  sections automatically (`classifiedFindingsMarkdown` emits all records regardless of `reviewerNames`;
  pr-ready does NOT add it to `reviewerNames`, `prready/review.go:92`), so no render change is required for
  parity.
- Docs: ARCHITECTURE §2 (epic-ready "Engine today" cell: fully-deterministic → deterministic heuristics +
  required adjudicated review) and §3/§4 (epic-ready joins the require-lenses gate; the fast-follow is done);
  CLAUDE.md / AGENTS.md Completion Rule (extend "pr-ready and task-done require an adjudicated review" to
  epic-ready, add the `--scope epic-ready` recorder example **with the base-consistency rule**);
  `rubrics/epic-ready-review-rubric.md` rewrite (choice 3).

## Escape hatch & migration (data-migration lens: clean)

Identical to build B. `METAREVIEW_ALLOW_MECHANICAL_PASS=1` restores the legacy deterministic epic-ready pass
for one migration release (`reviewstate.RequireAdjudicatedReview()` — shared, no new flag). No `schemaVersion`
bump: `reviewedScope` is an existing field on `ReviewEvidence`; `"epic-ready"` is a new *value*, not a new
field. Old records don't break: `DiscoverReviewEvidence` filters on `Kind == ReviewEvidenceKind`, so legacy
epic-ready *run* records (which carry `scope:"epic-ready"` but no `kind`) are skipped, never mis-read as
markers. Every existing epic-ready PASS now reads as "needs an adjudicated review" until one is recorded —
expected and correct, the migration build B already imposed on pr-ready/task-done.

## Test plan (TDD)

Mirror `tests/go/test-require-adjudicated-review.sh`, adding an epic-ready set (a new
`test-epic-ready-adjudicated-review.sh` or an epic block in the existing script). Reuse its `mkfsmrun*`
helpers.

**Reviewer unit** (`internal/reviewers/epicready_test.go`): `RunEpicReady` with `RequireLenses=true` and no
`Adversarial.Present` emits the `adversarial-review-reviewer` block; a matching passing marker → no block;
a non-pass verdict → blocks on the review's own findings; an emulated-but-passing marker → the advisory note;
`WorkingTreeUnattested=true` with a passing marker → the `working-tree-unattested` block;
`RequireLenses=false` → nothing (opt-out). **Coexistence:** an adversarial block AND a `missingChildEvidence`
block coexist (the pre-check is independent). **context-risk interaction:** a context-risk context pre-empts
and does not also emit the adversarial block (choice 2 / C2).

**Wiring unit** (`internal/epicready`): `Create` resolves `LatestReviewEvidence` for `"epic-ready"` over
`git.BaseSHA..git.HeadSHA`; a marker for that exact span satisfies; a stale-head or different-base marker does
not; a dirty tree sets `WorkingTreeUnattested`.

**CLI unit** (`cmd/metareview`): `record-lenses --scope epic-ready` accepted and round-trips a marker with
`reviewedScope:"epic-ready"`; `--scope bogus` rejected. **Reject suite (mirror build-B lines 120-162 with
`--scope epic-ready`)** — the two bugs build B's dogfood caught must be pinned for epic-ready, not assumed:
at minimum `reject-no-fromrun` (forged independence: `--mode subagent-adjudicated` without `--from-run`) and
`reject-wrong-diff` (`--from-run` over a run whose init reviewed a different base..head).

**Behavioral end-to-end** (shell): fresh repo with an epic + children → `review epic-ready` blocks
(adversarial-review-reviewer, NOT_REVIEWED-style) → `record-lenses --scope epic-ready` (in-session-emulated) →
PASSes with the weaker-evidence advisory note → new commit → blocks again (currency). A
`subagent-adjudicated --from-run` scenario over a forged matching FSM audit passes without the advisory note.
**Base-advance scenario (base-asymmetry HIGH):** main has ≥1 commit past the branch point; drive the gate and
the recorder with the **identical explicit `--base`** and assert PASS (proves the base-consistency rule holds
the happy path); optionally assert that mismatched bases (default gate vs `--base main` recorder) do NOT match,
documenting the trap.

**Lens-config seam (choice 5)** — unit-test the `review-lenses` node rubric param: with no `rubric` param the
`Instructions` text names the task-done rubric (default unchanged); with `rubric: rubrics/epic-ready-review-rubric.md`
it names the epic rubric; `validateLenses` accepts a string `rubric`, rejects a non-string, and still rejects an
unknown param and an out-of-range `lenses` count. Parse-test `workflows/epic-review-loop.yaml` (loads, its
`discover` node carries the epic rubric). `internal/fsm/kind` must stay at 100% — cover both the param-present
and param-absent branches. (This is the concrete proof the epic lenses are not in lockstep: the same node emits
a different rubric for epic vs task-done.)

**Feature-flag off** restores the old deterministic epic-ready pass.

**Coverage (finding: floors, not 100%):** new code lands in `internal/reviewers` (floor 97.7),
`internal/epicready` (floor 81.6), and `cmd/metareview` (floor 81.2) — none are fsm packages, so the
per-package floor in `tests/coverage-floor.txt` applies (not the require-100 set). Each touched package stays
≥ its floor; regenerate with `bash tests/coverage.sh --update-floor` if coverage rises. **Mutation-cover** the
new reviewer branch and `Create` wiring (block/allow/stale/emulated/working-tree/opt-out) with `gremlins
unleash --workers 1 --timeout-coefficient 30 ./internal/reviewers/ ./internal/epicready/`; construct a
distinguishing test for any survivor.

## Non-goals

- Deleting the existing epic-ready heuristics (they stay as pre-checks *and* are the roll-up-freshness guard).
- Turning epic-ready into a fix-loop (it stays a one-shot review; ARCHITECTURE §4).
- Embedding an FSM invocation inside `epicready.Create` (the agent drives the loop and bridges via
  `--from-run`, keeping `Create` model-free and hermetically testable).
- Making the adjudicated marker attest the *roll-up* (child logs/evidence/intent) — that freshness is guarded
  by the deterministic pre-checks, which re-read current state every run; the marker attests the integration
  diff only. (This is the resolution of review findings A1/F1, not an unexamined omission.)
- Shard remediation for an oversized epic integration diff (pre-existing gap; filed as follow-up).
- Changes to `internal/reviewstate` (already scope-agnostic). **Revised after the diff review:** `internal/
  reviewers/adversarial.go` gains a small per-scope `WorkflowHint` field so a blocked epic-ready is steered to
  `epic-review-loop` (its rubric) rather than the default `review-loop` (task-done rubric) — the diff review
  caught that the scope-generic remediation strings, left unchanged, would tell epic-ready to run the
  task-done-rubric workflow, which `validateFromRunDiff` would then credit for epic-ready, quietly defeating
  choice 5. The gate LOGIC in adversarial.go stays scope-agnostic; only the guidance text is parameterized.
- Named epic-specific lens-persona dispatch from the FSM node (choice 5 builds the rubric-divergence seam and
  the per-scope recorded lens set; fanning out *named* epic personas rather than a count is left reachable via
  the same param surface but is not built now).

## Review history

The first draft of this spec was reviewed by nine adversarial lenses (three subagents: design; precision+intent;
testing+security+migration). They confirmed the mechanical wiring was accurate but found **three HIGH** issues
that this revision resolves: (A1/F1) the marker's base..head currency cannot attest or track the roll-up the
draft made the lens surface — resolved by attesting the integration diff and guarding roll-up freshness with
the pre-checks; (C1) the draft's "epic-ready reviews only the committed span" was false — resolved with a
`working-tree-unattested` path; (base-asymmetry) the merge-base-vs-tip default base would silently wedge a
correctly-recorded marker — resolved with the base-consistency rule and a base-advance test. Plus mediums/lows:
context-risk placement, the large-epic no-shard limitation, coverage floors, the reject-suite test gaps, the
rubric-rewrite deliverable, and three stale scope enumerations — all folded above.

Maintainer sign-off (2026-09-03) approved the spec **with one caveat**: the epic review's lenses must be able to
diverge from and add to the pr-ready/task-done lenses, not move in lockstep. Investigating it revealed the FSM
`review-lenses` node hard-codes the task-done rubric, which *would* have forced lockstep — resolved in choice 5
by making the node's rubric a per-workflow param (default unchanged) plus an `epic-review-loop.yaml` that
applies the epic rubric, with the epic lens set recorded independently in the marker.
