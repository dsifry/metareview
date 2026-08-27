package gitcontext

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
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
	// Without stderr the caller sees a bare "exit status 128" and cannot tell
	// which git invocation failed.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
		}
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
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

// measureBranchFiles totals the branch diff without retaining any of its text.
// The raw (unfiltered) scan needs only the byte count, and excludes usually hide
// the largest content, so materialising it would be the peak allocation.
func measureBranchFiles(root, base string, excludes []string, run RunGitFunc) (int, error) {
	paths, err := branchPaths(root, base, excludes, run)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, p := range paths {
		out, err := run(root, []string{"GIT_LITERAL_PATHSPECS=1"},
			"diff", "--no-renames", "--text", "--no-textconv", base+"..HEAD", "--", p)
		if err != nil {
			return 0, err
		}
		total += len(out)
	}
	return total, nil
}

// fileHash is the v4 content key for one path (spec §3). Fields are joined as
// len:bytes\0 so a path containing a newline cannot forge a boundary.
func fileHash(path, diff string) string {
	sum := sha256.Sum256([]byte(HashFields("mrv-file-v4", path, diff)))
	return hex.EncodeToString(sum[:])[:16]
}

// HashFields encodes fields as len:bytes\0 for the v4 hash preimages. It is the
// single definition of that encoding: contextprofile consumes it, and the file,
// source, chunk, shard and plan hashes must all stay byte-identical forever.
func HashFields(fields ...string) string {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(strconv.Itoa(len(f)))
		b.WriteString(":")
		b.WriteString(f)
		b.WriteString("\x00")
	}
	return b.String()
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
