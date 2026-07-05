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
