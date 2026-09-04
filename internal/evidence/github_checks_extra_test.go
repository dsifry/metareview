package evidence

import (
	"context"
	"testing"
)

// When no Runner is injected, ImportGitHubChecks uses the package default. Swapping the default
// seam lets us exercise that fallback wiring without shelling out to gh.
func TestImportGitHubChecksUsesDefaultRunnerWhenUnset(t *testing.T) {
	original := defaultRunner
	t.Cleanup(func() { defaultRunner = original })
	called := false
	defaultRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		called = true
		return []byte(`[{"name":"go test","bucket":"pass"}]`), nil
	}

	bundle, err := ImportGitHubChecks(context.Background(), "3", GitHubCheckOptions{})
	if err != nil {
		t.Fatalf("import checks: %v", err)
	}
	if !called {
		t.Fatalf("the default runner should have been invoked")
	}
	if !bundle.HasSuccessfulValidation(KindCICheck) {
		t.Fatalf("a passing check via the default runner should validate: %+v", bundle.Receipts)
	}
}

// Malformed JSON from gh is a hard error, not a silently empty bundle.
func TestImportGitHubChecksRejectsMalformedJSON(t *testing.T) {
	_, err := ImportGitHubChecks(context.Background(), "3", GitHubCheckOptions{
		Runner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte(`{not valid json`), nil
		},
	})
	if err == nil {
		t.Fatalf("malformed gh JSON must produce an error")
	}
}

// The real default runner shells out to the named binary and returns its stdout. Exercising it
// against a trivial always-present command keeps the production code path covered without
// depending on gh being installed.
func TestDefaultRunnerExecutesCommand(t *testing.T) {
	out, err := defaultRunner(context.Background(), "printf", "hello")
	if err != nil {
		t.Fatalf("default runner: %v", err)
	}
	if string(out) != "hello" {
		t.Fatalf("expected stdout %q, got %q", "hello", out)
	}
	if _, err := defaultRunner(context.Background(), "metareview-no-such-binary-xyzzy"); err == nil {
		t.Fatalf("a missing binary should surface an error from the default runner")
	}
}
