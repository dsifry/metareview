package taskdone

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/contextprofile"
	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/shardpack"
)

// fakeWriter records what the review asked of the pack writer.
type fakeWriter struct {
	writes      int
	prunes      int
	rollbacks   int
	discovers   int
	collections int
	lastPlan    contextprofile.ShardPlan
	lastHdr     shardpack.Header
	lastFiles   []gitcontext.BranchFile
	writeErr    error
	pruneErr    error
	gcErr       error
	found       shardpack.Found
	discoverErr error
}

func (f *fakeWriter) Write(root string, plan contextprofile.ShardPlan, header shardpack.Header, files []gitcontext.BranchFile) (func() error, error) {
	f.writes++
	f.lastPlan, f.lastHdr, f.lastFiles = plan, header, files
	rollback := func() error {
		f.rollbacks++
		return nil
	}
	if f.writeErr != nil {
		// Write can fail after the pack set is already in place, so it returns a
		// usable rollback alongside the error; the caller must run it.
		return rollback, f.writeErr
	}
	return rollback, nil
}

func (f *fakeWriter) Prune(root, scope, targetID, keepPlanHash string) error {
	f.prunes++
	return f.pruneErr
}

func (f *fakeWriter) Discover(root, scope, targetID string, plan contextprofile.ShardPlan, explicit []string) (shardpack.Found, error) {
	f.discovers++
	return f.found, f.discoverErr
}

func (f *fakeWriter) GC(root, scope, targetID string, plan contextprofile.ShardPlan) error {
	f.collections++
	return f.gcErr
}

// shardedTaskRepo builds a repo whose branch diff is over the context cap, with a
// task file for task-done to resolve.
func shardedTaskRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	write("seed.txt", "seed\n")
	run("add", ".")
	run("commit", "-m", "initial")
	run("checkout", "-q", "-b", "feature")
	write("docs/tasks/big-task.md", "# Big task\n\nA change too large for one review context.\n")
	filler := func(seed string) string {
		var b strings.Builder
		for i := 0; b.Len() < 25_000; i++ {
			fmt.Fprintf(&b, "%s line %04d 0123456789abcdef\n", seed, i)
		}
		return b.String()
	}
	for i := 0; i < 8; i++ {
		write(fmt.Sprintf("src/file%02d.txt", i), filler(fmt.Sprintf("f%02d", i)))
	}
	run("add", "-A")
	run("commit", "-m", "big branch change")
	return root
}

func TestTaskDonePlanIsWrittenAndPrunedWithMatchingHash(t *testing.T) {
	root := shardedTaskRepo(t)
	writer := &fakeWriter{}
	result, err := Create(root, "docs/tasks/big-task.md", Options{Base: "main", ShardWriter: writer})
	if err != nil {
		t.Fatal(err)
	}
	if writer.writes != 1 || writer.prunes != 1 {
		t.Fatalf("writes=%d prunes=%d, want 1 and 1", writer.writes, writer.prunes)
	}
	if len(writer.lastPlan.Shards) == 0 || writer.lastPlan.PlanHash == "" {
		t.Fatalf("no plan reached the writer: %+v", writer.lastPlan)
	}
	if writer.lastHdr.Scope != "task-done" {
		t.Fatalf("header scope = %q, want task-done", writer.lastHdr.Scope)
	}
	if writer.lastHdr.Budget != contextprofile.DefaultMaxBytesPerShard {
		t.Fatalf("header budget = %d, want %d", writer.lastHdr.Budget, contextprofile.DefaultMaxBytesPerShard)
	}
	if len(writer.lastFiles) == 0 {
		t.Fatal("the measured branch files must reach the writer")
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ContextRel)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, writer.lastPlan.PlanHash) {
		t.Fatal("the context pack does not name the plan hash the writer received")
	}
	if !strings.Contains(text, ".metareview/shards/task-done/") {
		t.Fatalf("the context pack does not name the pack directory:\n%s", text)
	}
}

func TestTaskDonePackWriteFailureFailsTheRun(t *testing.T) {
	root := shardedTaskRepo(t)
	writer := &fakeWriter{writeErr: errors.New("pack boom")}
	_, err := Create(root, "docs/tasks/big-task.md", Options{Base: "main", ShardWriter: writer})
	if err == nil || !strings.Contains(err.Error(), "pack boom") {
		t.Fatalf("err = %v, want the injected pack failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "docs", "metareview", "reviews")); !os.IsNotExist(statErr) {
		t.Fatal("a failed pack write must not leave review artifacts behind")
	}
}

func TestTaskDonePackRollbackRunsWhenTheReviewFails(t *testing.T) {
	root := shardedTaskRepo(t)
	// Make the review-log directory unwritable so the body fails after the packs
	// are in place.
	blocked := filepath.Join(root, "docs", "metareview", "reviews")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	writer := &fakeWriter{}
	if _, err := Create(root, "docs/tasks/big-task.md", Options{Base: "main", ShardWriter: writer}); err == nil {
		t.Skip("the review body unexpectedly succeeded; nothing proved")
	}
	if writer.rollbacks != 1 {
		t.Fatalf("rollbacks = %d, want exactly 1", writer.rollbacks)
	}
	if writer.prunes != 0 || writer.collections != 0 {
		t.Fatalf("housekeeping must not run for a failed review: prunes=%d gc=%d", writer.prunes, writer.collections)
	}
}

// Prune and GC are housekeeping after the run is already recorded: their failure
// must not discard the run the caller needs.
func TestTaskDoneHousekeepingFailuresDoNotDiscardTheRun(t *testing.T) {
	for _, tc := range []struct {
		name   string
		writer *fakeWriter
	}{
		{"prune", &fakeWriter{pruneErr: errors.New("prune boom")}},
		{"gc", &fakeWriter{gcErr: errors.New("gc boom")}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			root := shardedTaskRepo(t)
			result, err := Create(root, "docs/tasks/big-task.md", Options{Base: "main", ShardWriter: tc.writer})
			if err != nil {
				t.Fatalf("a %s failure must not fail the run: %v", tc.name, err)
			}
			if result.RunID == "" {
				t.Fatal("the run identifiers must survive a housekeeping failure")
			}
		})
	}
}

func TestTaskDoneDiscoversShardResults(t *testing.T) {
	root := shardedTaskRepo(t)
	writer := &fakeWriter{}
	if _, err := Create(root, "docs/tasks/big-task.md", Options{Base: "main", ShardWriter: writer}); err != nil {
		t.Fatal(err)
	}
	if writer.discovers != 1 {
		t.Fatalf("discovers = %d, want 1 — results must be looked for on every sharded run", writer.discovers)
	}
}
