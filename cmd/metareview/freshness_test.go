package main

import (
	"strings"
	"testing"
)

// Spec §6.4: an invalid METAREVIEW_MUTATION_FRESHNESS stops every gate before it runs.
func TestAnInvalidFreshnessModeIsAUsageError(t *testing.T) {
	root := gitRepo(t)
	t.Setenv("METAREVIEW_MUTATION_FRESHNESS", "strict")
	for _, args := range [][]string{
		{"review", "task-done", "docs/tasks/t.md", "--base", "main"},
		{"review", "epic-ready", "docs/tasks/t.md", "--base", "main"},
		{"review", "pr-ready", "--base", "main"},
	} {
		code, _, stderr := runCLI(t, root, nil, args...)
		if code != 2 || !strings.Contains(stderr, "METAREVIEW_MUTATION_FRESHNESS") {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
}
