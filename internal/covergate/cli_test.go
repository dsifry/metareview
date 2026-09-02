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
	floor := writeTmp(t, "f.txt", "internal/a 100\n")
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	base := []string{"--module", mod}
	// missing profile
	if code, _, _ := run(t, append(append([]string{}, base...), "--profile", "/no/such", "--floor", floor, "--require-100", req)...); code != 1 {
		t.Error("missing profile should exit 1")
	}
	// missing floor
	if code, _, _ := run(t, append(append([]string{}, base...), "--profile", prof, "--floor", "/no/such", "--require-100", req)...); code != 1 {
		t.Error("missing floor should exit 1")
	}
	// missing require list
	if code, _, _ := run(t, append(append([]string{}, base...), "--profile", prof, "--floor", floor, "--require-100", "/no/such")...); code != 1 {
		t.Error("missing require list should exit 1")
	}
}

func TestRunProfileParseError(t *testing.T) {
	prof := writeTmp(t, "p.txt", "mode: atomic\nbadline\n")
	floor := writeTmp(t, "f.txt", "")
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	if code, _, _ := run(t, "--module", mod, "--profile", prof, "--floor", floor, "--require-100", req); code != 1 {
		t.Fatal("malformed profile should exit 1")
	}
}

func TestRunFloorParseError(t *testing.T) {
	prof := writeTmp(t, "p.txt", "mode: atomic\n")
	floor := writeTmp(t, "f.txt", "only-one\n")
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	if code, _, _ := run(t, "--module", mod, "--profile", prof, "--floor", floor, "--require-100", req); code != 1 {
		t.Fatal("malformed floor should exit 1")
	}
}

func TestRunPass(t *testing.T) {
	prof := writeTmp(t, "p.txt", "mode: atomic\n"+
		mod+"/cmd/covergate/main.go:1.1,2.2 3 3\n"+ // required, 100%
		mod+"/internal/a/a.go:1.1,2.2 8 8\n") // floored 80, 100% -> ok
	floor := writeTmp(t, "f.txt", "internal/a 80\n")
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	code, out, _ := run(t, "--module", mod, "--profile", prof, "--floor", floor, "--require-100", req)
	if code != 0 || !strings.Contains(out, "coverage gate passed") {
		t.Fatalf("expected pass, got code=%d out=%q", code, out)
	}
}

func TestRunFail(t *testing.T) {
	prof := writeTmp(t, "p.txt", "mode: atomic\n"+
		mod+"/cmd/covergate/main.go:1.1,2.2 3 1\n"+ // 3 covered
		mod+"/cmd/covergate/main.go:3.1,4.2 1 0\n") // 1 uncovered -> 75% required FAIL
	floor := writeTmp(t, "f.txt", "")
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	code, out, errs := run(t, "--module", mod, "--profile", prof, "--floor", floor, "--require-100", req)
	if code != 1 || !strings.Contains(errs, "coverage gate FAILED") || !strings.Contains(out, "cmd/covergate") {
		t.Fatalf("expected fail, got code=%d out=%q errs=%q", code, out, errs)
	}
}

func TestRunUpdateFloor(t *testing.T) {
	prof := writeTmp(t, "p.txt", "mode: atomic\n"+
		mod+"/cmd/covergate/main.go:1.1,2.2 4 4\n"+ // required, excluded from floor
		mod+"/internal/new/new.go:1.1,2.2 9 1\n"+ // 9 covered
		mod+"/internal/new/new.go:3.1,4.2 1 0\n") // 1 uncovered -> 90% measured
	floor := writeTmp(t, "f.txt", "") // empty
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	code, out, _ := run(t, "--module", mod, "--profile", prof, "--floor", floor, "--require-100", req, "--update-floor")
	if code != 0 || !strings.Contains(out, "floor updated") {
		t.Fatalf("update-floor should pass, code=%d out=%q", code, out)
	}
	// the floor file now carries internal/new at 90.0, not cmd/covergate
	body, _ := os.ReadFile(floor)
	if !strings.Contains(string(body), "internal/new 90.0") || strings.Contains(string(body), "cmd/covergate") {
		t.Errorf("floor file wrong after update: %q", body)
	}
}

func TestRunUpdateFloorRefused(t *testing.T) {
	prof := writeTmp(t, "p.txt", "mode: atomic\n"+
		mod+"/cmd/covergate/main.go:1.1,2.2 4 4\n"+
		mod+"/internal/a/a.go:1.1,2.2 7 1\n"+ // 7 covered
		mod+"/internal/a/a.go:3.1,4.2 3 0\n") // 3 uncovered -> 70%, floor 80 -> refuse-lower
	floor := writeTmp(t, "f.txt", "internal/a 80\n")
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	if code, _, errs := run(t, "--module", mod, "--profile", prof, "--floor", floor, "--require-100", req, "--update-floor"); code != 1 || !strings.Contains(errs, "refusing to lower") {
		t.Fatalf("refuse-lower expected, code=%d errs=%q", code, errs)
	}
}

func TestRunUpdateFloorWriteError(t *testing.T) {
	prof := writeTmp(t, "p.txt", "mode: atomic\n"+
		mod+"/cmd/covergate/main.go:1.1,2.2 4 4\n"+ // required
		mod+"/internal/a/a.go:1.1,2.2 10 9\n") // measured, no floor -> UpdateFloor succeeds
	// A read-only floor file: readFloor (O_RDONLY) succeeds and parses empty, UpdateFloor succeeds,
	// then os.WriteFile (O_WRONLY) fails on the 0444 file — exercising Run's write-error branch.
	floor := writeTmp(t, "f.txt", "")
	if err := os.Chmod(floor, 0o444); err != nil {
		t.Fatal(err)
	}
	req := writeTmp(t, "r.txt", mod+"/cmd/covergate\n")
	code, _, errs := run(t, "--module", mod, "--profile", prof, "--floor", floor, "--require-100", req, "--update-floor")
	if code != 1 || !strings.Contains(errs, "writing floor") {
		t.Fatalf("read-only floor should trigger a write error, code=%d errs=%q", code, errs)
	}
}
