package mutationfresh

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/mutation"
)

// The e2e fixture's views: core = src/**/*.ts (all 24 kills), edge = src/c.ts, src/e.ts (3 + 4).
var bothViews = []string{"core", "edge"}

const editedE = "export function inc(n: number): number {\n  const r = n + 2;\n  return r;\n}\n\nexport function dec(n: number): number {\n  const r = n - 1;\n  return r;\n}\n"

func classifyViews(t *testing.T, report, root string, views []string) ReportFreshness {
	t.Helper()
	r, err := mutation.Load(report)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Classify(r, Worktree(root), views)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func editAttestation(t *testing.T, report string, edit func(map[string]any)) {
	t.Helper()
	path := filepath.Join(filepath.Dir(report), "attestation.json")
	data, _ := os.ReadFile(path)
	var att map[string]any
	if err := json.Unmarshal(data, &att); err != nil {
		t.Fatal(err)
	}
	edit(att)
	out, _ := json.Marshal(att)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func view(f ReportFreshness, name string) ViewFreshness {
	for _, v := range f.Views {
		if v.View == name {
			return v
		}
	}
	return ViewFreshness{}
}

// Spec §11.3: each view has its own classification; views may overlap and are never summed, and
// the report's own totals are unchanged.
func TestViewsOverlapAndAreNotSummed(t *testing.T) {
	report, root := realCase(t, "full")
	f := classifyViews(t, report, root, bothViews)
	if f.Verified != 24 || !view(f, "core").Scoped || view(f, "core").Verified != 24 || view(f, "edge").Verified != 7 {
		t.Fatalf("unchanged: %+v", f)
	}
	if strings.Join(f.ViewNames, ",") != "core,edge" {
		t.Errorf("view names %v", f.ViewNames)
	}
	write(t, root, "src/e.ts", editedE)
	f = classifyViews(t, report, root, bothViews)
	core, edge := view(f, "core"), view(f, "edge")
	if f.Stale != 4 || core.Stale != 4 || core.Verified != 20 || edge.Stale != 4 || edge.Verified != 3 {
		t.Errorf("edited e.ts: report %+v core %+v edge %+v", f.Tally, core, edge)
	}
	if len(edge.Causes) != 1 || edge.Causes[0].Cause != "src/e.ts" || len(edge.ReRun) != 1 {
		t.Errorf("edge causes %+v rows %+v", edge.Causes, edge.ReRun)
	}
	if none := classifyViews(t, report, root, nil); none.Views != nil {
		t.Errorf("no views requested: %+v", none.Views)
	}
}

// An attested report without views (attested before they were adopted) is not scoped.
func TestViewOnReportWithoutViewsIsUnscoped(t *testing.T) {
	report, root := realCase(t, "full")
	editAttestation(t, report, func(att map[string]any) { delete(att, "views") })
	f := classifyViews(t, report, root, []string{"edge"})
	if v := view(f, "edge"); v.Scoped || v.Verified != 24 || f.ViewNames != nil {
		t.Errorf("unscoped: %+v", f)
	}
}

func TestUnattestedReportUnderAView(t *testing.T) {
	report, root := realCase(t, "full")
	if err := os.Remove(filepath.Join(filepath.Dir(report), "attestation.json")); err != nil {
		t.Fatal(err)
	}
	f := classifyViews(t, report, root, []string{"core"})
	if v := view(f, "core"); v.Unattested != 24 || v.Scoped {
		t.Errorf("unattested: %+v", f)
	}
}

// Spec §11.2/§11.3: pending kills split by the deferral's inherited flag, counted winning a tie.
func TestPendingSplitsCountedAndInherited(t *testing.T) {
	for _, c := range []struct {
		name      string
		deferrals []any
		inherited int
	}{
		{"inherited only", []any{map[string]any{"reason": "timeout", "paths": []string{"src/c.ts"}, "inherited": true}}, 3},
		{"counted wins", []any{
			map[string]any{"reason": "timeout", "paths": []string{"src/c.ts"}, "inherited": true},
			map[string]any{"reason": "global", "paths": []string{"src/c.ts"}, "inherited": false},
		}, 0},
	} {
		report, root := realCase(t, "full")
		editAttestation(t, report, func(att map[string]any) { att["deferrals"] = c.deferrals })
		f := classifyViews(t, report, root, []string{"edge"})
		if f.Pending != 3 || f.PendingInherited != c.inherited || view(f, "edge").PendingInherited != c.inherited {
			t.Errorf("%s: %+v", c.name, f)
		}
	}
}

func TestCheckViews(t *testing.T) {
	report, _ := realCase(t, "full")
	r, err := mutation.Load(report)
	if err != nil {
		t.Fatal(err)
	}
	g := mutation.Report{Engine: "gremlins", Target: "g.json"}
	for _, c := range []struct {
		views []string
		want  string
	}{
		{nil, ""},
		{[]string{"core", "edge"}, ""},
		{[]string{"nope"}, `mutation view "nope" is not in the views of ` + report + " (core, edge)"},
		{[]string{"bad name!"}, `invalid mutation view name "bad name!"`},
		{[]string{""}, `invalid mutation view name ""`},
	} {
		err := CheckViews([]mutation.Report{g, r}, c.views)
		if got := errText(err); got != c.want {
			t.Errorf("%v: %q, want %q", c.views, got, c.want)
		}
	}
	editAttestation(t, report, func(att map[string]any) { delete(att, "views") })
	if err := CheckViews([]mutation.Report{r}, []string{"anything"}); err != nil {
		t.Errorf("a report without views is not scoped: %v", err)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(report), "attestation.json")); err != nil {
		t.Fatal(err)
	}
	if err := CheckViews([]mutation.Report{r}, []string{"anything"}); err != nil {
		t.Errorf("an unattested report is not scoped: %v", err)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func buildViews(t *testing.T, report, root, mode string, views []string) Result {
	t.Helper()
	r, err := mutation.Load(report)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Build([]mutation.Report{r}, Worktree(root), mode, views)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// Spec §11.3: one finding per view with the view in its fingerprint and title, and one section with
// a table row per view and a re-run list with a View column.
func TestBuildWithViews(t *testing.T) {
	report, root := realCase(t, "full")
	write(t, root, "src/e.ts", editedE)
	res := buildViews(t, report, root, Enforce, bothViews)
	if len(res.Findings) != 2 {
		t.Fatalf("findings: %+v", res.Findings)
	}
	for i, name := range bothViews {
		f := res.Findings[i]
		if f.View != name || f.Title != "Mutation evidence stale ("+name+"): src/e.ts changed" ||
			!strings.HasPrefix(f.Fingerprint, "mutation:stale:enforce:stryker:"+name+":src/e.ts:") || f.Classification != "blocking" {
			t.Errorf("finding %d: %+v", i, f)
		}
	}
	want := "## Mutation Evidence Freshness\n\n" +
		"- Mode: `enforce`\n" +
		"- Reports: 1 (1 attested)\n" +
		"- Kills: 20 verified, 4 stale, 0 pending, 0 unbound, 0 unattested\n" +
		"\n### Views\n\n" +
		"| View | Verified | Stale | Pending counted | Pending inherited | Unbound | Unattested |\n" +
		"| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n" +
		"| `core` | 20 | 4 | 0 | 0 | 0 | 0 |\n" +
		"| `edge` | 3 | 4 | 0 | 0 | 0 | 0 |\n" +
		"\n### Re-run list\n\n" +
		"| View | File | Cause | Stale kills |\n| --- | --- | --- | ---: |\n" +
		"| `core` | `src/e.ts` | `src/e.ts` | 4 |\n" +
		"| `edge` | `src/e.ts` | `src/e.ts` | 4 |"
	if res.Section != want {
		t.Errorf("section:\n%s\nwant:\n%s", res.Section, want)
	}
}

func TestBuildPendingAndUnattestedWithViews(t *testing.T) {
	report, root := realCase(t, "lockfile") // every kill pending
	res := buildViews(t, report, root, Advisory, []string{"edge"})
	if len(res.Findings) != 1 || res.Findings[0].Title != "Mutation evidence pending (edge): 7 kills" ||
		!strings.HasPrefix(res.Findings[0].Fingerprint, "mutation:pending:advisory:stryker:edge:") {
		t.Errorf("pending: %+v", res.Findings)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(report), "attestation.json")); err != nil {
		t.Fatal(err)
	}
	res = buildViews(t, report, root, Enforce, []string{"edge"})
	if len(res.Findings) != 1 || res.Findings[0].Title != "Mutation report has no attestation (edge)" || res.Findings[0].View != "edge" ||
		!strings.HasPrefix(res.Findings[0].Fingerprint, "mutation:unattested:enforce:stryker:edge:") {
		t.Errorf("unattested: %+v", res.Findings)
	}
}
