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
