package learning

import (
	"strconv"
	"strings"

	"github.com/dsifry/metareview/internal/markdown"
)

const maxLearningChangedFiles = 12

func learningChangedFilesMarkdown(files []string) string {
	filtered := nonGeneratedChangedFiles(files)
	if len(filtered) == 0 {
		return "No non-generated changed files discovered.\n"
	}

	omitted := 0
	if len(filtered) > maxLearningChangedFiles {
		omitted = len(filtered) - maxLearningChangedFiles
		filtered = filtered[:maxLearningChangedFiles]
	}

	var builder strings.Builder
	for _, file := range filtered {
		builder.WriteString("- " + markdown.InlineCode(file) + "\n")
	}
	if omitted > 0 {
		builder.WriteString("- ... " + strconv.Itoa(omitted) + " more changed files omitted\n")
	}
	return builder.String()
}

func nonGeneratedChangedFiles(files []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, file := range files {
		file = strings.TrimSpace(strings.ReplaceAll(file, "\\", "/"))
		if file == "" || seen[file] || isGeneratedMetareviewPath(file) {
			continue
		}
		seen[file] = true
		result = append(result, file)
	}
	return result
}

func isGeneratedMetareviewPath(path string) bool {
	generatedPrefixes := []string{
		".metareview/",
		"docs/metareview/context/",
		"docs/metareview/learning/",
		"docs/metareview/reviews/",
	}
	for _, prefix := range generatedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
