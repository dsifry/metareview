# QA shakedown runbook — iteratively escalating a project through metareview

How to take a project (any supported language) through metareview's `sdlc-loop-proved` and escalate from
"does the plumbing work" to "does it find real, proof-backed bugs." Written from the Go (`spf13/cast`)
and Python (`pytest`) shakedowns; the ladder is language-agnostic, the per-language details are in the
last section.

**Scope:** single-language pure projects first (Go, TS/Jest, TS/Vitest, Python/pytest). Mixed-language
projects (one repo with Go + TS + Python) come after each single language passes the full ladder.

## Ground rules (learned the hard way)

- **Throwaway repos only.** The loop makes commits. Clone/scaffold under a temp dir; never a repo you care about.
- **`--base` is effectively mandatory.** Omit it and base defaults to head → empty diff → silent clean
  `DONE`. Always pass `--base <ref>`.
- **The consent gate is expected.** First `fsm init` returns `ERR_CMDS_NOT_ALLOWED` with a `cmds_sha256`;
  re-init with `--allow-custom-cmds <sha>`. That sha is a human decision — you are consenting to the exact
  test command.
- **Recall vs. hidden gold are different measurements.** Recall = "does it find a bug we *planted/know*?"
  (we know the answer). Hidden gold = "does it find a *real, pre-existing* bug we did **not** plant?"
  (unknown — must be independently verified, and the judge is the false-positive guard).
- **Verify behavior ≠ verify a bug.** A behavioral difference can be *intended and tested*. Always check
  the project's own tests before calling something a bug (the cast `ToStringMapStringSlice` false positive
  — the codex adjudicate judge caught it by reading the tests; we had not).

## The escalation ladder

Run these in order. Stop and diagnose at the first rung that breaks — later rungs assume earlier ones.

1. **Wiring smoke.** `init` (consent) → `advance` into `discover` → confirm you get the base..head diff.
   Proves: on-ramp, consent gate, driver loop, the diff the loop reviews. No judge cost yet.
2. **Recall — obvious planted bug.** Plant a blatant defect (e.g. invert a comparison), commit so
   `base(clean)..head(buggy)` shows it. Run the full loop with **real, cold discover reviewers** (no hint).
   Fix it and carry a **proof**. Proves: recall on an easy case + the fix→prove→verify path end to end.
3. **Recall — subtle, latent planted bug.** Plant a one-line off-by-one / edge bug that keeps the suite
   **green** (no failing test to point at), so reviewers must *reason*. Proves: recall without a test crutch.
4. **Recall — real historical bug.** Find a real fix commit `F` in the project's history; set `--base F`
   with the repo checked out at `F^` (the parent, which contains the shipped bug). The diff `F..F^` shows
   the real bug reintroduced. Use `F`'s real regression test as the **reproduction proof**. Proves: recall
   on a genuine defect + the reproduction-form proof + the §9.2 symptom reviewer.
5. **Hidden gold — full review of current, unmodified code.** Review *all* source files (not a diff) with
   cold reviewers; **independently verify** every candidate against the project's tests before believing
   it, and let the codex judge adjudicate. Expected honest outcome on a mature library: few or zero
   confirmed bugs, and the judge rejecting plausible-but-tested false positives. (This is a full/sharded
   review, NOT the diff-based FSM loop.)
6. **Mutation survivors — test-hardening.** Run a mutation pass; each **survivor** (a mutant the project's
   own tests do NOT kill) is a *candidate* coverage gap. Confirm a gap only by writing a killing test that
   **deterministically** kills the mutant (fails on the mutant, passes on restore, re-run independently).
   Equivalent/unreachable mutants survive too and are NOT gaps — never file an unconfirmed survivor.

Proof forms to exercise across the rungs: a **pin** (mutate an added guard line, a test kills it) and a
**reproduction** (a committed test that fails-before / passes-after). Deletion proofs come with a
delete-a-function rung once the above pass.

## Driving the loop (the `--agent-prompt` contract)

`init` → then repeat: `advance` → on **exit 3 (NEEDS_INPUT)** do the node's work → `record node-output
--node <n> --data <file>` → `advance`. Node execs: `subagent` = *you* spawn real review agents (discover);
`inline` = *you* do it in-session (fix); `fork` = the CLI runs it (adjudicate, prove, verify). At `fix`,
the node now **elicits a proof** — return `{commit, summary, pins:[…]}` (the fix-node proof elicitation).
`prove` runs the real test command deterministically; a proven reproduction/pin clears `pins_proven`.

## Toolchains via Docker (no local installs)

Run each language's test runner in a container so nothing is installed on the host and worktrees stay
reproducible. metareview runs the consent-hashed `test` command with `cmd.Dir = <dynamic worktree>` and
appends the convention's flags, so:

- **Do NOT put `$PWD`/`$@` directly in the workflow `cmds.test.argv`** — the YAML parses `$NAME` as a
  metareview variable (`$PWD is not a declared var`). Instead commit a tiny wrapper **script** to the
  project and point `test_cmd` at it (`argv: ["bash", "run-tests.sh"]`); the script uses `$PWD`/`$@`
  freely. The convention's appended flags arrive as the script's `"$@"`.
- The wrapper mounts the worktree and runs the containerized runner, e.g. for pytest:
  `exec docker run --rm -v "$PWD":/w -w /w mrv-pytest python -m pytest -q "$@"`.
- **Set `TMPDIR=/tmp`** for the metareview commands so worktrees land on a Docker-shareable path (macOS
  Docker Desktop does not share the default `/var/folders/...`).
- Build a small image per runner once (runner in the image, project code mounted). Node runners
  (jest/vitest) live in the project's `node_modules`, so either bake them into the image globally or
  ensure `node_modules` is present in the worktree.

## Per-language cheat sheet

| Language | `test_convention` | Runner (in `run-tests.sh`) | Report path | Mutation tool |
| --- | --- | --- | --- | --- |
| Go | `go` (default) | `go test ./... -json` | test2json on stdout | gremlins (`go install`) |
| TS / Jest | `typescript` | `npx jest --json` | Jest `--json` on stdout | Stryker (`@stryker-mutator`) |
| TS / Vitest | `vitest` | `npx vitest run --reporter=json` | Vitest json on stdout | Stryker |
| Python / pytest | `python` | `pytest -o junit_family=xunit1 --junit-xml=/dev/stdout` | JUnit XML on stdout | mutmut (`pip install`) |

Notes: the convention appends the report flags itself (Go `-json`; Jest/Vitest `--json`/`--reporter=json`;
pytest `-o junit_family=xunit1 --junit-xml=/dev/stdout`), so `run-tests.sh` supplies only the base runner
invocation. pytest **must** pin `xunit1` (its ≥6.1 default xunit2 drops the `file` attribute the convention
needs for nodeids). Mutation is Go-first via gremlins (metareview ingests gremlins JSON); Stryker's
cross-language schema covers TS, and mutmut covers Python.

## Workflow variants

`sdlc-loop-proved` ships Go-hardwired (`test_cmd: go test ./...`, no `test_convention`). For another
language, copy it to a path-loaded variant (`fsm init --workflow /path/to/variant.yaml`): give it a
distinct `workflow:` name, set `cmds.test.argv` to the wrapper script, and add `test_convention: <lang>`
to the `prove` node. Everything else (transitions, gates, convergence) is unchanged.

## Validated so far

- **Go** (`spf13/cast`): full ladder — recall on obvious, subtle-latent, and real-historical bugs (2/2
  cold reviewers each, reproduction proofs); hidden-gold review returned zero confirmed bugs with the
  judge rejecting one tested false positive; a mutation survivor found + closed with a killing test.
- **Python / pytest**: end-to-end via Docker — planted latent bug, codex-confirmed, **reproduction proof
  PROVEN**, `DONE(fixed)`. First non-Go proof that the seam works live.
- **TS / Jest, TS / Vitest**: conventions built + unit-tested (100%, mutation-verified); end-to-end runs
  pending (same Docker pattern as pytest).

## Mixed-language projects (after all singles pass)

One repo containing Go + TS + Python, each with its own test runner. The proved loop is single-runner
(`test_cmd`), so mixed projects need either per-language sub-runs (scope discover/prove to one language's
subtree at a time) or a workflow that selects the convention per finding's file. Run the same ladder;
the new surface is convention selection and per-subtree scoping, not the proof machinery.
