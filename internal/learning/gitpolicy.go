package learning

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var learningGitPolicyLines = []string{
	"# metareview: keep ephemeral review state local while syncing durable learning state",
	".metareview/*",
	"!.metareview/knowledge/",
	".metareview/knowledge/*",
	"!.metareview/knowledge/metareview.jsonl",
	"!.metareview/calibration.jsonl",
	"!.metareview/learning-runs.jsonl",
}

func EnsureLearningGitPolicy(root string) error {
	path := filepath.Join(root, ".gitignore")
	bytes, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	lines := retainedGitIgnoreLines(string(bytes))
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}
	lines = append(lines, learningGitPolicyLines...)

	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func retainedGitIgnoreLines(content string) []string {
	policy := map[string]bool{
		".metareview/": true,
	}
	for _, line := range learningGitPolicyLines {
		policy[line] = true
	}

	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	retained := []string{}
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if policy[strings.TrimSpace(line)] {
			continue
		}
		retained = append(retained, line)
	}
	for len(retained) > 0 && strings.TrimSpace(retained[len(retained)-1]) == "" {
		retained = retained[:len(retained)-1]
	}
	return retained
}
