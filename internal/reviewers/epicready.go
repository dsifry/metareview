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

var servicePathPattern = regexp.MustCompile(`(?i)(service|controller|worker|client)\.(go|js|ts|tsx|jsx|py|rb)$|(?i)(service|controller|worker|client)`)

func RunEpicReady(context EpicReadyContext) []Finding {
	var results []Finding
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
			Fingerprint:    "epic:context-risk:" + strings.Join(context.Git.RiskReasons, "|"),
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
		if strings.Contains(line, "pass") ||
			strings.Contains(line, "passed") ||
			strings.Contains(line, "exited 0") ||
			strings.Contains(line, "ok") {
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
	if !(strings.Contains(intent, "without executing") || strings.Contains(intent, "no eval") || strings.Contains(intent, "avoid eval")) {
		return false
	}
	evidence := strings.ToLower(context.Git.Diff)
	for _, child := range context.Children {
		evidence += "\n" + strings.ToLower(child.Body)
	}
	return strings.Contains(evidence, "eval(") || strings.Contains(evidence, "use eval")
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
