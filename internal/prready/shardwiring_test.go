package prready

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
	writes    int
	prunes    int
	lastPlan  contextprofile.ShardPlan
	lastHdr   shardpack.Header
	writeErr  error
	rollback  func() error
	rollbacks int
}

func (f *fakeWriter) Write(root string, plan contextprofile.ShardPlan, header shardpack.Header, files []gitcontext.BranchFile) (func() error, error) {
	f.writes++
	f.lastPlan, f.lastHdr = plan, header
	if f.writeErr != nil {
		// Write can fail after the new pack set is already in place, so it hands
		// back a usable rollback alongside the error; callers must run it.
		return func() error {
			f.rollbacks++
			return nil
		}, f.writeErr
	}
	return func() error {
		f.rollbacks++
		if f.rollback != nil {
			return f.rollback()
		}
		return nil
	}, nil
}

func (f *fakeWriter) Prune(root, scope, targetID, keepPlanHash string) error {
	f.prunes++
	return nil
}

func shardedRepo(t *testing.T) string {
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

func TestPlanIsWrittenAndPrunedWithMatchingHash(t *testing.T) {
	root := shardedRepo(t)
	writer := &fakeWriter{}
	result, err := Create(root, Options{Base: "main", ShardWriter: writer})
	if err != nil {
		t.Fatal(err)
	}
	if writer.writes != 1 || writer.prunes != 1 {
		t.Fatalf("writes=%d prunes=%d, want 1 and 1", writer.writes, writer.prunes)
	}
	if len(writer.lastPlan.Shards) == 0 || writer.lastPlan.PlanHash == "" {
		t.Fatalf("no plan reached the writer: %+v", writer.lastPlan)
	}
	if writer.lastHdr.Scope != "pr-ready" || writer.lastHdr.Budget != contextprofile.DefaultMaxBytesPerShard {
		t.Fatalf("header = %+v", writer.lastHdr)
	}
	// The context pack must name the same plan hash and pack directory.
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ContextRel)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, writer.lastPlan.PlanHash) {
		t.Fatal("the context pack does not name the plan hash")
	}
	if !strings.Contains(text, ".metareview/shards/pr-ready/") {
		t.Fatalf("the context pack does not name the pack directory:\n%s", text)
	}
}

func TestPackWriteFailureFailsTheRun(t *testing.T) {
	root := shardedRepo(t)
	writer := &fakeWriter{writeErr: errors.New("pack boom")}
	if _, err := Create(root, Options{Base: "main", ShardWriter: writer}); err == nil ||
		!strings.Contains(err.Error(), "pack boom") {
		t.Fatalf("err = %v, want the pack failure", err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "metareview")); !os.IsNotExist(err) {
		t.Fatal("a failed pack write must not leave review artifacts behind")
	}
}

func TestPackRollbackRunsWhenTheReviewFails(t *testing.T) {
	root := shardedRepo(t)
	// A directory where the review log must be written makes the body fail after
	// the packs are in place.
	blocked := filepath.Join(root, "docs", "metareview", "reviews")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	writer := &fakeWriter{}
	if _, err := Create(root, Options{Base: "main", ShardWriter: writer}); err == nil {
		t.Skip("the review body unexpectedly succeeded; nothing proved")
	}
	if writer.rollbacks != 1 {
		t.Fatalf("rollbacks = %d, want 1", writer.rollbacks)
	}
	if writer.prunes != 0 {
		t.Fatal("prune must not run when the review failed")
	}
}

// TestPackWriteErrorRunsRollback pins that a Write that fails after the packs
// are in place has its rollback honoured rather than discarded.
func TestPackWriteErrorRunsRollback(t *testing.T) {
	root := shardedRepo(t)
	writer := &fakeWriter{writeErr: errors.New("pack boom")}
	if _, err := Create(root, Options{Base: "main", ShardWriter: writer}); err == nil {
		t.Fatal("want the pack failure")
	}
	if writer.rollbacks != 1 {
		t.Fatalf("rollbacks = %d, want 1: a failed pack write must be rolled back", writer.rollbacks)
	}
}
