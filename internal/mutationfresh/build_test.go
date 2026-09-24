package mutationfresh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/mutation"
)

func TestModeFromEnv(t *testing.T) {
	env := func(v string) func(string) string { return func(string) string { return v } }
	for value, want := range map[string]string{"": Advisory, "advisory": Advisory, "enforce": Enforce} {
		if got, err := ModeFromEnv(env(value)); err != nil || got != want {
			t.Errorf("%q: %q %v", value, got, err)
		}
	}
	if _, err := ModeFromEnv(env("strict")); err == nil || !strings.Contains(err.Error(), ModeEnv) {
		t.Errorf("an unknown mode is a usage error naming %s: %v", ModeEnv, err)
	}
	if EffectiveMode("task-done", Enforce) != Advisory || EffectiveMode("pr-ready", Enforce) != Enforce {
		t.Error("task-done is always advisory; the other gates use the mode")
	}
}

func buildAt(t *testing.T, root, mode string, reports ...string) Result {
	t.Helper()
	var loaded []mutation.Report
	for _, p := range reports {
		r, err := mutation.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		loaded = append(loaded, r)
	}
	res, err := Build(loaded, Worktree(root), mode, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestBuildStaleFindingsSumCausesAcrossReportsAndFollowTheMode(t *testing.T) {
	report, root := realCase(t, "full")
	// A second copy of the same report and attestation, elsewhere: same cause, summed counts.
	copyDir := t.TempDir()
	for _, name := range []string{"incremental.json", "attestation.json"} {
		data := readFile(t, filepath.Join(filepath.Dir(report), name))
		write(t, copyDir, name, data)
	}
	write(t, root, "src/a.ts", "changed\n")
	res := buildAt(t, root, Advisory, report, filepath.Join(copyDir, "incremental.json"))
	if len(res.Findings) != 1 {
		t.Fatalf("one finding per distinct cause, got %d: %+v", len(res.Findings), res.Findings)
	}
	f := res.Findings[0]
	if f.Title != "Mutation evidence stale: src/a.ts changed" || !strings.HasPrefix(f.Finding, "34 kill(s) in ") ||
		f.Classification != "advisory" || f.Severity != "medium" || f.Owner != "implementer" || f.Reviewer != "mutation-freshness" {
		t.Errorf("advisory stale finding %+v", f)
	}
	digest := res.Freshness[0].Causes[0].Digest
	if f.Fingerprint != "mutation:stale:advisory:stryker:src/a.ts:"+sha256Hex([]byte("src/a.ts=" + digest))[:8] || f.Evidence[0].Path != "src/a.ts" {
		t.Errorf("fingerprint %q evidence %+v", f.Fingerprint, f.Evidence)
	}
	enforced := buildAt(t, root, Enforce, report)
	if e := enforced.Findings[0]; e.Classification != "blocking" || e.Severity != "high" || !strings.HasPrefix(e.Fingerprint, "mutation:stale:enforce:") {
		t.Errorf("enforce: %+v", e)
	}
}

func TestBuildPendingStaysAdvisoryAndUnattestedFollowsTheMode(t *testing.T) {
	report, root := realCase(t, "lockfile")
	dir := t.TempDir()
	write(t, dir, "incremental.json", `{"files":{"a.ts":{"mutants":[{"id":"1","mutatorName":"M","status":"Killed","location":{"start":{"line":1}}}]}}}`)
	gremlins := t.TempDir()
	write(t, gremlins, "g.json", `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"line":1,"column":1,"type":"CONDITIONALS_NEGATION","status":"KILLED"}]}]}`)
	res := buildAt(t, root, Enforce, report, filepath.Join(dir, "incremental.json"), filepath.Join(gremlins, "g.json"))
	byPrefix := map[string]int{}
	for _, f := range res.Findings {
		byPrefix[strings.Join(strings.SplitN(f.Fingerprint, ":", 3)[:2], ":")]++
		switch {
		case strings.HasPrefix(f.Fingerprint, "mutation:pending:enforce:stryker:"):
			if f.Classification != "advisory" || f.Title != "Mutation evidence pending: 24 kills" || !strings.Contains(f.Finding, "global input changed: package-lock.json") {
				t.Errorf("pending stays advisory under enforce: %+v", f)
			}
		case strings.HasPrefix(f.Fingerprint, "mutation:unattested:enforce:stryker:"):
			if f.Classification != "blocking" || f.Severity != "high" || !strings.Contains(f.Finding, "(missing)") {
				t.Errorf("unattested stryker blocks under enforce: %+v", f)
			}
		}
	}
	if byPrefix["mutation:pending"] != 1 || byPrefix["mutation:unattested"] != 1 {
		t.Errorf("one pending, one unattested (gremlins gets none): %v", byPrefix)
	}
	advisory := buildAt(t, root, Advisory, filepath.Join(dir, "incremental.json"))
	if advisory.Findings[0].Classification != "advisory" || advisory.Findings[0].Severity != "medium" {
		t.Errorf("unattested is advisory by default: %+v", advisory.Findings[0])
	}
}

func TestBuildSection(t *testing.T) {
	report, root := realCase(t, "full")
	dir := t.TempDir()
	write(t, dir, "incremental.json", `{"files":{"a.ts":{"mutants":[{"id":"1","mutatorName":"M","status":"Killed","location":{"start":{"line":1}}}]}}}`)
	gremlins := t.TempDir()
	write(t, gremlins, "g.json", `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"line":1,"column":1,"type":"CONDITIONALS_NEGATION","status":"KILLED"}]}]}`)
	lockReport, _ := realCase(t, "lockfile")
	write(t, root, "tests/c.test.ts", "// changed\n")
	res := buildAt(t, root, Advisory, report, filepath.Join(dir, "incremental.json"), filepath.Join(gremlins, "g.json"), lockReport)
	want := "## Mutation Evidence Freshness\n\n" +
		"- Mode: `advisory`\n" +
		"- Reports: 4 (2 attested, 2 unattested: missing, missing)\n" +
		"- Kills: 21 verified, 6 stale, 21 pending, 0 unbound, 2 unattested\n" +
		"- Deferrals: global input changed: package-lock.json\n" +
		"- Freshness not verifiable: gremlins\n\n" +
		"### Re-run list\n\n" +
		"| File | Cause | Stale kills |\n| --- | --- | ---: |\n" +
		"| `src/c.ts` | `tests/c.test.ts` | 6 |"
	if res.Section != want {
		t.Errorf("section:\n%s\n--- want ---\n%s", res.Section, want)
	}
	if empty, _ := Build(nil, Worktree(root), Advisory, nil); empty.Section != "" || len(empty.Findings) != 0 {
		t.Errorf("no reports, no section: %+v", empty)
	}
}

// Two causes: one finding each, ordered by cause; the re-run list is ordered by file, then cause.
func TestBuildOrdersFindingsAndRows(t *testing.T) {
	report, root := realCase(t, "full")
	write(t, root, "tests/c.test.ts", "// changed\n")
	write(t, root, "src/a.ts", "changed\n")
	res := buildAt(t, root, Advisory, report)
	if len(res.Findings) != 2 || res.Findings[0].Title != "Mutation evidence stale: src/a.ts changed" ||
		res.Findings[1].Title != "Mutation evidence stale: tests/c.test.ts changed" {
		t.Errorf("findings %+v", res.Findings)
	}
	if !strings.Contains(res.Section, "| `src/a.ts` | `src/a.ts` | 12 |\n| `src/b.ts` | `src/a.ts` | 5 |\n| `src/c.ts` | `tests/c.test.ts` | 3 |") {
		t.Errorf("rows:\n%s", res.Section)
	}
}

func TestBuildStopsOnContentErrors(t *testing.T) {
	report, _ := realCase(t, "full")
	r, _ := mutation.Load(report)
	if _, err := Build([]mutation.Report{r}, Worktree(t.TempDir()), Advisory, nil); err == nil {
		t.Error("a content error stops the review")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
