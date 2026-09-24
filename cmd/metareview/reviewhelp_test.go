package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `review <subcommand> --help` prints usage and runs nothing. It once ran a full task-done review
// with "--help" as the target, and committed that review log (target `--help`) to main.
func TestReviewHelpPrintsUsageAndRunsNothing(t *testing.T) {
	root := gitRepo(t)
	for _, args := range [][]string{
		{"review", "--help"},
		{"review", "task-done", "--help"},
		{"review", "task-done", "-h"},
		{"review", "task-done", "docs/tasks/t.md", "--base", "main", "--help"},
		{"review", "epic-ready", "--help"},
		{"review", "pr-ready", "--help"},
		{"review", "artifact", "-h"},
		{"review", "record-lenses", "--help"},
	} {
		code, stdout, stderr := runCLI(t, root, nil, args...)
		if code != 0 || !strings.Contains(stdout, "metareview review task-done <task-id-or-path>") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "metareview", "reviews")); !os.IsNotExist(err) {
		t.Errorf("a help request wrote a review log (stat err %v)", err)
	}
}
