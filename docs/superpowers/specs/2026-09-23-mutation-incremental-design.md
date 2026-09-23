# Change-driven mutation testing + attested evidence freshness — design

Status: r21 — final review pending (§11 amendment; r18 approved by `mrv-20260923-205431264136000-…`; r19 and r20 reviewed by `mrv-20260923-210334450256000-…` and `mrv-20260923-210703118968000-…`, one major each, fixed). Blocking only on major or critical findings.
`mrv-20260923-182025208234000-…`, r2 `mrv-20260923-183942601569000-…`, r3
`mrv-20260923-185110177502000-…`; targeted review r4 `mrv-20260923-190310858720000-…`; verification
of r5; full pragmatic reviews r6 `mrv-20260923-192628041933000-…`, r7
`mrv-20260923-193343742411000-…`, r8 `mrv-20260923-194341093013000-…` and r9
`mrv-20260923-195021334650000-…` (3 PASS, 7 NEEDS_REVISION converging on the relative-threshold
baseline, the full job's skip rule and `fetch-state` credentials) and r10
`mrv-20260923-195626283409000-…` (8 PASS, 2 NEEDS_REVISION on one gate blocker, fixed here; the
user then chose the absolute threshold for PRs, decision 15) and r11
`mrv-20260923-200028555622000-…` (9 PASS, 1 NEEDS_REVISION on the PR summary step's cold-run
case) and r12 `mrv-20260923-200340521908000-…` (9 PASS, 1 NEEDS_REVISION on testing which exit
code fails the `full` job) and r13 `mrv-20260923-200638249049000-…` (8 PASS, 2 NEEDS_REVISION
converging on one ledger blocker: a stale-only escalation could not be lifted once its rows were
superseded; fixed here). The user decisions are in §10. Origin:
`docs/0.13.0-candidates.md` §1.

Review stance (user instruction): handle real workflows and real edge cases; do not engineer for
rare races or synthetic cases; assume developers, CI and agents act in good faith. Validity of
mutation evidence is decided only by what changed (content digests), never by dates.

## 1. Problem and engine facts

On the Keeper MVP epic (TypeScript, StrykerJS 10.0.0, Vitest 4.1, real PostgreSQL; 15,660 mutants,
1,135 tests) every one-line edit forced a ~2.5–3.5 h mutation regeneration: the project harness
binds reports to whole-tree manifests and re-runs Stryker from scratch without `--incremental`.
Scoping each run to the change (rather than running `--incremental` over the whole `mutate` set)
matters because the dry run executes every test related to the mutated files (F4); on an
integration-heavy suite that is most of the suite. The harness restores the guarantees that scoping
gives up (F1, F2).

Facts verified in StrykerJS 10.0.0 (`@stryker-mutator/core/dist/src`), its instrumenter, Vitest
4.1.11, GitHub Actions and git (reviewers reproduced F4, F5, F9 and the §7.1 rows on a real project):

- F1. `--incremental` reuses per-mutant results: a Killed mutant is reused when its killing test is
  unchanged; a non-killed one when no covering test changed. Without `--force`, kills in unchanged
  regions of an edited file are reused, not re-run — a behaviour-changing edit elsewhere in the same
  file can invalidate them. `--force` re-runs every in-scope mutant. Mutants of files that no longer
  exist are dropped.
- F2. Out-of-scope mutants of files that still exist are carried forward without re-evaluation,
  regardless of `--force`; a changed test drops out of their `killedBy`; mutants in changed regions
  of out-of-scope files are removed.
- F3. The report embeds each file's source as Stryker first read it in that run.
- F4. The dry run always happens. The Vitest runner (`related: true` default) runs only tests whose
  transformed dependency graph reaches a mutated file (type-only imports elided); zero tests ⇒
  `No tests were executed` on stdout and stderr, exit 1, nothing written (`--allowEmpty` returns
  before the report is written). The Vitest runner always uses per-test coverage, so
  `killedBy`/`coveredBy` are populated; it reports no test locations, so any edit to a test file
  invalidates all of that file's tests in Stryker's own differ. `killedBy` depends on test order
  unless `disableBail: true`.
- F5. `--mutate` accepts `<file>` and `<file>:<startLine>-<endLine>` (1-based), comma-separated,
  combinable with `--force`, and replaces the config's `mutate`. Stryker's matcher is minimatch with
  `dot: false`.
- F6. The incremental file (`--incrementalFile`) is read and written in place. On
  SIGINT/SIGTERM/SIGHUP/SIGABRT Stryker writes partial results and exits 128+n. Thrown errors exit 1
  without writing. A threshold break exits 1 after writing.
- F7. Mutant ids are unique only within one report. The report has no timestamp.
- F8. Without a positional config file Stryker searches its own list of names.
- F9. The report's `files` contains only files that produced at least one mutant; a selected
  zero-mutant file (type-only, barrel, constants) is absent, and a run whose selection has no mutants
  but reachable tests succeeds with `files: {}`.
- F10. `actions/cache` includes the exact `path` list in each entry's version (a restore only hits
  entries saved with the identical list), and GitHub evicts entries not accessed for 7 days.

metareview today ingests `--mutation-report` files and accepts a stale report silently.

## 2. Goals

- G1 — Per-change cost bounded by the change: one related-tests dry run plus the mutants the change
  can affect (edited files are re-run whole, decision 12). Proven locally by deterministic counts
  (§7); the external trial's pass bar is a one-line edit ≤ 2 min p50 (decision 9), tracked in
  `docs/0.13.0-candidates.md` together with the share of PR runs that end pending a full run.
- G2 — Every change is re-verified in the same run or recorded as **deferred**, visibly; kills that
  depend on deferred work are never reported as verified. Planning is clock-free; the only
  time-dependent outcome is the per-invocation time budget, which always defers. Documented
  exceptions are in §8.
- G3 — The gate states exactly what it verified (accident-level check; not tamper proof).
- G4 — No scheduled job and no date-based validity. A full run happens only when something was
  deferred; stored state stays valid until what it describes changes.

## 3. Non-goals

- Changing any mutation engine or editing Stryker's report format.
- Test runners other than StrykerJS 10.x with its Vitest runner in the template; Yarn Plug'n'Play
  (no `node_modules`) is unsupported.
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
- **Excluded paths**: gitlinks; `<stateDir>/**`; Stryker's `tempDirName` (default `.stryker-tmp`)
  `/**`; Stryker's `jsonReporter.fileName` and `htmlReporter.fileName`; and paths whose category
  resolves to ignore (§5.3). The attestation records the resolved exclusions and the category lists
  so the gate applies the same rules.
- **Deferral**: `{reason, paths}`, `paths` being files or `["*"]`. A kill is covered by a deferral if
  its file is in `paths` or `paths` is `["*"]`.
- **Pending full run**: state is not usable, or the attestation has deferrals. Cleared by a successful
  full run, or by adopting a state without deferrals (a green catch-up, §5.7).
- **Edited files**: `mutate` files changed or new since the attested snapshot.

## 5. Part 1 — Harness template (`templates/mutation-incremental/`)

Zero-dependency Node ≥ 22.8 ESM, copied by a project to `tools/mutation-incremental/`. The repository
owns what runs; metareview owns the attestation contract (§5.5) and the glob dialect (§5.3).

### 5.1 CLI

`node tools/mutation-incremental/cli.mjs <command> [--config <path>]`. The top-level is
`git rev-parse --show-toplevel`; `--config` defaults to `<top>/mutation-incremental.json` and
resolves against the top-level; every command runs with cwd = top-level.

| Command | Behaviour |
|---|---|
| `plan [--also-state <dir>]...` | print the plan (§5.4) as JSON; adoption in memory only; no side effects |
| `run --mode incremental\|full [--also-state <dir>]...` | lock, adopt, plan, execute (§5.6), commit |
| `seed --from <report> [--replace]` | bootstrap from a full Stryker report (§5.8); refusing because usable state exists is exit 2; never exits 1 (it prints the score) |
| `fetch-state [--remote <name>]` | fetch the state branches into `<stateDir>/remote/{inc,full}/` (§5.7) |
| `publish-state --kind inc\|full [--remote <name>]` | push the canonical state to branch `mutation-state/<kind>` (§5.7) |
| `break-lock` | print and remove `<stateDir>/lock` |

Exit codes: 0 ok (deferrals included); 1 ok, but the threshold check fails (computed by the harness
on every run from the committed report and the current config, so a re-run of a red job stays red
and lowering the threshold turns it green): the check fails when the score is below
`thresholds.break`, the same rule on PRs, main and local runs (decision 15); 2 config/usage/validation; 3 lock held; 4 engine failure (nothing committed); 130
interrupted (SIGINT/SIGTERM/SIGHUP; nothing committed). Precedence: 2 > 3 > 130 > 4 > 1 > 0. When
`GITHUB_OUTPUT` is set, `run` appends on every exit path `pending_full=<true|false>` (the canonical
state afterwards is a pending full run; `false` if exit 2 happens before the config is readable) and
`exit_code=<n>`. Every command prints one stderr line:
`mutation-incremental: command=<c> invocations=<n> scope=<k> forced=<j> deferrals=<d> pending_full=<bool>`
(commands other than `run` and `plan` print zeros and the canonical state's `pending_full`). `plan`
exits 0 with its JSON, or 2. When the first of `.nvmrc`, `.node-version` that exists, trimmed, holds a numeric
version (after stripping a leading `v`) whose dot-separated components are not a prefix of the running
`node --version`'s components (also without `v`), `plan` and `run` print one warning line before the
summary line; aliases such as `lts/iron` are skipped. It is not an error (the file is already a
`global` input, so CI state is protected by digest; a local mismatch is the developer's choice).

### 5.2 Config `mutation-incremental.json`

```json
{
  "schemaVersion": 1,
  "stateDir": ".mutation",
  "stryker": { "command": ["npx", "--no-install", "stryker"], "configFile": "stryker.config.json", "extraArgs": [] },
  "mutate":  ["src/**/*.ts", "!src/**/*.d.ts"],
  "test":    ["tests/**/*.test.ts"],
  "support": ["tests/helpers/**", "tests/factories/**", "tests/fixtures/**"],
  "global":  ["package.json", "package-lock.json", "stryker.config.json", "vitest.config.ts", "tsconfig*.json", "migrations/**", ".env*", ".nvmrc", ".node-version"],
  "ignore":  ["docs/**", "**/*.md", ".github/**", ".vscode/**", ".gitignore", "LICENSE*", "Dockerfile", "Makefile", ".metareview/**", ".beads/**", ".claude/**"],
  "aliases": {},
  "runtime": { "commands": [], "env": [] },
  "budget":  { "maxForcedShare": 0.25, "maxForcedMutants": null, "maxMinutesPerInvocation": 20 },
  "residual": { "mode": "bounded" }
}
```

All keys are required; the values above are the shipped defaults (decisions 5, 6). Validation (exit
2 naming the reason): exactly these keys; `schemaVersion` = 1; `aliases` maps specifier prefixes to
repo-relative path prefixes; `residual.mode` ∈ `strict|bounded|off`; `maxForcedShare` in (0, 1];
`maxForcedMutants` positive integer or null; `maxMinutesPerInvocation` positive; `stateDir` is a
dedicated directory inside the top-level (not the top-level itself, not matching any `mutate`,
`test` or `support` pattern) and not under the OS temp dir; `node_modules/@stryker-mutator/core`
major version 10; `stryker.configFile` is JSON with `"testRunner": "vitest"`, no `"inPlace": true`, a
`mutate` array equal to config `mutate`, and `ignorePatterns` containing the exact string `<stateDir>`
or `<stateDir>/**`. `runtime.env` values are stored only as sha256 digests; the docs say never to list
secrets there. Runtime inputs must be identical on every machine that shares state (CI and
laptops), so the default has none: the docs recommend pinning Node with a tracked `.nvmrc` (a global
input) used by both `setup-node` and developers, rather than a `node --version` runtime command,
which would keep local state pending whenever a laptop's Node differs from CI's. The docs explain
sizing `maxMinutesPerInvocation` (a safety ceiling, not the target; two invocations plus setup must
fit the CI job timeout) and the budget against the ≤ 2 min bar, list common files to add to
`ignore`, and tell projects to gitignore `<stateDir>/`.

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

**Adopt** (`--also-state <dir>`, repeatable; change-based, decision 14): candidates are the primary
`stateDir` and each `<dir>`; only usable candidates count (a missing
directory, e.g. a branch `fetch-state` did not find, is simply not usable). For each, compute its change set `C`
against the current snapshot. Choose by, in order: candidates without deferrals first; then the
smallest `|C|`; then the primary; then the order given. So a PR keeps its own warm state unless it
carries deferrals that a newer main state has cleared, and an updated or rebased branch picks the
state closest to its files. Timestamps (`lastFullAt`, `completedAt`) are recorded for information and
never decide adoption or validity. `run` copies an adopted state's two files into `stateDir` (temp
file + rename); `plan` adopts in memory only. The chosen state is the run's **baseline**.

**Usable**: `attestation.json` parses with `schemaVersion` 1, `tool` =
`metareview-mutation-incremental` and `toolVersion` equal to the running harness's version, and
`incremental.json`'s sha256 equals `reportSha256`. Otherwise **cold**: incremental mode runs nothing
and records deferral `no usable state` on `["*"]`; full mode runs a full run. (A harness upgrade thus
costs one full run; the gate never looks at `toolVersion`.)

**Change set** `C`: paths whose digest differs from the attestation, new paths, attested paths absent
from `S`, differing runtime keys. A path whose category changed is in `C` under both categories.
`R` is the canonical `incremental.json`.

**Import graph**: parse `.ts .tsx .mts .cts .js .jsx .mjs .cjs` files (≤ 1 MiB) in `S`. Edges from
string specifiers in `import … from`, `export … from`, `import '…'`, `import('…')`, `require('…')`;
not `import type`/`export type`, not `vi.mock`. A specifier **resolves** when it starts with
`./`/`../` (relative to the file) or with a configured `aliases` key (longest key's prefix replaced),
to a regular file in `S`: the path, then + each extension above, then `<path>/index` + each
extension; a `.js/.jsx/.mjs/.cjs` specifier also tries its stem + `.ts .tsx .mts .cts`. A specifier
is a **package** when it is a Node builtin (`node:` prefix or `module.builtinModules`), or when
walking up from the importing file finds `node_modules/<name>` (`<name>` = first segment, or first
two for `@scope/`) whose realpath still contains a `node_modules/` segment — true for npm, pnpm and
yarn with `nodeLinker: node-modules`, false for workspace links into the repo's own sources. Anything
else is **unresolved**, and its file is an **open importer**. A file **reaches** P if it is P or
imports P through edges transitively. **Importer tests of P** = `test` files that reach P, plus every
`test` file that reaches an open importer (including a test file that is itself an open importer).
`plan` lists open importers and their unresolved specifiers so projects can add `aliases`.

**Steps** (current category; deleted paths by their attested category):

1. `mutate` changed/new ⇒ **edited** (forced whole-file, decision 12). `mutate` deleted ⇒ nothing
   (F1).
2. `test` changed/deleted with an entry in `R.testFiles` ⇒ files with a mutant whose `killedBy` or
   `coveredBy` intersects its `tests[].id` ⇒ **scope**. For every changed or new `test` file T:
   `mutate` files that T reaches which are absent from `R.files` or have `Survived`/`NoCoverage`
   mutants ⇒ **scope** (a new module's first test, or a new case covering an uncovered file); if T is
   an open importer or reaches one, every `mutate` file absent from `R.files` and every file in `R`
   with `Survived`/`NoCoverage` mutants ⇒ **scope**.
3. `support` changed/new ⇒ mutants whose `killedBy` or `coveredBy` intersects the ids of its importer
   tests ⇒ **forced**; no importer test ⇒ deferral `support <p> has no resolvable importer` on
   `["*"]`. `support` deleted ⇒ deferral `support <p> deleted` on `["*"]`.
4. Residual (`residual.mode`): for each changed/deleted `mutate` file Y, `T_Y` = union of `coveredBy`
   of Y's mutants in `R`; when a changed Y has no mutants in `R` (constants, a barrel, type-only
   code: no coverage to read), `T_Y` = the ids of Y's importer tests instead; Killed mutants in other files whose `killedBy` intersects `T_Y` ⇒
   **forced**. Changed/new `unclassified` U ⇒ Killed mutants killed by U's importer tests ⇒
   **forced**; none ⇒ deferral `unclassified <p> has no resolvable importer` on `["*"]`. Deleted
   `unclassified` ⇒ nothing. `off`: this step forces nothing and records deferral
   `residual off: <p>` on the files it would have forced.
5. `global` or runtime change ⇒ deferral `global input changed: <p>` on `["*"]`.

**Normalise**:

- Drop from edited, scope and forced every path that is not a regular file in `S` (deleted files;
  F1 drops their mutants).
- Edited files are forced whole-file and removed from scope; they are exempt from the budget.
- A non-edited file in both scope and forced is removed from scope and forced whole-file.
- Budget (applies only to forced entries that are not edited files): `N` = mutants of every status
  in `R`; `b` = `floor(maxForcedShare × N)`, lowered to `maxForcedMutants` when that is non-null and
  smaller; `forcedCount` = distinct non-edited forced mutants before any drop (whole-file entries count
  all their mutants in `R`; edited files never count). In `bounded` mode, `forcedCount > b` ⇒ the
  non-edited forced entries are dropped, files that were moved out of scope by the previous rule
  return to scope, and deferral `forced set <forcedCount> exceeds budget <b>` is recorded on every
  non-edited file that had forced mutants (including those back in scope, whose un-forced reuse is
  not trusted). Edited files stay forced and are never deferred by the budget. `strict` ignores the
  budget. The plan's `forcedCount` is this pre-drop value.
- Forced ranges `<file>:<startLine>-<endLine>` (whole-file: `<file>`), merged when `end + 1 >= start`.
- A scope or forced path containing `, { } [ ] ( ) ! * ? :` or a newline ⇒ exit 2 naming it.

Plan JSON: `{ "baseline", "cold", "pendingFull", "scope", "forced", "forcedCount", "deferrals":
[{"reason","paths"}], "changes": [{"path","category","kind"}], "openImporters":
[{"path","specifiers"}], "unclassified": [paths] }`. `baseline` is the adopted
candidate: the `--also-state` argument as typed, or the config's `stateDir` value as written for
the primary; null when cold. `kind` ∈ `changed|new|deleted`; runtime keys
appear in `changes` with category `global` and in reasons by their key (e.g. `env:DATABASE_IMAGE`).
In `off` mode, an unclassified path with no importer still records its `["*"]` deferral. `pendingFull` = cold, or
the (adopted) prior attestation has deferrals, or this plan adds any. A cold plan has empty `scope`,
`forced` and `changes`. `unclassified` lists changed unclassified paths so projects can extend
`ignore`. Object keys in byte order; `scope`, `paths`, `openImporters`, `specifiers`, `unclassified`
in byte order; `forced` by file then numeric start line; `deferrals` by `reason` then first path;
`changes` by `path` then `category`. Deferral de-duplication compares `reason` and sorted `paths`.

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
  "score": 92.31,
  "thresholdBreak": false,
  "lists": { "mutate": [], "test": [], "support": [], "global": [], "ignore": [] },
  "exclusions": [".mutation/**", ".stryker-tmp/**", "reports/mutation/mutation.json", "reports/mutation/mutation.html"],
  "files": { "src/a.ts": { "digest": "sha256:<hex>", "category": "mutate", "tracked": true } },
  "runtime": { "cmd:node\u001f--version": "<hex>" },
  "deferrals": [ { "reason": "global input changed: package-lock.json", "paths": ["*"] } ]
}
```

- `schemaVersion` is the gate contract; changes to the harness's planning rules change `toolVersion`
  only (§5.4 Usable).
- `engineVersion` from `node_modules/@stryker-mutator/core/package.json`; timestamps UTC with
  milliseconds; hex lowercase; `mode` is `incremental`, `full` or `seed`; `lastFullAt` is the
  `completedAt` of the last successful full run (null if none, and null after `seed`), carried
  forward by other runs.
- `score` = the committed report's mutation score, (Killed + Timeout) / (Killed + Timeout + Survived
  + NoCoverage) × 100 (Stryker's definition), or `null` when that denominator is 0. `thresholdBreak`
  = `score` is a number below the current Stryker config's `thresholds.break` (`false` when the
  threshold is null or absent, or `score` is null). Both are recomputed on every run.
- Report keys are repo-relative POSIX paths (Stryker runs at the top-level); a key that is not is
  treated by the gate as absent from `files` (unbound).
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
   edited + forced (if non-empty, `--force`), run unless invocation 1 timed out or failed. Each
   invocation k uses `work/inv-k.json`, a copy of the latest successful output (or of
   `incremental.json`; in full mode, no file, so Stryker starts fresh). argv (no shell):
   `<command> run <configFile> --incremental --incrementalFile <stateDir>/work/inv-k.json [--force]
   [--mutate <list joined by ,>] <extraArgs>`, spawned in its own process group. On exceeding
   `maxMinutesPerInvocation` (incremental mode only) and on SIGINT/SIGTERM/SIGHUP to the harness, the
   group gets SIGTERM, then SIGKILL after 30 s, and the harness waits for it to exit.
5. **Judge.** An invocation **succeeded** iff it exited 0 or 1, its file was written during the
   invocation (mtime/size/inode changed, or created), and it parses as a mutation-testing-report
   with a `files` object (selected files absent from `files` have zero mutants, F9). Outcomes:
   - success ⇒ latest output;
   - in full mode, `No tests were executed` is an engine failure (exit 4, nothing committed): a full
     run that finds no tests (e.g. a dependency bump broke every import) must never be attested;
   - incremental mode, exit 1, file not written, combined stdout+stderr contains
     `No tests were executed` ⇒ **no reachable tests**: not a failure; the latest output is unchanged. Selected files that have
     `Killed` mutants in the latest output get deferral `no reachable tests: <p>` (their killing
     tests no longer reach them, e.g. the only test was deleted); selected files without kills
     (type-only files, new modules before their first test) record nothing. A new module is picked up
     when a test first reaches it (§5.4 step 2);
   - timeout ⇒ deferral `time budget exceeded` on the selection's files; not a failure; if it is
     invocation 1, invocation 2 is skipped and its files get deferral `blocked by deferred scope`;
   - interrupted ⇒ remove `work/`, commit nothing, exit 130;
   - any other failure ⇒ remove `work/`, commit nothing, exit 4 (the run is retryable as is).
6. **No successful invocation** (none needed, or all no-tests/deferred) ⇒ the latest output is the
   canonical `incremental.json` unchanged; the attestation is still rewritten (new snapshot, new
   deferrals, recomputed `thresholdBreak`).
7. **Commit.** Write the attestation for the latest output's bytes to `work/attestation.json`; rename
   the output over `incremental.json`, then the attestation over `attestation.json`; remove `work/`.
   A crash between the renames leaves a pair whose hashes disagree ⇒ cold next time, never trusted.
   A cold incremental run with an `incremental.json` present re-attests those bytes with
   `lastFullAt: null` and the `no usable state` deferral; with no `incremental.json` it writes nothing
   and reports `pending_full=true`.

### 5.7 State storage and CI workflow template (`templates/mutation-incremental/github-workflow.yml`)

**Durable state (change-based, decision 13).** Two branches, each a single commit holding
`attestation.json` and `incremental.json` at the root: `mutation-state/inc` (main's latest
incremental state) and `mutation-state/full` (main's latest state without deferrals: a full run, or a
catch-up that ended with none, §5.7 job `full`). `publish-state --kind
<k>` builds a parentless commit with git plumbing (`hash-object`, `mktree`, `commit-tree`, author
and committer `mutation-incremental <noreply@localhost>` set explicitly) and force-pushes it to
`refs/heads/mutation-state/<k>`; the workflow gives `contents: write` only to the main-only jobs, so the
token is a push credential only there. It pushes only when the
canonical state is usable (otherwise it prints why and exits 0; a rejected or failed push exits 2,
so a ruleset that does not yet allow `mutation-state/*` fails the step visibly), and each branch has a single writer
(below), so it is replaced only when main's state changes; nothing expires. The docs mark
`publish-state` as CI-only. `fetch-state` and `publish-state` read the token from `MUTATION_STATE_TOKEN` (empty ⇒ unset) and
pass it to git as `http.<remote URL>.extraheader=AUTHORIZATION: basic <base64 of
x-access-token:<token>>` through `GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_<n>`/`GIT_CONFIG_VALUE_<n>`,
appended after any entries already in the environment (never argv); under `GITHUB_ACTIONS` they
first print `::add-mask::` for the base64 value. Unset ⇒ git's own credentials (local use). `fetch-state` lists `refs/heads/mutation-state/*` with `git ls-remote`
(so a missing branch is skipped, while a network or auth failure exits 2), fetches each present
branch without `--depth` (the commit has no parents, and a depth fetch would make the clone
shallow), and writes its two files into `<stateDir>/remote/inc/` and `<stateDir>/remote/full/`.
Repository rulesets that restrict updates or force-pushes on all branches must allow both on
`mutation-state/*`.

**Fast path.** `actions/cache` keeps the two canonical files (one identical path list, F10:
`<stateDir>/attestation.json` + `<stateDir>/incremental.json`) for quick restores while entries are
within GitHub's 7-day window; a cache miss falls back to the branches, never to cold.

The state files, caches, job summaries and logs embed sources and test failure text and are readable
by anyone who can read the repository (for public repositories: everyone, including fork PRs):
never put secrets in them.

Workflow (`push` to main and `pull_request` only; workflow-level `permissions: {}`; every checkout
uses `fetch-depth: 0` and `persist-credentials: false`; no secrets passed to mutation jobs;
`MUTATION_STATE_TOKEN: ${{ github.token }}` is set only on `fetch-state` and `publish-state` steps
(read-only in `incremental-pr`, so private repositories can fetch the state branches); marked insertion points hold
project setup such as Node from `.nvmrc`, `npm ci`, services). GitHub sets `permissions` per job and
not per event, so the incremental work is two jobs. Each job starts with checkout and the project
setup insertion point, then the steps below:

- Job `incremental-pr` (`if: github.event_name == 'pull_request'`; `permissions: contents: read`;
  timeout 60 min; concurrency `mutation-inc-${{ github.ref }}`, cancel in progress):
  1. restore the cache: key `mutation-pr-N-${{ github.sha }}`, restore-keys `mutation-pr-N-` then
     `mutation-main-`;
  2. `fetch-state`;
  3. `run --mode incremental --also-state <stateDir>/remote/full --also-state
     <stateDir>/remote/inc`, `continue-on-error: true`, exposing `exit_code`;
  4. when `exit_code` ∈ {0, 1} and `<stateDir>/attestation.json` exists: save the cache with key
     `mutation-pr-N-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}`;
  5. (`if: always()`, never fails) write to `$GITHUB_STEP_SUMMARY`: `exit_code`; `pending_full`;
     the deferral reasons; the score against `thresholds.break`, with "includes pending kills" when
     `pending_full=true` and labelled "previous state (this run committed nothing)" when `exit_code`
     ∉ {0, 1}; and main's score from the first of `<stateDir>/remote/inc`, `remote/full` whose
     report matches its `reportSha256` (none ⇒ "main state unavailable"), so a PR that is red only
     because main is already below the threshold says so (labelled "includes pending kills" when that
     attestation has deferrals). An empty `exit_code` prints only "run step did not execute or did
     not finish (see earlier step)"; a missing attestation otherwise (a cold
     first run, which exits 0, or exit 2) prints `no usable state`. A green job is never mistaken for
     fully verified work;
  6. fail the job when `exit_code` is empty or ≠ 0.
- Job `incremental-main` (`if: github.event_name == 'push'`; `permissions: contents: write`; timeout
  60 min; concurrency `mutation-inc-${{ github.ref }}`, no cancel in progress): the same steps with
  cache keys `mutation-main-…`, restore-keys `mutation-main-` (whichever `mutation-main-` entry is
  newest is restored; adoption re-ranks it against the branches), exposing `pending_full` and
  `exit_code`; step 4 also runs `publish-state --kind inc`; step 5 omits main's score.
- Job `full` (`needs: incremental-main`;
  `if: ${{ !cancelled() && needs.incremental-main.outputs.pending_full == 'true' }}`; timeout 350 min;
  `permissions: contents: write`; concurrency `mutation-main-full`, no cancel in progress — a newer
  pending run replaces an older one, which is safe because planning is content-based):
  1. `fetch-state`;
  2. **catch-up**: `run --mode incremental --also-state <stateDir>/remote/full --also-state
     <stateDir>/remote/inc` (`continue-on-error: true`). It brings the freshest published state up
     to this commit by executing, not just planning, so deferrals that arise only while running
     (`time budget exceeded`, `blocked by deferred scope`, `no reachable tests`) are re-derived, and a
     `pending_full` that a push inherited from a state a full run has since cleared is dropped;
  3. when the catch-up exited 0 or 1 with `pending_full=false`: save the cache (key
     `mutation-main-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}-catchup`),
     `publish-state --kind full` (so the no-deferral state never ages and the next push adopts it
     instead of redoing the delta), and stop; otherwise `run --mode full` with
     `continue-on-error: true` (full mode starts fresh);
  4. when the full run's `exit_code` ∈ {0, 1}: save the cache (key `mutation-main-${{ github.sha
     }}-${{ github.run_id }}-${{ github.run_attempt }}-full`, distinct from `incremental-main`'s
     key in the same workflow run), then `publish-state --kind full`;
  5. (a shell step, not an expression) fail the job when `final_exit` is empty or ≠ 0, where
     `final_exit` is the full run's
     `exit_code` when the full-run step ran and the catch-up's otherwise (a threshold break is red,
     but its state is published, so the full run is not repeated on every push). Publish steps are
     never `continue-on-error`.

  The `full` job is the only writer of `mutation-state/full`. It restores no cache; its baseline is
  the state branches. Setup, the catch-up (at most 2 × `maxMinutesPerInvocation`) and the full run
  must fit the job timeout together; the docs say so.
- Later runs on main and every PR adopt the `mutation-state/full` state when their own state carries deferrals
  (§5.4 Adopt), regardless of the order in which jobs finished.
- Local use: `fetch-state`, then `run --mode incremental --also-state <stateDir>/remote/full
  --also-state <stateDir>/remote/inc`. A new worktree can instead use `--also-state <first
  checkout>/.mutation`.

### 5.8 Seeding

`seed --from <report> [--replace]` takes the lock and refuses when usable state exists unless
`--replace` (which first copies the current pair into `<stateDir>/replaced/<UTC timestamp>/`); the
report must parse; its bytes become `incremental.json`; the attestation has the current snapshot,
`mode: "seed"`, `lastFullAt: null`, and deferral `seeded from <report>` on `["*"]` — the kills are
pending until the first full run on main. It commits as §5.6 step 7.

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
   `unclassified` path, or changed `mutate` path with no mutants in the report (constants, barrels:
   the gate has no import graph, so like support it invalidates every kill until the harness
   re-runs). The first cause found (ties by byte order of path) is the kill's recorded
   cause. Findings and the re-run list are keyed on recorded causes.
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
| `mutation:stale:<mode>:<engine>:<cause>:<cause8>` | distinct recorded cause | advisory, medium | blocking, high | implementer |
| `mutation:pending:<mode>:<engine>:<report8>` | report with pending kills | advisory, medium | advisory, medium | reviewer |
| `mutation:unattested:<mode>:<engine>:<report8>` | unattested **stryker** report | advisory, medium | blocking, high | reviewer |

- `<cause8>` = first 8 hex of sha256 of `<cause>=<current digest or absent>`. With several reports
  sharing a cause, their counts are summed into one finding.
- Pending stays advisory even under enforce: deferred work is never read as evidence (it is
  excluded from the score and shown separately), and full runs never block PRs (decision 2).
- Titles: `Mutation evidence stale: <cause> changed` / `Mutation evidence pending: <n> kills` /
  `Mutation report has no attestation`. Text templates are fixed in the implementation plan;
  evidence `{Type: "mutant", Path: <cause or report path>}`.
- Blocking findings use severity `high` because `findings.classForCount` counts `blocking` only at
  `critical`/`high` (`internal/findings/findings.go:806-815`).
- Non-Stryker unattested reports (gremlins) get no finding; the log states
  `freshness not verifiable for <engine>`.
- **Ledger lifecycle** (`findings.Reconcile`, for the three `mutation:` prefixes above only), applied
  only on runs that supplied at least one `--mutation-report` (a quick run without reports leaves
  these rows and their overrides alone): a row with the same scope and target (`sameRunTarget`) whose
  `<engine>` matches an engine among this run's supplied reports (a gremlins-only run leaves stryker
  rows alone), that is absent from the current run and `open` or `override-pending` becomes `superseded`
  (`overridden` rows are left as they are: their fingerprint pins the evidence the override
  accepted, and `EscalationLiftedByOverrides` reads only `overridden`, so a lifted escalation stays
  lifted)
  (never `fixed`; `fixedInRunId` empty); rows of other scopes or targets are untouched. Code changes
  this requires: the existing fix branch skips these prefixes under both of its conditions
  (`--previous-run` and escalation reset); `activeExisting` excludes `superseded` rows of these
  prefixes, so a recurring fingerprint opens a new `open` row; `overrideLines` renders the override
  request/grant of superseded rows, tagged `[superseded]`; `RequestOverride` and `GrantOverride`
  accept a `superseded` row of these prefixes when the request carries an escalation, exactly as
  they already accept a `fixed` one (`fixedWithEscalation`, `internal/findings/override.go`), so a
  stale-only escalation (§6.8) can still be lifted after the evidence is refreshed.

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

One row per (file, recorded cause), sorted by file then cause; omitted when empty. The docs note
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

- `src/a.ts` — `clamp(n, lo, hi)` with a block body on lines 1–5 (three statements on lines 2–4), so
  Stryker's `BlockStatement` mutant spans lines 1–5.
- `src/b.ts` — `import { clamp } from './a'` and `import type { Range } from './types'`; an
  expression-bodied arrow on line 4.
- `src/c.ts` — `import { LIMIT } from './limits'` on line 1; expression-bodied arrow `twice` on
  line 2 using `LIMIT`, tested only by `tests/c.test.ts`.
- `src/limits.ts` — `export const LIMIT = 100;` (no mutants: StrykerJS has no numeric-literal
  mutator).
- `src/types.ts` — `export interface Range { lo: number; hi: number }` (no mutants).
- `src/index.ts` — barrel `export * from './a'` (no mutants), imported by `tests/index.test.ts`.
- `tests/a.test.ts` (imports `a` and `node:assert`), `tests/b.test.ts` (imports `b`, `vitest` and
  `./helpers/make`), `tests/c.test.ts` (imports `@app/c` and `./helpers/fmt`), `tests/index.test.ts`
  (imports `../src/index`), `tests/helpers/make.ts`, `tests/helpers/fmt.ts`.
- `aliases: {"@app/": "src/"}` in the harness config and the same alias in `vitest.config.ts`
  (`resolve.alias`).
- Vitest 4.1.11, StrykerJS 10.0.0 with `disableBail: true` (so `killedBy` lists every killing test
  and does not depend on test order), pinned lockfile, `.gitignore` for `node_modules/`, `.mutation/`,
  `reports/`, `.stryker-tmp/`. Fixture config: `maxForcedShare: 1` except in the budget scenario.
- `testdata/mutation-incremental/expectations.json` states, per scenario, `N`, the budget, the
  hand-derived `forcedCount`, scope, forced entries, deferral reasons, invocation argv, and mutant
  identities `(file, mutatorName, location, replacement)` (the same key the equivalence row uses) —
  derived by hand from the fixture
  source, never from planner output. The script and the offline Node tests both read it.

`tests/e2e-mutation-incremental.sh` (local; needs network for `npm ci`; not in CI) copies the fixture
to `<worktree>/.e2e/<run-id>/` (gitignored; never `/tmp`), runs `git init`, adds a local bare remote
for `publish-state`/`fetch-state`, commits, and points `stryker.command` at a shim that records argv,
counts invocations and can sleep on request (for the time-budget row). Unless a row says otherwise,
each row starts from the committed tree and the state produced by the `full` row; survivor-variant
and threshold rows start from a `full` run of their variant. Rows about the CI jobs run the script's
emulation of the job's steps (same commands and exit-code gates as the workflow template).

| Scenario | Must hold |
|---|---|
| cold incremental | 0 invocations; `plan` shows `cold`, deferral `no usable state`; stderr `pending_full=true`; exit 0; PR job emulation: no cache save, the summary prints `no usable state`, the job is green |
| full | 1 invocation; deferrals `[]`; `lastFullAt` set |
| no change | 0 invocations |
| behaviour-preserving edit on `src/a.ts` line 3 | scope `[]`; forced `[src/a.ts, src/b.ts:4-4]` (edited whole file + residual) |
| behaviour-changing edit on `src/a.ts` line 2 that makes a line-4 mutant survive | forced `[src/a.ts, …]`; that mutant is `Survived` in the new report; equivalence holds |
| edit `tests/b.test.ts` | scope `[src/a.ts, src/b.ts]` |
| new test file `tests/b2.test.ts` importing `src/b.ts` (base fixture has no survivors) | scope `[]` |
| survivor variant: `src/a.ts` has a survivor on line 3; add a case to `tests/a.test.ts` that kills it | scope `[src/a.ts]`; that mutant is `Killed` in the new report; equivalence holds |
| base fixture with `thresholds.break` just below the full score (state published to `mutation-state/full` only); PR job emulation where the PR removes the assertion that kills `src/c.ts`'s mutants | exit 1; re-run with no change: exit 1; the step summary shows the score below the threshold and main's score (from `remote/full`) above it |
| change `LIMIT` in `src/limits.ts` | forced `[src/c.ts:2-2, src/limits.ts]` (edited, plus residual via its importer test `tests/c.test.ts`); 1 invocation; success; no deferral |
| edit `tests/helpers/make.ts` | forced `[src/a.ts:1-5, src/b.ts:4-4]`; `vitest`/`node:assert` imports are packages, not open importers |
| edit `tests/helpers/fmt.ts` | forced `[src/c.ts:2-2]` |
| remove the alias from the harness config, edit `tests/helpers/fmt.ts` | `c.test` is an open importer; forced includes `src/c.ts:2-2`; `plan` lists `@app/c`; also deferral `global input changed: mutation-incremental.json` |
| helper nothing imports | deferral `support … has no resolvable importer` |
| edit `package-lock.json` | deferral `global input changed: package-lock.json` |
| budget overflow (support edit, `maxForcedMutants: 1`; `expectations.json` states `forcedCount` > 1) | support-forced entries dropped; budget deferral on every file that had forced mutants |
| edit only `src/types.ts` | 1 invocation (forced `[src/types.ts]`); `No tests were executed`; no deferral; exit 0; `pending_full=false`; next `plan` shows no change |
| edit `src/a.ts` and `src/types.ts` together | 1 invocation, forced `[src/a.ts, src/b.ts:4-4, src/types.ts]`; succeeds |
| edit `src/types.ts` and `tests/helpers/make.ts` together | 1 invocation, forced `[src/a.ts:1-5, src/b.ts:4-4, src/types.ts]`; succeeds |
| edit `src/index.ts` | forced `[src/index.ts]` plus the `src/a.ts` mutants killed by `tests/index.test.ts` (residual via its importer tests; exact ranges in `expectations.json`); success; the report's `files` has no `src/index.ts` entry |
| delete `tests/c.test.ts` alone | scope `[src/c.ts]`; `No tests were executed`; deferral `no reachable tests: src/c.ts`; gate: `src/c.ts` kills pending |
| add `src/u.ts` alone, run; then add `tests/u.test.ts` importing it, run | first run: no-tests, no deferral; second run: scope includes `src/u.ts`; its mutants appear |
| add `src/v.ts` alone, run; then import it from existing `tests/a.test.ts`, run | second run: scope includes `src/v.ts`; its mutants appear |
| delete `src/b.ts` and `tests/b.test.ts` together | scope `[]`; forced `[src/a.ts]` (whole file: scoped via b.test and residual-forced); `src/b.ts` mutants absent afterwards |
| `git mv src/c.ts src/d.ts` with its test updated | no exit 2; `src/d.ts` forced whole-file |
| `residual.mode: off` / `strict` with `maxForcedMutants: 1` (`expectations.json` states `forcedCount` > 1) | deferral `residual off: src/a.ts` on `src/b.ts` / residual forced despite exceeding the budget |
| threshold break (fixture variant with a survivor and `thresholds.break: 100`) | exit 1; state committed |
| threshold break, then re-run with no change | exit 1 again |
| threshold break, then lower `thresholds.break` in `stryker.config.json` | exit 0; `pending_full=true` |
| Stryker killed mid-run | exit 4; canonical files unchanged from the adopted baseline; re-run retries the same plan |
| Ctrl-C / terminal closed (SIGHUP) mid-run | exit 130; canonical files unchanged |
| time budget exceeded (shim sleeps past a small `maxMinutesPerInvocation`) | deferral `time budget exceeded`; exit 0 |
| full run where every test fails to import (shim exits 1 with `No tests were executed` and writes no report) | exit 4; nothing committed; the job emulation publishes nothing |
| full job, time budget: a main push defers `time budget exceeded` and publishes `inc`; the full job's catch-up adopts `mutation-state/full` | shim still slow: catch-up `pending_full=true`, full run, `mutation-state/full` has no deferrals / shim fast: catch-up `pending_full=false`, no full run |
| full job, green catch-up (continues the shim-fast time-budget row) | the job is green; `mutation-state/full` republished with the catch-up's state (no deferrals); the next main push adopts it with an empty change set |
| full job, full run breaks the threshold (survivor variant, `thresholds.break: 100`, after a `package-lock.json` edit) | the job is red; `mutation-state/full` published; the next main push has `pending_full=false` and no `full` job |
| full job, inherited pending: global edit ⇒ full run published; a later push's `incremental-main` adopted the pre-full `inc` state (`pending_full=true`) | catch-up adopts the full state; `pending_full=false`; no second full run |
| concurrent run / `break-lock` | exit 3 / lock removed |
| `publish-state --kind inc`, fresh clone, `fetch-state`, `run --also-state …` | warm plan; no `no usable state` deferral |
| local state carries a `global input changed` deferral; `mutation-state/full` holds a later full run over that change | full state adopted (no deferrals), local deferral gone |
| un-rebased PR whose own state has no deferrals; newer full state on main | PR keeps its own state (no re-run of main's changes) |
| seed / `--replace` | deferral `seeded from …` / old pair in `replaced/<ts>/` |
| **equivalence** | after each edit scenario, a fresh non-incremental run on the same tree; mutants matched by `(file, mutatorName, location, replacement)`; every kill the gate classifies `verified` is a fresh kill (a fresh `Timeout` is re-run once); the number of compared kills is recorded so an empty comparison is visible |
| **equivalence negative control** | scenario `e-flip` (README) with the residual step disabled by a test-only switch: equivalence fails for exactly the listed mutant |
| **gate** | edit committed, `metareview review pr-ready --mutation-report …` before re-running: one stale finding with cause `src/a.ts`; after re-running: none; enforce: blocking; revert without re-running: a stale finding with cause `src/a.ts` blocks (a new fingerprint, since the
digest differs; recurrence of the same fingerprint is covered by the Go tests); after a `package-lock.json` edit and re-run: pending finding; commit a `LIMIT` edit in `src/limits.ts`, pr-ready before re-running: one stale finding with cause `src/limits.ts`; after re-running: no stale finding, and a pending finding is present (a new fingerprint, since the report changed) |

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
  builtins, open importers incl. a test that is itself one); the planner replaying every
  `real/<scenario>/` against `expectations.json`; edited files forced whole-file and exempt from the
  budget; budget overflow deferring every forced file; step 2's reached-module scoping; deleted and
  renamed paths dropped; runner outcomes with a fake engine (success, not written,
  `No tests were executed` with and without kills in the selection, invocation 2 after a no-tests
  invocation 1, full-mode no-tests ⇒ exit 4, timeout, any failure ⇒ nothing committed, interrupt
  incl. SIGHUP); `score`/`thresholdBreak` computed from report and config (incl. a null score) and
  the threshold exit (below, equal, above, null score, no threshold); the plan `baseline` field; the
  Node-version warning (`v` prefix, `20` vs `20.11.0`, `2` vs `20.1.0`, aliases skipped); zero-mutant changed files using
  importer tests for `T_Y`; adoption via repeated `--also-state` (no-deferral candidates first, then
  smallest change set, then primary, then order; `run` copies, `plan` does not);
  the step-summary script (every exit path; `remote/inc` preferred over `remote/full`; a hash
  mismatch falls through, none ⇒ "main state unavailable"; "includes pending kills"; "previous
  state" on exit ∉ {0, 1}; "run step did not execute"); `publish-state`/`fetch-state` against a
  local bare remote (missing branch skipped, no shallow clone, explicit committer, unusable state ⇒
  no push and exit 0, rejected push ⇒ exit 2, the token passed through
  `GIT_CONFIG_*` appended after existing entries and never in argv, empty token ⇒ unset); the `changed-during-run` digest;
  commit ordering; lock and `break-lock`; seeding; exit precedence and the `GITHUB_OUTPUT` lines;
  config validation; and a static workflow test (triggers; workflow-level `permissions: {}`;
  `incremental-pr` has `contents: read`; only `incremental-main` and `full` have `contents: write`;
  every checkout has `persist-credentials: false`; `MUTATION_STATE_TOKEN` appears only on
  `fetch-state` and `publish-state` steps; checkout and setup precede `fetch-state` in every job;
  the step summary is `if: always()`; the full job runs the catch-up before `run --mode full`, saves
  and publishes a green catch-up, and saves the cache before publishing; every saved cache key is
  distinct; cache saves require the attestation; the catch-up's publish step requires `pending_full == 'false'`;
  no job-level `github.token` env; `N` in cache keys is `github.event.pull_request.number`; the
  `full` job's fail step (a shell step, with a fake-exit test: catch-up 0 with `pending_full=true`,
  full run 1 ⇒ red) reads the full-run
  step's `exit_code` when that step ran and the catch-up's otherwise; publish steps are not
  `continue-on-error`; `incremental-main`'s summary omits main's score;
  identical cache path lists; `continue-on-error`; outputs; `!cancelled()`; restore-key order;
  no `--threshold` flag anywhere; publish only on exit 0/1; empty exit code fails).
- A CI test recomputes the manifest's hashes (planner- and report-affecting template modules,
  fixture, expectations, script, and each committed `real/<scenario>/` tree) and fails on any
  difference.
- Go, 100% statement coverage (`make cover`): glob vectors; parser fields; unchanged existing
  fingerprints; attestation validation (each unattested reason); HEAD reader with `.gitattributes
  eol=crlf` and a symlink; `tracked:false` handling; new-path handling per category; each kill class
  and cause order including the residual cause and a changed zero-mutant `mutate` file; one finding per recorded cause and summed counts;
  Timeout never a kill; modes including invalid value and task-done under enforce; findings per mode,
  gate and severity; gremlins no finding; `Reconcile` supersedes absent `mutation:*` rows of the same
  scope, target and engine only on runs with reports (a gremlins-only run leaves stryker rows), never on a run without reports, never on an `overridden` row (override granted → runs with and
  without reports: the override still holds, and a stale-only escalation lifted by it stays
  lifted; stale-only escalation → re-run with fresh reports supersedes the rows → override request
  and grant with `--escalation` → the escalation is lifted), skips the fix branch under
  both conditions, reopens a recurring fingerprint, and renders superseded override lines; log
  section keyed by (file, cause); digest behaviour; escalation bounds (stale + non-stale blocker
  still escalates at `maxAttempts`); learning filter. Classification tests use the committed real
  reports plus derived fixtures (with `reportSha256` recomputed) for states a real report cannot
  produce.

## 8. Known residuals

- Mutant-free code inside a changed module that also has mutants (a constant next to functions)
  affects tests outside its mutants' coverage; the residual rule reads that coverage, so it misses
  tests that reach only the constant (`strict` does not close this). A changed module with no
  mutants at all uses its importer tests instead (§5.4 step 4).
- Paths containing `, { } [ ] ( ) ! * ? :` cannot be passed to `--mutate` and exit 2 (e.g. Next.js
  App Router's `app/(auth)/[id]/page.tsx`); such projects keep those directories out of `mutate`.
- A new module with no test yet produces no mutants (no NoCoverage entries) until a test reaches it.
- Deleting a module together with its own test, with nothing else changed, leaves its old survivors
  in the report until the next run that invokes Stryker (the gate marks its kills stale).
- Computed specifiers (`import(variable)`) and files read by path at runtime are invisible to the
  import graph; support files with no importer defer everything, which covers the common fixture
  case.
- State outside the repo beyond the declared `runtime` inputs.
- The gate trusts the harness's attestation (G3).
- Stryker semantics are version-specific; the harness requires StrykerJS 10.x.
- On integration-heavy suites residual forcing may often exceed the budget and defer, and edited
  large files cost their full mutant count; the external trial measures both.

## 9. Documentation deliverables

- `docs/mutation-harness.md`: setup; config and defaults; which files to add to `ignore` and to
  gitignore (`<stateDir>/`); aliases and open importers; sizing the budget and time ceiling against
  the ≤ 2 min bar and the CI job timeout (setup, catch-up and the full run must fit together); pinning Node with a
  tracked `.nvmrc`; the state branches, `fetch-state`/`publish-state` (CI-only) and the cache fast
  path, and allowing `mutation-state/*` in branch rulesets; warm-starting a new worktree; that
  dependency bumps (global inputs) trigger one full run on main; that a threshold break turns every PR
  red until main is fixed (the step summary shows main's score); that editing a widely imported
  barrel or constants file often exceeds the budget and defers to a full run (prefer `import type`
  or `verbatimModuleSyntax` for type-only imports); that seeding helps local runs only (CI's first
  run is still a full run); that `incremental-pr` is not a merge-queue check unless `merge_group` is
  added; that main re-runs each merged PR's delta (PR caches are per PR); that re-running an old main
  workflow publishes an older, still valid state; to pass `<stateDir>/incremental.json` (not
  Stryker's `reports/mutation/mutation.json`) and every report on gated runs; that the step summary, logs and state are public on public
  repositories; that `MUTATION_STATE_TOKEN` is CI-only (local runs need no token); to limit the
  `mutation-state/*` ruleset bypass to GitHub Actions; that tuning-only config edits
  (`thresholds.break`, `budget`, `ignore`) are global inputs and cost one full run on main; that
  CI PR state is not gate evidence (run the harness locally before pr-ready); that a cancelled job
  commits nothing; that a PR which changes a global input (e.g. a lockfile bump) re-runs its whole delta
  from main's state on each push; the `MUTATION_STATE_TOKEN` wiring; that an enforced stale
  finding clears only on a later run that supplies reports (or by override), and that overrides on
  pending/unattested findings last only for that report and every mutation override is tied to the
  mode in its fingerprint; commit-before-rerun for pr-ready; reading
  the gate's freshness section; the planned 0.14.0 default enforcement. Linked from `USAGE.md`,
  `docs/quickstart.md` and the `--mutation-report` help text.
- Release notes word 0.13.0 as "harness plus freshness gate; the ≤ 2 min bar is pending the external
  trial".
- `docs/0.13.0-candidates.md` §1: next to the "prefer metareview's anchor-pin model" bullet, add that
  the transferable idea is scoped, attested binding (pins verify targeted fix claims, not a mutation
  score); add the external-trial follow-up (≤ 2 min p50 and the share of PR runs ending pending) and
  the survivor-severity follow-up (§3).
- `CHANGELOG.md`; `docs/ARCHITECTURE.md` (mutation freshness in the review model).
- `docs/mutation-harness.md` also covers: tests must not print environment or config values
  (summaries and logs are public); a red `incremental-main` on a global change triggers no full run
  for that commit (re-run it); repeated red `full` jobs (timeout or exit 4) never clear on their
  own; a PR whose own budget or time-budget deferral re-runs its delta from main on each push; a
  local `run --mode full` clears a PR's pending kills; the gate marks every kill stale after a
  zero-mutant `mutate` edit until the harness re-runs; superseded override requests disappear from
  `override list --pending`, and old grants on pending/unattested fingerprints accumulate under
  "Process Overrides"; private repositories without fork Actions run no PR job for forks; upgrading the harness (re-copy the template; the upgrade
  PR runs cold and green-pending; main pays one full run); that `mutation-state/full` means "main's
  latest state without deferrals" and may carry `mode: incremental` with an older `lastFullAt`;
  that a `full` job hitting its timeout will not clear on its own (raise the timeout or narrow
  `mutate`); editing the `push` trigger for a default branch other than `main`; optional
  `paths-ignore` mirroring `ignore` (keeping the `full` job's `needs` chain); that the state
  branches are as trusted as the repository's write collaborators; keeping `github.token` rather
  than a PAT for `MUTATION_STATE_TOKEN` (a PAT's pushes trigger other workflows).
- External-trial checklist (with the ≤ 2 min measurement): confirm on real GitHub Actions that
  `full` runs after a red `incremental-main` (outputs of a failed job) and that the URL-scoped
  auth header matches the remote.

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
11. Reviews use a pragmatic brief: real workflows and real edge cases, assume trust, no
    overengineering.
12. Edited source files are re-run whole (`--force`), because Stryker otherwise reuses kills in their
    unchanged regions.
13. Mutation state lives on change-based git branches (`mutation-state/inc`, `mutation-state/full`),
    with `actions/cache` as a fast path while its entries are within GitHub's 7-day window.
14. Validity and storage are change-based, never date-based.
15. The threshold rule is the same everywhere: a score below `thresholds.break` fails the job,
    on PRs too (revised after r10; the relative rule's main-reference machinery was removed as not
    worth its complexity). When main is below the threshold, PRs are red and the step summary
    says main is below it.

## 11. Amendment (r15, revised r16–r21; adoption contract)

Source: `docs/superpowers/specs/2026-09-23-mutation-incremental-adoption-contract.md` (decisions
K1–K9, user, 2026-09-23). This section is normative and overrides the sections it names; everything
not named here is unchanged from r14. Implementation plans read §§1–10 through this section.

### 11.1 Project verifier (K1; amends §5.1, §5.2, §5.6 step 7)

- Config `verify: {"command": [argv], "timeoutMinutes": <n or null>}` (optional). After `run`
  commits state (§5.6 step 7), in both modes, the harness runs it (no shell, cwd = top-level, own
  process group with §5.6's signal handling) once per view (§11.3), or once when no views are
  configured. Environment: the harness's own environment plus `MUTATION_REPORT=<stateDir>/incremental.json`,
  `MUTATION_ATTESTATION=<stateDir>/attestation.json`, and per view `MUTATION_VIEW=<name>`,
  `MUTATION_VIEW_PATTERNS=<JSON array>` and `MUTATION_VIEW_FILES=<JSON array of the resolved
  paths>` (all three unset without views), and `MUTATION_RUN_KIND=pr|other` (`pr` with `--pr`).
- The verifier reads the whole report; view scoping and waiver semantics are the project's (waivers
  are per mutant, so a mutant in several views is judged consistently). Kills covered by a deferral
  still show their previous status in the report. The attestation marks each deferral
  `"inherited": true|false` (§11.2) and splits `viewSummaries[view]` pending into `pendingCounted`
  and `pendingInherited`. A verifier enforcing a survivor bar treats **counted** pending kills as
  unverified on PR runs (`MUTATION_RUN_KIND=pr`) and ignores inherited ones, which main's `full`
  job owns; on other runs pending never fails the verifier (decision 2). The docs say so.
- Any verify failure is exit 1, the same class as a threshold break (state stays committed and is
  published): a non-zero exit, a command that cannot be spawned, or exceeding `timeoutMinutes`;
  stderr names each failing view and the reason. `thresholds.break` becomes optional; either check
  failing gives exit 1; a config with neither is valid and enforces nothing (docs).
- Not run when nothing was committed (a cold run with no `incremental.json`) and not run by `seed`.
  `verify` and `views` commands run only inside `run` and `plan`, never in `fetch-state` or
  `publish-state`; the static workflow test asserts `MUTATION_STATE_TOKEN` is absent from `run`
  steps. Verifier output lands in public logs and summaries (docs).
- Stated explicitly: **CI enforces the project's bar (threshold and/or verifier); the metareview
  gate judges only evidence freshness.** A gate-side survivor bar is out of scope for 0.13.0.
- Recommended waiver identity (docs): `(file, mutatorName, location, replacement)` plus the file's
  digest; mutant ids are report-local. The docs suggest a single-pass verifier that loops over the
  views in the attestation when per-view invocations are costly.

### 11.2 Pending on PRs (K2; amends §5.1, §5.7, decision 2)

- Config `pendingOnPr: "allow" | "full" | "full-on-global"`, default `allow` (decision 2 unchanged
  for projects that do not opt in). `residual.mode: "off"` together with `full-on-global` is exit 2
  (every residual edit would fail the PR).
- `run --pr` marks a PR run; the template passes it only when `pendingOnPr` ≠ `allow` (so under
  `allow` a verifier sees `MUTATION_RUN_KIND=other` and decision 2 holds). With `pendingOnPr` ≠
  `allow`, a PR run is **unbudgeted** (the forced-set budget is disabled, as in `strict`, so budget
  deferrals never arise for a PR's own changes) and uses `--max-minutes <m>` in place of
  `maxMinutesPerInvocation`; the time limit is then the only thing that can leave a PR's own work
  unrun. The sweep (`run --mode full`) is never `--pr`; full mode leaves no deferrals (a full run
  that finds no tests is exit 4, §5.6), so its verifier need not consider pending.
- **Counted and inherited deferrals.** A deferral in the run's resulting attestation is
  **inherited** only when (i) an identical `{reason, paths}` appears in the attestation of one of
  the run's `--also-state` directories (main's fetched `remote/inc` or `remote/full`; parsed,
  whether or not usable), **and** (ii) the run has not changed what the deferral is about: the path
  or runtime key named in its reason, and every path in `paths` other than `*`, has the same digest
  in this run's snapshot as in that attestation's `files`/`runtime` (a path absent from both counts
  as the same digest; reasons naming nothing, such as `no usable state` and `seeded from <report>`,
  need only (i)). Exceptions, always counted whatever (i) and (ii) say: a `no usable state`
  deferral added by a run whose own baseline is cold (a cold run executes nothing, so it must never
  pass on main's pending); and any deferral **produced by an invocation this run executed** —
  `time budget exceeded`, `blocked by deferred scope`, `no reachable tests` — because it describes
  this run's own unfinished work, even when main carries an identical entry. Every
  other deferral is **counted**, including deferrals carried from the PR's own cached state that
  fail (i) or (ii). Reasons and `paths` are canonical (fixed formats, byte-ordered paths) so the
  comparison is exact. **Inherited deferrals never suppress execution:** the run's scope, edited and
  forced work run as planned (the shortcut applies only to counted deferrals), so a PR's own changes
  are executed even while main is pending; their kills are then also covered by the inherited
  deferral and reported as `pendingInherited` ("executed, awaiting main's full run", not "never
  ran"). Each run with `--also-state` records `"inherited": true|false` on every deferral in its
  attestation (`false` on runs without `--also-state`; a deferral without the field, from an older
  attestation, reads as `false`). A kill covered by both a counted and an
  inherited deferral is counted (each kill lands in exactly one bucket). The step summary lists
  inherited deferrals as "inherited from main, cleared by main's full run".
- **Cause.** On exit 0 or 1 only, `run` appends to `GITHUB_OUTPUT` `pending_cause` and
  `pending_causes` (a JSON array of the counted `{reason, paths}`), computed from the plan's and the
  run's deferrals even when nothing is committed (a cold run with no `incremental.json` writes no
  attestation but still reports its counted `no usable state`). `pending_cause` is the first that
  applies of:
  1. `global` — a counted deferral with `paths: ["*"]` (global or runtime input, `no usable state`,
     a deleted or importer-less support file, an importer-less unclassified file, seeding);
  2. `unreachable` — a counted `no reachable tests: <p>` (the change removed the only test reaching
     code that has kills);
  3. `other` — a counted `residual off: <p>` (possible only under `allow`/`full`);
  4. `timeout` — a counted `time budget exceeded`, `blocked by deferred scope` (only ever follows a
     timed-out invocation 1, §5.6) or `forced set <n> exceeds budget <b>` (a PR run is unbudgeted,
     so this is only ever carried from main's budgeted state and becomes counted when the PR edits a
     named file): capacity effects, not code defects;
  5. `none` — only when no counted deferral exists (every counted reason falls in 1–4; a reason
     outside them is a harness bug and exit 2).
  Follow-up steps and jobs are gated on `exit_code` ∈ {0, 1}.
- **Routing.** Under `full`, any `pending_cause` ≠ `none` routes to the sweep. Under
  `full-on-global`, `global` and `timeout` route to the sweep and `unreachable` fails the
  `incremental-pr` job with each counted cause named in the step summary (`no reachable tests:
  src/x.ts` — add a test or remove the module; when the condition was carried from main, the
  summary says so). Any other routed-mode cause routes to the sweep. `global` takes precedence over
  `unreachable`, so a PR with both a lockfile edit and a removed only-test pays the sweep and is then
  judged by the project's bar. **A PR's verdict under `full-on-global` never depends
  on runner speed:** a timeout only makes the PR slower (it pays the sweep), never red; only
  `unreachable` needs a person.
- **Shortcut.** In a PR run with `pendingOnPr` ≠ `allow`, when the plan already contains a counted
  `["*"]` deferral, no invocation runs (the state is re-attested as in §5.6 step 6) and the cause is
  `global`.
- **Jobs (amends §5.7).** Under `full`/`full-on-global` the sweep runs in its own job:
  - `incremental-pr` (timeout ≥ setup (checkout, project setup, cache restore, `fetch-state`,
    summary) + 2 × (`<m>` + 1 min kill grace) + max(1, views) × (`verify.timeoutMinutes` + 0.5 min),
    and `verify.timeoutMinutes` must be set when `pendingOnPr` ≠ `allow` (else exit 2), so a job-level
    timeout cannot pre-empt the harness and turn a PR red on runner speed; concurrency `mutation-inc-${{ github.ref }}`,
    cancel in progress as before): step 3 passes `--pr --max-minutes <m>` and exposes `exit_code`,
    `pending_cause` and `pending_causes` (single-line JSON, capped at 50 entries with a count of the
    rest; routing reads only `pending_cause`; the workflow passes outputs to shell steps
    through `env:`, never `${{ }}` inside `run:`); the job succeeds when the cause routes to the sweep
    (the sweep job then decides; the summary says "verdict deferred to pr-full", including when the
    run exited 1 because a strict verifier counted the pending that the sweep will re-judge), fails on `unreachable`, and otherwise follows §5.7 step 6. Step 4's
    cache save of the incremental state runs only when `pending_cause` is `none` or `pendingOnPr`
    is `allow`.
  - `pr-full` (`needs: incremental-pr`; runs only when its `pending_cause` routes to the sweep and
    its `exit_code` ∈ {0, 1}; `permissions: contents: read`; timeout ≥ setup + one full sweep + max(1, views) ×
    `verify.timeoutMinutes`;
    concurrency `mutation-pr-full-${{ github.event.pull_request.number }}` with
    `cancel-in-progress: false`, so a running sweep is never cancelled and only the newest waiting
    one survives): checkout, setup, `run --mode full`; on exit 0/1 save the cache with key
    `mutation-pr-N-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}-full`
    (`N` = `github.event.pull_request.number`, restored by the existing `mutation-pr-N-` restore
    key), so a sweep that finishes after a newer push is adopted by the next push, which then
    re-runs only its own delta; fail when `exit_code` is empty or ≠ 0 (its run step is not
    `continue-on-error` in a way that hides this; a fake-exit test pins it). Skipped, it satisfies a
    required check, so **both `incremental-pr` and `pr-full` must be required checks** (a failing
    `incremental-pr` also skips `pr-full`).
- **Sizing and costs (docs).** `<m>` is sized so a timeout is rare (Keeper: ≥ 60 min); a breach costs
  a sweep, not a failure. Under `full` every PR exceeding `<m>` pays a sweep. A force-push during a
  running sweep lets that sweep finish (its state is reused) and queues one more if the new commit
  also needs a sweep. A PR rebased or not rebased during main's pending window may pay a sweep when
  its lockfile digest differs from main's (correct, but slower). A push whose `incremental-pr`
  finishes before a running sweep saves its cache re-plans from the older state (and may pay one
  more sweep); a PR that keeps timing out pays a sweep on every push until `<m>` is raised. Owners
  watch main's `full` job: while it is red, PRs stay green on inherited pending. Main's `full` job timeout
  (§5.7) likewise includes setup, the catch-up and 2 × max(1, views) × `verify.timeoutMinutes`; a
  project with many views uses a single-pass verifier to keep that term small.
- A local run never turns a PR green; no local state is imported into CI (contract option (b)
  rejected).

### 11.3 Views (K3; amends §5.2, §5.5, §6.1, §6.5, §6.6)

- One state whose `mutate` is unchanged; views are named pattern sets over it. Config `views`:
  `{"command": [argv]}` printing a JSON object `{"<name>": [patterns]}` on stdout, or
  `{"inline": {...}}` (Keeper uses `command`). Names match `[A-Za-z0-9._-]+`. A view matches a path
  per the §5.3 list rule (`!` entries applied). A views command that exits non-zero or prints
  anything but that shape is exit 2.
- **Read-time only.** Views never affect planning, budgets, invocations, adoption or validity. The
  views command is not a runtime input; a view-map change invalidates no kill.
- **Completeness (exit 2, at plan time before any invocation):** a view matching a path in `S` whose
  category is not `mutate`; an empty or duplicate name; or a `mutate` file in `S` that no view
  matches (deliberately stricter than "every file with mutants": a new file must be assigned before
  it has mutants). A bad map therefore also stops main's jobs; the PR job catches it first. Views
  may overlap; per-view counts are never summed.
- **Attestation** gains `views` (the resolved map, byte-ordered) and `viewSummaries`
  (`{name: {<Stryker status>: count, "pendingCounted": count, "pendingInherited": count}}`, pending =
  kills covered by a deferral, split by the deferral's `inherited` flag). `publish-state` never re-runs the views command.
- **Gate `--mutation-view <name>`** (repeatable): the name must appear in the map of every supplied
  attested report that has `views`; otherwise exit 2. An unattested report, or an attested one
  without `views`, is not scoped: it produces its usual findings under the view's fingerprint. Each
  view has its own classification, findings and counts (never a union). The gate accepts the flag
  repeated in one run (Keeper: 16). **Rendering:** one §6.6 "Mutation Evidence Freshness" section
  with a table row per view (verified, stale, pending counted, pending inherited, unbound,
  unattested) and one re-run list with a View column; findings stay one per view (ledger
  isolation) with the view named in the title (`Mutation evidence stale (<view>): <cause>
  changed`).
  With a view, fingerprints gain `:<view>` after `<engine>`
  (`mutation:stale:<mode>:<engine>:<view>:<cause>:<cause8>`, likewise for pending and unattested).
- **Ledger with views (amends §6.5, and `openForRun` for these three prefixes):** a run with views
  counts as open, and may supersede, only rows whose view is in its `--mutation-view` set; a run
  without views counts and supersedes all rows of these prefixes, viewed or not. On a run that
  supplies at least one attested report of the same engine carrying `views`, an `open` or
  `override-pending` row **from an earlier run**, whose fingerprint this run did not produce and
  whose view is absent from the maps of all such reports, and which has the same scope, target
  (`sameRunTarget`) and engine as the run, becomes `superseded` (a renamed or
  removed view never strands a blocker). Rows without a view are outside this sweep, and runs
  supplying no report with `views` never apply it. Implementations store the view as a structured
  field on the ledger row rather than re-parsing it from the fingerprint. Adopting views leaves any
  earlier unviewed rows to be cleared by one gate run with reports and no `--mutation-view` (cutover
  docs). `overridden` rows are left as in §6.5. An override on one view's
  fingerprint does not cover the same cause under another view (docs).

### 11.4 Edited files (K5; amends §5.4 steps 1 and 4, Normalise, §5.6 step 4, decision 12)

- Config `editedFiles: "residual" | "whole"`, default `"residual"`. `whole` is r14's rule.
- In `residual` mode, for each changed `mutate` file Y with an entry in `R.files` and at most 1 MiB
  (larger files use `whole`):
  1. **Hunks are content-based:** a minimal Myers line diff (deterministic tie-breaking) of
     `R.files[Y].source` (the attested source embedded in the report) against Y's current bytes,
     both with line endings normalised to LF and a leading BOM removed; never commits or `git diff`.
     Unchanged lines map old line numbers to new ones. Node and Go share conformance vectors for the
     diff, as for the glob dialect.
  2. **Placement:** Y goes whole-file into the **unforced scope** (invocation 1). Stryker rebuilds
     every mutant of Y, re-runs those whose code changed (changed-hunk and enclosing mutants alike;
     a mutant is only created inside its full span, so hunk ranges alone would drop mutants spanning
     the edit) and reuses unchanged-region results per F1. Scope is never budgeted, so changed-hunk
     mutants are mandatory and budget-exempt. The Normalise rule "a file in both scope and forced is
     forced whole-file" does not apply to Y.
  3. **Closure by coverage, not proximity:** a mutant of `R` in Y **intersects** a hunk when its old
     line range overlaps the hunk's old-side range, or, for a pure insertion, contains the insertion
     point. `T_H` = union of `coveredBy` of the intersecting mutants; plus Y's importer tests when
     some hunk intersects no mutant of `R` (a constant, static code, a pure addition outside every
     mutant). Every Killed mutant (any file; Y's own mapped to current lines) whose `killedBy`
     intersects `T_H` is forced in invocation 2 as ranges; these are budget-subject (overflow defers
     them as in §5.4). With `residual.mode: "off"` the closure forces nothing and records
     `residual off: <p>` on the files it would have forced.
  4. **Prerequisite:** `disableBail: true` (§11.5); `allowBail` is refused in this mode.
- A changed `mutate` file with no entry in `R.files` (new, or no mutants) is handled as in r14.
- Step 4's cross-file residual uses `T_H` for Y in `residual` mode.
- **Stated plainly:** `residual` widens the §8 residual relative to `whole` (a behaviour change that
  reaches same-file kills only through paths no covering test of the changed hunk exercises). It is
  accepted because the same class is already accepted for cross-file kills and full runs backstop
  it.
- **Gate:** unchanged in this respect; a changed file's own kills are stale by cause 1 (stricter than
  the planner; clears when the harness re-runs).

### 11.5 Static code and bail (K4, K6; amends §5.2 validation, §5.4 step 4, §6.3)

- **Out-of-mutant hunks, both modes (K4):** a hunk (§11.4.1) intersects no mutant of `R` when, by
  the §11.4.3 rule, no mutant of any status intersects it. In `whole` mode such a hunk adds Y's
  importer tests to `T_Y`; in `residual` mode §11.4.3 already does. **Gate mirror (both modes):**
  using the attested source embedded in the report and the shared diff, a changed `mutate` file
  with such a hunk is a blanket cause, like a zero-mutant file (§6.3). This is conservative and
  noisy for comment or import edits until the harness re-runs (docs).
- **Bail (K6):** config validation exits 2 unless the Stryker config has `disableBail: true` or the
  harness config sets `allowBail: true`; `allowBail: true` with `editedFiles: "residual"` is exit 2.
- `statically_tested`-style pins map to metareview anchor pins once R3 (prove cannot verify non-Go
  pins) is fixed (follow-up, not 0.13.0 scope).

### 11.6 State version, seeding, cutover, meta-artifacts (K7–K9; amends §5.4 Usable, §5.5, §5.8, §9)

- Attestation gains `stateVersion` (integer, starts at 1). **Usable** requires `stateVersion` equal
  to the harness's; `toolVersion` becomes informational (§5.5's first bullet reads "`stateVersion`"
  where it says `toolVersion`). `stateVersion` is bumped only when planning or attestation
  semantics change; a Node test fails when the planner's golden outputs change without a bump. An
  attestation without `stateVersion` is a mismatch (cold). The gate never reads `stateVersion`.
- `seed --from <report>` is repeatable. Tests are re-keyed by (file, name) and renumbered, mutants are renumbered (in byte order of
  file, then location), and
  `killedBy`/`coveredBy` are remapped; a path in
  several reports must have the same `source` (else exit 2); a mutant in several reports, matched by
  `(file, mutatorName, location, replacement)`, is merged (status by precedence Killed > Timeout >
  Survived > NoCoverage > others, i.e. Killed if killed in any, test lists
  unioned). `seed` does not run `verify`.
- Docs add: the cutover sequence (advisory; adopt with one state and views; `pendingOnPr` stays
  `allow` until `mutation-state/full` exists, since before then every PR run is cold and, under
  `full`/`full-on-global`, pays a full sweep; the first main push runs the full sweep and publishes;
  existing reports show `unattested`, advisory, until then; enforce only after
  `mutation-state/full` exists; 0.14.0 release notes repeat it); meta-artifact classification
  (files tests import or read → `support`; tooling-only catalog/decomposition JSON and docs →
  `ignore`; `global` only for inputs everything depends on; `tests/**` → `test`,
  helpers/factories → `support`; files read by path are invisible to the import graph).

### 11.7 Verification additions (amends §7)

- Fixture: a `views` map with two overlapping views covering every `mutate` file; `disableBail:
  true`; `expectations.json` keyed by `editedFiles` mode, the r14 rows being the `whole`-mode rows;
  `src/e.ts` with two independent multi-line functions `inc` (lines 1–4) and `dec` (lines 6–9),
  tested by `tests/e-inc.test.ts` and `tests/e-dec.test.ts` respectively, so the residual closure is
  narrower than the whole file.
- §7.1 rows: `residual` vs `whole` for the line-2 and line-3 edits of `src/a.ts`; a one-line edit
  inside `inc` (residual: `dec`'s kills reused, `inc`'s block mutant still present in the report,
  closure forces only mutants killed by `e-inc` tests); a pure-deletion hunk; an edit to a line with
  no mutant (importer tests added); a residual negative control (README `e-flip-residual`, naming the concrete edit to `inc` and the
  mutant of `src/a.ts` or `src/e.ts` whose kill it flips: the
  closure disabled by a test-only switch makes equivalence fail for exactly the listed mutant); a
  view pattern outside `mutate`, an unviewed `mutate` file and a failing views command (exit 2);
  `verify` failing for one view (exit 1, view named) and not run on a cold run; PRs under
  `full-on-global` with a lockfile edit (shortcut, `pr-full` sweep), with a time-budget deferral
  (`timeout` → `pr-full` sweep, never red for speed), with `blocked by deferred scope` (`timeout` →
  sweep), editing a file named in main's carried budget deferral (`timeout` → sweep), a `pr-full`
  sweep that exits 1 (job red, cache saved, next push does not repeat the sweep) or 4 (job red, no
  cache save), a PR test edit that times out on a scope identical to main's carried timeout
  deferral (counted, `timeout` → sweep), with a deleted only-test (`unreachable` → `incremental-pr` fails, cause named), with inherited deferrals only (no
  failure, summary lists them, the PR's own edits executed; a strict survivor-bar verifier still
  exits 0), with a counted pending kill under a strict verifier (exit 1 on a `--pr` run, exit 0
  otherwise), with a lockfile edit
  while main's `inc` carries a lockfile deferral (counted, cause `global`, full run), and on a cold baseline after a `stateVersion` bump (cause
  `global`, full run); a PR under `full` whose `pr-full` sweep succeeds (cache saved, next push re-runs
  only its delta), and a push during a running sweep (the sweep is not cancelled; the next push
  adopts its state); the gate's `--mutation-view` scoping (repeated flags in one run, one compact
  section with a row per view), per-view fingerprints, cross-view isolation,
  a renamed view superseding its rows (and a run with no `views`-carrying report
  superseding nothing), and an unattested report under a view keeping its blocker.
- §7.2 Node tests: the line diff (shared vectors), EOL/BOM normalisation, old→new mapping, pure
  insertions and deletions, `T_H`, the scope placement of Y, the 1 MiB fallback; counted vs
  inherited deferrals incl. the digest condition (unchanged digest ⇒ inherited, changed ⇒ counted,
  absent on both sides, nameless reasons, a cold run's `no usable state` always counted), canonical
  reasons, the precedence of counted over inherited per kill, the recorded `inherited` flag
  (`false` without `--also-state`), and
  `pending_cause` (only on exit 0/1); `pendingCounted`/`pendingInherited`; `MUTATION_RUN_KIND`; the shortcut; `--pr`/`--max-minutes`; `pending_cause` precedence and `pending_causes`;
  the static workflow test for `pr-full`
  (needs, gating on `pending_cause` per mode and `exit_code`, `cancel-in-progress: false`, cache key,
  read-only permissions); config validation (`allowBail` with `residual`, missing `disableBail`, `residual off` with
  `full-on-global`); views shape, names and completeness; `viewSummaries`; `verify` environment,
  per-view invocation, spawn failure and timeout (exit 1); `stateVersion` and its golden-output
  guard; seed merge and re-keying. Go tests: `--mutation-view` scoping and exit 2 on an unknown
  name; per-view fingerprints, view-aware `openForRun` and supersede, renamed views; the
  out-of-mutant-hunk blanket cause with the shared diff vectors.

### 11.8 Decisions (amends §10)

- Decision 2 is amended: projects may opt into `pendingOnPr: "full" | "full-on-global"`.
- Decision 12 is revised: edited files use `editedFiles`, default `residual` (§11.4); `whole` is
  available.
16. CI enforces the project's bar (threshold and/or verifier); the gate judges freshness.
17. One state per repository; per-unit evidence comes from overlapping, read-time views.
18. No local state is imported into CI.
19. A demanding consumer's adoption is gated by the adoption spec's viability measurement
    (contract §3 as amended by §7: the ≤ 20% bar applies to the mode actually adopted; under
    `full-on-global`, to the share of PR runs that pay a sweep).
20. Only deferrals found in main's published state, about inputs the run has not changed, are
    inherited; everything else a PR run carries counts toward its outcome.
21. Under `full-on-global` a PR's verdict never depends on runner speed within the harness's own
    limits: a timeout routes to the full sweep; only `no reachable tests` fails the PR (user and the
    Keeper agent, 2026-09-23). The one remaining speed-dependent bound is the `pr-full` job timeout
    (full mode has no per-invocation limit), sized like main's `full` job; an overrun is red and
    re-runnable, never silently green.
