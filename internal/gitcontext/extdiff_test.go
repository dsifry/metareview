package gitcontext

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A developer's own gitconfig may install an external diff driver (diff.external is how
// difftastic is normally wired up) or a textconv filter. git honours it for every diff we
// run, so without --no-ext-diff/--no-textconv the bytes we measure, chunk, hash and lint
// are the driver's rendering rather than the patch. Every git diff in this package feeds
// the review, so the hardening has to hold on all of them, not just the branch diff.
func TestCollectIgnoresExternalDiffDriver(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell driver script is POSIX")
	}
	r := newRepo(t)
	r.write("real.txt", "alpha\nbravo\n")
	r.commit("base")
	r.git("checkout", "-b", "feature")
	r.write("real.txt", "alpha\nCHARLIE-REAL-CONTENT\n")
	r.commit("change")

	driver := filepath.Join(t.TempDir(), "fake-difftastic.sh")
	if err := os.WriteFile(driver, []byte("#!/bin/sh\necho SENTINEL-EXTERNAL-DRIVER-OUTPUT\n"), 0o755); err != nil {
		t.Fatalf("write driver: %v", err)
	}
	r.git("config", "diff.external", driver)

	ctx, err := Collect(r.root, "main")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	for _, tc := range []struct {
		name string
		got  string
	}{
		{"Diff", ctx.Diff},
		{"DiffStat", ctx.DiffStat},
	} {
		if strings.Contains(tc.got, "SENTINEL-EXTERNAL-DRIVER-OUTPUT") {
			t.Errorf("%s came from the external driver, not git: %q", tc.name, tc.got)
		}
	}
	if !strings.Contains(ctx.Diff, "CHARLIE-REAL-CONTENT") {
		t.Errorf("Diff lost the real patch content: %q", ctx.Diff)
	}
	if ctx.FilteredDiffBytes == 0 {
		t.Error("FilteredDiffBytes is 0: the measured byte budget saw no patch")
	}
}

// GIT_EXTERNAL_DIFF is the other half: it is read from the environment and takes precedence
// over diff.external, so config hardening alone would not close it. Our runners inherit the
// caller's environment, so a driver exported in the developer's shell reaches every diff.
func TestCollectIgnoresExternalDiffEnvVar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell driver script is POSIX")
	}
	r := newRepo(t)
	r.write("real.txt", "alpha\nbravo\n")
	r.commit("base")
	r.git("checkout", "-b", "feature")
	r.write("real.txt", "alpha\nDELTA-REAL-CONTENT\n")
	r.commit("change")

	driver := filepath.Join(t.TempDir(), "env-driver.sh")
	if err := os.WriteFile(driver, []byte("#!/bin/sh\necho SENTINEL-ENV-DRIVER-OUTPUT\n"), 0o755); err != nil {
		t.Fatalf("write driver: %v", err)
	}
	t.Setenv("GIT_EXTERNAL_DIFF", driver)

	ctx, err := Collect(r.root, "main")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if strings.Contains(ctx.Diff, "SENTINEL-ENV-DRIVER-OUTPUT") {
		t.Errorf("Diff came from GIT_EXTERNAL_DIFF, not git: %q", ctx.Diff)
	}
	if !strings.Contains(ctx.Diff, "DELTA-REAL-CONTENT") {
		t.Errorf("Diff lost the real patch content: %q", ctx.Diff)
	}
}
