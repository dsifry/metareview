package mutationfresh

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsifry/metareview/internal/mutation"
)

// realCase copies a committed real report (Plan 1c's e2e run) and the e2e fixture tree into a fresh
// repository. It rebases every attested path outside src/ and tests/ onto that repository, so only
// the fixture's sources and tests can change, and returns the report path and root.
func realCase(t *testing.T, row string) (report, root string) {
	t.Helper()
	files := map[string]string{}
	fixture := "../../testdata/mutation-incremental-e2e"
	err := filepath.WalkDir(fixture, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(fixture, path)
		data, _ := os.ReadFile(path)
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	root = gitRepo(t, files)
	src := filepath.Join("../../testdata/mutation-incremental/real", row)
	reportBytes, err := os.ReadFile(filepath.Join(src, "incremental.json"))
	if err != nil {
		t.Fatal(err)
	}
	var att map[string]any
	attBytes, _ := os.ReadFile(filepath.Join(src, "attestation.json"))
	if err := json.Unmarshal(attBytes, &att); err != nil {
		t.Fatal(err)
	}
	current, _ := Worktree(root).Read(nil)
	_ = current
	entries := att["files"].(map[string]any)
	for path, raw := range entries {
		if hasPrefix(path, "src/") || hasPrefix(path, "tests/") {
			continue
		}
		entry := raw.(map[string]any)
		got, _ := Worktree(root).Read([]string{path})
		if e, ok := got[path]; ok {
			entry["digest"] = digestOf(e)
		} else {
			entry["digest"] = Absent
		}
	}
	att["config"] = ""
	dir := t.TempDir()
	write(t, dir, "incremental.json", string(reportBytes))
	out, _ := json.Marshal(att)
	write(t, dir, "attestation.json", string(out))
	return filepath.Join(dir, "incremental.json"), root
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

func classifyAt(t *testing.T, report, root string) ReportFreshness {
	t.Helper()
	r, err := mutation.Load(report)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Classify(r, Worktree(root), nil)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func causes(f ReportFreshness) map[string]int {
	out := map[string]int{}
	for _, c := range f.Causes {
		out[c.Cause] = c.Kills
	}
	return out
}

// Spec §6.3 on real reports: 24 kills (a.ts 12, b.ts 5, c.ts 3, e.ts 4).
func TestClassifyRealReportsUnchangedTreeIsVerified(t *testing.T) {
	report, root := realCase(t, "full")
	f := classifyAt(t, report, root)
	if !f.Attested || f.Verified != 24 || f.Stale+f.Pending+f.Unbound+f.Unattested != 0 {
		t.Errorf("unchanged tree: %+v", f)
	}
}

func TestClassifyRealReportsEditedSourceStalesItsKillsAndTheirCoverageClosure(t *testing.T) {
	report, root := realCase(t, "full")
	write(t, root, "src/a.ts", "export function grade(score: number, pass: number, top: number): string {\n  return 'ok';\n}\n")
	f := classifyAt(t, report, root)
	// a.ts's own 12 kills, plus b.ts's 5: killed by b.test, whose ids cover a.ts's mutants.
	if f.Stale != 17 || f.Verified != 7 || causes(f)["src/a.ts"] != 17 {
		t.Errorf("edited a.ts: %+v", f)
	}
	if len(f.ReRun) != 2 || f.ReRun[0].File != "src/a.ts" || f.ReRun[1].File != "src/b.ts" || f.ReRun[1].Cause != "src/a.ts" {
		t.Errorf("re-run rows %+v", f.ReRun)
	}
}

func TestClassifyRealReportsEditedTestStalesWhatItKilled(t *testing.T) {
	report, root := realCase(t, "full")
	write(t, root, "tests/c.test.ts", "// rewritten\n")
	f := classifyAt(t, report, root)
	if f.Stale != 3 || causes(f)["tests/c.test.ts"] != 3 || f.Verified != 21 {
		t.Errorf("edited c.test: %+v", f)
	}
}

func TestClassifyRealReportsGlobalAndZeroMutantEditsStaleEverything(t *testing.T) {
	for path, text := range map[string]string{"package-lock.json": "{}\n", "src/limits.ts": "export const LIMIT = 99;\n"} {
		report, root := realCase(t, "full")
		write(t, root, path, text)
		f := classifyAt(t, report, root)
		if f.Stale != 24 || causes(f)[path] != 24 {
			t.Errorf("edited %s: %+v", path, f)
		}
	}
}

func TestClassifyRealReportsDeferralsArePending(t *testing.T) {
	report, root := realCase(t, "lockfile") // "global input changed: package-lock.json" on ["*"]
	f := classifyAt(t, report, root)
	if f.Pending != 24 || f.Deferrals[0] != "global input changed: package-lock.json" {
		t.Errorf("lockfile row: %+v", f)
	}
	report, root = realCase(t, "delete-c-test") // "no reachable tests: src/c.ts"
	f = classifyAt(t, report, root)
	if f.Pending != 3 || f.Verified != 21 {
		t.Errorf("delete-c-test row: %+v", f)
	}
}

// Rules the real reports do not reach: a changed test file ranks before a changed mutate file,
// the byte-smallest cause wins a tie, an unattested file or a kill without killedBy is unbound, a
// new support file present only in the tree is a blanket cause, a new mutate or test file is not,
// and in HEAD mode an untracked attested path absent from HEAD is not a change.
func TestClassifySyntheticRules(t *testing.T) {
	root := gitRepo(t, map[string]string{"src/a.ts": "a", "src/b.ts": "b", "tests/a.test.ts": "t", "tests/b.test.ts": "u"})
	dir := t.TempDir()
	report := `{"files":{
		"src/a.ts":{"source":"a","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1","t2"],"coveredBy":["t1","t2"],"location":{"start":{"line":1},"end":{"line":1}}}]},
		"src/b.ts":{"source":"b","mutants":[{"id":"2","mutatorName":"M","status":"Killed","killedBy":["t2"],"coveredBy":["t2"],"location":{"start":{"line":1},"end":{"line":1}}},
		                                    {"id":"3","mutatorName":"M","status":"Killed","killedBy":[],"coveredBy":[],"location":{"start":{"line":1},"end":{"line":1}}},
		                                    {"id":"4","mutatorName":"M","status":"Survived","coveredBy":["t2"],"location":{"start":{"line":1},"end":{"line":1}}}]},
		"gen/x.ts":{"source":"g","mutants":[{"id":"5","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},
		"testFiles":{"tests/a.test.ts":{"tests":[{"id":"t1"}]},"tests/b.test.ts":{"tests":[{"id":"t2"}]}}}`
	write(t, dir, "incremental.json", report)
	files := map[string]any{}
	digests, _ := Worktree(root).Read([]string{"src/a.ts", "src/b.ts", "tests/a.test.ts", "tests/b.test.ts"})
	for path, category := range map[string]string{"src/a.ts": "mutate", "src/b.ts": "mutate", "tests/a.test.ts": "test", "tests/b.test.ts": "test"} {
		files[path] = map[string]any{"digest": digestOf(digests[path]), "category": category, "tracked": true}
	}
	files["zz-gone.txt"] = map[string]any{"digest": "sha256:0", "category": "unclassified", "tracked": false}
	att := validAttestation(sha256Hex([]byte(report)))
	att["files"] = files
	att["lists"] = map[string]any{"mutate": []string{"src/**"}, "test": []string{"tests/**"}, "support": []string{"helpers/**"}, "global": []string{}, "ignore": []string{"**/*.md"}}
	att["exclusions"] = []string{".mutation/**"}
	writeAttestation(t, dir, att)
	path := filepath.Join(dir, "incremental.json")

	// Worktree: zz-gone.txt is attested but missing, so it is a changed unclassified path, a blanket
	// cause. Stale is checked before unbound, so the kill without killedBy is stale too: all 4.
	f := classifyAt(t, path, root)
	if f.Stale != 4 || causes(f)["zz-gone.txt"] != 4 {
		t.Errorf("missing attested path: %+v", f)
	}
	// HEAD mode: an untracked attested path absent from HEAD is skipped; everything is unchanged.
	r, _ := mutation.Load(path)
	head, err := Classify(r, Head(root), nil)
	if err != nil {
		t.Fatal(err)
	}
	if head.Stale != 0 || head.Verified != 2 || head.Unbound != 2 {
		t.Errorf("HEAD mode: %+v (a.ts, b.ts#2 verified; b.ts#3 has no killedBy and gen/x.ts is unattested: unbound)", head)
	}
	// Both tests changed: a.ts's kill takes the byte-smallest test; b.ts's kill takes b.test.
	write(t, root, "tests/a.test.ts", "t2")
	write(t, root, "tests/b.test.ts", "u2")
	write(t, root, "src/new.ts", "n")        // a new mutate file cannot invalidate a kill
	write(t, root, "tests/new.test.ts", "n") // nor can a new test
	write(t, root, "notes.md", "n")          // ignored
	r, _ = mutation.Load(path)
	f, _ = Classify(r, Head(root), nil)
	if f.Stale != 0 {
		t.Errorf("uncommitted edits are not seen in HEAD mode: %+v", f)
	}
	f = classifyAt(t, path, root)
	if causes(f)["tests/a.test.ts"] != 2 || causes(f)["tests/b.test.ts"] != 1 {
		t.Errorf("test causes: %+v", f.Causes)
	}
	// A changed mutate file (rule 3): b.ts changed only; a.ts's kill is killed by t2, which covers b.ts.
	write(t, root, "tests/a.test.ts", "t")
	write(t, root, "tests/b.test.ts", "u")
	write(t, root, "src/b.ts", "b2")
	write(t, root, "zz-gone.txt", "g") // present now, but not the attested digest: still a blanket cause
	f = classifyAt(t, path, root)
	if causes(f)["src/b.ts"] != 3 || causes(f)["zz-gone.txt"] != 1 {
		// b.ts#2 and b.ts#3 by their own file; a.ts#1 because t2 covers b.ts (rule 3); gen/x.ts#5
		// (killed by t1 only) falls through to the blanket cause zz-gone.txt.
		t.Errorf("rule 3 and own file: %+v", f.Causes)
	}
	// A new support file present only in the tree is a blanket cause; it sorts before zz-gone.txt,
	// so every kill with no other cause records it.
	write(t, root, "src/b.ts", "b")
	write(t, root, "helpers/h.ts", "h")
	f = classifyAt(t, path, root)
	if causes(f)["helpers/h.ts"] != 4 {
		t.Errorf("a new support path is a blanket cause: %+v", f.Causes)
	}
}

func TestClassifyUnattestedAndGremlins(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "incremental.json", `{"files":{"a.ts":{"mutants":[{"id":"1","mutatorName":"M","status":"Killed","location":{"start":{"line":1}}}]}}}`)
	r, _ := mutation.Load(filepath.Join(dir, "incremental.json"))
	f, err := Classify(r, Worktree(t.TempDir()), nil)
	if err != nil || f.Attested || f.UnattestedReason != "missing" || f.Unattested != 1 {
		t.Errorf("unattested stryker: %+v %v", f, err)
	}
	// A parse without Stryker detail (gremlins) is never attested, even beside a harness attestation.
	g := mutation.Report{Engine: "stryker", Target: filepath.Join(dir, "incremental.json"), SHA256: r.SHA256,
		Mutants: []mutation.Mutant{{Status: mutation.Killed}}}
	writeAttestation(t, dir, validAttestation(r.SHA256))
	f, _ = Classify(g, Worktree(t.TempDir()), nil)
	if f.UnattestedReason != "engine" {
		t.Errorf("no detail: %+v", f)
	}
}

func TestClassifyStopsOnContentErrors(t *testing.T) {
	report, _ := realCase(t, "full")
	r, _ := mutation.Load(report)
	if _, err := Classify(r, Worktree(t.TempDir()), nil); err == nil {
		t.Error("listing paths outside a repository stops the review")
	}
	root := gitRepo(t, map[string]string{"src/a.ts": "a"})
	if err := os.Chmod(filepath.Join(root, "src/a.ts"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := Classify(r, Worktree(root), nil); err == nil {
		t.Error("an unreadable attested path stops the review")
	}
}

// Review Focus 2 at the gate: the attested config is digested without its views, so a view-map
// edit is not a change, and any other config edit is a global (blanket) cause.
func TestClassifyConfigDigestIgnoresViews(t *testing.T) {
	cfg := `{"schemaVersion":1,"views":{"inline":{"a":["src/**"]}}}`
	root := gitRepo(t, map[string]string{"src/a.ts": "a", "mi.json": cfg})
	dir := t.TempDir()
	report := `{"files":{"src/a.ts":{"source":"a","mutants":[{"id":"1","mutatorName":"M","status":"Killed","killedBy":["t1"],"coveredBy":["t1"],"location":{"start":{"line":1},"end":{"line":1}}}]}},"testFiles":{}}`
	write(t, dir, "incremental.json", report)
	att := validAttestation(sha256Hex([]byte(report)))
	att["config"] = "mi.json"
	att["files"] = map[string]any{
		"src/a.ts": map[string]any{"digest": digestOf(Entry{Data: []byte("a")}), "category": "mutate", "tracked": true},
		"mi.json":  map[string]any{"digest": configDigest([]byte(cfg)), "category": "global", "tracked": true},
	}
	writeAttestation(t, dir, att)
	path := filepath.Join(dir, "incremental.json")
	write(t, root, "mi.json", `{"views":{"inline":{"b":["src/a.ts"]}},"schemaVersion":1}`)
	if f := classifyAt(t, path, root); f.Verified != 1 || f.Stale != 0 {
		t.Errorf("a view-map edit is not a change: %+v", f)
	}
	write(t, root, "mi.json", `{"schemaVersion":2}`)
	if f := classifyAt(t, path, root); f.Stale != 1 || causes(f)["mi.json"] != 1 {
		t.Errorf("any other config edit is a global cause: %+v", f)
	}
}
