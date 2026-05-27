package reviewers

import "testing"

func TestPRReadyReviewersBlockUnresolvedReviewLogsAndMissingEvidence(t *testing.T) {
	findings := RunPRReady(PRReadyContext{
		Git: GitContext{ChangedFiles: []string{"internal/parser/parser.go"}},
		ReviewLogs: []PRReviewLog{{
			Target:                "task-1",
			Verdict:               "NEEDS_REVISION",
			HasUnresolvedBlockers: true,
			FindingIDs:            []string{"mrvf-task-1-001"},
		}},
	})

	assertFinding(t, findings, "pr-readiness-reviewer", "high", "Unresolved review blockers")
	assertFinding(t, findings, "validation-reviewer", "high", "Missing validation evidence")
	assertFinding(t, findings, "pr-readiness-reviewer", "high", "Missing PR evidence section")
}

func TestPRReadyReviewersReuseCriticalBranchDiffSignals(t *testing.T) {
	findings := RunPRReady(PRReadyContext{
		Git: GitContext{
			ChangedFiles: []string{"lib/unsafe.js"},
			Diff:         "+module.exports = input => eval(input);\n",
		},
		EvidenceText:       "bash tests/run-all.sh exited 0",
		PREvidenceMarkdown: "## metareview PR Evidence\n\n### Validation\n\n- bash tests/run-all.sh exited 0\n",
	})

	assertFinding(t, findings, "security-reviewer", "critical", "Unsafe eval introduced")
}

func TestPRReadyReviewersIncludeGitHubExternalBlockersWhenAvailable(t *testing.T) {
	findings := RunPRReady(PRReadyContext{
		EvidenceText:       "bash tests/run-all.sh exited 0",
		PREvidenceMarkdown: "## metareview PR Evidence\n\n### Validation\n\n- bash tests/run-all.sh exited 0\n",
		GitHub: PRGitHubContext{
			Available: true,
			Entries: []PRGitHubEntry{{
				Author: "reviewer",
				URL:    "https://github.com/acme/repo/pull/7#pullrequestreview-1",
				State:  "CHANGES_REQUESTED",
				Body:   "BLOCKER: parser accepts unsafe input",
			}},
		},
	})

	assertFinding(t, findings, "external-reviewer", "high", "External GitHub review blocker")
}

func TestPRReadyReviewersDoNotCopyGitHubBodyIntoFindings(t *testing.T) {
	externalBody := "BLOCKER token=redaction-test-value"
	findings := RunPRReady(PRReadyContext{
		EvidenceText:       "bash tests/run-all.sh exited 0",
		PREvidenceMarkdown: "## metareview PR Evidence\n\n### Validation\n\n- bash tests/run-all.sh exited 0\n",
		GitHub: PRGitHubContext{
			Available: true,
			Entries: []PRGitHubEntry{{
				Author: "reviewer",
				URL:    "https://github.com/acme/repo/pull/7#issuecomment-1",
				Body:   externalBody,
			}},
		},
	})

	assertFinding(t, findings, "external-reviewer", "high", "External GitHub review blocker")
	for _, finding := range findings {
		if finding.Found == externalBody ||
			finding.Fingerprint == "pr:github-external-blocker:"+externalBody {
			t.Fatalf("finding copied raw external body: %+v", finding)
		}
	}
}

func TestPRReadyReviewersDoNotRequireGitHubInLocalMode(t *testing.T) {
	findings := RunPRReady(PRReadyContext{
		EvidenceText:       "bash tests/run-all.sh exited 0",
		PREvidenceMarkdown: "## metareview PR Evidence\n\n### Validation\n\n- bash tests/run-all.sh exited 0\n",
		GitHub: PRGitHubContext{
			Available:         false,
			UnavailableReason: "gh-unavailable",
		},
	})
	if len(findings) != 0 {
		t.Fatalf("unavailable GitHub context should not create findings: %+v", findings)
	}
}

func TestPRReadyReviewersAllowCleanBranch(t *testing.T) {
	findings := RunPRReady(PRReadyContext{
		Git: GitContext{
			ChangedFiles: []string{"internal/parser/parser.go", "internal/parser/parser_test.go"},
			Diff:         "+return json.Valid(input)\n",
		},
		EvidenceText:       "bash tests/run-all.sh exited 0",
		PREvidenceMarkdown: "## metareview PR Evidence\n\n### Summary\n\nParser hardening.\n\n### Validation\n\n- bash tests/run-all.sh exited 0\n",
	})
	if len(findings) != 0 {
		t.Fatalf("clean PR-ready context should not produce findings: %+v", findings)
	}
}
