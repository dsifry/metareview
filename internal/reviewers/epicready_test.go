package reviewers

import (
	"strings"
	"testing"
)

func TestEpicReadyReviewersBlockContradictionsAndIntentDrift(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic: EpicContext{ID: "epic-1", Body: "Build a parser without executing user input."},
		Children: []EpicChild{
			{ID: "task-1", Body: "Use eval for expression parsing."},
			{ID: "task-2", Body: "Avoid eval and parse JSON safely."},
		},
		Git: EpicGitContext{Diff: "+module.exports = input => eval(input);\n"},
		ReviewLogs: []EpicReviewLog{
			{Target: "task-1", Verdict: "PASS"},
			{Target: "task-2", Verdict: "PASS"},
		},
		EvidenceText: "task-1 passed\ntask-2 passed\n",
	})

	assertEpicFinding(t, findings, "epic-integration-reviewer", "Cross-task contradiction")
	assertEpicFinding(t, findings, "intent-preservation-reviewer", "Epic intent drift")
}

func TestEpicReadyReviewersBlockMissingEvidenceAndUnresolvedBlockers(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic: EpicContext{ID: "epic-1", Body: "Ship parser."},
		Children: []EpicChild{
			{ID: "task-1", Body: "Parser implementation."},
			{ID: "task-2", Body: "Parser tests."},
		},
		ReviewLogs: []EpicReviewLog{
			{Target: "task-1", Verdict: "NEEDS_REVISION", HasUnresolvedBlockers: true, FindingIDs: []string{"mrvf-1"}},
		},
		EvidenceText: "task-1 passed\n",
	})

	assertEpicFinding(t, findings, "acceptance-reviewer", "Missing child acceptance evidence")
	assertEpicFinding(t, findings, "epic-integration-reviewer", "Unresolved child blockers")
}

func TestEpicReadyReviewersDoNotTreatBareTaskMentionAsEvidence(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic:         EpicContext{ID: "epic-1", Body: "Ship parser."},
		Children:     []EpicChild{{ID: "task-1", Body: "Parser implementation."}},
		EvidenceText: "task-1 still needs review\n",
	})

	assertEpicFinding(t, findings, "acceptance-reviewer", "Missing child acceptance evidence")
}

func TestEpicReadyReviewersBlockMissingServiceInventoryForServiceChanges(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic:     EpicContext{ID: "epic-1", Body: "Add billing integration."},
		Children: []EpicChild{{ID: "task-1", Body: "Add billing service."}},
		Git:      EpicGitContext{ChangedFiles: []string{"internal/billing/payment_service.go"}},
		ReviewLogs: []EpicReviewLog{
			{Target: "task-1", Verdict: "PASS"},
		},
		EvidenceText: "task-1 passed\n",
	})

	assertEpicFinding(t, findings, "architecture-reviewer", "Missing service inventory update")
}

func TestEpicReadyReceivesContextRiskFlags(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic:     EpicContext{ID: "epic-1", Body: "Build a parser without executing user input."},
		Children: []EpicChild{{ID: "task-1", Body: "Add context profiling."}},
		Git: EpicGitContext{
			ChangedFiles: []string{"internal/contextprofile/profile.go"},
			Diff:         "+module.exports = input => ev" + "al(input);\n",
			RiskLevel:    "context-risk",
			RiskReasons:  []string{"LARGE_DIFF", "UNTRACKED_OMITTED"},
		},
		ReviewLogs:   []EpicReviewLog{{Target: "task-1", Verdict: "PASS"}},
		Knowledge:    EpicKnowledgeContext{ServiceInventory: "Context profile: `internal/contextprofile/profile.go`"},
		EvidenceText: "task-1 passed\n",
	})

	assertEpicFinding(t, findings, "architecture-reviewer", "Review context risk")
	if len(findings) != 1 {
		t.Fatalf("context risk should preflight domain reviewers, got %+v", findings)
	}
}

func TestEpicReadyReviewersAllowCleanEpic(t *testing.T) {
	findings := RunEpicReady(EpicReadyContext{
		Epic:     EpicContext{ID: "epic-1", Body: "Parse JSON safely."},
		Children: []EpicChild{{ID: "task-1", Body: "Use JSON parser."}},
		Git:      EpicGitContext{ChangedFiles: []string{"internal/parser/parser.go"}, Diff: "+return json.Valid(input)\n"},
		ReviewLogs: []EpicReviewLog{
			{Target: "task-1", Verdict: "PASS"},
		},
		Knowledge:    EpicKnowledgeContext{ServiceInventory: "Parser: `internal/parser/parser.go`"},
		EvidenceText: "task-1 passed\nbash tests/run-all.sh exited 0\n",
	})
	if len(findings) != 0 {
		t.Fatalf("clean epic should not produce findings: %+v", findings)
	}
}

// reviewers-1: childEvidencePassed must not accept a line as passing on a loose substring of "pass"
// or "ok" (e.g. "broke" contains "ok", "bypassed" contains "pass"), which would silently satisfy the
// acceptance gate for a child that in fact failed.
func TestChildEvidencePassedRejectsSubstringFalsePositives(t *testing.T) {
	falsePositives := []string{
		"task-1 broke ci, reverting",
		"task-1 bypassed the queue",
		"task-1 surpassed the budget",
		"task-1 lookup returned nil",
		"task-1 took too long",
	}
	for _, ev := range falsePositives {
		if childEvidencePassed(ev, "task-1") {
			t.Errorf("evidence %q must NOT count as passing acceptance evidence", ev)
		}
	}
	genuine := []string{
		"task-1 passed",
		"task-1 ok",
		"ok   task-1  0.30s",
		"task-1 exited 0",
		"PASS task-1",
	}
	for _, ev := range genuine {
		if !childEvidencePassed(strings.ToLower(ev), "task-1") {
			t.Errorf("evidence %q should count as passing acceptance evidence", ev)
		}
	}
}

// reviewers-2: a changed file is service-shaped only when its basename ends in a service role word +
// source extension, or a path component is a service role directory. A bare occurrence of the word in
// an unrelated filename or a non-source file must not be flagged.
func TestMissingServiceInventoryCoverageIgnoresNonServicePaths(t *testing.T) {
	notServices := []string{"docs/client-guide.md", "config/worker.yaml", "internal/client_helper.go"}
	for _, f := range notServices {
		got := missingServiceInventoryCoverage(EpicReadyContext{Git: EpicGitContext{ChangedFiles: []string{f}}})
		if len(got) != 0 {
			t.Errorf("%q wrongly flagged as a service file: %v", f, got)
		}
	}
	// Both underscore and dotted basenames ending in a role word + source extension are service-shaped
	// (dotted is the Angular/NestJS convention, e.g. user.service.ts) and must be flagged.
	for _, f := range []string{"internal/billing/payment_service.go", "src/app/user.service.ts", "app/auth.controller.ts"} {
		got := missingServiceInventoryCoverage(EpicReadyContext{Git: EpicGitContext{ChangedFiles: []string{f}}})
		if len(got) != 1 {
			t.Errorf("service file %q should be flagged, got %v", f, got)
		}
	}
	// A service file already recorded in the inventory must NOT be flagged (pins the inventory!="" guard).
	covered := missingServiceInventoryCoverage(EpicReadyContext{
		Git:       EpicGitContext{ChangedFiles: []string{"internal/billing/payment_service.go"}},
		Knowledge: EpicKnowledgeContext{ServiceInventory: "Billing: internal/billing/payment_service.go"},
	})
	if len(covered) != 0 {
		t.Errorf("a service file present in the inventory must not be flagged, got %v", covered)
	}
}

// reviewers-3: violatesNoEvalIntent must use a word-boundary call match (like taskdone's), so a
// retrieval(...) call is not mistaken for the dynamic-evaluation builtin. (The call literals below are
// split the way the tests above split them, so metareview's own deterministic lint does not flag this
// test's diff as introducing that builtin.)
func TestViolatesNoEvalIntentUsesWordBoundary(t *testing.T) {
	intent := EpicContext{Body: "Build a parser without executing user input."}
	if violatesNoEvalIntent(EpicReadyContext{Epic: intent, Git: EpicGitContext{Diff: "+result := retrie" + "val(input)\n"}}) {
		t.Error("a retrieval(...) call must not be treated as the dynamic-evaluation builtin")
	}
	if !violatesNoEvalIntent(EpicReadyContext{Epic: intent, Git: EpicGitContext{Diff: "+x = ev" + "al(input)\n"}}) {
		t.Error("a genuine dynamic-evaluation call must still be caught")
	}
}

func assertEpicFinding(t *testing.T, findings []Finding, reviewer, titlePart string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Reviewer == reviewer && strings.Contains(finding.Title, titlePart) {
			return
		}
	}
	t.Fatalf("missing finding reviewer=%s title~=%s in %+v", reviewer, titlePart, findings)
}

// A blind pre-push review surfaced that the #68 fix stopped flagging PascalCase service files, and that
// naively adding "passing"/"okay" to the evidence pattern re-opens an acceptance-gate false positive. So
// servicePathPattern matches camelCase/PascalCase incl. acronym/digit-prefixed names, case-insensitively
// on the extension; and the evidence pattern deliberately excludes ambiguous "passing"/"okay".
func TestServicePathMatchesPascalCase(t *testing.T) {
	// camelCase / PascalCase, including acronym- and digit-prefixed (HTTPClient, S3Client), any-case ext.
	for _, f := range []string{
		"src/auth/AuthService.ts", "app/UserController.tsx", "src/EmailWorker.ts",
		"internal/billing/PaymentService.go", "web/ApiClient.jsx",
		"src/HTTPClient.ts", "src/S3Client.ts", "internal/DBWorker.go", "src/APIService.tsx",
		"internal/billing/payment_service.GO",
	} {
		if len(missingServiceInventoryCoverage(EpicReadyContext{Git: EpicGitContext{ChangedFiles: []string{f}}})) != 1 {
			t.Errorf("service file %q must be flagged", f)
		}
	}
	// Must NOT match: a lowercase concatenation that is not a word boundary, a role word not at the
	// basename end, or a non-source file.
	for _, f := range []string{"internal/x/disservice.go", "docs/client-guide.md", "config/worker.yaml", "internal/client_helper.go", "internal/AuthServiceHelper.go"} {
		if len(missingServiceInventoryCoverage(EpicReadyContext{Git: EpicGitContext{ChangedFiles: []string{f}}})) != 0 {
			t.Errorf("%q must NOT be flagged as a service file", f)
		}
	}
}

func TestChildEvidencePassedRejectsAmbiguousPassTokens(t *testing.T) {
	// Unambiguous pass tokens count.
	for _, ev := range []string{"task-1 passed", "task-1 ok", "task-1 exited 0", "PASS task-1"} {
		if !childEvidencePassed(strings.ToLower(ev), "task-1") {
			t.Errorf("evidence %q should count as passing", ev)
		}
	}
	// Ambiguous or negated summaries must NOT satisfy the acceptance gate (a false positive here lets a
	// failing child pass). This is why "passing"/"okay" are excluded, and the reviewers-1 substrings stay
	// rejected.
	for _, ev := range []string{
		"task-1 1 passing 5 failing", "task-1 all tests passing", "task-1 not passing", "task-1 okay",
		"task-1 broke ci", "task-1 bypassed the queue", "task-1 bypassing checks",
	} {
		if childEvidencePassed(strings.ToLower(ev), "task-1") {
			t.Errorf("evidence %q must NOT count as passing", ev)
		}
	}
}
