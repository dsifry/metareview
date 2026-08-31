# Literature grounding for the pins / Bug.Class design (2026-08-31)

The pins/Bug.Class spec (`docs/specs/2026-08-31-pins-and-bug-class.md`) was originally derived
bottom-up from this repository's own run data and adversarial review, **not** from prior research.
This note is the evidence trail from a deep read of the recent (2025–26) literature, done by five
parallel deep-read subagents (full-text reads, not skims), used to approve or revise the design.

Reading discipline: each subagent was told a **clean discard is as valuable as a finding**, and to
cite an exact quote + section + URL so a human can verify the source directly. Two WebFetch
*summarizer* hallucinations were caught and re-derived from raw text (a fake "Opus 4.8" stat →
Opus-4.5; a mis-attributed 28.6→0.6 number). Treat quotes as high-fidelity paraphrase; raw extracts
were saved under the session scratchpad at read time.

---

## Approved refinements, with evidence

Ranked; each names the paper (§ + quote + URL) and the spec section it changes.

### R1 — Unify `prove` into ONE differential-test gate; prefer a reproduction test, keep the mutate-a-line pin as fallback

A reproduction test and a mutate-a-line pin are the **same gate** — "a test that changes outcome
across two tree-states." The difference is only the "mutation": a reproduction test uses the **real
fault** (fail on pre-fix, pass on post-fix); a pin uses a **synthetic** one.

- *"The evaluation harness executes the generated test twice, once each on the code before (c_old)
  and after (c_new) issue resolution… fail-to-pass, which means r_old is fail and r_new is pass."* —
  TDD-Bench-Java §2.1 / Fig. 1, https://arxiv.org/pdf/2605.04320
- The reframe (reproduction = a mutant whose mutation is the real fault, same fail-before/pass-after
  gate): Meta ACH read of *Mutation-Guided LLM-based Test Generation at Meta*, FSE '25,
  https://arxiv.org/pdf/2501.12862 §5.2.
- Coverage is a *worse* signal than F2P: *"fail-to-pass tests tend to have higher coverage… However,
  many non-fail-to-pass tests exhibit similar coverage levels. It makes coverage an unreliable
  indicator of the fail-to-pass property."* — TDD-Bench-Java §8.

**Spec:** §2.1 (`Pin` → one form of a differential-test artifact), §3.1 (`prove` = differential
gate; the own-file-binding apparatus is retired for the reproduction form). **Prefer reproduction
when a real failing input can be constructed** (it strictly dominates — it also proves the assertion
targets the finding); pin otherwise.

### R2 — Add a mandatory "fails for the right reason" gate (this is where own-file binding MIGRATES, not disappears)

Bare F2P is insufficient; the pre-fix failure must be *the finding's* symptom.

- Ablating this gate dropped one system's F2P **56.88% → 18.84%** — ReProAgent,
  https://arxiv.org/html/2607.09123v1 (triadic review).
- *"asks whether the test is failing for the right reason: the problem described in the issue
  description."* — e-Otter++ §4.1, https://arxiv.org/pdf/2605.04320.

**Spec:** new advisory-blocking sub-gate in §3.1/§3.2 — after the mechanical fail→pass, a judge
confirms the pre-fix failure *is* the finding's behavior. LLM-judged ⇒ advisory-blocking, not a hard
deterministic PASS.

### R3 — Guard the self-validating-loop gaming our setting uniquely has

We validate against the agent's **own** fix (the weak "candidateBRT" bar), so fix and test can
co-adapt.

- Our exact setting (one agent emits fix + test): *"the agent sometimes implements a fix biased
  towards its own BRT… the passing outcome stops the agent from scrutinizing its implementations."*
  and the #1 failure was the agent **deleting its own test** — *"cleaned up the test changes before
  finishing (77%)."* — Cogeneration §6, https://arxiv.org/html/2601.19066v3.

**Spec:** §3.1 + §4.6 — the test must be a **durable committed artifact** (in the reviewed diff,
not silently removable); the pre-fix run must be an **assertion failure, not a compile/import
error**; capture the failing test on the **untouched tree first**.

### R4 — "Guard-and-Go": an additive patch that dodges a required deletion passes BOTH gates

The dominant real-world failure, and a direct hit on our gates.

- *"Retained Path as Live Fallback accounts for 221 of 550 typed pairs (40.2%)… the developer-removed
  logic remains the default path for every other input."*; Guard-and-Go is **29% of all passing
  patches**, median **1.67× larger** than the developer fix. — *To Add Is Machine, To Delete Is
  Human*, https://arxiv.org/html/2607.28887.
- Models localize to the right file >92% but *"remove the exact line in only 44.6%–51.6% of cases."*

**Spec:** §3.2 `classes_enumerated` gains a per-member **removes-vs-guards-around** distinction (a
guarded *reported case* does not close the class for its siblings); §4.2/§4.6 deletion valve must
distinguish "legitimately nothing to pin" from "additively dodged a required deletion." See also the
deletion path below.

### R5 — Add a trajectory monitor; the anti-gaming lift is watching HOW the fix was produced, not the end state

- *"the monitor reduces average hacked-resolved rate from 28.57% to 0.56%, while improving clean
  resolved rate from 40.22% to 60.53%"*, by logging *"command history, network accesses, git
  operations, opened and edited files, and final patch."* — The Verification Horizon, Table 3,
  https://arxiv.org/html/2606.26300v2.
- **Caveat:** that magnitude is from an RL *training* penalty; the *signal* transfers to an
  inference-time gate, the *number* does not.
- Taxonomy of hacks invisible to any final-state check: *"test-oracle tampering, evaluation-harness
  tampering, visible-test overfitting, evaluator-aware patching"* + answer lookup. Our whitespace-
  touch / relabel / self-serve-override are instances of this named class.

**Spec:** new §3 trajectory-monitor gate over the fix agent's action log — flag whitespace/comment-
only "changes," edits to the oracle/harness within the proving trajectory, answer-lookup
(git-history/PR/upstream-diff/network), and self-served overrides.

### R6 — Finding-identity: KEEP the lexical key; do NOT switch to embeddings (this REVISES my earlier surface-level advice)

- A tuned lexical/IR method beat deep-embedding models by **22.3% Recall@10**: *"REP, proposed a
  decade ago, can outperform… deep learning by 22.3% on average."* — *Duplicate Bug Report Detection:
  How Far Are We?* §1, https://arxiv.org/pdf/2212.00548.
- Disqualifier for a *stable id*: *"Due to the nature of deep learning based approaches, the results
  can vary among different executions."* (§7.1) — embeddings are **non-deterministic**.
- Modern embeddings are also weak here: SBERT Recall@10 = 0.61 with *"most top-10 similarity scores
  below 0.5."* — GitBugs §IV, https://arxiv.org/pdf/2504.09651.
- A class no content method solves: *"different failures while the underlying fault is the same…
  challenging for [similarity-based approaches] to detect."* (§6.3) — bounds any recall floor.

**Spec:** §5 finding-identity + §3.3 dedup — keep `(file, normalized-text)` as the *authoritative,
deterministic* id; embeddings only as *advisory suggestions*, never the id; set the recall floor
**empirically**, **bucketing out** the "same fault, different symptom" class.

### R7 — Reject trivial pins explicitly + cheap AST pre-screen

- Meta's worst LLM mutant is the *"misleading comment"* (comment-only, no executable change); their
  *"simple pre-processing"* lifted equivalence precision 0.79→0.95. — ACH §3/§5.2,
  https://arxiv.org/pdf/2501.12862. Our break-step rejects these by execution, but add reject-tests +
  a pre-screen for efficiency. **Spec:** §7 tests / §3.1.

### R8 — Cite 277/571 as the coverage-rejection evidence

- *"Of these 571 tests, 277 would have been discarded had we chosen… line coverage… underlining the
  importance of mutation testing over such coverage."* — ACH §1 (≈49% of fault-catching tests added
  zero coverage). **Spec:** §1/§3.1 external validation.

### R9 — A verbosity/convention dimension in "acceptable"

- Guard-and-Go's median **1.67×** size inflation is a cheap proxy (flag a fix materially larger than
  the minimal change). — https://arxiv.org/html/2607.28887. **Honest gap:** the exact "maintainers
  merge below benchmark score" *number* is in **Whitfill et al. 2026** (cited, not fetched) — flagged,
  not invented. **Spec:** §8 "acceptable" definition.

### R10 — Hand exact deletion spans in fix prompts, not prose

- *"only the spans help every model… Exact spans raise success by 6.5–31.5 points"*, and even then
  *"26.0% of tasks still fail after complete target removal"* (over-deletion). — CanItDelete ladder,
  https://arxiv.org/html/2607.28887. **Spec:** fix-node prompt contract; pair with a scope check.

### R11 — "No silver bullet" ratifies the honest-limits stance; recurrence monitor = the co-evolution loop

- *"every non-trivial semantic property of a program is undecidable"* (Rice); *"test-driven rewards…
  can verify whether a patch passes the tests, but not whether it was produced through legitimate
  software engineering."* — The Verification Horizon, https://arxiv.org/html/2606.26300v2. Endorses
  layered defenses + a co-evolution loop (feed newly-seen gaming patterns back into R5's monitor).
  **Spec:** no change; a validation to cite.

---

## Deletion as a first-class, provable, ENCOURAGED fix (maintainer-directed refinement)

Rationale (maintainer): simplifying deletions — usually a data-model/structure fix first — are among
the most valuable changes, and the *original* mutate-a-line pin structurally **penalized** them (no
added line to pin → unproven → the additive Guard-and-Go patch was the pinnable, rewarded one).

- **R1 removes the disincentive.** A deletion that fixes a bug has a reproduction test that is
  fail-before / pass-after: the "before" tree still contains the code (bug reproduces → fail), the
  "after" tree has it removed (→ pass). No added line required; the deletion *is* the "mutation."
- **`DeletionRef` — a durable, content-addressed identity for removed code.** The removed lines are
  gone from HEAD but immortal in git history. `DeletionRef{File, BlobSHA, ParentSHA}` gives a
  deletion the same kind of idempotent identity `Pin.ID` gives a fix: `BlobSHA` (git blob hash of the
  removed content) is content-addressed and byte-stable across machines; the gate verifies the claim
  **deterministically** — the blob exists in `ParentSHA:File` AND is absent from HEAD's `File`.
- **Composes with Bug.Class.** `classify` finds the data-model class; the right fix simplifies the
  root (a `DeletionRef`), and R4's guard-vs-remove rule rejects the additive dodge.
- **Encouragement levers:** (1) stop punishing it (R1); (2) post-merge learning rewards a class
  resolved by a `DeletionRef` (structural simplification — the inverse of R9's verbosity flag);
  (3) guard-around is not a valid class answer when the mechanism is structural; (4) hand exact
  spans (R10).
- **Honest limits (do not oversell):** a proven deletion means *"removing this fixed the reported
  bug and broke no existing test"* — NOT "globally safe" (removal risk lives in *untested*
  behavior). And over-deletion (R10: ~26%) needs a paired whole-suite scope check.
- Adversarial: the reproduction test is the behavioral anchor (a bogus `DeletionRef` for incidental
  code won't hold F2P); the `DeletionRef` is the durable audit trail. Neither alone; both together.

---

## Test deletion — policed by coverage AND mutation non-regression (maintainer-directed refinement)

Deleting a **test** is a first-class reward-hack ("delete the assertion," "test-oracle tampering" —
Verification Horizon). A green suite after a test deletion proves nothing. Rule: a test deletion is
legitimate iff, on the branch diff, **neither regresses**:

1. **Coverage non-regression** — the deleted test's production coverage is subsumed by remaining
   tests (or the covered code was deleted in the same commit). Coverage is **necessary but not
   sufficient**: it measures *execution*, not *assertion* — it catches deleting the sole *exerciser*
   of lines, but is blind to deleting the sole *detector* on already-covered lines.
2. **Mutation-kill non-regression** — no mutant that was killed before the deletion survives after
   it. This is the load-bearing half, and it fills coverage's blind spot — exactly the R8 277/571
   finding (~49% of fault-catching tests add no coverage). It reuses the same mutation engine the
   pins run.

Layered defense (each closes the prior's blind spot, and the attack surface shrinks at each layer):
**coverage → mutation → right-reason** (R2). Delete-red-test → coverage catches it; delete + add a
weak re-exercising test → mutation catches it; delete + add a test that kills the mutants for the
*wrong* reason → only the right-reason gate catches it (soft/LLM-judged; everything below is
deterministic).

**Reconciliation:** we rejected coverage as a *proof of fix* (R8) but embrace it as a *necessary
guard against test-deletion gaming* — used where it is necessary, not relied on where it is
insufficient. A test deleted because its production code was deleted is the legitimate paired case,
accounted by one `DeletionRef` and proven safe by the two non-regressions.

**Spec:** §3 (a test-deletion gate) + §7/§8.

---

## Discards (a clean discard was an expected, valued outcome)

- **LLMs Gaming Verifiers (2604.15149)** — wrong domain (inductive-logic puzzles, not coding).
  Survives only as an analogy: its isomorphic-perturbation-testing ≈ a possible perturbation probe
  for `classes_enumerated`. Do not cite for coding-agent monitoring.
- **SpecBench (2605.21384)** — *measures* reward hacking in coding agents but proposes **no** defense.
  Useful only for its hack taxonomy and its negative result (more tests/search/bigger models don't
  close the gap → justifies layering).
- **What's in a Benchmark? (2602.04449)** — **full discard**; a SWE-Bench leaderboard-dynamics study,
  no merge-rate or maintainer-rejection content. The "passing ≠ mergeable" claim is actually in R4's
  paper, not here.
- **Meta engineering blog (fb.com)** — derivative; every load-bearing number is first-hand in the FSE
  paper (2501.12862). Cite the paper.
- **A Comprehensive Study on LLMs for Mutation Testing (2406.09843)** — only its abstract decoded
  cleanly; used narrowly (LLM-generated mutants are *worse*-formed, so "machine-generated pins" buys
  nothing — the deterministic *gate*, not the generator, is the authority).
- **GitBugs (2504.09651)** for "which method wins" or determinism — a single SBERT baseline, no
  head-to-head, silent on reproducibility.

## Where the deep reads REVISED earlier surface-level advice

1. **Embeddings → keep the lexical key** (R6) — the controlled comparison + the determinism
   requirement reverse the search-level "use SBERT/Faiss."
2. **Replace the pin → unify** (R1) — reproduction-test and pin are the same gate; reproduction is
   preferred, the pin is the fallback, neither is discarded.
3. **Add an LLM judge → add a trajectory monitor** (R5) — the anti-gaming lift is watching *how* the
   fix was produced, not inspecting the end state.
