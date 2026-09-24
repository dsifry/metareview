package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Spec §11.3: --mutation-view is repeatable, and a name that is invalid, has no report to scope, or
// is missing from an attested report's view map is a usage error (exit 2) on every gate.
func TestMutationViewUsageErrors(t *testing.T) {
	root := gitRepo(t)
	report, err := filepath.Abs("../../testdata/mutation-incremental/real/full/incremental.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, gate := range [][]string{
		{"review", "task-done", "docs/tasks/t.md", "--base", "main"},
		{"review", "epic-ready", "docs/tasks/t.md", "--base", "main"},
		{"review", "pr-ready", "--base", "main"},
	} {
		for _, c := range []struct {
			args []string
			want string
		}{
			{[]string{"--mutation-view", "core"}, "--mutation-view needs --mutation-report"},
			{[]string{"--mutation-report", report, "--mutation-view", "core", "--mutation-view", "nope"}, `mutation view "nope" is not in the views of`},
			{[]string{"--mutation-view", "bad name", "--mutation-report", report}, `invalid mutation view name "bad name"`},
		} {
			args := append(append([]string(nil), gate...), c.args...)
			code, _, stderr := runCLI(t, root, nil, args...)
			if code != 2 || !strings.Contains(stderr, c.want) {
				t.Errorf("%v: exit %d, stderr %q", args, code, stderr)
			}
		}
	}
}

// A repeated --mutation-view name is one view.
func TestAppendUniqueCollapsesRepeatedViews(t *testing.T) {
	if got := appendUnique(appendUnique([]string{"core"}, "edge"), "core"); strings.Join(got, ",") != "core,edge" {
		t.Errorf("got %v", got)
	}
}
