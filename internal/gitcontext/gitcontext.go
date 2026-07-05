package gitcontext

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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
}

func Collect(root, requestedBase string) (Context, error) {
	return collect(root, requestedBase, nil, nil)
}

func CollectWithExcludes(root, requestedBase string, excludes []string) (Context, error) {
	return collect(root, requestedBase, excludes, nil)
}

func CollectWithExcludesExcept(root, requestedBase string, excludes, exceptions []string) (Context, error) {
	return collect(root, requestedBase, excludes, exceptions)
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
			return "", fmt.Errorf("Invalid git base: %s", requestedBase)
		}
		return base, nil
	}
	for _, candidate := range [][]string{
		{"merge-base", "HEAD", "main"},
		{"merge-base", "HEAD", "master"},
		{"rev-parse", "HEAD~1"},
	} {
		base, err := git(root, candidate...)
		if err == nil && base != "" {
			return base, nil
		}
	}
	return "", fmt.Errorf("Invalid git base: unable to resolve default base")
}

func validateRef(ref string) error {
	if strings.TrimSpace(ref) == "" ||
		strings.HasPrefix(ref, "-") ||
		strings.Contains(ref, "..") ||
		!refPattern.MatchString(ref) {
		return fmt.Errorf("Invalid git base: %s", ref)
	}
	return nil
}

func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
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

func limitedGit(root string, args ...string) (string, bool, error) {
	out, truncated, _, err := limitedGitMeasured(root, args...)
	return out, truncated, err
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
