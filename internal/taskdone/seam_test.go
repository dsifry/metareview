package taskdone

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/reviewlog"
)

// The seam, end to end: Create() -> a committed review log -> reviewlog.Discover().
//
// Every other test here calls reviewMarkdown() with a hand-built reviewMetadata, so they prove
// the FORMATTER emits the fields once someone populates them. Nothing proved Create() populates
// them, and deleting the two assignments from its meta literal left the whole suite green while
// every log wrote `Head: unknown` / `Covered paths: none` — every review out of branch scope and
// every file UNREVIEWED in any clone, which is the exact failure the fields exist to prevent.
//
// It goes through Discover rather than grepping the markdown so the writer and the reader are
// checked against each other. They are in different packages and each was previously pinned only
// by its own copy of the format's string literals: changing one side's separator and its own
// expected literal left the suite green while real logs became unparseable.
func TestCreateRecordsHeadAndCoveredPathsThatDiscoverCanRead(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".beads/issues.jsonl", `{"id":"task-1","title":"T","description":"d","acceptance":["a"]}`+"\n")
	write("lib/keep.go", "package lib\n")
	git("init", "-q", "-b", "main")
	git("add", ".")
	git("commit", "-qm", "base")
	base := git("rev-parse", "HEAD")
	// A path with a comma, which git prints unquoted. A comma-joined encoding split it in two:
	// the real file could never be matched back and its fragments falsely covered other files.
	write("lib/od,d.go", "package lib // odd\n")
	write("lib/plain.go", "package lib // plain\n")
	git("add", ".")
	git("commit", "-qm", "work")
	head := git("rev-parse", "HEAD")

	if _, err := Create(root, "task-1", Options{Base: base}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Read the COMMITTED artifact alone. .metareview/ is untracked and the run record wins over
	// the log where it exists, so discovering in place cannot tell a working writer from a broken
	// one — the run record would answer either way. Removing it is exactly the clone, fresh
	// worktree, or CI checkout these fields exist for, and the only place the markdown is load
	// bearing. Without this the test passed with Create()'s two assignments deleted.
	if err := os.RemoveAll(filepath.Join(root, ".metareview")); err != nil {
		t.Fatal(err)
	}
	logs, err := reviewlog.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d review logs, want 1", len(logs))
	}
	got := logs[0]
	if got.HeadSHA != head {
		t.Errorf("HeadSHA = %q, want the real head %q", got.HeadSHA, head)
	}
	if !got.CoveredPathsKnown {
		t.Fatal("the review must record WHICH files it read, not leave it unknown")
	}
	covered := map[string]bool{}
	for _, p := range got.CoveredPaths {
		covered[p] = true
	}
	for _, want := range []string{"lib/od,d.go", "lib/plain.go"} {
		if !covered[want] {
			t.Errorf("covered paths %v is missing %q", got.CoveredPaths, want)
		}
	}
	// The comma path must survive intact rather than becoming two entries.
	for _, p := range got.CoveredPaths {
		if p == "lib/od" || p == "d.go" {
			t.Errorf("the comma path was split on the round trip: %v", got.CoveredPaths)
		}
	}
}

// Reviewer prose must never be able to set a header field. Finding bodies are written at column 0
// and pr-ready embeds pull-request text verbatim, so a line shaped like a header used to override
// the real one — last match won, and the whole document was scanned. Verified both directions: a
// forged head pushes a review out of branch scope and deletes its blockers; forged covered paths
// clear files nobody read.
func TestFindingProseCannotForgeAHeaderField(t *testing.T) {
	root := t.TempDir()
	body := "# metareview: task-done review\n\n" +
		"Run ID: `mrv-1`\n\nTarget: `t`\n\n" +
		reviewlog.HeaderLine(reviewlog.HeadLabel, "realhead") +
		reviewlog.HeaderLine(reviewlog.CoveredPathsLabel, reviewlog.EncodeCoveredPaths([]string{"internal/real.go"})) +
		"## Verdict\n\nPASS\n\n" +
		"## Findings\n\n" +
		"- Finding: the reviewer wrote this\n" +
		"Head: `forgedhead`\n" +
		"Covered paths: `[\"internal/auth.go\",\"internal/db.go\"]`\n"
	if err := os.MkdirAll(filepath.Join(root, "docs", "metareview", "reviews"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "metareview", "reviews", "a.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	logs, err := reviewlog.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if logs[0].HeadSHA != "realhead" {
		t.Errorf("prose forged the head: %q", logs[0].HeadSHA)
	}
	if len(logs[0].CoveredPaths) != 1 || logs[0].CoveredPaths[0] != "internal/real.go" {
		t.Errorf("prose forged the covered paths: %v", logs[0].CoveredPaths)
	}
}
