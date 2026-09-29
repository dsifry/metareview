package findings

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/markdown"
)

// ReviewerTable renders the rows of a gate log's "## Reviewer Results" table: one per reviewer of the gate's
// fixed set, then one for every other reviewer a finding names, in name order. The adversarial-review-reviewer
// (the lens-review requirement) is not in any gate's fixed set, so its findings had no row and the table's
// counts did not add up to the findings listed below it (#143).
func ReviewerTable(fixed []string, records []Record) string {
	reviewers := append([]string(nil), fixed...)
	known := map[string]bool{}
	for _, reviewer := range fixed {
		known[reviewer] = true
	}
	var extra []string
	for _, record := range records {
		if !known[record.Reviewer] {
			known[record.Reviewer] = true
			extra = append(extra, record.Reviewer)
		}
	}
	sort.Strings(extra)
	reviewers = append(reviewers, extra...)
	lines := make([]string, 0, len(reviewers))
	for _, reviewer := range reviewers {
		var blockers, nonBlockers []string
		for _, record := range records {
			if record.Reviewer != reviewer {
				continue
			}
			if CountByClass([]Record{record}).Blocking > 0 {
				blockers = append(blockers, record.Title)
			} else {
				nonBlockers = append(nonBlockers, record.Title)
			}
		}
		verdict := "PASS"
		note := "No blocking findings."
		if len(blockers) > 0 {
			verdict = "NEEDS_REVISION"
			note = strings.Join(blockers, "; ")
		} else if len(nonBlockers) > 0 {
			verdict = "PASS_ADVISORY"
			note = strings.Join(nonBlockers, "; ")
		}
		lines = append(lines, fmt.Sprintf("| %s | %s | %d | %s |", tableCell(reviewer), verdict, len(blockers), tableCell(note)))
	}
	return strings.Join(lines, "\n")
}

// tableCell keeps free text inside one Markdown table cell: one physical line, and no bare "|".
func tableCell(text string) string {
	return strings.ReplaceAll(singleLine(text), "|", `\|`)
}

// ClassifiedMarkdown renders a gate log's four finding sections, each always present ("No findings in this
// class." when empty). A finding raised by another run than runID — a blocker carried in from the ledger —
// says so on its first line, so it is not read as this run's own fresh finding (#143).
func ClassifiedMarkdown(records []Record, runID string) string {
	sections := []struct {
		title string
		label string
	}{
		{title: "## Blocking Findings", label: "blocking"},
		{title: "## Advisory Findings", label: "advisory"},
		{title: "## Follow-up Findings", label: "follow-up"},
		{title: "## Warnings", label: "warning"},
	}
	var output []string
	for _, section := range sections {
		var items []string
		for _, record := range records {
			if displayClass(record) != section.label {
				continue
			}
			carried := ""
			if record.RunID != "" && record.RunID != runID {
				carried = "- Carried forward from: " + markdown.InlineCode(record.RunID) +
					" (raised by an earlier run; the findings ledger holds its current state)\n"
			}
			items = append(items, "### "+record.ID+": "+record.Title+"\n\n"+
				carried+
				"- Reviewer: "+record.Reviewer+"\n"+
				"- Severity: "+record.Severity+"\n"+
				"- Classification: "+record.Classification+"\n"+
				"- Finding: "+record.Finding+"\n"+
				"- Expected: "+record.Expected+"\n"+
				"- Found: "+record.Found+"\n"+
				"- Recommendation: "+record.Recommendation+"\n")
		}
		body := "No findings in this class.\n"
		if len(items) > 0 {
			body = strings.Join(items, "\n")
		}
		output = append(output, section.title+"\n\n"+body)
	}
	return strings.Join(output, "\n\n")
}

func displayClass(record Record) string {
	counts := CountByClass([]Record{record})
	// An if-chain rather than a tagless `switch {}`: Go's coverage tool emits no counter for a
	// tagless-switch case expression, so its guards read as permanently uncovered and mutation testing
	// can never exercise them. As plain `if` conditions they are both covered and mutation-killable.
	if counts.Blocking > 0 {
		return "blocking"
	}
	if counts.Advisory > 0 {
		return "advisory"
	}
	if counts.FollowUp > 0 {
		return "follow-up"
	}
	return "warning"
}
