package gitcontext

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxDiffBytes = 60000
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
}

func Collect(root, requestedBase string) (Context, error) {
	base, err := resolveBase(root, requestedBase)
	if err != nil {
		return Context{}, err
	}
	head, err := git(root, "rev-parse", "HEAD")
	if err != nil {
		return Context{}, err
	}
	diff, diffTruncated, err := limitedGit(root, "diff", base+"..HEAD")
	if err != nil {
		return Context{}, err
	}
	stagedDiff, stagedDiffTruncated, err := limitedGit(root, "diff", "--cached")
	if err != nil {
		return Context{}, err
	}
	workingTreeDiff, workingTreeDiffTruncated, err := limitedGit(root, "diff")
	if err != nil {
		return Context{}, err
	}
	workingTreeFiles := splitLines(tryGit(root, "diff", "--name-only"))
	untrackedFiles := splitLines(tryGit(root, "ls-files", "--others", "--exclude-standard"))
	untrackedExcerpts, err := readUntrackedExcerpts(root, untrackedFiles)
	if err != nil {
		return Context{}, err
	}
	return Context{
		BaseSHA:                  base,
		HeadSHA:                  head,
		Branch:                   tryGit(root, "branch", "--show-current"),
		StatusShort:              tryGit(root, "status", "--short"),
		ChangedFiles:             splitLines(tryGit(root, "diff", "--name-only", base+"..HEAD")),
		StagedFiles:              splitLines(tryGit(root, "diff", "--cached", "--name-only")),
		UnstagedFiles:            workingTreeFiles,
		WorkingTreeFiles:         workingTreeFiles,
		UntrackedFiles:           untrackedFiles,
		DiffStat:                 tryGit(root, "diff", "--stat", base+"..HEAD"),
		StagedStat:               tryGit(root, "diff", "--cached", "--stat"),
		WorkingTreeStat:          tryGit(root, "diff", "--stat"),
		Diff:                     diff,
		DiffTruncated:            diffTruncated,
		StagedDiff:               stagedDiff,
		StagedDiffTruncated:      stagedDiffTruncated,
		WorkingTreeDiff:          workingTreeDiff,
		WorkingTreeDiffTruncated: workingTreeDiffTruncated,
		UntrackedExcerpts:        untrackedExcerpts,
	}, nil
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
	out, err := git(root, args...)
	if err != nil {
		return "", false, err
	}
	return truncate(out, maxDiffBytes)
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

func readUntrackedExcerpts(root string, files []string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	limit := len(files)
	if limit > maxUntrackedFiles {
		limit = maxUntrackedFiles
	}
	sections := make([]string, 0, limit)
	for _, rel := range files[:limit] {
		path, err := safeJoin(rootAbs, rel)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		text := string(bytes)
		if len(text) > maxUntrackedFileBytes {
			text = text[:maxUntrackedFileBytes]
			for !utf8.ValidString(text) && len(text) > 0 {
				text = text[:len(text)-1]
			}
		}
		sections = append(sections, untrackedExcerpt(rel, text))
	}
	return strings.Join(sections, "\n"), nil
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
