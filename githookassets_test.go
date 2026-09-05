package metareview

import "testing"

// TestGitHookAssetsEmbedded guards the real historical bug this embed exists to prevent: the installer
// once pointed core.hooksPath at an on-disk hooks/git that is absent in a consumer repo, so the gate was
// silently inert while the CLI reported it "active". The scripts are embedded so the same bytes ship
// through every install path; this test fails loudly if a rename or a moved directory makes any embedded
// path resolve to nothing. (The root package has no executable statements and is excluded from the
// coverage gate — this test is for its own sake, not for coverage.)
func TestGitHookAssetsEmbedded(t *testing.T) {
	for _, name := range []string{
		"hooks/git/pre-push",
		"hooks/git/post-commit",
		"hooks/session-start-check.sh",
	} {
		data, err := GitHookAssets.ReadFile(name)
		if err != nil {
			t.Errorf("embedded asset %q: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("embedded asset %q is empty", name)
		}
	}
}
