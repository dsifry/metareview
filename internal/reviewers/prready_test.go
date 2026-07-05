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

func TestPRReadyReviewersIgnoreSupersededGitHubReviewBlockersAfterApproval(t *testing.T) {
	findings := RunPRReady(PRReadyContext{
		EvidenceText:       "bash tests/run-all.sh exited 0",
		PREvidenceMarkdown: "## metareview PR Evidence\n\n### Validation\n\n- bash tests/run-all.sh exited 0\n",
		GitHub: PRGitHubContext{
			Available:      true,
			ReviewDecision: "APPROVED",
			Entries: []PRGitHubEntry{{
				Author: "coderabbitai",
				URL:    "https://github.com/acme/repo/pull/7#pullrequestreview-1",
				State:  "CHANGES_REQUESTED",
				Body:   "BLOCKER: earlier finding fixed by later commit",
			}, {
				Author: "coderabbitai",
				URL:    "https://github.com/acme/repo/pull/7#pullrequestreview-2",
				State:  "APPROVED",
				Body:   "",
			}},
		},
	})

	if len(findings) != 0 {
		t.Fatalf("superseded GitHub review blockers should not block after approval: %+v", findings)
	}
}

func TestPRReadyReviewersKeepCommentedGitHubReviewBlockersAfterApproval(t *testing.T) {
	findings := RunPRReady(PRReadyContext{
		EvidenceText:       "bash tests/run-all.sh exited 0",
		PREvidenceMarkdown: "## metareview PR Evidence\n\n### Validation\n\n- bash tests/run-all.sh exited 0\n",
		GitHub: PRGitHubContext{
			Available:      true,
			ReviewDecision: "APPROVED",
			Entries: []PRGitHubEntry{{
				Author: "reviewer",
				URL:    "https://github.com/acme/repo/pull/7#pullrequestreview-3",
				State:  "COMMENTED",
				Body:   "BLOCKER: please resolve this before merging",
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

func TestPRReadyReviewersAcceptStructuredValidationReceipt(t *testing.T) {
	findings := RunPRReady(PRReadyContext{
		EvidenceText:       `{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":0,"summary":"go test ./... exited 0"}`,
		PREvidenceMarkdown: "## metareview PR Evidence\n\n### Summary\n\nParser hardening.\n\n### Validation\n\n- structured validation: go test ./... exited 0 (exit 0)\n",
	})
	if len(findings) != 0 {
		t.Fatalf("structured validation receipt should satisfy PR-ready: %+v", findings)
	}
}

func TestPRReadyReviewersRejectFailedStructuredValidationReceipt(t *testing.T) {
	findings := RunPRReady(PRReadyContext{
		EvidenceText:       `{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":1,"summary":"tests passed before final failure"}`,
		PREvidenceMarkdown: "## metareview PR Evidence\n\n### Summary\n\nParser hardening.\n\n### Validation\n\n- structured validation: go test ./... exited 1 (exit 1)\n",
	})
	assertFinding(t, findings, "validation-reviewer", "high", "Missing validation evidence")
}

func TestPRReadyReviewersRejectMalformedStructuredValidationReceipt(t *testing.T) {
	findings := RunPRReady(PRReadyContext{
		EvidenceText:       `{"schemaVersion":1,"kind":"validation","summary":"missing exitCode defaults to zero"}`,
		PREvidenceMarkdown: "## metareview PR Evidence\n\n### Summary\n\nParser hardening.\n\n### Validation\n\n- malformed validation receipt\n",
	})
	assertFinding(t, findings, "validation-reviewer", "high", "Missing validation evidence")
}

func TestPRReadyReviewersRejectMixedFailedCICheckReceipt(t *testing.T) {
	findings := RunPRReady(PRReadyContext{
		EvidenceText: `{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":0,"summary":"go test ./... exited 0"}` + "\n" +
			`{"schemaVersion":1,"kind":"ci-check","command":["gh","pr","checks","3"],"exitCode":1,"summary":"lint fail"}`,
		PREvidenceMarkdown: "## metareview PR Evidence\n\n### Summary\n\nParser hardening.\n\n### Validation\n\n- structured validation: go test ./... exited 0 (exit 0)\n- structured ci-check: lint fail (exit 1)\n",
	})
	assertFinding(t, findings, "validation-reviewer", "high", "Missing validation evidence")
}
