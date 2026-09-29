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
		// not failures: the word "fail" in prose, and a bounded clause reporting that nothing failed
		"the regression tests fail on the old code": {passing + "- the new tests fail against origin/main and pass here", 0},
		"two tests fail on the old code":            {passing + "- the 2 tests fail on origin/main", 0},
		"fails, failing, failure mode":              {passing + "- covers the failure mode where the gate fails open; a failing case is pinned", 0},
		"a Fail-safe":                               {passing + "- the Fail-safe path is exercised", 0},
		"zero failed":                               {passing + "Tests: 12 passed, 0 failed", 0},
		"no tests failed":                           {passing + "no tests failed", 0},
		"0 tests failed.":                           {passing + "0 tests failed.", 0},
		"none of the checks failed":                 {passing + "none of the checks failed", 0},
		"0 of 10 failed":                            {passing + "0 of 10 failed", 0},
		"0/10 failed":                               {passing + "0/10 failed", 0},
		"Failures: 0, Errors: 0":                    {passing + "Tests run: 5, Failures: 0, Errors: 0", 0},
		"0 failing, 0 errors":                       {passing + "12 passing, 0 failing, 0 errors", 0},
		"0 failed in 1s":                            {passing + "5 passed, 0 failed in 1.2s", 0},
		"0 failed (cached)":                         {passing + "5 passed, 0 failed (cached)", 0},
		"dotnet all passed":                         {passing + "Passed!  - Failed: 0, Passed: 5", 0},
		"ansible failed=0":                          {passing + "ok=5 changed=0 failed=0", 0},
		"CRLF zero":                                 {passing + "Tests: 3 passed, 0 failed\r\nexit 0 ok", 0},
		"error cases in prose":                      {passing + "- covers 2 error paths; added 2 error cases", 0},
		"fail2ban":                                  {passing + "- fail2ban config untouched", 0},
		"a test named should fail":                  {"✓ should fail (3 ms)\nTests: 5 passed, 5 total", 0},
		"a go subtest named fail":                   {"--- PASS: TestX/fail (0.00s)\nPASS\nok  \tpkg\t0.1s", 0},
		"ansible recap":                             {passing + "ok=5 changed=0 unreachable=0 failed=0 skipped=0", 0},
		// failures: any "failed", the shapes tools print, and a zero that sits beside a real count
		"go test FAIL line":           {"ok  \tgithub.com/a\t0.1s\nFAIL\tgithub.com/b\t0.2s", 1},
		"go test --- FAIL":            {passing + "--- FAIL: TestY (0.00s)", 1},
		"pytest FAILED":               {passing + "FAILED tests/test_x.py::test_y - assert 1 == 2", 1},
		"a failure count":             {passing + "=== 2 failed, 10 passed in 0.3s ===", 1},
		"jest summary":                {passing + "Tests:       1 failed, 3 passed, 4 total", 1},
		"junit failures":              {passing + "Tests run: 5, Failures: 1, Errors: 0", 1},
		"maven errors":                {passing + "Tests run: 5, Failures: 0, Errors: 1", 1},
		"BUILD FAILURE":               {passing + "[INFO] BUILD FAILURE", 1},
		"FAILURES!":                   {passing + "FAILURES!", 1},
		"dotnet":                      {passing + "Failed!  - Failed: 1, Passed: 5", 1},
		"label then count":            {passing + "Passed: 0 Failed: 3", 1},
		"zero passed then failed":     {passing + "0 passed 3 failed", 1},
		"zero skipped then failed":    {passing + "Tests: 0 skipped 3 failed", 1},
		"zero failed then failed":     {passing + "Tests: 0 failed 1 failed", 1},
		"a version before failed":     {passing + "go 1.0 failed", 1},
		"exited 0 build failed":       {"exited 0 build failed", 1},
		"test(s) failed":              {passing + "3 test(s) failed", 1},
		"go vet failed":               {passing + "go vet failed", 1},
		"failed to compile":           {passing + "Failed to compile.", 1},
		"command failed":              {passing + "Command failed.", 1},
		"a test that failed":          {passing + "TestFoo failed", 1},
		"no tests ran, one failed":    {passing + "no tests skipped; 1 failed", 1},
		"a zero beside a real one":    {passing + "0 failed in unit, 2 failed in integration", 1},
		"mocha failing":               {passing + "3 passing\n1 failing", 1},
		"tests failing":               {passing + "2 tests failing", 1},
		"rspec failure":               {passing + "10 examples, 1 failure", 1},
		"node test fail":              {passing + "# tests 5\n# pass 4\n# fail 1", 1},
		"node test info fail":         {passing + "ℹ fail 1", 1},
		"bun fail":                    {passing + "5 pass\n1 fail", 1},
		"TAP not ok":                  {passing + "ok 1 - a\nnot ok 2 - b", 1},
		"make Error":                  {passing + "make: *** [test] Error 2", 1},
		"tsc":                         {passing + "src/a.ts(1,1): error TS2322: Type 'x' is not assignable", 1},
		"msbuild":                     {passing + "Program.cs(3,1): error CS1002: ; expected", 1},
		"npm ERR":                     {passing + "npm ERR! code 1", 1},
		"exit status":                 {passing + "exit status 1", 1},
		"a nonzero exit":              {passing + "golangci-lint run ./... exited 1", 1},
		"an error line":               {passing + "error: cannot find package", 1},
		"a failure with no success":   {"FAIL\tgithub.com/b\t0.2s", 1},
		"no success signal":           {"I looked at it", 1},
		"a failure beats a success":   {"tests passed\nlint failed", 1},
		"only the success, no report": {passing, 0},
		"pytest error summary":        {passing + "=========== 5 passed, 1 error in 0.52s ===========", 1},
		"pytest error during":         {passing + "!!! Interrupted: 1 error during collection !!!", 1},
		"pytest ERROR line":           {passing + "ERROR tests/test_a.py - ImportError", 1},
		"mypy":                        {passing + "Found 2 errors in 1 file (checked 3 source files)", 1},
		"clang":                       {passing + "1 error generated.", 1},
		"rustc":                       {passing + "error[E0308]: mismatched types", 1},
		"segfault":                    {passing + "Segmentation fault (core dumped)", 1},
		"killed":                      {passing + "Killed", 1},
		"traceback":                   {passing + "Traceback (most recent call last):", 1},
		"negative exit":               {passing + "exit code: -1", 1},
		"rc=1":                        {passing + "rc=1", 1},
		"an error starting with 0":    {passing + "Error: 0 is not a valid port", 1},

		"a fail verdict":         {passing + "Result: Fail", 1},
		"a lower-case status":    {passing + `{"status":"fail"}`, 1},
		"build fail at line end": {passing + "build fail", 1},
		"shard 0 failed":         {passing + "shard 0 failed", 1},
		"worker 0 failed.":       {passing + "worker 0 failed.", 1},
		"exit code 0 failed":     {"tests passed\nexit code 0 failed", 1},
		"exit code: 2":           {passing + "exit code: 2", 1},
		"exited with code":       {passing + "Process exited with code 1", 1},
		"exit=1":                 {passing + "exit=1", 1},
		"return code":            {passing + "return code: 3", 1},
		"found errors":           {passing + "Found 2 errors.", 1},
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
