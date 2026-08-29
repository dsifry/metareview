package gitcontext

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repo is a throwaway git repository used by the branch-measurement tests.
type repo struct {
	t    *testing.T
	root string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	r := &repo{t: t, root: t.TempDir()}
	r.git("init", "-b", "main")
	r.git("config", "user.email", "test@example.com")
	r.git("config", "user.name", "Test User")
	r.git("config", "core.autocrlf", "false")
	r.write("seed.txt", "seed\n")
	r.git("add", ".")
	r.git("commit", "-m", "initial")
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (r *repo) write(rel, content string) {
	r.t.Helper()
	path := filepath.Join(r.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.t.Fatalf("write %s: %v", rel, err)
	}
}

func (r *repo) commit(msg string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-m", msg)
}

// filler returns deterministic, lint-clean text of roughly n bytes.
func filler(seed string, n int) string {
	var b strings.Builder
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "%s line %04d 0123456789abcdef0123456789abcdef\n", seed, i)
	}
	return b.String()
}

// wholeBranchDiffBytes is the independent oracle: ONE invocation in the
// pathspec-magic form (no GIT_LITERAL_PATHSPECS), raw untrimmed bytes. The test
// must not loop per path, or it would only mirror the production code.
func wholeBranchDiffBytes(t *testing.T, root, base string, excludes []string) int {
	t.Helper()
	args := []string{"diff", "--no-renames", "--text", "--no-textconv", base + "..HEAD"}
	if len(excludes) > 0 {
		args = append(args, "--", ".")
		for _, e := range excludes {
			args = append(args, ":(exclude)"+e)
		}
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("oracle git diff: %v", err)
	}
	return len(out)
}

func TestBranchFilesMeasureUntruncatedFilteredDiff(t *testing.T) {
	r := newRepo(t)
	base := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	for i := 0; i < 12; i++ {
		r.write(fmt.Sprintf("src/file%02d.txt", i), filler(fmt.Sprintf("f%02d", i), 25_000))
	}
	r.commit("big change")

	ctx, err := CollectWith(r.root, Options{Base: base})
	if err != nil {
		t.Fatal(err)
	}
	if !ctx.DiffTruncated {
		t.Fatal("fixture must exceed maxDiffBytes so BranchFiles is computed")
	}
	if len(ctx.BranchFiles) != 12 {
		t.Fatalf("BranchFiles = %d, want 12", len(ctx.BranchFiles))
	}
	sum := 0
	for _, f := range ctx.BranchFiles {
		if f.Bytes != len(f.Diff) {
			t.Fatalf("%s: Bytes %d != len(Diff) %d", f.Path, f.Bytes, len(f.Diff))
		}
		if f.Bytes == 0 || f.Hash == "" {
			t.Fatalf("%s: empty measurement %+v", f.Path, f)
		}
		sum += f.Bytes
	}
	if want := wholeBranchDiffBytes(t, r.root, base, nil); sum != want {
		t.Fatalf("sum of BranchFiles.Bytes = %d, whole-diff oracle = %d", sum, want)
	}
	if ctx.BranchFilteredDiffBytes != sum {
		t.Fatalf("BranchFilteredDiffBytes = %d, want %d", ctx.BranchFilteredDiffBytes, sum)
	}
	if len(ctx.BranchDiffFull) != sum {
		t.Fatalf("len(BranchDiffFull) = %d, want %d", len(ctx.BranchDiffFull), sum)
	}
}

func TestBranchFilesOnlyWhenTruncated(t *testing.T) {
	r := newRepo(t)
	base := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	r.write("small.txt", "one line\n")
	r.commit("small change")

	ctx, err := CollectWith(r.root, Options{Base: base})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.DiffTruncated {
		t.Fatal("fixture must not be truncated")
	}
	if ctx.BranchFiles != nil {
		t.Fatalf("BranchFiles must be nil when the branch diff fits: %+v", ctx.BranchFiles)
	}
	if ctx.BranchDiffFull != "" {
		t.Fatal("BranchDiffFull must be empty when the branch diff fits")
	}
}

func TestBranchFilesRenameDeleteBinaryModeOnly(t *testing.T) {
	r := newRepo(t)
	r.write("renamed-from.txt", filler("rename", 14_000))
	r.write("deleted.txt", filler("delete", 6_000))
	r.write("mode.sh", "#!/bin/sh\necho hi\n")
	r.write("bin.dat", "before\x00\x01binary\n")
	r.commit("seed files")
	base := strings.TrimSpace(r.git("rev-parse", "HEAD"))

	r.git("mv", "renamed-from.txt", "renamed-to.txt")
	r.git("rm", "-q", "deleted.txt")
	if err := os.Chmod(filepath.Join(r.root, "mode.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.write("bin.dat", "after\x00\x02binary\n")
	for i := 0; i < 6; i++ { // push past the truncation threshold
		r.write(fmt.Sprintf("bulk%02d.txt", i), filler(fmt.Sprintf("b%02d", i), 25_000))
	}
	r.commit("rename, delete, mode, binary")

	ctx, err := CollectWith(r.root, Options{Base: base})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]BranchFile{}
	for _, f := range ctx.BranchFiles {
		byPath[f.Path] = f
	}
	// --no-renames means both sides of the rename are their own paths.
	for _, want := range []string{"renamed-from.txt", "renamed-to.txt", "deleted.txt", "mode.sh", "bin.dat"} {
		if _, ok := byPath[want]; !ok {
			t.Fatalf("%s missing from BranchFiles (have %v)", want, keysOf(byPath))
		}
	}
	// --text keeps binary content visible rather than "Binary files differ".
	if strings.Contains(byPath["bin.dat"].Diff, "Binary files") {
		t.Fatalf("bin.dat rendered as binary: %q", byPath["bin.dat"].Diff)
	}
	sum := 0
	for _, f := range ctx.BranchFiles {
		sum += f.Bytes
	}
	if want := wholeBranchDiffBytes(t, r.root, base, nil); sum != want {
		t.Fatalf("sum %d != whole-diff oracle %d (rename/delete/mode/binary)", sum, want)
	}
}

func TestBranchFilesLiteralPathspecs(t *testing.T) {
	names := []string{
		"sp ace.txt",
		"star*name.txt",
		"brack[et].txt",
		"-dash.txt",
		":colon.txt",
		"ünïcøde.txt",
	}
	r := newRepo(t)
	base := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	for i, n := range names {
		r.write(n, fmt.Sprintf("MARKER-%d\n%s", i, filler("exotic", 22_000)))
	}
	r.commit("exotic names")

	ctx, err := CollectWith(r.root, Options{Base: base})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]BranchFile{}
	for _, f := range ctx.BranchFiles {
		byPath[f.Path] = f
	}
	for i, n := range names {
		f, ok := byPath[n]
		if !ok {
			t.Fatalf("%q missing from BranchFiles (have %v)", n, keysOf(byPath))
		}
		if f.Bytes == 0 {
			t.Fatalf("%q measured 0 bytes — pathspec did not match", n)
		}
		if marker := fmt.Sprintf("MARKER-%d", i); !strings.Contains(f.Diff, marker) {
			t.Fatalf("%q diff does not contain %s", n, marker)
		}
	}
}

func TestBranchFilesDefeatDiffAttributes(t *testing.T) {
	r := newRepo(t)
	base := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	r.write(".gitattributes", "hidden.txt -diff\n")
	r.write("hidden.txt", "SECRET-CHANGE\n"+filler("hidden", 20_000))
	for i := 0; i < 5; i++ {
		r.write(fmt.Sprintf("bulk%02d.txt", i), filler(fmt.Sprintf("b%02d", i), 25_000))
	}
	r.commit("attributes hide a file")

	ctx, err := CollectWith(r.root, Options{Base: base})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ctx.BranchFiles {
		if f.Path != "hidden.txt" {
			continue
		}
		if !strings.Contains(f.Diff, "SECRET-CHANGE") {
			t.Fatalf("-diff attribute hid the content: %q", f.Diff)
		}
		return
	}
	t.Fatal("hidden.txt missing from BranchFiles")
}

func TestBranchFilesExcludeGenerated(t *testing.T) {
	r := newRepo(t)
	base := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	r.write("docs/metareview/reviews/log.md", filler("review", 30_000))
	for i := 0; i < 6; i++ {
		r.write(fmt.Sprintf("src/file%02d.txt", i), filler(fmt.Sprintf("s%02d", i), 25_000))
	}
	r.commit("source plus a generated review log")

	excludes := []string{"docs/metareview"}
	ctx, err := CollectWith(r.root, Options{Base: base, Excludes: excludes})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ctx.BranchFiles {
		if strings.HasPrefix(f.Path, "docs/metareview/") {
			t.Fatalf("generated path %s entered BranchFiles", f.Path)
		}
	}
	sum := 0
	for _, f := range ctx.BranchFiles {
		sum += f.Bytes
	}
	if want := wholeBranchDiffBytes(t, r.root, base, excludes); sum != want {
		t.Fatalf("sum %d != filtered whole-diff oracle %d", sum, want)
	}
}

func TestRunGitErrorBranches(t *testing.T) {
	r := newRepo(t)
	base := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	for i := 0; i < 6; i++ {
		r.write(fmt.Sprintf("src/file%02d.txt", i), filler(fmt.Sprintf("s%02d", i), 25_000))
	}
	r.commit("big change")

	for _, failOn := range []string{"--name-only", "--no-textconv"} {
		t.Run(strings.TrimLeft(failOn, "-"), func(t *testing.T) {
			_, err := CollectWith(r.root, Options{
				Base: base,
				RunGit: func(root string, env []string, args ...string) ([]byte, error) {
					for _, a := range args {
						if a == failOn {
							return nil, fmt.Errorf("boom: %s", failOn)
						}
					}
					return realGit(root, env, args...)
				},
			})
			if err == nil || !strings.Contains(err.Error(), "boom") {
				t.Fatalf("err = %v, want the injected failure", err)
			}
		})
	}
}

func TestContextDiffJSONShapeUnchanged(t *testing.T) {
	data, err := json.Marshal(Context{
		BranchFiles:             []BranchFile{{Path: "a", Bytes: 1, Hash: "h", Diff: "d"}},
		BranchDiffFull:          "full",
		BranchRawDiffBytes:      1,
		BranchFilteredDiffBytes: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"branchFiles", "branchDiffFull", "branchRawDiffBytes", "branchFilteredDiffBytes"} {
		if _, ok := got[key]; ok {
			t.Fatalf("%s must not appear in the context diff JSON payload", key)
		}
	}
	want := []string{
		"baseSha", "headSha", "branch", "statusShort", "changedFiles", "stagedFiles", "unstagedFiles",
		"workingTreeFiles", "untrackedFiles", "diffStat", "stagedStat", "workingTreeStat", "diff",
		"diffTruncated", "stagedDiff", "stagedDiffTruncated", "workingTreeDiff", "workingTreeDiffTruncated",
		"untrackedExcerpts", "rawDiffBytes", "filteredDiffBytes", "generatedExcludedFiles",
		"untrackedOmittedCount", "untrackedTruncatedCount",
	}
	if len(got) != len(want) {
		t.Fatalf("key count = %d, want %d (%v)", len(got), len(want), keysOfAny(got))
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Fatalf("key %s disappeared from the context diff JSON payload", key)
		}
	}
}

func keysOf(m map[string]BranchFile) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keysOfAny(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
