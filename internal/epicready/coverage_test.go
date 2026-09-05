package epicready

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/contextprofile"
	"github.com/dsifry/metareview/internal/epicsource"
	"github.com/dsifry/metareview/internal/findings"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/reviewlog"
	"github.com/dsifry/metareview/internal/runchain"
	"github.com/dsifry/metareview/internal/tasksource"
)

// epicRepo builds a git repo (config isolated) with a Beads epic + one child, a feature branch
// change, so Create resolves an epic with children over a real base..head diff, under the gate.
func epicRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=gc.auto", "GIT_CONFIG_VALUE_0=0", "GIT_CONFIG_KEY_1=maintenance.auto", "GIT_CONFIG_VALUE_1=false")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	write(".beads/issues.jsonl",
		`{"id":"epic-1","title":"Epic One","body":"the epic body"}`+"\n"+
			`{"id":"child-1","title":"Child One","body":"child body","parent":"epic-1"}`+"\n")
	write("seed.txt", "seed\n")
	run("add", ".")
	run("commit", "-m", "initial")
	run("checkout", "-q", "-b", "feature")
	write("src/a.go", "package src\n\nvar A = 1\n")
	run("add", "-A")
	run("commit", "-m", "feature change")
	return root
}

// --- Create end to end + error branches (deterministic seams) ---

func TestCreateBlockingAndGate(t *testing.T) {
	root := epicRepo(t)
	res, err := Create(root, "epic-1", Options{Base: "main"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// No lens marker: the require-lenses gate blocks; the context pack records the Beads gate effect.
	if !res.Blocking {
		t.Errorf("expected a blocking epic-ready without a marker")
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(res.ContextRel)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Gate effect: `gate`") {
		t.Errorf("Beads repo should have gate effect 'gate':\n%s", body)
	}
	if !strings.Contains(string(body), "child body") {
		t.Errorf("context pack should include the child:\n%s", body)
	}
}

func TestCreateEpicResolveError(t *testing.T) {
	// A path-like target that does not exist fails to resolve.
	if _, err := Create(epicRepo(t), "docs/epics/nope.md", Options{Base: "main"}); err == nil {
		t.Fatal("expected an error for an unresolvable epic target")
	}
}

func TestCreateGitContextError(t *testing.T) {
	if _, err := Create(epicRepo(t), "epic-1", Options{Base: "no-such-base"}); err == nil {
		t.Fatal("expected an error for an invalid base ref")
	}
}

func TestCreateEvidenceError(t *testing.T) {
	root := epicRepo(t)
	if _, err := Create(root, "epic-1", Options{Base: "main", EvidencePath: filepath.Join(root, "nope.md")}); err == nil {
		t.Fatal("expected an error for a missing evidence file")
	}
}

func TestCreateMutationReportError(t *testing.T) {
	root := epicRepo(t)
	if _, err := Create(root, "epic-1", Options{Base: "main", MutationReportPaths: []string{filepath.Join(root, "missing.json")}}); err == nil {
		t.Fatal("expected an error for an unreadable mutation report")
	}
}

func TestCreateCollaboratorErrors(t *testing.T) {
	sentinel := errors.New("boom")
	cases := []struct {
		name  string
		patch func(t *testing.T)
	}{
		{"knowledge", func(t *testing.T) {
			orig := collectKnowledge
			t.Cleanup(func() { collectKnowledge = orig })
			collectKnowledge = func(string) (knowledge.Context, error) { return knowledge.Context{}, sentinel }
		}},
		{"discoverLogs", func(t *testing.T) {
			orig := discoverLogs
			t.Cleanup(func() { discoverLogs = orig })
			discoverLogs = func(string) ([]reviewlog.Summary, error) { return nil, sentinel }
		}},
		{"unresolvedBlocking", func(t *testing.T) {
			orig := unresolvedBlocking
			t.Cleanup(func() { unresolvedBlocking = orig })
			unresolvedBlocking = func(string) ([]findings.Record, error) { return nil, sentinel }
		}},
		{"resolveChain", func(t *testing.T) {
			orig := resolveChain
			t.Cleanup(func() { resolveChain = orig })
			resolveChain = func(string, runchain.Options) (runchain.Decision, error) { return runchain.Decision{}, sentinel }
		}},
		{"reconcile", func(t *testing.T) {
			orig := reconcileFindings
			t.Cleanup(func() { reconcileFindings = orig })
			reconcileFindings = func(string, findings.Run, []findings.Input, findings.Options) (findings.Result, error) {
				return findings.Result{}, sentinel
			}
		}},
		{"appendJSONL", func(t *testing.T) {
			orig := appendJSONL
			t.Cleanup(func() { appendJSONL = orig })
			appendJSONL = func(string, any) error { return sentinel }
		}},
		{"mkdirContext", func(t *testing.T) {
			orig := mkdirAll
			t.Cleanup(func() { mkdirAll = orig })
			mkdirAll = func(p string, m os.FileMode) error {
				if strings.Contains(p, filepath.Join("metareview", "context")) {
					return sentinel
				}
				return orig(p, m)
			}
		}},
		{"mkdirReviews", func(t *testing.T) {
			orig := mkdirAll
			t.Cleanup(func() { mkdirAll = orig })
			mkdirAll = func(p string, m os.FileMode) error {
				if strings.Contains(p, filepath.Join("metareview", "reviews")) {
					return sentinel
				}
				return orig(p, m)
			}
		}},
		{"writeContext", func(t *testing.T) {
			orig := writeFile
			t.Cleanup(func() { writeFile = orig })
			writeFile = func(p string, d []byte, m os.FileMode) error {
				if strings.Contains(p, filepath.Join("metareview", "context")) {
					return sentinel
				}
				return orig(p, d, m)
			}
		}},
		{"writeReview", func(t *testing.T) {
			orig := writeFile
			t.Cleanup(func() { writeFile = orig })
			writeFile = func(p string, d []byte, m os.FileMode) error {
				if strings.Contains(p, filepath.Join("metareview", "reviews")) {
					return sentinel
				}
				return orig(p, d, m)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := epicRepo(t)
			c.patch(t)
			_, err := Create(root, "epic-1", Options{Base: "main"})
			if !errors.Is(err, sentinel) {
				t.Fatalf("%s: expected the injected error, got %v", c.name, err)
			}
		})
	}
}

// A metareview-path epic target takes the exceptions branch (no generated-context filtering).
func TestCreateMetareviewTargetException(t *testing.T) {
	root := epicRepo(t)
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=gc.auto", "GIT_CONFIG_VALUE_0=0", "GIT_CONFIG_KEY_1=maintenance.auto", "GIT_CONFIG_VALUE_1=false")
	p := filepath.Join(root, "docs", "metareview", "epic.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("# epic\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "add metareview epic"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if _, err := Create(root, "docs/metareview/epic.md", Options{Base: "main"}); err != nil {
		t.Fatalf("Create with a metareview target: %v", err)
	}
}

// A second run naming the first as previous exercises the run-chain link append.
func TestCreatePreviousRunChain(t *testing.T) {
	root := epicRepo(t)
	at := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	first, err := Create(root, "epic-1", Options{Base: "main", Now: at})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := Create(root, "epic-1", Options{Base: "main", Now: at.Add(time.Hour), PreviousRunID: first.RunID, MaxAttempts: 3}); err != nil {
		t.Fatalf("second Create: %v", err)
	}
}

// --- pure helpers ---

func TestUniquePathsBumpsOnCollision(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	id1, _, rel1 := uniquePaths(root, "epic-1", at)
	p := filepath.Join(root, filepath.FromSlash(rel1))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	id2, _, _ := uniquePaths(root, "epic-1", at)
	if id1 == id2 {
		t.Fatalf("collision not resolved: both runs share ID %s", id1)
	}
}

func TestReadEvidence(t *testing.T) {
	if got, err := readEvidence(""); err != nil || got != "" {
		t.Fatalf("empty path: %q, %v", got, err)
	}
	if _, err := readEvidence(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("missing file must error")
	}
	big := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", 15000)), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readEvidence(big)
	if err != nil || len(got) != 12000 {
		t.Fatalf("expected truncation to 12000, got len=%d err=%v", len(got), err)
	}
	// A small file is returned verbatim.
	small := filepath.Join(t.TempDir(), "small.txt")
	if err := os.WriteFile(small, []byte("- go test ./... pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := readEvidence(small); err != nil || got != "- go test ./... pass\n" {
		t.Fatalf("small evidence should be verbatim: %q, %v", got, err)
	}
}

func TestFilterGeneratedHelpers(t *testing.T) {
	if got := filterGeneratedFiles([]string{".metareview/x", "docs/metareview/y.md", "src/a.go"}); len(got) != 1 || got[0] != "src/a.go" {
		t.Fatalf("filterGeneratedFiles: %v", got)
	}
	if !isGeneratedMetareviewPath(".metareview/x") || !isGeneratedMetareviewPath("docs/metareview/y") || isGeneratedMetareviewPath("src/a.go") {
		t.Fatal("isGeneratedMetareviewPath")
	}
	if got := generatedTargetExceptions("docs/metareview/e.md"); len(got) != 1 {
		t.Fatalf("generatedTargetExceptions metareview: %v", got)
	}
	if got := generatedTargetExceptions("docs/epics/e.md"); got != nil {
		t.Fatalf("generatedTargetExceptions non-metareview: %v", got)
	}
	if len(generatedMetareviewPathExcludes()) == 0 {
		t.Fatal("generatedMetareviewPathExcludes empty")
	}
	if isGeneratedDiffSection("diff --git only-three") {
		t.Error("short diff header not generated")
	}
	if !isGeneratedDiffSection("diff --git a/.metareview/x b/.metareview/x") {
		t.Error("metareview diff section is generated")
	}
	// filterGeneratedDiff / filterGeneratedUntrackedExcerpts drop generated sections, keep source.
	diff := "diff --git a/.metareview/x b/.metareview/x\n+gen\ndiff --git a/src/a.go b/src/a.go\n+src\n"
	if out := filterGeneratedDiff(diff); strings.Contains(out, ".metareview") || !strings.Contains(out, "src/a.go") {
		t.Errorf("filterGeneratedDiff: %q", out)
	}
	unt := "--- .metareview/x\ngen\n--- notes.txt\nkeep\n"
	if out := filterGeneratedUntrackedExcerpts(unt); strings.Contains(out, ".metareview") || !strings.Contains(out, "notes.txt") {
		t.Errorf("filterGeneratedUntrackedExcerpts: %q", out)
	}
	// filterGeneratedGitContext threads all fields through the filters.
	g := filterGeneratedGitContext(gitcontext.Context{ChangedFiles: []string{".metareview/x", "src/a.go"}})
	if len(g.ChangedFiles) != 1 || g.ChangedFiles[0] != "src/a.go" {
		t.Errorf("filterGeneratedGitContext: %v", g.ChangedFiles)
	}
}

func TestVerdictForCounts(t *testing.T) {
	if v, _, blk, r := verdictForCounts(findings.ClassCounts{Blocking: 1}, "gate", 3, 3); v != "ESCALATED" || !blk || r == "" {
		t.Fatalf("ESCALATED: %q %v %q", v, blk, r)
	}
	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{Blocking: 1}, "gate", 1, 3); v != "NEEDS_REVISION" || !blk {
		t.Fatalf("NEEDS_REVISION: %q %v", v, blk)
	}
	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{Advisory: 1}, "gate", 1, 3); v != "PASS_ADVISORY" || blk {
		t.Fatalf("PASS_ADVISORY: %q %v", v, blk)
	}
	if v, _, blk, _ := verdictForCounts(findings.ClassCounts{}, "gate", 1, 3); v != "PASS" || blk {
		t.Fatalf("PASS: %q %v", v, blk)
	}
}

func TestClassForDisplay(t *testing.T) {
	cases := []struct{ classification, severity, want string }{
		{"blocking", "high", "blocking"},
		{"advisory", "", "advisory"},
		{"follow-up", "", "follow-up"},
		{"warning", "", "warning"},
	}
	for _, c := range cases {
		if got := classForDisplay(findings.Record{Classification: c.classification, Severity: c.severity}); got != c.want {
			t.Errorf("classForDisplay(%q,%q) = %q, want %q", c.classification, c.severity, got, c.want)
		}
	}
}

func TestRestoreSnapshotsAndRemoveEmptyDirs(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(existing, []byte("cur"), 0o644); err != nil {
		t.Fatal(err)
	}
	created := filepath.Join(dir, "sub", "new.txt")
	if err := os.MkdirAll(filepath.Dir(created), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(created, []byte("rm"), 0o644); err != nil {
		t.Fatal(err)
	}
	restoreSnapshots(map[string]fileSnapshot{
		existing: {existed: true, content: []byte("ORIG")},
		created:  {existed: false},
	})
	if got, _ := os.ReadFile(existing); string(got) != "ORIG" {
		t.Errorf("restore existing: %q", got)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("absent-in-snapshot file must be removed")
	}
	// removeEmptyDirs prunes the empty docs tree without error.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "metareview", "reviews"), 0o755); err != nil {
		t.Fatal(err)
	}
	removeEmptyDirs(root)
	// snapshot of a missing file records existed=false.
	if s := snapshot(filepath.Join(root, "nope")); s.existed {
		t.Error("snapshot of a missing file should be existed=false")
	}
}

func TestMarkdownListAndSmallHelpers(t *testing.T) {
	if markdownList(nil, "EMPTY") != "EMPTY" {
		t.Error("nil list -> empty sentinel")
	}
	if got := markdownList([]string{"a", "", "a", "b"}, "E"); got != "- a\n- b" {
		t.Errorf("markdownList dedup: %q", got)
	}
	if markdownList([]string{"", ""}, "E") != "E" {
		t.Error("all-empty -> sentinel")
	}
	if firstNonEmpty("", "  ", "x") != "x" || firstNonEmpty("", " ") != "" {
		t.Error("firstNonEmpty")
	}
	if got := findingIDs([]findings.Record{{ID: "a"}, {ID: "b"}}); len(got) != 2 || got[0] != "a" {
		t.Errorf("findingIDs: %v", got)
	}
	for kind, want := range map[string]string{"beads": "beads-epic", "markdown": "path", "other": "advisory"} {
		if got := epicTargetType(epicsource.Source{Kind: kind}); got != want {
			t.Errorf("epicTargetType(%q)=%q want %q", kind, got, want)
		}
	}
	refs := sourceRefs(epicsource.Source{Kind: "beads", ID: "e", Path: ".beads/issues.jsonl", ChildIDs: []string{"c1"}})
	if len(refs) != 2 || refs[0]["path"] != ".beads/issues.jsonl" || refs[1]["id"] != "c1" {
		t.Fatalf("sourceRefs: %v", refs)
	}
	// A markdown epic (no Path=="" case is covered by the beads one having a path; test the no-path id form)
	if refs := sourceRefs(epicsource.Source{Kind: "advisory", ID: "e"}); refs[0]["id"] != "e" || refs[0]["path"] != "" {
		t.Fatalf("sourceRefs no-path: %v", refs)
	}
}

func TestFindingTargetIDAndLogSortKey(t *testing.T) {
	if findingTargetID(map[string]any{"id": "x"}) != "x" {
		t.Error("map[string]any id")
	}
	if findingTargetID(map[string]any{"path": "p"}) != "p" {
		t.Error("map[string]any path")
	}
	if findingTargetID(map[string]string{"id": "y"}) != "y" {
		t.Error("map[string]string id")
	}
	if findingTargetID(map[string]string{"path": "q"}) != "q" {
		t.Error("map[string]string path")
	}
	if findingTargetID("neither") != "" {
		t.Error("unrecognized target -> empty")
	}
	if logSortKey(reviewlog.Summary{RunID: "r"}) != "r" {
		t.Error("logSortKey RunID")
	}
	if logSortKey(reviewlog.Summary{Path: "p"}) != "p" {
		t.Error("logSortKey Path fallback")
	}
}

func TestChildLogHelpers(t *testing.T) {
	// latestChildLogs picks the latest (by sort key) log per child, in child order; empty childIDs -> nil.
	if latestChildLogs(nil, nil) != nil {
		t.Error("no children -> nil")
	}
	logs := []reviewlog.Summary{
		{Target: "c1", RunID: "r1", Verdict: "PASS"},
		{Target: "c1", RunID: "r2", Verdict: "NEEDS_REVISION"},
		{Target: "c2", RunID: "r3", Verdict: "PASS"},
		{Target: "other", RunID: "r9"},
	}
	got := latestChildLogs(logs, []string{"c1", "c2"})
	if len(got) != 2 || got[0].Target != "c1" || got[0].RunID != "r2" || got[1].Target != "c2" {
		t.Fatalf("latestChildLogs: %+v", got)
	}
	// resolveChildren: a resolvable child (beads child-1, nil err -> success append) + a path-like
	// child that does not exist (tasksource errors -> the advisory fallback branch).
	root := epicRepo(t)
	children := resolveChildren(root, []string{"child-1", "docs/tasks/ghost.md"})
	if len(children) != 2 {
		t.Fatalf("resolveChildren count: %v", children)
	}
	var sawAdvisory bool
	for _, c := range children {
		if c.ID == "docs/tasks/ghost.md" && c.Kind == "advisory" {
			sawAdvisory = true
		}
	}
	if !sawAdvisory {
		t.Errorf("a path-like unresolvable child should be an advisory fallback: %+v", children)
	}
	// childOpenBlockerLogs: empty children -> nil; a clean store yields no logs.
	if got, err := childOpenBlockerLogs(root, nil); err != nil || got != nil {
		t.Fatalf("childOpenBlockerLogs empty: %v %v", got, err)
	}
	if got, err := childOpenBlockerLogs(root, []string{"child-1"}); err != nil || got != nil {
		t.Fatalf("childOpenBlockerLogs clean store: %v %v", got, err)
	}
	// With unresolved blockers: a matching child yields one log; a non-child is skipped; a duplicate
	// child is de-duplicated. findingTargetID reads the target id from the record.
	orig := unresolvedBlocking
	t.Cleanup(func() { unresolvedBlocking = orig })
	unresolvedBlocking = func(string) ([]findings.Record, error) {
		return []findings.Record{
			{ID: "f1", Target: map[string]any{"id": "child-1"}},
			{ID: "f2", Target: map[string]any{"id": "not-a-child"}},
			{ID: "f3", Target: map[string]any{"id": "child-1"}},
		}, nil
	}
	logs, err := childOpenBlockerLogs(root, []string{"child-1"})
	if err != nil {
		t.Fatalf("childOpenBlockerLogs: %v", err)
	}
	if len(logs) != 1 || logs[0].Target != "child-1" || !logs[0].HasUnresolvedBlockers {
		t.Fatalf("expected one deduped child blocker log, got %+v", logs)
	}
	// error path via the seam.
	unresolvedBlocking = func(string) ([]findings.Record, error) { return nil, errors.New("boom") }
	if _, err := childOpenBlockerLogs(root, []string{"child-1"}); err == nil {
		t.Fatal("childOpenBlockerLogs should surface the blocker-store error")
	}
}

func TestReviewerAndContextMarkdownHelpers(t *testing.T) {
	epic := epicsource.Source{ID: "e", Title: "E", Body: "epic body", ChildIDs: []string{"c1"}}
	children := []tasksource.Source{{ID: "c1", Title: "C1", Body: "child body"}}
	logs := []reviewlog.Summary{{Target: "c1", Verdict: "PASS", Path: "docs/x.md"}}
	kn := knowledge.Context{ServiceInventoryPath: "docs/SI.md", ServiceInventory: "inv", Facts: []knowledge.Fact{{Source: "k", Text: "f"}}}

	rc := reviewerContext(epic, children, gitcontext.Context{ChangedFiles: []string{"src/a.go"}}, contextprofile.FromGit(gitcontext.Context{}, contextprofile.Options{}), kn, logs, "evidence text")
	if rc.Epic.ID != "e" || len(rc.Children) != 1 || len(rc.ReviewLogs) != 1 {
		t.Fatalf("reviewerContext: %+v", rc)
	}
	if len(reviewerChildren(children)) != 1 || len(reviewerLogs(logs)) != 1 {
		t.Fatal("reviewerChildren/reviewerLogs")
	}
	cm := contextMarkdown("run-1", epic, children, gitcontext.Context{ChangedFiles: []string{"src/a.go"}}, contextprofile.FromGit(gitcontext.Context{}, contextprofile.Options{}), kn, logs, "the-evidence", "gate")
	for _, want := range []string{"epic body", "child body", "src/a.go", "the-evidence", "docs/SI.md", "Gate effect: `gate`"} {
		if !strings.Contains(cm, want) {
			t.Errorf("contextMarkdown missing %q", want)
		}
	}
	if !strings.Contains(childrenMarkdown(nil), "No child tasks") {
		t.Error("childrenMarkdown empty")
	}
	if !strings.Contains(childrenMarkdown(children), "child body") {
		t.Error("childrenMarkdown populated")
	}
	if !strings.Contains(reviewLogsMarkdown(nil), "No child review logs") {
		t.Error("reviewLogsMarkdown empty")
	}
	if !strings.Contains(reviewLogsMarkdown(logs), "c1: PASS") {
		t.Error("reviewLogsMarkdown populated")
	}
	if !strings.Contains(knowledgeMarkdown(kn), "docs/SI.md") || !strings.Contains(knowledgeMarkdown(kn), "k: f") {
		t.Error("knowledgeMarkdown populated")
	}
	if !strings.Contains(knowledgeMarkdown(knowledge.Context{}), "No service inventory found.") {
		t.Error("knowledgeMarkdown empty")
	}
}

// import contextprofile used above.
var _ = contextprofile.Options{}
