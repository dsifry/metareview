package gitcontext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const maxDiffBytes = 120000
const maxUntrackedFiles = 20
const maxUntrackedFileBytes = 4000

var refPattern = regexp.MustCompile(`^[A-Za-z0-9._/@{}^~:-]+$`)

type Context struct {
	BaseSHA                  string   `json:"baseSha"`
	HeadSHA                  string   `json:"headSha"`
	Branch                   string   `json:"branch"`
	StatusShort              string   `json:"statusShort"`
	ChangedFiles             []string `json:"changedFiles"`
	StagedFiles              []string `json:"stagedFiles"`
	UnstagedFiles            []string `json:"unstagedFiles"`
	WorkingTreeFiles         []string `json:"workingTreeFiles"`
	UntrackedFiles           []string `json:"untrackedFiles"`
	DiffStat                 string   `json:"diffStat"`
	StagedStat               string   `json:"stagedStat"`
	WorkingTreeStat          string   `json:"workingTreeStat"`
	Diff                     string   `json:"diff"`
	DiffTruncated            bool     `json:"diffTruncated"`
	StagedDiff               string   `json:"stagedDiff"`
	StagedDiffTruncated      bool     `json:"stagedDiffTruncated"`
	WorkingTreeDiff          string   `json:"workingTreeDiff"`
	WorkingTreeDiffTruncated bool     `json:"workingTreeDiffTruncated"`
	UntrackedExcerpts        string   `json:"untrackedExcerpts"`
	RawDiffBytes             int      `json:"rawDiffBytes"`
	FilteredDiffBytes        int      `json:"filteredDiffBytes"`
	GeneratedExcludedFiles   []string `json:"generatedExcludedFiles"`
	UntrackedOmittedCount    int      `json:"untrackedOmittedCount"`
	UntrackedTruncatedCount  int      `json:"untrackedTruncatedCount"`

	// Branch measurements (spec 0.8.3 §4.1): untruncated, exclude-filtered, and
	// never part of the context-diff JSON payload.
	BranchFiles             []BranchFile `json:"-"`
	BranchDiffFull          string       `json:"-"`
	BranchRawDiffBytes      int          `json:"-"`
	BranchFilteredDiffBytes int          `json:"-"`
}

func Collect(root, requestedBase string) (Context, error) {
	return CollectWith(root, Options{Base: requestedBase})
}

// CollectWith is the seam-carrying entry point; the other Collect* helpers wrap it.
func CollectWith(root string, opts Options) (Context, error) {
	return collectWith(root, opts)
}

func CollectWithExcludes(root, requestedBase string, excludes []string) (Context, error) {
	return CollectWith(root, Options{Base: requestedBase, Excludes: excludes})
}

func CollectWithExcludesExcept(root, requestedBase string, excludes, exceptions []string) (Context, error) {
	return CollectWith(root, Options{Base: requestedBase, Excludes: excludes, Exceptions: exceptions})
}

func collectWith(root string, opts Options) (Context, error) {
	ctx, err := collect(root, opts.Base, opts.Excludes, opts.Exceptions)
	if err != nil {
		return Context{}, err
	}
	if !ctx.DiffTruncated {
		return ctx, nil
	}
	effectiveExcludes := opts.Excludes
	if len(opts.Exceptions) > 0 {
		effectiveExcludes = exactExcludesExcept(root, ctx.BaseSHA, opts.Excludes, opts.Exceptions)
	}
	files, full, err := collectBranchFiles(root, ctx.BaseSHA, effectiveExcludes, opts.runGit())
	if err != nil {
		return Context{}, err
	}
	ctx.BranchFiles = files
	ctx.BranchDiffFull = full
	for _, f := range files {
		ctx.BranchFilteredDiffBytes += f.Bytes
	}
	ctx.BranchRawDiffBytes = ctx.BranchFilteredDiffBytes
	if len(effectiveExcludes) > 0 {
		raw, err := measureBranchFiles(root, ctx.BaseSHA, nil, opts.runGit())
		if err != nil {
			return Context{}, err
		}
		ctx.BranchRawDiffBytes = raw
	}
	return ctx, nil
}

func collect(root, requestedBase string, excludes, exceptions []string) (Context, error) {
	base, err := resolveBase(root, requestedBase)
	if err != nil {
		return Context{}, err
	}
	head, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return Context{}, err
	}
	effectiveExcludes := excludes
	if len(exceptions) > 0 {
		effectiveExcludes = exactExcludesExcept(root, base, excludes, exceptions)
	}
	diff, diffTruncated, branchFilteredDiffBytes, err := limitedGitMeasured(root, withPathspec([]string{"diff", base + "..HEAD"}, effectiveExcludes)...)
	if err != nil {
		return Context{}, err
	}
	stagedDiff, stagedDiffTruncated, stagedFilteredDiffBytes, err := limitedGitMeasured(root, withPathspec([]string{"diff", "--cached"}, effectiveExcludes)...)
	if err != nil {
		return Context{}, err
	}
	workingTreeDiff, workingTreeDiffTruncated, workingTreeFilteredDiffBytes, err := limitedGitMeasured(root, withPathspec([]string{"diff"}, effectiveExcludes)...)
	if err != nil {
		return Context{}, err
	}
	changedFiles := splitLines(tryGit(root, withPathspec([]string{"diff", "--name-only", base + "..HEAD"}, effectiveExcludes)...))
	stagedFiles := splitLines(tryGit(root, withPathspec([]string{"diff", "--cached", "--name-only"}, effectiveExcludes)...))
	workingTreeFiles := splitLines(tryGit(root, withPathspec([]string{"diff", "--name-only"}, effectiveExcludes)...))
	untrackedFiles := splitLines(tryGit(root, withPathspec([]string{"ls-files", "--others", "--exclude-standard"}, effectiveExcludes)...))
	untrackedExcerpts, untrackedOmittedCount, untrackedTruncatedCount, filteredUntrackedBytes, err := readUntrackedExcerpts(root, untrackedFiles)
	if err != nil {
		return Context{}, err
	}
	filteredDiffBytes := branchFilteredDiffBytes + stagedFilteredDiffBytes + workingTreeFilteredDiffBytes + filteredUntrackedBytes
	rawDiffBytes := filteredDiffBytes
	if len(effectiveExcludes) > 0 {
		_, _, rawBranchBytes, err := limitedGitMeasured(root, "diff", base+"..HEAD")
		if err != nil {
			return Context{}, err
		}
		_, _, rawStagedBytes, err := limitedGitMeasured(root, "diff", "--cached")
		if err != nil {
			return Context{}, err
		}
		_, _, rawWorkingTreeBytes, err := limitedGitMeasured(root, "diff")
		if err != nil {
			return Context{}, err
		}
		rawUntrackedFiles := splitLines(tryGit(root, "ls-files", "--others", "--exclude-standard"))
		_, _, _, rawUntrackedBytes, err := readUntrackedExcerpts(root, rawUntrackedFiles)
		if err != nil {
			return Context{}, err
		}
		rawDiffBytes = rawBranchBytes + rawStagedBytes + rawWorkingTreeBytes + rawUntrackedBytes
	}
	excludedGeneratedFiles := generatedExcludedFiles(root, base, effectiveExcludes, changedFiles, stagedFiles, workingTreeFiles, untrackedFiles)
	return Context{
		BaseSHA:                  base,
		HeadSHA:                  head,
		Branch:                   tryGit(root, "branch", "--show-current"),
		StatusShort:              tryGit(root, "status", "--short"),
		ChangedFiles:             changedFiles,
		StagedFiles:              stagedFiles,
		UnstagedFiles:            workingTreeFiles,
		WorkingTreeFiles:         workingTreeFiles,
		UntrackedFiles:           untrackedFiles,
		DiffStat:                 tryGit(root, withPathspec([]string{"diff", "--stat", base + "..HEAD"}, excludes)...),
		StagedStat:               tryGit(root, withPathspec([]string{"diff", "--cached", "--stat"}, excludes)...),
		WorkingTreeStat:          tryGit(root, withPathspec([]string{"diff", "--stat"}, excludes)...),
		Diff:                     diff,
		DiffTruncated:            diffTruncated,
		StagedDiff:               stagedDiff,
		StagedDiffTruncated:      stagedDiffTruncated,
		WorkingTreeDiff:          workingTreeDiff,
		WorkingTreeDiffTruncated: workingTreeDiffTruncated,
		UntrackedExcerpts:        untrackedExcerpts,
		RawDiffBytes:             rawDiffBytes,
		FilteredDiffBytes:        filteredDiffBytes,
		GeneratedExcludedFiles:   excludedGeneratedFiles,
		UntrackedOmittedCount:    untrackedOmittedCount,
		UntrackedTruncatedCount:  untrackedTruncatedCount,
	}, nil
}

func exactExcludesExcept(root, base string, excludes, exceptions []string) []string {
	exceptionSet := stringSet(normalizedPaths(exceptions))
	rawFiles := [][]string{
		splitLines(tryGit(root, "diff", "--name-only", base+"..HEAD")),
		splitLines(tryGit(root, "diff", "--cached", "--name-only")),
		splitLines(tryGit(root, "diff", "--name-only")),
		splitLines(tryGit(root, "ls-files", "--others", "--exclude-standard")),
	}
	seen := map[string]bool{}
	var result []string
	for _, group := range rawFiles {
		for _, file := range group {
			file = filepath.ToSlash(filepath.Clean(file))
			if file == "." || file == "" || exceptionSet[file] || !matchesAnyExclude(file, excludes) || seen[file] {
				continue
			}
			seen[file] = true
			result = append(result, file)
		}
	}
	sort.Strings(result)
	return result
}

func normalizedPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			result = append(result, filepath.ToSlash(filepath.Clean(path)))
		}
	}
	return result
}

func matchesAnyExclude(file string, excludes []string) bool {
	for _, exclude := range excludes {
		if matchesExclude(file, strings.TrimSpace(exclude)) {
			return true
		}
	}
	return false
}

func matchesExclude(file, exclude string) bool {
	exclude = filepath.ToSlash(filepath.Clean(exclude))
	if exclude == "." || exclude == "" {
		return false
	}
	if strings.HasSuffix(exclude, "/**") {
		prefix := strings.TrimSuffix(exclude, "/**")
		return file == prefix || strings.HasPrefix(file, prefix+"/")
	}
	return file == exclude
}

func generatedExcludedFiles(root, base string, excludes []string, changedFiles, stagedFiles, workingTreeFiles, untrackedFiles []string) []string {
	if len(excludes) == 0 {
		return []string{}
	}
	filtered := stringSet(changedFiles, stagedFiles, workingTreeFiles, untrackedFiles)
	rawFiles := [][]string{
		splitLines(tryGit(root, "diff", "--name-only", base+"..HEAD")),
		splitLines(tryGit(root, "diff", "--cached", "--name-only")),
		splitLines(tryGit(root, "diff", "--name-only")),
		splitLines(tryGit(root, "ls-files", "--others", "--exclude-standard")),
	}
	excluded := map[string]bool{}
	for _, group := range rawFiles {
		for _, file := range group {
			if file != "" && !filtered[file] {
				excluded[file] = true
			}
		}
	}
	result := make([]string, 0, len(excluded))
	for file := range excluded {
		result = append(result, file)
	}
	sort.Strings(result)
	return result
}

func stringSet(groups ...[]string) map[string]bool {
	seen := map[string]bool{}
	for _, group := range groups {
		for _, value := range group {
			if value != "" {
				seen[value] = true
			}
		}
	}
	return seen
}

func withPathspec(args []string, excludes []string) []string {
	if len(excludes) == 0 {
		return args
	}
	out := append([]string{}, args...)
	out = append(out, "--", ".")
	for _, exclude := range excludes {
		exclude = strings.TrimSpace(exclude)
		if exclude != "" {
			out = append(out, ":(exclude)"+exclude)
		}
	}
	return out
}

func resolveBase(root, requestedBase string) (string, error) {
	if requestedBase != "" {
		if err := validateRef(requestedBase); err != nil {
			return "", err
		}
		base, err := git(root, "rev-parse", "--verify", requestedBase+"^{commit}")
		if err != nil {
			return "", fmt.Errorf("invalid git base: %s", requestedBase)
		}
		return base, nil
	}
	// These two run BEFORE the loop, and discarding their errors undid the guard below twice
	// over: a stuck git burned two more full deadlines before the abort could fire, and — worse —
	// a timed-out `rev-parse HEAD` left head empty, so `base != head` was trivially true and
	// standing on the default branch resolved to an empty range again. The bug the empty-range
	// check exists to prevent, restored by the very stall the deadline exists to catch.
	head, err := git(root, "rev-parse", "HEAD")
	if errors.Is(err, ErrTimeout) {
		return "", err
	}
	branch, err := git(root, "rev-parse", "--abbrev-ref", "HEAD")
	if errors.Is(err, ErrTimeout) {
		return "", err
	}
	for _, name := range []string{"main", "master"} {
		base, err := git(root, "merge-base", "HEAD", name)
		if errors.Is(err, ErrTimeout) {
			// A stall aborts resolution rather than falling through to the next candidate. Every
			// candidate would spend the full deadline — three of them, so a stale index.lock held
			// the synchronous Stop gate for triple the bound it was given — and worse, a repo
			// slow enough to time out on `merge-base main` but not on `HEAD~1` would resolve the
			// WRONG base and silently scope the branch to one commit.
			return "", err
		}
		if err != nil || base == "" {
			continue
		}
		if base != head {
			return base, nil
		}
		// The merge base IS this commit, and the two cases that produce that are opposite.
		//
		// Standing ON the default branch, the merge base is useless — it answers "you are where
		// you are" — and the scope spanned an empty range, so committed unreviewed work on main
		// reported blocked:false and exit 0 while the identical commit on a feature branch
		// blocked. Fall through to HEAD~1.
		//
		// On a feature branch that has NOT diverged, the same equality is the truth: the branch
		// genuinely has no commits of its own, and an empty commit range is the correct answer.
		// Falling through to HEAD~1 there would hand it the default branch's last commit as its
		// own work, and the hook would block on files the session never touched — which is how a
		// fix for one empty range becomes a false blocker in the other.
		if branch == name {
			break
		}
		return base, nil
	}
	base, err := git(root, "rev-parse", "HEAD~1")
	if errors.Is(err, ErrTimeout) {
		return "", err
	}
	if err == nil && base != "" {
		return base, nil
	}
	return "", fmt.Errorf("invalid git base: unable to resolve default base")
}

func validateRef(ref string) error {
	if strings.TrimSpace(ref) == "" ||
		strings.HasPrefix(ref, "-") ||
		strings.Contains(ref, "..") ||
		!refPattern.MatchString(ref) {
		return fmt.Errorf("invalid git base: %s", ref)
	}
	return nil
}

// hardenDiff pins a diff invocation to git's own patch output. A developer's gitconfig can
// install an external diff driver (diff.external is how difftastic is normally wired up) or
// a textconv filter, and git honours it for every diff this package runs — so the bytes we
// measure, chunk, hash and lint would be the driver's rendering rather than the patch, and
// the review would pass over content nobody saw. Both flags are diff-family options, so they
// go directly after the subcommand. internal/fsm/gate/git.go pins its own diffs the same way.
func hardenDiff(args []string) []string {
	if len(args) == 0 || args[0] != "diff" {
		return args
	}
	out := make([]string, 0, len(args)+2)
	out = append(out, args[0])
	for _, flag := range []string{"--no-ext-diff", "--no-textconv"} {
		if !slices.Contains(args, flag) {
			out = append(out, flag)
		}
	}
	// diff.noprefix=true is an ordinary documented gitconfig setting, and it survives both flags
	// above: git then writes `diff --git f.go f.go`. Every consumer of this payload finds the
	// post-image path by its b/ prefix, so an unpinned prefix makes ChangedPaths empty and every
	// candidate unverified_no_evidence - the review passes over content nobody saw, which is the
	// exact outcome this function exists to prevent. Pin them unless the caller set its own.
	if !hasPrefixFlag(args, "--src-prefix") && !hasPrefixFlag(args, "--no-prefix") {
		out = append(out, "--src-prefix=a/")
	}
	if !hasPrefixFlag(args, "--dst-prefix") && !hasPrefixFlag(args, "--no-prefix") {
		out = append(out, "--dst-prefix=b/")
	}
	return append(out, args[1:]...)
}

func hasPrefixFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}

// Deadline bounds a single git invocation. Exported as a var so a caller running under a
// synchronous host hook can shorten it, and so the timeout path can be tested.
//
// This package is where the risk actually lives: CollectWithExcludes runs several git commands
// and is called BEFORE anything else in the branch scope, so these are the calls that sit on a
// stale index.lock or an unresponsive filesystem. Bounding only the caller's later rev-list left
// the real stall unbounded while the comment claimed otherwise.
var Deadline = 20 * time.Second

// ErrTimeout marks a git call that exceeded Deadline. It is a sentinel rather than a message so
// callers can tell a stalled repository from an ordinary git failure without matching on text:
// the two demand opposite responses, since an ordinary failure is often worth retrying with
// another argument and a stall never is.
var ErrTimeout = errors.New("git timed out")

func git(root string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Deadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", hardenDiff(args)...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		// The FULL command, not just args[0]. Two different calls here are both `rev-parse`, so
		// the subcommand alone cannot say which one stalled — for an operator reading the message
		// or a test distinguishing "aborted at the first call" from "kept going".
		return "", fmt.Errorf("git %s after %s: %w", strings.Join(args, " "), Deadline, ErrTimeout)
	}
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s", message)
	}
	return strings.TrimSpace(string(out)), nil
}

func tryGit(root string, args ...string) string {
	out, err := git(root, args...)
	if err != nil {
		return ""
	}
	return out
}

func limitedGitMeasured(root string, args ...string) (string, bool, int, error) {
	out, err := git(root, args...)
	if err != nil {
		return "", false, 0, err
	}
	truncated, wasTruncated, err := truncate(out, maxDiffBytes)
	return truncated, wasTruncated, len(out), err
}

func truncate(value string, limit int) (string, bool, error) {
	if len(value) <= limit {
		return value, false, nil
	}
	if limit <= 0 {
		return "", true, nil
	}
	truncated := value[:limit]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated, true, nil
}

func splitLines(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{}
	}
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func readUntrackedExcerpts(root string, files []string) (string, int, int, int, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", 0, 0, 0, err
	}
	limit := len(files)
	if limit > maxUntrackedFiles {
		limit = maxUntrackedFiles
	}
	omitted := len(files) - limit
	sections := make([]string, 0, limit)
	truncatedCount := 0
	totalBytes := 0
	for index, rel := range files {
		path, err := safeJoin(rootAbs, rel)
		if err != nil {
			return "", 0, 0, 0, err
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		totalBytes += int(info.Size())
		if index >= limit {
			continue
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return "", 0, 0, 0, err
		}
		text := string(bytes)
		if len(text) > maxUntrackedFileBytes {
			truncatedCount++
			text = text[:maxUntrackedFileBytes]
			for !utf8.ValidString(text) && len(text) > 0 {
				text = text[:len(text)-1]
			}
		}
		sections = append(sections, untrackedExcerpt(rel, text))
	}
	return strings.Join(sections, "\n"), omitted, truncatedCount, totalBytes, nil
}

func safeJoin(rootAbs, rel string) (string, error) {
	clean := filepath.Clean(rel)
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("untracked file is outside repository root: %s", rel)
	}
	path := filepath.Join(rootAbs, clean)
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("untracked file is outside repository root: %s", rel)
	}
	return pathAbs, nil
}

func untrackedExcerpt(rel, text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "+" + line
	}
	return "--- " + rel + "\n" + strings.Join(lines, "\n")
}
