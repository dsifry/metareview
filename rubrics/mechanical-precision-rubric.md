# Mechanical-Precision Review Rubric

Use this rubric for the **Mechanical-precision** lens of an artifact/code review. The
Mechanical-precision lens asks one question and only one: **can an implementer build *exactly*
what is written, without inventing a detail the artifact left undefined?** It is the "hostile
implementer" lens — read every contract as an adversary who will implement the most literal,
laziest reading and see what breaks.

This lens is distinct from Feasibility and Architecture, and the boundary is the point of the
lens:

- **Feasibility** asks whether the approach *will work* — is the path viable in principle.
- **Architecture** asks whether the structure is *right* — is the data model sound, cohesive,
  decoupled.
- **Mechanical-precision** takes both of those as granted and asks whether what is specified is
  *precise enough to build one specific correct thing*. A design can be a good idea (feasible)
  built on a sound model (architecture) and still be unbuildable-as-written because a referenced
  field is defined nowhere, an invariant is stated in prose with nothing to enforce it, or an
  operation is described so two reasonable implementers build two incompatible things.

The adversarial stance: assume the artifact contains a contract that cannot be built as written
— find it. Do not confirm the design reads well; hunt for the referent an implementer would have
to invent, the invariant nothing enforces, the sentence that admits two incompatible builds.

## Verdicts

- PASS: no blocking mechanical-precision findings — every buildable contract in the artifact
  determines one correct implementation.
- NEEDS_REVISION: one or more blocking findings (undefined referent, unenforced invariant,
  ambiguous operation with divergent reasonable readings, identity collision, cross-section
  mechanism contradiction, undefined execution/verification model).
- ESCALATE: a contract's buildability depends on a decision the artifact cannot settle by itself
  and that belongs to a human (a mechanism the whole design is built on is undefined and picking
  one is a design decision, not a clarification).
- NOT_APPLICABLE: the artifact specifies nothing that will be built to — a pure narrative,
  status, or discussion doc with no data models, contracts, algorithms, or operations. State the
  surface you checked and found absent.

## What To Hunt For

For each category, find the case where an implementer building *exactly* what is written
produces something wrong, ambiguous, or impossible. Report each distinct issue with the section
(or file:line) and the verbatim text that makes the finding true. The discipline that keeps this
lens from becoming "I'd like more detail": a blocking finding must name **what an implementer
would have to invent or guess**, and show either that the literal reading is wrong or that two
reasonable readings diverge into incompatible builds. "The artifact does not determine one
correct build" is the bar; "I would have written it differently" is not.

### Undefined Referents

- A type, field, function, artifact, or file referenced by a contract but defined nowhere in the
  artifact — the implementer must invent its shape.
- A field used in one section that is absent from the data model the other section defines (a
  root referencing a member's `span` when the member type has no span).
- Block on a contract that names something the artifact never defines.

### Unenforced Invariants

- A "one-of" / "exactly one" / "mutually exclusive" constraint stated in prose with no field,
  discriminator, or structure that makes the illegal state unrepresentable — nothing stops an
  implementer from building a value that violates it.
- A stated uniqueness/ordering/cardinality rule with no key or mechanism that enforces it.
- Block on an invariant the data model permits violating.

### Ambiguous Operations (Divergent Literal Readings)

- A concrete operation — a git query, a comparison, an execution step, an ID derivation —
  described so that two reasonable implementers build two incompatible things, and the difference
  is behavioral (not cosmetic).
- A verification described by its *intent* ("verify the deleted code existed") whose most literal
  reading is trivially true for any input, or is not the operation the intent needs (checking a
  whole-file blob when the claim is about a removed span).
- Block on an operation whose literal reading is wrong, or whose reasonable readings diverge into
  different behavior.

### Identity & Collision Gaps

- An ID, key, or fingerprint scheme that is null/undefined for a case it must cover, or that
  collides across two instances it is required to tell apart.
- A shared key derived so that two distinct records map to one (a proof keyed on a field that is
  empty for one of its own kinds; two paired mutants with identical identity).
- Block on an identity scheme that collides or is undefined where it must hold.

### Cross-Section Mechanism Contradictions

- Two sections that each specify a concrete mechanism, where the mechanisms are incompatible: one
  section requires data or capability a *decision recorded elsewhere in the same artifact* says is
  not available (a per-test coverage attribution required by one section, forbidden by another's
  "the test command is opaque, no coverage instrumentation").
- A gate/outcome kind used in one place that is not among the kinds the model defines.
- Block on two contracts in the same artifact that cannot both be built.

### Undefined Execution / Verification Model

- A check described by *what it concludes* but not by the operations that perform it — "execute
  the test in a tree with the deletion applied" without saying which tree, built how, or how its
  result is read back.
- A monitor/gate that consumes data (an action log, a trajectory, a per-step record) the
  artifact's persisted model does not produce — the data source does not exist.
- Block on a check whose inputs the artifact never says how to produce, or whose steps it never
  states.

## What NOT To Flag (Anti-Overlap)

- Do NOT flag whether the approach is sound or will work (defer to Feasibility). This lens
  assumes the approach and judges whether it is specified precisely enough to build.
- Do NOT flag whether the data model or structure is well-designed, cohesive, or decoupled
  (defer to Architecture). "This is the wrong model" is Architecture; "this model as written has
  a field referenced but undefined / an invariant nothing enforces" is Mechanical-precision.
- Do NOT flag a *missing* requirement (defer to Completeness). The boundary is sharp: something
  **absent** is Completeness; something **present but not precisely buildable** — underspecified,
  ambiguous, self-contradictory — is Mechanical-precision.
- Do NOT flag scope drift (Scope and alignment), security vulnerabilities (Security), test
  quality (Testing-quality), or migration safety (Data-migration).

## Evidence Rules

Every blocking finding must cite the artifact (section heading or file:line + the verbatim
contract text that makes the finding true — the "quote-the-line" gate) and state, concretely,
**what an implementer would have to invent or guess** and why the gap is behavioral: the literal
reading that is wrong, the two reasonable readings that diverge, the field referenced but never
defined, or the two sections that cannot both be built. A finding that only says "add more
detail" without naming the undetermined build is not a mechanical-precision finding — it is a
preference, and belongs nowhere in the review.
