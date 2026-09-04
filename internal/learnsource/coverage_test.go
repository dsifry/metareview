package learnsource

import (
	"errors"
	"testing"

	"github.com/dsifry/metareview/internal/gitcontext"
	"github.com/dsifry/metareview/internal/githubcontext"
	"github.com/dsifry/metareview/internal/knowledge"
	"github.com/dsifry/metareview/internal/reviewlog"
)

// Collect fans out to four context collectors; if any one fails, the whole collection must abort
// with that error rather than returning a partial Context. Each subtest fails exactly one collector
// (leaving the other three at their real implementations) and asserts the failure propagates.
func TestCollectPropagatesCollectorFailures(t *testing.T) {
	sentinel := errors.New("collector boom")

	realDiscover, realGit, realGitHub, realKnowledge := discoverLogs, collectGit, collectGitHub, collectKnowledge
	t.Cleanup(func() {
		discoverLogs, collectGit, collectGitHub, collectKnowledge = realDiscover, realGit, realGitHub, realKnowledge
	})

	// baseline sets every collector to a succeeding stub so a subtest can break exactly one in
	// isolation — otherwise an earlier real collector (e.g. git on a non-repo tempdir) would fail
	// first and mask the branch under test.
	baseline := func() {
		discoverLogs = func(string) ([]reviewlog.Summary, error) { return nil, nil }
		collectGit = func(string, string, []string) (gitcontext.Context, error) { return gitcontext.Context{}, nil }
		collectGitHub = func(string, string) (githubcontext.Context, error) { return githubcontext.Context{}, nil }
		collectKnowledge = func(string) (knowledge.Context, error) { return knowledge.Context{}, nil }
	}

	cases := []struct {
		name    string
		breakIt func()
	}{
		{"review-log discovery", func() {
			discoverLogs = func(string) ([]reviewlog.Summary, error) { return nil, sentinel }
		}},
		{"git collection", func() {
			collectGit = func(string, string, []string) (gitcontext.Context, error) {
				return gitcontext.Context{}, sentinel
			}
		}},
		{"github collection", func() {
			collectGitHub = func(string, string) (githubcontext.Context, error) {
				return githubcontext.Context{}, sentinel
			}
		}},
		{"knowledge collection", func() {
			collectKnowledge = func(string) (knowledge.Context, error) { return knowledge.Context{}, sentinel }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseline()
			tc.breakIt()
			_, err := Collect(t.TempDir(), Options{})
			if !errors.Is(err, sentinel) {
				t.Fatalf("expected the %s failure to propagate, got %v", tc.name, err)
			}
		})
	}

	// With every collector succeeding, Collect assembles and returns a Context with no error.
	baseline()
	if _, err := Collect(t.TempDir(), Options{}); err != nil {
		t.Fatalf("all-success collection should not error: %v", err)
	}
}

// unresolvedFindings collects the finding IDs of only those review logs that still carry unresolved
// blockers, de-duplicating across logs and preserving first-seen order.
func TestUnresolvedFindingsDedupesAndSkipsResolvedLogs(t *testing.T) {
	logs := []reviewlog.Summary{
		{HasUnresolvedBlockers: true, FindingIDs: []string{"a", "b", "a"}},
		{HasUnresolvedBlockers: false, FindingIDs: []string{"c"}},     // resolved log — its IDs are ignored
		{HasUnresolvedBlockers: true, FindingIDs: []string{"b", "d"}}, // b already seen, d is new
	}
	got := unresolvedFindings(logs)
	want := []string{"a", "b", "d"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
