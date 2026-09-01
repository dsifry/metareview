package kind

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/gate"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// This drives ReproductionProver.ProveDeletions end to end against a REAL git repo and a REAL
// `go test`, with no seams injected (git = gate.RealExec, tests via the real exec) — so the
// DeletionRef↔diff binding, the fail-before/pass-after reproduction, and the whole-suite scope check
// are all exercised together. The fix DELETES the buggy `if n == 999` block; the reproduction test
// fails on the pre-fix tree (999 wrongly allowed) and passes once the block is gone.

func dGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func dWrite(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func dRev(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func TestProveDeletionRealGit(t *testing.T) {
	const preCalc = "package fixture\n\nfunc Allow(n int) bool {\n\tif n == 999 {\n\t\treturn true\n\t}\n\treturn n < 10\n}\n"
	const postCalc = "package fixture\n\nfunc Allow(n int) bool {\n\treturn n < 10\n}\n"
	const reproTest = "package fixture\n\nimport \"testing\"\n\nfunc TestNo999(t *testing.T) {\n\tif Allow(999) {\n\t\tt.Fatal(\"999 must not be allowed\")\n\t}\n}\n"

	dir := t.TempDir()
	dGit(t, dir, "init", "-q", "-b", "main")
	dWrite(t, dir, "go.mod", "module fixture\n\ngo 1.22\n")
	dWrite(t, dir, "calc.go", preCalc)
	dGit(t, dir, "add", "-A")
	dGit(t, dir, "commit", "-qm", "pre")
	pre := dRev(t, dir)
	// The fix: delete the buggy block AND add the reproduction test.
	dWrite(t, dir, "calc.go", postCalc)
	dWrite(t, dir, "calc_test.go", reproTest)
	dGit(t, dir, "add", "-A")
	dGit(t, dir, "commit", "-qm", "post")
	post := dRev(t, dir)

	proof := run.DifferentialProof{
		ID: "d1", Finding: "f1", Kind: run.ProofDeletion, Test: "TestNo999",
		Deletes: &run.DeletionRef{File: "calc.go", ParentSHA: pre, Removed: "\tif n == 999 {\n\t\treturn true\n\t}"},
	}
	rp := ReproductionProver{Exec: gate.RealExec} // real git; Run nil → real `go test`
	spec := ProveSpec{Dir: dir, TestCmd: []string{"go", "test", "./..."}, PreFixSHA: pre, PostFixSHA: post}
	rs, err := rp.ProveDeletions(context.Background(), []run.DifferentialProof{proof}, spec)
	if err != nil {
		t.Fatalf("ProveDeletions: %v", err)
	}
	if len(rs) != 1 || !rs[0].Proven || rs[0].Outcome != run.PinProven {
		t.Fatalf("a genuine deletion (binding + fail-before/pass-after + green scope) must be proven: %+v", rs[0])
	}

	// A DeletionRef whose span the reviewed diff does not actually remove is malformed, even though the
	// reproduction would still hold — the binding is what ties the proof to the real removed code.
	bad := proof
	badRef := *proof.Deletes
	badRef.Removed = "\tif n == 12345 {\n\t\treturn true\n\t}" // never in the diff
	bad.Deletes = &badRef
	rs, err = rp.ProveDeletions(context.Background(), []run.DifferentialProof{bad}, spec)
	if err != nil {
		t.Fatalf("ProveDeletions: %v", err)
	}
	if rs[0].Proven || rs[0].Outcome != run.PinMalformed {
		t.Fatalf("a span the diff does not remove must be malformed: %+v", rs[0])
	}
}
