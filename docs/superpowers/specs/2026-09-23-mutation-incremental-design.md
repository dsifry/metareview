# Change-driven mutation testing + attested evidence freshness — design

Status: r6 — verification fixes applied; proceeding to the implementation plan per the user's
decision (the plan gets its own review gate). Review history: full artifact reviews r1
`mrv-20260923-182025208234000-…`, r2 `mrv-20260923-183942601569000-…`, r3
`mrv-20260923-185110177502000-…`; targeted review r4 `mrv-20260923-190310858720000-…`; a pragmatic
verification pass on r5 (real workflows, assume trust — user instruction) confirmed the engine and
CI claims and found two real-workflow bugs, fixed here, plus simplifications applied here. The user
decisions are in §10. Origin: `docs/0.13.0-candidates.md` §1.

Review stance for everything that follows (user instruction): handle real workflows and real edge
cases; do not engineer for rare races or synthetic cases; assume developers, CI and agents act in
good faith.

## 1. Problem and engine facts

On the Keeper MVP epic (TypeScript, StrykerJS 10.0.0, Vitest 4.1, real PostgreSQL; 15,660 mutants,
1,135 tests) every one-line edit forced a ~2.5–3.5 h mutation regeneration: the project harness
binds reports to whole-tree manifests and re-runs Stryker from scratch without `--incremental`.
Scoping each run to the change (rather than running `--incremental` over the whole `mutate` set)
matters because the dry run executes every test related to the mutated files (F4); on an
integration-heavy suite that is most of the suite. The harness restores the guarantees that scoping
gives up (F2).

Facts verified in StrykerJS 10.0.0 (`@stryker-mutator/core/dist/src`), its instrumenter, Vitest
4.1.11, GitHub Actions and git (the r5 verification re-ran F4, F5, F9, F10 on a real project):

- F1. `--incremental` reuses per-mutant results: a Killed mutant is reused when its killing test is
  unchanged; a non-killed one when no covering test changed. `--force` re-runs every in-scope
  mutant. Mutants of files that no longer exist are dropped.
- F2. Out-of-scope mutants of files that still exist are carried forward without re-evaluation,
  regardless of `--force`; a changed test drops out of their `killedBy`; mutants in changed regions
  of out-of-scope files are removed.
- F3. The report embeds each file's source as Stryker first read it in that run.
- F4. The dry run always happens. The Vitest runner (`related: true` default) runs only tests whose
  transformed dependency graph reaches a mutated file (type-only imports elided); zero tests ⇒
  `No tests were executed` on stdout and stderr, exit 1, nothing written. The Vitest runner always
  uses per-test coverage, so `killedBy`/`coveredBy` are populated.
- F5. `--mutate` accepts `<file>` and `<file>:<startLine>-<endLine>` (1-based), comma-separated,
  combinable with `--force`, and replaces the config's `mutate`. Stryker's matcher is minimatch with
  `dot: false`.
- F6. The incremental file (`--incrementalFile`) is read and written in place. On
  SIGINT/SIGTERM/SIGHUP/SIGABRT Stryker writes partial results and exits 128+n. Thrown errors exit 1
  without writing. A threshold break exits 1 after writing, computed over the whole report.
- F7. Mutant ids are unique only within one report. The report has no timestamp.
- F8. Without a positional config file Stryker searches its own list of names.
- F9. The report's `files` contains only files that produced at least one mutant; a selected
  zero-mutant file (type-only, barrel, constants) is absent, and a run whose selection has no mutants
  but reachable tests succeeds with `files: {}`.
- F10. `actions/cache` includes the exact `path` list in each entry's version: a restore only hits
  entries saved with the identical list.

metareview today ingests `--mutation-report` files and accepts a stale report silently.

## 2. Goals

- G1 — Per-change cost bounded by the change: one related-tests dry run plus the mutants the change
  can affect. Proven locally by deterministic counts (§7); the external trial's pass bar is a
  one-line edit ≤ 2 min p50 (decision 9), tracked in `docs/0.13.0-candidates.md`.
- G2 — Every change is re-verified in the same run or recorded as **deferred**, visibly; kills that
  depend on deferred work are never reported as verified. Planning is clock-free; the only
  time-dependent outcome is the per-invocation time budget, which always defers. Documented
  exceptions are in §8.
- G3 — The gate states exactly what it verified (accident-level check; not tamper proof).
- G4 — No scheduled job. A full run happens only when something was deferred.

## 3. Non-goals

- Changing any mutation engine or editing Stryker's report format.
- Test runners other than StrykerJS 10.x with its Vitest runner in the template.
- Pin (`prove`) verification speed; the `prove` non-Go pin bug is a separate ticket.
- Keeper adoption and migration: a separate follow-up (decision 7).
- Checkpoint/resume of long runs (candidates §7).
- Requiring mutation evidence: a gate given no `--mutation-report` behaves exactly as today.
- The severity of metareview's existing survivor findings (medium ⇒ counted as warnings by
  `classForCount`): a separate follow-up.
- Tamper resistance, rare races, and adversarial inputs (review stance above).

## 4. Terms

- **Kill**: a mutant whose metareview status is `killed` (Stryker `Killed`); `Timeout` is
  `unresolved` and never a kill.
- **Categories** (§5.3): `mutate`, `test`, `support`, `global`, `unclassified`. The config file and
  `tools/mutation-incremental/**` are always `global`.
- **Excluded paths**: gitlinks; `<stateDir>/**`; `.mutation-full/**`; Stryker's `tempDirName`
  (default `.stryker-tmp`) `/**`; Stryker's `jsonReporter.fileName` and `htmlReporter.fileName`; and
  paths whose category resolves to ignore (§5.3). The attestation records the resolved exclusions
  and the category lists so the gate applies the same rules.
- **Deferral**: `{reason, paths}`, `paths` being files or `["*"]`. A kill is covered by a deferral if
  its file is in `paths`.
- **Pending full run**: state is not usable, or the attestation has deferrals. Cleared only by a
  successful full run.

## 5. Part 1 — Harness template (`templates/mutation-incremental/`)

Zero-dependency Node ≥ 22.8 ESM, copied by a project to `tools/mutation-incremental/`. The repository
owns what runs; metareview owns the attestation contract (§5.5) and the glob dialect (§5.3).

### 5.1 CLI

`node tools/mutation-incremental/cli.mjs <command> [--config <path>]`. The top-level is
`git rev-parse --show-toplevel`; `--config` defaults to `<top>/mutation-incremental.json` and
resolves against the top-level; every command runs with cwd = top-level.

| Command | Behaviour |
|---|---|
| `plan [--also-state <dir>]` | print the plan (§5.4) as JSON; no side effects |
| `run --mode incremental\|full [--also-state <dir>]` | lock, adopt, plan, execute (§5.6), commit |
| `seed --from <report> [--replace]` | bootstrap from a full Stryker report (§5.8) |
| `seed --from-state <dir> [--replace]` | adopt a state produced by this harness (§5.8) |
| `break-lock` | print and remove `<stateDir>/lock` |

Exit codes: 0 ok (deferrals included); 1 ok, committed report breaks Stryker's threshold; 2
config/usage/validation; 3 lock held; 4 engine failure. Precedence: 2 > 3 > 4 > 1 > 0; a run whose
invocations were all deferred exits 0. When `GITHUB_OUTPUT` is set, `run` appends on every exit path
`pending_full=<true|false>` (the canonical state afterwards is a pending full run; `false` if exit 2
happens before the config is readable) and `exit_code=<n>`. Every command prints one stderr line:
`mutation-incremental: mode=<m> invocations=<n> scope=<k> forced=<j> deferrals=<d> pending_full=<bool>`.

### 5.2 Config `mutation-incremental.json`

```json
{
  "schemaVersion": 1,
  "stateDir": ".mutation",
  "stryker": { "command": ["npx", "--no-install", "stryker"], "configFile": "stryker.config.json", "extraArgs": [] },
  "mutate":  ["src/**/*.ts", "!src/**/*.d.ts"],
  "test":    ["tests/**/*.test.ts"],
  "support": ["tests/helpers/**", "tests/factories/**", "tests/fixtures/**"],
  "global":  ["package.json", "package-lock.json", "stryker.config.json", "vitest.config.ts", "tsconfig*.json", "migrations/**"],
  "ignore":  ["docs/**", "**/*.md", ".github/**", "**/.*"],
  "aliases": {},
  "runtime": { "commands": [["node", "--version"]], "env": [] },
  "budget":  { "maxForcedShare": 0.25, "maxForcedMutants": null, "maxMinutesPerInvocation": 20 },
  "residual": { "mode": "bounded" }
}
```

All keys are required; the values above are the shipped defaults (decisions 5, 6). Validation (exit
2 naming the reason): exactly these keys; `schemaVersion` = 1; `aliases` maps specifier prefixes to
repo-relative path prefixes; `residual.mode` ∈ `strict|bounded|off`; `maxForcedShare` in (0, 1];
`maxForcedMutants` positive integer or null; `maxMinutesPerInvocation` positive; `stateDir` inside
the top-level and not under the OS temp dir; `node_modules/@stryker-mutator/core` major version 10;
`stryker.configFile` is JSON with `"testRunner": "vitest"`, no `"inPlace": true`, a `mutate` array
equal to config `mutate`, and `ignorePatterns` containing `<stateDir>`; `runtime.env` names matching
`/(TOKEN|SECRET|PASS|KEY|CREDENTIAL|DSN|URL|URI|AUTH)/i` are rejected (the state directory is
readable by every pull request, §5.7). The docs explain sizing `maxMinutesPerInvocation` (a safety
ceiling, not the target) and the budget against the ≤ 2 min bar.

### 5.3 Glob dialect (shared contract; Node and Go implementations)

Repo-relative POSIX paths, case-sensitive; a leading `./` in a pattern is removed. `*` matches any
run of characters except `/`; `?` one character except `/`; `**` as a whole segment matches zero or
more segments; `{a,b}` alternation (no nesting); no character classes. As in Stryker, `*`, `?` and
`**` do not match a segment starting with `.` unless the pattern segment starts with `.`. A list
matches iff some non-`!` entry matches and no `!` entry matches. Category: the first list that
matches in the order `global`, `support`, `test`, `mutate`; else `ignore` ⇒ excluded; else
`unclassified`. Conformance vectors in `testdata/mutation-incremental/glob-vectors.json` run in both
implementations.

### 5.4 Planner (deterministic)

**Snapshot** `S`: each path from `git ls-files -c -o --exclude-standard --deduplicate -z` that is not
excluded and exists as a regular file or symlink: `sha256:<hex>` of the bytes, or
`symlink:<hex of sha256 of the link text>`. Each entry records category and `tracked`. **Runtime
map**: `cmd:<argv joined by U+001F>` ⇒ sha256 of stdout; `env:<NAME>` ⇒ sha256 of the value or
`unset`.

**Adopt** (`--also-state <dir>`): if `<dir>` holds a usable state whose `lastFullAt` is later than the
primary's (`null` is earliest), or the primary is not usable, plan from it; `run` then copies its two
files into `stateDir` (temp file + rename); `plan` adopts in memory only.

**Usable**: `attestation.json` parses with `schemaVersion` 1 and `tool` =
`metareview-mutation-incremental`, and `incremental.json`'s sha256 equals `reportSha256`. Otherwise
**cold**: incremental mode runs nothing and records deferral `no usable state` on `["*"]`; full mode
runs a full run.

**Change set** `C`: paths whose digest differs from the attestation, new paths, attested paths absent
from `S`, differing runtime keys. A path whose category changed is in `C` under both categories.
`R` is the canonical `incremental.json`.

**Import graph**: parse `.ts .tsx .mts .cts .js .jsx .mjs .cjs` files (≤ 1 MiB) in `S`. Edges from
string specifiers in `import … from`, `export … from`, `import '…'`, `import('…')`, `require('…')`;
not `import type`/`export type`, not `vi.mock`. A specifier **resolves** when it starts with
`./`/`../` (relative to the file) or with a configured `aliases` key (longest key's prefix replaced),
to a regular file in `S`: the path, then + each extension above, then `<path>/index` + each
extension; a `.js/.jsx/.mjs/.cjs` specifier also tries its stem + `.ts .tsx .mts .cts`. A specifier
is a **package** when it is a Node builtin (`node:` prefix or `module.builtinModules`) or resolves
with Node's resolution to a path under a `node_modules/` directory whose realpath is not inside the
top-level. Anything else is **unresolved**, and its file is an **open importer**. **Importer tests of
P** = `test` files that reach P through edges transitively, plus every `test` file that reaches an
open importer. `plan` lists open importers and their unresolved specifiers so projects can add
`aliases` (workspace packages linked into the repo are the typical case).

**Steps** (current category; deleted paths by their attested category):

1. `mutate` changed/new ⇒ **scope**. `mutate` deleted ⇒ nothing (F1).
2. `test` changed/deleted with an entry in `R.testFiles` ⇒ files with a mutant whose `killedBy` or
   `coveredBy` intersects its `tests[].id` ⇒ **scope**. `test` new, or changed without an entry ⇒
   files in `R` with a `Survived` or `NoCoverage` mutant ⇒ **scope**.
3. `support` changed/new ⇒ mutants whose `killedBy` or `coveredBy` intersects the ids of its importer
   tests ⇒ **forced**; no importer test ⇒ deferral `support <p> has no resolvable importer` on
   `["*"]`. `support` deleted ⇒ deferral `support <p> deleted` on `["*"]`.
4. Residual (`residual.mode`): for each changed/deleted `mutate` file Y, `T_Y` = union of `coveredBy`
   of Y's mutants in `R`; Killed mutants in other files whose `killedBy` intersects `T_Y` ⇒
   **forced**. Changed/new `unclassified` U ⇒ Killed mutants killed by U's importer tests ⇒
   **forced**; none ⇒ deferral `unclassified <p> has no resolvable importer` on `["*"]`. Deleted
   `unclassified` ⇒ nothing. `off`: this step forces nothing and records deferral
   `residual off: <p>` on the files it would have forced.
5. `global` or runtime change ⇒ deferral `global input changed: <p>` on `["*"]`.

**Normalise**:

- Drop from scope and forced every path that is not a regular file in `S` (deleted files; F1 drops
  their mutants).
- A file in both scope and forced is forced whole-file.
- Budget: `N` = mutants of every status in `R`; `b` = `floor(maxForcedShare × N)`, lowered to
  `maxForcedMutants` when that is non-null and smaller; `forcedCount` = distinct forced mutants
  (whole-file entries count all their mutants). In `bounded` mode, `forcedCount > b` ⇒ forced emptied
  and deferral `forced set <forcedCount> exceeds budget <b>` on the forced files. `strict` ignores the
  budget.
- Forced ranges `<file>:<startLine>-<endLine>` (whole-file: `<file>`), merged when `end + 1 >= start`.
- A scope or forced path containing `, { } [ ] ( ) ! * ? :` or a newline ⇒ exit 2 naming it.

Plan JSON: `{ "cold", "scope", "forced", "forcedCount", "deferrals": [{"reason","paths"}],
"changes": [{"path","category","kind"}], "openImporters": [{"path","specifiers"}] }`. Object keys in
byte order; `scope`, `paths`, `openImporters`, `specifiers` in byte order; `forced` by file then
numeric start line; `deferrals` by `reason` then first path; `changes` by `path` then `category`.
Deferral de-duplication compares `reason` and sorted `paths`.

### 5.5 Attestation `<stateDir>/attestation.json` (contract read by metareview)

```json
{
  "schemaVersion": 1,
  "tool": "metareview-mutation-incremental",
  "toolVersion": "0.13.0",
  "engine": "stryker",
  "engineVersion": "10.0.0",
  "mode": "incremental",
  "completedAt": "2026-09-23T18:00:00.000Z",
  "lastFullAt": "2026-09-22T09:00:00.000Z",
  "report": "incremental.json",
  "reportSha256": "<lowercase hex>",
  "lists": { "mutate": [], "test": [], "support": [], "global": [], "ignore": [] },
  "exclusions": [".mutation/**", ".mutation-full/**", ".stryker-tmp/**", "reports/mutation/mutation.json", "reports/mutation/mutation.html"],
  "files": { "src/a.ts": { "digest": "sha256:<hex>", "category": "mutate", "tracked": true } },
  "runtime": { "cmd:node\u001f--version": "<hex>" },
  "deferrals": [ { "reason": "global input changed: package-lock.json", "paths": ["*"] } ]
}
```

- `engineVersion` from `node_modules/@stryker-mutator/core/package.json`; timestamps UTC with
  milliseconds; hex lowercase; `mode` is `incremental`, `full` or `seed`; `lastFullAt` is the
  `completedAt` of the last successful full run (null if none), carried forward by other runs.
- `lists` include the implicit `global` entries; `exclusions` are the resolved patterns of §4 other
  than `ignore` and gitlinks.
- `files` is the start-of-run snapshot; a path whose on-disk digest at the end of the run differs
  from the snapshot gets digest `changed-during-run` (never matches, so it is re-planned next time).
- `deferrals`: an incremental run appends new ones (de-duplicated; previous then new); a successful
  full run writes `[]`.

### 5.6 Execution

1. **Lock.** Create `<stateDir>/lock` with `O_EXCL`, content `{pid, hostname, startedAt}`. If it
   exists: same host and pid not alive ⇒ replace it; otherwise exit 3 (`break-lock` clears a lock
   left on another machine). Released on every exit path.
2. **Work directory.** Remove `<stateDir>/work/` if present and create it.
3. **Adopt** (§5.4), then **plan**.
4. **Invoke.** Incremental mode: invocation 1 = scope (if non-empty, no `--force`); invocation 2 =
   forced (if non-empty, `--force`), run only when invocation 1 succeeded or scope was empty. Each
   invocation k uses `work/inv-k.json`, a copy of the latest successful output (or of
   `incremental.json`; in full mode, no file, so Stryker starts fresh). argv (no shell):
   `<command> run <configFile> --incremental --incrementalFile <stateDir>/work/inv-k.json [--force]
   [--mutate <list joined by ,>] <extraArgs>`, spawned in its own process group. On exceeding
   `maxMinutesPerInvocation` (incremental mode only) and on SIGINT/SIGTERM to the harness, the group
   gets SIGTERM, then SIGKILL after 30 s, and the harness waits for it to exit.
5. **Judge.** An invocation **succeeded** iff it exited 0 or 1, its file was written during the
   invocation (mtime/size/inode changed, or created), and it parses as a mutation-testing-report
   with a `files` object (selected files absent from `files` have zero mutants, F9). Outcomes:
   - success ⇒ latest output; exit 1 ⇒ threshold break;
   - exit 1, file not written, combined stdout+stderr contains `No tests were executed` ⇒ **nothing
     is recorded**: no test reaches those files, so no kill can depend on them (they keep their
     snapshot digests); not a failure;
   - timeout ⇒ deferral `time budget exceeded` on the selection's files; not a failure;
   - other failure of invocation 1 or of the full-mode invocation ⇒ remove `work/`, exit 4;
   - other failure of invocation 2 ⇒ deferral `forced run failed` on the forced files; commit
     invocation 1's output; exit 4.
   When invocation 1 times out, invocation 2 is skipped and its files get deferral
   `blocked by deferred scope`.
6. **No invocation needed** ⇒ the latest output is the canonical `incremental.json`.
7. **Commit.** Write the attestation for the latest output's bytes to `work/attestation.json`; rename
   the output over `incremental.json`, then the attestation over `attestation.json`; remove `work/`.
   A crash between the renames leaves a pair whose hashes disagree ⇒ cold next time, never trusted.
   A cold incremental run with no `incremental.json` writes nothing and reports `pending_full=true`.

### 5.7 CI workflow template (`templates/mutation-incremental/github-workflow.yml`)

`permissions: contents: read`; triggers `push` (branches: main) and `pull_request`; checkout with
`persist-credentials: false`, `fetch-depth: 0`; no secrets passed to mutation jobs (project setup
secrets, if any, stay scoped to their own step). The state directory embeds sources and test failure
text and is readable by every pull request, including forks: never put secrets in it. Marked
insertion points hold project setup (Node, `npm ci`, services).

Two cache families, each saved and restored with one identical path list (F10): **inc** =
`<stateDir>/attestation.json` + `<stateDir>/incremental.json`; **full** =
`.mutation-full/attestation.json` + `.mutation-full/incremental.json`.

- Job `incremental` (timeout 60 min; concurrency `mutation-inc-${{ github.ref }}`, cancel in
  progress on PRs only):
  1. restore **inc**: key `mutation-<main-inc|pr-N>-${{ github.sha }}`, restore-keys `mutation-pr-N-`
     then `mutation-main-inc-` (PRs) or `mutation-main-inc-` (main);
  2. restore **full**: restore-keys `mutation-main-full-`;
  3. `run --mode incremental --also-state .mutation-full`, `continue-on-error: true`, exposing
     `pending_full` and `exit_code`;
  4. when `exit_code` ∈ {0, 1, 4}: save **inc** with key
     `mutation-<main-inc|pr-N>-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}`, and on
     main upload artifact `mutation-state-main` (the two **inc** files, `include-hidden-files: true`,
     retention 30 days);
  5. fail the job when `exit_code` ≠ 0.
- Job `full` (main only; `needs: incremental`;
  `if: ${{ !cancelled() && needs.incremental.outputs.pending_full == 'true' }}`; timeout 350 min;
  concurrency `mutation-main-full`, no cancel in progress): restore **inc** (restore-keys
  `mutation-main-inc-${{ github.sha }}-`) and **full**; `plan --also-state .mutation-full`; stop if it
  shows no pending full run; else `run --mode full`, copy the two state files into `.mutation-full/`,
  save **full** with key `mutation-main-full-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}`.
  A threshold break fails the job.
- Incremental runs adopt the newest full state (`--also-state`, by `lastFullAt`), so a full run's
  results reach every later run on main and every PR regardless of save order.
- Cache limits: GitHub evicts entries unused for 7 days and least-recently-used entries past 10 GB;
  eviction ⇒ cold ⇒ one full run on main, never wrong results. The doc says an idle repository starts
  cold and recommends deleting `mutation-pr-<N>-*` caches when a PR closes.

### 5.8 Seeding

Both forms take the lock, refuse when usable state exists unless `--replace` (which first copies the
current pair into `<stateDir>/replaced/<UTC timestamp>/`), and commit as §5.6 step 7.

- `seed --from <report>`: the report must parse; its bytes become `incremental.json`; the attestation
  has the current snapshot, `mode: "seed"`, and deferral `seeded from <report>` on `["*"]` — the kills
  are pending until the first full run on main.
- `seed --from-state <dir>`: copies `<dir>/attestation.json` and `<dir>/incremental.json`, which must
  form a usable state. The doc fetches main's latest state by run id:
  `gh run list --workflow <file> --branch main --event push --status success --limit 1 --json databaseId`,
  then `gh run download <id> --name mutation-state-main --dir <stateDir>/download`, then
  `seed --from-state <stateDir>/download`.

## 6. Part 2 — Gate-side verification (metareview Go)

### 6.1 Inputs

For each `--mutation-report <path>` the gate reads `attestation.json` in the report's directory. The
report is **attested** iff it parses, `schemaVersion` = 1, `tool` = `metareview-mutation-incremental`,
`engine` equals the report's engine, `report` resolves to the report path, and `reportSha256` equals
the report's bytes; otherwise **unattested** with the first failing reason (missing, unparseable,
version, tool, engine, report mismatch, hash mismatch).

Parse additionally from Stryker reports: `mutants[].id`, `killedBy`, `coveredBy`,
`testFiles[path].tests[].id` (ids used only within one report). New `Mutant`/`Report` fields are
`json:"-"`; `Mutant.File` stays the raw report key, so existing fingerprints are unchanged.

### 6.2 Content under review

task-done and epic-ready read the working tree; pr-ready reads HEAD unless `--include-working-tree`.
Digests as §5.4. HEAD mode uses `git ls-tree -z HEAD -- <path>` for presence and mode (`120000`
symlink; `160000` gitlink ⇒ excluded) and `git cat-file --filters HEAD:<path>` for content. A path
that is missing or not a regular file or symlink has digest `absent`. A read error other than absence
stops the review.

### 6.3 Classification of kills in an attested report

**Changed paths**: attested paths whose current digest differs from `files` (in HEAD mode, an
attested `tracked: false` path absent from HEAD is skipped); plus present paths not in `files`
(listed with `git ls-files -c -o --exclude-standard --deduplicate -z`, or `git ls-tree -r -z HEAD` in
HEAD mode; gitlinks and `exclusions` skipped), categorised with the attestation's `lists` (§5.3):
ignored ones skipped, new `mutate`/`test` skipped (they cannot invalidate a kill), the rest changed.
An unknown category is treated as `global`.

Each kill gets one class, first match wins:

1. **stale** — a cause applies, checked in order: its own file changed; a changed `test` file whose
   ids intersect its `killedBy`; a changed `mutate` file Y whose mutants' `coveredBy` union
   intersects its `killedBy` (the planner's residual rule); any changed `support`, `global` or
   `unclassified` path. The first cause found (ties by byte order of path) is the kill's cause in the
   re-run list. Every changed path that is a cause for at least one kill gets its own finding.
2. **pending** — its file is covered by a deferral.
3. **unbound** — its file is not in `files`, or its `killedBy` is empty.
4. **verified**.

Kills in unattested reports are counted **unattested**. Survivors, uncovered and unresolved mutants
are never removed or reclassified.

### 6.4 Modes

`METAREVIEW_MUTATION_FRESHNESS`: unset, empty or `advisory` ⇒ advisory; `enforce` ⇒ enforce; other
values stop the review with a usage error (exit 2). The effective mode is `advisory` at task-done.

### 6.5 Findings

`<report8>` = first 8 hex of `reportSha256` (unattested: of the report bytes); `<mode>` = effective
mode.

| Fingerprint | One per | Advisory mode | Enforce (pr-ready, epic-ready) | Owner |
|---|---|---|---|---|
| `mutation:stale:<mode>:<engine>:<cause>:<cause8>` | changed causing path | advisory, medium | blocking, high | implementer |
| `mutation:pending:<mode>:<engine>:<report8>` | report with pending kills | advisory, medium | advisory, medium | reviewer |
| `mutation:unattested:<mode>:<engine>:<report8>` | unattested **stryker** report | advisory, medium | blocking, high | reviewer |

- `<cause8>` = first 8 hex of sha256 of `<cause>=<current digest or absent>`.
- Titles: `Mutation evidence stale: <cause> changed` / `Mutation evidence pending: <n> kills` /
  `Mutation report has no attestation`. Text templates are fixed in the implementation plan;
  evidence `{Type: "mutant", Path: <cause or report path>}`.
- Blocking findings use severity `high` because `findings.classForCount` counts `blocking` only at
  `critical`/`high` (`internal/findings/findings.go:806-815`).
- Non-Stryker unattested reports (gremlins) get no finding; the log states
  `freshness not verifiable for <engine>`.
- **Ledger lifecycle** (`findings.Reconcile`, for the three `mutation:` prefixes above only): on every
  run, a row with the same scope and target (`sameRunTarget`) that is absent from the current run and
  `open`, `override-pending` or `overridden` becomes `superseded` (never `fixed`; `fixedInRunId`
  empty); rows of other scopes or targets are untouched. `superseded` rows are not active, so a
  recurring fingerprint opens a new `open` row. FINDINGS.md keeps override request/grant lines on
  superseded rows.

The score summary excludes stale, pending, unbound and unattested kills and counts them; metareview
applies no score threshold, so this changes no verdict by itself.

### 6.6 Review-log section

In all three gates when any report was supplied, after the `## Verdict` value line and any
sharded-review block:

```markdown
## Mutation Evidence Freshness

- Mode: `advisory`
- Reports: 2 (1 attested, 1 unattested: missing)
- Kills: 120 verified, 12 stale, 30 pending, 3 unbound, 40 unattested
- Deferrals: global input changed: package-lock.json
- Freshness not verifiable: gremlins

### Re-run list

| File | Cause | Stale kills |
| --- | --- | ---: |
| `src/a.ts` | `src/a.ts` | 12 |
```

One row per file with stale kills (its cause), sorted by file; omitted when empty. The docs note
that pr-ready reads HEAD, so commit before re-running the harness.

### 6.7 pr-ready digest

`MutationContext` gains serialized `Mode string` and `Freshness []ReportFreshness` (per report:
`Attested`, `UnattestedReason`, `ReportSha256`, `AttestationSha256`, class counts, stale causes with
counts, deferral reasons), `omitempty`, populated only when reports are supplied. Within one metareview
version, runs without reports keep their digest; runs with reports change it whenever the mode, an
attestation, a classification or a report changes.

### 6.8 Escalation exemption

In pr-ready and epic-ready: if every blocking finding among this run's open findings starts with
`mutation:stale:`, a run with `attempt >= maxAttempts` returns `NEEDS_REVISION` while
`attempt < 2 × maxAttempts`; at `attempt >= 2 × maxAttempts` it escalates with reason
`stale mutation evidence not refreshed after <attempt> attempts`.

### 6.9 Post-merge learning

`internal/learning/candidates.go` skips records whose fingerprint starts with any of the three
prefixes in `knowledgeFromFindings`, `repeatedBlockerThemes` and `confidenceForFinding`.

## 7. Verification

### 7.1 Local proof (gate to any external trial)

**Fixture** `testdata/mutation-incremental-e2e/` (committed; its README lists every file verbatim and
each scenario's exact edit):

- `src/a.ts` — `clamp(n, lo, hi)`, three statements on lines 2–4.
- `src/b.ts` — imports `clamp` from `./a` and `type { Range }` from `./types`; an expression-bodied
  arrow on line 4.
- `src/c.ts` — independent `twice(n)` on line 2, tested only by `tests/c.test.ts`.
- `src/types.ts` — `export interface Range { lo: number; hi: number }` (no mutants).
- `src/index.ts` — barrel `export * from './a'` (no mutants).
- `tests/a.test.ts` (imports `a` and `node:assert`), `tests/b.test.ts` (imports `b`, `vitest` and
  `./helpers/make`), `tests/c.test.ts` (imports `@app/c` via `aliases: {"@app/": "src/"}` and
  `./helpers/fmt`), `tests/helpers/make.ts`, `tests/helpers/fmt.ts`.
- Vitest 4.1.11, StrykerJS 10.0.0, pinned lockfile, `.gitignore` for `node_modules/`, `.mutation/`,
  `.mutation-full/`, `reports/`, `.stryker-tmp/`. Fixture config: `maxForcedShare: 1` except in the
  budget scenario.
- `testdata/mutation-incremental/expectations.json` states, per scenario, `N`, the budget, the
  hand-derived `forcedCount`, scope, forced ranges, deferral reasons, invocation argv, and mutant
  identities `(file, mutatorName, start line, replacement)` — derived by hand from the fixture
  source, never from planner output. The script and the offline Node tests both read it.

`tests/e2e-mutation-incremental.sh` (local; needs network for `npm ci`; not in CI) copies the fixture
to `<worktree>/.e2e/<run-id>/` (gitignored; never `/tmp`), runs `git init` and commits, and points
`stryker.command` at a shim that records argv and counts invocations.

| Scenario | Must hold |
|---|---|
| cold incremental | 0 invocations; deferral `no usable state` |
| full | 1 invocation; deferrals `[]`; `lastFullAt` set |
| no change | 0 invocations |
| behaviour-preserving edit on `src/a.ts` line 3 | scope `[src/a.ts]`; forced `[src/b.ts:4-4]` |
| edit `tests/b.test.ts` | scope `[src/a.ts, src/b.ts]` |
| new test file | scope = expectations |
| edit `tests/helpers/make.ts` | forced `[src/a.ts:2-4, src/b.ts:4-4]`; `vitest`/`node:assert` imports are packages, not open importers |
| edit `tests/helpers/fmt.ts` | forced `[src/c.ts:2-2]` |
| remove the alias, edit `tests/helpers/fmt.ts` | `c.test` is an open importer; forced includes `src/c.ts:2-2`; `plan` lists `@app/c` |
| helper nothing imports | deferral `support … has no resolvable importer` |
| edit `package-lock.json` | deferral `global input changed: package-lock.json` |
| budget overflow | forced empty; budget deferral |
| edit only `src/types.ts` | 1 invocation; `No tests were executed`; nothing recorded; exit 0; `pending_full=false` |
| edit `src/a.ts` and `src/types.ts` together | 1 invocation; success |
| edit `src/index.ts` | success; `files` has no `src/index.ts` entry |
| delete `src/b.ts` and `tests/b.test.ts` together | scope `[src/a.ts]`; forced per expectations; `src/b.ts` mutants absent from the report afterwards |
| `git mv src/c.ts src/d.ts` with its test updated | no exit 2; `src/d.ts` in scope |
| `residual.mode: off` / `strict` | deferral `residual off: src/a.ts` on `src/b.ts` / forced ignores budget |
| threshold break (fixture variant with a survivor) | exit 1; state committed |
| Stryker killed mid-run | exit 4; canonical files unchanged |
| time budget exceeded | deferral `time budget exceeded`; exit 0 |
| concurrent run / `break-lock` | exit 3 / lock removed |
| seed / from-state / `--replace` | deferral `seeded from …` / pair copied / old pair in `replaced/<ts>/` |
| **equivalence** | after each edit scenario, a fresh non-incremental run on the same tree; mutants matched by `(file, mutatorName, location, replacement)`; every kill the gate classifies `verified` is a fresh kill (a fresh `Timeout` is re-run once) |
| **equivalence negative control** | scenario `e-flip` (README) with the residual step disabled by a test-only switch: equivalence fails for exactly the listed mutant |
| **gate** | edit committed, `metareview review pr-ready --mutation-report …` before re-running: one stale finding with cause `src/a.ts`; after re-running: none; enforce: blocking; revert without re-running: the finding recurs and blocks; after a `package-lock.json` edit and re-run: pending finding |

The script writes `testdata/mutation-incremental/real/manifest.json` (sha256 of the planner- and
report-affecting template modules, fixture sources, `expectations.json`, the script, lockfile,
Stryker config; tool versions; per-scenario tree digests; wall-clock per row, recorded not asserted)
and commits each scenario's report, attestation and tree under `real/<scenario>/` with `projectRoot`
rewritten to `.`.

### 7.2 CI

- Node: `node --test --experimental-test-coverage --test-coverage-include='templates/mutation-incremental/**/*.mjs' --test-coverage-lines=100 --test-coverage-branches=100 --test-coverage-functions=100`,
  run by a root-package Go test that fails (not skips) when `CI` is set and Node ≥ 22.8 is missing,
  fails on a non-zero exit, and asserts every non-test `.mjs` under the template appears in the
  coverage table at 100%. Tests: glob vectors; import graph (relative, aliases, packages incl.
  builtins, open importers); the planner replaying every `real/<scenario>/` against
  `expectations.json`; budget; deleted/renamed paths dropped from scope; runner outcomes with a fake
  engine (success, not written, `No tests were executed`, timeout, invocation-2 failure, threshold);
  commit ordering; lock and `break-lock`; seeding; exit precedence and the `GITHUB_OUTPUT` lines;
  config validation; and a static workflow test (triggers, permissions, identical cache path lists per
  family, `continue-on-error`, outputs, `!cancelled()`, restore-key order, `include-hidden-files`).
- A CI test recomputes the manifest's hashes (template modules, fixture, expectations, script, and
  each committed `real/<scenario>/` tree) and fails on any difference.
- Go, 100% statement coverage (`make cover`): glob vectors; parser fields; unchanged existing
  fingerprints; attestation validation (each unattested reason); HEAD reader with `.gitattributes
  eol=crlf` and a symlink; `tracked:false` handling; new-path handling per category; each kill class
  and cause order including the residual cause; one finding per causing path; Timeout never a kill;
  modes including invalid value and task-done under enforce; findings per mode, gate and severity;
  gremlins no finding; `Reconcile` supersedes absent `mutation:*` rows of the same scope and target
  only and reopens a recurring fingerprint; log section; digest behaviour; escalation bounds (stale +
  non-stale blocker still escalates at `maxAttempts`); learning filter. Classification tests use the
  committed real reports plus derived fixtures (with `reportSha256` recomputed) for states a real
  report cannot produce.

## 8. Known residuals

- Code in a changed module that carries no mutants (constants, type-only code) affects tests without
  appearing in coverage; the import-graph step narrows but does not close this; `residual.mode:
  strict` does not close it either.
- Computed specifiers (`import(variable)`) and files read by path at runtime are invisible to the
  import graph; support files with no importer defer everything, which covers the common fixture
  case.
- State outside the repo beyond the declared `runtime` inputs.
- The gate trusts the harness's attestation (G3).
- Stryker semantics are version-specific; the harness requires StrykerJS 10.x.
- On integration-heavy suites residual forcing may often exceed the budget and defer; the external
  trial measures how often and tunes the budget.
- An idle repository (no push for 7 days) starts cold and pays one full run.

## 9. Documentation deliverables

- `docs/mutation-harness.md` (setup, config and defaults, aliases and open importers, sizing the
  budget and time ceiling against the ≤ 2 min bar, CI, seeding, cache limits, commit-before-rerun for
  pr-ready, reading the gate's freshness section), linked from `USAGE.md`, `docs/quickstart.md` and
  the `--mutation-report` help text.
- `docs/0.13.0-candidates.md` §1: next to the "prefer metareview's anchor-pin model" bullet, add that
  the transferable idea is scoped, attested binding (pins verify targeted fix claims, not a mutation
  score); add the external-trial follow-up (≤ 2 min p50) and the survivor-severity follow-up (§3).
- `CHANGELOG.md`; `docs/ARCHITECTURE.md` (mutation freshness in the review model).

## 10. Decisions (user, 2026-09-23)

1. Build template + gate staged in one worktree/branch (`mutation-incremental`); prove locally first.
2. Global-input full runs happen post-merge on main, never blocking PRs.
3. Enforcement ramp: task-done advisory always; pr-ready/epic-ready advisory in 0.13.0 with opt-in
   `enforce`; default enforcement planned for 0.14.0.
4. The gate reads a harness attestation.
5. Forced sets past a threshold (default 25% of mutants) defer to a pending full run.
6. The residual is handled deterministically by coverage closure with configurable `residual.mode`
   (`strict|bounded|off`, default `bounded`); no scheduled runs.
7. Keeper adoption and migration are out of this spec.
8. Stale-only runs escalate at 2 × `maxAttempts` (supersedes "never escalate").
9. External-trial pass bar: a one-line edit's incremental run ≤ 2 min p50.
10. After three full review rounds, r4 was re-reviewed only by the lenses that failed r3.
11. r5 got a small verification pass scoped to real workflows, assuming trust; no overengineering.
