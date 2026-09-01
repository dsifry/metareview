# metareview task-done context

Run ID: `mrv-20260901-042205337128000-task-done-t0-1-finding-identity-rewire-01ca0bf9`

## Task

# T0.1 evidence — apply the frozen finding-identity at every identity-sensitive site (the rewire)

**Task:** T0.1 (Epic E0) — the rewire half. The spike, ground truth, and frozen primitives
(`FindingKey`/`SameFault`/`ContinuityThreshold`/`FindingScheme`) landed in #37; this completes T0.1 by
applying them at every identity-sensitive site and landing the §5.1(a) golden-domain and §5.2
migration decisions.
**Spec:** `docs/specs/2026-09-01-t0.1-finding-identity-spike.md` §3 (S1–S7), §5.1, §5.2, §9.8 R6.
**Plan:** `docs/plans/2026-08-31-pins-bug-class-decomposition.md` T0.1.
**Base:** `main` (`7adebfa`).

## What landed

- **S1 — `run.BugID` removed** (`internal/fsm/run/canonical.go`). The text-only `hex(sha1(issueText))[:12]`
  is superseded by the file-aware `run.FindingKey`. Its only surviving trace is the §5.2 migration's
  scheme-0 branch (`FindingKeyForScheme(0, …)`), which reproduces the exact pre-T0.1 bytes so a
  persisted scheme-0 id stays recognizable. Value pin retained (`FindingKeyForScheme(0,"any.go","x") ==
  "11f6ad8ec52a"`).
- **S2 — `dedupCandidates`** (`internal/fsm/kind/kind.go`): keyed on `run.FindingKey(f.File, f.IssueText)`
  instead of raw `f.IssueText`. The one site a `BugID`-only grep misses.
- **S4 — the known-id cap set** (`allIDs`): `run.FindingKey(c.File, c.IssueText)`.
- **S5 — matched golden (§5.1(a), APPROVED):** the matched golden inherits the winning candidate's file,
  `run.FindingKey(cands[winner].File, golden.Comment)`, so its id shares one `(file,text)` domain with
  every candidate id — closing the review #35b fileless-golden / file-keyed-candidate split.
- **S6/S7 — candidate mints** (the four confirm-loop returns + the four `secondOpinion` returns):
  `run.FindingKey(cand.File, cand.IssueText)`.
- **§5.2 migration (APPROVED):** `internal/fsm/run/identity.go` gains `FindingKeyForScheme(scheme,file,
  text)` (scheme ≤ 0 = pre-T0.1 text-only sha1; current = `FindingKey`) and `MigrateFindingID(fromScheme,
  file,text) (current, changed)` — migrate-on-read from the retained `(file,text)`, so a pin/override
  keyed on an old id is re-found rather than orphaned. Scope is the FSM id (`run.BugID`), not reviewlog
  `mrvf-*` ids. No live consumer yet (Unproven clear / class carry is T1.3+); delivered as a tested
  primitive, exactly as `SameFault` was.

## The §3.3 site map, re-grepped against live code (line numbers drifted since #37)

| Site | Location | Now keys on |
| --- | --- | --- |
| S1 derivation | `run/canonical.go` (removed) | — (→ `FindingKey`) |
| S2 `dedupCandidates` | `kind.go` | `FindingKey(file,text)` |
| S4 `allIDs` | `kind.go` | `FindingKey(file,text)` |
| S5 matched-golden | `kind.go` | `FindingKey(winnerFile,comment)` |
| S6 confirm-loop mints | `kind.go` (×4) | `FindingKey(cand.File,cand.IssueText)` |
| S7 `secondOpinion` mints | `kind.go` (×4) | `FindingKey(cand.File,cand.IssueText)` |

## TDD (red observed first)

- **S2:** `TestDedupCandidatesIsFileAware` written first → observed red (collapsed same-text/different-file
  to 1) → S2 rewired → green.
- **Migration:** `TestFindingKeyForSchemeReproducesHistory` / `TestMigrateFindingIDResolvesAnOldKeyedOverride`
  written first → compile-red (undefined) → implemented → green.
- Per-path same-text/different-file regressions added: `TestDedupBugsKeysOnFileAwareID` (S3),
  `TestAdjudicateKeepsSameTextInDifferentFilesDistinct` (S2+S4+S6), `TestMatchedGoldenInheritsWinnerFileDomain`
  (S5), `TestAdjudicatePreflightCountsSameTextDifferentFilesAsTwo` (S4), plus a file-aware id assertion
  in `TestEscalationFailureKeepsTheFinding` (S7).

## Fixture updates forced by the frozen semantics (not product changes)

`NormalizeText` collapses digit runs to a `#` token (the maintainer-approved T2 property: `"line 42" ≡
"line 51"`). Test fixtures that distinguished findings only by a trailing digit — `"c0"/"c1"/"c2"`,
`"bug 0"/"bug 1"`, `"t0".."t249"` — therefore now share one identity and are invalid. They were updated
to distinguish by a digit-free token or a distinct file (`kind_test.go`, machine `findings(n)` helper,
the `big`/`fat` cap fixtures). The frozen-floor gate `TestT0IdentityMeetsFrozenFloors` was **not**
touched (τ/NormalizeText unchanged): still T1–T3 exact 100%, T4 recall 11/12, precision negatives 0.

## Golden fixtures

`internal/fsm/run/testdata/golden-log.jsonl` and `golden-snapshots.jsonl` build confirmed bugs with
**literal** ids (`bug("b1")`), independent of the identity derivation, so they did **not** drift and
needed no regeneration. `TestGoldenPrefixes` stays green and the files are byte-identical.

## Mutation verification (each applied, observed red, restored — not a claimed kill)

| Predicate | Mutation | Killing test |
| --- | --- | --- |
| S2 dedup file-aware | drop the file → `FindingKey("", text)` | `TestDedupCandidatesIsFileAware` |
| S4 cap-set file-aware | drop the file | `TestAdjudicatePreflightCountsSameTextDifferentFilesAsTwo` |
| S5 golden inherits winner file | `FindingKey("", comment)` | `TestMatchedGoldenInheritsWinnerFileDomain` |
| S6 real_but_ungold mint | drop the file (line 563) | `TestAdjudicateKeepsSameTextInDifferentFilesDistinct` |
| S7 secondOpinion mint | drop the file (line 612) | `TestEscalationFailureKeepsTheFinding` |
| migration scheme switch | `scheme <= 0` → `scheme <= -1` | `TestFindingKeyForSchemeReproducesHistory`, `TestMigrateFindingID…` |
| migration changed flag | `!= current` → `== current` | `TestMigrateFindingIDResolvesAnOldKeyedOverride` |

## Verification (all green)

- `go build ./...` — OK.
- `go test ./...` — all `ok`, no failures.
- `bash tests/coverage.sh` — coverage gate passed; `internal/fsm/{run,kind,machine}` at 100.0%.
- `gofmt -l internal cmd` — clean.


## Git

- Base: `bc1d9b1c712efb1bd72992bd4fd1990df85315a8`
- Head: `be27fc2c4ec56da13567889bbf1b5c5d9b3061fb`
- Branch: `t0.1-rewire`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `38889`
- Filtered diff bytes: `38889`
- Risk level: `none`

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/metareview/evidence/t0.1-finding-identity-rewire.md
- internal/fsm/kind/kind.go
- internal/fsm/kind/kind_test.go
- internal/fsm/machine/fork_test.go
- internal/fsm/machine/harness_test.go
- internal/fsm/machine/machine_test.go
- internal/fsm/run/canonical.go
- internal/fsm/run/canonical_test.go
- internal/fsm/run/identity.go
- internal/fsm/run/identity_test.go

### Manifest Blockers
No manifest blockers.

## Changed Files

- docs/metareview/evidence/t0.1-finding-identity-rewire.md
- internal/fsm/kind/kind.go
- internal/fsm/kind/kind_test.go
- internal/fsm/machine/fork_test.go
- internal/fsm/machine/harness_test.go
- internal/fsm/machine/machine_test.go
- internal/fsm/run/canonical.go
- internal/fsm/run/canonical_test.go
- internal/fsm/run/identity.go
- internal/fsm/run/identity_test.go

## Diff

```diff
diff --git a/docs/metareview/evidence/t0.1-finding-identity-rewire.md b/docs/metareview/evidence/t0.1-finding-identity-rewire.md
new file mode 100644
index 0000000..cf8b5f4
--- /dev/null
+++ b/docs/metareview/evidence/t0.1-finding-identity-rewire.md
@@ -0,0 +1,87 @@
+# T0.1 evidence — apply the frozen finding-identity at every identity-sensitive site (the rewire)
+
+**Task:** T0.1 (Epic E0) — the rewire half. The spike, ground truth, and frozen primitives
+(`FindingKey`/`SameFault`/`ContinuityThreshold`/`FindingScheme`) landed in #37; this completes T0.1 by
+applying them at every identity-sensitive site and landing the §5.1(a) golden-domain and §5.2
+migration decisions.
+**Spec:** `docs/specs/2026-09-01-t0.1-finding-identity-spike.md` §3 (S1–S7), §5.1, §5.2, §9.8 R6.
+**Plan:** `docs/plans/2026-08-31-pins-bug-class-decomposition.md` T0.1.
+**Base:** `main` (`7adebfa`).
+
+## What landed
+
+- **S1 — `run.BugID` removed** (`internal/fsm/run/canonical.go`). The text-only `hex(sha1(issueText))[:12]`
+  is superseded by the file-aware `run.FindingKey`. Its only surviving trace is the §5.2 migration's
+  scheme-0 branch (`FindingKeyForScheme(0, …)`), which reproduces the exact pre-T0.1 bytes so a
+  persisted scheme-0 id stays recognizable. Value pin retained (`FindingKeyForScheme(0,"any.go","x") ==
+  "11f6ad8ec52a"`).
+- **S2 — `dedupCandidates`** (`internal/fsm/kind/kind.go`): keyed on `run.FindingKey(f.File, f.IssueText)`
+  instead of raw `f.IssueText`. The one site a `BugID`-only grep misses.
+- **S4 — the known-id cap set** (`allIDs`): `run.FindingKey(c.File, c.IssueText)`.
+- **S5 — matched golden (§5.1(a), APPROVED):** the matched golden inherits the winning candidate's file,
+  `run.FindingKey(cands[winner].File, golden.Comment)`, so its id shares one `(file,text)` domain with
+  every candidate id — closing the review #35b fileless-golden / file-keyed-candidate split.
+- **S6/S7 — candidate mints** (the four confirm-loop returns + the four `secondOpinion` returns):
+  `run.FindingKey(cand.File, cand.IssueText)`.
+- **§5.2 migration (APPROVED):** `internal/fsm/run/identity.go` gains `FindingKeyForScheme(scheme,file,
+  text)` (scheme ≤ 0 = pre-T0.1 text-only sha1; current = `FindingKey`) and `MigrateFindingID(fromScheme,
+  file,text) (current, changed)` — migrate-on-read from the retained `(file,text)`, so a pin/override
+  keyed on an old id is re-found rather than orphaned. Scope is the FSM id (`run.BugID`), not reviewlog
+  `mrvf-*` ids. No live consumer yet (Unproven clear / class carry is T1.3+); delivered as a tested
+  primitive, exactly as `SameFault` was.
+
+## The §3.3 site map, re-grepped against live code (line numbers drifted since #37)
+
+| Site | Location | Now keys on |
+| --- | --- | --- |
+| S1 derivation | `run/canonical.go` (removed) | — (→ `FindingKey`) |
+| S2 `dedupCandidates` | `kind.go` | `FindingKey(file,text)` |
+| S4 `allIDs` | `kind.go` | `FindingKey(file,text)` |
+| S5 matched-golden | `kind.go` | `FindingKey(winnerFile,comment)` |
+| S6 confirm-loop mints | `kind.go` (×4) | `FindingKey(cand.File,cand.IssueText)` |
+| S7 `secondOpinion` mints | `kind.go` (×4) | `FindingKey(cand.File,cand.IssueText)` |
+
+## TDD (red observed first)
+
+- **S2:** `TestDedupCandidatesIsFileAware` written first → observed red (collapsed same-text/different-file
+  to 1) → S2 rewired → green.
+- **Migration:** `TestFindingKeyForSchemeReproducesHistory` / `TestMigrateFindingIDResolvesAnOldKeyedOverride`
+  written first → compile-red (undefined) → implemented → green.
+- Per-path same-text/different-file regressions added: `TestDedupBugsKeysOnFileAwareID` (S3),
+  `TestAdjudicateKeepsSameTextInDifferentFilesDistinct` (S2+S4+S6), `TestMatchedGoldenInheritsWinnerFileDomain`
+  (S5), `TestAdjudicatePreflightCountsSameTextDifferentFilesAsTwo` (S4), plus a file-aware id assertion
+  in `TestEscalationFailureKeepsTheFinding` (S7).
+
+## Fixture updates forced by the frozen semantics (not product changes)
+
+`NormalizeText` collapses digit runs to a `#` token (the maintainer-approved T2 property: `"line 42" ≡
+"line 51"`). Test fixtures that distinguished findings only by a trailing digit — `"c0"/"c1"/"c2"`,
+`"bug 0"/"bug 1"`, `"t0".."t249"` — therefore now share one identity and are invalid. They were updated
+to distinguish by a digit-free token or a distinct file (`kind_test.go`, machine `findings(n)` helper,
+the `big`/`fat` cap fixtures). The frozen-floor gate `TestT0IdentityMeetsFrozenFloors` was **not**
+touched (τ/NormalizeText unchanged): still T1–T3 exact 100%, T4 recall 11/12, precision negatives 0.
+
+## Golden fixtures
+
+`internal/fsm/run/testdata/golden-log.jsonl` and `golden-snapshots.jsonl` build confirmed bugs with
+**literal** ids (`bug("b1")`), independent of the identity derivation, so they did **not** drift and
+needed no regeneration. `TestGoldenPrefixes` stays green and the files are byte-identical.
+
+## Mutation verification (each applied, observed red, restored — not a claimed kill)
+
+| Predicate | Mutation | Killing test |
+| --- | --- | --- |
+| S2 dedup file-aware | drop the file → `FindingKey("", text)` | `TestDedupCandidatesIsFileAware` |
+| S4 cap-set file-aware | drop the file | `TestAdjudicatePreflightCountsSameTextDifferentFilesAsTwo` |
+| S5 golden inherits winner file | `FindingKey("", comment)` | `TestMatchedGoldenInheritsWinnerFileDomain` |
+| S6 real_but_ungold mint | drop the file (line 563) | `TestAdjudicateKeepsSameTextInDifferentFilesDistinct` |
+| S7 secondOpinion mint | drop the file (line 612) | `TestEscalationFailureKeepsTheFinding` |
+| migration scheme switch | `scheme <= 0` → `scheme <= -1` | `TestFindingKeyForSchemeReproducesHistory`, `TestMigrateFindingID…` |
+| migration changed flag | `!= current` → `== current` | `TestMigrateFindingIDResolvesAnOldKeyedOverride` |
+
+## Verification (all green)
+
+- `go build ./...` — OK.
+- `go test ./...` — all `ok`, no failures.
+- `bash tests/coverage.sh` — coverage gate passed; `internal/fsm/{run,kind,machine}` at 100.0%.
+- `gofmt -l internal cmd` — clean.
diff --git a/internal/fsm/kind/kind.go b/internal/fsm/kind/kind.go
index 33ba165..a28b4a5 100644
--- a/internal/fsm/kind/kind.go
+++ b/internal/fsm/kind/kind.go
@@ -450,13 +450,17 @@ func callAs(ctx context.Context, j judge.Judge, in machine.ExecInput, req judge.
 	return v, err
 }
 
-// dedupCandidates keeps the first occurrence of each issue text.
+// dedupCandidates keeps the first occurrence of each finding identity. It keys on the file-aware
+// run.FindingKey (T0.1), not raw issue text: the same sentence about two different files is two
+// distinct faults, so collapsing on text alone would drop a real bug. A genuine same-(file,text)
+// repeat still collapses to its first occurrence.
 func dedupCandidates(fs []run.Finding) []run.Finding {
 	seen := map[string]bool{}
 	var out []run.Finding
 	for _, f := range fs {
-		if !seen[f.IssueText] {
-			seen[f.IssueText] = true
+		k := run.FindingKey(f.File, f.IssueText)
+		if !seen[k] {
+			seen[k] = true
 			out = append(out, f)
 		}
 	}
@@ -489,7 +493,7 @@ func (e *adjudicateExec) Execute(ctx context.Context, in machine.ExecInput) (jso
 		allIDs[b.ID] = true
 	}
 	for _, c := range cands {
-		allIDs[run.BugID(c.IssueText)] = true
+		allIDs[run.FindingKey(c.File, c.IssueText)] = true
 	}
 	if len(allIDs) > run.MaxDeltaList {
 		return nil, errs.E(CodeTooManyBugs, fmt.Sprintf("%d bugs would be known (max %d)", len(allIDs), run.MaxDeltaList), "reason", "preflight")
@@ -517,7 +521,10 @@ func (e *adjudicateExec) Execute(ctx context.Context, in machine.ExecInput) (jso
 		if winner >= 0 {
 			gi := g
 			desc, _ := run.CapText(golden.Comment, run.MaxDesc)
-			confirmed = append(confirmed, run.Bug{ID: run.BugID(golden.Comment), Desc: desc, File: cands[winner].File, Line: cands[winner].Line, Verdict: run.VerdictMatched, Confidence: best, GoldenIdx: &gi})
+			// §5.1(a): the matched golden inherits the winning candidate's file, so its id lives in the
+			// same (file, text) domain as every candidate id — no fileless-golden vs file-keyed-candidate
+			// split (review #35b).
+			confirmed = append(confirmed, run.Bug{ID: run.FindingKey(cands[winner].File, golden.Comment), Desc: desc, File: cands[winner].File, Line: cands[winner].Line, Verdict: run.VerdictMatched, Confidence: best, GoldenIdx: &gi})
 		}
 	}
 	var rejected []run.Bug
@@ -534,7 +541,7 @@ func (e *adjudicateExec) Execute(ctx context.Context, in machine.ExecInput) (jso
 		// empty or unreadable diff says nothing about this candidate, so fall through and ask.
 		if cand.File != "" && len(judge.ChangedPaths(in.Diff.Text)) > 0 && !judge.DiffHasFile(in.Diff.Text, cand.File) {
 			desc, _ := run.CapText(cand.IssueText, run.MaxDesc)
-			confirmed = append(confirmed, run.Bug{ID: run.BugID(cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictUnverifiedNoEvidence})
+			confirmed = append(confirmed, run.Bug{ID: run.FindingKey(cand.File, cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictUnverifiedNoEvidence})
 			continue
 		}
 		diff, truncated, diffHash := judge.ContextForClaim(in.Diff.Text, in.Diff.Truncated, cand.File, cand.Line, cand.IssueText, judge.MaxDiffBytes)
@@ -547,22 +554,22 @@ func (e *adjudicateExec) Execute(ctx context.Context, in machine.ExecInput) (jso
 		// finding because the transport failed, not because the judge decided anything.
 		if v.ParseError != "" {
 			desc, _ := run.CapText(cand.IssueText, run.MaxDesc)
-			confirmed = append(confirmed, run.Bug{ID: run.BugID(cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictCheckedButUnverified})
+			confirmed = append(confirmed, run.Bug{ID: run.FindingKey(cand.File, cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictCheckedButUnverified})
 			continue
 		}
 		real := v.Decision && v.Confidence >= AdjudicateThreshold
 		if real {
 			desc, _ := run.CapText(cand.IssueText, run.MaxDesc)
-			confirmed = append(confirmed, run.Bug{ID: run.BugID(cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictRealButUngold, Confidence: v.Confidence})
+			confirmed = append(confirmed, run.Bug{ID: run.FindingKey(cand.File, cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictRealButUngold, Confidence: v.Confidence})
 		} else if second, ok := e.secondOpinion(ctx, in, cand, &index); ok {
 			confirmed = append(confirmed, second)
 		} else {
 			desc, _ := run.CapText(cand.IssueText, run.MaxShort)
-			rejected = append(rejected, run.Bug{ID: run.BugID(cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictHallucination, Confidence: v.Confidence})
+			rejected = append(rejected, run.Bug{ID: run.FindingKey(cand.File, cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictHallucination, Confidence: v.Confidence})
 		}
 	}
 	// validity by construction: the pre-flight bounds the size and count, every
-	// Desc is capped, ids are deduplicated and never empty (BugID of any text).
+	// Desc is capped, ids are deduplicated and never empty (FindingKey of any (file, text)).
 	out := adjudicateOut{Confirmed: dedupBugs(confirmed), Rejected: dedupBugs(rejected)}
 	return json.RawMessage(run.MarshalCanonical(out)), nil
 }
@@ -594,7 +601,7 @@ func (e *adjudicateExec) secondOpinion(ctx context.Context, in machine.ExecInput
 			return run.Bug{}, false // escalation deliberately unavailable: the rejection stands
 		}
 		// The second opinion could not be built, so nothing decided this finding.
-		return run.Bug{ID: run.BugID(cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictCheckedButUnverified}, true
+		return run.Bug{ID: run.FindingKey(cand.File, cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictCheckedButUnverified}, true
 	}
 	diff, truncated, diffHash := judge.ContextForClaim(in.Diff.Text, in.Diff.Truncated, cand.File, cand.Line, cand.IssueText, judge.MaxDiffBytes)
 	v, err := callAs(ctx, esc.Judge, in, judge.Request{Kind: judge.KindAdjudicate, Index: *index, Input: judge.AdjudicateInput{Diff: diff, DiffTruncated: truncated, DiffContextHash: diffHash, Candidate: cand, Sandbox: esc.Root != ""}}, esc)
@@ -602,13 +609,13 @@ func (e *adjudicateExec) secondOpinion(ctx context.Context, in machine.ExecInput
 	if err != nil {
 		// The second opinion never arrived, so nothing decided this finding. Keeping the
 		// first arm's rejection would drop it on the strength of a check that did not run.
-		return run.Bug{ID: run.BugID(cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictCheckedButUnverified}, true
+		return run.Bug{ID: run.FindingKey(cand.File, cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictCheckedButUnverified}, true
 	}
 	if v.ParseError != "" {
-		return run.Bug{ID: run.BugID(cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictCheckedButUnverified}, true
+		return run.Bug{ID: run.FindingKey(cand.File, cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictCheckedButUnverified}, true
 	}
 	if v.Decision && v.Confidence >= AdjudicateThreshold {
-		return run.Bug{ID: run.BugID(cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictRealButUngold, Confidence: v.Confidence}, true
+		return run.Bug{ID: run.FindingKey(cand.File, cand.IssueText), Desc: desc, File: cand.File, Line: cand.Line, Verdict: run.VerdictRealButUngold, Confidence: v.Confidence}, true
 	}
 	return run.Bug{}, false // both arms agree it is not real
 }
diff --git a/internal/fsm/kind/kind_test.go b/internal/fsm/kind/kind_test.go
index f97052b..94b689f 100644
--- a/internal/fsm/kind/kind_test.go
+++ b/internal/fsm/kind/kind_test.go
@@ -321,13 +321,15 @@ func bugs(from, to int) []run.Bug {
 
 func TestK3Composition(t *testing.T) {
 	ctx := context.Background()
-	fs := findings("c0", "c1", "c2")
-	goldens := []run.Golden{{Comment: "g0"}, {Comment: "g1"}}
+	// Candidate/golden texts are digit-free and mutually distinct: the frozen finding identity
+	// (T0.1) collapses digit runs, so "c0"/"c1"/"c2" would normalize to one identity and dedup away.
+	fs := findings("cx", "cy", "cz")
+	goldens := []run.Golden{{Comment: "gx"}, {Comment: "gy"}}
 	script := judge.Script{Calls: map[judge.ScriptKey]judge.ScriptRow{}}
 	key := func(kind string, idx int) judge.ScriptKey {
 		return judge.ScriptKey{Kind: kind, Node: "adjudicate", Iter: 2, Index: idx}
 	}
-	// g0: c0 0.5 (provisional), c1 0.9 wins, c2 no. g1: c0 no, c1 0.9 wins again (equal to nothing else), c2 match:false 0.99 never
+	// gx: cx 0.5 (provisional), cy 0.9 wins, cz no. gy: cx 0.0 no, cy 0.9 wins again, cz match:false 0.99 never
 	script.Calls[key(judge.KindMatch, 5)] = rowFor(true, 0.5)
 	script.Calls[key(judge.KindMatch, 6)] = rowFor(true, 0.9)
 	script.Calls[key(judge.KindMatch, 7)] = rowFor(false, 0.99)
@@ -354,10 +356,10 @@ func TestK3Composition(t *testing.T) {
 	}
 	var out adjudicateOut
 	_ = json.Unmarshal(raw, &out)
-	if len(out.Confirmed) != 3 || out.Confirmed[0].Desc != "g0" || out.Confirmed[0].Verdict != run.VerdictMatched || *out.Confirmed[0].GoldenIdx != 0 || out.Confirmed[0].Confidence != 0.9 || out.Confirmed[0].File != "f.go" || out.Confirmed[0].Line != 2 || out.Confirmed[0].ID != run.BugID("g0") {
-		t.Fatalf("matched g0: %+v", out.Confirmed)
+	if len(out.Confirmed) != 3 || out.Confirmed[0].Desc != "gx" || out.Confirmed[0].Verdict != run.VerdictMatched || *out.Confirmed[0].GoldenIdx != 0 || out.Confirmed[0].Confidence != 0.9 || out.Confirmed[0].File != "f.go" || out.Confirmed[0].Line != 2 || out.Confirmed[0].ID != run.FindingKey("f.go", "gx") {
+		t.Fatalf("matched gx: %+v", out.Confirmed)
 	}
-	if out.Confirmed[1].Desc != "g1" || *out.Confirmed[1].GoldenIdx != 1 || out.Confirmed[2].Verdict != run.VerdictRealButUngold || out.Confirmed[2].Desc != "c2" || out.Confirmed[2].ID != run.BugID("c2") || out.Confirmed[2].Confidence != 0.7 {
+	if out.Confirmed[1].Desc != "gy" || *out.Confirmed[1].GoldenIdx != 1 || out.Confirmed[2].Verdict != run.VerdictRealButUngold || out.Confirmed[2].Desc != "cz" || out.Confirmed[2].ID != run.FindingKey("f.go", "cz") || out.Confirmed[2].Confidence != 0.7 {
 		t.Fatalf("confirmed: %+v", out.Confirmed)
 	}
 	if len(out.Rejected) != 0 {
@@ -382,13 +384,14 @@ func TestK3Composition(t *testing.T) {
 	r = mustNew(t, judge.NewMock(script), true)
 	ex, _ = r.Executor(MatchThenAdjudicate)
 	a = &audits{}
-	raw, err = ex.Execute(ctx, execInput(run.Snapshot{Iteration: 2, Findings: findings("c0", "c1"), Goldens: goldens[:1]}, adjNode, 0, a))
+	raw, err = ex.Execute(ctx, execInput(run.Snapshot{Iteration: 2, Findings: findings("cx", "cy"), Goldens: goldens[:1]}, adjNode, 0, a))
 	_ = json.Unmarshal(raw, &out)
 	if err != nil || len(a.events) != 2 || len(out.Confirmed) != 1 || out.Confirmed[0].Line != 2 || len(out.Rejected) != 0 {
 		t.Fatalf("supersession: %v %d %+v", err, len(a.events), out)
 	}
-	// ties keep the first; duplicate texts collapse (first location kept); rejected hallucination;
-	// a parse error is NOT a judgment, so that candidate is kept as checked_but_unverified
+	// ties keep the first; a same-(file,text) duplicate collapses (first location kept) — a same text
+	// in a DIFFERENT file would NOT (T0.1); rejected hallucination; a parse error is NOT a judgment,
+	// so that candidate is kept as checked_but_unverified
 	script = judge.Script{Calls: map[judge.ScriptKey]judge.ScriptRow{
 		key(judge.KindMatch, 0):      rowFor(true, 0.6),
 		key(judge.KindMatch, 1):      rowFor(true, 0.6),
@@ -399,13 +402,13 @@ func TestK3Composition(t *testing.T) {
 	r = mustNew(t, judge.NewMock(script), true)
 	ex, _ = r.Executor(MatchThenAdjudicate)
 	a = &audits{}
-	dup := []run.Finding{{IssueText: "c0", File: "a.go", Line: 1}, {IssueText: "c1", File: "b.go", Line: 2}, {IssueText: "c0", File: "z.go", Line: 9}, {IssueText: "c2"}}
+	dup := []run.Finding{{IssueText: "cx", File: "a.go", Line: 1}, {IssueText: "cy", File: "b.go", Line: 2}, {IssueText: "cx", File: "a.go", Line: 9}, {IssueText: "cz"}}
 	raw, err = ex.Execute(ctx, execInput(run.Snapshot{Iteration: 2, Findings: dup, Goldens: goldens[:1]}, adjNode, 0, a))
 	_ = json.Unmarshal(raw, &out)
 	if err != nil || len(out.Confirmed) != 2 || out.Confirmed[0].File != "a.go" || len(out.Rejected) != 1 || out.Rejected[0].Verdict != run.VerdictHallucination || out.Rejected[0].Confidence != 0.69 {
 		t.Fatalf("tie/dedup/reject: %v %+v", err, out)
 	}
-	if out.Confirmed[1].Desc != "c2" || out.Confirmed[1].Verdict != run.VerdictCheckedButUnverified {
+	if out.Confirmed[1].Desc != "cz" || out.Confirmed[1].Verdict != run.VerdictCheckedButUnverified {
 		t.Fatalf("an unparseable reply must be kept as checked_but_unverified, not dropped: %+v", out.Confirmed[1])
 	}
 	if a.events[2].Error == "" || !strings.HasPrefix(a.events[2].Error, "parse: ") || string(a.events[2].Verdict) != "null" || a.events[4].Error == "" {
@@ -472,7 +475,7 @@ func TestK3Composition(t *testing.T) {
 	script = judge.Script{Calls: map[judge.ScriptKey]judge.ScriptRow{key(judge.KindMatch, 0): rowFor(true, 0.9)}}
 	r = mustNew(t, judge.NewMock(script), true)
 	ex, _ = r.Executor(MatchThenAdjudicate)
-	raw, err = ex.Execute(ctx, execInput(run.Snapshot{Iteration: 2, Findings: findings("g0"), Goldens: goldens[:1]}, adjNode, 0, &audits{}))
+	raw, err = ex.Execute(ctx, execInput(run.Snapshot{Iteration: 2, Findings: findings("gx"), Goldens: goldens[:1]}, adjNode, 0, &audits{}))
 	_ = json.Unmarshal(raw, &out)
 	if err != nil || len(out.Confirmed) != 1 {
 		t.Fatalf("golden-equal candidate: %v %+v", err, out)
@@ -480,7 +483,9 @@ func TestK3Composition(t *testing.T) {
 	// pre-flight refusals: too many candidates+goldens, union cliff, worst-case size
 	big := make([]run.Finding, 250)
 	for i := range big {
-		big[i] = run.Finding{IssueText: fmt.Sprint("t", i)}
+		// Distinct file per finding: a bare numeric suffix collapses under the identity's digit-run
+		// normalization, so text alone would dedup 250 candidates to one and never trip the cap.
+		big[i] = run.Finding{IssueText: "t", File: fmt.Sprintf("f%d.go", i)}
 	}
 	var gs []run.Golden
 	for i := 0; i < 10; i++ {
@@ -495,7 +500,7 @@ func TestK3Composition(t *testing.T) {
 	}
 	fat := make([]run.Finding, 130)
 	for i := range fat {
-		fat[i] = run.Finding{IssueText: strings.Repeat("x", run.MaxDesc) + fmt.Sprint(i)}
+		fat[i] = run.Finding{IssueText: strings.Repeat("x", run.MaxDesc), File: fmt.Sprintf("f%d.go", i)}
 	}
 	if _, err := ex.Execute(ctx, execInput(run.Snapshot{Findings: fat}, adjNode, 0, a)); !errs.Is(err, CodeTooManyBugs) || errs.As(err).Field("reason") != "preflight" || len(a.events) != 0 {
 		t.Fatalf("preflight size: %v", err)
@@ -690,6 +695,121 @@ func (r *recordingJudge) Call(_ context.Context, req judge.Request) (judge.Verdi
 	return judge.Verdict{Decision: true, Confidence: 0.9}, nil
 }
 
+// S2 (T0.1): dedupCandidates must key on the file-aware finding identity, not raw issue text. The
+// same sentence about two different files is two distinct faults; collapsing them on text alone
+// drops a real bug (the precision the file component adds). A genuine same-(file,text) repeat still
+// collapses.
+func TestDedupCandidatesIsFileAware(t *testing.T) {
+	in := []run.Finding{
+		{IssueText: "same text", File: "a.go", Line: 1},
+		{IssueText: "same text", File: "b.go", Line: 2},
+		{IssueText: "same text", File: "a.go", Line: 9}, // a real same-(file,text) duplicate → collapses
+	}
+	out := dedupCandidates(in)
+	if len(out) != 2 {
+		t.Fatalf("file-aware dedup must keep a.go and b.go but collapse the a.go repeat, got %d: %+v", len(out), out)
+	}
+	if out[0].File != "a.go" || out[0].Line != 1 || out[1].File != "b.go" {
+		t.Fatalf("dedup must keep the first occurrence of each (file,text): %+v", out)
+	}
+}
+
+// S3 (T0.1): dedupBugs keys on Bug.ID, which is now the file-aware FindingKey. Two confirmed bugs
+// with the same text in different files carry distinct ids and must both survive; a genuine repeat
+// (a golden a candidate also won, or two goldens sharing a comment+file) collapses to the first.
+func TestDedupBugsKeysOnFileAwareID(t *testing.T) {
+	a := run.Bug{ID: run.FindingKey("a.go", "same"), Desc: "same", File: "a.go", Verdict: run.VerdictRealButUngold}
+	b := run.Bug{ID: run.FindingKey("b.go", "same"), Desc: "same", File: "b.go", Verdict: run.VerdictRealButUngold}
+	if a.ID == b.ID {
+		t.Fatal("precondition: same-text/different-file bugs must derive distinct ids")
+	}
+	out := dedupBugs([]run.Bug{a, b, a})
+	if len(out) != 2 || out[0].File != "a.go" || out[1].File != "b.go" {
+		t.Fatalf("dedupBugs must keep the two distinct-id bugs and collapse the repeat: %+v", out)
+	}
+}
+
+// S2+S4+S6 (T0.1) end-to-end: the same finding text about two DIFFERENT files is two faults. Every
+// identity-sensitive site on the confirm path — dedupCandidates, the known-id cap set, and the mint
+// itself — must keep them distinct through to two confirmed bugs with different file-aware ids.
+func TestAdjudicateKeepsSameTextInDifferentFilesDistinct(t *testing.T) {
+	ctx := context.Background()
+	text := "the guard is unpinned"
+	key := func(idx int) judge.ScriptKey {
+		return judge.ScriptKey{Kind: judge.KindAdjudicate, Node: "adjudicate", Iter: 2, Index: idx}
+	}
+	script := judge.Script{Calls: map[judge.ScriptKey]judge.ScriptRow{key(0): adjRow(true, 0.9), key(1): adjRow(true, 0.9)}}
+	r := mustNew(t, judge.NewMock(script), true)
+	ex, _ := r.Executor(MatchThenAdjudicate)
+	cands := []run.Finding{{IssueText: text, File: "a.go", Line: 1}, {IssueText: text, File: "b.go", Line: 2}}
+	raw, err := ex.Execute(ctx, execInput(run.Snapshot{Iteration: 2, Findings: cands}, adjNode, 0, &audits{}))
+	if err != nil {
+		t.Fatal(err)
+	}
+	var out adjudicateOut
+	_ = json.Unmarshal(raw, &out)
+	if len(out.Confirmed) != 2 {
+		t.Fatalf("same text in two files must yield two confirmed bugs, got %+v", out.Confirmed)
+	}
+	if out.Confirmed[0].ID == out.Confirmed[1].ID {
+		t.Fatalf("the two bugs must carry distinct file-aware ids: %+v", out.Confirmed)
+	}
+	if out.Confirmed[0].ID != run.FindingKey("a.go", text) || out.Confirmed[1].ID != run.FindingKey("b.go", text) {
+		t.Fatalf("confirmed ids must be the file-aware FindingKey: %+v", out.Confirmed)
+	}
+}
+
+// S5 (§5.1(a)): a matched golden's id lives in the WINNING candidate's (file, text) domain — the
+// golden text keyed under the winner's file — closing the review #35b split where a fileless golden id
+// mixed with file-keyed candidate ids. The winner here is b.go, so the golden id is
+// FindingKey("b.go", comment), never FindingKey("", comment).
+func TestMatchedGoldenInheritsWinnerFileDomain(t *testing.T) {
+	ctx := context.Background()
+	comment := "the loader runs twice on init"
+	key := func(idx int) judge.ScriptKey {
+		return judge.ScriptKey{Kind: judge.KindMatch, Node: "adjudicate", Iter: 2, Index: idx}
+	}
+	// Both candidates match the golden (so neither is re-adjudicated); b.go wins on confidence.
+	script := judge.Script{Calls: map[judge.ScriptKey]judge.ScriptRow{key(0): rowFor(true, 0.3), key(1): rowFor(true, 0.9)}}
+	r := mustNew(t, judge.NewMock(script), true)
+	ex, _ := r.Executor(MatchThenAdjudicate)
+	cands := []run.Finding{{IssueText: "alpha", File: "a.go", Line: 1}, {IssueText: "beta", File: "b.go", Line: 2}}
+	snap := run.Snapshot{Iteration: 2, Findings: cands, Goldens: []run.Golden{{Comment: comment}}}
+	raw, err := ex.Execute(ctx, execInput(snap, adjNode, 0, &audits{}))
+	if err != nil {
+		t.Fatal(err)
+	}
+	var out adjudicateOut
+	_ = json.Unmarshal(raw, &out)
+	if len(out.Confirmed) != 1 || out.Confirmed[0].Verdict != run.VerdictMatched {
+		t.Fatalf("expected one matched golden: %+v", out.Confirmed)
+	}
+	got := out.Confirmed[0]
+	if got.File != "b.go" || got.ID != run.FindingKey("b.go", comment) {
+		t.Fatalf("matched golden must inherit the winner's file domain: %+v", got)
+	}
+	if got.ID == run.FindingKey("", comment) {
+		t.Fatal("the golden id must not live in the fileless domain (the review #35b split)")
+	}
+}
+
+// S4 (T0.1): the known-id cap set keys each candidate on its file-aware id, so the same text in two
+// files counts as TWO known ids. Keying on text alone would undercount and let an over-cap set of
+// known bugs slip past the union pre-flight.
+func TestAdjudicatePreflightCountsSameTextDifferentFilesAsTwo(t *testing.T) {
+	ctx := context.Background()
+	r := mustNew(t, judge.NewMock(judge.Script{Calls: map[judge.ScriptKey]judge.ScriptRow{}}), true)
+	ex, _ := r.Executor(MatchThenAdjudicate)
+	text := "the guard is unpinned"
+	cands := []run.Finding{{IssueText: text, File: "a.go", Line: 1}, {IssueText: text, File: "b.go", Line: 2}}
+	a := &audits{}
+	// 255 already-known bugs + 2 distinct-file candidates = 257 > MaxDeltaList: refuse before any spend.
+	_, err := ex.Execute(ctx, execInput(run.Snapshot{Iteration: 2, Findings: cands, AllFound: bugs(0, 255)}, adjNode, 0, a))
+	if !errs.Is(err, CodeTooManyBugs) || len(a.events) != 0 {
+		t.Fatalf("two distinct-file candidates over the cap must refuse at the union pre-flight: %v (%d events)", err, len(a.events))
+	}
+}
+
 // Each candidate must be judged against its OWN file's hunks. A single cut shared by the
 // node is the first MaxDiffBytes of the whole branch diff, so a candidate in a file that
 // sorts late gets evidence that cannot contain the answer - and the judge answers anyway,
@@ -894,6 +1014,10 @@ func TestEscalationFailureKeepsTheFinding(t *testing.T) {
 	for _, b := range out.Confirmed {
 		if strings.Contains(b.Desc, "deploy.py") && b.Verdict == run.VerdictCheckedButUnverified {
 			kept = true
+			// S7 (T0.1): the secondOpinion path mints its bug id with the file-aware FindingKey too.
+			if b.ID != run.FindingKey("server.go", "server.go disagrees with scripts/deploy.py") {
+				t.Errorf("escalation-kept bug id must be the file-aware FindingKey: %q", b.ID)
+			}
 		}
 	}
 	if !kept {
diff --git a/internal/fsm/machine/fork_test.go b/internal/fsm/machine/fork_test.go
index b81325c..a113781 100644
--- a/internal/fsm/machine/fork_test.go
+++ b/internal/fsm/machine/fork_test.go
@@ -702,7 +702,7 @@ func TestF7DiffRuns(t *testing.T) {
 			if err := llmCall(in, i, 10); err != nil {
 				return nil, err
 			}
-			bugs = append(bugs, run.Bug{ID: run.BugID(f.IssueText), Desc: f.IssueText, Verdict: "real_but_ungold", Confidence: 0.9})
+			bugs = append(bugs, run.Bug{ID: run.FindingKey(f.File, f.IssueText), Desc: f.IssueText, Verdict: "real_but_ungold", Confidence: 0.9})
 		}
 		return json.RawMessage(run.MarshalCanonical(run.Delta{Confirmed: bugs})), nil
 	}
diff --git a/internal/fsm/machine/harness_test.go b/internal/fsm/machine/harness_test.go
index a8f0d7b..defb232 100644
--- a/internal/fsm/machine/harness_test.go
+++ b/internal/fsm/machine/harness_test.go
@@ -131,7 +131,7 @@ func newRegistry() *fakeRegistry {
 			if err := llmCall(in, i, 10); err != nil {
 				return nil, err
 			}
-			bugs = append(bugs, run.Bug{ID: run.BugID(f.IssueText), Desc: f.IssueText, Verdict: "real_but_ungold", Confidence: 0.9})
+			bugs = append(bugs, run.Bug{ID: run.FindingKey(f.File, f.IssueText), Desc: f.IssueText, Verdict: "real_but_ungold", Confidence: 0.9})
 		}
 		return json.RawMessage(run.MarshalCanonical(run.Delta{Confirmed: bugs})), nil
 	}}
@@ -457,7 +457,9 @@ func wantCodeE(t *testing.T, err error, code string) *errs.Error {
 func findings(n int) string {
 	var fs []run.Finding
 	for i := 0; i < n; i++ {
-		fs = append(fs, run.Finding{IssueText: fmt.Sprintf("bug %d", i), File: "f.go", Line: i + 1})
+		// Digit-free, mutually distinct text: the frozen finding identity (T0.1) collapses digit runs,
+		// so "bug 0"/"bug 1" would share one id and N findings would fold to one. Distinguish by letter.
+		fs = append(fs, run.Finding{IssueText: "bug " + string(rune('a'+i)), File: "f.go", Line: i + 1})
 	}
 	return string(run.MarshalCanonical(run.Delta{Findings: fs}))
 }
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index d268358..6cdc69d 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -624,10 +624,10 @@ func TestM4Convergence(t *testing.T) {
 		t.Fatalf("loop 1: %+v", r)
 	}
 	// iteration 1: fix its own new bug and one old one → 7 of 9 remain: progress, not fixed, loop again
-	newBug := run.BugID("bug 8")
-	present = func(id string) bool { return id != newBug && id != run.BugID("bug 0") }
+	newBug := run.FindingKey("", "bug new") // the iter-1 discover finding carries no file
+	present = func(id string) bool { return id != newBug && id != run.FindingKey("f.go", "bug a") }
 	h.advance(m)
-	h.record(m, "discover", string(run.MarshalCanonical(run.Delta{Findings: []run.Finding{{IssueText: "bug 8"}}})))
+	h.record(m, "discover", string(run.MarshalCanonical(run.Delta{Findings: []run.Finding{{IssueText: "bug new"}}})))
 	h.advance(m)
 	h.advance(m)
 	h.advance(m)
diff --git a/internal/fsm/run/canonical.go b/internal/fsm/run/canonical.go
index 8c48fea..90d39a9 100644
--- a/internal/fsm/run/canonical.go
+++ b/internal/fsm/run/canonical.go
@@ -2,7 +2,6 @@ package run
 
 import (
 	"bytes"
-	"crypto/sha1"
 	"crypto/sha256"
 	"encoding/hex"
 	"encoding/json"
@@ -99,12 +98,6 @@ func LineHash(line []byte) string {
 	return hex.EncodeToString(sum[:])
 }
 
-// BugID is the stable identity of a finding: hex(sha1(issueText))[:12].
-func BugID(issueText string) string {
-	sum := sha1.Sum([]byte(issueText))
-	return hex.EncodeToString(sum[:])[:12]
-}
-
 // Key builds the node@iter key used by NodeOutputs/Applied.
 func Key(node string, iter int) string { return node + "@" + strconv.Itoa(iter) }
 
diff --git a/internal/fsm/run/canonical_test.go b/internal/fsm/run/canonical_test.go
index c240a53..9b18f0d 100644
--- a/internal/fsm/run/canonical_test.go
+++ b/internal/fsm/run/canonical_test.go
@@ -96,8 +96,10 @@ func TestOutputHashAndLineHash(t *testing.T) {
 
 // R13: value pins the statement-coverage gate cannot see.
 func TestValuePins(t *testing.T) {
-	if got := BugID("x"); got != "11f6ad8ec52a" {
-		t.Fatalf("BugID(x) = %q", got)
+	// The pre-T0.1 text-only identity (BugID, now removed) survives only as the migration's scheme-0
+	// derivation; its value is pinned so a persisted scheme-0 id stays reproducible from retained text.
+	if got := FindingKeyForScheme(0, "any.go", "x"); got != "11f6ad8ec52a" {
+		t.Fatalf("FindingKeyForScheme(0, x) = %q", got)
 	}
 	if got := Key("n", 2); got != "n@2" {
 		t.Fatalf("Key = %q", got)
diff --git a/internal/fsm/run/identity.go b/internal/fsm/run/identity.go
index 7c37116..fa57f6d 100644
--- a/internal/fsm/run/identity.go
+++ b/internal/fsm/run/identity.go
@@ -62,6 +62,32 @@ func FindingKey(file, text string) string {
 // old scheme are MIGRATED forward from the retained source text — never silently orphaned.
 const FindingScheme = 1
 
+// FindingKeyForScheme derives a finding id under a specific historical FindingScheme, so a persisted
+// id minted under an older scheme can be reproduced from the retained (file, text) and translated
+// forward (spike §5.2 migrate-on-read). Scheme ≤ 0 is the pre-T0.1 derivation — hex(sha1(text))[:12],
+// the file IGNORED — reproduced here rather than via a public BugID (which T0.1 removes). Any current
+// scheme is the file-aware FindingKey.
+//
+// When FindingScheme is next bumped and FindingKey's derivation changes, this switch must gain a case
+// that reproduces the retiring scheme's exact bytes; until then scheme 1 is FindingKey by identity.
+func FindingKeyForScheme(scheme int, file, text string) string {
+	if scheme <= 0 {
+		sum := sha1.Sum([]byte(text)) // #nosec G401 -- identity digest, not security
+		return hex.EncodeToString(sum[:])[:12]
+	}
+	return FindingKey(file, text)
+}
+
+// MigrateFindingID translates a finding id minted under fromScheme forward to the current
+// FindingScheme, recomputing from the retained (file, text), and reports whether the id moved. A pin
+// or override keyed on the old id is re-found by looking it up under the returned current id rather
+// than orphaning it (spike §5.2(a)). changed is false when the id was already current — the no-op
+// case — which is exactly when fromScheme's derivation already equals the current one.
+func MigrateFindingID(fromScheme int, file, text string) (current string, changed bool) {
+	current = FindingKey(file, text)
+	return current, FindingKeyForScheme(fromScheme, file, text) != current
+}
+
 // ContinuityThreshold (τ) is the frozen Jaccard bar at/above which two SAME-FILE findings are the
 // same fault for continuity (Unproven clear, class carry). Frozen 2026-09-01 against the pre-locked
 // ground truth: T4 recall 92% (≥90% floor) and precision 100% on 274 real same-file distinct-fault
diff --git a/internal/fsm/run/identity_test.go b/internal/fsm/run/identity_test.go
index 510e2f2..d277077 100644
--- a/internal/fsm/run/identity_test.go
+++ b/internal/fsm/run/identity_test.go
@@ -113,6 +113,71 @@ func TestSameFault(t *testing.T) {
 	}
 }
 
+// --- §5.2 identity-scheme migration (versioned derivation, migrate-on-read) ---
+
+// FindingKeyForScheme must reproduce each historical derivation exactly, so a persisted id minted
+// under an older scheme can be recognized and translated forward from the retained (file,text).
+func TestFindingKeyForSchemeReproducesHistory(t *testing.T) {
+	// Scheme 0 is the pre-T0.1 text-only sha1 — file IGNORED — so its value is pinned to the
+	// historical BugID output and is identical across files.
+	if got := FindingKeyForScheme(0, "any/file.go", "x"); got != "11f6ad8ec52a" {
+		t.Fatalf("scheme 0 must reproduce the historical text-only id, got %q", got)
+	}
+	if FindingKeyForScheme(0, "a.go", "x") != FindingKeyForScheme(0, "b.go", "x") {
+		t.Error("scheme 0 ignores the file (text-only), so two files share the id")
+	}
+	// The current scheme is file-aware and equals FindingKey.
+	if FindingKeyForScheme(FindingScheme, "a.go", "x") != FindingKey("a.go", "x") {
+		t.Error("the current scheme must equal FindingKey")
+	}
+	if FindingKeyForScheme(FindingScheme, "a.go", "x") == FindingKeyForScheme(FindingScheme, "b.go", "x") {
+		t.Error("the current scheme is file-aware")
+	}
+	// The change of derivation is real: the same finding hashes differently under 0 and the current.
+	if FindingKeyForScheme(0, "a.go", "x") == FindingKeyForScheme(FindingScheme, "a.go", "x") {
+		t.Error("scheme 0 and the current scheme must differ, or there is nothing to migrate")
+	}
+}
+
+// MigrateFindingID translates a persisted id forward and reports whether it moved, so an override or
+// Unproven gap keyed on the old id is re-found by translation, never orphaned (§5.2(a)). Dropping the
+// scheme switch (FindingKeyForScheme ignoring the scheme) makes changed always false and orphans the
+// override — the mutation this test kills.
+func TestMigrateFindingIDResolvesAnOldKeyedOverride(t *testing.T) {
+	file, text := "internal/fsm/kind/kind.go", "the guard is unpinned"
+	// A run persisted under scheme 0 recorded an override keyed on the old finding id.
+	oldID := FindingKeyForScheme(0, file, text)
+	override := map[string]string{oldID: "granted: accepted risk"}
+	// The new binary computes the current id; looking the override up under it would orphan it,
+	// unless the migration translates the old id space forward from the retained (file,text).
+	newID, changed := MigrateFindingID(0, file, text)
+	if !changed {
+		t.Fatal("migrating a scheme-0 id to the current file-aware scheme must report a change")
+	}
+	if newID != FindingKey(file, text) {
+		t.Fatalf("migration must yield the current-scheme id, got %q", newID)
+	}
+	if _, orphaned := override[newID]; orphaned {
+		t.Fatal("test bug: the new id must not already be present in the old-keyed map")
+	}
+	// Migrate-on-read: rebuild the override under the current id space from the retained (file,text).
+	migrated := map[string]string{}
+	for old, reason := range override {
+		if FindingKeyForScheme(0, file, text) != old {
+			t.Fatalf("the persisted old id must reproduce from retained text")
+		}
+		cur, _ := MigrateFindingID(0, file, text)
+		migrated[cur] = reason
+	}
+	if _, ok := migrated[newID]; !ok {
+		t.Fatal("the override must resolve under the current id after migration")
+	}
+	// An id already at the current scheme does not move.
+	if _, changed := MigrateFindingID(FindingScheme, file, text); changed {
+		t.Error("an id already at the current scheme must not report a change")
+	}
+}
+
 // --- The T0.1 frozen-floor gate over the pre-locked labeled set ---
 
 type variant struct {



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

# T0.1 evidence — apply the frozen finding-identity at every identity-sensitive site (the rewire)

**Task:** T0.1 (Epic E0) — the rewire half. The spike, ground truth, and frozen primitives
(`FindingKey`/`SameFault`/`ContinuityThreshold`/`FindingScheme`) landed in #37; this completes T0.1 by
applying them at every identity-sensitive site and landing the §5.1(a) golden-domain and §5.2
migration decisions.
**Spec:** `docs/specs/2026-09-01-t0.1-finding-identity-spike.md` §3 (S1–S7), §5.1, §5.2, §9.8 R6.
**Plan:** `docs/plans/2026-08-31-pins-bug-class-decomposition.md` T0.1.
**Base:** `main` (`7adebfa`).

## What landed

- **S1 — `run.BugID` removed** (`internal/fsm/run/canonical.go`). The text-only `hex(sha1(issueText))[:12]`
  is superseded by the file-aware `run.FindingKey`. Its only surviving trace is the §5.2 migration's
  scheme-0 branch (`FindingKeyForScheme(0, …)`), which reproduces the exact pre-T0.1 bytes so a
  persisted scheme-0 id stays recognizable. Value pin retained (`FindingKeyForScheme(0,"any.go","x") ==
  "11f6ad8ec52a"`).
- **S2 — `dedupCandidates`** (`internal/fsm/kind/kind.go`): keyed on `run.FindingKey(f.File, f.IssueText)`
  instead of raw `f.IssueText`. The one site a `BugID`-only grep misses.
- **S4 — the known-id cap set** (`allIDs`): `run.FindingKey(c.File, c.IssueText)`.
- **S5 — matched golden (§5.1(a), APPROVED):** the matched golden inherits the winning candidate's file,
  `run.FindingKey(cands[winner].File, golden.Comment)`, so its id shares one `(file,text)` domain with
  every candidate id — closing the review #35b fileless-golden / file-keyed-candidate split.
- **S6/S7 — candidate mints** (the four confirm-loop returns + the four `secondOpinion` returns):
  `run.FindingKey(cand.File, cand.IssueText)`.
- **§5.2 migration (APPROVED):** `internal/fsm/run/identity.go` gains `FindingKeyForScheme(scheme,file,
  text)` (scheme ≤ 0 = pre-T0.1 text-only sha1; current = `FindingKey`) and `MigrateFindingID(fromScheme,
  file,text) (current, changed)` — migrate-on-read from the retained `(file,text)`, so a pin/override
  keyed on an old id is re-found rather than orphaned. Scope is the FSM id (`run.BugID`), not reviewlog
  `mrvf-*` ids. No live consumer yet (Unproven clear / class carry is T1.3+); delivered as a tested
  primitive, exactly as `SameFault` was.

## The §3.3 site map, re-grepped against live code (line numbers drifted since #37)

| Site | Location | Now keys on |
| --- | --- | --- |
| S1 derivation | `run/canonical.go` (removed) | — (→ `FindingKey`) |
| S2 `dedupCandidates` | `kind.go` | `FindingKey(file,text)` |
| S4 `allIDs` | `kind.go` | `FindingKey(file,text)` |
| S5 matched-golden | `kind.go` | `FindingKey(winnerFile,comment)` |
| S6 confirm-loop mints | `kind.go` (×4) | `FindingKey(cand.File,cand.IssueText)` |
| S7 `secondOpinion` mints | `kind.go` (×4) | `FindingKey(cand.File,cand.IssueText)` |

## TDD (red observed first)

- **S2:** `TestDedupCandidatesIsFileAware` written first → observed red (collapsed same-text/different-file
  to 1) → S2 rewired → green.
- **Migration:** `TestFindingKeyForSchemeReproducesHistory` / `TestMigrateFindingIDResolvesAnOldKeyedOverride`
  written first → compile-red (undefined) → implemented → green.
- Per-path same-text/different-file regressions added: `TestDedupBugsKeysOnFileAwareID` (S3),
  `TestAdjudicateKeepsSameTextInDifferentFilesDistinct` (S2+S4+S6), `TestMatchedGoldenInheritsWinnerFileDomain`
  (S5), `TestAdjudicatePreflightCountsSameTextDifferentFilesAsTwo` (S4), plus a file-aware id assertion
  in `TestEscalationFailureKeepsTheFinding` (S7).

## Fixture updates forced by the frozen semantics (not product changes)

`NormalizeText` collapses digit runs to a `#` token (the maintainer-approved T2 property: `"line 42" ≡
"line 51"`). Test fixtures that distinguished findings only by a trailing digit — `"c0"/"c1"/"c2"`,
`"bug 0"/"bug 1"`, `"t0".."t249"` — therefore now share one identity and are invalid. They were updated
to distinguish by a digit-free token or a distinct file (`kind_test.go`, machine `findings(n)` helper,
the `big`/`fat` cap fixtures). The frozen-floor gate `TestT0IdentityMeetsFrozenFloors` was **not**
touched (τ/NormalizeText unchanged): still T1–T3 exact 100%, T4 recall 11/12, precision negatives 0.

## Golden fixtures

`internal/fsm/run/testdata/golden-log.jsonl` and `golden-snapshots.jsonl` build confirmed bugs with
**literal** ids (`bug("b1")`), independent of the identity derivation, so they did **not** drift and
needed no regeneration. `TestGoldenPrefixes` stays green and the files are byte-identical.

## Mutation verification (each applied, observed red, restored — not a claimed kill)

| Predicate | Mutation | Killing test |
| --- | --- | --- |
| S2 dedup file-aware | drop the file → `FindingKey("", text)` | `TestDedupCandidatesIsFileAware` |
| S4 cap-set file-aware | drop the file | `TestAdjudicatePreflightCountsSameTextDifferentFilesAsTwo` |
| S5 golden inherits winner file | `FindingKey("", comment)` | `TestMatchedGoldenInheritsWinnerFileDomain` |
| S6 real_but_ungold mint | drop the file (line 563) | `TestAdjudicateKeepsSameTextInDifferentFilesDistinct` |
| S7 secondOpinion mint | drop the file (line 612) | `TestEscalationFailureKeepsTheFinding` |
| migration scheme switch | `scheme <= 0` → `scheme <= -1` | `TestFindingKeyForSchemeReproducesHistory`, `TestMigrateFindingID…` |
| migration changed flag | `!= current` → `== current` | `TestMigrateFindingIDResolvesAnOldKeyedOverride` |

## Verification (all green)

- `go build ./...` — OK.
- `go test ./...` — all `ok`, no failures.
- `bash tests/coverage.sh` — coverage gate passed; `internal/fsm/{run,kind,machine}` at 100.0%.
- `gofmt -l internal cmd` — clean.

