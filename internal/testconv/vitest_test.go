package testconv

import "testing"

func TestVitestRunArgs(t *testing.T) {
	c := vitestConvention{}
	run := c.RunArgs([]string{"npx", "vitest", "run"}, "Calculator adds")
	if got := run[len(run)-3:]; got[0] != "--testNamePattern" || got[1] != `^Calculator adds$` || got[2] != "--reporter=json" {
		t.Fatalf("Vitest RunArgs must use --testNamePattern and --reporter=json, got %v", run)
	}
	base := []string{"npx", "vitest", "run"}
	_ = c.RunArgs(base, "x")
	if len(base) != 3 {
		t.Fatal("RunArgs must not mutate the base command")
	}
	suite := c.SuiteArgs([]string{"npx", "vitest", "run"})
	if suite[len(suite)-1] != "--reporter=json" || len(suite) != 4 {
		t.Fatalf("Vitest SuiteArgs must append only --reporter=json, got %v", suite)
	}
}

// Vitest shares IsTestFile/DirHasTests/DeletesATest/SemanticallyNull with the TS convention (embedded);
// a spot check confirms the promotion.
func TestVitestSharesTSPredicates(t *testing.T) {
	c := vitestConvention{}
	if !c.IsTestFile("a/b.spec.ts") || c.IsTestFile("a/b.ts") {
		t.Fatal("Vitest must inherit the TS test-file rule")
	}
	if c.SemanticallyNull("x", "x") {
		t.Fatal("Vitest must inherit the dependency-free (false) trivial-pin pre-screen")
	}
}

func TestVitestParseReport(t *testing.T) {
	c := vitestConvention{}
	must := func(out string) TestReport {
		r, err := c.ParseReport(1, out, "")
		if err != nil {
			t.Fatalf("ParseReport error: %v", err)
		}
		return r
	}
	// Vitest's Jest-compatible json (note: its simplified format may OMIT numRuntimeErrorTestSuites — the
	// failed-suite-with-no-assertions fallback is what catches a transpile failure there).
	vitestPass := `{"numTotalTestSuites":1,"testResults":[{"status":"passed","assertionResults":[{"fullName":"Calculator adds","status":"passed"}]}]}`
	if Classify(must(vitestPass), "Calculator adds") != ClsPassed {
		t.Fatal("a Vitest passed assertion -> ClsPassed")
	}
	vitestFail := `{"numTotalTestSuites":1,"testResults":[{"status":"failed","assertionResults":[{"ancestorTitles":["Calculator"],"title":"adds","status":"failed"}]}]}`
	if Classify(must(vitestFail), "Calculator adds") != ClsAssert {
		t.Fatal("a Vitest failed assertion -> ClsAssert")
	}
	// A suite that failed to load with no assertions -> BuildFailed even without numRuntimeErrorTestSuites.
	vitestTranspile := `{"numTotalTestSuites":1,"testResults":[{"status":"failed","message":"Error: cannot resolve","assertionResults":[]}]}`
	if Classify(must(vitestTranspile), "Calculator adds") != ClsCompile {
		t.Fatal("a Vitest failed suite with no assertions -> ClsCompile")
	}
	// Same fail-closed contract as Jest: no report line is an error.
	if _, err := c.ParseReport(1, "no report here\n", ""); err == nil {
		t.Fatal("output with no report line must be an error")
	}
}
