package testconv

import "regexp"

func init() { register(vitestConvention{}) }

// vitestConvention supports TS/JS repositories driven by VITEST. It shares everything with the Jest
// convention except the runner invocation: it embeds typeScriptConvention (so IsTestFile, DirHasTests,
// DeletesATest, and SemanticallyNull are identical) and overrides only the flags and the report-name.
//
// Vitest's results reporter is `--reporter=json` (NOT Jest's bare `--json`), and its output is a
// Jest-compatible aggregated result — the same `numTotalTestSuites` / `testResults` / `assertionResults`
// shape, a simplified version of Jest's — so the shared parseJSCompatReport reads it directly
// (verified against Vitest's reporter docs, 2026-09-01). The base command must be a one-shot run
// (`vitest run`, or CI mode) — the convention does not add a subcommand, matching how the Jest
// convention leaves non-interactive invocation to the consent-hashed base command.
type vitestConvention struct{ typeScriptConvention }

func (vitestConvention) Name() string { return "vitest" }

// RunArgs narrows the base Vitest command to the single named test and asks for the JSON reporter. The
// test identity is the full name, matched by Vitest's `--testNamePattern`, anchored and regexp-quoted.
func (vitestConvention) RunArgs(base []string, test string) []string {
	return append(append([]string(nil), base...), "--testNamePattern", "^"+regexp.QuoteMeta(test)+"$", "--reporter=json")
}

// SuiteArgs runs the whole suite (no name filter) with Vitest's JSON reporter.
func (vitestConvention) SuiteArgs(base []string) []string {
	return append(append([]string(nil), base...), "--reporter=json")
}

// ParseReport reads Vitest's Jest-compatible json report through the shared parser.
func (vitestConvention) ParseReport(code int, stdout, stderr string) (TestReport, error) {
	return parseJSCompatReport("vitest", code, stdout)
}
