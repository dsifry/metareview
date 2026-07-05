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
	ReasonLargeDiff          = "LARGE_DIFF"
	ReasonUntrackedOmitted   = "UNTRACKED_OMITTED"
	ReasonUntrackedTruncated = "UNTRACKED_TRUNCATED"

	DefaultLargeDiffBytes = 120000
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

func ShardPlanMarkdown(profile Profile, options ShardOptions) string {
	if profile.RiskLevel != RiskContextRisk {
		return ""
	}
	plan, err := PlanShards(profile, options)
	if err != nil {
		return "## Context Shard Plan\n\nUnable to generate shard plan: " + err.Error()
	}
	if len(plan.Shards) == 0 {
		return "## Context Shard Plan\n\nNo shardable source paths were detected."
	}
	lines := []string{
		"## Context Shard Plan",
		"",
		"- Source diff hash: `" + plan.SourceDiffHash + "`",
	}
	for _, shard := range plan.Shards {
		lines = append(lines, "- "+shard.ID+": "+strings.Join(shard.Paths, ", ")+" ("+fmt.Sprint(shard.ByteCount)+" bytes, prompt pack `"+shard.PromptPackPath+"`)")
	}
	return strings.Join(lines, "\n")
}

func riskReasons(git gitcontext.Context, filteredDiffBytes int, options Options) []string {
	var reasons []string
	if git.DiffTruncated || git.StagedDiffTruncated || git.WorkingTreeDiffTruncated {
		reasons = append(reasons, ReasonDiffTruncated)
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
	files := make([]FileProfile, 0, len(byPath))
	for path, diffBytes := range byPath {
		files = append(files, FileProfile{Path: path, DiffBytes: diffBytes})
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
