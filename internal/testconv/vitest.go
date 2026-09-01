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
//
// The json reporter prints to STDOUT by default (Vitest docs: "Can either be printed to the terminal or
// written to a file using the outputFile configuration option"), which is what ParseReport reads. If a
// repository hard-wires `outputFile` in its Vitest config, the report goes to that file and stdout is
// empty — ParseReport then finds no report line and fails CLOSED (unverifiable), never silently wrong.
// That is the same misconfiguration edge as Jest's `--outputFile`; such a repo must not redirect the
// json reporter away from stdout in its consent-hashed base command / config.
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
