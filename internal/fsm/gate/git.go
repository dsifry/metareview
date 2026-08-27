// Package gate holds the deterministic gates evaluated between states and the
// narrow Git interface they need. Every gate returns a coded *run.GateError
// with evidence in Detail; nothing here calls an LLM.
package gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dsifry/metareview/internal/fsm/errs"
)

// Error codes produced by the Git implementations.
const (
	CodeGit    = "ERR_GIT"
	CodeGitRef = "ERR_GIT_REF"
)

// Git is the read-only view of a work tree the gates need.
type Git interface {
	Head(ctx context.Context) (string, error)
	// RevParse resolves any ref (branch, HEAD~1, sha) to a full commit sha.
	RevParse(ctx context.Context, ref string) (string, error)
	// IsAncestor reports whether a is an ancestor of b (exit 1 → false, nil).
	IsAncestor(ctx context.Context, a, b string) (bool, error)
	// CommitCount counts from..to.
	CommitCount(ctx context.Context, from, to string) (int, error)
	// Status returns clean and the porcelain v2 status incl. untracked files.
	Status(ctx context.Context) (clean bool, porcelain string, err error)
	// Diff returns `git diff from..to` cut at a rune boundary ≤ max bytes.
	Diff(ctx context.Context, from, to string, max int) (diff string, truncated bool, err error)
	// WorkingDiff returns `git diff HEAD` cut like Diff.
	WorkingDiff(ctx context.Context, max int) (string, bool, error)
	// WorkTree returns a content hash of the working tree (tracked + untracked,
	// ignored excluded): `git add -A` into a scratch index, then `write-tree`.
	WorkTree(ctx context.Context) (string, error)
}

// Exec runs git in dir with extra env entries and args; it returns stdout,
// stderr, the exit code, and a non-nil err only when the process could not
// be run at all.
type Exec func(ctx context.Context, dir string, env []string, args ...string) (stdout, stderr []byte, code int, err error)

// RealExec shells out to git with prompts, external diff drivers, and
// config injection disabled.
func RealExec(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.fsmonitor=false", "-c", "diff.external=", "--no-pager"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(scrubEnv(cmd.Environ()), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C", "GIT_EXTERNAL_DIFF=", "GIT_CONFIG_NOSYSTEM=1")
	cmd.Env = append(cmd.Env, env...)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if ok := asExit(err, &ee); ok {
			code, err = ee.ExitCode(), nil
		}
	}
	return []byte(out.String()), []byte(errb.String()), code, err
}

// scrubEnv drops GIT_* overrides that could redirect git to another
// repository or inject configuration.
func scrubEnv(environ []string) []string {
	out := environ[:0:0]
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		switch {
		case k == "GIT_DIR", k == "GIT_WORK_TREE", k == "GIT_INDEX_FILE", k == "GIT_EXTERNAL_DIFF", k == "GIT_OBJECT_DIRECTORY", k == "GIT_ALTERNATE_OBJECT_DIRECTORIES", strings.HasPrefix(k, "GIT_CONFIG"):
			continue
		}
		out = append(out, kv)
	}
	return out
}

func asExit(err error, target **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*target = ee
	}
	return ok
}

var shaPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$|^HEAD$`)

// ValidSHA reports whether s is a sha argument the gates may pass to git.
func ValidSHA(s string) bool { return shaPattern.MatchString(s) }

// ValidRef reports whether ref is safe to hand to rev-parse: non-empty, no
// leading '-', no control characters or whitespace.
func ValidRef(ref string) bool {
	if ref == "" || strings.HasPrefix(ref, "-") || !utf8.ValidString(ref) {
		return false
	}
	for _, r := range ref {
		if r < 0x20 || r == 0x7f || r == ' ' {
			return false
		}
	}
	return true
}

type execGit struct {
	dir string
	x   Exec
}

// NewExec returns a Git backed by x in dir.
func NewExec(dir string, x Exec) Git { return &execGit{dir: dir, x: x} }

func (g *execGit) run(ctx context.Context, args ...string) (string, int, error) {
	return g.runEnv(ctx, nil, args...)
}

func (g *execGit) runEnv(ctx context.Context, env []string, args ...string) (string, int, error) {
	out, stderr, code, err := g.x(ctx, g.dir, env, args...)
	if err != nil {
		return "", 0, errs.Wrap(errs.E(CodeGit, fmt.Sprintf("git %s: %v", args[0], err), "op", args[0]), err)
	}
	if code != 0 && code != 1 {
		return "", code, errs.E(CodeGit, fmt.Sprintf("git %s exited %d: %s", args[0], code, strings.TrimSpace(string(stderr))), "op", args[0], "exit", strconv.Itoa(code))
	}
	return string(out), code, nil
}

func (g *execGit) Head(ctx context.Context) (string, error) {
	return g.RevParse(ctx, "HEAD")
}

func (g *execGit) RevParse(ctx context.Context, ref string) (string, error) {
	if !ValidRef(ref) {
		return "", errs.E(CodeGitRef, "invalid ref", "ref", ref)
	}
	out, code, err := g.run(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(out)
	if code != 0 || !shaPattern.MatchString(sha) || len(sha) != 40 {
		return "", errs.E(CodeGit, "unknown ref", "ref", ref, "op", "rev-parse")
	}
	return sha, nil
}

func shaArgs(shas ...string) error {
	for _, s := range shas {
		if !ValidSHA(s) {
			return errs.E(CodeGitRef, "invalid sha", "ref", s)
		}
	}
	return nil
}

func (g *execGit) IsAncestor(ctx context.Context, a, b string) (bool, error) {
	if err := shaArgs(a, b); err != nil {
		return false, err
	}
	_, code, err := g.run(ctx, "merge-base", "--is-ancestor", "--end-of-options", a, b)
	if err != nil {
		return false, err
	}
	return code == 0, nil
}

func (g *execGit) CommitCount(ctx context.Context, from, to string) (int, error) {
	if err := shaArgs(from, to); err != nil {
		return 0, err
	}
	out, code, err := g.run(ctx, "rev-list", "--count", "--end-of-options", from+".."+to)
	if err != nil {
		return 0, err
	}
	n, perr := strconv.Atoi(strings.TrimSpace(out))
	if code != 0 || perr != nil {
		return 0, errs.E(CodeGit, "rev-list --count produced "+strings.TrimSpace(out), "op", "rev-list")
	}
	return n, nil
}

func (g *execGit) Status(ctx context.Context) (bool, string, error) {
	out, code, err := g.run(ctx, "status", "--porcelain=v2", "--untracked-files=all")
	if err != nil {
		return false, "", err
	}
	if code != 0 {
		return false, "", errs.E(CodeGit, "git status exited 1", "op", "status")
	}
	return strings.TrimSpace(out) == "", out, nil
}

func (g *execGit) Diff(ctx context.Context, from, to string, max int) (string, bool, error) {
	if err := shaArgs(from, to); err != nil {
		return "", false, err
	}
	out, code, err := g.run(ctx, "diff", "--no-ext-diff", "--no-textconv", "--end-of-options", from+".."+to)
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, errs.E(CodeGit, "git diff exited 1", "op", "diff")
	}
	d, t := Cut(out, max)
	return d, t, nil
}

func (g *execGit) WorkingDiff(ctx context.Context, max int) (string, bool, error) {
	out, code, err := g.run(ctx, "diff", "--no-ext-diff", "--no-textconv", "HEAD")
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, errs.E(CodeGit, "git diff HEAD exited 1", "op", "diff")
	}
	d, t := Cut(out, max)
	return d, t, nil
}

// WorkTree hashes the working tree through a scratch index so content
// changes (not just paths) move the hash. The scratch index lives in the
// OS temp dir and is removed afterwards.
func (g *execGit) WorkTree(ctx context.Context) (string, error) {
	f, err := os.CreateTemp("", "mrv-index-*")
	if err != nil {
		return "", errs.Wrap(errs.E(CodeGit, "scratch index: "+err.Error(), "op", "write-tree"), err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	defer os.Remove(name)
	env := []string{"GIT_INDEX_FILE=" + name}
	if _, code, err := g.runEnv(ctx, env, "add", "-A", "--"); err != nil {
		return "", err
	} else if code != 0 {
		return "", errs.E(CodeGit, "git add -A exited 1", "op", "add")
	}
	out, code, err := g.runEnv(ctx, env, "write-tree")
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	if code != 0 || len(id) != 40 {
		return "", errs.E(CodeGit, "write-tree produced "+id, "op", "write-tree")
	}
	return id, nil
}

// Cut truncates s to at most max bytes at a rune boundary.
func Cut(s string, max int) (string, bool) {
	if max < 0 || len(s) <= max {
		return s, false
	}
	i := max
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i], true
}

// TreeHash is the working-tree fingerprint: sha256(head + "\n" + workTree),
// where workTree is Git.WorkTree's content hash; the porcelain status is
// stored beside it as evidence but does not feed the hash.
func TreeHash(head, workTree string) string {
	sum := sha256.Sum256([]byte(head + "\n" + workTree))
	return hex.EncodeToString(sum[:])
}
