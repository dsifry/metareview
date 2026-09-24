# metareview task-done context

Run ID: `mrv-20260924-051035853973000-task-done-2026-09-23-mutation-incremental-2b-views-6f8a9d6c`

## Task

# Mutation-incremental 2b — views, out-of-mutant hunks, gate rows — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the Go freshness gate for 0.13.0. Add §11.3 views (`--mutation-view`, per-view classification, fingerprints and ledger isolation), add the §11.5 out-of-mutant-hunk blanket cause, and prove the §7.1/§11.7 gate rows on real Stryker reports.

**Architecture:**
- `internal/mutationfresh` gains a Go port of the harness's line diff (`diff.go`, run on the shared `diff-vectors.json`).
- `Classify` keeps one class per kill, then tallies it into the report total and into every view whose patterns match the kill's file.
- `Build` derives findings and the section per view when views are requested, and exactly as today when they are not.
- The findings ledger stores the view as a structured field. `openForRun`, the supersede rule and the rename sweep read that field.

**Tech Stack:** Go 1.26 (module `github.com/dsifry/metareview`), git, and the existing Node e2e (`tests/e2e-mutation-incremental.mjs`, real Stryker 10 and Vitest 4).

**Spec:** `docs/superpowers/specs/2026-09-23-mutation-incremental-design.md` (r21): §11.3 (views, gate and ledger), §11.5 (gate mirror of out-of-mutant hunks), §11.7 (the Go test list and the §7.1 gate rows), §11.2 (`inherited`, for the pending split in the view table).

**Plan series:** 1a–1c (harness, done) → 2a (gate, done) → **2b (this plan)** → release checks.

## Global Constraints

- **Coverage and lint:**
  - Every gated Go package stays at 100% statement coverage.
  - `go vet ./...` and `gofmt` are clean.
  - Coverage is measured with `go test -cover` per package; `make cover` stops at golangci-lint on this machine (the Plan 1a ruling).
- **Without `--mutation-view`, a run is byte-identical to 2a's:** same fingerprints, titles, section text and pr-ready digest. The new fields are `omitempty`.
- **The line diff must equal the harness's on `testdata/mutation-incremental/diff-vectors.json`,** including `maxEditDistance`.
- **`--mutation-view` usage errors exit 2:**
  - the name is invalid (not `[A-Za-z0-9._-]+`) or empty;
  - the flag is given without `--mutation-report`;
  - the name is missing from the `views` map of any supplied attested report that has one.
- **Review stance (user):** real workflows, assume trust. Block only for a credible normal-usage failure; record hardening as follow-ups.

## Review Focus

1. **A run without views after views are adopted.** It must count and supersede every freshness row, viewed or not, per §11.3. Test: `TestUnviewedRunSupersedesViewedRows`.
2. **A report attested before views existed (no `views` key), run with `--mutation-view core`.** It is not scoped: every kill counts under `core`, and the fingerprints carry `:core`. Test: `TestViewOnReportWithoutViewsIsUnscoped`.
3. **Overlapping views.** A kill in `src/c.ts` counts under both `core` and `edge`. The section never sums views, and the header totals stay the report totals. Test: `TestViewsOverlapAndAreNotSummed`.
4. **A comment-only edit outside every mutant span.** It stales every kill as a blanket cause (conservative, as the spec says). Deleting a mutate file keeps 2a's behaviour. Tests: `TestOutOfMutantHunkIsABlanketCause` and `TestDeletedMutateFileIsNotBlanket`.
5. **A renamed view.** Its old rows are superseded by the next run whose reports carry `views`. A run whose reports carry no `views` supersedes nothing that way. Test: `TestRenamedViewSupersedesItsRows`.

---

### Task 1: Go line diff and the out-of-mutant-hunk blanket cause (§11.5)

**Files:**
- Create: `internal/mutationfresh/diff.go`, `internal/mutationfresh/diff_test.go`
- Modify: `internal/mutationfresh/classify.go` (`changedPaths` keeps current content for mutate files; `causeFinder` sends out-of-mutant files to `blanket`)
- Test: `internal/mutationfresh/classify_test.go`

**Interfaces:**
- Produces:
  - `lineDiff(oldText, newText string) ([]hunk, bool)` (false above `maxEditDistance`);
  - `type hunk struct{ OldStart, OldEnd, NewStart, NewEnd int }`;
  - `hunkIntersects(start, end int, h hunk) bool`.

- [ ] **Step 1: Write the failing vector test.** It loads `../../testdata/mutation-incremental/diff-vectors.json`, asserts `maxEditDistance == 2000`, and for every case compares `lineDiff(old, new)` hunks with the case's `hunks` (nil equals empty). It also checks one case above the limit: 2001 distinct replaced lines return `false`. Then `TestHunkIntersects` pins these rows:

  | Hunk | Mutant span | Intersects? |
  |---|---|---|
  | replace [2,2] | [1,5] | yes |
  | replace [2,2] | [3,3] | no |
  | pure insertion after 5 | [1,5] | no |
  | pure insertion after 2 | [1,5] | yes |

- [ ] **Step 2: Run it.** `go test ./internal/mutationfresh/ -run 'TestLineDiff|TestHunk'`. Expected: FAIL, `lineDiff` undefined.
- [ ] **Step 3: Port `lib/diff.mjs` line for line.**
  - `normalizeText` strips one leading BOM, then CRLF/CR become LF.
  - `splitLines`: empty text gives no lines, and one trailing empty line is dropped.
  - Myers with the same tie rule: insertion when `k == -d || (k != d && v[k-1] < v[k+1])`.
  - `trace` keeps the `v` slice for diagonals `-d-1..d+1` before step d.
  - Backtrack and hunk assembly are identical to the JS.
  - The header comment names the JS twin and the shared vectors.
- [ ] **Step 4: Run it.** Expected: PASS.
- [ ] **Step 5: Write the failing classify tests.** They use `realCase(t, "full")`.
  - `TestOutOfMutantHunkIsABlanketCause`, sub-case "e.ts": replace `src/e.ts` line 5 (the blank line between `inc` and `dec`) with `// between`. Expect `Stale == 24` and `causes(f)["src/e.ts"] == 24`.
  - Same test, sub-case "a.ts": append `// trailing note\n` after line 5 of `src/a.ts`. This is a pure insertion after the block mutant [1,5], so it intersects nothing. Expect `Stale == 24` with cause `src/a.ts`.
  - The existing `TestClassifyRealReportsEditedSourceStalesItsKillsAndTheirCoverageClosure` (an edit inside mutants) stays at 17. That is the negative control.
  - `TestDeletedMutateFileIsNotBlanket`: remove `src/c.ts`. Expect exactly its own 3 kills plus its coverage closure, the same as 2a. Record the count you observe on the real report in the test comment, and check it by hand against the report: `c.ts`'s mutant is covered only by the `c.test` ids.
- [ ] **Step 6: Run them.** Expected: the e.ts and a.ts sub-cases FAIL, with 4 and 17 stale instead of 24.
- [ ] **Step 7: Implement.**
  - `changedPaths` returns `map[string]change` where `change` gains `data []byte; present bool`, filled from `entries[p]`.
  - In `causeFinder`, the `mutate && len(mutants) > 0` branch goes to `blanket` when `c.present && outOfMutant(d.Files[p], c.data)`.
  - `outOfMutant(file, current)` runs `lineDiff(file.Source, string(current))`:
    - false (over the limit) → true;
    - a hunk with no mutant `hunkIntersects` → true;
    - otherwise → false.
  - Mutant spans use `StartLine` and `EndLine`, which are old lines, because `Source` is what Stryker read.
- [ ] **Step 8: Run the package.** `go test -cover ./internal/mutationfresh/`. Expected: PASS at 100.0%.
- [ ] **Step 9: Commit.** `feat(mutationfresh): an edit outside every mutant is a blanket cause (§11.5)`
- [ ] **Step 10: Fix the low finding carried over from 2a's cross-shard review (spec §6.3, "present paths").**
  - **The bug:** in working-tree mode, a tracked file removed with plain `rm` is still listed by `ls-files -c`. `changedPaths` then sees an unattested path whose digest is `absent`, and that stales every kill until the deletion is staged.
  - **Failing test:** `TestDeletedUnattestedPathIsNotAChange`.
    - On `realCase(t, "full")`, commit a new support file `tests/helpers/old.ts`, so it is unattested, then remove it with `os.Remove`.
    - Expect `Stale == 0` and `Verified == 24`.
  - **Run it.** Expected: FAIL (24 stale).
  - **Implement:** in `changedPaths`, skip (`continue`) an unattested path that is absent from `entries`.
  - **Run** `go test -cover ./internal/mutationfresh/`. Expected: 100.0%.
  - **Commit.** `fix(mutationfresh): a removed unattested path is not a change`

### Task 2: Views in classification, findings and the section (§11.3)

**Files:**
- Modify: `internal/mutationfresh/{attest,classify,build}.go`
- Create: `internal/mutationfresh/views.go`, `internal/mutationfresh/views_test.go`

**Interfaces:**
- Consumes: `MatchList(path, patterns)` (glob.go), the §5.3 list rule with `!`.
- Produces:
  - `Attestation.Views map[string][]string` (`json:"views"`).
  - The tally fields move into an embedded `Tally{Verified, Stale, Pending, PendingInherited, Unbound, Unattested int; Causes []CauseCount; ReRun []ReRunRow}`. `ReportFreshness` embeds it, and the JSON names are unchanged (`pendingInherited,omitempty` is new).
  - `ReportFreshness.Views []ViewFreshness` (`json:"views,omitempty"`), and `ReportFreshness.ViewNames []string` (`json:"-"`, the names in the report's map; nil when it has none).
  - `type ViewFreshness struct{ View string; Scoped bool; Tally }`.
  - `Classify(r, content, views []string)`.
  - `Build(reports, content, mode, views []string)`.
  - `CheckViews(reports []mutation.Report, views []string) error` returns `*ViewError` (a usage error).
  - `findings.Input.View` is set on every freshness finding when views are requested.

- [ ] **Step 1: Write the failing tests.** All on `realCase(t, "full")`, whose map is `core: src/**/*.ts` and `edge: src/c.ts, src/e.ts`.
  - `TestViewsOverlapAndAreNotSummed`:
    - `Classify(..., []string{"core","edge"})` on an unchanged tree.
    - `Views` is `[core{Verified 24}, edge{Verified 7}]`, both `Scoped`, and the report total is `Verified 24`.
    - Then edit `src/c.ts` line 2. `core` has `Stale 3`, `edge` has `Stale 3`, and the report total is `Stale 3`.
  - `TestViewOnReportWithoutViewsIsUnscoped`: delete `views` from the attestation in the test's copy, and re-hash nothing (the attestation isn't hashed). Expect `Views[0] = {core, Scoped false, Verified 24}`.
  - `TestUnattestedReportUnderAView`: remove `attestation.json`. Expect `Views[0].Unattested == 24`.
  - `TestPendingSplitsCountedAndInherited`: a synthetic attestation with two deferrals on `src/c.ts`, one `inherited: true` and one `false`. The kill is counted. With only the inherited deferral, `PendingInherited == 3` and `Pending == 0`. Here `Pending` means counted, so 2a's meaning is unchanged for reports without inherited deferrals.
  - `TestCheckViews`, with these rows:

    | Views requested | Result |
    |---|---|
    | none | nil |
    | `["core"]` | nil |
    | `["nope"]` | `*ViewError` naming `nope` and the report |
    | `["bad name!"]` | `*ViewError` |
    | `[""]` | `*ViewError` |
    | `["core","core"]` | nil (duplicates collapse) |
    | any views, unattested report | nil |
    | any views, attested report without `views` | nil |
  - `TestBuildWithViews`:
    - Stale fingerprint `mutation:stale:advisory:stryker:edge:src/c.ts:<8>`, title `Mutation evidence stale (edge): src/c.ts changed`, `Input.View == "edge"`.
    - One finding per view, and none summed across views.
    - Pending and unattested get `:<view>` after the engine.
    - The section is one table plus a re-run list with a View column. The table has a header row and one row per view: `| View | Verified | Stale | Pending counted | Pending inherited | Unbound | Unattested |`.
    - Assert the exact section text for the c.ts edit.
  - `TestBuildWithoutViewsIsUnchanged`: the existing 2a golden assertions still pass. Keep them untouched; this case needs no new assertion.
- [ ] **Step 2: Run the tests.** Expected: FAIL (compile errors on the new signatures).
- [ ] **Step 3: Implement.**
  - **Classify:**
    - Compute `class, cause` per kill as today, into a `kill{file, class, cause}` list.
    - Add each kill to `out.Tally` and to each requested view's tally.
    - A view's tally takes a kill when the report has no `views` map (`Scoped=false`), or when `MatchList(file, att.Views[view])` matches.
    - An unattested report gives every view `Unattested = total kills`.
  - **Deferral:** `deferral(deferrals, file)` returns `none|counted|inherited`. Counted wins when both cover the kill (§11.7).
  - **Build:** calls `freshnessFindings` once per "slice": the unviewed slice (view "") when `views` is empty, else one slice per view.
    - A slice view is `""` or a name, and the fingerprint adds `":"+view` only when the view is non-empty.
    - The title adds ` (<view>)` after `stale`, `pending` and the attestation noun: `Mutation report has no attestation (<view>)`.
  - **Section without views:** unchanged.
  - **Section with views:**
    - The same header bullets: mode, reports, deferrals, not verifiable. The "Kills:" bullet keeps report totals.
    - Then `### Views`, followed by the table.
    - Then `### Re-run list`, whose columns are `| View | File | Cause | Stale kills |`.
- [ ] **Step 4: Run the tests.** `go test -cover ./internal/mutationfresh/`. Expected: PASS at 100.0%. Update the 2a callers of `Classify`/`Build` to pass `nil`.
- [ ] **Step 5: Commit.** `feat(mutationfresh): per-view classification, findings and section (§11.3)`

### Task 3: View-aware ledger (§11.3 ledger paragraph)

**Files:**
- Modify: `internal/findings/findings.go`
- Test: `internal/findings/views_test.go`

**Interfaces:**
- Produces:
  - `Input.View`, `Record.View string` (`json:"view,omitempty"`), copied in `normalize`.
  - `Options.MutationViews []string`.
  - `Options.MutationViewMaps map[string][]string` (engine → union of view names in that engine's attested reports carrying `views`; the key is absent when no such report exists).
  - `openForRun(records, run, options)`.

- [ ] **Step 1: Write the failing tests.** They use the existing findings test helpers: a temp root, `Reconcile` twice with run ids r1 and r2, and the same target and scope.
  - `TestViewedRunCountsOnlyItsViews`:
    - r1 produces a `core` stale row and an `edge` stale row, both blocking.
    - r2 runs with `MutationViews ["core"]` and reproduces neither.
    - Expect: `core` superseded, `edge` still open but **not** in r2's `OpenFindings`, and `OpenBlockingCount == 0`.
  - `TestUnviewedRunSupersedesViewedRows`: after r1 as above, r2 has `MutationEngines ["stryker"]` and no views. Expect both rows superseded.
  - `TestRenamedViewSupersedesItsRows`:
    - r1 has an `edge` row.
    - r2 has `MutationViews ["core"]` and `MutationViewMaps {"stryker": ["core","periphery"]}`. Expect `edge` superseded.
    - Variant: r2 with `MutationViewMaps` nil keeps `edge` open.
    - Variant: an unviewed row is untouched by the sweep.
    - Variant: an `overridden` row is untouched.
  - `TestOverrideOnOneViewDoesNotCoverAnother`: the fingerprints differ, so a grant on `core`'s leaves `edge`'s open. This pins the documented behaviour.
- [ ] **Step 2: Run them.** Expected: FAIL (compile).
- [ ] **Step 3: Implement.**
  - `inViewScope(record, options)` returns true when the fingerprint is not a freshness fingerprint, or when `len(options.MutationViews) == 0`, or when `slices.Contains(options.MutationViews, record.View)`.
  - `openForRun` keeps a record only when `Blocks && sameRunTarget && inViewScope`.
  - `supersedesFreshness` adds `inViewScope`, OR the rename sweep. The sweep applies when all of these hold:
    - `record.View != ""`;
    - `names, ok := options.MutationViewMaps[engine]` exists;
    - `!slices.Contains(names, record.View)`;
    - the status is open or override-pending;
    - `sameRunTarget`;
    - `!current[fp]`;
    - `record.RunID != run.ID`.
  - `activeExisting` is unchanged.
- [ ] **Step 4: Run the package.** `go test -cover ./internal/findings/`. Expected: PASS at 100.0%.
- [ ] **Step 5: Commit.** `feat(findings): view-aware freshness rows, supersede and rename sweep (§11.3)`

### Task 4: `--mutation-view` on the three gates

**Files:**
- Modify:
  - `cmd/metareview/main.go`: the flag on task-done, epic-ready and pr-ready, the usage lines, and `mustMutationViews`;
  - `internal/reviewers/mutation.go`: `LoadMutationContext(root, paths, views, scope, head)`, `Views()`, `ViewMaps()`;
  - `internal/{taskdone,prready,epicready}/review.go`: `Options.MutationViews`, which is passed through and into `findings.Options`.
- Test:
  - `cmd/metareview/views_test.go`;
  - the existing `mutationctx_test.go` in each gate (the new argument).

**Interfaces:**
- Consumes: `mutationfresh.CheckViews` and `Build(..., views)` from Task 2, and `findings.Options.MutationViews` and `MutationViewMaps` from Task 3.

- [ ] **Step 1: Write the failing CLI tests.** They use the package's `gitRepo` and run helpers, as `freshness_test.go` does, and copy a real report as in `realCase`. For each gate:
  - `--mutation-view core` without `--mutation-report`: exit 2, stderr names the flag.
  - `--mutation-view nope` with the real `full` report: exit 2, stderr contains `nope`.
  - `--mutation-view 'bad name'`: exit 2.
- [ ] **Step 2: Run them.** Expected: FAIL. Today this is `Unknown option: --mutation-view`, which is also exit 2, so assert on the stderr text to make the failure real.
- [ ] **Step 3: Implement.**
  - **Flag parsing:** `--mutation-view` appends to `options.MutationViews` for all three gates.
  - **`mustMutationViews(reportPaths, views)`** runs before each review's `options :=`, after `mustFreshnessMode()`:
    - views without reports → exit 2;
    - `mutation.LoadAll` then `mutationfresh.CheckViews` → on error, exit 2.
  - **reviewers:**
    - `LoadMutationContext` passes views to `Build`.
    - `MutationContext.Views` is `[]string` (`json:"views,omitempty"`), so it enters pr-ready's digest only when used.
    - `ViewMaps()` builds engine → sorted unique names from `Freshness[i].ViewNames` where `Attested && ViewNames != nil`.
  - **Gates:** `MutationViews: ctx.Views` and `MutationViewMaps: ctx.ViewMaps()` go into `findings.Options`, next to `MutationEngines`.
- [ ] **Step 4: Run.** `go test -cover ./cmd/metareview/ ./internal/reviewers/ ./internal/taskdone/ ./internal/prready/ ./internal/epicready/`. Expected: PASS, each at 100.0%.
- [ ] **Step 5: Commit.** `feat(gates): --mutation-view scopes mutation evidence per view (§11.3)`

### Task 5: The §7.1 gate rows on real reports

**Files:**
- Create: `internal/prready/views_test.go` (gate-level, HEAD mode), `internal/taskdone/views_test.go` (working tree)
- Modify: `tests/e2e-mutation-incremental.mjs` (one gate row against the live e2e repository)

- [ ] **Step 1: Write the gate-row tests.**
  - `internal/taskdone/views_test.go`, `TestGateRowsWithViews`:
    - Commit the real `full` report and its fixture tree into a repo, following the pattern of `taskdone/freshness_test.go`.
    - **Repeated flags, one section:**
      - Run the review with `MutationViews ["core","edge"]` and mode `enforce`, after editing `src/c.ts` line 2 in the working tree.
      - The log contains one `## Mutation Evidence Freshness` with a `core` row and an `edge` row (3 stale each).
      - There are two stale findings with fingerprints ending `:core:src/c.ts:<8>` and `:edge:src/c.ts:<8>`.
      - The verdict is NEEDS_REVISION.
    - **Cross-view isolation:**
      - Re-run on the same chain with `["core"]` only, after refreshing the report so that `core` is fresh. Use a copy of the report whose attestation digest for `src/c.ts` is updated to the edited content.
      - Expect the `core` row superseded and the `edge` row still open in the ledger.
      - Expect `edge` not counted in the run's open findings, so the verdict is not blocked by it.
    - **Renamed view:**
      - Re-run with a report whose map renames `edge` to `periphery`, using `["core"]`.
      - Expect the `edge` row superseded.
      - Then run with a report whose attestation has no `views`: nothing more is superseded by the sweep.
    - **Unattested under a view:**
      - Run with `["core"]` and a report with no attestation, in `enforce` mode.
      - Expect the `mutation:unattested:enforce:stryker:core:<8>` finding, and the verdict blocks.
  - `internal/prready/views_test.go`, `TestPrReadyViewsReadHead`: an uncommitted `src/c.ts` edit leaves `core` and `edge` verified. After committing it, both are stale. This mirrors 2a's HEAD test with views.
- [ ] **Step 2: Run them.** Expected: PASS if Tasks 1–4 are right. If a row fails, it is a bug in those tasks: fix it there, test first.
- [ ] **Step 3: Add one e2e row.**
  - After the `full` row, build the gate once: `go build -o <WORK>/metareview ./cmd/metareview`, from ROOT.
  - Run `review pr-ready --base <fixture commit> --mutation-report .mutation/incremental.json --mutation-view core --mutation-view edge` in REPO, with `METAREVIEW_ALLOW_MECHANICAL_PASS=1`.
  - Check that the review log (the path printed on stdout) has the `### Views` table with `core` 24 verified and `edge` 7 verified, 0 stale.
  - Check that `--mutation-view nope` exits 2.
  - If pr-ready needs repository setup the fixture lacks, use `review task-done` with a one-line task file committed in REPO. Ledger that as a ruling.
- [ ] **Step 4: Run.**
  - `go test -cover ./internal/taskdone/ ./internal/prready/`
  - `node tests/e2e-mutation-incremental.mjs`. Expected: all checks PASS, including the new `gate:` checks. It uses the network the first time; keep it under `.e2e/`.
- [ ] **Step 5: Commit.** `test(mutation-incremental): §7.1 gate rows with views on real reports`

### Task 6: Docs and changelog

**Files:** `docs/mutation-harness.md` §11, `CHANGELOG.md`

- [ ] **Step 1:** Update `docs/mutation-harness.md` §11 with:
  - `--mutation-view <name>` (repeatable); the exit-2 cases; unscoped reports; per-view fingerprints and titles; the section table;
  - **cutover:** after adopting views, run the gate once with reports and no `--mutation-view` to clear the unviewed rows;
  - an override on one view's fingerprint does not cover another view;
  - an edit outside every mutant span (a comment or import) stales every kill until the harness re-runs, which is conservative;
  - `Pending counted` vs `Pending inherited`.
  - Remove the "arrives with Plan 2b" wording.
- [ ] **Step 2:** Extend the 0.13.0 CHANGELOG entry with views and the out-of-mutant cause.
- [ ] **Step 3:** Run `go test ./...` and `go vet ./...`, then `gofmt -l .`. Expected: all green and nothing listed.
- [ ] **Step 4: Commit.** `docs(mutation-incremental): views, cutover and out-of-mutant hunks`

## Follow-ups (recorded, not in 2b)

- 2a's deferred minors:
  - a symlinked config reads as stale;
  - integer-like key order in `configDigest`;
  - the config path is uncovered in HEAD-mode real tests;
  - HEAD read errors should name the path and filter.
- `statically_tested` pins → anchor pins (R3), which the spec leaves out of 0.13.0.

## Self-Review

- **Spec coverage:**

  | Spec | Task |
  |---|---|
  | §11.3 flag, exit 2, unscoped reports, per-view classification, fingerprints, titles, section | 2, 4 |
  | §11.3 ledger: structured view, `openForRun`, supersede, rename sweep, overrides per view | 3 |
  | §11.5 gate mirror (out-of-mutant hunk → blanket) | 1 |
  | §11.7 Go tests (scoping, exit 2, fingerprints, `openForRun`/supersede, renames, diff vectors) | 1–4 |
  | §11.7 §7.1 gate rows | 5 |
  | §11.3 cutover and override docs, §11.5 noise docs | 6 |
- **Types:**
  - `Classify(r, content, views)` and `Build(reports, content, mode, views)` → `Result`, carried into `MutationContext.Views`/`ViewMaps()` and then `findings.Options.MutationViews`/`MutationViewMaps`.
  - `Input.View` → `Record.View`.
- **Placeholders:** the §7.1 rows name their exact expectations. Where a count depends on the real report (deleted `c.ts`'s closure), the step says to derive it from the report and pin it.


## Git

- Base: `0bebec4698dfc5a3940cde6468fa9970874a4acf`
- Head: `8de2a518d91fa442e7c0b7500476a766bf45952f`
- Branch: `mutation-incremental`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `96077`
- Filtered diff bytes: `96077`
- Risk level: `none`

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- CHANGELOG.md
- cmd/metareview/main.go
- cmd/metareview/views_test.go
- docs/mutation-harness.md
- internal/epicready/mutationctx_test.go
- internal/epicready/review.go
- internal/findings/findings.go
- internal/findings/views_test.go
- internal/mutationfresh/attest.go
- internal/mutationfresh/build.go
- internal/mutationfresh/build_test.go
- internal/mutationfresh/classify.go
- internal/mutationfresh/classify_test.go
- internal/mutationfresh/diff.go
- internal/mutationfresh/diff_test.go
- internal/mutationfresh/outofmutant_test.go
- internal/mutationfresh/views.go
- internal/mutationfresh/views_test.go
- internal/prready/mutationctx_test.go
- internal/prready/review.go
- internal/prready/views_test.go
- internal/reviewers/mutation.go
- internal/reviewers/mutation_test.go
- internal/reviewers/viewmaps_test.go
- internal/taskdone/mutationctx_test.go
- internal/taskdone/review.go
- tests/e2e-mutation-incremental.mjs

### Manifest Blockers
No manifest blockers.

## Changed Files

- CHANGELOG.md
- cmd/metareview/main.go
- cmd/metareview/views_test.go
- docs/mutation-harness.md
- internal/epicready/mutationctx_test.go
- internal/epicready/review.go
- internal/findings/findings.go
- internal/findings/views_test.go
- internal/mutationfresh/attest.go
- internal/mutationfresh/build.go
- internal/mutationfresh/build_test.go
- internal/mutationfresh/classify.go
- internal/mutationfresh/classify_test.go
- internal/mutationfresh/diff.go
- internal/mutationfresh/diff_test.go
- internal/mutationfresh/outofmutant_test.go
- internal/mutationfresh/views.go
- internal/mutationfresh/views_test.go
- internal/prready/mutationctx_test.go
- internal/prready/review.go
- internal/prready/views_test.go
- internal/reviewers/mutation.go
- internal/reviewers/mutation_test.go
- internal/reviewers/viewmaps_test.go
- internal/taskdone/mutationctx_test.go
- internal/taskdone/review.go
- tests/e2e-mutation-incremental.mjs

## Diff

```diff
diff --git a/CHANGELOG.md b/CHANGELOG.md
index bfc38c2..1146841 100644
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -27,6 +27,11 @@
   - A chain blocked only by stale evidence waits for a refresh for up to 2 × `maxAttempts` before
     escalating.
   - Runs without reports review exactly as before, and their pr-ready input digest is unchanged.
+  - `--mutation-view <name>` (repeatable) judges the evidence per view of the harness's view map.
+    Each view gets its own counts, findings and fingerprints, and a table row in the section.
+    Findings of other views are left alone, and a renamed view's findings are superseded.
+  - An edit to a mutated file that touches no mutant stales every kill until the harness re-runs.
+    Pending kills are split into counted and inherited.
 
 ## 0.12.0 - 2026-09-10
 
diff --git a/cmd/metareview/main.go b/cmd/metareview/main.go
index 34fb3ac..7472bf6 100644
--- a/cmd/metareview/main.go
+++ b/cmd/metareview/main.go
@@ -12,6 +12,7 @@ import (
 	"os"
 	"os/exec"
 	"path/filepath"
+	"slices"
 	"strconv"
 	"strings"
 	"time"
@@ -116,9 +117,9 @@ Usage:
   metareview evidence run -- <command> [args...]
   metareview evidence import --github-checks <pr-number> [--repo <owner/repo>]
   metareview review artifact <path> [--previous-run <run-id>] [--scaffold-only]
-  metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--mutation-report <path>]... [--shard-result <path>]... [--cross-shard-result <path>]
-  metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--mutation-report <path>]...
-  metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--mutation-report <path>]... [--github-pr <number>] [--include-working-tree] [--shard-result <path>]... [--cross-shard-result <path>]
+  metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--mutation-report <path>]... [--mutation-view <name>]... [--shard-result <path>]... [--cross-shard-result <path>]
+  metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--mutation-report <path>]... [--mutation-view <name>]...
+  metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--mutation-report <path>]... [--mutation-view <name>]... [--github-pr <number>] [--include-working-tree] [--shard-result <path>]... [--cross-shard-result <path>]
   METAREVIEW_MUTATION_FRESHNESS=advisory|enforce  freshness of --mutation-report evidence (docs/mutation-harness.md)
   metareview review record-lenses [--scope pr-ready|task-done|epic-ready] [--base <ref>] [--verdict <v>] [--mode subagent-adjudicated|in-session-emulated] [--lenses a,b,c] [--from-run <fsm-run-id>]
   metareview learn --post-merge <pr-number> [--base <ref>] [--github-pr <number>] [--session-root <path>]
@@ -319,6 +320,9 @@ func dispatch(args []string) {
 			case "--mutation-report":
 				options.MutationReportPaths = append(options.MutationReportPaths, mustMutationReport(flagValue(args, i, "--mutation-report")))
 				i++
+			case "--mutation-view":
+				options.MutationViews = appendUnique(options.MutationViews, flagValue(args, i, "--mutation-view"))
+				i++
 			case "--shard-result":
 				options.ShardResultPaths = append(options.ShardResultPaths, mustResultFile(flagValue(args, i, "--shard-result")))
 				i++
@@ -330,6 +334,7 @@ func dispatch(args []string) {
 				exit(2)
 			}
 		}
+		mustMutationViews(options.MutationReportPaths, options.MutationViews)
 		result, err := taskdone.Create(workdir, args[2], options)
 		exitOnErr(err)
 		_, _ = fmt.Fprintln(stdout, result.ReviewRel)
@@ -359,11 +364,15 @@ func dispatch(args []string) {
 			case "--mutation-report":
 				options.MutationReportPaths = append(options.MutationReportPaths, mustMutationReport(flagValue(args, i, "--mutation-report")))
 				i++
+			case "--mutation-view":
+				options.MutationViews = appendUnique(options.MutationViews, flagValue(args, i, "--mutation-view"))
+				i++
 			default:
 				_, _ = fmt.Fprintf(stderr, "Unknown option: %s\n", args[i])
 				exit(2)
 			}
 		}
+		mustMutationViews(options.MutationReportPaths, options.MutationViews)
 		result, err := epicready.Create(workdir, args[2], options)
 		exitOnErr(err)
 		_, _ = fmt.Fprintln(stdout, result.ReviewRel)
@@ -564,6 +573,9 @@ func dispatch(args []string) {
 			case "--mutation-report":
 				options.MutationReportPaths = append(options.MutationReportPaths, mustMutationReport(flagValue(args, i, "--mutation-report")))
 				i++
+			case "--mutation-view":
+				options.MutationViews = appendUnique(options.MutationViews, flagValue(args, i, "--mutation-view"))
+				i++
 			case "--github-pr":
 				options.GitHubPR = flagValue(args, i, "--github-pr")
 				i++
@@ -580,6 +592,7 @@ func dispatch(args []string) {
 				exit(2)
 			}
 		}
+		mustMutationViews(options.MutationReportPaths, options.MutationViews)
 		result, err := prready.Create(workdir, options)
 		exitOnErr(err)
 		_, _ = fmt.Fprintln(stdout, result.ReviewRel)
@@ -1188,6 +1201,33 @@ func mustFreshnessMode() {
 	}
 }
 
+// mustMutationViews rejects --mutation-view names the review could not scope (spec §11.3): a view
+// without a report, an invalid name, or one missing from an attested report's view map.
+func mustMutationViews(reportPaths, views []string) {
+	if len(views) == 0 {
+		return
+	}
+	if len(reportPaths) == 0 {
+		_, _ = fmt.Fprintln(stderr, "--mutation-view needs --mutation-report")
+		exit(2)
+	}
+	reports, err := mutation.LoadAll(reportPaths)
+	if err == nil {
+		err = mutationfresh.CheckViews(reports, views)
+	}
+	if err != nil {
+		_, _ = fmt.Fprintf(stderr, "%v\n", err)
+		exit(2)
+	}
+}
+
+func appendUnique(list []string, value string) []string {
+	if slices.Contains(list, value) {
+		return list
+	}
+	return append(list, value)
+}
+
 // mustMutationReport rejects a --mutation-report file the review could not act on, at the point
 // the operator can still fix it. Parsing here as well as in the review is deliberate: a typo in a
 // path or a truncated report otherwise surfaces as a review that found nothing, which reads
diff --git a/cmd/metareview/views_test.go b/cmd/metareview/views_test.go
new file mode 100644
index 0000000..a4c3760
--- /dev/null
+++ b/cmd/metareview/views_test.go
@@ -0,0 +1,44 @@
+package main
+
+import (
+	"path/filepath"
+	"strings"
+	"testing"
+)
+
+// Spec §11.3: --mutation-view is repeatable, and a name that is invalid, has no report to scope, or
+// is missing from an attested report's view map is a usage error (exit 2) on every gate.
+func TestMutationViewUsageErrors(t *testing.T) {
+	root := gitRepo(t)
+	report, err := filepath.Abs("../../testdata/mutation-incremental/real/full/incremental.json")
+	if err != nil {
+		t.Fatal(err)
+	}
+	for _, gate := range [][]string{
+		{"review", "task-done", "docs/tasks/t.md", "--base", "main"},
+		{"review", "epic-ready", "docs/tasks/t.md", "--base", "main"},
+		{"review", "pr-ready", "--base", "main"},
+	} {
+		for _, c := range []struct {
+			args []string
+			want string
+		}{
+			{[]string{"--mutation-view", "core"}, "--mutation-view needs --mutation-report"},
+			{[]string{"--mutation-report", report, "--mutation-view", "core", "--mutation-view", "nope"}, `mutation view "nope" is not in the views of`},
+			{[]string{"--mutation-view", "bad name", "--mutation-report", report}, `invalid mutation view name "bad name"`},
+		} {
+			args := append(append([]string(nil), gate...), c.args...)
+			code, _, stderr := runCLI(t, root, nil, args...)
+			if code != 2 || !strings.Contains(stderr, c.want) {
+				t.Errorf("%v: exit %d, stderr %q", args, code, stderr)
+			}
+		}
+	}
+}
+
+// A repeated --mutation-view name is one view.
+func TestAppendUniqueCollapsesRepeatedViews(t *testing.T) {
+	if got := appendUnique(appendUnique([]string{"core"}, "edge"), "core"); strings.Join(got, ",") != "core,edge" {
+		t.Errorf("got %v", got)
+	}
+}
diff --git a/docs/mutation-harness.md b/docs/mutation-harness.md
index fd7bbad..35ad7a6 100644
--- a/docs/mutation-harness.md
+++ b/docs/mutation-harness.md
@@ -311,8 +311,12 @@ and main pays one full run (`stateVersion` changes make older states cold).
   digests whether the kill still describes the code under review:
   - **verified** — nothing it depends on changed;
   - **stale** — its file changed, a test that killed it changed, a changed file's covering tests
-    killed it, or a support, global or unclassified input changed; the first cause is recorded;
-  - **pending** — its file is covered by a deferral;
+    killed it, or a support, global or unclassified input changed; the first cause is recorded.
+    A changed `mutate` file whose edit touches no mutant (a comment, an import, a new function
+    before it has mutants) counts like a global input and stales every kill until the harness
+    re-runs. This is conservative on purpose: the gate has no import graph;
+  - **pending** — its file is covered by a deferral. It is "inherited" when only deferrals
+    carried from main's state cover it, and "counted" when the run's own changes caused it;
   - **unbound** — its file is not attested, or no test is recorded as killing it;
   - **unattested** — the report has no valid attestation.
 - **What it reads.** task-done and epic-ready read the working tree. pr-ready reads HEAD (unless
@@ -327,7 +331,19 @@ and main pays one full run (`stateVersion` changes make older states cold).
   pending and unattested findings it no longer produces. A run without reports leaves them alone.
 - **Escalation.** A chain blocked only by stale evidence waits for a refresh for up to
   2 × `maxAttempts` before it escalates ("stale mutation evidence not refreshed after N attempts").
-- **Views.** `--mutation-view` (per-view findings) arrives with views support.
+- **Views.** Pass `--mutation-view <name>` (repeatable) to judge the evidence per view.
+  - Each view gets its own counts and findings. The finding's title names the view, as in
+    `Mutation evidence stale (core): src/a.ts changed`, and its fingerprint carries it.
+  - The section gains a table with a row per view: verified, stale, pending counted, pending
+    inherited, unbound and unattested. The re-run list gains a View column.
+  - Views are never summed.
+  - A report without a view map is not scoped: every kill counts under every view. This covers an
+    unattested report, and one attested before views were adopted.
+  - These are usage errors (exit 2): an invalid name, `--mutation-view` without `--mutation-report`,
+    and a name missing from an attested report's map.
+  - A run with views owns only its views' findings. A run without views owns them all.
+  - When the reports' maps no longer name a view (it was renamed or removed), that view's open
+    findings are superseded.
 - An enforced stale finding clears only on a later run that supplies reports, or by override.
   Overrides on pending or unattested findings last only for that report. Every mutation override is
   tied to the mode in its fingerprint, and an override on one view does not cover the same cause
diff --git a/internal/epicready/mutationctx_test.go b/internal/epicready/mutationctx_test.go
index c1d248c..8c8a893 100644
--- a/internal/epicready/mutationctx_test.go
+++ b/internal/epicready/mutationctx_test.go
@@ -12,7 +12,7 @@ import (
 // A skipped report is indistinguishable from a package with no survivors, so the failure mode of
 // getting this wrong is a gate that passes because it looked at less.
 func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
-	if _, err := mutationContextFor(t.TempDir(), nil); err != nil {
+	if _, err := mutationContextFor(t.TempDir(), nil, nil); err != nil {
 		t.Errorf("no reports at all is the ordinary case: %v", err)
 	}
 	for name, path := range map[string]string{
@@ -21,7 +21,7 @@ func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
 		"empty file":    writeTemp(t, ""),
 		"files is text": writeTemp(t, `{"files":"none"}`),
 	} {
-		ctx, err := mutationContextFor(t.TempDir(), []string{path})
+		ctx, err := mutationContextFor(t.TempDir(), []string{path}, nil)
 		if err == nil {
 			t.Errorf("%s: must be an error, got %d reports", name, len(ctx.Reports))
 			continue
@@ -35,7 +35,7 @@ func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
 // A readable report reaches the reviewers, so the review actually acts on it.
 func TestAReadableMutationReportReachesTheReviewers(t *testing.T) {
 	path := writeTemp(t, `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"type":"T","status":"LIVED","line":3}]}]}`)
-	ctx, err := mutationContextFor(t.TempDir(), []string{path})
+	ctx, err := mutationContextFor(t.TempDir(), []string{path}, nil)
 	if err != nil {
 		t.Fatal(err)
 	}
diff --git a/internal/epicready/review.go b/internal/epicready/review.go
index c5b4fd7..130b76b 100644
--- a/internal/epicready/review.go
+++ b/internal/epicready/review.go
@@ -31,6 +31,8 @@ type Options struct {
 	// MutationReportPaths are --mutation-report files: a mutation-testing engine's output,
 	// either mutation-testing-report-schema or gremlins JSON. Empty is the ordinary case.
 	MutationReportPaths []string
+	// MutationViews are --mutation-view names (spec §11.3): the evidence is judged per view.
+	MutationViews []string
 }
 
 type Result struct {
@@ -146,7 +148,7 @@ func Create(root, target string, options Options) (Result, error) {
 	if report.Capabilities.Beads || report.Capabilities.Metaswarm {
 		gateEffect = "gate"
 	}
-	mutationContext, err := mutationContextFor(root, options.MutationReportPaths)
+	mutationContext, err := mutationContextFor(root, options.MutationReportPaths, options.MutationViews)
 	if err != nil {
 		return Result{}, err
 	}
@@ -189,10 +191,12 @@ func Create(root, target string, options Options) (Result, error) {
 			previousRunIDs = append(previousRunIDs, link.ID)
 		}
 		reconciled, err := reconcileFindings(root, run, rawFindings, findings.Options{
-			PreviousRunID:   options.PreviousRunID,
-			PreviousRunIDs:  previousRunIDs,
-			ResetRunIDs:     chain.ResetRunIDs,
-			MutationEngines: mutationContext.Engines(),
+			PreviousRunID:    options.PreviousRunID,
+			PreviousRunIDs:   previousRunIDs,
+			ResetRunIDs:      chain.ResetRunIDs,
+			MutationEngines:  mutationContext.Engines(),
+			MutationViews:    mutationContext.Views,
+			MutationViewMaps: mutationContext.ViewMaps(),
 		})
 		if err != nil {
 			return err
@@ -855,6 +859,6 @@ func firstNonEmpty(values ...string) string {
 // mutationContextFor loads the declared mutation reports. An unreadable or unrecognised report is
 // an error that stops the review, never a skipped file: a mutation gate that quietly drops a
 // report is a gate that passes because it looked at less.
-func mutationContextFor(root string, paths []string) (reviewers.MutationContext, error) {
-	return reviewers.LoadMutationContext(root, paths, "epic-ready", false)
+func mutationContextFor(root string, paths, views []string) (reviewers.MutationContext, error) {
+	return reviewers.LoadMutationContext(root, paths, views, "epic-ready", false)
 }
diff --git a/internal/findings/findings.go b/internal/findings/findings.go
index 150f241..0e6c882 100644
--- a/internal/findings/findings.go
+++ b/internal/findings/findings.go
@@ -34,6 +34,12 @@ type Options struct {
 	// MutationEngines are the engines of this run's --mutation-report files, and empty when the run
 	// supplied none, so mutation-freshness rows and their overrides are left alone (spec §6.5).
 	MutationEngines []string
+	// MutationViews are this run's --mutation-view names (spec §11.3). With views, only freshness
+	// rows of those views count as open and may be superseded; without, every freshness row does.
+	MutationViews []string
+	// MutationViewMaps holds, per engine, the view names in the maps of this run's attested reports
+	// of that engine that carry one. An engine is absent when none does; the rename sweep needs it.
+	MutationViewMaps map[string][]string
 }
 
 type Evidence struct {
@@ -55,6 +61,8 @@ type Input struct {
 	Owner              string     `json:"owner,omitempty"`
 	KnowledgeCandidate bool       `json:"knowledgeCandidate,omitempty"`
 	Fingerprint        string     `json:"fingerprint"`
+	// View is the mutation view a freshness finding belongs to (spec §11.3), stored on its row.
+	View string `json:"view,omitempty"`
 }
 
 type Record struct {
@@ -78,6 +86,8 @@ type Record struct {
 	Fingerprint        string     `json:"fingerprint"`
 	Target             any        `json:"target"`
 	FixedInRunID       string     `json:"fixedInRunId,omitempty"`
+	// View is the mutation view of a freshness row (spec §11.3), empty for every other row.
+	View string `json:"view,omitempty"`
 
 	// Process-exception provenance (see override.go). An override is never a fix:
 	// FixedInRunID stays empty.
@@ -181,7 +191,7 @@ func Reconcile(root string, run Run, current []Input, options Options) (Result,
 		return Result{}, err
 	}
 	activeCurrent := make([]Record, 0, len(current))
-	openFindings := openForRun(all, run)
+	openFindings := openForRun(all, run, options)
 	for _, record := range all {
 		if record.Status == "open" &&
 			record.Fingerprint != "" &&
@@ -226,12 +236,26 @@ func freshnessEngine(fingerprint string) string {
 	return parts[3]
 }
 
-// supersedesFreshness: this run supplied reports of the row's engine and did not reproduce it.
+// supersedesFreshness: this run supplied reports of the row's engine and did not reproduce it, and
+// the row is of one of the run's views (spec §11.3), or of a view that no view map of the run's
+// reports of that engine names any more (the rename sweep).
 func supersedesFreshness(record Record, run Run, options Options, current map[string]bool) bool {
-	return len(options.MutationEngines) > 0 && IsFreshnessFingerprint(record.Fingerprint) &&
-		slices.Contains(options.MutationEngines, freshnessEngine(record.Fingerprint)) &&
-		(record.Status == "open" || record.Status == StatusOverridePending) &&
-		sameRunTarget(record, run) && !current[record.Fingerprint]
+	engine := freshnessEngine(record.Fingerprint)
+	if len(options.MutationEngines) == 0 || !IsFreshnessFingerprint(record.Fingerprint) ||
+		!slices.Contains(options.MutationEngines, engine) ||
+		(record.Status != "open" && record.Status != StatusOverridePending) ||
+		!sameRunTarget(record, run) || current[record.Fingerprint] || record.RunID == run.ID {
+		return false
+	}
+	names, mapped := options.MutationViewMaps[engine]
+	renamed := record.View != "" && mapped && !slices.Contains(names, record.View)
+	return inViewScope(record, options) || renamed
+}
+
+// inViewScope: a run with --mutation-view owns only the freshness rows of its views (spec §11.3).
+func inViewScope(record Record, options Options) bool {
+	return len(options.MutationViews) == 0 || !IsFreshnessFingerprint(record.Fingerprint) ||
+		slices.Contains(options.MutationViews, record.View)
 }
 
 // OnlyStaleBlockers reports that every open blocking finding is stale mutation evidence (spec §6.8).
@@ -787,6 +811,7 @@ func normalize(run Run, finding Input, index int, createdAt string) Record {
 		KnowledgeCandidate: finding.KnowledgeCandidate,
 		BeadsFollowupID:    nil,
 		Fingerprint:        finding.Fingerprint,
+		View:               finding.View,
 		Target:             run.Target,
 		CreatedAt:          createdAt,
 		UpdatedAt:          createdAt,
@@ -879,10 +904,10 @@ func classForCount(classification, severity string) string {
 	}
 }
 
-func openForRun(records []Record, run Run) []Record {
+func openForRun(records []Record, run Run, options Options) []Record {
 	open := make([]Record, 0, len(records))
 	for _, record := range records {
-		if Blocks(record.Status) && sameRunTarget(record, run) {
+		if Blocks(record.Status) && sameRunTarget(record, run) && inViewScope(record, options) {
 			open = append(open, record)
 		}
 	}
diff --git a/internal/findings/views_test.go b/internal/findings/views_test.go
new file mode 100644
index 0000000..a49873b
--- /dev/null
+++ b/internal/findings/views_test.go
@@ -0,0 +1,147 @@
+package findings
+
+import "testing"
+
+func viewedStale(view string) Input {
+	in := staleInput()
+	in.Fingerprint = "mutation:stale:enforce:stryker:" + view + ":src/a.ts:0123abcd"
+	in.View = view
+	return in
+}
+
+func openFingerprints(res Result) map[string]bool {
+	out := map[string]bool{}
+	for _, r := range res.OpenFindings {
+		out[r.Fingerprint] = true
+	}
+	return out
+}
+
+// Spec §11.3: a run with views counts as open, and supersedes, only rows of its views.
+func TestViewedRunCountsOnlyItsViews(t *testing.T) {
+	root := t.TempDir()
+	core, edge := viewedStale("core"), viewedStale("edge")
+	opts := Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core", "edge"}}
+	if _, err := Reconcile(root, run("r1"), []Input{core, edge}, opts); err != nil {
+		t.Fatal(err)
+	}
+	if got := statusOf(t, root, edge.Fingerprint); len(got) != 1 || got[0] != "open/" {
+		t.Fatalf("the view is stored on the row: %v", got)
+	}
+	res, err := Reconcile(root, run("r2"), nil, Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core"}})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if got := statusOf(t, root, core.Fingerprint); got[0] != StatusSuperseded+"/" {
+		t.Errorf("core: %v", got)
+	}
+	if got := statusOf(t, root, edge.Fingerprint); got[0] != "open/" {
+		t.Errorf("edge stays open: %v", got)
+	}
+	if res.OpenBlockingCount != 0 || openFingerprints(res)[edge.Fingerprint] {
+		t.Errorf("edge is not this run's: %d open blocking", res.OpenBlockingCount)
+	}
+	records, _ := All(root)
+	for _, r := range records {
+		if r.Fingerprint == edge.Fingerprint && r.View != "edge" {
+			t.Errorf("row view %q", r.View)
+		}
+	}
+}
+
+// A run without views counts and supersedes every freshness row, viewed or not.
+func TestUnviewedRunSupersedesViewedRows(t *testing.T) {
+	root := t.TempDir()
+	core, edge := viewedStale("core"), viewedStale("edge")
+	if _, err := Reconcile(root, run("r1"), []Input{core, edge}, Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core", "edge"}}); err != nil {
+		t.Fatal(err)
+	}
+	if res, err := Reconcile(root, run("r2"), nil, Options{}); err != nil || res.OpenBlockingCount != 2 {
+		t.Fatalf("a run without reports counts every row: %d %v", res.OpenBlockingCount, err)
+	}
+	if _, err := Reconcile(root, run("r3"), nil, Options{MutationEngines: []string{"stryker"}}); err != nil {
+		t.Fatal(err)
+	}
+	for _, fp := range []string{core.Fingerprint, edge.Fingerprint} {
+		if got := statusOf(t, root, fp); got[0] != StatusSuperseded+"/" {
+			t.Errorf("%s: %v", fp, got)
+		}
+	}
+}
+
+// Spec §11.3: a view absent from every view map of the run's reports of that engine has its rows
+// superseded; runs whose reports carry no map, unviewed rows and overridden rows are untouched.
+func TestRenamedViewSupersedesItsRows(t *testing.T) {
+	root := t.TempDir()
+	edge, plain := viewedStale("edge"), staleInput()
+	if _, err := Reconcile(root, run("r1"), []Input{plain}, Options{MutationEngines: []string{"stryker"}}); err != nil {
+		t.Fatal(err)
+	}
+	if _, err := Reconcile(root, run("r2"), []Input{edge}, Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"edge"}}); err != nil {
+		t.Fatal(err)
+	}
+	coreOnly := Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core"}}
+	if _, err := Reconcile(root, run("r3"), nil, coreOnly); err != nil {
+		t.Fatal(err)
+	}
+	if got := statusOf(t, root, edge.Fingerprint); got[0] != "open/" {
+		t.Fatalf("no report with a view map: %v", got)
+	}
+	coreOnly.MutationViewMaps = map[string][]string{"stryker": {"core", "periphery"}}
+	if _, err := Reconcile(root, run("r4"), nil, coreOnly); err != nil {
+		t.Fatal(err)
+	}
+	if got := statusOf(t, root, edge.Fingerprint); got[0] != StatusSuperseded+"/" {
+		t.Errorf("renamed view: %v", got)
+	}
+	if got := statusOf(t, root, plain.Fingerprint); got[0] != "open/" {
+		t.Errorf("an unviewed row is outside the sweep: %v", got)
+	}
+}
+
+func TestRenameSweepLeavesOverriddenRows(t *testing.T) {
+	root := t.TempDir()
+	edge := viewedStale("edge")
+	res, err := Reconcile(root, run("r1"), []Input{edge}, Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"edge"}})
+	if err != nil {
+		t.Fatal(err)
+	}
+	id := res.NewFindings[0].ID
+	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "edge is being retired", Now: "2026-09-24T00:00:00Z"}); err != nil {
+		t.Fatal(err)
+	}
+	if err := GrantOverride(root, id, OverrideGrant{By: "human", Reason: "accepted for the rename", Now: "2026-09-24T00:00:01Z"}); err != nil {
+		t.Fatal(err)
+	}
+	sweep := Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core"}, MutationViewMaps: map[string][]string{"stryker": {"core"}}}
+	if _, err := Reconcile(root, run("r2"), nil, sweep); err != nil {
+		t.Fatal(err)
+	}
+	if got := statusOf(t, root, edge.Fingerprint); got[0] != StatusOverridden+"/" {
+		t.Errorf("overridden: %v", got)
+	}
+}
+
+// Per-view fingerprints differ, so an override on one view never covers the same cause in another.
+func TestOverrideOnOneViewDoesNotCoverAnother(t *testing.T) {
+	root := t.TempDir()
+	opts := Options{MutationEngines: []string{"stryker"}, MutationViews: []string{"core", "edge"}}
+	res, err := Reconcile(root, run("r1"), []Input{viewedStale("core"), viewedStale("edge")}, opts)
+	if err != nil {
+		t.Fatal(err)
+	}
+	id := res.NewFindings[0].ID
+	if err := RequestOverride(root, id, OverrideRequest{By: "agent", Reason: "core accepted for now", Now: "2026-09-24T00:00:00Z"}); err != nil {
+		t.Fatal(err)
+	}
+	if err := GrantOverride(root, id, OverrideGrant{By: "human", Reason: "accepted by the maintainer", Now: "2026-09-24T00:00:01Z"}); err != nil {
+		t.Fatal(err)
+	}
+	res, err = Reconcile(root, run("r2"), []Input{viewedStale("core"), viewedStale("edge")}, opts)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if res.OpenBlockingCount != 1 || !openFingerprints(res)[viewedStale("edge").Fingerprint] {
+		t.Errorf("edge still blocks: %d %v", res.OpenBlockingCount, openFingerprints(res))
+	}
+}
diff --git a/internal/mutationfresh/attest.go b/internal/mutationfresh/attest.go
index ce99c2b..684297f 100644
--- a/internal/mutationfresh/attest.go
+++ b/internal/mutationfresh/attest.go
@@ -20,6 +20,7 @@ type Attestation struct {
 	Exclusions    []string             `json:"exclusions"`
 	Files         map[string]FileEntry `json:"files"`
 	Deferrals     []Deferral           `json:"deferrals"`
+	Views         map[string][]string  `json:"views"`
 }
 
 // FileEntry is one attested path: its start-of-run digest, category and whether git tracked it.
diff --git a/internal/mutationfresh/build.go b/internal/mutationfresh/build.go
index b0c07e3..593b356 100644
--- a/internal/mutationfresh/build.go
+++ b/internal/mutationfresh/build.go
@@ -20,21 +20,46 @@ type Result struct {
 	Section   string
 }
 
-// Build classifies every report, then derives the findings (spec §6.5) and the section (§6.6).
-func Build(reports []mutation.Report, content Content, mode string) (Result, error) {
+// Build classifies every report, then derives the findings (spec §6.5) and the section (§6.6),
+// once per requested view (§11.3) or once for the whole report when none is requested.
+func Build(reports []mutation.Report, content Content, mode string, views []string) (Result, error) {
 	res := Result{Mode: mode}
 	for _, r := range reports {
-		f, err := Classify(r, content)
+		f, err := Classify(r, content, views)
 		if err != nil {
 			return Result{}, err
 		}
 		res.Freshness = append(res.Freshness, f)
 	}
-	res.Findings = freshnessFindings(res.Freshness, mode)
-	res.Section = section(res.Freshness, mode)
+	slices := []string{""}
+	if len(views) > 0 {
+		slices = views
+	}
+	for _, v := range slices {
+		res.Findings = append(res.Findings, freshnessFindings(res.Freshness, mode, v)...)
+	}
+	res.Section = section(res.Freshness, mode, views)
 	return res, nil
 }
 
+// tallyOf is the report's own tally for view "", else that view's.
+func tallyOf(f ReportFreshness, view string) Tally {
+	for _, v := range f.Views {
+		if v.View == view {
+			return v.Tally
+		}
+	}
+	return f.Tally
+}
+
+// viewParts are the fingerprint segment (":<view>") and the title qualifier (" (<view>)").
+func viewParts(view string) (fingerprint, title string) {
+	if view == "" {
+		return "", ""
+	}
+	return ":" + view, " (" + view + ")"
+}
+
 func gate(mode string) (classification, severity string) {
 	if mode == Enforce {
 		return "blocking", "high" // classForCount counts blocking only at high or critical
@@ -42,16 +67,17 @@ func gate(mode string) (classification, severity string) {
 	return "advisory", "medium"
 }
 
-func freshnessFindings(list []ReportFreshness, mode string) []findings.Input {
+func freshnessFindings(list []ReportFreshness, mode, view string) []findings.Input {
 	type staleKey struct{ engine, cause string }
 	type staleSum struct {
 		digest  string
 		kills   int
 		reports []string
 	}
+	fpView, titleView := viewParts(view)
 	sums := map[staleKey]*staleSum{}
 	for _, f := range list {
-		for _, c := range f.Causes {
+		for _, c := range tallyOf(f, view).Causes {
 			key := staleKey{f.Engine, c.Cause}
 			if sums[key] == nil {
 				sums[key] = &staleSum{digest: c.Digest}
@@ -73,57 +99,64 @@ func freshnessFindings(list []ReportFreshness, mode string) []findings.Input {
 		s := sums[k]
 		out = append(out, findings.Input{
 			Reviewer: reviewer, Severity: severity, Classification: classification, Owner: "implementer",
-			Title:          "Mutation evidence stale: " + k.cause + " changed",
+			Title:          "Mutation evidence stale" + titleView + ": " + k.cause + " changed",
 			Finding:        fmt.Sprintf("%d kill(s) in %s were verified against a different %s, so they are not evidence for the code under review.", s.kills, strings.Join(s.reports, ", "), k.cause),
 			Expected:       "Every kill the gate counts was verified against the content under review.",
 			Found:          fmt.Sprintf("%s is now %s.", k.cause, s.digest),
 			Recommendation: "Re-run the harness (node tools/mutation-incremental/cli.mjs run --mode incremental), commit before running pr-ready, and pass the refreshed <stateDir>/incremental.json.",
 			Evidence:       []findings.Evidence{{Type: "mutant", Path: k.cause}},
-			Fingerprint:    fmt.Sprintf("mutation:stale:%s:%s:%s:%s", mode, k.engine, k.cause, sha256Hex([]byte(k.cause + "=" + s.digest))[:8]),
+			Fingerprint:    fmt.Sprintf("mutation:stale:%s:%s%s:%s:%s", mode, k.engine, fpView, k.cause, sha256Hex([]byte(k.cause + "=" + s.digest))[:8]),
+			View:           view,
 		})
 	}
 	for _, f := range list {
-		if f.Pending > 0 {
+		t := tallyOf(f, view)
+		if t.Pending > 0 {
 			out = append(out, findings.Input{
 				Reviewer: reviewer, Severity: "medium", Classification: "advisory", Owner: "reviewer",
-				Title:          fmt.Sprintf("Mutation evidence pending: %d kills", f.Pending),
-				Finding:        fmt.Sprintf("%d kill(s) in %s are covered by a deferral (%s) and wait for a full run, so they are not counted as evidence.", f.Pending, f.Path, strings.Join(f.Deferrals, "; ")),
+				Title:          fmt.Sprintf("Mutation evidence pending%s: %d kills", titleView, t.Pending),
+				Finding:        fmt.Sprintf("%d kill(s) in %s are covered by a deferral (%s) and wait for a full run, so they are not counted as evidence.", t.Pending, f.Path, strings.Join(f.Deferrals, "; ")),
 				Expected:       "Deferred work is re-verified by a full run before it counts.",
 				Recommendation: "Let main's full run clear the deferrals, or run the harness with --mode full locally.",
 				Evidence:       []findings.Evidence{{Type: "mutant", Path: f.Path}},
-				Fingerprint:    fmt.Sprintf("mutation:pending:%s:%s:%s", mode, f.Engine, f.ReportSha256[:8]),
+				Fingerprint:    fmt.Sprintf("mutation:pending:%s:%s%s:%s", mode, f.Engine, fpView, f.ReportSha256[:8]),
+				View:           view,
 			})
 		}
 		if !f.Attested && f.Engine == "stryker" {
 			out = append(out, findings.Input{
 				Reviewer: reviewer, Severity: severity, Classification: classification, Owner: "reviewer",
-				Title:          "Mutation report has no attestation",
-				Finding:        fmt.Sprintf("%s has no valid attestation (%s), so the gate cannot tell whether its %d kill(s) still describe the code under review.", f.Path, f.UnattestedReason, f.Unattested),
+				Title:          "Mutation report has no attestation" + titleView,
+				Finding:        fmt.Sprintf("%s has no valid attestation (%s), so the gate cannot tell whether its %d kill(s) still describe the code under review.", f.Path, f.UnattestedReason, t.Unattested),
 				Expected:       "Mutation evidence comes from the mutation-incremental harness, which attests what it verified.",
 				Recommendation: "Produce the report with the harness and pass <stateDir>/incremental.json (its attestation.json sits beside it).",
 				Evidence:       []findings.Evidence{{Type: "mutant", Path: f.Path}},
-				Fingerprint:    fmt.Sprintf("mutation:unattested:%s:%s:%s", mode, f.Engine, f.ReportSha256[:8]),
+				Fingerprint:    fmt.Sprintf("mutation:unattested:%s:%s%s:%s", mode, f.Engine, fpView, f.ReportSha256[:8]),
+				View:           view,
 			})
 		}
 	}
 	return out
 }
 
-// section renders "## Mutation Evidence Freshness" (spec §6.6), or "" without reports.
-func section(list []ReportFreshness, mode string) string {
+// section renders "## Mutation Evidence Freshness" (spec §6.6), or "" without reports. With views
+// (§11.3) it adds a table row per view and a View column to the re-run list.
+func section(list []ReportFreshness, mode string, views []string) string {
 	if len(list) == 0 {
 		return ""
 	}
-	var attested, verified, stale, pending, unbound, unattested int
+	var attested int
+	var total Tally
 	var reasons, deferrals, unverifiable []string
 	seenDeferral, seenEngine := map[string]bool{}, map[string]bool{}
-	rows := map[[2]string]int{}
+	perView := make([]Tally, len(views))
+	rows := map[[3]string]int{}
 	for _, f := range list {
-		verified += f.Verified
-		stale += f.Stale
-		pending += f.Pending
-		unbound += f.Unbound
-		unattested += f.Unattested
+		total.Verified += f.Verified
+		total.Stale += f.Stale
+		total.Pending += f.Pending
+		total.Unbound += f.Unbound
+		total.Unattested += f.Unattested
 		if f.Attested {
 			attested++
 		} else {
@@ -139,8 +172,22 @@ func section(list []ReportFreshness, mode string) string {
 				deferrals = append(deferrals, d)
 			}
 		}
-		for _, row := range f.ReRun {
-			rows[[2]string{row.File, row.Cause}] += row.Kills
+		if len(views) == 0 {
+			for _, row := range f.ReRun {
+				rows[[3]string{"", row.File, row.Cause}] += row.Kills
+			}
+		}
+		for i, v := range views {
+			t := tallyOf(f, v)
+			perView[i].Verified += t.Verified
+			perView[i].Stale += t.Stale
+			perView[i].Pending += t.Pending
+			perView[i].PendingInherited += t.PendingInherited
+			perView[i].Unbound += t.Unbound
+			perView[i].Unattested += t.Unattested
+			for _, row := range t.ReRun {
+				rows[[3]string{v, row.File, row.Cause}] += row.Kills
+			}
 		}
 	}
 	var b strings.Builder
@@ -151,7 +198,7 @@ func section(list []ReportFreshness, mode string) string {
 		fmt.Fprintf(&b, ", %d unattested: %s", len(reasons), strings.Join(reasons, ", "))
 	}
 	b.WriteString(")\n")
-	fmt.Fprintf(&b, "- Kills: %d verified, %d stale, %d pending, %d unbound, %d unattested\n", verified, stale, pending, unbound, unattested)
+	fmt.Fprintf(&b, "- Kills: %d verified, %d stale, %d pending, %d unbound, %d unattested\n", total.Verified, total.Stale, total.Pending, total.Unbound, total.Unattested)
 	if len(deferrals) > 0 {
 		fmt.Fprintf(&b, "- Deferrals: %s\n", strings.Join(deferrals, "; "))
 	}
@@ -159,17 +206,32 @@ func section(list []ReportFreshness, mode string) string {
 		sort.Strings(unverifiable)
 		fmt.Fprintf(&b, "- Freshness not verifiable: %s\n", strings.Join(unverifiable, ", "))
 	}
+	if len(views) > 0 {
+		b.WriteString("\n### Views\n\n| View | Verified | Stale | Pending counted | Pending inherited | Unbound | Unattested |\n| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
+		for i, v := range views {
+			t := perView[i]
+			fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %d | %d |\n", v, t.Verified, t.Stale, t.Pending-t.PendingInherited, t.PendingInherited, t.Unbound, t.Unattested)
+		}
+	}
 	if len(rows) > 0 {
-		keys := make([][2]string, 0, len(rows))
+		keys := make([][3]string, 0, len(rows))
 		for k := range rows {
 			keys = append(keys, k)
 		}
 		sort.Slice(keys, func(i, j int) bool {
-			return keys[i][0]+"\x00"+keys[i][1] < keys[j][0]+"\x00"+keys[j][1]
+			return strings.Join(keys[i][:], "\x00") < strings.Join(keys[j][:], "\x00")
 		})
-		b.WriteString("\n### Re-run list\n\n| File | Cause | Stale kills |\n| --- | --- | ---: |\n")
+		if len(views) == 0 {
+			b.WriteString("\n### Re-run list\n\n| File | Cause | Stale kills |\n| --- | --- | ---: |\n")
+		} else {
+			b.WriteString("\n### Re-run list\n\n| View | File | Cause | Stale kills |\n| --- | --- | --- | ---: |\n")
+		}
 		for _, k := range keys {
-			fmt.Fprintf(&b, "| `%s` | `%s` | %d |\n", k[0], k[1], rows[k])
+			if len(views) == 0 {
+				fmt.Fprintf(&b, "| `%s` | `%s` | %d |\n", k[1], k[2], rows[k])
+			} else {
+				fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | %d |\n", k[0], k[1], k[2], rows[k])
+			}
 		}
 	}
 	return strings.TrimRight(b.String(), "\n")
diff --git a/internal/mutationfresh/build_test.go b/internal/mutationfresh/build_test.go
index cd3548a..969ded0 100644
--- a/internal/mutationfresh/build_test.go
+++ b/internal/mutationfresh/build_test.go
@@ -34,7 +34,7 @@ func buildAt(t *testing.T, root, mode string, reports ...string) Result {
 		}
 		loaded = append(loaded, r)
 	}
-	res, err := Build(loaded, Worktree(root), mode)
+	res, err := Build(loaded, Worktree(root), mode, nil)
 	if err != nil {
 		t.Fatal(err)
 	}
@@ -120,7 +120,7 @@ func TestBuildSection(t *testing.T) {
 	if res.Section != want {
 		t.Errorf("section:\n%s\n--- want ---\n%s", res.Section, want)
 	}
-	if empty, _ := Build(nil, Worktree(root), Advisory); empty.Section != "" || len(empty.Findings) != 0 {
+	if empty, _ := Build(nil, Worktree(root), Advisory, nil); empty.Section != "" || len(empty.Findings) != 0 {
 		t.Errorf("no reports, no section: %+v", empty)
 	}
 }
@@ -143,7 +143,7 @@ func TestBuildOrdersFindingsAndRows(t *testing.T) {
 func TestBuildStopsOnContentErrors(t *testing.T) {
 	report, _ := realCase(t, "full")
 	r, _ := mutation.Load(report)
-	if _, err := Build([]mutation.Report{r}, Worktree(t.TempDir()), Advisory); err == nil {
+	if _, err := Build([]mutation.Report{r}, Worktree(t.TempDir()), Advisory, nil); err == nil {
 		t.Error("a content error stops the review")
 	}
 }
diff --git a/internal/mutationfresh/classify.go b/internal/mutationfresh/classify.go
index 166b657..22531bd 100644
--- a/internal/mutationfresh/classify.go
+++ b/internal/mutationfresh/classify.go
@@ -9,20 +9,39 @@ import (
 // ReportFreshness is one report's classification (spec §6.3). It is serialized into pr-ready's
 // reviewer-input digest (§6.7), so it holds no machine-dependent path.
 type ReportFreshness struct {
-	Path              string       `json:"-"`
-	Engine            string       `json:"engine"`
-	Attested          bool         `json:"attested"`
-	UnattestedReason  string       `json:"unattestedReason,omitempty"`
-	ReportSha256      string       `json:"reportSha256"`
-	AttestationSha256 string       `json:"attestationSha256,omitempty"`
-	Verified          int          `json:"verified"`
-	Stale             int          `json:"stale"`
-	Pending           int          `json:"pending"`
-	Unbound           int          `json:"unbound"`
-	Unattested        int          `json:"unattested"`
-	Causes            []CauseCount `json:"staleCauses,omitempty"`
-	ReRun             []ReRunRow   `json:"reRun,omitempty"`
-	Deferrals         []string     `json:"deferrals,omitempty"`
+	Path              string `json:"-"`
+	Engine            string `json:"engine"`
+	Attested          bool   `json:"attested"`
+	UnattestedReason  string `json:"unattestedReason,omitempty"`
+	ReportSha256      string `json:"reportSha256"`
+	AttestationSha256 string `json:"attestationSha256,omitempty"`
+	Tally
+	Deferrals []string `json:"deferrals,omitempty"`
+	// Views (spec §11.3) is one classification per requested view, in request order; nil when the
+	// run requested none. ViewNames are the names in the report's own map (nil without one).
+	Views     []ViewFreshness `json:"views,omitempty"`
+	ViewNames []string        `json:"-"`
+}
+
+// Tally is the kill classes of one report or one view of it. Pending counts every pending kill;
+// PendingInherited is the part covered only by inherited deferrals (§11.2).
+type Tally struct {
+	Verified         int          `json:"verified"`
+	Stale            int          `json:"stale"`
+	Pending          int          `json:"pending"`
+	PendingInherited int          `json:"pendingInherited,omitempty"`
+	Unbound          int          `json:"unbound"`
+	Unattested       int          `json:"unattested"`
+	Causes           []CauseCount `json:"staleCauses,omitempty"`
+	ReRun            []ReRunRow   `json:"reRun,omitempty"`
+}
+
+// ViewFreshness is one view's classification. Scoped is false when the report has no view map (an
+// unattested report, or one attested before views), in which case every kill counts.
+type ViewFreshness struct {
+	View   string `json:"view"`
+	Scoped bool   `json:"scoped"`
+	Tally
 }
 
 // CauseCount is one recorded cause: the changed path, its current digest and the kills it stales.
@@ -39,12 +58,66 @@ type ReRunRow struct {
 	Kills int    `json:"kills"`
 }
 
-type change struct{ category, digest string }
+type change struct {
+	category, digest string
+	data             []byte // current content; nil when absent
+}
+
+// tallier accumulates one Tally, with its stale causes and re-run rows.
+type tallier struct {
+	t     Tally
+	stale map[string]int
+	rows  map[ReRunRow]int
+}
+
+func newTallier() *tallier {
+	return &tallier{stale: map[string]int{}, rows: map[ReRunRow]int{}}
+}
+
+const (
+	classVerified = iota
+	classStale
+	classPending
+	classPendingInherited
+	classUnbound
+)
+
+func (a *tallier) add(file string, class int, cause string) {
+	switch class {
+	case classStale:
+		a.t.Stale++
+		a.stale[cause]++
+		a.rows[ReRunRow{File: file, Cause: cause}]++
+	case classPendingInherited:
+		a.t.PendingInherited++
+		a.t.Pending++
+	case classPending:
+		a.t.Pending++
+	case classUnbound:
+		a.t.Unbound++
+	default:
+		a.t.Verified++
+	}
+}
+
+func (a *tallier) finish(changed map[string]change) Tally {
+	for _, cause := range sortedKeys(a.stale) {
+		a.t.Causes = append(a.t.Causes, CauseCount{Cause: cause, Digest: changed[cause].digest, Kills: a.stale[cause]})
+	}
+	for row, kills := range a.rows {
+		a.t.ReRun = append(a.t.ReRun, ReRunRow{File: row.File, Cause: row.Cause, Kills: kills})
+	}
+	sort.Slice(a.t.ReRun, func(i, j int) bool {
+		return a.t.ReRun[i].File+"\x00"+a.t.ReRun[i].Cause < a.t.ReRun[j].File+"\x00"+a.t.ReRun[j].Cause
+	})
+	return a.t
+}
 
 // Classify reads the report's attestation and gives every kill exactly one class (spec §6.3):
 // stale (first cause wins), pending, unbound or verified. Kills in an unattested report are
-// unattested. Survivors and every other status are never reclassified.
-func Classify(r mutation.Report, content Content) (ReportFreshness, error) {
+// unattested. Survivors and every other status are never reclassified. Each requested view tallies
+// the kills in the files its patterns match (§11.3), or every kill when the report has no map.
+func Classify(r mutation.Report, content Content, views []string) (ReportFreshness, error) {
 	out := ReportFreshness{Path: r.Target, Engine: r.Engine, ReportSha256: r.SHA256}
 	att, reason, attSHA := readAttestation(r.Target, r.Engine, r.SHA256)
 	out.AttestationSha256 = attSHA
@@ -54,9 +127,15 @@ func Classify(r mutation.Report, content Content) (ReportFreshness, error) {
 	if reason != "" {
 		out.UnattestedReason = reason
 		out.Unattested = r.Score().Killed
+		for _, v := range views {
+			out.Views = append(out.Views, ViewFreshness{View: v, Tally: Tally{Unattested: out.Unattested}})
+		}
 		return out, nil
 	}
 	out.Attested = true
+	if att.Views != nil {
+		out.ViewNames = sortedKeys(att.Views)
+	}
 	for _, d := range att.Deferrals {
 		out.Deferrals = append(out.Deferrals, d.Reason)
 	}
@@ -65,42 +144,38 @@ func Classify(r mutation.Report, content Content) (ReportFreshness, error) {
 		return ReportFreshness{}, err
 	}
 	causeOf := causeFinder(r.Detail, changed)
-	stale := map[string]int{}
-	rows := map[ReRunRow]int{}
+	total := newTallier()
+	perView := make([]*tallier, len(views))
+	for i := range views {
+		perView[i] = newTallier()
+	}
 	for _, file := range sortedKeys(r.Detail.Files) {
 		for _, m := range r.Detail.Files[file].Mutants {
 			if m.Status != mutation.Killed {
 				continue
 			}
-			if cause := causeOf(file, m.KilledBy); cause != "" {
-				out.Stale++
-				stale[cause]++
-				rows[ReRunRow{File: file, Cause: cause}]++
-				continue
-			}
+			class, cause := classVerified, causeOf(file, m.KilledBy)
 			_, attested := att.Files[file]
-			switch {
-			case deferred(att.Deferrals, file):
-				out.Pending++
+			switch kind := deferred(att.Deferrals, file); {
+			case cause != "":
+				class = classStale
+			case kind != classVerified:
+				class = kind
 			case !attested || len(m.KilledBy) == 0:
-				out.Unbound++
-			default:
-				out.Verified++
+				class = classUnbound
+			}
+			total.add(file, class, cause)
+			for i, v := range views {
+				if att.Views == nil || MatchList(file, att.Views[v]) {
+					perView[i].add(file, class, cause)
+				}
 			}
 		}
 	}
-	for _, cause := range sortedKeys(stale) {
-		out.Causes = append(out.Causes, CauseCount{Cause: cause, Digest: changed[cause].digest, Kills: stale[cause]})
+	out.Tally = total.finish(changed)
+	for i, v := range views {
+		out.Views = append(out.Views, ViewFreshness{View: v, Scoped: att.Views != nil, Tally: perView[i].finish(changed)})
 	}
-	for row, kills := range rows {
-		out.ReRun = append(out.ReRun, ReRunRow{File: row.File, Cause: row.Cause, Kills: kills})
-	}
-	sort.Slice(out.ReRun, func(i, j int) bool {
-		if out.ReRun[i].File != out.ReRun[j].File {
-			return out.ReRun[i].File < out.ReRun[j].File
-		}
-		return out.ReRun[i].Cause < out.ReRun[j].Cause
-	})
 	return out, nil
 }
 
@@ -140,13 +215,15 @@ func changedPaths(att Attestation, content Content) (map[string]change, error) {
 		}
 		entry, attested := att.Files[p]
 		if !attested {
-			changed[p] = change{category: Categorize(p, att.Lists), digest: digest}
+			if digest != Absent { // a listed path removed from disk is not present (§6.3)
+				changed[p] = change{category: Categorize(p, att.Lists), digest: digest}
+			}
 			continue
 		}
 		if digest == entry.Digest || (content.Head() && !entry.Tracked && digest == Absent) {
 			continue
 		}
-		changed[p] = change{category: entry.Category, digest: digest}
+		changed[p] = change{category: entry.Category, digest: digest, data: entries[p].Data}
 	}
 	return changed, nil
 }
@@ -154,7 +231,8 @@ func changedPaths(att Attestation, content Content) (map[string]change, error) {
 // causeFinder returns a kill's recorded cause, in the §6.3 order: its own file; the byte-smallest
 // changed test whose ids it was killed by; the byte-smallest changed mutate file with mutants whose
 // coverage includes a killing test; else the byte-smallest changed support, global, unclassified or
-// zero-mutant mutate path (the gate has no import graph, so these invalidate every kill).
+// mutate path with no mutants or with an edit outside every mutant (§11.5): the gate has no import
+// graph, so these invalidate every kill.
 func causeFinder(d *mutation.StrykerDetail, changed map[string]change) func(file string, killedBy []string) string {
 	testOf := map[string]string{}
 	type covering struct {
@@ -171,7 +249,7 @@ func causeFinder(d *mutation.StrykerDetail, changed map[string]change) func(file
 					testOf[id] = p
 				}
 			}
-		case c.category == "mutate" && len(d.Files[p].Mutants) > 0:
+		case c.category == "mutate" && len(d.Files[p].Mutants) > 0 && !outOfMutant(d.Files[p], c):
 			ids := map[string]bool{}
 			for _, m := range d.Files[p].Mutants {
 				for _, id := range m.CoveredBy {
@@ -210,15 +288,45 @@ func causeFinder(d *mutation.StrykerDetail, changed map[string]change) func(file
 	}
 }
 
-func deferred(deferrals []Deferral, file string) bool {
+// outOfMutant: the file's edit has a hunk that no mutant of any status intersects, or cannot be
+// diffed at all. Mutant spans are lines of the source Stryker read, the diff's old side. A deleted
+// file is changed everywhere, which its mutants intersect.
+func outOfMutant(file mutation.StrykerFile, c change) bool {
+	if c.digest == Absent {
+		return false
+	}
+	hunks, ok := lineDiff(file.Source, string(c.data))
+	if !ok {
+		return true
+	}
+	for _, h := range hunks {
+		hit := false
+		for _, m := range file.Mutants {
+			hit = hit || hunkIntersects(m.StartLine, m.EndLine, h)
+		}
+		if !hit {
+			return true
+		}
+	}
+	return false
+}
+
+// deferred is the pending class of a kill in file: classPending when any counted deferral covers
+// it, else classPendingInherited when an inherited one does (§11.2), else classVerified (none).
+func deferred(deferrals []Deferral, file string) int {
+	kind := classVerified
 	for _, d := range deferrals {
 		for _, p := range d.Paths {
-			if p == "*" || p == file {
-				return true
+			if p != "*" && p != file {
+				continue
 			}
+			if !d.Inherited {
+				return classPending
+			}
+			kind = classPendingInherited
 		}
 	}
-	return false
+	return kind
 }
 
 func sortedKeys[V any](m map[string]V) []string {
diff --git a/internal/mutationfresh/classify_test.go b/internal/mutationfresh/classify_test.go
index a82cd61..314e5c9 100644
--- a/internal/mutationfresh/classify_test.go
+++ b/internal/mutationfresh/classify_test.go
@@ -70,7 +70,7 @@ func classifyAt(t *testing.T, report, root string) ReportFreshness {
 	if err != nil {
 		t.Fatal(err)
 	}
-	f, err := Classify(r, Worktree(root))
+	f, err := Classify(r, Worktree(root), nil)
 	if err != nil {
 		t.Fatal(err)
 	}
@@ -176,7 +176,7 @@ func TestClassifySyntheticRules(t *testing.T) {
 	}
 	// HEAD mode: an untracked attested path absent from HEAD is skipped; everything is unchanged.
 	r, _ := mutation.Load(path)
-	head, err := Classify(r, Head(root))
+	head, err := Classify(r, Head(root), nil)
 	if err != nil {
 		t.Fatal(err)
 	}
@@ -190,7 +190,7 @@ func TestClassifySyntheticRules(t *testing.T) {
 	write(t, root, "tests/new.test.ts", "n") // nor can a new test
 	write(t, root, "notes.md", "n")          // ignored
 	r, _ = mutation.Load(path)
-	f, _ = Classify(r, Head(root))
+	f, _ = Classify(r, Head(root), nil)
 	if f.Stale != 0 {
 		t.Errorf("uncommitted edits are not seen in HEAD mode: %+v", f)
 	}
@@ -223,7 +223,7 @@ func TestClassifyUnattestedAndGremlins(t *testing.T) {
 	dir := t.TempDir()
 	write(t, dir, "incremental.json", `{"files":{"a.ts":{"mutants":[{"id":"1","mutatorName":"M","status":"Killed","location":{"start":{"line":1}}}]}}}`)
 	r, _ := mutation.Load(filepath.Join(dir, "incremental.json"))
-	f, err := Classify(r, Worktree(t.TempDir()))
+	f, err := Classify(r, Worktree(t.TempDir()), nil)
 	if err != nil || f.Attested || f.UnattestedReason != "missing" || f.Unattested != 1 {
 		t.Errorf("unattested stryker: %+v %v", f, err)
 	}
@@ -231,7 +231,7 @@ func TestClassifyUnattestedAndGremlins(t *testing.T) {
 	g := mutation.Report{Engine: "stryker", Target: filepath.Join(dir, "incremental.json"), SHA256: r.SHA256,
 		Mutants: []mutation.Mutant{{Status: mutation.Killed}}}
 	writeAttestation(t, dir, validAttestation(r.SHA256))
-	f, _ = Classify(g, Worktree(t.TempDir()))
+	f, _ = Classify(g, Worktree(t.TempDir()), nil)
 	if f.UnattestedReason != "engine" {
 		t.Errorf("no detail: %+v", f)
 	}
@@ -240,14 +240,14 @@ func TestClassifyUnattestedAndGremlins(t *testing.T) {
 func TestClassifyStopsOnContentErrors(t *testing.T) {
 	report, _ := realCase(t, "full")
 	r, _ := mutation.Load(report)
-	if _, err := Classify(r, Worktree(t.TempDir())); err == nil {
+	if _, err := Classify(r, Worktree(t.TempDir()), nil); err == nil {
 		t.Error("listing paths outside a repository stops the review")
 	}
 	root := gitRepo(t, map[string]string{"src/a.ts": "a"})
 	if err := os.Chmod(filepath.Join(root, "src/a.ts"), 0o000); err != nil {
 		t.Fatal(err)
 	}
-	if _, err := Classify(r, Worktree(root)); err == nil {
+	if _, err := Classify(r, Worktree(root), nil); err == nil {
 		t.Error("an unreadable attested path stops the review")
 	}
 }
diff --git a/internal/mutationfresh/diff.go b/internal/mutationfresh/diff.go
new file mode 100644
index 0000000..8497b66
--- /dev/null
+++ b/internal/mutationfresh/diff.go
@@ -0,0 +1,133 @@
+package mutationfresh
+
+import "strings"
+
+// Content-based line diff (spec §11.4.1), a port of templates/mutation-incremental/lib/diff.mjs.
+// Both implementations run testdata/mutation-incremental/diff-vectors.json, so a change here needs
+// the same change there. Above maxEditDistance lineDiff gives up and the caller treats the file as
+// changed everywhere.
+const maxEditDistance = 2000
+
+// hunk is one changed region in old and new line numbers (1-based, inclusive). A pure insertion
+// has OldEnd == OldStart-1; a pure deletion has NewEnd == NewStart-1.
+type hunk struct {
+	OldStart int `json:"oldStart"`
+	OldEnd   int `json:"oldEnd"`
+	NewStart int `json:"newStart"`
+	NewEnd   int `json:"newEnd"`
+}
+
+func splitLines(text string) []string {
+	text = strings.TrimPrefix(text, "\uFEFF")
+	text = strings.ReplaceAll(text, "\r\n", "\n")
+	text = strings.ReplaceAll(text, "\r", "\n")
+	if text == "" {
+		return nil
+	}
+	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
+}
+
+// editOps is Myers' O(ND) shortest edit script with the harness's tie rule: the insertion ("down")
+// move when k == -d or v[k-1] < v[k+1], otherwise the deletion ("right") move. trace[d] keeps v for
+// diagonals -d-1 .. d+1 as it was before step d.
+func editOps(a, b []string) ([]byte, bool) {
+	n, m := len(a), len(b)
+	off := n + m + 1
+	v := make([]int, 2*(n+m)+3)
+	var trace [][]int
+	for d := 0; d <= maxEditDistance; d++ {
+		trace = append(trace, append([]int(nil), v[off-d-1:off+d+2]...))
+		for k := -d; k <= d; k += 2 {
+			x := v[off+k-1] + 1
+			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
+				x = v[off+k+1]
+			}
+			y := x - k
+			for x < n && y < m && a[x] == b[y] {
+				x++
+				y++
+			}
+			v[off+k] = x
+			if x >= n && y >= m {
+				return backtrack(trace, n, m), true
+			}
+		}
+	}
+	return nil, false
+}
+
+func backtrack(trace [][]int, n, m int) []byte {
+	var ops []byte
+	x, y := n, m
+	for d := len(trace) - 1; d >= 0; d-- {
+		w := trace[d]
+		at := func(k int) int { return w[k+d+1] }
+		k := x - y
+		prevK := k - 1
+		if k == -d || (k != d && at(k-1) < at(k+1)) {
+			prevK = k + 1
+		}
+		prevX := at(prevK)
+		prevY := prevX - prevK
+		for x > prevX && y > prevY {
+			ops = append(ops, '=')
+			x--
+			y--
+		}
+		if d > 0 {
+			if x == prevX {
+				ops = append(ops, '+')
+			} else {
+				ops = append(ops, '-')
+			}
+		}
+		x, y = prevX, prevY
+	}
+	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
+		ops[i], ops[j] = ops[j], ops[i]
+	}
+	return ops
+}
+
+// lineDiff returns the changed hunks between two texts after BOM and line-ending normalisation, or
+// false when the edit distance exceeds maxEditDistance.
+func lineDiff(oldText, newText string) ([]hunk, bool) {
+	ops, ok := editOps(splitLines(oldText), splitLines(newText))
+	if !ok {
+		return nil, false
+	}
+	var hunks []hunk
+	i, j := 0, 0
+	open := false
+	for _, op := range ops {
+		if op == '=' {
+			i++
+			j++
+			open = false
+			continue
+		}
+		if !open {
+			hunks = append(hunks, hunk{OldStart: i + 1, OldEnd: i, NewStart: j + 1, NewEnd: j})
+			open = true
+		}
+		cur := &hunks[len(hunks)-1]
+		if op == '-' {
+			i++
+			cur.OldEnd = i
+		} else {
+			j++
+			cur.NewEnd = j
+		}
+	}
+	return hunks, true
+}
+
+// hunkIntersects (spec §11.4.3): a mutant [start, end] (old lines) intersects a changed hunk when
+// the ranges overlap, and a pure insertion when it contains the insertion point strictly inside.
+func hunkIntersects(start, end int, h hunk) bool {
+	if h.OldEnd >= h.OldStart {
+		return start <= h.OldEnd && end >= h.OldStart
+	}
+	after := h.OldStart - 1
+	return start <= after && end >= after+1
+}
diff --git a/internal/mutationfresh/diff_test.go b/internal/mutationfresh/diff_test.go
new file mode 100644
index 0000000..787287e
--- /dev/null
+++ b/internal/mutationfresh/diff_test.go
@@ -0,0 +1,70 @@
+package mutationfresh
+
+import (
+	"encoding/json"
+	"fmt"
+	"os"
+	"reflect"
+	"strings"
+	"testing"
+)
+
+// The harness runs the same vectors (templates/mutation-incremental/lib/diff.mjs), so the gate and
+// the planner agree on which lines an edit touched.
+func TestLineDiffVectors(t *testing.T) {
+	data, err := os.ReadFile("../../testdata/mutation-incremental/diff-vectors.json")
+	if err != nil {
+		t.Fatal(err)
+	}
+	var vectors struct {
+		MaxEditDistance int `json:"maxEditDistance"`
+		Cases           []struct {
+			Name  string `json:"name"`
+			Old   string `json:"old"`
+			New   string `json:"new"`
+			Hunks []hunk `json:"hunks"`
+		} `json:"cases"`
+	}
+	if err := json.Unmarshal(data, &vectors); err != nil {
+		t.Fatal(err)
+	}
+	if vectors.MaxEditDistance != maxEditDistance {
+		t.Fatalf("maxEditDistance %d, vectors say %d", maxEditDistance, vectors.MaxEditDistance)
+	}
+	for _, c := range vectors.Cases {
+		got, ok := lineDiff(c.Old, c.New)
+		if !ok || len(got)+len(c.Hunks) > 0 && !reflect.DeepEqual(got, c.Hunks) {
+			t.Errorf("%s: got %+v (%v), want %+v", c.Name, got, ok, c.Hunks)
+		}
+	}
+}
+
+func TestLineDiffGivesUpAboveTheEditDistance(t *testing.T) {
+	var a, b strings.Builder
+	for i := 0; i <= maxEditDistance; i++ {
+		fmt.Fprintf(&a, "a%d\n", i)
+		fmt.Fprintf(&b, "b%d\n", i)
+	}
+	if _, ok := lineDiff(a.String(), b.String()); ok {
+		t.Error("a diff above the edit distance must give up")
+	}
+}
+
+// Spec §11.4.3: ranges overlap for a changed hunk; a pure insertion intersects a mutant that spans
+// the insertion point.
+func TestHunkIntersects(t *testing.T) {
+	for _, c := range []struct {
+		start, end int
+		h          hunk
+		want       bool
+	}{
+		{1, 5, hunk{OldStart: 2, OldEnd: 2, NewStart: 2, NewEnd: 2}, true},
+		{3, 3, hunk{OldStart: 2, OldEnd: 2, NewStart: 2, NewEnd: 2}, false},
+		{1, 5, hunk{OldStart: 6, OldEnd: 5, NewStart: 6, NewEnd: 6}, false},
+		{1, 5, hunk{OldStart: 3, OldEnd: 2, NewStart: 3, NewEnd: 3}, true},
+	} {
+		if got := hunkIntersects(c.start, c.end, c.h); got != c.want {
+			t.Errorf("[%d,%d] %+v = %v", c.start, c.end, c.h, got)
+		}
+	}
+}
diff --git a/internal/mutationfresh/outofmutant_test.go b/internal/mutationfresh/outofmutant_test.go
new file mode 100644
index 0000000..b1047fa
--- /dev/null
+++ b/internal/mutationfresh/outofmutant_test.go
@@ -0,0 +1,65 @@
+package mutationfresh
+
+import (
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+// Spec §11.5: an edit that intersects no mutant of the file (by the §11.4.3 rule) may change what
+// the file exports without touching a mutant, so it is a blanket cause, like a zero-mutant file.
+func TestOutOfMutantHunkIsABlanketCause(t *testing.T) {
+	for path, text := range map[string]string{
+		// line 5 is the blank line between inc (1–4) and dec (6–9)
+		"src/e.ts": "export function inc(n: number): number {\n  const r = n + 1;\n  return r;\n}\n// between\nexport function dec(n: number): number {\n  const r = n - 1;\n  return r;\n}\n",
+		// a pure insertion after the block mutant [1,5] is outside it
+		"src/a.ts": "export function grade(score: number, pass: number, top: number): string {\n  if (score < pass) return 'fail';\n  if (score > top) return 'over';\n  return 'ok';\n}\n// trailing note\n",
+	} {
+		report, root := realCase(t, "full")
+		write(t, root, path, text)
+		f := classifyAt(t, report, root)
+		if f.Stale != 24 || causes(f)[path] != 24 {
+			t.Errorf("%s: %+v", path, f)
+		}
+	}
+}
+
+// Beyond the edit distance the gate cannot tell which lines changed, so the file is a blanket cause.
+func TestUndiffableMutateFileIsABlanketCause(t *testing.T) {
+	report, root := realCase(t, "full")
+	big := ""
+	for i := 0; i <= maxEditDistance; i++ {
+		big += "// filler\n"
+	}
+	write(t, root, "src/c.ts", big)
+	if f := classifyAt(t, report, root); f.Stale != 24 || causes(f)["src/c.ts"] != 24 {
+		t.Errorf("undiffable c.ts: %+v", f)
+	}
+}
+
+// A deleted mutate file keeps 2a's rule: its own kills and those of the tests covering it.
+func TestDeletedMutateFileIsNotBlanket(t *testing.T) {
+	report, root := realCase(t, "full")
+	if err := os.Remove(filepath.Join(root, "src/c.ts")); err != nil {
+		t.Fatal(err)
+	}
+	f := classifyAt(t, report, root)
+	if f.Stale < 3 || f.Stale == 24 || causes(f)["src/c.ts"] != f.Stale {
+		t.Errorf("deleted c.ts: %+v", f)
+	}
+}
+
+// Spec §6.3 counts present paths: a tracked, unattested file removed with rm (still in the index)
+// is not a change.
+func TestDeletedUnattestedPathIsNotAChange(t *testing.T) {
+	report, root := realCase(t, "full")
+	write(t, root, "tests/helpers/old.ts", "export const old = 1;\n")
+	git(t, root, "add", "-A")
+	git(t, root, "commit", "-qm", "old helper")
+	if err := os.Remove(filepath.Join(root, "tests/helpers/old.ts")); err != nil {
+		t.Fatal(err)
+	}
+	if f := classifyAt(t, report, root); f.Stale != 0 || f.Verified != 24 {
+		t.Errorf("rm'd helper: %+v", f)
+	}
+}
diff --git a/internal/mutationfresh/views.go b/internal/mutationfresh/views.go
new file mode 100644
index 0000000..c32bafe
--- /dev/null
+++ b/internal/mutationfresh/views.go
@@ -0,0 +1,40 @@
+package mutationfresh
+
+import (
+	"fmt"
+	"regexp"
+	"slices"
+	"strings"
+
+	"github.com/dsifry/metareview/internal/mutation"
+)
+
+var viewName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
+
+// CheckViews validates --mutation-view names against the supplied reports (spec §11.3): every name
+// matches [A-Za-z0-9._-]+, and appears in the view map of every attested report that has one. An
+// unattested report, or one attested before views, is not scoped and accepts any name. An error is
+// a usage error (exit 2).
+func CheckViews(reports []mutation.Report, views []string) error {
+	for _, v := range views {
+		if !viewName.MatchString(v) {
+			return fmt.Errorf("invalid mutation view name %q", v)
+		}
+	}
+	for _, r := range reports {
+		if r.Detail == nil {
+			continue
+		}
+		att, reason, _ := readAttestation(r.Target, r.Engine, r.SHA256)
+		if reason != "" || att.Views == nil {
+			continue
+		}
+		names := sortedKeys(att.Views)
+		for _, v := range views {
+			if !slices.Contains(names, v) {
+				return fmt.Errorf("mutation view %q is not in the views of %s (%s)", v, r.Target, strings.Join(names, ", "))
+			}
+		}
+	}
+	return nil
+}
diff --git a/internal/mutationfresh/views_test.go b/internal/mutationfresh/views_test.go
new file mode 100644
index 0000000..f5a8542
--- /dev/null
+++ b/internal/mutationfresh/views_test.go
@@ -0,0 +1,226 @@
+package mutationfresh
+
+import (
+	"encoding/json"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/mutation"
+)
+
+// The e2e fixture's views: core = src/**/*.ts (all 24 kills), edge = src/c.ts, src/e.ts (3 + 4).
+var bothViews = []string{"core", "edge"}
+
+const editedC = "export function clamp(n: number): number {\n  return n;\n}\n"
+
+func classifyViews(t *testing.T, report, root string, views []string) ReportFreshness {
+	t.Helper()
+	r, err := mutation.Load(report)
+	if err != nil {
+		t.Fatal(err)
+	}
+	f, err := Classify(r, Worktree(root), views)
+	if err != nil {
+		t.Fatal(err)
+	}
+	return f
+}
+
+func editAttestation(t *testing.T, report string, edit func(map[string]any)) {
+	t.Helper()
+	path := filepath.Join(filepath.Dir(report), "attestation.json")
+	data, _ := os.ReadFile(path)
+	var att map[string]any
+	if err := json.Unmarshal(data, &att); err != nil {
+		t.Fatal(err)
+	}
+	edit(att)
+	out, _ := json.Marshal(att)
+	if err := os.WriteFile(path, out, 0o644); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func view(f ReportFreshness, name string) ViewFreshness {
+	for _, v := range f.Views {
+		if v.View == name {
+			return v
+		}
+	}
+	return ViewFreshness{}
+}
+
+// Spec §11.3: each view has its own classification; views may overlap and are never summed, and
+// the report's own totals are unchanged.
+func TestViewsOverlapAndAreNotSummed(t *testing.T) {
+	report, root := realCase(t, "full")
+	f := classifyViews(t, report, root, bothViews)
+	if f.Verified != 24 || !view(f, "core").Scoped || view(f, "core").Verified != 24 || view(f, "edge").Verified != 7 {
+		t.Fatalf("unchanged: %+v", f)
+	}
+	if strings.Join(f.ViewNames, ",") != "core,edge" {
+		t.Errorf("view names %v", f.ViewNames)
+	}
+	write(t, root, "src/c.ts", editedC)
+	f = classifyViews(t, report, root, bothViews)
+	core, edge := view(f, "core"), view(f, "edge")
+	if f.Stale != 3 || core.Stale != 3 || core.Verified != 21 || edge.Stale != 3 || edge.Verified != 4 {
+		t.Errorf("edited c.ts: report %+v core %+v edge %+v", f.Tally, core, edge)
+	}
+	if len(edge.Causes) != 1 || edge.Causes[0].Cause != "src/c.ts" || len(edge.ReRun) != 1 {
+		t.Errorf("edge causes %+v rows %+v", edge.Causes, edge.ReRun)
+	}
+	if none := classifyViews(t, report, root, nil); none.Views != nil {
+		t.Errorf("no views requested: %+v", none.Views)
+	}
+}
+
+// An attested report without views (attested before they were adopted) is not scoped.
+func TestViewOnReportWithoutViewsIsUnscoped(t *testing.T) {
+	report, root := realCase(t, "full")
+	editAttestation(t, report, func(att map[string]any) { delete(att, "views") })
+	f := classifyViews(t, report, root, []string{"edge"})
+	if v := view(f, "edge"); v.Scoped || v.Verified != 24 || f.ViewNames != nil {
+		t.Errorf("unscoped: %+v", f)
+	}
+}
+
+func TestUnattestedReportUnderAView(t *testing.T) {
+	report, root := realCase(t, "full")
+	if err := os.Remove(filepath.Join(filepath.Dir(report), "attestation.json")); err != nil {
+		t.Fatal(err)
+	}
+	f := classifyViews(t, report, root, []string{"core"})
+	if v := view(f, "core"); v.Unattested != 24 || v.Scoped {
+		t.Errorf("unattested: %+v", f)
+	}
+}
+
+// Spec §11.2/§11.3: pending kills split by the deferral's inherited flag, counted winning a tie.
+func TestPendingSplitsCountedAndInherited(t *testing.T) {
+	for _, c := range []struct {
+		name      string
+		deferrals []any
+		inherited int
+	}{
+		{"inherited only", []any{map[string]any{"reason": "timeout", "paths": []string{"src/c.ts"}, "inherited": true}}, 3},
+		{"counted wins", []any{
+			map[string]any{"reason": "timeout", "paths": []string{"src/c.ts"}, "inherited": true},
+			map[string]any{"reason": "global", "paths": []string{"src/c.ts"}, "inherited": false},
+		}, 0},
+	} {
+		report, root := realCase(t, "full")
+		editAttestation(t, report, func(att map[string]any) { att["deferrals"] = c.deferrals })
+		f := classifyViews(t, report, root, []string{"edge"})
+		if f.Pending != 3 || f.PendingInherited != c.inherited || view(f, "edge").PendingInherited != c.inherited {
+			t.Errorf("%s: %+v", c.name, f)
+		}
+	}
+}
+
+func TestCheckViews(t *testing.T) {
+	report, _ := realCase(t, "full")
+	r, err := mutation.Load(report)
+	if err != nil {
+		t.Fatal(err)
+	}
+	g := mutation.Report{Engine: "gremlins", Target: "g.json"}
+	for _, c := range []struct {
+		views []string
+		want  string
+	}{
+		{nil, ""},
+		{[]string{"core", "edge"}, ""},
+		{[]string{"nope"}, `mutation view "nope" is not in the views of ` + report + " (core, edge)"},
+		{[]string{"bad name!"}, `invalid mutation view name "bad name!"`},
+		{[]string{""}, `invalid mutation view name ""`},
+	} {
+		err := CheckViews([]mutation.Report{g, r}, c.views)
+		if got := errText(err); got != c.want {
+			t.Errorf("%v: %q, want %q", c.views, got, c.want)
+		}
+	}
+	editAttestation(t, report, func(att map[string]any) { delete(att, "views") })
+	if err := CheckViews([]mutation.Report{r}, []string{"anything"}); err != nil {
+		t.Errorf("a report without views is not scoped: %v", err)
+	}
+	if err := os.Remove(filepath.Join(filepath.Dir(report), "attestation.json")); err != nil {
+		t.Fatal(err)
+	}
+	if err := CheckViews([]mutation.Report{r}, []string{"anything"}); err != nil {
+		t.Errorf("an unattested report is not scoped: %v", err)
+	}
+}
+
+func errText(err error) string {
+	if err == nil {
+		return ""
+	}
+	return err.Error()
+}
+
+func buildViews(t *testing.T, report, root, mode string, views []string) Result {
+	t.Helper()
+	r, err := mutation.Load(report)
+	if err != nil {
+		t.Fatal(err)
+	}
+	res, err := Build([]mutation.Report{r}, Worktree(root), mode, views)
+	if err != nil {
+		t.Fatal(err)
+	}
+	return res
+}
+
+// Spec §11.3: one finding per view with the view in its fingerprint and title, and one section with
+// a table row per view and a re-run list with a View column.
+func TestBuildWithViews(t *testing.T) {
+	report, root := realCase(t, "full")
+	write(t, root, "src/c.ts", editedC)
+	res := buildViews(t, report, root, Enforce, bothViews)
+	if len(res.Findings) != 2 {
+		t.Fatalf("findings: %+v", res.Findings)
+	}
+	for i, name := range bothViews {
+		f := res.Findings[i]
+		if f.View != name || f.Title != "Mutation evidence stale ("+name+"): src/c.ts changed" ||
+			!strings.HasPrefix(f.Fingerprint, "mutation:stale:enforce:stryker:"+name+":src/c.ts:") || f.Classification != "blocking" {
+			t.Errorf("finding %d: %+v", i, f)
+		}
+	}
+	want := "## Mutation Evidence Freshness\n\n" +
+		"- Mode: `enforce`\n" +
+		"- Reports: 1 (1 attested)\n" +
+		"- Kills: 21 verified, 3 stale, 0 pending, 0 unbound, 0 unattested\n" +
+		"\n### Views\n\n" +
+		"| View | Verified | Stale | Pending counted | Pending inherited | Unbound | Unattested |\n" +
+		"| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n" +
+		"| `core` | 21 | 3 | 0 | 0 | 0 | 0 |\n" +
+		"| `edge` | 4 | 3 | 0 | 0 | 0 | 0 |\n" +
+		"\n### Re-run list\n\n" +
+		"| View | File | Cause | Stale kills |\n| --- | --- | --- | ---: |\n" +
+		"| `core` | `src/c.ts` | `src/c.ts` | 3 |\n" +
+		"| `edge` | `src/c.ts` | `src/c.ts` | 3 |"
+	if res.Section != want {
+		t.Errorf("section:\n%s\nwant:\n%s", res.Section, want)
+	}
+}
+
+func TestBuildPendingAndUnattestedWithViews(t *testing.T) {
+	report, root := realCase(t, "lockfile") // every kill pending
+	res := buildViews(t, report, root, Advisory, []string{"edge"})
+	if len(res.Findings) != 1 || res.Findings[0].Title != "Mutation evidence pending (edge): 7 kills" ||
+		!strings.HasPrefix(res.Findings[0].Fingerprint, "mutation:pending:advisory:stryker:edge:") {
+		t.Errorf("pending: %+v", res.Findings)
+	}
+	if err := os.Remove(filepath.Join(filepath.Dir(report), "attestation.json")); err != nil {
+		t.Fatal(err)
+	}
+	res = buildViews(t, report, root, Enforce, []string{"edge"})
+	if len(res.Findings) != 1 || res.Findings[0].Title != "Mutation report has no attestation (edge)" || res.Findings[0].View != "edge" ||
+		!strings.HasPrefix(res.Findings[0].Fingerprint, "mutation:unattested:enforce:stryker:edge:") {
+		t.Errorf("unattested: %+v", res.Findings)
+	}
+}
diff --git a/internal/prready/mutationctx_test.go b/internal/prready/mutationctx_test.go
index 4b292d6..f2b6f02 100644
--- a/internal/prready/mutationctx_test.go
+++ b/internal/prready/mutationctx_test.go
@@ -12,7 +12,7 @@ import (
 // A skipped report is indistinguishable from a package with no survivors, so the failure mode of
 // getting this wrong is a gate that passes because it looked at less.
 func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
-	if _, err := mutationContextFor(t.TempDir(), nil, false); err != nil {
+	if _, err := mutationContextFor(t.TempDir(), nil, nil, false); err != nil {
 		t.Errorf("no reports at all is the ordinary case: %v", err)
 	}
 	for name, path := range map[string]string{
@@ -21,7 +21,7 @@ func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
 		"empty file":    writeTemp(t, ""),
 		"files is text": writeTemp(t, `{"files":"none"}`),
 	} {
-		ctx, err := mutationContextFor(t.TempDir(), []string{path}, false)
+		ctx, err := mutationContextFor(t.TempDir(), []string{path}, nil, false)
 		if err == nil {
 			t.Errorf("%s: must be an error, got %d reports", name, len(ctx.Reports))
 			continue
@@ -35,7 +35,7 @@ func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
 // A readable report reaches the reviewers, so the review actually acts on it.
 func TestAReadableMutationReportReachesTheReviewers(t *testing.T) {
 	path := writeTemp(t, `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"type":"T","status":"LIVED","line":3}]}]}`)
-	ctx, err := mutationContextFor(t.TempDir(), []string{path}, false)
+	ctx, err := mutationContextFor(t.TempDir(), []string{path}, nil, false)
 	if err != nil {
 		t.Fatal(err)
 	}
diff --git a/internal/prready/review.go b/internal/prready/review.go
index 6338310..14d95be 100644
--- a/internal/prready/review.go
+++ b/internal/prready/review.go
@@ -47,6 +47,8 @@ type Options struct {
 	// MutationReportPaths are --mutation-report files: a mutation-testing engine's output,
 	// either mutation-testing-report-schema or gremlins JSON. Empty is the ordinary case.
 	MutationReportPaths []string
+	// MutationViews are --mutation-view names (spec §11.3): the evidence is judged per view.
+	MutationViews []string
 	// ShardWriter is the pack-writing seam; nil uses the real filesystem.
 	ShardWriter shardpack.Writer
 }
@@ -330,7 +332,7 @@ func Create(root string, options Options) (Result, error) {
 	if err != nil {
 		return Result{}, err
 	}
-	mutationContext, err := mutationContextFor(root, options.MutationReportPaths, !options.IncludeWorkingTree)
+	mutationContext, err := mutationContextFor(root, options.MutationReportPaths, options.MutationViews, !options.IncludeWorkingTree)
 	if err != nil {
 		return Result{}, err
 	}
@@ -473,10 +475,12 @@ func Create(root string, options Options) (Result, error) {
 			return writeFile(reviewPath, []byte(reviewMarkdown(runID, contextRel, options.PreviousRunID, gateEffect, reused.Verdict, reviewGit.ChangedFiles, nil, prEvidence, joinSections(reviewmanifest.ShardedReviewMarkdown(manifest, aggregate), mutationContext.FreshnessSection), meta)), 0o644)
 		}
 		reconciled, err := reconcileFindings(root, run, rawFindings, findings.Options{
-			PreviousRunID:   options.PreviousRunID,
-			PreviousRunIDs:  previousRunIDs,
-			ResetRunIDs:     chain.ResetRunIDs,
-			MutationEngines: mutationContext.Engines(),
+			PreviousRunID:    options.PreviousRunID,
+			PreviousRunIDs:   previousRunIDs,
+			ResetRunIDs:      chain.ResetRunIDs,
+			MutationEngines:  mutationContext.Engines(),
+			MutationViews:    mutationContext.Views,
+			MutationViewMaps: mutationContext.ViewMaps(),
 		})
 		if err != nil {
 			return err
@@ -1612,8 +1616,8 @@ func shardTargetID(git gitcontext.Context) string {
 // mutationContextFor loads the declared mutation reports. An unreadable or unrecognised report is
 // an error that stops the review, never a skipped file: a mutation gate that quietly drops a
 // report is a gate that passes because it looked at less.
-func mutationContextFor(root string, paths []string, head bool) (reviewers.MutationContext, error) {
-	return reviewers.LoadMutationContext(root, paths, "pr-ready", head)
+func mutationContextFor(root string, paths, views []string, head bool) (reviewers.MutationContext, error) {
+	return reviewers.LoadMutationContext(root, paths, views, "pr-ready", head)
 }
 
 // joinSections joins the non-empty sections that follow the verdict line.
diff --git a/internal/prready/views_test.go b/internal/prready/views_test.go
new file mode 100644
index 0000000..513ddf1
--- /dev/null
+++ b/internal/prready/views_test.go
@@ -0,0 +1,185 @@
+package prready
+
+import (
+	"crypto/sha256"
+	"encoding/hex"
+	"encoding/json"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/findings"
+)
+
+// The §7.1 gate rows (spec §11.3, §11.7) on the real Stryker report of Plan 1c's e2e run, whose view
+// map is core = src/**/*.ts (24 kills) and edge = src/c.ts, src/e.ts (3 + 4), at pr-ready (HEAD,
+// enforce, where freshness can block).
+
+const editedC = "export function clamp(n: number): number {\n  return n;\n}\n"
+
+// fixtureRepo commits the e2e fixture tree onto a small pr-ready repository.
+func fixtureRepo(t *testing.T) string {
+	t.Helper()
+	root := smallPRReadyRepo(t)
+	fixture := "../../testdata/mutation-incremental-e2e"
+	err := filepath.WalkDir(fixture, func(path string, d os.DirEntry, err error) error {
+		if err != nil || d.IsDir() {
+			return err
+		}
+		rel, _ := filepath.Rel(fixture, path)
+		data, _ := os.ReadFile(path)
+		full := filepath.Join(root, rel)
+		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
+			return err
+		}
+		return os.WriteFile(full, data, 0o644)
+	})
+	if err != nil {
+		t.Fatal(err)
+	}
+	commitAll(t, root, "fixture")
+	return root
+}
+
+// realReport copies the real "full" report beside an attestation rebased onto root: attested paths
+// outside src/ and tests/ take root's content, every other path of the gate repository is ignored,
+// and edit may change the attestation further (its views, a src digest).
+func realReport(t *testing.T, root string, edit func(att map[string]any)) string {
+	t.Helper()
+	src := "../../testdata/mutation-incremental/real/full"
+	report, err := os.ReadFile(filepath.Join(src, "incremental.json"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	var att map[string]any
+	data, _ := os.ReadFile(filepath.Join(src, "attestation.json"))
+	if err := json.Unmarshal(data, &att); err != nil {
+		t.Fatal(err)
+	}
+	for path, raw := range att["files"].(map[string]any) {
+		if strings.HasPrefix(path, "src/") || strings.HasPrefix(path, "tests/") {
+			continue
+		}
+		raw.(map[string]any)["digest"] = digestAt(root, path)
+	}
+	att["config"] = ""
+	lists := att["lists"].(map[string]any)
+	lists["ignore"] = append(lists["ignore"].([]any), "**", ".metareview/**")
+	if edit != nil {
+		edit(att)
+	}
+	dir := t.TempDir()
+	out, _ := json.Marshal(att)
+	if err := os.WriteFile(filepath.Join(dir, "incremental.json"), report, 0o644); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(dir, "attestation.json"), out, 0o644); err != nil {
+		t.Fatal(err)
+	}
+	return filepath.Join(dir, "incremental.json")
+}
+
+func digestAt(root, path string) string {
+	data, err := os.ReadFile(filepath.Join(root, path))
+	if err != nil {
+		return "absent"
+	}
+	h := sha256.Sum256(data)
+	return "sha256:" + hex.EncodeToString(h[:])
+}
+
+func rowsByView(t *testing.T, root, prefix string) map[string]string {
+	t.Helper()
+	records, err := findings.All(root)
+	if err != nil {
+		t.Fatal(err)
+	}
+	out := map[string]string{}
+	for _, r := range records {
+		if strings.HasPrefix(r.Fingerprint, prefix) {
+			out[r.View] = r.Status
+			if r.Classification != "blocking" {
+				out[r.View] += " (" + r.Classification + ")"
+			}
+		}
+	}
+	return out
+}
+
+func review(t *testing.T, root, report string, views ...string) (Result, string) {
+	t.Helper()
+	res, err := Create(root, Options{Base: "main", MutationReportPaths: []string{report}, MutationViews: views})
+	if err != nil {
+		t.Fatal(err)
+	}
+	log, _ := os.ReadFile(filepath.Join(root, res.ReviewRel))
+	return res, string(log)
+}
+
+func TestGateRowsWithViews(t *testing.T) {
+	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
+	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "enforce")
+	root := fixtureRepo(t)
+	report := realReport(t, root, nil)
+
+	// An uncommitted edit is not under review in HEAD mode: both views verified.
+	if err := os.WriteFile(filepath.Join(root, "src/c.ts"), []byte(editedC), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	if res, log := review(t, root, report, "core", "edge"); !strings.Contains(log, "| `core` | 24 | 0 |") || !strings.Contains(log, "| `edge` | 7 | 0 |") {
+		t.Fatalf("uncommitted: %s\n%s", res.Verdict, log)
+	}
+
+	// Repeated flags, one section: a row per view, a finding per view, and enforce blocks.
+	commitAll(t, root, "edit c")
+	res, log := review(t, root, report, "core", "edge")
+	if strings.Count(log, "## Mutation Evidence Freshness") != 1 ||
+		!strings.Contains(log, "| `core` | 21 | 3 |") || !strings.Contains(log, "| `edge` | 4 | 3 |") ||
+		!strings.Contains(log, "Mutation evidence stale (core): src/c.ts changed") || !strings.Contains(log, "Mutation evidence stale (edge): src/c.ts changed") {
+		t.Fatalf("committed: %s\n%s", res.Verdict, log)
+	}
+	if got := rowsByView(t, root, "mutation:stale:enforce:stryker:"); got["core"] != "open" || got["edge"] != "open" || len(got) != 2 {
+		t.Fatalf("per-view rows: %v", got)
+	}
+
+	// Cross-view isolation: fresh evidence for core alone supersedes only core's row, and edge's
+	// open row does not block a run that does not ask for edge.
+	fresh := func(att map[string]any) {
+		att["files"].(map[string]any)["src/c.ts"].(map[string]any)["digest"] = digestAt(root, "src/c.ts")
+	}
+	_, log = review(t, root, realReport(t, root, fresh), "core")
+	if got := rowsByView(t, root, "mutation:stale:"); got["core"] != findings.StatusSuperseded || got["edge"] != "open" || strings.Contains(strings.Split(log, "## metareview PR Evidence")[0], "(edge)") {
+		t.Fatalf("isolation: %v\n%s", got, log)
+	}
+
+	// A report attested without views supersedes nothing by the rename sweep.
+	noViews := func(att map[string]any) { fresh(att); delete(att, "views") }
+	review(t, root, realReport(t, root, noViews), "core")
+	if got := rowsByView(t, root, "mutation:stale:"); got["edge"] != "open" {
+		t.Fatalf("no view map: %v", got)
+	}
+
+	// A renamed view (edge → periphery) supersedes the old view's rows.
+	renamed := func(att map[string]any) {
+		fresh(att)
+		views := att["views"].(map[string]any)
+		views["periphery"] = views["edge"]
+		delete(views, "edge")
+	}
+	review(t, root, realReport(t, root, renamed), "core")
+	if got := rowsByView(t, root, "mutation:stale:"); got["edge"] != findings.StatusSuperseded {
+		t.Fatalf("renamed view: %v", got)
+	}
+
+	// An unattested report under a view keeps its blocker, fingerprinted with the view.
+	unattested := realReport(t, root, nil)
+	if err := os.Remove(filepath.Join(filepath.Dir(unattested), "attestation.json")); err != nil {
+		t.Fatal(err)
+	}
+	res, log = review(t, root, unattested, "core")
+	if got := rowsByView(t, root, "mutation:unattested:enforce:stryker:core:"); got["core"] != "open" ||
+		!strings.Contains(log, "Mutation report has no attestation (core)") {
+		t.Fatalf("unattested: %v %s\n%s", got, res.Verdict, log)
+	}
+}
diff --git a/internal/reviewers/mutation.go b/internal/reviewers/mutation.go
index 176de1d..22e92c8 100644
--- a/internal/reviewers/mutation.go
+++ b/internal/reviewers/mutation.go
@@ -27,6 +27,8 @@ type MutationContext struct {
 	// supplied (omitempty), so a run without reports keeps its digest.
 	Mode      string                          `json:"mode,omitempty"`
 	Freshness []mutationfresh.ReportFreshness `json:"freshness,omitempty"`
+	// Views are the run's --mutation-view names (spec §11.3), in the digest only when used.
+	Views []string `json:"views,omitempty"`
 	// FreshnessFindings and FreshnessSection are derived from the fields above.
 	FreshnessFindings []findings.Input `json:"-"`
 	FreshnessSection  string           `json:"-"`
@@ -80,11 +82,32 @@ func (m MutationContext) Engines() []string {
 	return out
 }
 
+// ViewMaps is, per engine, the sorted view names in the maps of this run's attested reports that
+// carry one (findings.Options.MutationViewMaps, the rename sweep of spec §11.3).
+func (m MutationContext) ViewMaps() map[string][]string {
+	var out map[string][]string
+	for _, f := range m.Freshness {
+		if !f.Attested || f.ViewNames == nil {
+			continue
+		}
+		if out == nil {
+			out = map[string][]string{}
+		}
+		for _, name := range f.ViewNames {
+			if !slices.Contains(out[f.Engine], name) {
+				out[f.Engine] = append(out[f.Engine], name)
+			}
+		}
+		sort.Strings(out[f.Engine])
+	}
+	return out
+}
+
 // LoadMutationContext loads the declared reports and judges their freshness against the content
 // under review: HEAD for pr-ready (unless --include-working-tree), the working tree otherwise. An
 // unreadable report, an invalid mode or a content read error stops the review: a mutation gate
 // that quietly drops a report passes because it looked at less.
-func LoadMutationContext(root string, paths []string, scope string, head bool) (MutationContext, error) {
+func LoadMutationContext(root string, paths, views []string, scope string, head bool) (MutationContext, error) {
 	if len(paths) == 0 {
 		return MutationContext{}, nil
 	}
@@ -100,9 +123,9 @@ func LoadMutationContext(root string, paths []string, scope string, head bool) (
 	if head {
 		content = mutationfresh.Head(root)
 	}
-	res, err := mutationfresh.Build(reports, content, mutationfresh.EffectiveMode(scope, mode))
+	res, err := mutationfresh.Build(reports, content, mutationfresh.EffectiveMode(scope, mode), views)
 	if err != nil {
 		return MutationContext{}, err
 	}
-	return MutationContext{Reports: reports, Mode: res.Mode, Freshness: res.Freshness, FreshnessFindings: res.Findings, FreshnessSection: res.Section}, nil
+	return MutationContext{Reports: reports, Mode: res.Mode, Freshness: res.Freshness, Views: views, FreshnessFindings: res.Findings, FreshnessSection: res.Section}, nil
 }
diff --git a/internal/reviewers/mutation_test.go b/internal/reviewers/mutation_test.go
index 7841503..88566de 100644
--- a/internal/reviewers/mutation_test.go
+++ b/internal/reviewers/mutation_test.go
@@ -259,11 +259,11 @@ func freshnessRepo(t *testing.T) (root, report string) {
 }
 
 func TestLoadMutationContext(t *testing.T) {
-	if ctx, err := LoadMutationContext(t.TempDir(), nil, "pr-ready", true); err != nil || ctx.Mode != "" || len(ctx.Freshness) != 0 {
+	if ctx, err := LoadMutationContext(t.TempDir(), nil, nil, "pr-ready", true); err != nil || ctx.Mode != "" || len(ctx.Freshness) != 0 {
 		t.Errorf("no reports: nothing loaded, nothing serialized: %+v %v", ctx, err)
 	}
 	root, report := freshnessRepo(t)
-	ctx, err := LoadMutationContext(root, []string{report}, "task-done", false)
+	ctx, err := LoadMutationContext(root, []string{report}, nil, "task-done", false)
 	if err != nil || ctx.Mode != "advisory" || ctx.Freshness[0].Verified != 1 || !strings.Contains(ctx.FreshnessSection, "1 verified") {
 		t.Fatalf("worktree: %+v %v", ctx, err)
 	}
@@ -275,8 +275,8 @@ func TestLoadMutationContext(t *testing.T) {
 		t.Fatal(err)
 	}
 	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "enforce")
-	wt, _ := LoadMutationContext(root, []string{report}, "pr-ready", false)
-	head, _ := LoadMutationContext(root, []string{report}, "pr-ready", true)
+	wt, _ := LoadMutationContext(root, []string{report}, nil, "pr-ready", false)
+	head, _ := LoadMutationContext(root, []string{report}, nil, "pr-ready", true)
 	if wt.Freshness[0].Stale != 1 || head.Freshness[0].Stale != 0 || wt.Mode != "enforce" {
 		t.Errorf("working tree %+v, HEAD %+v", wt.Freshness[0], head.Freshness[0])
 	}
@@ -288,14 +288,14 @@ func TestLoadMutationContext(t *testing.T) {
 		t.Errorf("the enforced stale finding joins the context's findings: %+v", wt.Findings())
 	}
 	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "bogus")
-	if _, err := LoadMutationContext(root, []string{report}, "pr-ready", false); err == nil {
+	if _, err := LoadMutationContext(root, []string{report}, nil, "pr-ready", false); err == nil {
 		t.Error("an invalid mode is an error")
 	}
 	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "")
-	if _, err := LoadMutationContext(root, []string{filepath.Join(root, "missing.json")}, "pr-ready", false); err == nil {
+	if _, err := LoadMutationContext(root, []string{filepath.Join(root, "missing.json")}, nil, "pr-ready", false); err == nil {
 		t.Error("an unreadable report is an error")
 	}
-	if _, err := LoadMutationContext(t.TempDir(), []string{report}, "pr-ready", false); err == nil {
+	if _, err := LoadMutationContext(t.TempDir(), []string{report}, nil, "pr-ready", false); err == nil {
 		t.Error("a content error (not a repository) is an error")
 	}
 }
diff --git a/internal/reviewers/viewmaps_test.go b/internal/reviewers/viewmaps_test.go
new file mode 100644
index 0000000..1bc71e4
--- /dev/null
+++ b/internal/reviewers/viewmaps_test.go
@@ -0,0 +1,26 @@
+package reviewers
+
+import (
+	"reflect"
+	"testing"
+
+	"github.com/dsifry/metareview/internal/mutationfresh"
+)
+
+// The rename sweep (spec §11.3) needs, per engine, the view names of the attested reports that
+// carry a map; unattested reports and reports without a map contribute nothing.
+func TestViewMaps(t *testing.T) {
+	if got := (MutationContext{}).ViewMaps(); got != nil {
+		t.Errorf("no reports: %v", got)
+	}
+	ctx := MutationContext{Freshness: []mutationfresh.ReportFreshness{
+		{Engine: "stryker", Attested: true, ViewNames: []string{"edge", "core"}},
+		{Engine: "stryker", Attested: true, ViewNames: []string{"core", "periphery"}},
+		{Engine: "stryker", Attested: true},
+		{Engine: "gremlins", Attested: false, ViewNames: []string{"x"}},
+	}}
+	want := map[string][]string{"stryker": {"core", "edge", "periphery"}}
+	if got := ctx.ViewMaps(); !reflect.DeepEqual(got, want) {
+		t.Errorf("got %v, want %v", got, want)
+	}
+}
diff --git a/internal/taskdone/mutationctx_test.go b/internal/taskdone/mutationctx_test.go
index daccb72..ceec47b 100644
--- a/internal/taskdone/mutationctx_test.go
+++ b/internal/taskdone/mutationctx_test.go
@@ -12,7 +12,7 @@ import (
 // A skipped report is indistinguishable from a package with no survivors, so the failure mode of
 // getting this wrong is a gate that passes because it looked at less.
 func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
-	if _, err := mutationContextFor(t.TempDir(), nil); err != nil {
+	if _, err := mutationContextFor(t.TempDir(), nil, nil); err != nil {
 		t.Errorf("no reports at all is the ordinary case: %v", err)
 	}
 	for name, path := range map[string]string{
@@ -21,7 +21,7 @@ func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
 		"empty file":    writeTemp(t, ""),
 		"files is text": writeTemp(t, `{"files":"none"}`),
 	} {
-		ctx, err := mutationContextFor(t.TempDir(), []string{path})
+		ctx, err := mutationContextFor(t.TempDir(), []string{path}, nil)
 		if err == nil {
 			t.Errorf("%s: must be an error, got %d reports", name, len(ctx.Reports))
 			continue
@@ -35,7 +35,7 @@ func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
 // A readable report reaches the reviewers, so the review actually acts on it.
 func TestAReadableMutationReportReachesTheReviewers(t *testing.T) {
 	path := writeTemp(t, `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"type":"T","status":"LIVED","line":3}]}]}`)
-	ctx, err := mutationContextFor(t.TempDir(), []string{path})
+	ctx, err := mutationContextFor(t.TempDir(), []string{path}, nil)
 	if err != nil {
 		t.Fatal(err)
 	}
diff --git a/internal/taskdone/review.go b/internal/taskdone/review.go
index 745ce57..046da70 100644
--- a/internal/taskdone/review.go
+++ b/internal/taskdone/review.go
@@ -37,6 +37,8 @@ type Options struct {
 	// MutationReportPaths are --mutation-report files: a mutation-testing engine's output,
 	// either mutation-testing-report-schema or gremlins JSON. Empty is the ordinary case.
 	MutationReportPaths []string
+	// MutationViews are --mutation-view names (spec §11.3): the evidence is judged per view.
+	MutationViews []string
 	// ShardWriter is the pack-writing seam; nil uses the real filesystem.
 	ShardWriter shardpack.Writer
 }
@@ -160,7 +162,7 @@ func Create(root, target string, options Options) (Result, error) {
 	if err != nil {
 		return Result{}, err
 	}
-	mutationContext, err := mutationContextFor(root, options.MutationReportPaths)
+	mutationContext, err := mutationContextFor(root, options.MutationReportPaths, options.MutationViews)
 	if err != nil {
 		return Result{}, err
 	}
@@ -229,10 +231,12 @@ func Create(root, target string, options Options) (Result, error) {
 			previousRunIDs = append(previousRunIDs, link.ID)
 		}
 		reconciled, err := reconcileFindings(root, run, rawFindings, findings.Options{
-			PreviousRunID:   options.PreviousRunID,
-			PreviousRunIDs:  previousRunIDs,
-			ResetRunIDs:     chain.ResetRunIDs,
-			MutationEngines: mutationContext.Engines(),
+			PreviousRunID:    options.PreviousRunID,
+			PreviousRunIDs:   previousRunIDs,
+			ResetRunIDs:      chain.ResetRunIDs,
+			MutationEngines:  mutationContext.Engines(),
+			MutationViews:    mutationContext.Views,
+			MutationViewMaps: mutationContext.ViewMaps(),
 		})
 		if err != nil {
 			return err
@@ -791,8 +795,8 @@ func firstNonEmpty(values ...string) string {
 // mutationContextFor loads the declared mutation reports. An unreadable or unrecognised report is
 // an error that stops the review, never a skipped file: a mutation gate that quietly drops a
 // report is a gate that passes because it looked at less.
-func mutationContextFor(root string, paths []string) (reviewers.MutationContext, error) {
-	return reviewers.LoadMutationContext(root, paths, "task-done", false)
+func mutationContextFor(root string, paths, views []string) (reviewers.MutationContext, error) {
+	return reviewers.LoadMutationContext(root, paths, views, "task-done", false)
 }
 
 // joinSections joins the non-empty sections that follow the verdict line.
diff --git a/tests/e2e-mutation-incremental.mjs b/tests/e2e-mutation-incremental.mjs
index 70ab490..169be2f 100644
--- a/tests/e2e-mutation-incremental.mjs
+++ b/tests/e2e-mutation-incremental.mjs
@@ -176,6 +176,25 @@ const ROWS = [
     for (const f of STATE_FILES) cpSync(join(REPO, '.mutation', f), join(WORK, 'baseline', f));
     exportRow('full');
   }],
+  // The gate on the live state (spec §7.1, §11.3): pr-ready with both views reads HEAD, where the
+  // full run's attested content is unchanged, so every kill is verified in its view; an unknown
+  // view is a usage error.
+  ['gate', async () => {
+    reset();
+    const bin = join(WORK, 'metareview');
+    execFileSync('go', ['build', '-o', bin, './cmd/metareview'], { cwd: ROOT, stdio: 'inherit' });
+    const env = { ...process.env, METAREVIEW_ALLOW_MECHANICAL_PASS: '1', METAREVIEW_MUTATION_FRESHNESS: 'enforce' };
+    const gate = (...views) => spawnSync(bin, ['review', 'pr-ready', '--base', 'HEAD', '--mutation-report', '.mutation/incremental.json',
+      ...views.flatMap((v) => ['--mutation-view', v])], { cwd: REPO, env, encoding: 'utf8' });
+    const r = gate('core', 'edge');
+    const logPath = r.stdout.trim().split('\n').pop();
+    const log = logPath && existsSync(join(REPO, logPath)) ? readFileSync(join(REPO, logPath), 'utf8') : '';
+    check('gate: one freshness section with a row per view',
+      log.split('## Mutation Evidence Freshness').length === 2 && log.includes('| `core` | 24 | 0 |') && log.includes('| `edge` | 7 | 0 |'),
+      `exit ${r.status}: ${r.stderr.slice(-500)}\n${log.slice(0, 1500)}`);
+    const bad = gate('nope');
+    check('gate: an unknown view is exit 2', bad.status === 2 && bad.stderr.includes('"nope"'), `exit ${bad.status}: ${bad.stderr}`);
+  }],
   ['no-change', async () => {
     reset();
     const r = await cli(['run', '--mode', 'incremental']);



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

No external validation evidence supplied.
