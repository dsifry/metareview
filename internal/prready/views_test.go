package prready

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
)

// The §7.1 gate rows (spec §11.3, §11.7) on the real Stryker report of Plan 1c's e2e run, whose view
// map is core = src/**/*.ts (24 kills) and edge = src/c.ts, src/e.ts (3 + 4), at pr-ready (HEAD,
// enforce, where freshness can block).

// An edit inside inc; src/e.ts has no static mutant, so it stales its own 4 kills only.
const editedE = "export function inc(n: number): number {\n  const r = n + 2;\n  return r;\n}\n\nexport function dec(n: number): number {\n  const r = n - 1;\n  return r;\n}\n"

// fixtureRepo commits the e2e fixture tree onto a small pr-ready repository.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	root := smallPRReadyRepo(t)
	fixture := "../../testdata/mutation-incremental-e2e"
	err := filepath.WalkDir(fixture, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(fixture, path)
		data, _ := os.ReadFile(path)
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		return os.WriteFile(full, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	commitAll(t, root, "fixture")
	return root
}

// realReport copies the real "full" report beside an attestation rebased onto root: attested paths
// outside src/ and tests/ take root's content, every other path of the gate repository is ignored,
// and edit may change the attestation further (its views, a src digest).
func realReport(t *testing.T, root string, edit func(att map[string]any)) string {
	t.Helper()
	src := "../../testdata/mutation-incremental/real/full"
	report, err := os.ReadFile(filepath.Join(src, "incremental.json"))
	if err != nil {
		t.Fatal(err)
	}
	var att map[string]any
	data, _ := os.ReadFile(filepath.Join(src, "attestation.json"))
	if err := json.Unmarshal(data, &att); err != nil {
		t.Fatal(err)
	}
	for path, raw := range att["files"].(map[string]any) {
		if strings.HasPrefix(path, "src/") || strings.HasPrefix(path, "tests/") {
			continue
		}
		raw.(map[string]any)["digest"] = digestAt(root, path)
	}
	att["config"] = ""
	lists := att["lists"].(map[string]any)
	lists["ignore"] = append(lists["ignore"].([]any), "**", ".metareview/**")
	if edit != nil {
		edit(att)
	}
	dir := t.TempDir()
	out, _ := json.Marshal(att)
	if err := os.WriteFile(filepath.Join(dir, "incremental.json"), report, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "attestation.json"), out, 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "incremental.json")
}

func digestAt(root, path string) string {
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		return "absent"
	}
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:])
}

func rowsByView(t *testing.T, root, prefix string) map[string]string {
	t.Helper()
	records, err := findings.All(root)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, r := range records {
		if strings.HasPrefix(r.Fingerprint, prefix) {
			out[r.View] = r.Status
			if r.Classification != "blocking" {
				out[r.View] += " (" + r.Classification + ")"
			}
		}
	}
	return out
}

func recordID(t *testing.T, root, prefix string) string {
	t.Helper()
	records, err := findings.All(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		if strings.HasPrefix(r.Fingerprint, prefix) {
			return r.ID
		}
	}
	t.Fatalf("no row %s", prefix)
	return ""
}

func review(t *testing.T, root, report string, views ...string) (Result, string) {
	t.Helper()
	res, err := Create(root, Options{Base: "main", MutationReportPaths: []string{report}, MutationViews: views})
	if err != nil {
		t.Fatal(err)
	}
	log, _ := os.ReadFile(filepath.Join(root, res.ReviewRel))
	return res, string(log)
}

// Spec §11.3: a run with views owns only its views' freshness rows, so another view's open stale row
// left by an earlier run is not an unresolved review blocker for it; a run without views owns them all.
func TestAnotherViewsRowIsNotAnUnresolvedBlocker(t *testing.T) {
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	root := smallPRReadyRepo(t)
	edge := findings.Record{ID: "mrvf-earlier-001", RunID: "mrv-earlier", Scope: "pr-ready", Status: "open", Severity: "high",
		Classification: "blocking", Title: "Mutation evidence stale (edge): src/e.ts changed", View: "edge",
		Fingerprint: "mutation:stale:enforce:stryker:edge:src/e.ts:0123abcd", Target: map[string]any{"type": "branch", "id": "feature"}}
	saved := unresolvedBlocking
	t.Cleanup(func() { unresolvedBlocking = saved })
	unresolvedBlocking = func(string) ([]findings.Record, error) { return []findings.Record{edge}, nil }
	for _, c := range []struct {
		views []string
		want  bool
	}{{[]string{"core"}, false}, {nil, true}} {
		res, err := Create(root, Options{Base: "main", MutationViews: c.views})
		if err != nil {
			t.Fatal(err)
		}
		log, _ := os.ReadFile(filepath.Join(root, res.ReviewRel))
		if got := strings.Contains(string(log), "Unresolved review blockers"); got != c.want {
			t.Errorf("views %v: unresolved review blockers %v, want %v", c.views, got, c.want)
		}
	}
}

func TestGateRowsWithViews(t *testing.T) {
	t.Setenv("METAREVIEW_ALLOW_MECHANICAL_PASS", "1")
	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "enforce")
	root := fixtureRepo(t)
	report := realReport(t, root, nil)

	// An uncommitted edit is not under review in HEAD mode: both views verified.
	if err := os.WriteFile(filepath.Join(root, "src/e.ts"), []byte(editedE), 0o644); err != nil {
		t.Fatal(err)
	}
	if res, log := review(t, root, report, "core", "edge"); !strings.Contains(log, "| `core` | 24 | 0 |") || !strings.Contains(log, "| `edge` | 7 | 0 |") {
		t.Fatalf("uncommitted: %s\n%s", res.Verdict, log)
	}

	// Repeated flags, one section: a row per view, a finding per view, and enforce blocks.
	commitAll(t, root, "edit e")
	res, log := review(t, root, report, "core", "edge")
	if strings.Count(log, "## Mutation Evidence Freshness") != 1 ||
		!strings.Contains(log, "| `core` | 20 | 4 |") || !strings.Contains(log, "| `edge` | 3 | 4 |") ||
		!strings.Contains(log, "Mutation evidence stale (core): src/e.ts changed") || !strings.Contains(log, "Mutation evidence stale (edge): src/e.ts changed") {
		t.Fatalf("committed: %s\n%s", res.Verdict, log)
	}
	if got := rowsByView(t, root, "mutation:stale:enforce:stryker:"); got["core"] != "open" || got["edge"] != "open" || len(got) != 2 {
		t.Fatalf("per-view rows: %v", got)
	}

	// Cross-view isolation: fresh evidence for core alone supersedes only core's row, and edge's
	// open row does not block a run that does not ask for edge.
	fresh := func(att map[string]any) {
		att["files"].(map[string]any)["src/e.ts"].(map[string]any)["digest"] = digestAt(root, "src/e.ts")
	}
	_, log = review(t, root, realReport(t, root, fresh), "core")
	if got := rowsByView(t, root, "mutation:stale:"); got["core"] != findings.StatusSuperseded || got["edge"] != "open" {
		t.Fatalf("isolation: %v", got)
	}
	blocking := strings.Split(strings.Split(log, "## Blocking Findings")[1], "## Advisory Findings")[0]
	if id := recordID(t, root, "mutation:stale:enforce:stryker:edge:"); strings.Contains(blocking, id) || strings.Contains(blocking, "(edge)") {
		t.Fatalf("edge's row (%s) blocks a run that did not ask for edge:\n%s", id, blocking)
	}

	// A report attested without views supersedes nothing by the rename sweep.
	noViews := func(att map[string]any) { fresh(att); delete(att, "views") }
	review(t, root, realReport(t, root, noViews), "core")
	if got := rowsByView(t, root, "mutation:stale:"); got["edge"] != "open" {
		t.Fatalf("no view map: %v", got)
	}

	// A renamed view (edge → periphery) supersedes the old view's rows.
	renamed := func(att map[string]any) {
		fresh(att)
		views := att["views"].(map[string]any)
		views["periphery"] = views["edge"]
		delete(views, "edge")
	}
	review(t, root, realReport(t, root, renamed), "core")
	if got := rowsByView(t, root, "mutation:stale:"); got["edge"] != findings.StatusSuperseded {
		t.Fatalf("renamed view: %v", got)
	}

	// An unattested report under a view keeps its blocker, fingerprinted with the view.
	unattested := realReport(t, root, nil)
	if err := os.Remove(filepath.Join(filepath.Dir(unattested), "attestation.json")); err != nil {
		t.Fatal(err)
	}
	res, log = review(t, root, unattested, "core")
	if got := rowsByView(t, root, "mutation:unattested:enforce:stryker:core:"); got["core"] != "open" ||
		!strings.Contains(log, "Mutation report has no attestation (core)") {
		t.Fatalf("unattested: %v %s\n%s", got, res.Verdict, log)
	}
}
