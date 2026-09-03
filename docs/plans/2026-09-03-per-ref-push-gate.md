# Spec: per-ref pre-push gating (fail-closed MVP) — issue #82

**Date**: 2026-09-03 · **Branch**: `per-ref-push-gate` · **Issue**: #82 · **Option**: B (fail-closed MVP,
maintainer-selected 2026-09-03)

## 1. The gap (reproduced)

The pre-push hook drains git's ref lines from stdin and always runs `metareview review gate --push`, which
measures the **checked-out branch** (`ResolveBranchScope` → `gitcontext` resolves HEAD from the working tree).
A push that carries a **different** ref is therefore judged against the checked-out branch, not its own content:

```
# checked out: main (clean/reviewed). evil holds unreviewed backdoor.go.
git push origin evil        # BLOCKED  — gate sees evil's unreviewed files
git push origin evil:main   # ALLOWED  — gate sees clean main; remote main now has the backdoor
```

The hook fires in both cases; in the second it exits 0 with no `--no-verify`. This is a *silent* bypass of a
gate whose whole point is bypass-resistance. (Full transcript: reproduction repo, this session.)

## 2. Scope of this change (Option B)

Close the *silent* bypass with the smallest fail-closed change; do **not** attempt to review a non-checked-out
ref's own content (that is Option A, a possible follow-up).

- The common path is unchanged: `git push`, `git push origin HEAD`, `git push origin main` while on `main` —
  every push whose pushed sha **equals** the checked-out `HEAD` — gates exactly as today.
- A pushed **non-deletion** ref whose local sha **differs** from `HEAD` is **BLOCKED** (fail closed), naming
  the ref, with a message telling the operator to check the ref out and push it (so the gate reviews its own
  content) or push with `--no-verify` to override deliberately.
- A ref **deletion** (all-zero local sha) is skipped (no content to review), exactly as today.
- A **tag** push (remote ref under `refs/tags/`) is skipped: a tag is a pointer to commits already gated on
  their branch, and an annotated tag's object sha never equals HEAD, so gating it would falsely block a legit
  `git push origin v1.0` (added after review — #82 acceptance lists tags among operations not to falsely block).

Accepted consequence: `git push --all` / `git push origin a b` that sends several branches at once blocks on
every branch that is not the checked-out one, even if reviewed. That is coherent with the gate's stance ("to
push a ref through the gate, check it out") and the message tells the operator what to do. Documented, not a bug.

## 3. Where the decision lives — Go, not bash

The per-ref judgement goes in `internal/status` (unit- and mutation-testable via the injected `RunGit` seam),
**not** in the hook. New bash logic is outside the gremlins net; the repo convention is "judgement in the
tested CLI, the hook is a thin shim." So:

### 3.1 `internal/status` — new `PushGateForRefs`

```go
// PushGateForRefs applies the pre-push gate to the refs git streams on stdin (one line each:
// "<local-ref> <local-sha> <remote-ref> <remote-sha>"). It closes issue #82's silent bypass:
//   - a deletion (all-zero local sha) is skipped;
//   - if every non-deletion pushed ref's local sha == the checked-out HEAD, it delegates to PushGate
//     (the pushed content IS the checked-out branch — gate it exactly as before);
//   - otherwise it BLOCKS (fail closed), naming each ref whose sha != HEAD, because the gate measures the
//     checked-out branch and cannot verify a different ref's content.
// stdin may be empty (nothing to push, or a manual invocation) → not blocked.
func PushGateForRefs(root, base, stdin string, run RunGit) (blocked bool, message string, err error)
```

- Parse: split stdin into lines, skip blank lines, split each into 4 fields (`local-ref local-sha
  remote-ref remote-sha`). A line with fewer than 2 fields is malformed → **fail closed** (block) rather than
  silently ignore.
- Deletion: `local-sha` all-zero (`^0+$`) → skip.
- If no non-deletion refs remain → return `false` (allow).
- Resolve `HEAD` via `run(root, "rev-parse", "HEAD")` (trim). If it errors or is empty → **fail closed**.
- Collect every non-deletion ref whose `local-sha != HEAD`. If any → block with a message naming each
  (`<remote-ref> ← <local-ref> @ <sha12>`), the checked-out HEAD sha, and the check-out-or-`--no-verify`
  remedy.
- If all non-deletion refs == HEAD → `return PushGate(root, base, run)` (unchanged behaviour).

Sha comparison is exact string match after trimming; git streams full 40/64-hex shas on both sides and
`rev-parse HEAD` returns the full sha, so no abbreviation ambiguity.

### 3.2 `cmd/metareview` — `--pre-push-stdin`

`review gate --push` gains an opt-in flag `--pre-push-stdin`. Only the hook passes it. When set, the CLI reads
all of stdin and calls `status.PushGateForRefs(root, base, stdin, nil)`. Without it, `--push` calls
`status.PushGate` exactly as today (a human typing `review gate --push` in a terminal is never affected, and
never blocks on a tty read). Blocked → print message to stderr, exit 1; else exit 0 (unchanged).

### 3.3 `hooks/git/pre-push` — thin forward

The hook already must drain stdin (broken-pipe safety). Change: **capture** it (`STDIN="$(cat)"`) instead of
merely draining. **Keep** the bash `have_content` deletion shortcut — if every pushed ref is a deletion, exit
0 without consulting the binary (a `git push --delete` of a merged branch must succeed even when metareview is
not installed; moving deletion-skip wholesale into the CLI would newly require the binary for a deletion — a
regression). Otherwise forward the captured stdin to the gate with the new flag
(`"$BIN" review gate --push --pre-push-stdin $BASEARG <<<"$STDIN"`). The CLI still skips deletion lines when
computing HEAD-match (a mixed deletion + content push), so deletion-skip is covered on both sides; the bash
shortcut is only the binary-independence fast path for the all-deletions case. Binary-not-found still fails
closed **after** stdin is drained. Exit-code handling (0 allow / 1 block / other → block) is unchanged. Update
the header comment: the checked-out-branch silent bypass is now closed (a non-HEAD ref blocks).

Mirror the identical edit into the embedded copy (`githookassets.go` is `go:embed hooks/git/*`, so editing the
file updates the embed) — verify the embed and the materialized install carry the new hook.

## 4. Tests (TDD — write first)

**`internal/status` (unit, mutation-checked):**
- deletion-only stdin → not blocked, gate not consulted.
- all non-deletion shas == HEAD → delegates to PushGate (blocked iff the branch is unreviewed) — inject a
  `RunGit` returning a known HEAD + a scope with/without blockers.
- one non-deletion sha != HEAD → blocked, message names that ref and the HEAD sha; the matching ref alone does
  not rescue it.
- mixed: a deletion + a non-HEAD ref → blocked on the non-HEAD ref (deletion ignored, not a rescue).
- `rev-parse HEAD` errors / empty → fail closed (blocked).
- malformed line (1 field) → fail closed.
- empty stdin → not blocked.

**`internal/githooktest` (black-box hook):**
- keep block-on-nonzero / allow-on-zero / fail-closed-on-missing-binary (fakeBin, unaffected by the new flag).
- keep the existing pure-deletion test — the bash shortcut still exits 0 without the binary (unchanged).
- new: a fakeBin that records its stdin proves the hook **forwards** git's ref lines to the gate (so the CLI
  can see them). The per-ref HEAD-match logic itself is a Go unit test on `PushGateForRefs`.

**Coverage/mutation:** `bash tests/coverage.sh` green; `gremlins unleash --workers 1 --timeout-coefficient 30
./internal/status/` clean on the new logic (a survivor = a blind test — add the distinguishing case).

## 5. Acceptance (from #82)

- [x] Pushing an unreviewed non-checked-out ref/range is blocked (fail closed), naming the ref. → §3.1
- [x] The common `git push` of the current branch behaves exactly as today. → sha == HEAD delegates to PushGate.
- [x] A ref deletion is not falsely blocked. → deletion skip.
- [x] Tags are not falsely blocked. → `refs/tags/*` skip (added after review).
- [ ] "Pushing a reviewed *cross-ref* range passes without `--no-verify`" — **deliberately not met** under
  Option B (blocked with a check-out-or-`--no-verify` remedy). Option A is the follow-up if that workflow is real.

## 6. Out of scope / follow-ups

- Option A (resolve each pushed ref at its own sha and read its review coverage as-of that sha) — only if
  cross-ref reviewed pushes prove to be a real workflow.
- Range gating (`--range base..head`) — not needed for Option B.
