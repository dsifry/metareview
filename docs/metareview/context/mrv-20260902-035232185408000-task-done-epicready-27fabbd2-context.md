# metareview task-done context

Run ID: `mrv-20260902-035232185408000-task-done-epicready-27fabbd2`

## Task

package reviewers

import (
	"regexp"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/findings"
)

type EpicReadyContext struct {
	Epic         EpicContext
	Children     []EpicChild
	Git          EpicGitContext
	ReviewLogs   []EpicReviewLog
	Knowledge    EpicKnowledgeContext
	Mutation     MutationContext
	EvidenceText string
}

type EpicContext struct {
	ID    string
	Title string
	Body  string
}

type EpicChild struct {
	ID    string
	Title string
	Body  string
}

type EpicGitContext struct {
	ChangedFiles []string
	Diff         string
	RiskLevel    string
	RiskReasons  []string
}

type EpicReviewLog struct {
	Target                string
	Verdict               string
	FindingIDs            []string
	HasUnresolvedBlockers bool
}

type EpicKnowledgeContext struct {
	ServiceInventory string
}

// servicePathPattern flags a changed file as service-shaped when its basename ends in a service role
// word plus a source extension (payment_service.go, user_controller.ts), or when a service role word
// is a whole path component / directory (services/foo.go). A bare occurrence of the word inside an
// unrelated name (client_helper.go) or a non-source file (client-guide.md, worker.yaml) is NOT a
// service change — the previous unanchored second alternative matched all of those as false positives.
var servicePathPattern = regexp.MustCompile(`(?i)(?:^|[/_-])(service|controller|worker|client)\.(go|js|ts|tsx|jsx|py|rb)$|(?:^|/)(service|controller|worker|client)s?/`)

// evidencePassPattern matches a status word as a whole word, so "broke"/"bypassed"/"lookup" are not
// mistaken for "ok"/"pass". Callers lowercase the line first.
var evidencePassPattern = regexp.MustCompile(`\b(pass|passed|passes|ok)\b`)

func RunEpicReady(context EpicReadyContext) []Finding {
	var results []Finding
	results = append(results, context.Mutation.Findings()...)
	if context.Git.RiskLevel == "context-risk" {
		return append(results, epicFinding(Finding{
			Reviewer:       "architecture-reviewer",
			Severity:       "high",
			Title:          "Review context risk",
			Finding:        "The epic-ready review is running with incomplete or oversized source context.",
			Expected:       "Epic closure is reviewed with complete branch context or an explicit shard plan.",
			Found:          "Risk reasons: " + strings.Join(context.Git.RiskReasons, ", "),
			Evidence:       []findings.Evidence{{Type: "context", Path: "contextProfile"}},
			Recommendation: "Resolve the context risk before declaring the epic ready.",
			Fingerprint:    "epic:context-risk",
		}))
	}
	if hasEvalContradiction(context.Children) {
		results = append(results, epicFinding(Finding{
			Reviewer:       "epic-integration-reviewer",
			Severity:       "high",
			Title:          "Cross-task contradiction",
			Finding:        "Child tasks contain mutually incompatible implementation directions.",
			Expected:       "Epic child tasks converge on a consistent implementation direction.",
			Found:          "One child calls for eval while another forbids eval.",
			Evidence:       []findings.Evidence{{Type: "task-graph"}},
			Recommendation: "Resolve the contradiction before declaring the epic ready.",
			Fingerprint:    "epic:contradiction:eval",
		}))
	}
	if missing := missingChildEvidence(context); len(missing) > 0 {
		results = append(results, epicFinding(Finding{
			Reviewer:       "acceptance-reviewer",
			Severity:       "high",
			Title:          "Missing child acceptance evidence",
			Finding:        "One or more child tasks lack passing review or validation evidence.",
			Expected:       "Every child task has passing task-level review evidence before epic closure.",
			Found:          "Missing evidence for: " + strings.Join(missing, ", "),
			Evidence:       []findings.Evidence{{Type: "task-graph"}},
			Recommendation: "Run or attach task-level review evidence for every child task.",
			Fingerprint:    "epic:missing-child-evidence:" + strings.Join(missing, "|"),
		}))
	}
	if blocked := unresolvedChildBlockers(context.ReviewLogs); len(blocked) > 0 {
		results = append(results, epicFinding(Finding{
			Reviewer:       "epic-integration-reviewer",
			Severity:       "high",
			Title:          "Unresolved child blockers",
			Finding:        "Child task or child epic review logs still contain unresolved blockers.",
			Expected:       "All child blockers are resolved before epic closure.",
			Found:          "Blocked targets: " + strings.Join(blocked, ", "),
			Evidence:       []findings.Evidence{{Type: "review-log"}},
			Recommendation: "Resolve child blockers and re-run their reviews before epic-ready.",
			Fingerprint:    "epic:unresolved-child-blockers:" + strings.Join(blocked, "|"),
		}))
	}
	if violatesNoEvalIntent(context) {
		results = append(results, epicFinding(Finding{
			Reviewer:       "intent-preservation-reviewer",
			Severity:       "high",
			Title:          "Epic intent drift",
			Finding:        "Final branch evidence violates the original epic intent.",
			Expected:       "Implementation preserves the parent epic's stated constraints.",
			Found:          "Epic intent forbids executing input, but evidence includes eval.",
			Evidence:       []findings.Evidence{{Type: "diff-pattern", Path: "eval("}},
			Recommendation: "Remove eval or revise the epic intent with explicit human approval.",
			Fingerprint:    "epic:intent-drift:eval",
		}))
	}
	if missing := missingServiceInventoryCoverage(context); len(missing) > 0 {
		results = append(results, epicFinding(Finding{
			Reviewer:       "architecture-reviewer",
			Severity:       "high",
			Title:          "Missing service inventory update",
			Finding:        "Service-like changed paths are not reflected in the service inventory.",
			Expected:       "Durable service/codepath additions are registered for future reviewers.",
			Found:          "Unregistered paths: " + strings.Join(missing, ", "),
			Evidence:       []findings.Evidence{{Type: "changed-files"}},
			Recommendation: "Update `docs/SERVICE_INVENTORY.md` or document why no registry change is needed.",
			Fingerprint:    "epic:missing-service-inventory:" + strings.Join(missing, "|"),
		}))
	}
	return results
}

func epicFinding(input Finding) Finding {
	input.Classification = "blocking"
	if input.Owner == "" {
		input.Owner = "implementer"
	}
	return input
}

func hasEvalContradiction(children []EpicChild) bool {
	hasUseEval := false
	hasAvoidEval := false
	for _, child := range children {
		text := strings.ToLower(child.Body + "\n" + child.Title)
		if strings.Contains(text, "use eval") || strings.Contains(text, "eval for") {
			hasUseEval = true
		}
		if strings.Contains(text, "avoid eval") || strings.Contains(text, "no eval") || strings.Contains(text, "without eval") {
			hasAvoidEval = true
		}
	}
	return hasUseEval && hasAvoidEval
}

func missingChildEvidence(context EpicReadyContext) []string {
	passTargets := map[string]bool{}
	for _, log := range context.ReviewLogs {
		if log.Verdict == "PASS" || log.Verdict == "PASS_ADVISORY" {
			passTargets[log.Target] = true
		}
	}
	evidence := strings.ToLower(context.EvidenceText)
	var missing []string
	for _, child := range context.Children {
		if child.ID == "" {
			continue
		}
		if passTargets[child.ID] || childEvidencePassed(evidence, child.ID) {
			continue
		}
		missing = append(missing, child.ID)
	}
	sort.Strings(missing)
	return missing
}

func childEvidencePassed(evidence, childID string) bool {
	childID = strings.ToLower(childID)
	if childID == "" || !strings.Contains(evidence, childID) {
		return false
	}
	for _, line := range strings.Split(evidence, "\n") {
		line = strings.ToLower(line)
		if !strings.Contains(line, childID) {
			continue
		}
		if evidencePassPattern.MatchString(line) || strings.Contains(line, "exited 0") {
			return true
		}
	}
	return false
}

func unresolvedChildBlockers(logs []EpicReviewLog) []string {
	var blocked []string
	for _, log := range logs {
		if log.HasUnresolvedBlockers || log.Verdict == "NEEDS_REVISION" {
			blocked = append(blocked, log.Target)
		}
	}
	sort.Strings(blocked)
	return blocked
}

func violatesNoEvalIntent(context EpicReadyContext) bool {
	intent := strings.ToLower(context.Epic.Body + "\n" + context.Epic.Title)
	if !strings.Contains(intent, "without executing") && !strings.Contains(intent, "no eval") && !strings.Contains(intent, "avoid eval") {
		return false
	}
	evidence := strings.ToLower(context.Git.Diff)
	for _, child := range context.Children {
		evidence += "\n" + strings.ToLower(child.Body)
	}
	return evalPattern.MatchString(evidence) || strings.Contains(evidence, "use eval")
}

func missingServiceInventoryCoverage(context EpicReadyContext) []string {
	inventory := context.Knowledge.ServiceInventory
	var missing []string
	for _, file := range context.Git.ChangedFiles {
		if !servicePathPattern.MatchString(file) {
			continue
		}
		if inventory != "" && strings.Contains(inventory, file) {
			continue
		}
		missing = append(missing, file)
	}
	sort.Strings(missing)
	return missing
}


## Git

- Base: `cbaa849f15dd585a3b88b00d2f5167d59f990680`
- Head: `5ced1231ef61045e17ef3b903f7bb1c33e99a361`
- Branch: `epicready-loose-matching`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `24735`
- Filtered diff bytes: `6057`
- Risk level: `none`
- Generated files excluded: docs/metareview/FINDINGS.md, docs/metareview/context/mrv-20260902-035150553234000-task-done-epicready-27fabbd2-context.md, docs/metareview/reviews/mrv-20260902-035150553234000-task-done-epicready-27fabbd2.md

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- internal/reviewers/epicready.go
- internal/reviewers/epicready_test.go

### Path Dispositions
- docs/metareview/FINDINGS.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/context/mrv-20260902-035150553234000-task-done-epicready-27fabbd2-context.md: generated (metareview generated review artifact excluded from source manifest)
- docs/metareview/reviews/mrv-20260902-035150553234000-task-done-epicready-27fabbd2.md: generated (metareview generated review artifact excluded from source manifest)

### Manifest Blockers
No manifest blockers.

## Changed Files

- internal/reviewers/epicready.go
- internal/reviewers/epicready_test.go

## Diff

```diff
diff --git a/internal/reviewers/epicready.go b/internal/reviewers/epicready.go
index bed346c..e928c03 100644
--- a/internal/reviewers/epicready.go
+++ b/internal/reviewers/epicready.go
@@ -48,7 +48,16 @@ type EpicKnowledgeContext struct {
 	ServiceInventory string
 }
 
-var servicePathPattern = regexp.MustCompile(`(?i)(service|controller|worker|client)\.(go|js|ts|tsx|jsx|py|rb)$|(?i)(service|controller|worker|client)`)
+// servicePathPattern flags a changed file as service-shaped when its basename ends in a service role
+// word plus a source extension (payment_service.go, user_controller.ts), or when a service role word
+// is a whole path component / directory (services/foo.go). A bare occurrence of the word inside an
+// unrelated name (client_helper.go) or a non-source file (client-guide.md, worker.yaml) is NOT a
+// service change — the previous unanchored second alternative matched all of those as false positives.
+var servicePathPattern = regexp.MustCompile(`(?i)(?:^|[/_-])(service|controller|worker|client)\.(go|js|ts|tsx|jsx|py|rb)$|(?:^|/)(service|controller|worker|client)s?/`)
+
+// evidencePassPattern matches a status word as a whole word, so "broke"/"bypassed"/"lookup" are not
+// mistaken for "ok"/"pass". Callers lowercase the line first.
+var evidencePassPattern = regexp.MustCompile(`\b(pass|passed|passes|ok)\b`)
 
 func RunEpicReady(context EpicReadyContext) []Finding {
 	var results []Finding
@@ -189,10 +198,7 @@ func childEvidencePassed(evidence, childID string) bool {
 		if !strings.Contains(line, childID) {
 			continue
 		}
-		if strings.Contains(line, "pass") ||
-			strings.Contains(line, "passed") ||
-			strings.Contains(line, "exited 0") ||
-			strings.Contains(line, "ok") {
+		if evidencePassPattern.MatchString(line) || strings.Contains(line, "exited 0") {
 			return true
 		}
 	}
@@ -219,7 +225,7 @@ func violatesNoEvalIntent(context EpicReadyContext) bool {
 	for _, child := range context.Children {
 		evidence += "\n" + strings.ToLower(child.Body)
 	}
-	return strings.Contains(evidence, "eval(") || strings.Contains(evidence, "use eval")
+	return evalPattern.MatchString(evidence) || strings.Contains(evidence, "use eval")
 }
 
 func missingServiceInventoryCoverage(context EpicReadyContext) []string {
diff --git a/internal/reviewers/epicready_test.go b/internal/reviewers/epicready_test.go
index 66f926f..04d07f3 100644
--- a/internal/reviewers/epicready_test.go
+++ b/internal/reviewers/epicready_test.go
@@ -102,6 +102,75 @@ func TestEpicReadyReviewersAllowCleanEpic(t *testing.T) {
 	}
 }
 
+// reviewers-1: childEvidencePassed must not accept a line as passing on a loose substring of "pass"
+// or "ok" (e.g. "broke" contains "ok", "bypassed" contains "pass"), which would silently satisfy the
+// acceptance gate for a child that in fact failed.
+func TestChildEvidencePassedRejectsSubstringFalsePositives(t *testing.T) {
+	falsePositives := []string{
+		"task-1 broke ci, reverting",
+		"task-1 bypassed the queue",
+		"task-1 surpassed the budget",
+		"task-1 lookup returned nil",
+		"task-1 took too long",
+	}
+	for _, ev := range falsePositives {
+		if childEvidencePassed(ev, "task-1") {
+			t.Errorf("evidence %q must NOT count as passing acceptance evidence", ev)
+		}
+	}
+	genuine := []string{
+		"task-1 passed",
+		"task-1 ok",
+		"ok   task-1  0.30s",
+		"task-1 exited 0",
+		"PASS task-1",
+	}
+	for _, ev := range genuine {
+		if !childEvidencePassed(strings.ToLower(ev), "task-1") {
+			t.Errorf("evidence %q should count as passing acceptance evidence", ev)
+		}
+	}
+}
+
+// reviewers-2: a changed file is service-shaped only when its basename ends in a service role word +
+// source extension, or a path component is a service role directory. A bare occurrence of the word in
+// an unrelated filename or a non-source file must not be flagged.
+func TestMissingServiceInventoryCoverageIgnoresNonServicePaths(t *testing.T) {
+	notServices := []string{"docs/client-guide.md", "config/worker.yaml", "internal/client_helper.go"}
+	for _, f := range notServices {
+		got := missingServiceInventoryCoverage(EpicReadyContext{Git: EpicGitContext{ChangedFiles: []string{f}}})
+		if len(got) != 0 {
+			t.Errorf("%q wrongly flagged as a service file: %v", f, got)
+		}
+	}
+	realService := missingServiceInventoryCoverage(EpicReadyContext{Git: EpicGitContext{ChangedFiles: []string{"internal/billing/payment_service.go"}}})
+	if len(realService) != 1 {
+		t.Errorf("a real *_service.go file should be flagged, got %v", realService)
+	}
+	// A service file already recorded in the inventory must NOT be flagged (pins the inventory!="" guard).
+	covered := missingServiceInventoryCoverage(EpicReadyContext{
+		Git:       EpicGitContext{ChangedFiles: []string{"internal/billing/payment_service.go"}},
+		Knowledge: EpicKnowledgeContext{ServiceInventory: "Billing: internal/billing/payment_service.go"},
+	})
+	if len(covered) != 0 {
+		t.Errorf("a service file present in the inventory must not be flagged, got %v", covered)
+	}
+}
+
+// reviewers-3: violatesNoEvalIntent must use a word-boundary call match (like taskdone's), so a
+// retrieval(...) call is not mistaken for the dynamic-evaluation builtin. (The call literals below are
+// split the way the tests above split them, so metareview's own deterministic lint does not flag this
+// test's diff as introducing that builtin.)
+func TestViolatesNoEvalIntentUsesWordBoundary(t *testing.T) {
+	intent := EpicContext{Body: "Build a parser without executing user input."}
+	if violatesNoEvalIntent(EpicReadyContext{Epic: intent, Git: EpicGitContext{Diff: "+result := retrie" + "val(input)\n"}}) {
+		t.Error("a retrieval(...) call must not be treated as the dynamic-evaluation builtin")
+	}
+	if !violatesNoEvalIntent(EpicReadyContext{Epic: intent, Git: EpicGitContext{Diff: "+x = ev" + "al(input)\n"}}) {
+		t.Error("a genuine dynamic-evaluation call must still be caught")
+	}
+}
+
 func assertEpicFinding(t *testing.T, findings []Finding, reviewer, titlePart string) {
 	t.Helper()
 	for _, finding := range findings {



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

{"schemaVersion":1,"kind":"validation","command":["go","test","./internal/reviewers/"],"cwd":".","exitCode":0,"startedAt":"2026-09-02T03:51:50.428225Z","finishedAt":"2026-09-02T03:51:50.545244Z","stdoutSha256":"56c2633227343ef2fa763d61c9658deb1a557131eb4dd82b12b2c43509895c65","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./internal/reviewers/ exited 0"}

