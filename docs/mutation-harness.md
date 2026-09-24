# Change-driven mutation testing (StrykerJS)

metareview ships a harness template that runs StrykerJS only for what a change can affect, and records
exactly what it verified in an attested state. The gate then judges whether mutation evidence is still
fresh. Validity depends only on what changed (content digests), never on dates.

Status in 0.13.0: harness plus freshness gate. The external trial's pass bar — a one-line edit's
incremental run at ≤ 2 min p50 — is still pending (see the checklist at the end).

Design and rationale: `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md`.

## 1. What it does

- `run --mode incremental` plans from the changes since the last attested state and runs Stryker at
  most twice:
  - an unforced invocation over the files whose kills may have changed (including edited files);
  - a `--force` invocation over edited files and the mutants their covering tests could affect.
- Anything it cannot re-verify within its budget or time limit is recorded as a **deferral**. A
  deferral means a full run is pending; the state carries it until a full run or a clean catch-up
  clears it.
- CI keeps the state on two git branches (`mutation-state/inc`, `mutation-state/full`), with
  `actions/cache` as a fast path. A full run happens only when something was deferred. There are no
  scheduled jobs.

## 2. Set up

1. Copy the template into the repository, without its `test/` directory:

   ```bash
   mkdir -p tools/mutation-incremental
   cp -R <metareview>/templates/mutation-incremental/cli.mjs <metareview>/templates/mutation-incremental/lib tools/mutation-incremental/
   cp <metareview>/templates/mutation-incremental/mutation-incremental.example.json mutation-incremental.json
   cp <metareview>/templates/mutation-incremental/github-workflow.yml .github/workflows/mutation.yml
   ```

2. Configure Stryker (JSON config, StrykerJS 10.x, installed in `node_modules`):
   - `"testRunner": "vitest"`;
   - `"disableBail": true`, so `killedBy` lists every killing test and does not depend on test order;
   - a `mutate` array equal to the harness config's `mutate`;
   - `ignorePatterns` containing the state dir (`".mutation"`);
   - no `"inPlace": true`.
3. Gitignore the state dir (`.mutation/`). Pin Node with a tracked `.nvmrc` that both
   `setup-node` and developers use. The file is a global input, so a laptop with a different Node
   never pollutes CI state.
4. Edit the workflow's project-setup steps (Node, `npm ci`, services). Edit the `push` branch if the
   default branch is not `main`. Optionally add `paths-ignore` mirroring `ignore`, but keep the
   `full` job's `needs` chain intact.
5. Make **both** `incremental-pr` and `pr-full` required checks. A skipped job satisfies a required
   check.
6. Repository rulesets that restrict updates or force-pushes on all branches must allow both on
   `mutation-state/*`. Limit the bypass to GitHub Actions.
7. The workflow sets `MUTATION_STATE_TOKEN: ${{ github.token }}` on the `fetch-state` and
   `publish-state` steps only. Keep `github.token` rather than a personal access token: a PAT's
   pushes trigger other workflows. Local runs need no token.

## 3. Configuration (`mutation-incremental.json`)

All keys below are required unless marked optional. The values shown are the defaults of the
example file.

| Key | Meaning |
|---|---|
| `schemaVersion` | `1` |
| `stateDir` | `.mutation` — a dedicated directory inside the repo, not under the OS temp dir, not matching `mutate`/`test`/`support` |
| `stryker` | `command` (argv, no shell), `configFile` (JSON), `extraArgs` |
| `mutate`, `test`, `support`, `global`, `ignore` | glob lists (below) |
| `aliases` | specifier prefix → repo path prefix, e.g. `{"@app/": "src/"}` |
| `runtime` | `commands` (argv whose stdout digest is an input) and `env` (variable names, stored as sha256 digests only) |
| `budget` | `maxForcedShare` (0.25), `maxForcedMutants` (null), `maxMinutesPerInvocation` (20) |
| `residual.mode` | `bounded` (default; the budget applies), `strict` (no budget), `off` (records `residual off` deferrals instead of forcing) |
| `editedFiles` (optional) | `residual` (default: an edited file goes whole into the unforced scope; its changed lines and its coverage closure are forced) or `whole` (the edited file is forced whole) |
| `pendingOnPr` (optional) | `allow` (default), `full` or `full-on-global` (§6) |
| `allowBail` (optional) | allow a Stryker config without `disableBail` (refused with `editedFiles: "residual"`) |
| `verify` (optional) | `{"command": [argv], "timeoutMinutes": n or null}` — the project's own bar (§8); `timeoutMinutes` is required when `pendingOnPr` is not `allow` |
| `views` (optional) | `{"command": [argv]}` printing `{"name": [patterns]}`, or `{"inline": {...}}` (§8) |

Sizing:

- `maxMinutesPerInvocation` is a safety ceiling, not a target. Size the budget and the ceiling so a
  typical one-line change stays well under the ≤ 2 min bar.
- Two invocations plus setup must fit the CI job timeout. Setup, the catch-up and the full run must
  fit the `full` job's timeout together.
- GitHub-hosted runners cap a job at 360 minutes. Above roughly 13 minutes of verifier time per view,
  use a single-pass verifier (§8).

## 4. What goes where

- **Categories.** The first list that matches, in the order `global`, `support`, `test`, `mutate`,
  decides a path's category. Paths matching `ignore` are excluded. Anything else is `unclassified`.
  The config file and `tools/mutation-incremental/**` are always `global`.
- **What each category costs.**
  - A `global` change defers everything to a full run.
  - A `support` change forces the kills of its importer tests.
  - An `unclassified` change forces the kills of its importer tests, or defers everything when
    nothing imports it.
  - Add docs, CI config and editor files to `ignore`.
- **Meta-artifacts.**
  - Files that tests import or read belong in `support`; tooling-only JSON and docs in `ignore`.
  - `tests/**` is `test`; helpers and factories are `support`.
  - `global` is only for inputs everything depends on: lockfile, Stryker/Vitest/TS config,
    migrations, `.nvmrc`.
  - Keep metareview's own files (`.metareview/**`, `docs/metareview/**`) in `ignore`, as the
    example config does. Otherwise a review log committed after the harness ran counts as a new
    unclassified path, and the gate reads every kill as stale.
- **Aliases and open importers.** A relative or alias specifier that reaches no file in the
  repository, and is not an installed package or a Node builtin, makes its file an **open
  importer**. Every test reaching an open importer is treated as reaching everything. `plan` lists
  open importers and their specifiers, so you can add `aliases`.
  - A gitignored generated module imported by a relative path makes its importers open importers.
  - Files read by path at runtime, and computed `import(x)`, are invisible to the import graph.
- **Barrels and constants.** Editing a widely imported barrel or constants file often exceeds the
  budget and defers to a full run. Prefer `import type` (or `verbatimModuleSyntax`) for type-only
  imports.
- **Paths Stryker cannot take.** A path containing `, { } [ ] ( ) ! * ? :` cannot be passed to
  `--mutate`, and `plan`/`run` exit 2 naming it (e.g. Next.js `app/(auth)/[id]/page.tsx`). Keep such
  directories out of `mutate`.

## 5. Commands, exit codes and outputs

```bash
node tools/mutation-incremental/cli.mjs plan [--also-state <dir>]...
node tools/mutation-incremental/cli.mjs run --mode incremental|full [--also-state <dir>]... [--pr [--max-minutes <m>]]
node tools/mutation-incremental/cli.mjs seed --from <report>... [--replace]
node tools/mutation-incremental/cli.mjs fetch-state [--remote <name>]
node tools/mutation-incremental/cli.mjs publish-state --kind inc|full [--remote <name>]
node tools/mutation-incremental/cli.mjs break-lock
node tools/mutation-incremental/cli.mjs summary --job pr|pr-full|main|full [--exit-code <n>] [--pr]
```

- `plan` prints the plan as JSON and changes nothing.
- `publish-state` is CI-only. It publishes only a usable state; otherwise it says why and exits 0.
- `summary` writes the step summary.

Every command runs from the git top-level, and every command prints one summary line on stderr.

Exit codes:

| Code | Meaning |
|---|---|
| 0 | ok (deferrals included) |
| 1 | ok, but the score is below `thresholds.break` or the verifier failed (the state is committed) |
| 2 | config, usage or validation error |
| 3 | lock held (`break-lock` clears a lock left by a crashed run on another machine) |
| 4 | engine failure — nothing committed; re-running retries the same plan |
| 130 | interrupted — nothing committed |

When both apply, the higher-priority code wins: 2 > 3 > 130 > 4 > 1 > 0.

With `GITHUB_OUTPUT` set, `run` appends:

- on every exit path: `pending_full` and `exit_code`;
- on exit 0 or 1, also:
  - `pending_cause`: `global`, `unreachable`, `other`, `timeout` or `none`;
  - `pending_causes`: single-line JSON of the counted deferrals, at most 50 entries, then
    `{"more": n}`;
  - `route`: `verdict`, `sweep` or `fail` (§6).

## 6. How CI works

**Jobs.**

| Job | When | What it does |
|---|---|---|
| `incremental-pr` | every PR push | Fetches main's state, then runs incremental with `--pr` over its own cached state and main's. Saves its cache, writes the summary, and decides by exit code and route. |
| `pr-full` | after `incremental-pr`, only when the route is `sweep` | Runs `run --mode full` (the sweep); red unless it exits 0. Never has a token, never publishes. |
| `incremental-main` | every push to main | Same as the PR job without `--pr`; publishes `mutation-state/inc`. |
| `full` | after `incremental-main`, when `pending_full` is true | First a **catch-up** incremental run from the freshest published state. If that clears pending, it publishes `mutation-state/full` and stops; otherwise it runs a full run and publishes it. |

**State branches.**

- `mutation-state/inc` is main's latest incremental state.
- `mutation-state/full` is "main's latest state without deferrals". It may carry
  `mode: incremental` with an older `lastFullAt`.
- Each branch is a single parentless commit, force-pushed, and has one writer, so nothing expires.
- The cache is a fast path. A miss falls back to the branches, never to a cold start.

**Pending on PRs** (`pendingOnPr`). What happens when a PR run ends with a counted deferral:

| Mode | Cause `global` | `timeout` | `unreachable` | `other` |
|---|---|---|---|---|
| `allow` | green, pending | green, pending | green, pending | green, pending |
| `full` | sweep | sweep | sweep | sweep |
| `full-on-global` | sweep | sweep | **PR fails** | sweep |

- A deferral is **inherited**, not counted, only when main's published state carries the identical
  deferral about inputs the PR has not changed. Inherited deferrals never route or fail anything.
  The summary lists them as "inherited from main, cleared by main's full run".
- Under `full-on-global`, a PR's verdict never depends on runner speed within the harness's own
  limits: a timeout only makes the PR slower (it pays the sweep). The remaining speed-bounded reds
  are the verifier's timeout, setup, and the `pr-full` job timeout. An overrun is red and
  re-runnable, never silently green.

**Sizing under `pendingOnPr: "full"` or `"full-on-global"`.**

- The workflow variable `MUTATION_PR_MAX_MINUTES` (`<m>`, default 60) replaces
  `maxMinutesPerInvocation` on routed PR runs. Size it so a timeout is rare. A timeout costs a sweep,
  not a failure.
- `incremental-pr`'s `timeout-minutes` must be at least:

  setup + 2 × (`<m>` + 1) + max(1, views) × (`verify.timeoutMinutes` + 0.5)

  This keeps a job-level timeout from pre-empting the harness and turning a PR red because of runner
  speed. The template's 150 fits the defaults (60 minutes, one view, no verifier); raise it as views
  and verifier time grow.
- `pr-full`'s `timeout-minutes` must be at least setup + one full run + max(1, views) ×
  `verify.timeoutMinutes`. Size it like main's `full` job. An overrun is red and re-runnable.
- Under `full`, every PR that exceeds `<m>` pays a sweep.
- A PR that keeps timing out pays a sweep on every push until `<m>` is raised.
- A force-push during a running sweep lets that sweep finish (its state is reused by the next push)
  and queues one more sweep if the new commit also needs one.
- A PR rebased (or not rebased) during main's pending window may pay a sweep when its lockfile digest
  differs from main's. That is correct, just slower.
- A push whose `incremental-pr` finishes before a running sweep saves its cache re-plans from the
  older state, and may pay one more sweep.
- Watch main's `full` job: while it is red, PRs stay green on inherited pending.

**What things cost.**

- A change to a global input costs one full run on main. That includes a dependency bump, and
  tuning-only config edits (`thresholds.break`, `budget`, `ignore`).
- A threshold break turns every PR red until main is fixed. The step summary shows main's score, so
  a PR that is red only because main is below the threshold says so.
- A PR that changes a global input (e.g. a lockfile bump), or carries its own budget or time
  deferral, re-runs its delta from main's state on each push.
- Main re-runs each merged PR's delta: PR caches are per PR.
- Re-running an old main workflow publishes an older, still valid state.
- A cancelled job commits nothing.
- A red `incremental-main` on a global change triggers no full run for that commit; re-run it.
- Repeated red `full` jobs (timeout or exit 4) never clear on their own. A `full` job that hits its
  timeout will not clear by itself either: raise the timeout or narrow `mutate`.
- `incremental-pr` is not a merge-queue check unless `merge_group` is added to the triggers.
- Private repositories without fork Actions run no PR job for forks.

## 7. Local use

```bash
node tools/mutation-incremental/cli.mjs fetch-state
node tools/mutation-incremental/cli.mjs run --mode incremental --also-state .mutation/remote/full --also-state .mutation/remote/inc
```

- A new worktree can warm-start from the first checkout:
  `run --mode incremental --also-state <first checkout>/.mutation`.
- `seed --from <full Stryker report>` bootstraps local state. Its kills stay pending until a full
  run. Seeding helps local runs only: CI's first run is still a full run.
- A local `run --mode full` clears pending kills locally.
- A local run never turns a PR green: no local state is imported into CI. CI's PR state is not gate
  evidence either, so run the harness locally before `pr-ready`.

## 8. Verifier and views

**Verifier.** CI enforces the project's bar, a threshold and/or a verifier; the gate judges only
freshness.

- After `run` commits state, the verifier runs once per view (once without views), without a shell,
  in its own process group, with these variables:
  - `MUTATION_REPORT` and `MUTATION_ATTESTATION` (absolute paths);
  - per view, `MUTATION_VIEW`, `MUTATION_VIEW_PATTERNS` (JSON) and `MUTATION_VIEW_FILES` (JSON);
  - `MUTATION_RUN_KIND`: `pr` on routed PR runs, `other` otherwise.
- Any failure is exit 1, and stderr names the view. That includes a non-zero exit, a command that
  cannot be spawned, and exceeding `timeoutMinutes`.
- The verifier does not run when nothing was committed, and `seed` never runs it.
- Kills covered by a deferral still show their previous status in the report. The attestation marks
  each deferral `inherited`, and each view summary splits pending kills into `pendingCounted` and
  `pendingInherited`.
- A verifier that enforces a survivor bar treats **counted** pending kills as unverified on PR runs
  (`MUTATION_RUN_KIND=pr`) and ignores inherited ones. On other runs, pending never fails it.
- Identify waivers by `(file, mutatorName, location, replacement)` plus the file's digest; mutant
  ids are only meaningful inside one report.
- When per-view invocations are costly, write a single-pass verifier that loops over the views in
  the attestation.

**Views.** Views are named, possibly overlapping pattern sets over `mutate`.

- They are read-time only: they never change planning or validity, and a view-map change invalidates
  no kill.
- `run` and `plan` exit 2 before any invocation when:
  - a view matches a non-`mutate` file;
  - a name is empty or duplicated;
  - a `mutate` file is in no view. Assign a new file to a view before it has mutants.
- Views may overlap; per-view counts are never summed.

## 9. Security and privacy

- The state files, caches, job summaries and logs embed sources and test output. On public
  repositories anyone can read them, including fork PRs. Never put secrets in them:
  - tests must not print environment or config values;
  - `runtime.env` stores digests only, but never list secrets there;
  - verifier output lands in public logs and summaries.
- The state branches are as trusted as the repository's write collaborators.
- The token reaches git only as an HTTP header, through `GIT_CONFIG_*` (never argv), and is masked
  first under GitHub Actions.

## 10. Upgrading and cutover

1. Adopt the harness with one state and your views.
2. Keep `pendingOnPr: "allow"` until `mutation-state/full` exists. Before then every PR run is cold,
   and under `full` or `full-on-global` would pay a full sweep.
3. The first push to main runs the full sweep and publishes `mutation-state/full`.
4. Until then, existing reports show as `unattested` (advisory).
5. Run the gate once with the reports and no `--mutation-view`, to clear earlier unviewed findings.
6. Enforce, and switch `pendingOnPr` if wanted, only after `mutation-state/full` exists.

Upgrading the harness means re-copying the template. The upgrade PR runs cold and green-pending,
and main pays one full run (`stateVersion` changes make older states cold).

## 11. The gate

- Pass `<stateDir>/incremental.json` — not Stryker's `reports/mutation/mutation.json` — and every
  report on gated runs. The gate reads `attestation.json` beside each report.
- **What the gate checks.** For every kill in an attested report, it re-derives from content
  digests whether the kill still describes the code under review:
  - **verified** — nothing it depends on changed;
  - **stale** — its file changed, a test that killed it changed, a changed file's covering tests
    killed it, or a support, global or unclassified input changed; the first cause is recorded.
    A changed `mutate` file whose edit touches no mutant (a comment, an import, a new function
    before it has mutants) counts like a global input and stales every kill until the harness
    re-runs. So does a changed `mutate` file with a static (module-level) mutant, which Stryker
    reports with no covering tests. This is conservative on purpose: the gate has no import graph;
  - **pending** — its file is covered by a deferral. It is "inherited" when only deferrals
    carried from main's state cover it, and "counted" when the run's own changes caused it;
  - **unbound** — its file is not attested, or no test is recorded as killing it;
  - **unattested** — the report has no valid attestation.
- **What it reads.** task-done and epic-ready read the working tree. pr-ready reads HEAD (unless
  `--include-working-tree`), so commit before re-running the harness.
- **Modes.** Set `METAREVIEW_MUTATION_FRESHNESS=advisory|enforce`; the default is `advisory`, and
  any other value is a usage error (exit 2). task-done is always advisory.
  - Under `enforce` (pr-ready, epic-ready), stale and unattested Stryker findings block.
  - Pending never blocks: deferred work is not evidence, and full runs never block PRs.
- **The review log** gains a "Mutation Evidence Freshness" section: mode, reports, kill counts by
  class, deferral reasons, and a re-run list of (file, cause) rows.
- **Clearing.** A later run that supplies fresh reports of the same engine supersedes stale,
  pending and unattested findings it no longer produces. A run without reports leaves them alone.
- **Escalation.** A chain blocked only by stale evidence waits for a refresh for up to
  2 × `maxAttempts` before it escalates ("stale mutation evidence not refreshed after N attempts").
- **Views.** Pass `--mutation-view <name>` (repeatable) to judge the evidence per view.
  - Each view gets its own counts and findings. The finding's title names the view, as in
    `Mutation evidence stale (core): src/a.ts changed`, and its fingerprint carries it.
  - The section gains a table with a row per view: verified, stale, pending counted, pending
    inherited, unbound and unattested. The re-run list gains a View column.
  - Views are never summed.
  - A report without a view map is not scoped: every kill counts under every view. This covers an
    unattested report, and one attested before views were adopted.
  - These are usage errors (exit 2): an invalid name, `--mutation-view` without `--mutation-report`,
    and a name missing from an attested report's map.
  - A run with views owns only its views' findings. A run without views owns them all.
  - When the reports' maps no longer name a view (it was renamed or removed), that view's open
    findings are superseded.
- An enforced stale finding clears only on a later run that supplies reports, or by override.
  Overrides on pending or unattested findings last only for that report. Every mutation override is
  tied to the mode in its fingerprint, and an override on one view does not cover the same cause
  under another view.
- Default enforcement is planned for 0.14.0.

## 12. Known residuals

- Mutant-free code inside a changed module that also has mutants (a constant next to functions) can
  affect tests outside its mutants' coverage. The residual closure reads that coverage, so it misses
  tests that reach only the constant. A changed module with no mutants at all uses its importer
  tests instead.
- A new module with no test yet produces no mutants until a test reaches it.
- Deleting a module together with its own test, with nothing else changed, leaves its old survivors
  in the report until the next run that invokes Stryker.
- Computed specifiers and files read by path are invisible to the import graph.
- State outside the repository, beyond the declared `runtime` inputs, is not tracked.
- The gate trusts the harness's attestation (accident-level, not tamper-proof).
- Stryker semantics are version-specific: the harness requires StrykerJS 10.x with the Vitest
  runner. Yarn Plug'n'Play is unsupported.
- `residual` edited-file handling reaches same-file kills only through the changed hunks' covering
  tests. Full runs backstop it.

## 13. External-trial checklist

- Measure a one-line edit's incremental run: the pass bar is ≤ 2 min p50.
- Measure the share of PR runs that end pending. Under `full-on-global`, measure the share that pay a
  sweep; the bar is ≤ 20%.
- Confirm on real GitHub Actions that `full` runs after a red `incremental-main` (outputs of a failed
  job).
- Confirm that the URL-scoped auth header matches the remote.

## 14. Troubleshooting

- **Exit 3:** another run holds `<stateDir>/lock`. A lock left by a crashed run on the same machine
  is replaced automatically; `break-lock` removes one left on another machine.
- **"no usable state":** there is no attestation, or it is from another `stateVersion`. Incremental
  runs execute nothing until a full run (or `seed`/`fetch-state`) provides a state.
- **Read the step summary.** It shows the exit code, the pending causes with their paths, inherited
  deferrals, the score against the threshold, and (on PRs) main's score and when
  `mutation-state/full` last completed.
- `MUTATION_ALLOW_TMP_STATE` and `MUTATION_TEST_DISABLE_RESIDUAL` exist for the harness's own tests
  only.
