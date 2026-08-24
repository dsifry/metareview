# Testing-Quality Review Rubric

Use this rubric for the **Testing-quality** lens of an artifact/code review. The Testing-quality
lens attacks the assumption that the tests verify the behavior they claim to. Tests can lie —
they can pass while asserting nothing, cover mocks instead of real logic, or stay unchanged
when the behavior they purport to cover changed in the diff. This is a *diff-scoped* lens:
judge whether the test changes in this diff actually verify the behavior changes in this diff,
not whether the entire test suite is comprehensive.

The adversarial stance: assume there may be a fundamental mistake hiding in the tests — find
it. Do not confirm the tests are well-shaped; hunt for the test that looks thorough but proves
nothing.

## Verdicts

- PASS: no blocking testing-quality findings.
- NEEDS_REVISION: one or more blocking testing-quality findings (false-confidence assertions,
  behavioral change with no test modification, tests exercising only mocks).
- ESCALATE: a test's coverage cannot be determined from the diff alone (test runner config or
  CI matrix not visible).
- NOT_APPLICABLE: the diff touches no code that has or should have tests (pure docs, config
 -only). State the surface you checked and found absent.

## What To Hunt For

For each category, find the case where the tests lie. Report each distinct issue with file:line
and the test code. Only flag issues you are confident are real testing-quality defects in THIS
diff, not generic "add more tests" advice.

### False-Confidence Assertions
- `toBeTruthy()`, `toBeDefined()`, `not.toBeNull()`, "doesn't throw", or bare `assert(x)` that
  assert nothing about the behavior — the test passes regardless of whether the code is correct.
- A test that checks a return value's existence but never its content, type, or shape.
- A test with only a `try/catch` that swallows errors and passes unconditionally.
- Block on assertions that cannot fail when the code is wrong.

### Behavioral Change With Zero Test Modifications
- New logic, changed branches, or modified state transitions in the diff with no corresponding
  test change — the tests are stale and will pass even though they no longer cover the new
  behavior.
- A function signature changed (new param, changed return type) but no test caller updated.
- Block on a behavioral change in the diff with no test modifications.

### Tests Verifying Mocks, Not Real Logic
- A test that asserts the mock was called with certain args but never checks the real return
  value or side effect.
- A test where the mock replaces the unit under test so thoroughly that the real code is never
  exercised.
- A spy/stub configured to return a canned value, then the test asserts the same canned value
  back — a tautology.
- Block on tests that exercise only the mock, not the real code.

### Untested New Branches / Lifecycle Paths
- A new `if`/`switch` branch, a new error path, a new lifecycle hook
  (`onMount`/`onUnmount`/`beforeDestroy`/`componentDidCatch`) with no test that triggers it.
- A new edge case handled in the code (null input, empty list, concurrent call) with no test
  for that case.
- Block on a new branch or lifecycle path with no test that triggers it.

### Sentinel-Semantics Reuse In Mocks
- A mock returning `null`/empty/`[]` that no longer matches what the real function returns in
  the new code, so the test passes against a stale sentinel — the mock's sentinel meant "nothing
  here" but the real code now returns `[]` meaning "empty but valid."
- Block on a mock whose sentinel semantics diverge from the real code's new semantics.

### Mirror-Tests-That-Miss-The-Machine
- Tests that mirror the implementation's structure so closely (copy the same conditional logic
  into the test) that they pass even when both are wrong — they test the code against itself,
  not against the spec.
- A parameterized test whose data table only covers the cases the code already handles, never
  a case the spec requires but the code misses.
- Block on tests that verify the implementation against itself rather than against the spec.

## What NOT To Flag (Anti-Overlap)

- Do NOT flag security vulnerabilities (defer to Security).
- Do NOT flag architecture soundness, data-model correctness, or concurrency (defer to
  Architecture).
- Do NOT flag migration safety, schema drift, or data loss (defer to Data-migration).
- Do NOT flag whether tests exist at all when no test code is in the diff (defer to
  Completeness for "missing verification").
- Do NOT re-report `eval(` or `missing-test` issues the deterministic gates already catch.

## Evidence Rules

Every blocking finding must cite the test code (file:line + the verbatim assertion or test
block that makes the finding true — the "quote-the-line" gate) and state the failure mode (what
the test claims to verify vs. what it actually verifies), not just "this test is weak." If the
diff's stated intent is to change test behavior, judge whether the test change actually verifies
the behavior change or merely silences a failing test.
