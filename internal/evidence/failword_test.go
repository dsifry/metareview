package evidence

import "testing"

// mr-r3y: freeform evidence fails closed — any "failed", and the shapes tools print, count as a failure — except the
// word "fail" in prose and reports that nothing failed. Before the fix, "the new tests fail against origin/main" (the
// tests CATCH the regression) marked passing evidence exit 1; the first cut of the fix narrowed "failed" and let real
// failure output through (#-review), so the default stays closed.
func TestFreeformFailureReadsFailuresNotTheWordFail(t *testing.T) {
	const passing = "go test ./... exited 0\n"
	for name, tc := range map[string]struct {
		text string
		want int
	}{
		// not failures: the word "fail" in prose, and reports that nothing failed
		"the regression tests fail on the old code": {passing + "- the new tests fail against origin/main and pass here", 0},
		"fails, failing, failure mode":              {passing + "- covers the failure mode where the gate fails open; a failing case is pinned", 0},
		"a Fail-safe":                               {passing + "- the Fail-safe path is exercised", 0},
		"zero failed":                               {passing + "Tests: 12 passed, 0 failed", 0},
		"no tests failed":                           {passing + "no tests failed", 0},
		"none of the checks failed":                 {passing + "none of the checks failed", 0},
		"0 of 10 failed":                            {passing + "0 of 10 failed", 0},
		"Failures: 0":                               {passing + "Tests run: 5, Failures: 0, Errors: 0", 0},
		"0 failing, 0 errors":                       {passing + "12 passing, 0 failing, 0 errors", 0},
		// failures: any "failed", and tool-output shapes
		"go test FAIL line":           {"ok  \tgithub.com/a\t0.1s\nFAIL\tgithub.com/b\t0.2s", 1},
		"go test --- FAIL":            {passing + "--- FAIL: TestY (0.00s)", 1},
		"pytest FAILED":               {passing + "FAILED tests/test_x.py::test_y - assert 1 == 2", 1},
		"a failure count":             {passing + "=== 2 failed, 10 passed in 0.3s ===", 1},
		"jest summary":                {passing + "Tests:       1 failed, 3 passed, 4 total", 1},
		"junit failures":              {passing + "Tests run: 5, Failures: 1, Errors: 0", 1},
		"dotnet":                      {passing + "Failed!  - Failed: 1, Passed: 5", 1},
		"test(s) failed":              {passing + "3 test(s) failed", 1},
		"go vet failed":               {passing + "go vet failed", 1},
		"failed to compile":           {passing + "Failed to compile.", 1},
		"command failed":              {passing + "Command failed.", 1},
		"a test that failed":          {passing + "TestFoo failed", 1},
		"a zero beside a real one":    {passing + "0 failed in unit, 2 failed in integration", 1},
		"mocha failing":               {passing + "3 passing\n1 failing", 1},
		"rspec failure":               {passing + "10 examples, 1 failure", 1},
		"TAP not ok":                  {passing + "ok 1 - a\nnot ok 2 - b", 1},
		"make Error":                  {passing + "make: *** [test] Error 2", 1},
		"tsc":                         {passing + "src/a.ts(1,1): error TS2322: Type 'x' is not assignable", 1},
		"exit status":                 {passing + "exit status 1", 1},
		"a nonzero exit":              {passing + "golangci-lint run ./... exited 1", 1},
		"an error line":               {passing + "error: cannot find package", 1},
		"a failure with no success":   {"FAIL\tgithub.com/b\t0.2s", 1},
		"no success signal":           {"I looked at it", 1},
		"a failure beats a success":   {"tests passed\nlint failed", 1},
		"only the success, no report": {passing, 0},
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
