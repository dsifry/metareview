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
	// RequireLenses gates whether a real adjudicated lens review is required to pass (build B fast-follow;
	// default on, off restores the legacy deterministic pass for one migration release). Adversarial is the
	// resolved review-evidence for the epic's integration diff at this head, supplied by the caller. The
	// marker attests the integration diff; roll-up freshness is guarded separately by the deterministic
	// pre-checks below, which re-read current child logs/evidence/intent every run.
	RequireLenses bool
	Adversarial   AdversarialReviewStatus
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
// word plus a source extension, in any common convention: separator-delimited (payment_service.go,
// user.service.ts) or camelCase/PascalCase (AuthService.ts, UserController.tsx); or when a role word is
// a whole path component (services/foo.go). Arms 1 and 3 are case-insensitive; the camelCase arm 2 is
// case-SENSITIVE and its boundary is any alphanumeric (so acronym/digit-prefixed HTTPClient.ts,
// S3Client.ts match) — which, because it requires a *capitalized* role word, still excludes an
// all-lowercase concatenation that is not a word boundary (disservice.go). A role word not at the
// basename end (AuthServiceHelper.go, client_helper.go) and a non-source file (client-guide.md,
// worker.yaml) never match.
var servicePathPattern = regexp.MustCompile(
	`(?:^|[/_.-])(?i:service|controller|worker|client)\.(?i:go|js|ts|tsx|jsx|py|rb)$` +
		`|[A-Za-z0-9](?:Service|Controller|Worker|Client)\.(?i:go|js|ts|tsx|jsx|py|rb)$` +
		`|(?:^|/)(?i:(?:service|controller|worker|client)s?)/`)

// evidencePassPattern matches a status word as a whole word, so "broke"/"bypassed"/"lookup" are not
// mistaken for "ok"/"pass" (callers lowercase the line first). It deliberately EXCLUDES the ambiguous
// "passing"/"okay": a summary like "1 passing 5 failing" or a negated "not passing" would then wrongly
// satisfy the acceptance gate. For an acceptance gate a false positive (a failing child counted as
// passed) is worse than a false negative (a passing child needing clearer evidence), so only
// unambiguous pass tokens count. (The bare "ok"/"passed" tokens have a pre-existing negation gap —
// "not ok" still matches — tracked in the backlog; this is not widened here.)
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
	// Build B fast-follow: require an adjudicated adversarial review over the epic's integration diff at this
	// head. This runs on the normal path only — the context-risk early return above already blocks, so a
	// redundant adversarial block there buys nothing and would be unreachable. The shared reviewer is
	// scope-agnostic; the caller (epicready.Create) resolves the review-evidence marker into context.Adversarial.
	results = append(results, adversarialReviewFindings(context.RequireLenses, context.Adversarial)...)
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
