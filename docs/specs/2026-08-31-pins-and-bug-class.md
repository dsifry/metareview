# Spec: Pins & Bug.Class — proof of fix, and defect-class enumeration

Status: **Literature-grounded refinements approved (§9, evidence in docs/research/). Re-converging.**
Prior: adversarially converged (7 passes). PINS: shippable (Ship 1). BUG.CLASS: validated path (Ship 2).

> **Adversarial convergence (2026-08-31):** a 7-pass convergent adversarial-review loop (6 lenses →
> refute-verify → synthesize per pass) ran over this spec until two consecutive passes found zero
> critical items. It resolved 7 verified criticals that six manual review rounds had missed —
> including two sections specifying the rollout oppositely, a dedup key that contradicted its own
> spike, the deep root where `pins_proven` and `classes_enumerated` checked disjoint code (now bound
> to the finding's own file), a second text-only collapse site in the real code (`BugID`/`dedupBugs`),
> and a finding-identity spike that measured precision when the blocker was recall. None were
> regressions of a prior fix. Passes 6 and 7 were dry.
>
> **Acceptance (PR#30):** the two committed artifact reviews are `NEEDS_REVISION` by design — they are
> the honest record of what the reviews caught, and every blocking finding is either addressed inline
> (marked `review fix …` / `PR#30…`) or recorded as a known-open ⧗ item or an accepted tighten-later
> decision. This spec is accepted by the maintainer as the agreed *design path*, not as a completed,
> zero-open-item artifact. The code that implements it is separately task-done-gated per chunk.
Date: 2026-08-31
Supersedes the pins/`require_classes` work stranded on `prep-0.11.0` (never merged; see PR #23,
closed).

---

## 1. What problem this solves, and the evidence it is real

Two failures the FSM loop has been *compensating* for rather than *designing out*. Both are the
same shape — **the work unit is an instance when it should be a class** — seen at two levels.

**At the fix node.** Handed a list of findings, an agent fixes the findings. Measured over two
iterations of this repository's own loop (recorded verbatim in the `FixClass` doc comment on
`prep-0.11.0`):

| Told | Did | Left |
|---|---|---|
| the review-log header could be forged | bounded the 2 fields named | 6 more |
| 3 of 4 writers lacked a test | added 1 | 3 |
| a list is duplicated, define it once | exported the list | 3 copies untouched |
| empty-vs-unknown claimed but unimplemented | added the flag to 1 type | the 2nd type |

Four findings, one behaviour each, every one a faithful reading of "fix the instance."

**At the human/agent level.** This same session, across PRs #24–#28, the pattern recurred in *my*
edits: a fix reopening, one step earlier in the same function, the exact hole it had just closed;
the seam-untested defect class recurring 5+ times. The user's own summary: *"Four of those five
root causes are the same failure in how I fix: I fix the instance I was shown and not the class."* *(Scope note, review W3: the mechanisms in this spec live in the FSM nodes; they help
the human/agent case only insofar as that work is routed through the FSM gates. Direct human editing
outside the loop is out of scope here — the framing is motivational, and the shared root is why the
FSM fix is worth making, not a claim that both levels are delivered.)*

**The `fix` node also takes the agent's word that a fix happened.** `{commit, summary}` is not
evidence. The loop then re-discovers the same bug next round because nothing checked the claim.

### Measured this session (2026-08-31), grounding every design choice below

Over the 10 real run audit logs in `.metareview/runs/`:

- **Pins already work.** Run `mrv-20260830-193926…-proved` closed a full
  `discover→adjudicate→fix→prove→verify→discover` loop. The pins design is sound; its only failure
  was a **seam** (`agentEdit.Reduce` dropped `Pins`, so the `pins_proven` gate passed vacuously on
  every run), not a data-model flaw.
- **The class signal is lens convergence on one defect, not file co-location.** 611 findings across
  77 files; 69/77 files (90%) carry >1 finding — so "same file" is far too common to discriminate.
  But `internal/status/fsmruns.go` had **24 findings that collapse to 8 distinct**, and 3 of those 8
  all hit L118, the self-clearing-gate defect, through different lenses. Convergence on a
  `(file, region, defect)` is the signal; co-location is not.
- **~~The 100-finding cap can hide the second instance.~~ CORRECTED (review B6):** there is **no
  truncating 100-cap** in the code. The only hard limit is `MaxDeltaList=256`, which *rejects* an
  over-large delta loudly, never silently truncates; `findings=100` was the real lens volume for
  those rounds. And adjudicate **already** exact-dedups by issue text (`dedupCandidates`,
  `kind.go:621`). So the "top and bottom straddle the cap" risk was a misreading — a class's second
  instance is lost only if the *lenses never find it*, which is a discovery-quality problem, not a
  truncation one. This removes the §3.3 dedup work and the §4.3 cap change from scope.
- **The 4 `PinOutcome` values are earned.** `internal/mutation/verify.go` produces each from a
  distinct cause with a distinct remedy (below). Not over-modeling.

---

## 2. Data model (the core of this spec)

Design rule for this section: **the simplest model that makes each required claim checkable by the
machine.** Every field below is justified against that bar; simplifications considered and rejected
are marked ✗, accepted ones ✓.

### 2.1 Pin — a checkable claim that one test holds one line

> **→ Refined by §9 (approved 2026-08-31): a `Pin` is one form of a differential-test artifact; a *reproduction test* is preferred, this mutate-a-line pin is the fallback (§9.1). A deletion is proven the same way and carries a `DeletionRef` (§9.4).** Read §9 as authoritative where it conflicts with the text below.

```go
type Pin struct {
    ID      string `json:"id"`      // idempotent content hash of {Finding,File,From,To} — the reference/override key
    Finding string `json:"finding"` // the confirmed-finding id this pin proves a fix for (the CLEAR key)
    File    string `json:"file"`    // production file the fix touched, repo-relative
    From    string `json:"from"`    // exact text to replace; must appear in File exactly once AND in the commit diff
    To      string `json:"to"`      // a COMPILING change that breaks the behaviour the fix introduced
    Test    string `json:"test"`    // the test the fix claims pins this line — selects, never executes
}
```

**Why this shape.** The agent declares *what to break*, never *what to run*: the test command comes
from the workflow's consent-hashed `cmds` block, not from the pin. A pin that named its own verifier
would be an agent grading its own homework. `From`/`To` carry no line numbers — text that must
appear exactly once is unambiguous and does not drift.

- ✗ *Considered:* line numbers instead of anchor text. Rejected — they drift across edits and make
  the audit log lie after any change above them.
- ✓ *Kept:* `Test` is advisory (for the human re-running by hand and for the report); the machine
  proves the line via mutation regardless of the named test.
- ✓ *Added `ID`* (idempotent, content-derived): a deterministic hash of `{Finding,File,From,To}`, so
  the same pin always yields the same id — dedupable, referenceable, and the key an override addresses
  a specific `unverifiable` pin by (§4.1/B7). Distinct from `Finding`: the **id is what the pin IS**
  (reference/reliance), the **finding is what it clears against** (survives a reworded fix, which
  changes the id but not the finding). No randomness/timestamp in the derivation — it is a pure hash,
  so it is stable across machines and replays.
- ✓ *Added `Finding`* (review fix B1/B5): the id of the confirmed finding this pin answers. It is
  the **clear key** for `Snapshot.Unproven` — a pin clears its gap by *finding id*, which survives a
  redone fix rewording `From`/`To`, where an anchor-text key would go stale forever (the B5
  dead-end). It is also the chain link tying the proof to *what was supposed to be fixed*.
- ✓ *`From` must be a line the commit ADDED* — it must appear as a `+` line in the diff, not merely
  be present in the file (review fixes B1 + round-2 R1). "Present in the diff text" was not enough:
  unified-diff **context lines** are unchanged pre-existing code, so requiring only diff-presence let
  the agent anchor to a well-tested pre-existing line next to the real change. Requiring an *added*
  line binds the mutation to code the fix actually introduced. Enforced in `prove` (§3.1).
- **What a pin HONESTLY proves (round-2 R1) — read this before trusting the gate.** A proven pin
  establishes *"this added line is exercised by a test that fails when the line is broken"* —
  change-coverage-under-mutation. It does **not** prove the fix is *correct* (a test can assert wrong
  behaviour) nor that the fix is *complete*. It is strictly stronger than "the agent says it is
  fixed" and strictly weaker than "the fix is proven." The gate and every doc must claim that weaker
  true thing, never "proof." Residual, narrow, tighten-if-abused: an added line can still carry
  pre-existing behaviour (a reformat, or new + old on one line), so the mutation could target the old
  part — mitigated by the added-line rule, not eliminated.

### 2.2 PinResult / PinOutcome — what checking a pin found

```go
type PinResult struct {
    Pin     Pin        `json:"pin"`               // embedded; its Pin.ID is the reference/override key (round-2 R8: no separate PinID field)
    Proven  bool       `json:"proven"`            // true iff break-fails AND restore-passes
    Outcome PinOutcome `json:"outcome,omitempty"`
    Detail  string     `json:"detail,omitempty"`
}

type PinOutcome string
const (
    PinProven       PinOutcome = "proven"       // break failed the tests, restore passed them
    PinSurvived     PinOutcome = "survived"     // mutation compiled, tests still passed → test gap
    PinMalformed    PinOutcome = "malformed"    // anchor absent/ambiguous, won't compile, OR compiles-but-semantically-null (comment/whitespace/dead-code) → bad pin
    PinUnverifiable PinOutcome = "unverifiable" // tree/baseline could not answer → environment
)
```

**Why four values, not two.** For the *gate*, only proven-vs-not matters. The other three exist for
the *actor*, because each demands opposite work, and `verify.go` already distinguishes them at
distinct sites:

| Outcome | Cause (from verify.go) | Owner | Remedy | Blocks? |
|---|---|---|---|---|
| proven | break→fail, restore→pass | — | — | no (clears) |
| survived | compiled, tests passed | implementer | write/strengthen a test | **yes** |
| malformed | anchor ≠ 1 match, won't compile, or compiles-but-null (comment/whitespace/dead-code, caught by the §9.8 AST pre-screen) | the fix agent | rewrite the pin | no (report) |
| unverifiable | copy failed, baseline red, exec killed | environment | fix the tree | **yes — override-clearable** (§4.1) |

- ✓ *Kept all four:* collapsing survived+malformed would report a real test gap and a typo the same
  way, sending the actor to the wrong work.
- *`malformed` is "report" for triage, not "accept" (review #34):* the `Blocks?` column says it is
  not a *test-gap* block like `survived` — but a fix that **supplied** a malformed pin still does not
  satisfy `pins_proven`, because §3.1 clause (b) requires every supplied pin be `Proven`. The pin must
  be rewritten to `Proven` (or, for a trivial pin the §9.8 AST pre-screen classifies malformed,
  replaced with a real one) before the fix passes.
- `unverifiable` **blocks** but is environment-owned and clearable only by a recorded override
  (§4.1): it must not be a silent way to escape proof, and must not livelock a genuinely broken tree.

### 2.3 FixClass / FixInstance — the enumeration the reviewer cannot do for you

```go
type FixClass struct {
    BugClassID string       `json:"bug_class_id"`       // durable BugClass.ID this answers (PR#30-CR: fix-side carrier for the class id)
    Name      string        `json:"name"`               // the shared defect, stated once
    Findings  []string      `json:"findings,omitempty"` // reported ids that belong to this class
    Instances []FixInstance `json:"instances"`          // every DECLARED-OR-KNOWN member answered (PR#30: NOT "every site reported or not" — unreported siblings are detected post-merge, not gated; §3.2)
}

type FixInstance struct {
    Member      string `json:"member,omitempty"`  // the BugClass member (finding id) this answers, when answering a declared class
    File        string `json:"file"`
    Disposition string `json:"disposition"`       // fixed | fixed-elsewhere | already-correct | out-of-scope
    Reason      string `json:"reason,omitempty"`  // required for anything but "fixed"
}
```

**Why this shape.** Enumerating is the step being skipped, so the schema *demands the enumeration in
writing* — and writing it down is most of the cognitive work being skipped. Once written, it is
checkable: a declared `fixed` instance whose file the commit never touched is an oversight or a false
claim, and both are worth surfacing.

- ✓ *Kept `Instances` as the load-bearing field.* It is exactly what the reviewer could not
  enumerate for you; `Findings` is the cheap back-link to what triggered the class.
- ✓ *Kept 3 dispositions.* "already-correct" (I checked, it was fine) vs "out-of-scope" (I chose not
  to) vs a silent omission are three different facts, and post-merge learning needs to tell them
  apart. `Reason` is mandatory for the two non-`fixed` values so declining is a recorded decision.
- ✓ *Added `Member`* (review fix B3): the back-reference from an instance to the class member it
  answers. Without it a 3-member class could be "answered" by one `FixInstance` with two members
  silently dropped and no gate able to tell — the `classes_enumerated` gate now checks **every**
  `BugClass.Member` has a matching `FixInstance` (§3.2 link 3). The non-`fixed` dispositions are the
  reachable exit (§4.6): a member that needs no code change is answered `already-correct`/`out-of-scope`+reason, never forced.
- ✗ *Considered:* collapsing to `fixed` / `not-fixed`. Rejected — it erases the checked-vs-ignored
  distinction, which is the whole point of asking.
- ✗ *Considered:* a unified `Claim` type spanning Pin and FixInstance ("agent declares, machine
  checks"). Rejected as over-unification: they verify against *different artifacts* (the test suite
  vs the diff) and will evolve independently. They share a *pattern*, not a *type*.

### 2.4 Carried state (Snapshot / Delta additions), and what actually touches the wire

```go
// classify → fix : the groupings the fix node must answer
Delta.Classes     []BugClass   // Snapshot.Classes accumulates the open ones
// fix → classes_enumerated gate : the fix's account of each class
Delta.FixClasses  []FixClass
// fix → prove → gate : the pin claims and what proving them found
Delta.Pins        []Pin
Delta.PinResults  []PinResult
// derived, never persisted (recomputed by Fold): the open gaps that drive re-discover
Snapshot.Unproven []Pin        // a pin CLEARS when a later PinResult with the same Pin.Finding is Proven
Snapshot.Classes  []BugClass   // open classes; carries the MINTED id across rounds. A class clears only
                               // when every member is RESOLVED: pins_proven at the member's own file
                               // (fixed) or the remedy file (fixed-elsewhere), or accounted
                               // already-correct/out-of-scope — never on a claimed Disposition alone (§3.2)
```

**`Unproven` lifecycle — add, clear, and re-add, in temporal fold order (review fix B5 + round-2 R4).**
The fold processes events in order and maintains `Unproven` as it goes:
- **Add:** when the `fix` node emits a `Delta.Pin`, its `Finding` gap enters `Unproven`.
- **Clear:** a *later* `PinResult` (from the `prove` node) with `Proven=true` for that `Finding`
  removes the gap. Clearing is by `Finding`, so it survives a redone fix that rewrote `From`/`To`.
- **Re-add:** if that same `Finding` is later re-discovered (the fix regressed), it enters `Unproven`
  again. This is why clearing must be **temporal** (a later Proven clears the gaps open *at that
  point*), NOT a set-recompute over all history — a set-recompute would clear a fresh regression gap
  against a stale historical `Proven`, passing `pins_proven` vacuously on a regressed fix, which is
  the #24 self-clearing shape. The rule is: *a Proven PinResult clears only gaps currently open.*

**Class carriers (review fix B4).** `Delta.Classes`/`Snapshot.Classes` carry `classify`'s output to
`fix` and across replay; `Delta.FixClasses` carries the fix's account to `classes_enumerated`.

**Override-key stability (round-2 data-migration warning).** `Pin.ID` is a content hash of a *single
fix's* pins — stable for that fix, and a redo legitimately mints a new one (clearing keys on `Finding`,
not `Pin.ID`, so that is fine). `BugClass.ID` is **minted once and carried** (§3.2), so it is durable
by construction. The residual hazard both share: if the hash *derivation itself* ever changes,
overrides keyed on old ids silently stop matching. Freeze the derivations, or version them and migrate
override keys on change — the same discipline §2.4 applies to `SchemaVersion`.

Plus finding provenance so gates select on structure, never prose (the #24 lesson — `pins_proven`
once matched `strings.HasPrefix(IssueText, …)`):

```go
Finding.Source   // e.g. "mutation-verify"
Finding.Category // "unproven-fix" (blocks) | "malformed-pin" (reports) | "unverifiable"
```

**Schema compatibility (review fix W1 — keep the 10 existing runs readable).** `SchemaVersion`
stays **1**; every field above is added as an **additive optional** field. Forward reads are then
safe: the strict decoder (`fold.go:478` `DisallowUnknownFields`) rejects unknown *keys*, never
*missing* ones, so the 10 persisted runs decode unchanged against the widened structs. Two caveats,
both stated so the implementer cannot trip them: (a) do **not** bump `SchemaVersion` — `fold.go:23`
hard-rejects a mismatched version and would make every existing run unreadable; (b) only
`Delta.Pins`/`PinResults`/`Classes`/`FixClasses` enter the persisted wire — `Finding.Source`/
`Category` already exist on the shipped struct (no-op), and `Snapshot.*` is recomputed by `Fold` and
never persisted. The one-way door: once a proved run writes these fields, an *older/rolled-back*
binary can no longer `Fold` that run. Gate the first pins-writing deploy accordingly; there is no
downgrade path.

---

## 3. Machinery

### 3.0 Where each operation sits in the loop

Two operations, and they are **not the same operation and not in the same place**, because one is
deterministic and one is judgement:

- **Dedup** *removes* redundant findings — the 24 lens-echoes of "L118 self-clearing gate" are one
  finding seen many times, and collapse to **one**.
- **Classify** *groups* distinct findings — "L118 self-clearing gate" and a distant "L26 reason
  not required" that share a mechanism become **one class with two members**, still two findings.

Conflating them is the trap: dedup must never merge two real instances (that would destroy the class
before it can be seen), and classify must never be handed raw echoes (it would group noise).

```
 discover ─[findings_nonempty]→ adjudicate ─[confirmed_nonempty]→ classify ─[classes_sound]→ fix ─[commit_exists ∧ classes_enumerated]→ prove ─[pins_proven]→ verify ─→ …
   (review-lenses,                (judge fan-out,                  (ONE set-level               (agent-edit,                             (mutation-verify,
    subagent fan-out)              phase-1 confirm)                 LLM call: group)             emits pins + fix instances)              deterministic)
        │
        └── DEDUP exists in `adjudicate` (`dedupCandidates`), keyed on exact issue-text TODAY but
            CORRECTED to `(file, normalized-text)` per §3.3 (and `BugID`/`dedupBugs` likewise). No new
            dedup *layer* — a key correction. `classify` consumes the deduped confirmed set.
```

| Operation | Placement | Why there | Exec | Kind |
|---|---|---|---|---|
| **Dedup** (collapse echoes) | **in `adjudicate`** (`dedupCandidates` + `BugID`/`dedupBugs`), key CORRECTED to `(file, normalized-text)` per §3.3 | file-in-key so a cross-file class member is not collapsed; the spike validated the 24→8 collapse with this key | adjudicate | **deterministic** |
| **Classify** (group into classes) | a **new node between `adjudicate` and `fix`** | it must read **all confirmed findings together**, and the adjudicate fan-out's completion barrier is exactly where the full confirmed set first exists in one place | node | **judgement (LLM)** |

**Why classify is its own node and not a phase of `adjudicate`.** A node has one exec kind. `adjudicate`
is a *fan-out* (N parallel per-finding judges); classify is *one* set-level call. They cannot share a
node under the exec model, and the barrier where the fan-out completes is the natural node boundary —
which is also the first moment the whole confirmed set exists at once. So the boundary is not
overhead; it is the barrier classify needs.

**Why classify is a node and not a gate.** Gates are deterministic predicates. "These share a
mechanism" is a judgement, so it cannot be a gate. What *is* a gate is everything downstream that
makes the judgement checkable: `classes_sound` (every member is a confirmed finding, no singletons
dressed as classes) after classify, and `classes_enumerated` (each `fixed` instance is in the diff)
after fix.

**Rollout of the placement** (matches §6): `classify` is present from the FIRST shipped version of
Bug.Class — there is **no** interim where the fix agent groups its own classes (§3.2 invariant 4).
1. **Step 4:** *add the `classify` node* (stateful over open `Snapshot.Classes`). The grouping
   judgement lives **upstream**, in a node that sees the entire confirmed set at the barrier —
   including instances the fix agent would never have been shown. The one who fixes has the instance
   blind spot, so the class call is made before the work reaches them, not by them.
2. **Step 5:** `require_classes` + `classes_enumerated` with the own-location member check; a class
   clears only on validated member resolution.

### 3.1 Pins / prove (the deterministic sandwich)

> **→ Refined by §9 (approved 2026-08-31): `prove` is ONE differential-test gate — reproduction test preferred, pin fallback (§9.1) — plus a mandatory *fails-for-the-right-reason* check (§9.2, a veto-only reviewer, where the own-file binding migrates) and self-validating-loop anti-gaming (§9.3).** Read §9 as authoritative where it conflicts with the text below.

```
… → fix ──[commit_exists]──> prove ──[pins_proven]──> verify → …
         (agent-edit)      (mutation-verify)        (review-lenses)
```

- **`fix`** (agent-edit): emits `{commit, summary, pins[]}`. Schema *requires* ≥1 pin when the fix
  claims to have changed behaviour; a fix that shows no checkable work is refused.
- **`prove`** (mutation-verify): the only deterministic non-gate node. For each pin: apply
  `From→To`, run the consent-hashed test cmd — it must fail; restore, it must pass. Emits
  `PinResult` per pin.
- **`prove` binds the pin to an ADDED line (review fix B1 + round-2 R1).** Before mutating, `prove`
  rejects any pin whose `From` is not a line the commit **added** (a `+` line in the diff) — outcome
  `malformed`, owner the fix agent. Diff-*presence* alone was defeated by context lines; requiring an
  *added* line ties the mutation to code the fix introduced.
- **`pins_proven`** gate: passes iff **both** —
  (a) every confirmed finding that OWES a pin has a `Proven` pin whose `Finding` matches it **and
  whose `File` equals that finding's own location** (`Finding.File`, i.e. the member's
  `ClassMember.File`); AND (b) every pin the fix SUPPLIED is `Proven`.
  The **own-file clause in (a) is load-bearing** (adversarial pass 2): without it `pins_proven` and
  `classes_enumerated` check *disjoint* code — a sham added edit in the finding's file satisfies the
  class gate's own-location touch (§3.2 inv. 3), while one real tested line in a *different* file,
  pinned under this finding's agent-written `Finding` label, satisfies the proof. Both gates then go
  green on an unproven finding — the #24 self-clearing shape, amplified when one tested line is
  relabeled across many findings. Binding the pin's `File` to the finding's own file makes the
  proven line and the class-gate's touched line the SAME code. (b) independently blocks a supplied
  `survived`/`malformed` pin.
  A finding **owes a pin** iff (machine-determinable): its fix touches a source file in a package
  that **has test files** (§4.2) **and** added a line to pin **in the finding's OWN file
  (`Finding.File`)** (PR#31-CR: the trigger is scoped to `Finding.File`, matching (a)'s pin-file
  requirement — so a cross-file remedy never creates an owed pin no valid pin can satisfy). A
  **pure-deletion** fix,
  a **no-test-package** fix, or a fix whose remedy genuinely lies in a **different file than the
  finding** (no pinnable added line in the finding's own file) owes no pin and records **"no pinnable
  change"** naming the actual fix file (§4.6) — auditable, never a silent pass; a supplied pin is
  still held to `Proven` by (b). This own-file binding is the **tightest achievable**: that a proven
  line is *causally* the fix is unprovable, so the gate binds to **co-location in the finding's own
  file**, consistent with invariant 3. Selects on `Finding.Source`/`Category`, **never on issue
  text**.

**The seam fix, and the test that would have caught it.** `agentEdit.Reduce` must carry `Pins` into
the Delta. The regression test that makes `pins_proven` non-vacuous: a fix emitting a pin that does
*not* hold its line must drive the gate to **fail**. A gate that has never been observed failing is
not a gate (this is the vacuous-pass lesson from #24, and the blind-test lesson from the whole
session — the test must be shown red before it is trusted).

### 3.2 Bug.Class — the class-vs-instance *adjudication*

> **Round-2 redesign (authoritative).** The prose further down in this section describes the
> mechanism and the gaming analysis; where it conflicts with this subsection, this wins. The review
> found the class layer repeated the instance-vs-class error one level up (content-addressed id,
> clears-on-agent-word, no adjudicator/fixer separation in rollout). This is the corrected
> end-to-end. It is DESCRIBED and its edge cases enumerated; the items marked ⧗ need a spike before
> build — Bug.Class is not "done," it is a validated path.

#### The four invariants the redesign holds

1. **Durable identity, minted not derived (fixes C1/R2).** A class `ID` is minted **once**, when the
   class is first recognised, and carried in `Snapshot.Classes` unchanged for the class's life. It is
   **never** re-derived from the member set. Each round, `classify` is given the *open* classes
   (id + mechanism + members) and must either **continue** one (reuse its id, possibly changing
   membership) or **mint** a new one. So a class keeps its identity across regroups: an override or
   dissolution keyed on the id survives, and `Snapshot.Classes` does not grow unbounded — a continued
   class updates in place, a resolved class clears. The mint happens in the `classify` node (host
   side), never in the deterministic fold, which cannot generate ids.

2. **Clearing only on a VALIDATED signal — composition with pins (fixes C3/R2).** A class member is
   *resolved* only when its fix is `pins_proven` — for a `fixed` member, a Proven pin in the
   member's own `ClassMember.File`; for a **`fixed-elsewhere`** member, a Proven pin in the named
   remedy file `FixInstance.File` (PR#31-cursor: `fixed-elsewhere` MUST be a resolving disposition,
   validated the same way but at the remedy file — otherwise it clears `classes_enumerated` yet
   never resolves, so the class never clears and `require_classes` re-demands it forever, the
   dead-end this valve exists to open) — or the member is accounted `already-correct`/`out-of-scope`
   with a reason, or its remedy file has no tests and it records "no pinnable change" (the same
   auditable valve as pins). A class clears from `Snapshot.Classes` only when **every** member is
   resolved. The fold never clears a class on the
   agent's *claimed* `Disposition` alone — the disposition is a claim; the pin proof is the
   validation. This is exactly how `Unproven` clears (only a `prove` result mutates it), lifted to
   the class layer, and it is what stops the class snapshot from self-clearing on the agent's word.

3. **The gate binds each member to its OWN location (fixes C4).** `classes_enumerated`: for a member
   marked `fixed`, the commit diff must touch **that member's own `File`** (carried in `ClassMember`
   from its confirmed finding), not merely "some file in the diff." One edit in fileA can no longer
   "answer" members that live in fileB/fileC. Non-`fixed` members need a `Reason`. Every member must
   have a `FixInstance` (no silent drop).

4. **Adjudicator/fixer separation from day one (fixes C5/R5).** There is **no** interim rollout where
   the fix agent groups its own classes. `classify` (the adjudicator side, over the full confirmed
   set) exists from the first shipped version of Bug.Class; until it ships, Bug.Class is simply
   **inactive** (pins still run). The party that draws class boundaries is never the party that
   answers them.

#### What Bug.Class honestly does and does NOT do (fixes C6/R6 — remove the overclaim)

It **forces the fixer to answer every KNOWN member** (validated, per-location) and **detects missed
classes post-merge** via recurrence. It does **not** force discovery of siblings *no lens reported* —
you cannot gate on what nobody found. The earlier prose "EVERY site, reported or not" / "instance-only
fixing is no longer expressible" **overclaimed**; the honest claim is: *known* instances cannot be
silently dropped, and *unreported* siblings are surfaced by post-merge recurrence one round later,
not prevented at the gate. `classify`'s measured splitting-bias (§5) is consistent with this: a class
it splits too finely leaks back toward instances, caught post-merge, not at the gate — which is why
grouping is advisory (§4.5), not why it is safe.

#### Edge cases (enumerated — the "get the edge cases right" the maintainer asked for)

| Edge case | Handling |
|---|---|
| A class member is fixed, the fix regresses next round | the member's `Finding` re-enters `Unproven` (R4) and the class re-opens (its members are no longer all resolved) — the durable id means it is the *same* class, not a new one |
| `classify` regroups: a member moves between classes | ids are carried, membership updates in place; a member's old class re-evaluates its resolved-set |
| A class shrinks to one member (others resolved) | it is no longer a multi-member class; when the last member resolves it clears. A *newly* single-member grouping is just an instance (no class obligation) |
| Two open classes merge (classify decides they share a mechanism) | one id is continued, the other is marked merged-into (a tombstone carrying the surviving id) so overrides keyed on the retired id still resolve — ⧗ needs the tombstone rule specified |
| A member finding is rejected on re-adjudication | it leaves the class; if <2 remain the class dissolves (not a defect) |
| The fix touches the member's file for an unrelated reason | ⧗ file-granularity is the floor; line/site-granularity is the tightening if the file check proves too coarse in practice (tighten-if-abused) |

#### Prerequisite this exposes

**Cross-round-stable finding identity (⧗, also needed by metric-1, §8).** Class continuity, `Unproven`
clear/re-add, and the silent-regression metric all require a finding to keep the same id when
re-discovered in a later round. The current model does not define one (adjudicate re-confirms each
round). This is a **prerequisite for Bug.Class and for metric-1**, and needs its own small design +
spike before either is built.

#### Validation plan (what "validated" means before build)

- ⧗ **Durable-identity / continuity spike:** over a multi-round replay, can `classify`, given the open
  classes, reliably continue the right class rather than minting duplicates? Measure id churn.
- ⧗ **Finding-identity spike — PRECISION half done, RECALL half open (2026-08-31):** a
  `(file, top-6-normalized-words)` signature gives **0% within-run false-merges over all 611 real
  findings** — that is *precision* (distinct findings never collapse to one id). It is only the
  false-merge half. The property this prerequisite actually needs is *recall under paraphrase*: a
  re-discovered finding, reworded by the LLM next round, must keep its id or the `Unproven` gap never
  clears. That direction is **unmeasured** — 66% is cross-run recurrence, which cannot separate "did
  not recur" from "recurred but the signature minted a new id" (a paraphrase that shifts the top-6
  words is invisible in it). Before freeze (§6): build a paraphrase ground-truth set (the same
  finding reworded across rounds) and meet a **recall floor**. Use the simple key provisionally; it
  is not yet a proven cross-round identity.
- ✓ **Grouping viability:** done (§5) — classify recovers classes, errs toward splitting, no dangerous merges.
- ⧗ **End-to-end replay:** a class created → partially fixed → carried with stable id → cleared only
  when the last member is `pins_proven`. This is the acceptance test for the whole Bug.Class path.


This is the core question, and it has two layers: **who decides a set of findings is one class**
(adjudication), and **who is then forced to fix the class rather than an instance** (the fix
schema+gate). They must both exist; the second without the first only forces enumeration of
whatever the fix agent happens to notice.

#### Why adjudication cannot be deterministic clustering

The obvious cheap design is to cluster confirmed findings by the §3.3 dedup key
`(file, normalized-text)` (NO region bucketing — §4.4/§5). It is the right key for collapsing **lens
echoes** (the 24→8
on `fsmruns.go`: many lenses, one defect, one site). But it is the *wrong* tool for the case the
user named — **"two instances, one at the top and one at the bottom, are actually one class bug."**
Those two sit in different regions, often different files; co-location splits them precisely when
they most need joining. The thing that unites them is a *shared mechanism* (the same data-model
assumption violated in two places), which only a reading of both together can see. So:

> **Class-vs-instance is a judgement the adjudicator makes, not a distance the machine measures.**
> The machine's job is to make that judgement *checkable*, not to make it.

#### The adjudication mechanism (the `classify` node — see §3.0 for placement)

`adjudicate` today emits a per-finding `is_real` verdict, one finding at a time, in parallel. Class
detection cannot live there: to notice that finding *a* (top) and finding *b* (bottom) are one
class, the judge must see them **together**, which a per-finding fan-out never does. So the grouping
is a distinct node, `classify`, sitting at the fan-out's completion barrier (§3.0):

1. **`adjudicate` — confirm** (unchanged): per-finding `is_real` verdicts, parallel fan-out.
2. **`classify` — group**: one set-level call reads *all confirmed findings together* (adjudicate's
   already-deduped confirmed set, §3.3) and emits `Bug.Class` groupings — each a
   claim that *these member findings share one mechanism, and the fix belongs at that mechanism, not
   at each site.*

`classify` is a single call over the confirmed set, not a fan-out. (The pre-review draft called a
new dedup layer a prerequisite here; review B6 found adjudicate already dedups and there is no
truncating cap, so that prerequisite dissolved — see §3.3.)

`classify` runs **only when ≥2 findings are confirmed** (a class needs two members; a single-finding
round skips the call). Its grouping is **advisory** (§4.5): the fix agent works to it by default but
may split or dissolve a class **only by recording a reason**, via the same override lifecycle used
everywhere else. So an LLM over-group is correctable without forcing edits to unrelated code, and a
fix agent that dissolves a *real* class leaves an auditable record that post-merge recurrence (link 4
above) then checks.

```go
// Emitted by adjudicate phase 2. A claim about SHARED MECHANISM, made checkable downstream.
type BugClass struct {
    ID        string       `json:"id"`        // MINTED once at creation, carried forward unchanged — NOT derived from members (round-2 R2/C1)
    Name      string       `json:"name"`      // the shared mechanism, stated once
    Mechanism string       `json:"mechanism"` // WHY these are one bug: the assumption/structure at fault
    Members   []ClassMember `json:"members"`  // each member carries its OWN location, from its confirmed finding
}

type ClassMember struct {
    Finding string `json:"finding"` // the confirmed finding id
    File    string `json:"file"`    // where THAT finding lives — the gate checks the fix touches THIS, not just any diff file (round-2 C4)
    Line    int    `json:"line"`
}
```

A class of **one** member is just an instance — it carries no special obligation, so the adjudicator
gains nothing by inventing singleton "classes."

#### Making a semantic judgement checkable

No gate can verify "these three share a root cause" directly. But the judgement is made checkable by
the **chain** it forces, each link of which *is* mechanical:

1. Every `Member` must itself be a **confirmed real finding** (phase 1). A class cannot smuggle in a
   rejected finding.
2. The `fix` node receives **classes with members**, and `require_classes` obliges it to answer each
   member with a `FixInstance` — plus enumerate any *further* instances it found (the reviewer could
   not). Instance-only fixing is no longer expressible in the schema.
3. `classes_enumerated` gate (see §3.2 redesign invariant 3 — this supersedes any older wording):
   **every `BugClass` member must have a matching `FixInstance` (by `FixInstance.Member`)**, and for a
   `fixed` member the commit diff must touch **that member's own `ClassMember.File`** — NOT merely
   "some file in the diff" (PR#30: the agent controls `FixInstance.File`, so one edit in file A must
   not "answer" a member that lives in file B). **Cross-file remedy (PR#31-CR, symmetric with the
   `pins_proven` valve):** a member whose genuine remedy is in ANOTHER file uses disposition
   `fixed-elsewhere`, naming the remedy file in `FixInstance.File`; the gate then requires that named
   file in the diff. This is the auditable exception — gameable (the agent can point it anywhere) but
   RECORDED and flagged for post-merge, *tighten-if-abused* — so the tight own-file default holds for
   the ordinary case while a real elsewhere-fix has a valid path. A non-`fixed` member carries a
   `Reason`.
4. Post-merge learning flags classes whose members' fixes touched **disjoint structural sites** —
   evidence the grouping was wrong (an over-group) and a calibration signal for the adjudicator.

So no single link proves "same root cause," but a false class cannot pass the whole chain quietly:
an over-group forces the fix agent to either fix unrelated sites (visible in the diff) or mark them
`out-of-scope` with a reason (visible in the record); an under-group (calling a class a set of
one-offs) is what phase 2 exists to catch, and what post-merge learning measures the adjudicator on.

#### The two gaming directions, and the defence

| Failure | Who does it | Defence |
|---|---|---|
| **Over-group** (lump unrelated findings to look thorough) | adjudicator | each member must be independently confirmed; post-merge flags disjoint-site classes; `Mechanism` must name a shared cause, not a category |
| **Under-group** (keep everything as one-offs to avoid the class fix) | adjudicator / fix agent | phase 2's whole purpose; measured by post-merge recurrence — a defect re-discovered next round at a sibling site *was* a missed class |

The honest tell of a real class, from the data, is that the **same fix resolves every member** — but
that is a *heuristic about real classes*, NOT what the gate checks (PR#30: the gate checks each member
against its OWN location, per invariant 3; "one site clears many" as a gate criterion is exactly the
C4 hole the redesign closed). Post-merge recurrence checks the shared-cause claim over time.

### 3.3 Dedup and the cap — mostly already handled (corrected after review B6)

The pre-review draft proposed a new "dedup before the cap" layer at fold time to stop a 100-cap
truncating a class member. Verified against the code, that was wrong on three counts and is
**removed from scope**:

- The fold **replaces** findings wholesale (`fold.go:13-14`); there is no cross-lens accumulation to
  dedup at fold time.
- Adjudicate **already** dedups confirmed candidates. NOTE (PR#30-CR): the existing `dedupCandidates`
  keys on exact issue text ALONE, so two real findings with identical text in **different files**
  collapse — dropping a class member before `classify` sees it. The fix is to add the file to the
  key: `(file, normalized-text)`, **NO region bucketing** — the spike (§4.4/§5) reproduced the 24→8
  collapse with exactly this key, while line-region buckets split true duplicates and made it *worse*
  (24→11). Small correction to the existing dedup, not a new layer. Regression case: identical text
  in two **different files** must NOT collapse; identical text at two lines *within one file* is a
  true duplicate and DOES collapse (per §4.4/§5).
  - **Second collapse site (adversarial pass 4) — `dedupCandidates` is NOT the only text-only key.**
    `run.BugID(IssueText)` (`canonical.go:103`) hashes issue text ALONE, and it is used at **every**
    adjudication path in `kind.go` — `dedupCandidates` (the candidate key), the `allIDs` preflight
    (`kind.go:489`), `dedupBugs` on the confirmed/rejected sets (`kind.go:563`), the matched-golden
    branch (`kind.go:517`, `BugID(golden.Comment)`), and the second-opinion branches
    (`kind.go:594/602/605`). So a same-text/different-file finding can be dropped, undercounted, or
    collapsed at any of them before `classify`. The file-in-key discipline must extend to the Bug
    identity at **every one of these sites**: derive `Bug.ID` from `(File, normalized-text)` (and the
    golden path from its own file) so the different-file guarantee holds end-to-end, with a
    same-text/different-file regression over all paths. This changes the `BugID` derivation — exactly the frozen-derivation / override-key
    hazard in §2.4 — so it is done as part of the Ship-1 ⧗ cross-round-stable finding-identity task,
    with the keyed overrides migrated, never silently.
  `classify` consumes the deduped confirmed set.
- There is **no truncating 100-cap**. The only hard limit is `MaxDeltaList=256` (`kind.go:200`), a
  *reject*, not a truncate. A round producing >256 findings fails loudly (the whole delta is
  refused) — a real but visible backstop, not a silent class-hider.

**What remains, and it is small:** confirm that `classify` (§3.2) runs over adjudicate's
already-deduped confirmed set, and decide only whether `MaxDeltaList=256` is high enough — deferred
to the first real round that actually approaches it (none of the 10 logged runs did; the max was
100). No dedup code, no 100→300 change.

---

## 4. Decisions (settled 2026-08-31)

1. **`unverifiable` blocks — environment-owned, override-clearable, and now WIRED (review fix B7).**
   It blocks `pins_proven` (so a fix cannot escape proof by breaking the tree), owned by the
   **environment**, exit only via a **recorded override keyed on `Pin.ID`** — the id addition (§2.1)
   is what makes this exit physically pullable, which it was not before. A silent pass would be an
   escape hatch; a hard block with no key would livelock a broken tree (a dead-end). *Tighten-if-abused
   (deferred):* the grant is self-servable via `--by`; gate it behind an authenticated authority only
   if it is actually abused.
2. **`require_pins` fires on a determinable trigger, with a self-asserted valve (review fix B8).**
   The pre-review trigger — "a file under a package the test cmd *covers*" — is not statically
   computable for an opaque consent-hashed cmd. Replace it with something a machine *can* decide: the
   fix touches a source file in a package that **has test files at all** (`go list` / the language's
   equivalent — determinable, no coverage instrumentation). Otherwise the fix records **"no pinnable
   change"** with a reason. That "none" path is the reachable valve (§4.6) — self-asserted for now,
   *tighten-if-abused* by cross-checking the claim against the diff's file set later. The trigger is
   deliberately looser than "the line is test-observable": a pin on a genuinely unobservable line
   returns `survived`, which is a real (if sometimes annoying) signal to add a test, not a dead-end.
3. **~~Dedup before the cap, raise to 300.~~ VACATED by review B6.** There is no truncating cap to
   raise (the only limit is a `MaxDeltaList=256` hard-reject), and adjudicate already exact-dedups,
   so both halves of this decision addressed a problem that does not exist. Nothing to build here.
   The only live question — is `256` ever too low — is deferred to the first round that approaches
   it (none of the 10 logged runs exceeded 100).
4. **Class-signature scheme — spiked and decided (2026-08-31): dedup on `(file, normalized-text)`,
   NO region bucketing.** The spike over the 611-finding corpus reproduced the known 24→8 collapse
   with a plain near-exact-text key; adding line-region buckets or Jaccard similarity made it *worse*
   (24→11), because it split true duplicates that sit in different line-regions. The simpler key is
   the decision. Cross-run class identity for the efficacy numbers (§8) reuses the same normalized
   text; that reuse is confirmed adequate to *measure* recurrence (it produced the 66%/72% figures)
   but is not asserted correct per-class — post-merge recurrence (§3.2 link 4) is the real check.
5. **`classify` runs only when ≥2 findings are confirmed; grouping is advisory-with-recorded-reason,
   and the valve is now WIRED (review fix B7).** A class needs two members, so a single-finding round
   skips the call. When it runs, its grouping is the default the fix agent works to; the agent may
   dissolve a class **by recording a reason keyed on `BugClass.ID`** — the override the design leaned
   on could not previously be pulled, because a class had no id (that was a dead-end hiding in a
   valve). Likewise an `unverifiable` pin's override is keyed on `Pin.ID` (§4.1). *Tighten-if-abused
   (deferred per the maintainer's steer):* the dissolution reason is unreviewed free text and the
   grant is self-servable; post-merge recurrence (§3.2 link 4) is the abuse detector, and an
   authenticated out-of-workflow grant is the tightening if that detector ever fires.

### 4.6 Safety valves & dead-end analysis (the loop must never be unable to proceed)

> **→ Refined by §9 (approved 2026-08-31): deletion is no longer a valve-skip — it is a first-class, provable, *encouraged* fix (§9.4), the additive Guard-and-Go dodge is rejected (§9.5), and test deletion is policed by mutation non-regression, coverage where obtainable (§9.6).** Read §9 as authoritative where it conflicts with the text below.

Design rule, from the maintainer's steer: **strict gates are fine; a gate with no reachable exit is
not.** A blocking gate is acceptable when the actor can always *either* fix the thing *or* pull a
valve. A gate that can block with no possible resolution is a **dead-end** — a bug, fixed here. A
gate whose valve is *gameable* is a **safety valve** — kept simple now, tightened only if abused.

| Gate / block | The strict block | The reachable exit | Reachable? | Gameable valve → tighten-later? |
|---|---|---|---|---|
| `pins_proven` = survived | a real test gap | write/strengthen the test | yes | — |
| `pins_proven` = unverifiable | tree can't answer | recorded override (§4.1) | **only once the override is WIRED to the PinResult — see B7 fix below** | grant is self-servable → tighten later |
| `pins_proven` anchor not in diff (B1) | pin doesn't touch the fix | move the pin onto a changed line | yes — a real fix always has a changed line | — |
| `require_pins` on a fix | code change unproven | supply a pin, OR record "no pinnable change" | yes — the "none" path is always available | "none" is self-asserted → tighten later |
| `pins_proven`: a pure-DELETION fix (no added line to pin) | the fix removed code, nothing to anchor | **superseded by §9.4:** prove it as a `Kind:"deletion"` `DifferentialProof` — a reproduction test (fail-before/pass-after) plus a `DeletionRef`; deletion is now a first-class provable fix, no longer a "no pinnable line" valve-skip | yes | the reproduction test + the `DeletionRef`↔diff binding are machine-checked (§9.4) → hard, not self-asserted |
| `pins_proven` (a) own-file: the finding's remedy is genuinely in ANOTHER file | no pinnable added line in the finding's own file | record "no pinnable change" naming the actual fix file | yes — the same valve as deletion | self-asserted (the fix file is named and in the diff) → tighten-if-abused |
| class member `fixed-elsewhere` (remedy in another file) | own-file pin impossible | resolves via a Proven pin in the REMEDY file `FixInstance.File` (or "no pinnable change" if it has no tests) | yes — invariant 2 makes `fixed-elsewhere` a resolving disposition (PR#31-cursor: without this it cleared the gate but never resolved → class re-demanded forever) | gameable/auditable → tighten-if-abused |
| `classes_enumerated` member unanswered (B3) | a class member ignored | give it a disposition: `fixed`, `already-correct`, or `out-of-scope`+reason | yes — the non-`fixed` dispositions are the valve | reason unreviewed → tighten later |
| `classify` over-groups | unrelated findings lumped | dissolve the class with a recorded reason (§4.5) | **only once dissolution is WIRED to a class id — see B7 fix** | reason unreviewed → tighten later |
| convergence / `Unproven` (B5) | pins keep re-driving discover | a proven pin clears its `Unproven` entry | **only once Unproven has a clearing rule — see B5 fix; today it is a DEAD-END** | — |

Two rows above are true dead-ends today (unverifiable/dissolution valves that can't physically be
pulled, and `Unproven` that never clears); they are fixed in §2–§3 below. The rest are strict gates
with a reachable exit, several of them gameable-but-acceptable per the maintainer's steer — those
carry a **tighten-if-abused** note rather than a hard fix now.

---

## 5. Spikes

> **→ Refined by §9 (approved 2026-08-31): finding-identity KEEPS the deterministic lexical `(file, normalized-text)` key — NOT embeddings (§9.8); the recall floor is set empirically, bucketing out the unsolvable same-fault/different-symptom class.** Read §9 as authoritative where it conflicts with the text below.

**Done (this spec is built on them):**
- Pins closed a full proved loop end-to-end — the model works. ✓
- Findings do not discriminate by file (90% false-positive); they discriminate by lens-convergence
  on a defect (24→8 on `fsmruns.go`). ✓
- The 4 `PinOutcome` values each have a distinct cause+remedy in `verify.go`. ✓

**Dedup-by-defect key — DONE (2026-08-31):** reproduced 24→8 on `fsmruns.go` with a plain
`(file, near-exact-text)` key; region/Jaccard variants under-merged (→11) and were rejected (§4.4).
Global collapse 611→166 (72%) confirms the recurrence baseline. Confirmed the dedup/classify split
holds on real data: exact dedup collapses the 16 true duplicates but preserves the 3 distinct-wording
L118 findings that `classify` should group. **Not** established: pre-cap distinct-per-round vs 300 —
the logs are capped, so this needs a first uncapped run (§3.3 honest limit).

**`classify` grouping — DONE (2026-08-31, review fix B10).** Ran a blind `classify` pass over a
30-finding corpus from the real runs (3 files) with a 3-class / 12-distractor ground truth locked
*before* the run. Results:
- **Recall:** converge-predicate class 4/4; self-clearing-suppression class 3/4 (one member split to a
  singleton — a conservative miss); header class 4/7 *by my label* — but classify **split** my coarse
  7-member class into three finer, defensible classes (unbounded-parsing, coveredpaths-presence,
  untested-behaviour), two of which are real classes I had labelled as distractors. It found structure
  I missed, not noise.
- **Precision:** zero dangerous merges — no group joined two distinct ground-truth cores. Every
  "distractor" it absorbed formed a coherent class (e.g. the "3 of 4 writers untested" class, live
  from the FixClass doc). It correctly separated the forgery-surface bug from the self-clearing bug.
- **Behaviour:** `classify` errs toward **splitting (safe), never lumping (dangerous)**; the only real
  miss was in the conservative direction.
- **Meta-finding that vindicates decision §4.5 (advisory, not binding):** the class boundary is
  genuinely fuzzy — a careful human grader (me) and `classify` disagreed on *granularity* while both
  being defensible. Forcing classify's grouping as binding would therefore be wrong; advisory with a
  recorded-reason override is the right authority model. Caveat: N=1 run, one model, 30 findings — a
  viability demonstration, not a calibration; the build-time Level-3 corpus (§7) still owns the
  numeric bar.

**Remaining, before/while building:**

**Remaining, before/while building:**
- **Escape-hatch probe.** Construct a fix that makes a pin `unverifiable` on purpose (delete the test
  cmd) and confirm decision #1's chosen semantics actually blocks it.
- **Class-in-diff gate.** On a real multi-instance class from the run data, confirm the
  `classes_enumerated` gate fails when an enumerated `fixed` instance is absent from the diff.

---

## 6. Migration / build order

Safe-migration guard first (unchanged): keep `SchemaVersion=1`, additive-optional fields; only
`Delta.Pins/PinResults/Classes/FixClasses` touch the wire; the first pins-writing deploy is a one-way
door — gate it.

### Ship 1 — PINS (shippable, with ONE shared prerequisite)

**Prerequisite ⧗ (PR#30-CR):** cross-round-stable finding identity. `Unproven`'s clear/re-add keys on
`Pin.Finding`, so a re-discovered finding that got a *new* id would never clear its old gap — a
**recall / false-split** failure, and the one PINS is not self-contained without. The spike measured
only the **opposite** direction: `(file, normalized-text)` (the SAME key as the dedup §3.3, no region
bucketing) gave **0% false-merge** over 611 findings — that is *precision* (distinct findings never
collapse), NOT the recall property this prerequisite needs; the 66% figure is recurrence rate, not
the identity invariant, and cannot separate "did not recur" from "recurred but got a new id." The
**recall-under-paraphrase** direction — the actual blocker — is **unmeasured** (no paraphrase ground
truth in the corpus, no protocol in §5/§7/§8). Before PINS can claim self-containment, Ship 1 must
(a) build a paraphrase ground-truth set and define a **recall floor** on it, alongside the precision
floor + same-text/different-file regression; and (b) freeze the algorithm only once BOTH floors are
met. This is **not** de-risked — it is a Ship-1 task, open (⧗) —
the same ⧗ status §3.2 and Ship 2 carry, not "done."

1. Port the pins data model (§2): `Pin{ID,Finding,File,From,To,Test}`, `PinResult`, `PinOutcome`,
   `Snapshot.Unproven`, `Finding.Source`/`Category`. The types were sound; the ids were the hole.
2. Fix the `agentEdit.Reduce` seam + the gate-can-fail regression test (proven red-then-green first).
3. Wire `prove`/`pins_proven` into `sdlc-loop-proved`: the **added-line bind (R1)**, the reduced
   owes-a-pin quantifier + **deletion valve (R3)**, and the `Unproven` add/clear/re-add lifecycle
   (R4). Claim the honest property — *"added lines are test-guarded under mutation,"* never "proof."

That is a complete, honest, shippable improvement over the fix node taking the agent's word. It does
NOT depend on anything below.

### Ship 2 — BUG.CLASS (a validated path, not yet buildable — gated on ⧗ spikes)

Prerequisites, each a small design+spike before code (§3.2, §5):
- ⧗ **Cross-round-stable finding identity** — the base both class-continuity and metric-1 need.
- ⧗ **Durable-identity / continuity spike** — classify continues open classes without id churn.
- ⧗ **End-to-end replay** — class created → partially fixed → carried → cleared only on member
  `pins_proven`. The acceptance test for the whole path.

Then, in order, with `classify` present from the start (no fixer-grouping interim):
4. `classify` node (stateful over open `Snapshot.Classes`) → `BugClass` with minted carried ids.
5. `require_classes` + `classes_enumerated` with the **own-location** member check; class clears only
   on validated member resolution; dissolution/override keyed on the durable `BugClass.ID`.
6. Post-merge recurrence (the abuse detector §3.2 depends on) — or explicitly mark the advisory valves
   unguarded-until-then as a recorded decision (round-2 scope).

Validation (parallel, gates the efficacy claim not the loop): the control arm, a cross-region-capable
sibling oracle (round-2: the §4.4 same-file signature cannot measure cross-file classes), and the
co-present-sibling check on the gold fixtures.

Each step task-done-gated and mutation-verified; no code without a test observed failing without it.

---

## 7. Testing plan across the SDLC

Four levels; each tests a different claim, and a passing level below never substitutes for the one above.

**L1 — Unit + mutation.** Every new type, node, gate. Build rule: no code lands without a test observed failing without it. Each fix mutation-verified. `verify.go` must produce all four `PinOutcome` values from their distinct causes.

**L2 — Integration (FSM machine tests).** A full `discover→adjudicate→classify→fix→prove→verify` with injected outputs. The one that matters most: a fix emitting a pin that does NOT hold its line drives `pins_proven` **red**, and the `agentEdit.Reduce`-drops-`Pins` mutation must redden it (the #24 vacuous-pass). Acceptance: every gate in this feature has a witnessed red and a witnessed green.

**L3 — Behavioural on a known-answer corpus.** The planted-defect fixtures — the only way to measure the *judgement*. Guardrails absolute (100%): a proven fix never blocked; a confirmed finding never dropped by grouping; a planted unproven fix never passed. Judgement relative: `classify` grouping must beat co-location on precision and per-finding-independence on recall — **and the numeric bar (recall floor, max dangerous-merge rate, corpus size, pre-locked ground-truth protocol) is defined HERE, not deferred** (review v2 testing hole 1: the classify spike was viability-only, N=1, and its ground truth was adjusted post-hoc — a pre-locked L3 corpus is what makes the bar real).

**L4 — Efficacy on real runs — see §8.**

---

## 8. Efficacy: how we will know it worked

### Baseline (measured 2026-08-31): 66% of distinct defect-signatures recurred across the 10 runs.

**The baseline is confounded — a control arm is required.** Most baseline runs abandoned at `fix` (control never returned), so they re-discovered defects partly because the loop never finished a fix — a cause the Stop-hook/completion gate already fixed, independent of pins/Bug.Class. To attribute improvement to pins/class, compare a **full arm** against a **control arm** (post-Stop-hook loop, pins + `classify` disabled).

**Open caveat (review v2 testing hole 3, unresolved):** "restricted to bugs the loop reached `fix` on in both arms" is itself selection-biased — pins/class *change* what the loop reaches fix on, so the intersection excludes the bugs where the intervention mattered most. A sound isolation needs both arms replayed from an *identical pre-fix confirmed-finding state per bug*, not a post-hoc intersection. This is not yet solved in the spec.

| # | Metric | Target | Reads which claim |
|---|---|---|---|
| 1 | **Silent-regression re-discovery** — of bugs whose fix passed `pins_proven`, the fraction re-found because the same line broke again with no test catching it (B2: this, not general recurrence, is what a pin prevents) | **→ 0** | pins prevent regression on the proven line |
| 2 | **Sibling-recurrence after a class fix** — a later finding whose §4.4 signature (NOT classify's own grouping) matches a fixed class member | **≤ ½ control-arm rate** | Bug.Class fixes the class |
| 3 | **Rounds to converge / unfixed trajectory** — falls monotonically, reaches zero | fewer rounds | the loop closes |
| 4 | **Cost per proven fix** | bounded, reported | proof is affordable |

**Open caveats on the metrics (review v2, unresolved):**
- Metric-1 needs a **cross-round-stable finding identity** to tell silent-regression from incompleteness recurrence; §2.4 does not yet define one (testing focus-4).
- Metric-2's §4.4 signature is **same-file near-text**, so it structurally cannot see the cross-file "top and bottom" class that is classify's whole reason to exist — it measures the easy case, not the target one (testing hole 2). A cross-region-capable sibling oracle is still owed.

### The gold acceptance test: replay #24–#28.
Run the new loop on those pre-fix states; if `classify` groups the class and `require_classes` forces the sibling, it would have caught what I missed. **Precondition:** verify per fixture that both siblings co-exist at one base ref (they surfaced across five sequential PRs and may not); run each N times with a required hit-rate over `BugClass.Members`, since grouping is non-deterministic.

### Falsifiable success/failure.
Succeeded iff, on real runs vs the control arm: silent-regression re-discovery → ~0, sibling-recurrence ≤ ½ control, convergence in fewer rounds, guardrails never regress, cost bounded. **Failed** — reconsider, don't patch — if recurrence doesn't fall, or if a guardrail had to be loosened to keep the loop moving.

---

## 9. Literature-grounded refinements (APPROVED 2026-08-31)

> **Altitude (added after PR#32 review).** §9 states design-level **requirements and contracts** and
> **defers exact enforcement algorithms to build-time** (marked ⧗) — the git-object verification, the
> mutant-identity scheme, and the schema-decode invariants are settled by code + tests, not by prose.
> Two review rounds showed that specifying those algorithms in prose reliably introduces subtle
> errors; the honest boundary is: *what must be true* here, *how it is checked* at build.
>
> **Residuals pass (mechanical-precision lens, 2026-08-31).** A dedicated buildability review found
> four residual defects the earlier passes missed and they are fixed here: §9.6 no longer requires
> per-test coverage that §4.2's opaque cmd cannot yield (coverage is now an advisory layer, mutation-
> kill is the deterministic gate); §9.2's LLM check is reframed as a **veto-only reviewer**, not an
> "advisory-blocking gate" (the term collided with `PASS_ADVISORY`); §9.7 names its trajectory/action-
> log input and marks it a ⧗ capture prerequisite absent from §2.4; and §9.8 R7 is corrected — a
> trivial pin *survives* the break step and is rejected earlier as `malformed` (§2.2, widened).

These are approved from a deep read of the 2025–26 literature (full evidence + citations:
`docs/research/2026-08-31-pins-bugclass-literature.md`). Where a refinement conflicts with earlier
prose, **this section wins**; the earlier text is superseded and will be reconciled. Each carries a
one-line evidence pointer; the research note has the exact quotes, sections, and URLs.

### 9.1 `prove` is ONE differential-test gate; a reproduction test is preferred, the mutate-a-line pin is the fallback (R1)

A reproduction test and a pin are the same gate — "a test that changes outcome across two
tree-states" — differing only in the "mutation": a reproduction test uses the **real fault**
(fail on the pre-fix tree, pass on the post-fix tree); a pin uses a **synthetic** one (fail on the
mutated line, pass on restore). **Prefer the reproduction test when a real failing input can be
constructed** — it strictly dominates, because it also proves the assertion targets the finding —
and fall back to the mutate-a-line pin otherwise. Both ride the same deterministic machinery.
- *Supersedes:* §2.1 (`Pin` becomes one form of a differential-test artifact — a `{test, target}`
  reproduction test OR a `{File,From,To}` pin), §3.1 (the own-file-binding apparatus is retired for
  the reproduction form — see 9.2 for where it migrates), and the honest-claim text (now: a proven
  reproduction test says the *behavior* is under test; a proven pin says the *added line* is).
- Evidence: fail-before/pass-after F2P is the identical gate (TDD-Bench-Java §2.1); coverage is an
  unreliable proxy for it (§8); reproduction = "a mutant whose mutation is the real fault" (ACH §5.2).
- **The tagged wire contract (PR#32-fix — integrate with the pin state/result model):**

  ```go
  type DifferentialProof struct {
      ID      string       // idempotent content hash of the proof — the reference/override key (was Pin.ID)
      Finding string       // the confirmed finding this proves (the clear key, as Pin.Finding)
      Kind    string       // "reproduction" | "pin" | "deletion"
      Test    string       // reproduction/deletion: the committed test id in the reviewed diff
      Pin     *Pin         // set iff Kind=="pin"    (exactly one of Pin/Deletes per Kind)
      Deletes *DeletionRef // set iff Kind=="deletion"
  }
  ```

  **One-of invariant (PR#32 review):** exactly one shape is populated for each `Kind` —
  `Kind=="pin"` ⇒ `Pin` set, `Deletes` nil; `Kind=="deletion"` ⇒ `Deletes` set, `Pin` nil;
  `Kind=="reproduction"` ⇒ both nil (the `Test` is the whole artifact). ⧗ *Build-time:* enforced at
  decode (a malformed record is rejected, not silently accepted). **Override/`unverifiable` key
  (PR#32 fix):** now `DifferentialProof.ID`, NOT `Pin.ID` — `Pin` is nil for reproduction/deletion, so
  keying on `Pin.ID` would leave those proofs unclearable. `Delta.Pins`/`PinResults`/`Snapshot.Unproven`
  generalise to `DifferentialProof`/`ProofResult`; the four `PinOutcome` values and the clear-by-
  `Finding` rule are unchanged; a `Pin` is the `Kind:"pin"` case, not a separate model. **This
  supersession is the general contract, not a suggestion (review #34):** `ProofResult` is `PinResult`
  with a `DifferentialProof` in place of `Pin` (same `Proven`/`Outcome`/`Detail`, same four
  `PinOutcome` values); §2.4's `Delta.Pins`/`PinResults` and §3.1's `prove`/`pins_proven` **read as
  their `DifferentialProof`/`ProofResult` generalizations** — the pin-only text there is the
  `Kind:"pin"` special case, superseded as the general rule. `pins_proven` then generalizes **by
  clause, not wholesale** (review #34 verify): **`Finding`-clearing and clause (b)** hold for every
  `Kind` — a reproduction/deletion proof clears its `Finding` exactly as a pin does, and a supplied
  non-`Proven` proof of any kind blocks. Clause (a)'s **own-file `File`-binding** (`File==Finding.File`)
  is the **`Kind:"pin"` case only**: a reproduction proof has `Pin==nil` (no pin `File`), so the
  binding is **retired for it and discharged by the §9.2 symptom reviewer** (per *Supersedes* above);
  a deletion proof satisfies clause (a) when its `DeletionRef.File`==`Finding.File` — the deletion
  removes code from the finding's **own** location, the same own-file guarantee a pin's `File` gives,
  so the proven code and the class gate's touched line stay the *same* code and the C4 disjoint-code
  hole stays closed (review #34 verify-2). This is **distinct from** §9.5's `DeletionRef.File`==
  `Root.File` check, which is the structural-class Guard-and-Go gate (`Root` exists only on
  `Remedy:"structural"` classes and may live in another file) — a separate gate, not clause (a). So
  the own-file guarantee is preserved for every kind, by the kind's own mechanism — never by demanding
  a pin `File` a non-pin proof does not carry, and never by substituting the class root for the
  finding's own file. ⧗ *Build-time:* the exact `ProofResult`
  struct, the per-`Kind` pass predicate, and the generalized replay/clearing are the build contract;
  the spec requirement is that **no proof kind may be omitted or silently given pin-only semantics**.
- **Execution contract for a committed reproduction/deletion test (PR#32-fix):** the test lives in the
  reviewed diff (§9.3). `prove` (a) checks out the **pre-fix** tree, (b) **overlays only the test file**
  from the fix commit onto it — **all test-only files the fix adds or changes** (helpers included),
  never production code — (c) runs the target test — it must **FAIL** with an assertion (not
  compile/import) error, (d) applies the full fix, (e) re-runs — it must **PASS**. Capture order is
  fixed: the failing run is on the pre-fix tree with the test files overlaid, before any production
  change. ⧗ Build-time: the test-vs-production file partition is a determinable contract (test paths /
  package membership), with a case for a fix whose test needs a new helper.

### 9.2 Mandatory "fails for the right reason" check — a veto-only reviewer; where the own-file binding MIGRATES (R2)

Bare fail→pass is insufficient: the pre-fix failure must be **the finding's own symptom**, not an
unrelated error. This is the reproduction-form reincarnation of the own-file binding — it is what
stops the agent proving an unrelated behavior. Its shape respects §3.0 ("gates are deterministic
predicates"): the mechanical differential check (fail-before/pass-after) is the **deterministic
gate**; the symptom match is an **LLM reviewer, not a second gate**. That reviewer is **veto-only** —
a mismatch raises a **blocking finding (NEEDS_REVISION)**, and a match adds nothing and never
produces a PASS. So an LLM can veto a proof, never certify one: the deterministic differential is the
sole PASS path, and on LLM approval the outcome is exactly whatever that differential already
returned. (The earlier "advisory-blocking" label is dropped — it collided with the model's
`PASS_ADVISORY`, where *advisory* means the review *passes*; this reviewer only ever blocks.) **Fail-closed (review #34):** a reviewer timeout, error, or
unusable/unparseable output is itself a **blocking `NEEDS_REVISION`** (or a bounded retry) — never
treated as approval, so a missing veto can never become a silent pass.
- Evidence: ablating this gate dropped F2P 56.88%→18.84% (ReProAgent).

### 9.3 Anti-gaming for the self-validating loop (R3)

We validate against the agent's own fix (the weak "candidateBRT" bar), which is co-adaptable. Three
deterministic guards: (a) the test is a **durable committed artifact** in the reviewed diff — it
cannot be silently deleted (the #1 documented failure — the agent deleted its own test in 77% of
failure cases); (b) the pre-fix run must be an **assertion failure, not a compile/import error**;
(c) capture the failing test on the **untouched tree first**.
- Evidence: Cogeneration §6 (our exact single-agent-emits-fix+test setting).

### 9.4 Deletion as a first-class, provable, ENCOURAGED fix (maintainer-directed)

The original mutate-a-line pin structurally **penalized** deletion (no added line to pin → unproven),
rewarding the additive Guard-and-Go dodge instead. Fix:
- **R1 removes the disincentive:** a deletion that fixes a bug has a reproduction test that is
  fail-before/pass-after (the pre-fix tree still has the code → bug reproduces → fail; the post-fix
  tree removed it → pass). The deletion *is* the mutation; no added line needed.
- **`DeletionRef` — a whole-file blob PLUS the removed span (PR#32-fix).** The removed text is NOT a
  git blob (blobs are whole files), so hashing "the removed text" as a blob and verifying it against
  `ParentSHA:File` was wrong — a legitimate partial deletion would fail while any file edit
  (including Guard-and-Go) would pass. The evidence is two separate identities:

  ```go
  type DeletionRef struct {
      File      string // repo-relative path the code was removed from
      ParentSHA string // a commit where the pre-deletion file still exists (durable in history)
      FileBlob  string // git blob hash of the WHOLE ParentSHA:File — the durable, content-addressed container
      Removed   string // the exact removed SPAN (the deleted text) — the deletion's own identity
  }
  // idempotent deletion id = hash(File + "\x00" + Removed); FileBlob+ParentSHA anchor it in real history
  ```

  **What proves a deletion (PR#32 round 2): the REVIEWED DIFF, anchored durably — not free-floating
  history.** Containment-in-some-parent + absent-from-HEAD is necessary but NOT sufficient (a rewrite
  or an unrelated edit satisfies it). The binding requirement: `ParentSHA` is **this fix commit's
  parent**, and the commit's own diff for `File` **removes exactly `Removed`** (`Removed` appears as
  deleted `-` lines in the reviewed change). `FileBlob` (the whole `ParentSHA:File` blob) is the
  durable, content-addressed anchor so the removed span stays referenceable forever; `Removed` is the
  deletion's identity. ⧗ *Build-time contract:* the exact diff-parse + containment + one-match check
  (and rejecting a same-span reappearance elsewhere in the file) is an implementation task with tests
  — the requirement is "the reviewed commit deletes this span from this file, anchored at its parent."
- **Encouragement levers:** stop punishing it (R1); post-merge learning **rewards** a class resolved
  by a `DeletionRef` (structural simplification — the inverse of the verbosity flag, 9.7); hand
  **exact deletion spans** in the fix prompt, not prose (R10: spans move models 6.5–31.5 pts, prose
  does nothing).
- **Honest limits:** a proven deletion means "removing this fixed the reported bug and broke no
  *existing* test" — NOT "globally safe" (removal risk is in *untested* behavior); over-deletion
  (~26%) needs a paired whole-suite scope check.
- Adversarial: the reproduction test is the behavioral anchor (a bogus `DeletionRef` won't hold F2P);
  the `DeletionRef` is the durable audit trail. Neither alone.
- *Supersedes:* §4.2/§4.6 — deletion moves from a "no pinnable change" valve-skip to a first-class
  provable fix.
- Evidence: additive bias is large and named (*To Add Is Machine, To Delete Is Human*).

### 9.5 Guard-and-Go: reject the additive dodge of a required deletion (R4)

The dominant real-world failure — fixing a should-be-*deleted* bug by *adding a guard* around the
root cause, leaving it live (29% of passing patches; 40.2% of those keep the removed logic as the
default path). It passes both `pins_proven` (the guard is pinnable) and `classes_enumerated` (the
reported member is "answered").
- **Made machine-checkable (PR#32-fix).** `classify` tags each `BugClass` with a `Remedy`
  (`"structural"` — mechanism is a redundant/wrong structure whose fix is removal — or `"local"`).
  For a `Remedy:"structural"` class, `classify` declares the **root** as a `BugClass.Root
  struct{ File string; Span string }` (a `ClassMember` has no span field — PR#32 fix), and
  `classes_enumerated` requires a fix `DeletionRef` whose `File`==`Root.File` and whose `Removed`
  contains `Root.Span`, verified by §9.4. ⧗ *Build-time data-availability:* `classify` must produce
`Root.Span` as actual source text from findings that carry only file+line — reading the span from the
file at that line is a build-time task; the requirement is that a structural class names a concrete
removable span, not a bare line number. A `fixed` FixInstance that only adds lines and carries no
  matching root `DeletionRef` **does not close** a structural class — the guard-around dodge. A
  `local`-remedy class is unaffected. The match (`DeletionRef` ↔ `Root`) is the machine-checkable
  relation; ⧗ its exact form (equality + containment) is a build-time contract.
- Evidence: same paper as 9.4.

### 9.6 Test-deletion gate — mutation non-regression (deterministic), coverage where obtainable (maintainer-directed)

Deleting a **test** is a first-class reward-hack; a green suite afterward proves nothing. A test
deletion is legitimate iff **mutation-kill non-regression holds** (always run) **and, wherever a
coverage signal is obtainable, coverage non-regression also holds**. Where §4.2's opaque
consent-hashed cmd yields no coverage signal, the coverage conjunct is simply **absent — not a
block** (§4.2 says it may not exist), and mutation-kill alone gates. The rule is one predicate, not
two: coverage is a hard conjunct exactly when a signal exists, never otherwise.
1. **Coverage non-regression (a conjunct where a coverage signal exists).** Where the project exposes
   production coverage, the deleted test's coverage must be subsumed by remaining tests (or the
   covered code was deleted in the same commit) — a **hard requirement when a signal is obtainable**.
   Coverage is **necessary-where-obtainable but not sufficient**, and **not always obtainable**: it
   measures *execution*, not *assertion* (it catches deleting the sole *exerciser* of lines, blind to
   deleting the sole *detector* on already-covered lines), and **§4.2's opaque consent-hashed test cmd
   yields no coverage to attribute** — a single all-or-nothing invocation, not a per-test instrumented
   run. So the coverage conjunct **applies where a signal is available and is recorded *unavailable*
   otherwise** — never required on a capability §4.2 says may be absent — and mutation-kill (2) gates
   alone in that case. A usable **coverage signal means a per-line/per-file coverage
   *profile*** the cmd can emit (e.g. a Go `-coverprofile`) — measured once with the test present and
   once with it deleted, then **diffed**: a production line that goes covered→uncovered is a
   regression (review #34 verify-2). Per-*test* attribution is **not** required — a whole-suite
   before/after profile diff answers "is the deleted test's coverage subsumed by the rest." What does
   **not** qualify is the **aggregate pass/fail** an opaque cmd emits with no line profile at all
   (§4.2's single all-or-nothing invocation): that counts as *unavailable*, so mutation-kill gates
   alone. ⧗ *Build-time:* detect whether the cmd yields a line coverage profile and select the
   applicable layers.
2. **Mutation-kill non-regression** — no mutant killed before the deletion survives after it. The
   load-bearing half; fills coverage's blind spot (the R8 277/571 finding); reuses the pins' engine.
   **Mutant identity + deleted-target exclusion (PR#32, round 2):** a mutant needs a **unique,
   stable id across the diff** — `(File, mutated-span-content)` alone collides when the same content
   recurs or two operators hit one span, so the id must also fold in the site and operator. ⧗ The
   exact scheme (e.g. `(File, normalized-site, operator, span)`) is a build-time contract with a
   collision test. A mutant whose target span was **removed** in the same commit — provable because
   it lies within a fix `DeletionRef`'s `Removed` (§9.4) — has no post-change target and is
   **excluded** from the non-regression set (not a "lost kill"; the code it tested is legitimately
   gone). Every *other* previously-killed mutant must still be killed — this is what lets a paired
   code+test deletion pass.

Layered (each closes the prior's blind spot, attack surface shrinks each layer): **coverage (where
obtainable) → mutation (the deterministic gate) → right-reason (9.2)**. Reconciliation: we reject
coverage as a *proof of fix* (9.8) but embrace it as a *guard against test-deletion gaming* — used
where necessary, not relied on where insufficient, and **run only where §4.2's opaque cmd actually
yields a coverage signal**. That is why mutation-kill is the always-available gate (it needs only
per-mutant pass/fail from the same opaque cmd) and coverage is a required conjunct wherever a signal
exists (and simply absent, never a block, where it does not). A test deleted because its code was deleted is the legitimate paired case,
accounted by one `DeletionRef` and proven safe by mutation-kill non-regression (plus coverage
non-regression where obtainable).
- Evidence: coverage ≪ mutation for fault detection (277/571, ACH §1); test-oracle tampering as a
  named hack (Verification Horizon).

### 9.7 Trajectory monitor — watch HOW the fix was produced, not just the end state (R5)

Our final-state gates are blind to a whole hack class (test-oracle/harness tampering, answer-lookup,
whitespace-touch). Add a monitor with two declared input sources, because the flags split by what the
data model already produces:
- **From existing artifacts — the reviewed git diff and the override records — NOT new `Delta` wire
  fields (review #34):** whitespace/comment-only "changes" (read from the diff), and self-served
  overrides (the §4.1/§4.5 override records keyed on `DifferentialProof.ID`/`BugClass.ID` per §9.1 —
  `Pin.ID` is nil for reproduction/deletion proofs). These need no addition to §2.4's persisted
  `Delta`; the monitor reads them from the diff and the override log directly.
- **From a fix-node action/trajectory record — which §2.4's wire model does NOT yet carry:** edits to
  the oracle/harness *within the proving trajectory*, and answer-lookup (git-history / PR /
  upstream-diff / network fetches). The fix node today emits only `{commit, summary, pins[]}`, from
  which neither is recoverable. ⧗ *Build-time prerequisite:* capture a fix-trajectory record —
  minimally the ordered per-step edits (path + content delta) and any fetch/network attempt — and add
  it to §2.4's additive-optional wire model as this monitor's declared input — its exact fields,
  persistence, and replay/ordering are the ⧗ build-time schema contract, like every other
  additive-optional field in §2.4. The trajectory-dependent flags are **blocked on that record**; the
  diff/override flags do not wait for it.

Feed newly-seen patterns back from post-merge recurrence — the co-evolution loop the literature says
is mandatory.
- Evidence: a trajectory monitor drove hacked-resolved 28.57%→0.56% (Verification Horizon); **caveat:
  that magnitude is a training-time penalty — the signal transfers to an inference gate, the number
  does not.**

### 9.8 Smaller, evidence-backed

- **Finding-identity: KEEP the lexical `(file, normalized-text)` key (R6) — do NOT switch to
  embeddings.** A tuned lexical method beat deep-embeddings by 22.3% Recall@10, and embeddings are
  *non-deterministic* across runs — fatal for a *stable id*. Embeddings only as advisory suggestions,
  never the id. Recall floor set empirically, bucketing out the "same fault, different symptom" class
  (unsolvable by any content method). *Supersedes §5's implication that embeddings are the answer.*
- **Reject trivial pins (R7):** a comment/whitespace/dead-code pin *compiles and breaks no test*, so
  the break step would return **`survived`** (§2.2) — the wrong signal, since it sends the actor to
  "write/strengthen a test" for a comment. Reject such pins **before** the break step with a cheap
  **AST pre-screen** (plus explicit reject-tests), classifying them **`malformed`** (§2.2, widened to
  cover a compiles-but-semantically-null mutation) so the fix agent rewrites the pin rather than
  chasing a phantom test gap.
- **Cite 277/571 (R8)** as the external validation for rejecting line-coverage as proof-of-fix.
- **Verbosity/convention dimension in "acceptable" (R9):** a fix materially larger than the minimal
  change (Guard-and-Go median 1.67×) is flagged. Exact merge-rate is in Whitfill et al. 2026 (cited,
  not fetched) — a gap, not invented.
- **"Machine-generated vs agent-declared pin" is the wrong axis:** LLM mutants are quantifiably
  worse-formed, so the deterministic *gate* is the authority, not the generator — keep pins
  agent-declared.
- **"No silver bullet" (R11)** ratifies the honest-limits stance (a pin proves a line/behavior under
  test, not correctness — an undecidable property) and endorses the layered, co-evolving defense.
