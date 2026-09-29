package evidence

import "testing"

// mr-r3y: freeform evidence reads a failure from tool-output shapes and from a stated failed check — never from the
// word "fail" in prose. Before the fix, "the new tests fail against origin/main" (the tests CATCH the regression) marked
// passing evidence exit 1.
func TestFreeformFailureNeedsAFailureShapeNotTheWord(t *testing.T) {
	const passing = "go test ./... exited 0\n"
	for name, tc := range map[string]struct {
		text string
		want int
	}{
		// prose: not failures
		"the regression tests fail on the old code": {passing + "- the new tests fail against origin/main and pass here", 0},
		"fails, failing, failure mode":                {passing + "- covers the failure mode where the gate fails open; a failing case is pinned", 0},
		"a test that failed before the fix":           {passing + "- TestX failed before the fix and passes now", 0},
		"zero failed":                                 {passing + "Tests: 12 passed, 0 failed", 0},
		// tool output and stated failures: failures
		"go test FAIL line":     {"ok  \tgithub.com/a\t0.1s\nFAIL\tgithub.com/b\t0.2s", 1},
		"go test --- FAIL":      {passing + "--- FAIL: TestY (0.00s)", 1},
		"pytest FAILED":         {passing + "FAILED tests/test_x.py::test_y - assert 1 == 2", 1},
		"a failure count":       {passing + "=== 2 failed, 10 passed in 0.3s ===", 1},
		"jest summary":          {passing + "Tests:       1 failed, 3 passed, 4 total", 1},
		"junit failures":        {passing + "Tests run: 5, Failures: 1, Errors: 0", 1},
		"a nonzero exit":        {passing + "golangci-lint run ./... exited 1", 1},
		"an error line":         {passing + "error: cannot find package", 1},
		"a stated failed check": {passing + "- lint failed", 1},
		"tests failed":          {"go test ./... ran; tests failed", 1},
		"build has failed":      {passing + "the build has failed on CI", 1},
		"no success signal":     {"I looked at it", 1},
	} {
		bundle, err := Parse([]byte(tc.text))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := bundle.Receipts[0].ExitCode; got != tc.want {
			t.Errorf("%s: exit %d, want %d", name, got, tc.want)
		}
	}
}
