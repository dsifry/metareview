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
