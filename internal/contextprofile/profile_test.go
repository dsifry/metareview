package contextprofile

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/gitcontext"
)

func TestGeneratedMetareviewArtifactsDoNotConsumeSourceBudget(t *testing.T) {
	profile := FromGit(gitcontext.Context{
		RawDiffBytes:           200_000,
		FilteredDiffBytes:      512,
		GeneratedExcludedFiles: []string{"docs/metareview/reviews/generated.md"},
		Diff:                   sampleDiff("internal/reviewer/review.go", "+func realChange() {}\n"),
	}, Options{LargeDiffBytes: 1_000})

	if profile.RawDiffBytes != 200_000 {
		t.Fatalf("RawDiffBytes = %d, want raw generated-heavy budget", profile.RawDiffBytes)
	}
	if profile.FilteredDiffBytes != 512 {
		t.Fatalf("FilteredDiffBytes = %d, want source-only filtered budget", profile.FilteredDiffBytes)
	}
	if profile.RiskLevel != RiskNone {
		t.Fatalf("RiskLevel = %q, want %q; reasons=%v", profile.RiskLevel, RiskNone, profile.RiskReasons)
	}
	if !contains(profile.GeneratedExcludedFiles, "docs/metareview/reviews/generated.md") {
		t.Fatalf("GeneratedExcludedFiles = %#v, want generated artifact recorded", profile.GeneratedExcludedFiles)
	}
}

func TestLargeSourceDiffProducesContextRisk(t *testing.T) {
	profile := FromGit(gitcontext.Context{
		RawDiffBytes:      1_600,
		FilteredDiffBytes: 1_600,
		Diff:              sampleDiff("internal/reviewer/review.go", "+"+strings.Repeat("x", 1_200)+"\n"),
	}, Options{LargeDiffBytes: 1_000})

	if profile.RiskLevel != RiskContextRisk {
		t.Fatalf("RiskLevel = %q, want %q", profile.RiskLevel, RiskContextRisk)
	}
	if !contains(profile.RiskReasons, ReasonLargeDiff) {
		t.Fatalf("RiskReasons = %#v, want %s", profile.RiskReasons, ReasonLargeDiff)
	}
}

func TestOmittedUntrackedFilesProduceContextRisk(t *testing.T) {
	profile := FromGit(gitcontext.Context{
		FilteredDiffBytes:     400,
		UntrackedOmittedCount: 3,
	}, Options{LargeDiffBytes: 1_000})

	if profile.RiskLevel != RiskContextRisk {
		t.Fatalf("RiskLevel = %q, want %q", profile.RiskLevel, RiskContextRisk)
	}
	if !contains(profile.RiskReasons, ReasonUntrackedOmitted) {
		t.Fatalf("RiskReasons = %#v, want %s", profile.RiskReasons, ReasonUntrackedOmitted)
	}
}

func TestTruncatedUntrackedFileProducesContextRisk(t *testing.T) {
	profile := FromGit(gitcontext.Context{
		FilteredDiffBytes:       400,
		UntrackedTruncatedCount: 1,
	}, Options{LargeDiffBytes: 1_000})

	if profile.RiskLevel != RiskContextRisk {
		t.Fatalf("RiskLevel = %q, want %q", profile.RiskLevel, RiskContextRisk)
	}
	if !contains(profile.RiskReasons, ReasonUntrackedTruncated) {
		t.Fatalf("RiskReasons = %#v, want %s", profile.RiskReasons, ReasonUntrackedTruncated)
	}
}

func sampleDiff(path, body string) string {
	return "diff --git a/" + path + " b/" + path + "\n" +
		"index 0000000..1111111 100644\n" +
		"--- a/" + path + "\n" +
		"+++ b/" + path + "\n" +
		"@@ -1 +1 @@\n" +
		body
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// A file can be both committed on the branch and dirty in the working tree. The
// committed bytes are in a shard pack; the uncommitted ones are in no pack at
// all, so the manifest has to disclose them. Assigning Source=SourceBranch and
// overwriting the byte count with the branch total dropped that second fact:
// the file never appeared in LocalPaths and the per-file table stopped summing
// to FilteredDiffBytes, so unreviewed bytes were silently undisclosed.
func TestBranchFileWithLocalEditsKeepsItsLocalBytes(t *testing.T) {
	git := gitcontext.Context{
		ChangedFiles:     []string{"src/both.go"},
		WorkingTreeFiles: []string{"src/both.go"},
		BranchFiles:      []gitcontext.BranchFile{{Path: "src/both.go", Bytes: 400, Hash: "h1"}},
		WorkingTreeDiff: "diff --git a/src/both.go b/src/both.go\n" +
			"--- a/src/both.go\n+++ b/src/both.go\n@@ -1 +1,2 @@\n" +
			"+uncommitted line one\n+uncommitted line two\n",
	}

	files := filesFromGit(git)
	var profile *FileProfile
	for i := range files {
		if files[i].Path == "src/both.go" {
			profile = &files[i]
		}
	}
	if profile == nil {
		t.Fatal("the file is missing from the profile entirely")
	}
	if profile.Source != SourceBranch {
		t.Fatalf("a branch-committed file is still a branch file: %q", profile.Source)
	}
	if profile.LocalBytes <= 0 {
		t.Fatalf("the uncommitted bytes were dropped: LocalBytes=%d (they are in no shard pack, so they must be disclosed)", profile.LocalBytes)
	}
}

// The fixture that matters: a branch diff IS present. Reading the local
// contribution out of the shared byPath map picked up those branch bytes, so
// every clean committed file reported LocalBytes > 0 and was disclosed as
// locally modified. The first attempt at this fix missed it because its fixture
// left git.Diff empty — the one input that made the bug visible.
func TestCleanBranchFileHasNoLocalBytes(t *testing.T) {
	git := gitcontext.Context{
		ChangedFiles: []string{"src/clean.go", "src/dirty.go"},
		BranchFiles: []gitcontext.BranchFile{
			{Path: "src/clean.go", Bytes: 400, Hash: "h1"},
			{Path: "src/dirty.go", Bytes: 500, Hash: "h2"},
		},
		Diff: "diff --git a/src/clean.go b/src/clean.go\n--- a/src/clean.go\n+++ b/src/clean.go\n" +
			"@@ -1 +1,2 @@\n+committed line one\n+committed line two\n" +
			"diff --git a/src/dirty.go b/src/dirty.go\n--- a/src/dirty.go\n+++ b/src/dirty.go\n" +
			"@@ -1 +1,2 @@\n+committed change\n",
		WorkingTreeFiles: []string{"src/dirty.go"},
		WorkingTreeDiff: "diff --git a/src/dirty.go b/src/dirty.go\n--- a/src/dirty.go\n+++ b/src/dirty.go\n" +
			"@@ -1 +1,2 @@\n+uncommitted line\n",
	}

	got := map[string]FileProfile{}
	for _, f := range filesFromGit(git) {
		got[f.Path] = f
	}
	if clean := got["src/clean.go"]; clean.LocalBytes != 0 {
		t.Fatalf("a committed file with no local edits must report no local bytes, got %d", clean.LocalBytes)
	}
	if dirty := got["src/dirty.go"]; dirty.LocalBytes <= 0 {
		t.Fatalf("a committed file that is also dirty must report its uncommitted bytes, got %d", dirty.LocalBytes)
	}
}
