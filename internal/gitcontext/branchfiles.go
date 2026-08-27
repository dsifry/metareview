package gitcontext

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// BranchFile is the untruncated, exclude-filtered branch diff of one path,
// measured and hashed. It never reaches the context-diff JSON payload.
type BranchFile struct {
	Path  string
	Bytes int
	Hash  string
	Diff  string
}

// RunGitFunc runs git and returns its raw, untrimmed stdout. env entries are
// added to the child environment (e.g. GIT_LITERAL_PATHSPECS=1).
type RunGitFunc func(root string, env []string, args ...string) ([]byte, error)

// Options configures Collect. A nil RunGit uses the real git binary.
type Options struct {
	Base       string
	Excludes   []string
	Exceptions []string
	RunGit     RunGitFunc
}

func realGit(root string, env []string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (o Options) runGit() RunGitFunc {
	if o.RunGit != nil {
		return o.RunGit
	}
	return realGit
}

// branchPaths lists the branch-diff paths in the pathspec-magic form (no
// GIT_LITERAL_PATHSPECS, so :(exclude) is honoured), split on NUL.
func branchPaths(root, base string, excludes []string, run RunGitFunc) ([]string, error) {
	args := []string{"diff", "--name-only", "-z", "--no-renames", base + "..HEAD"}
	args = append(args, pathspecArgs(excludes)...)
	out, err := run(root, nil, args...)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func pathspecArgs(excludes []string) []string {
	var out []string
	for _, exclude := range excludes {
		exclude = strings.TrimSpace(exclude)
		if exclude != "" {
			out = append(out, ":(exclude)"+exclude)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return append([]string{"--", "."}, out...)
}

// collectBranchFiles measures each branch path with a literal pathspec. The env
// var and a :(literal) prefix are mutually exclusive, so the path is passed bare.
func collectBranchFiles(root, base string, excludes []string, run RunGitFunc) ([]BranchFile, string, error) {
	paths, err := branchPaths(root, base, excludes, run)
	if err != nil {
		return nil, "", err
	}
	files := make([]BranchFile, 0, len(paths))
	var full strings.Builder
	for _, p := range paths {
		out, err := run(root, []string{"GIT_LITERAL_PATHSPECS=1"},
			"diff", "--no-renames", "--text", "--no-textconv", base+"..HEAD", "--", p)
		if err != nil {
			return nil, "", err
		}
		text := string(out)
		files = append(files, BranchFile{Path: p, Bytes: len(text), Hash: fileHash(p, text), Diff: text})
		full.WriteString(text)
	}
	return files, full.String(), nil
}

// fileHash is the v4 content key for one path (spec §3). Fields are joined as
// len:bytes\0 so a path containing a newline cannot forge a boundary.
func fileHash(path, diff string) string {
	sum := sha256.Sum256([]byte(hashFields("mrv-file-v4", path, diff)))
	return hex.EncodeToString(sum[:])[:16]
}

// HashFields encodes fields as len:bytes\0 for the v4 hash preimages.
func hashFields(fields ...string) string {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(itoa(len(f)))
		b.WriteString(":")
		b.WriteString(f)
		b.WriteString("\x00")
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// AddedLines returns the added lines of the branch diff (untruncated when it was
// measured) together with staged, working-tree and untracked additions.
func AddedLines(ctx Context) []string {
	branch := ctx.BranchDiffFull
	if branch == "" {
		branch = ctx.Diff
	}
	var lines []string
	for _, text := range []string{branch, ctx.StagedDiff, ctx.WorkingTreeDiff, ctx.UntrackedExcerpts} {
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				lines = append(lines, strings.TrimPrefix(line, "+"))
			} else if text == ctx.UntrackedExcerpts && line != "" && !strings.HasPrefix(line, "--- ") {
				lines = append(lines, line)
			}
		}
	}
	return lines
}
