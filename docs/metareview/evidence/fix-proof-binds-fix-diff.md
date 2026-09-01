# Evidence — the proof binds against the fix diff, not base..head

**Task:** the linchpin gap the first live `sdlc-loop-proved` shakedown found — the differential-proof
machinery bound against the reviewed diff (`base..head`), not the fix's own diff (`FixEntryHead..head`),
so a loop fix could pass with the proof silently bypassed. **Base:** `main` (`9291168`). See
[[proof-binds-against-base-not-fix-diff]].

## The gap (found by the shakedown, confirmed in code)

Every node — including `prove` — was handed the `BaseSHA..head` diff (`machine.go` `runNode`;
`machine/types.go`: "Diff is the base..head diff handed to kinds"). But `prove`'s pin added-line bind
(`isAddedLineInFile`) and `owesPin` (`AddedLinesInFile`) conceptually want the **fix's** diff. In the
loop, `base` is the original pre-bug commit and a fix that RESTORES removed code nets out against it, so
`git diff base..fix` is empty for that file and the fix's added line is invisible → the pin is
**malformed** (advisory), `owesPin` sees no added line → owes no proof → `pins_proven` passes on nothing
and the loop reaches `DONE(fixed)` on the LLM verify alone. The first shakedown reproduced exactly this:
pin `malformed`, `proven=0`, `unproven=1`, yet `outcome=fixed`.

## The fix

A kind now DECLARES whether it needs the fix-scoped diff, and the machine honors it:
- `workflow.KindInfo` gains **`FixScopedDiff bool`**.
- `proveKind.Info()` sets it `true`.
- `machine.runNode` computes the diff base per kind: `FixScopedDiff && FixEntryHead != ""` →
  `FixEntryHead..head` (the fix's own diff); otherwise `base..head` (unchanged for review kinds). Falls
  back to `base..head` when `FixEntryHead` is unset. `FixEntryHead` is the pre-fix anchor, so
  `FixEntryHead..head` is exactly what the fix changed — so the pin bind and `owesPin` now see the
  fix's added lines regardless of how they relate to the original base.

Declarative and generic: the machine keys on `Info().FixScopedDiff`, not on the `prove` kind by name
(no import of `kind` into `machine`).

## Verification — unit AND the re-run shakedown (the fix's whole point)

- `internal/fsm/machine`, `internal/fsm/workflow`, `internal/fsm/kind` (and all `internal/fsm/*` +
  `workflows`) remain **100.0%**. New `TestRunNodeFixScopedDiffUsesFixEntryHead`: a fix-scoped kind
  receives `FixEntryHead..head` while a review kind receives `base..head`. **Mutation-verified** (always
  using `base` reddens the new test). `gofmt`/`go vet`/golangci-lint clean; full `go test ./...` green.
- **Re-ran the exact shakedown** (constructed `Clamp` repo, `sdlc-loop-proved`, real codex judge) with
  the fixed binary. The same fix + pin that came back **malformed** before now comes back **proven**:
  `outcome=proven`, with the real `go test` output *"--- FAIL: TestClampHigh … Clamp(250) = 250, want
  100"* (mutation killed) then restore passing. **Zero** prove findings. Final: `DONE(fixed)`,
  `proven=1, unproven=0` (the pin is now in the §9.6 killed-mutant set). The deterministic proof
  actually gated the outcome — no longer riding on the LLM verify.

## Shepherding round 1

- **Cursor Bugbot (Medium): "Prove retries lose the fix-scoped diff."** Correct observation, but it is
  the *designed* fork-safety behavior, not a regression: a fork into a post-fix state clears
  `FixEntryHead` on purpose so `commit_exists` fails closed until the fix baseline is re-established
  (fork_test.go:466). The productive retry for a failed proof is a fork back into the FIX node, which
  re-baselines `FixEntryHead` via `FixBaselineData` — and then the fix-scoped diff works. Re-running
  `prove` on the same commit (a fork `--from prove`) cannot change a pin outcome anyway. Documented the
  fallback explicitly at the branch so it is not mistaken for an oversight; no code change to the path.
- **CodeRabbit (Minor/Trivial):** updated the stale "Diff is the base..head diff" contract comment in
  `types.go` (it is now per-kind); added a `proveKind.Info().FixScopedDiff` assertion (catches the flag
  being dropped, which the generic machine test cannot); fixed an MD022 blank-line nit in this doc.

## Left as-is (deliberately, pending the retry we just did)

The malformed-pin-is-advisory semantics are unchanged: with the binding fixed, `owesPin` now correctly
sees the fix's added line and would demand a proof (blocking) for a fix that adds a line in the
finding's own file, so a real proofless fix blocks via the owed-pin marker — no need to flip
malformed→blocking. (Confirmed the shakedown now proves rather than passing vacuously.)
