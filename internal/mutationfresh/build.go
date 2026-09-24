package mutationfresh

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/mutation"
)

const reviewer = "mutation-freshness"

// Result is what a gate needs from the freshness check: the classifications (for pr-ready's
// digest), the findings, and the review-log section.
type Result struct {
	Mode      string
	Freshness []ReportFreshness
	Findings  []findings.Input
	Section   string
}

// Build classifies every report, then derives the findings (spec §6.5) and the section (§6.6),
// once per requested view (§11.3) or once for the whole report when none is requested.
func Build(reports []mutation.Report, content Content, mode string, views []string) (Result, error) {
	res := Result{Mode: mode}
	for _, r := range reports {
		f, err := Classify(r, content, views)
		if err != nil {
			return Result{}, err
		}
		res.Freshness = append(res.Freshness, f)
	}
	slices := []string{""}
	if len(views) > 0 {
		slices = views
	}
	for _, v := range slices {
		res.Findings = append(res.Findings, freshnessFindings(res.Freshness, mode, v)...)
	}
	res.Section = section(res.Freshness, mode, views)
	return res, nil
}

// tallyOf is the report's own tally for view "", else that view's.
func tallyOf(f ReportFreshness, view string) Tally {
	for _, v := range f.Views {
		if v.View == view {
			return v.Tally
		}
	}
	return f.Tally
}

// viewParts are the fingerprint segment (":<view>") and the title qualifier (" (<view>)").
func viewParts(view string) (fingerprint, title string) {
	if view == "" {
		return "", ""
	}
	return ":" + view, " (" + view + ")"
}

func gate(mode string) (classification, severity string) {
	if mode == Enforce {
		return "blocking", "high" // classForCount counts blocking only at high or critical
	}
	return "advisory", "medium"
}

func freshnessFindings(list []ReportFreshness, mode, view string) []findings.Input {
	type staleKey struct{ engine, cause string }
	type staleSum struct {
		digest  string
		kills   int
		reports []string
	}
	fpView, titleView := viewParts(view)
	sums := map[staleKey]*staleSum{}
	for _, f := range list {
		for _, c := range tallyOf(f, view).Causes {
			key := staleKey{f.Engine, c.Cause}
			if sums[key] == nil {
				sums[key] = &staleSum{digest: c.Digest}
			}
			sums[key].kills += c.Kills
			sums[key].reports = append(sums[key].reports, f.Path)
		}
	}
	keys := make([]staleKey, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].engine+"\x00"+keys[i].cause < keys[j].engine+"\x00"+keys[j].cause
	})
	classification, severity := gate(mode)
	var out []findings.Input
	for _, k := range keys {
		s := sums[k]
		out = append(out, findings.Input{
			Reviewer: reviewer, Severity: severity, Classification: classification, Owner: "implementer",
			Title:          "Mutation evidence stale" + titleView + ": " + k.cause + " changed",
			Finding:        fmt.Sprintf("%d kill(s) in %s were verified against a different %s, so they are not evidence for the code under review.", s.kills, strings.Join(s.reports, ", "), k.cause),
			Expected:       "Every kill the gate counts was verified against the content under review.",
			Found:          fmt.Sprintf("%s is now %s.", k.cause, s.digest),
			Recommendation: "Re-run the harness (node tools/mutation-incremental/cli.mjs run --mode incremental), commit before running pr-ready, and pass the refreshed <stateDir>/incremental.json.",
			Evidence:       []findings.Evidence{{Type: "mutant", Path: k.cause}},
			Fingerprint:    fmt.Sprintf("mutation:stale:%s:%s%s:%s:%s", mode, k.engine, fpView, k.cause, sha256Hex([]byte(k.cause + "=" + s.digest))[:8]),
			View:           view,
		})
	}
	for _, f := range list {
		t := tallyOf(f, view)
		if t.Pending > 0 {
			out = append(out, findings.Input{
				Reviewer: reviewer, Severity: "medium", Classification: "advisory", Owner: "reviewer",
				Title:          fmt.Sprintf("Mutation evidence pending%s: %d kills", titleView, t.Pending),
				Finding:        fmt.Sprintf("%d kill(s) in %s are covered by a deferral (%s) and wait for a full run, so they are not counted as evidence.", t.Pending, f.Path, strings.Join(f.Deferrals, "; ")),
				Expected:       "Deferred work is re-verified by a full run before it counts.",
				Recommendation: "Let main's full run clear the deferrals, or run the harness with --mode full locally.",
				Evidence:       []findings.Evidence{{Type: "mutant", Path: f.Path}},
				Fingerprint:    fmt.Sprintf("mutation:pending:%s:%s%s:%s", mode, f.Engine, fpView, f.ReportSha256[:8]),
				View:           view,
			})
		}
		if !f.Attested && f.Engine == "stryker" {
			out = append(out, findings.Input{
				Reviewer: reviewer, Severity: severity, Classification: classification, Owner: "reviewer",
				Title:          "Mutation report has no attestation" + titleView,
				Finding:        fmt.Sprintf("%s has no valid attestation (%s), so the gate cannot tell whether its %d kill(s) still describe the code under review.", f.Path, f.UnattestedReason, t.Unattested),
				Expected:       "Mutation evidence comes from the mutation-incremental harness, which attests what it verified.",
				Recommendation: "Produce the report with the harness and pass <stateDir>/incremental.json (its attestation.json sits beside it).",
				Evidence:       []findings.Evidence{{Type: "mutant", Path: f.Path}},
				Fingerprint:    fmt.Sprintf("mutation:unattested:%s:%s%s:%s", mode, f.Engine, fpView, f.ReportSha256[:8]),
				View:           view,
			})
		}
	}
	return out
}

// section renders "## Mutation Evidence Freshness" (spec §6.6), or "" without reports. With views
// (§11.3) it adds a table row per view and a View column to the re-run list.
func section(list []ReportFreshness, mode string, views []string) string {
	if len(list) == 0 {
		return ""
	}
	var attested int
	var total Tally
	var reasons, deferrals, unverifiable []string
	seenDeferral, seenEngine := map[string]bool{}, map[string]bool{}
	perView := make([]Tally, len(views))
	rows := map[[3]string]int{}
	for _, f := range list {
		total.Verified += f.Verified
		total.Stale += f.Stale
		total.Pending += f.Pending
		total.Unbound += f.Unbound
		total.Unattested += f.Unattested
		if f.Attested {
			attested++
		} else {
			reasons = append(reasons, f.UnattestedReason)
			if f.Engine != "stryker" && !seenEngine[f.Engine] {
				seenEngine[f.Engine] = true
				unverifiable = append(unverifiable, f.Engine)
			}
		}
		for _, d := range f.Deferrals {
			if !seenDeferral[d] {
				seenDeferral[d] = true
				deferrals = append(deferrals, d)
			}
		}
		if len(views) == 0 {
			for _, row := range f.ReRun {
				rows[[3]string{"", row.File, row.Cause}] += row.Kills
			}
		}
		for i, v := range views {
			t := tallyOf(f, v)
			perView[i].Verified += t.Verified
			perView[i].Stale += t.Stale
			perView[i].Pending += t.Pending
			perView[i].PendingInherited += t.PendingInherited
			perView[i].Unbound += t.Unbound
			perView[i].Unattested += t.Unattested
			for _, row := range t.ReRun {
				rows[[3]string{v, row.File, row.Cause}] += row.Kills
			}
		}
	}
	var b strings.Builder
	b.WriteString("## Mutation Evidence Freshness\n\n")
	fmt.Fprintf(&b, "- Mode: `%s`\n", mode)
	fmt.Fprintf(&b, "- Reports: %d (%d attested", len(list), attested)
	if len(reasons) > 0 {
		fmt.Fprintf(&b, ", %d unattested: %s", len(reasons), strings.Join(reasons, ", "))
	}
	b.WriteString(")\n")
	fmt.Fprintf(&b, "- Kills: %d verified, %d stale, %d pending, %d unbound, %d unattested\n", total.Verified, total.Stale, total.Pending, total.Unbound, total.Unattested)
	if len(deferrals) > 0 {
		fmt.Fprintf(&b, "- Deferrals: %s\n", strings.Join(deferrals, "; "))
	}
	if len(unverifiable) > 0 {
		sort.Strings(unverifiable)
		fmt.Fprintf(&b, "- Freshness not verifiable: %s\n", strings.Join(unverifiable, ", "))
	}
	if len(views) > 0 {
		b.WriteString("\n### Views\n\n| View | Verified | Stale | Pending counted | Pending inherited | Unbound | Unattested |\n| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for i, v := range views {
			t := perView[i]
			fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %d | %d |\n", v, t.Verified, t.Stale, t.Pending-t.PendingInherited, t.PendingInherited, t.Unbound, t.Unattested)
		}
	}
	if len(rows) > 0 {
		keys := make([][3]string, 0, len(rows))
		for k := range rows {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return strings.Join(keys[i][:], "\x00") < strings.Join(keys[j][:], "\x00")
		})
		if len(views) == 0 {
			b.WriteString("\n### Re-run list\n\n| File | Cause | Stale kills |\n| --- | --- | ---: |\n")
		} else {
			b.WriteString("\n### Re-run list\n\n| View | File | Cause | Stale kills |\n| --- | --- | --- | ---: |\n")
		}
		for _, k := range keys {
			if len(views) == 0 {
				fmt.Fprintf(&b, "| `%s` | `%s` | %d |\n", k[1], k[2], rows[k])
			} else {
				fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | %d |\n", k[0], k[1], k[2], rows[k])
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
