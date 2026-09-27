package gitcontext

import (
	"reflect"
	"strings"
	"testing"
)

// AC-3.1 (#175): once main advances past the branch point, `--base main` still reviews only the branch's change.
// Its tip used to be the base, so main's new commits appeared in the diff, inverted, as if the branch deleted them.
func TestABranchBaseReviewsOnlyTheBranchAfterTheBaseAdvances(t *testing.T) {
	r := newRepo(t)
	fork := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	r.git("checkout", "-q", "-b", "feat")
	r.write("feat.go", "package p\n")
	r.commit("feat")
	r.git("checkout", "-q", "main")
	r.write("other.go", "package p\n")
	r.commit("main moves on")
	r.git("checkout", "-q", "feat")

	ctx, err := Collect(r.root, "main")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.BaseSHA != fork {
		t.Errorf("base = %s, want the fork point %s", ctx.BaseSHA, fork)
	}
	if !reflect.DeepEqual(ctx.ChangedFiles, []string{"feat.go"}) {
		t.Errorf("changed files = %v, want only the branch's feat.go", ctx.ChangedFiles)
	}
	if ctx.RequestedBase != "main" {
		t.Errorf("requested base = %q, want the base as typed", ctx.RequestedBase)
	}
	// An exact commit is still honoured as given (AC-3.3).
	tip := strings.TrimSpace(r.git("rev-parse", "main"))
	if exact, err := Collect(r.root, tip); err != nil || exact.BaseSHA != tip {
		t.Errorf("--base <sha> = %s (%v), want %s", exact.BaseSHA, err, tip)
	}
}

func TestHeadAndIsAncestor(t *testing.T) {
	r := newRepo(t)
	first := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	r.write("b.go", "package p\n")
	r.commit("second")
	second := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	if head, err := Head(r.root); err != nil || head != second {
		t.Fatalf("Head = %s %v", head, err)
	}
	if ok, err := IsAncestor(r.root, first, second); err != nil || !ok {
		t.Fatalf("first is an ancestor of second: %v %v", ok, err)
	}
	if ok, err := IsAncestor(r.root, second, first); err != nil || ok {
		t.Fatalf("second is not an ancestor of first: %v %v", ok, err)
	}
	// An unknown commit is an operational failure (exit 128), not "no".
	if _, err := IsAncestor(r.root, strings.Repeat("f", 40), second); err == nil {
		t.Fatal("an unknown commit must be an error")
	}
	if _, err := Head(t.TempDir()); err == nil {
		t.Fatal("Head outside a repository must fail")
	}
}

// ForkPoint is where HEAD forked from main or master — never the HEAD~1 fallback the default base uses on the
// default branch or without a main/master, which would let a narrow review pass as covering the whole branch (#176).
func TestForkPoint(t *testing.T) {
	r := newRepo(t)
	fork := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	if _, ok, err := ForkPoint(r.root); err != nil || ok {
		t.Fatalf("on main itself there is no fork point: ok=%v err=%v", ok, err)
	}
	r.git("checkout", "-q", "-b", "feat")
	r.write("f.go", "package p\n")
	r.commit("feat 1")
	r.write("g.go", "package p\n")
	r.commit("feat 2") // HEAD~1 is no longer the fork point
	if got, ok, err := ForkPoint(r.root); err != nil || !ok || got != fork {
		t.Fatalf("ForkPoint = %s %v %v, want %s", got, ok, err, fork)
	}
	r.git("branch", "-q", "-m", "main", "develop") // a trunk that is neither main nor master
	if _, ok, err := ForkPoint(r.root); err != nil || ok {
		t.Fatalf("without a local main or master there is no fork point: ok=%v err=%v", ok, err)
	}
}

func TestCommitExists(t *testing.T) {
	r := newRepo(t)
	head := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	if ok, err := CommitExists(r.root, head); err != nil || !ok {
		t.Fatalf("HEAD exists: %v %v", ok, err)
	}
	if ok, err := CommitExists(r.root, strings.Repeat("e", 40)); err != nil || ok {
		t.Fatalf("an unknown commit does not exist: %v %v", ok, err)
	}
	if _, err := CommitExists(t.TempDir(), head); err == nil {
		t.Fatal("outside a repository must be an error")
	}
}
