package findings

import (
	"strings"
	"testing"
)

func TestDisplayClass(t *testing.T) {
	cases := []struct {
		name   string
		record Record
		want   string
	}{
		{name: "blocking", record: Record{Classification: "blocking", Severity: "high"}, want: "blocking"},
		{name: "spec-contract blocks regardless of severity", record: Record{Classification: "spec-contract", Severity: "low"}, want: "blocking"},
		{name: "advisory", record: Record{Classification: "advisory", Severity: "low"}, want: "advisory"},
		{name: "follow-up", record: Record{Classification: "follow-up", Severity: "low"}, want: "follow-up"},
		{name: "unknown class is a warning", record: Record{Classification: "novel", Severity: "high"}, want: "warning"},
		{name: "demoted low-severity blocking is a warning", record: Record{Classification: "blocking", Severity: "low"}, want: "warning"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := displayClass(tc.record); got != tc.want {
				t.Fatalf("displayClass = %q, want %q", got, tc.want)
			}
		})
	}
}

// Every reviewer a finding names has a row, so the table's blocking counts add up to the findings below it (#143).
func TestReviewerTableGivesEveryFindingsReviewerARow(t *testing.T) {
	records := []Record{
		{Reviewer: "validation-reviewer", Title: "Missing validation evidence", Classification: "blocking", Severity: "high"},
		{Reviewer: "zeta-reviewer", Title: "An advisory", Classification: "advisory", Severity: "low"},
		{Reviewer: "adversarial-review-reviewer", Title: "No adjudicated lens review recorded", Classification: "blocking", Severity: "high"},
		{Reviewer: "adversarial-review-reviewer", Title: "Second", Classification: "blocking", Severity: "high"},
	}
	got := ReviewerTable([]string{"pr-readiness-reviewer", "validation-reviewer"}, records)
	want := strings.Join([]string{
		"| pr-readiness-reviewer | PASS | 0 | No blocking findings. |",
		"| validation-reviewer | NEEDS_REVISION | 1 | Missing validation evidence |",
		"| adversarial-review-reviewer | NEEDS_REVISION | 2 | No adjudicated lens review recorded; Second |",
		"| zeta-reviewer | PASS_ADVISORY | 0 | An advisory |",
	}, "\n")
	if got != want {
		t.Fatalf("table:\n%s\nwant:\n%s", got, want)
	}
}

// A blocker carried in from the ledger says which run raised it; the run's own findings do not (#143).
func TestClassifiedMarkdownTagsCarriedFindings(t *testing.T) {
	records := []Record{
		{ID: "mrvf-now-001", RunID: "mrv-now", Title: "Fresh", Reviewer: "r", Classification: "blocking", Severity: "high"},
		{ID: "mrvf-old-002", RunID: "mrv-old", Title: "Carried", Reviewer: "r", Classification: "blocking", Severity: "high"},
		{ID: "mrvf-x-003", Title: "No run recorded", Reviewer: "r", Classification: "advisory", Severity: "low"},
	}
	got := ClassifiedMarkdown(records, "mrv-now")
	fresh := got[strings.Index(got, "### mrvf-now-001"):strings.Index(got, "### mrvf-old-002")]
	if strings.Contains(fresh, "Carried forward") {
		t.Errorf("the run's own finding must not be tagged:\n%s", fresh)
	}
	if !strings.Contains(got, "### mrvf-old-002: Carried\n\n- Carried forward from: `mrv-old` (raised by an earlier run; the findings ledger holds its current state)\n- Reviewer: r\n") {
		t.Errorf("a carried finding must name its run first:\n%s", got)
	}
	if strings.Count(got, "Carried forward") != 1 {
		t.Errorf("only the carried finding is tagged:\n%s", got)
	}
	for _, heading := range []string{"## Blocking Findings\n\n", "## Advisory Findings\n\n", "## Follow-up Findings\n\nNo findings in this class.\n", "## Warnings\n\nNo findings in this class.\n"} {
		if !strings.Contains(got, heading) {
			t.Errorf("missing %q:\n%s", heading, got)
		}
	}
}

// Reviewer names and titles are free text: each stays inside its own cell.
func TestReviewerTableKeepsFreeTextInItsCell(t *testing.T) {
	got := ReviewerTable(nil, []Record{{Reviewer: "odd|reviewer", Title: "a | b\\|c d\\\\|e `C:\\dir`\nnext line", Classification: "blocking", Severity: "high"}})
	if want := "| odd\\|reviewer | NEEDS_REVISION | 1 | a \\| b\\|c d\\\\\\|e `C:\\dir` next line |"; got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
