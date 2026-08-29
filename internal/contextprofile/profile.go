package contextprofile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/gitcontext"
)

const (
	RiskNone        = "none"
	RiskAdvisory    = "advisory"
	RiskContextRisk = "context-risk"

	ReasonDiffTruncated      = "DIFF_TRUNCATED"
	ReasonLocalDiffTruncated = "LOCAL_DIFF_TRUNCATED"
	ReasonDiffOversize       = "DIFF_OVERSIZE"
	ReasonLargeDiff          = "LARGE_DIFF"
	ReasonUntrackedOmitted   = "UNTRACKED_OMITTED"
	ReasonUntrackedTruncated = "UNTRACKED_TRUNCATED"

	DefaultLargeDiffBytes = 120000
)

// maxBranchDiffBytes bounds the measured branch diff (var so tests can lower it).
var maxBranchDiffBytes = 16 << 20

// FileProfile.Source values.
const (
	SourceBranch    = "branch"
	SourceStaged    = "staged"
	SourceWorktree  = "worktree"
	SourceUntracked = "untracked"
)

type Options struct {
	LargeDiffBytes int
}

type Risk struct {
	Level   string
	Reasons []string
}

type FileProfile struct {
	Path      string
	DiffBytes int
	Hash      string
	Source    string
	// LocalBytes are the staged, working-tree or untracked bytes for this path.
	// A file can be both committed on the branch and dirty locally: the committed
	// bytes reach a shard pack, the local ones reach no pack at all. Recording
	// them separately is what lets the manifest disclose the difference, rather
	// than the branch total silently replacing them.
	LocalBytes int
}

type Profile struct {
	RawDiffBytes            int
	FilteredDiffBytes       int
	GeneratedExcludedFiles  []string
	UntrackedOmittedCount   int
	UntrackedTruncatedCount int
	Risk                    Risk
	RiskLevel               string
	RiskReasons             []string
	Files                   []FileProfile
}

func FromGit(git gitcontext.Context, options Options) Profile {
	rawDiffBytes := git.RawDiffBytes
	if rawDiffBytes == 0 {
		rawDiffBytes = len(git.Diff)
	}
	filteredDiffBytes := git.FilteredDiffBytes
	if filteredDiffBytes == 0 {
		filteredDiffBytes = len(git.Diff)
	}
	// When the branch diff was measured per file, the truncated branch text is
	// fiction: use the measured branch bytes plus the local contributions, taken
	// directly rather than by subtraction (which would double-count).
	if branch := branchDiffBytes(git); branch > 0 {
		local := len(git.StagedDiff) + len(git.WorkingTreeDiff) + len(git.UntrackedExcerpts)
		filteredDiffBytes = branch + local
		rawDiffBytes = filteredDiffBytes
		if git.BranchRawDiffBytes > 0 {
			rawDiffBytes = git.BranchRawDiffBytes + local
		}
	}

	reasons := riskReasons(git, filteredDiffBytes, options)
	level := RiskNone
	if len(reasons) > 0 {
		level = RiskContextRisk
	}
	files := filesFromGit(git)
	profile := Profile{
		RawDiffBytes:            rawDiffBytes,
		FilteredDiffBytes:       filteredDiffBytes,
		GeneratedExcludedFiles:  append([]string{}, git.GeneratedExcludedFiles...),
		UntrackedOmittedCount:   git.UntrackedOmittedCount,
		UntrackedTruncatedCount: git.UntrackedTruncatedCount,
		Risk:                    Risk{Level: level, Reasons: reasons},
		RiskLevel:               level,
		RiskReasons:             reasons,
		Files:                   files,
	}
	return profile
}

func Markdown(profile Profile) string {
	lines := []string{
		"## Context Profile",
		"",
		"- Raw diff bytes: `" + fmt.Sprint(profile.RawDiffBytes) + "`",
		"- Filtered diff bytes: `" + fmt.Sprint(profile.FilteredDiffBytes) + "`",
		"- Risk level: `" + firstNonEmpty(profile.RiskLevel, RiskNone) + "`",
	}
	if len(profile.RiskReasons) > 0 {
		lines = append(lines, "- Risk reasons: `"+strings.Join(profile.RiskReasons, "`, `")+"`")
	}
	if len(profile.GeneratedExcludedFiles) > 0 {
		lines = append(lines, "- Generated files excluded: "+strings.Join(profile.GeneratedExcludedFiles, ", "))
	}
	if profile.UntrackedOmittedCount > 0 {
		lines = append(lines, "- Untracked files omitted: `"+fmt.Sprint(profile.UntrackedOmittedCount)+"`")
	}
	if profile.UntrackedTruncatedCount > 0 {
		lines = append(lines, "- Untracked files truncated: `"+fmt.Sprint(profile.UntrackedTruncatedCount)+"`")
	}
	return strings.Join(lines, "\n")
}

func riskReasons(git gitcontext.Context, filteredDiffBytes int, options Options) []string {
	var reasons []string
	if git.DiffTruncated {
		reasons = append(reasons, ReasonDiffTruncated)
	}
	if git.StagedDiffTruncated || git.WorkingTreeDiffTruncated {
		reasons = append(reasons, ReasonLocalDiffTruncated)
	}
	if branchBytes := branchDiffBytes(git); branchBytes > maxBranchDiffBytes {
		reasons = append(reasons, ReasonDiffOversize)
	}
	if filteredDiffBytes > largeDiffLimit(options) {
		reasons = append(reasons, ReasonLargeDiff)
	}
	if git.UntrackedOmittedCount > 0 {
		reasons = append(reasons, ReasonUntrackedOmitted)
	}
	if git.UntrackedTruncatedCount > 0 {
		reasons = append(reasons, ReasonUntrackedTruncated)
	}
	return reasons
}

func largeDiffLimit(options Options) int {
	if options.LargeDiffBytes > 0 {
		return options.LargeDiffBytes
	}
	return DefaultLargeDiffBytes
}

func branchDiffBytes(git gitcontext.Context) int {
	total := 0
	for _, f := range git.BranchFiles {
		total += f.Bytes
	}
	return total
}

func filesFromGit(git gitcontext.Context) []FileProfile {
	byPath := map[string]int{}
	addDiffProfiles(byPath, git.Diff)
	addDiffProfiles(byPath, git.StagedDiff)
	addDiffProfiles(byPath, git.WorkingTreeDiff)
	addUntrackedProfiles(byPath, git.UntrackedExcerpts)
	for _, path := range append(append(append(append([]string{}, git.ChangedFiles...), git.StagedFiles...), git.WorkingTreeFiles...), git.UntrackedFiles...) {
		if strings.TrimSpace(path) != "" {
			byPath[path] += 0
		}
	}
	// Local bytes are accumulated from the local passes alone. Reading them out
	// of byPath does not work: byPath is seeded with git.Diff above, so every
	// branch file would carry the branch diff as its "local" contribution and
	// every clean committed file would be disclosed as locally modified — a
	// disclosure that flags everything discloses nothing.
	localOnly := map[string]int{}
	addDiffProfiles(localOnly, git.StagedDiff)
	addDiffProfiles(localOnly, git.WorkingTreeDiff)
	addUntrackedProfiles(localOnly, git.UntrackedExcerpts)

	branch := map[string]gitcontext.BranchFile{}
	for _, f := range git.BranchFiles {
		branch[f.Path] = f
		byPath[f.Path] = f.Bytes
	}
	files := make([]FileProfile, 0, len(byPath))
	for path, diffBytes := range byPath {
		file := FileProfile{Path: path, DiffBytes: diffBytes, Source: sourceOf(git, path)}
		if b, ok := branch[path]; ok {
			file.Hash = b.Hash
			file.Source = SourceBranch
			file.LocalBytes = localOnly[path]
		} else if file.Source != SourceBranch {
			file.LocalBytes = localOnly[path]
		}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func addDiffProfiles(byPath map[string]int, text string) {
	var currentPath string
	var currentBytes int
	flush := func() {
		if currentPath != "" {
			byPath[currentPath] += currentBytes
		}
		currentPath = ""
		currentBytes = 0
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			currentPath = diffHeaderPath(line)
		}
		if currentPath != "" {
			currentBytes += len(line)
		}
	}
	flush()
}

func diffHeaderPath(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return ""
	}
	return strings.TrimPrefix(fields[3], "b/")
}

func addUntrackedProfiles(byPath map[string]int, text string) {
	var currentPath string
	var currentBytes int
	flush := func() {
		if currentPath != "" {
			byPath[currentPath] += currentBytes
		}
		currentPath = ""
		currentBytes = 0
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(trimmed, "--- ") {
			flush()
			currentPath = strings.TrimSpace(strings.TrimPrefix(trimmed, "--- "))
		}
		if currentPath != "" {
			currentBytes += len(line)
		}
	}
	flush()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func sourceOf(git gitcontext.Context, path string) string {
	for _, p := range git.ChangedFiles {
		if p == path {
			return SourceBranch
		}
	}
	for _, p := range git.StagedFiles {
		if p == path {
			return SourceStaged
		}
	}
	for _, p := range git.UntrackedFiles {
		if p == path {
			return SourceUntracked
		}
	}
	return SourceWorktree
}
