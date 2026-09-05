package covergate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTmp writes content to a fresh file in the test's temp dir and returns its path.
func writeTmp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRunFlagError(t *testing.T) {
	if code, _, _ := run(t, "--nonesuch"); code != 2 {
		t.Fatalf("bad flag should exit 2, got %d", code)
	}
}

func TestRunMissingRequiredFlags(t *testing.T) {
	if code, _, errs := run(t, "--profile", "x"); code != 2 || !strings.Contains(errs, "are all required") {
		t.Fatalf("missing flags should exit 2 with a message, got %d %q", code, errs)
	}
}

func TestRunFileOpenErrors(t *testing.T) {
	prof := writeTmp(t, "p.txt", "mode: atomic\n"+mod+"/internal/a/a.go:1.1,2.2 4 4\n")
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	base := []string{"--module", mod}
	// missing profile
	if code, _, _ := run(t, append(append([]string{}, base...), "--profile", "/no/such", "--require-100", req)...); code != 1 {
		t.Error("missing profile should exit 1")
	}
	// missing require list
	if code, _, _ := run(t, append(append([]string{}, base...), "--profile", prof, "--require-100", "/no/such")...); code != 1 {
		t.Error("missing require list should exit 1")
	}
}

func TestRunProfileParseError(t *testing.T) {
	prof := writeTmp(t, "p.txt", "mode: atomic\nbadline\n")
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	if code, _, _ := run(t, "--module", mod, "--profile", prof, "--require-100", req); code != 1 {
		t.Fatal("malformed profile should exit 1")
	}
}

func TestRunPass(t *testing.T) {
	prof := writeTmp(t, "p.txt", "mode: atomic\n"+
		mod+"/cmd/covergate/main.go:1.1,2.2 3 3\n"+ // required, 100%
		mod+"/internal/a/a.go:1.1,2.2 8 6\n") // not required (excluded), 75% -> still ok
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	code, out, _ := run(t, "--module", mod, "--profile", prof, "--require-100", req)
	if code != 0 || !strings.Contains(out, "coverage gate passed") {
		t.Fatalf("expected pass, got code=%d out=%q", code, out)
	}
	if !strings.Contains(out, "ok (excluded)") {
		t.Errorf("an unrequired profile package should render as excluded: %q", out)
	}
}

func TestRunFail(t *testing.T) {
	prof := writeTmp(t, "p.txt", "mode: atomic\n"+
		mod+"/cmd/covergate/main.go:1.1,2.2 3 1\n"+ // 3 covered
		mod+"/cmd/covergate/main.go:3.1,4.2 1 0\n") // 1 uncovered -> 75% required FAIL
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	code, out, errs := run(t, "--module", mod, "--profile", prof, "--require-100", req)
	if code != 1 || !strings.Contains(errs, "coverage gate FAILED") || !strings.Contains(out, "cmd/covergate") {
		t.Fatalf("expected fail, got code=%d out=%q errs=%q", code, out, errs)
	}
}
