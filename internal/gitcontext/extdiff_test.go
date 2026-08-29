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

// The branch-file diffs are collected by their own runner (branchfiles.go realGit), and its
// argv carries --no-textconv but NOT --no-ext-diff, so hardenDiff() there is the only thing
// stopping an external driver from replacing the per-file patches. Those patches are what the
// shard plan is built from: byte counts, file hashes and the resulting cut. Deleting that
// hardenDiff call left the whole suite green, because the existing tests only cover the
// non-truncated Collect path.
func TestBranchFileDiffsIgnoreAnExternalDiffDriver(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell driver script is POSIX")
	}
	r := newRepo(t)
	r.write("real.txt", "alpha\n")
	r.commit("base")
	r.git("checkout", "-b", "feature")
	// Large enough that Collect truncates and so takes the branch-file path.
	var big strings.Builder
	for big.Len() < maxDiffBytes*2 {
		big.WriteString("DELTA-REAL-CONTENT " + strings.Repeat("x", 60) + "\n")
	}
	r.write("real.txt", big.String())
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
	if !ctx.DiffTruncated {
		t.Fatalf("fixture invalid: the diff must truncate so the branch-file path runs (bytes=%d)", ctx.FilteredDiffBytes)
	}
	if len(ctx.BranchFiles) == 0 {
		t.Fatal("no branch files collected")
	}
	for _, f := range ctx.BranchFiles {
		if strings.Contains(f.Diff, "SENTINEL-EXTERNAL-DRIVER-OUTPUT") {
			t.Errorf("branch file %s carries the driver's output, so the shard plan is built from it", f.Path)
		}
		if f.Bytes < 1000 {
			t.Errorf("branch file %s measured %d bytes: a driver's one-line output, not the patch", f.Path, f.Bytes)
		}
	}
}

// --no-textconv is the other half of the same guard and was entirely unpinned: deleting it from
// hardenDiff's flag list left `go test ./...` green. A .gitattributes textconv filter replaces
// the diff body with the filter's rendering, so the added code is absent from the reviewed
// payload while everything still looks like a normal patch.
func TestCollectIgnoresATextconvFilter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell driver script is POSIX")
	}
	r := newRepo(t)
	r.write("real.txt", "alpha\n")
	r.write(".gitattributes", "*.txt diff=fake\n")
	r.commit("base")
	r.git("checkout", "-b", "feature")
	r.write("real.txt", "alpha\nCHARLIE-REAL-CONTENT\n")
	r.commit("change")

	filter := filepath.Join(t.TempDir(), "fake-textconv.sh")
	if err := os.WriteFile(filter, []byte("#!/bin/sh\necho SENTINEL-TEXTCONV-OUTPUT\n"), 0o755); err != nil {
		t.Fatalf("write filter: %v", err)
	}
	r.git("config", "diff.fake.textconv", filter)

	ctx, err := Collect(r.root, "main")
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if strings.Contains(ctx.Diff, "SENTINEL-TEXTCONV-OUTPUT") {
		t.Errorf("Diff came from the textconv filter, not git: %q", ctx.Diff)
	}
	if !strings.Contains(ctx.Diff, "CHARLIE-REAL-CONTENT") {
		t.Errorf("Diff lost the real patch content: %q", ctx.Diff)
	}
}
