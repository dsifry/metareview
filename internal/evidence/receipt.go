package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/jsonl"
)

type Kind string

const (
	KindGeneric   Kind = "generic"
	KindTests     Kind = "tests"
	KindBuild     Kind = "build"
	KindTypecheck Kind = "typecheck"
	KindCoverage  Kind = "coverage"
	KindCICheck   Kind = "ci-check"
)

const (
	ReceiptKindValidation = "validation"
	ReceiptKindRuntime    = "runtime"
	ReceiptKindCICheck    = "ci-check"
)

type Receipt struct {
	SchemaVersion int       `json:"schemaVersion"`
	Kind          string    `json:"kind"`
	Command       []string  `json:"command,omitempty"`
	CWD           string    `json:"cwd,omitempty"`
	ExitCode      int       `json:"exitCode"`
	StartedAt     time.Time `json:"startedAt,omitempty"`
	FinishedAt    time.Time `json:"finishedAt,omitempty"`
	StdoutSHA256  string    `json:"stdoutSha256,omitempty"`
	StderrSHA256  string    `json:"stderrSha256,omitempty"`
	Summary       string    `json:"summary"`
	Covers        []string  `json:"covers,omitempty"`
}

type Bundle struct {
	Receipts     []Receipt
	FreeformText string
	Fallback     bool
}

type ParseOptions struct {
	Strict bool
	Now    time.Time
	MaxAge time.Duration
}

// modalVerb gates a hypothetical or negated "fail" ("should fail", "doesn't fail"). One source behind both
// proseFail and modalFail, so the two lists can never drift apart (mr-b08).
const modalVerb = `should|shall|will|would|must|can|could|may|might|expected to|not|doesn't|don't|didn't|won't|cannot|never`

// shellTag is the shell name (or *.sh/*.bash/*.zsh script) a shell prints before its message, shared by the
// shell-shape patterns below (mr-b08) so the alternatives cannot drift between them.
const shellTag = `(?:[^\s:]*/)?-?(?:bash|sh|dash|zsh|ash|ksh)|\S+\.(?:sh|bash|zsh)`

var (
	successPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^ok\s+\S+`),
		regexp.MustCompile(`(?i)\b(go test|tests?|test suite).*\b(pass|passed|ok|exited 0)\b`),
		regexp.MustCompile(`(?i)\b(npm run build|build|tsc --noEmit|typecheck|coverage).*\b(pass|passed|ok|success|exited 0)\b`),
		regexp.MustCompile(`(?i)\bexited 0\b`),
	}
	// failurePatterns read a failure fail-closed: any "fail"/"failed" (as the base reader's (?i)\bFAIL\b did), and the
	// shapes tools print, count. hasFailureSignal first strips ANSI escapes and neutralizes zero reports (zeroClause,
	// zeroLabel); then a counted "fail" (countFail, a modal "1 should fail" aside) counts; then "did/does fail" becomes "failed" (reportedFail); and only
	// then is prose "fail" neutralized (proseFail, mr-r3y) — that order is what keeps the exemption from hiding a count
	// or a report.
	failurePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(exit(ed)?|exit[ _-]?code|exit[ _-]?status|return[ _-]?code|exited with( code| status)?|exit (code|status) (was|is))\s*[:=]?\s*-?[1-9][0-9]*\b|\brc[ \t]*[:=][ \t]*-?[1-9]`),
		regexp.MustCompile(`(?mi)"exit_?code"[ \t]*[:=][ \t]*"?-?[1-9]`),                                                                    // a quoted JSON/RPC key anywhere on a line
		regexp.MustCompile(`\bFAIL(URES?)?\b`),                                                                                              // FAIL, BUILD FAILURE, FAILURES! (upper case)
		regexp.MustCompile(`(?im)(^|[^/\w])fail($|[^-.\w]|-($|\W)|\.($|\W))`),                                                               // "fail" in any case — not a path segment (TestX/fail), file (fail.test.ts) or compound (Fail-safe); prose is neutralized first (proseFail)
		regexp.MustCompile(`(?i)\bfailed\b`),                                                                                                // any form: "Failed: 1", "Command failed.", "go vet failed"
		regexp.MustCompile(`(?i)\b(failures?|errors?)[ \t]*[:=][ \t]*[1-9]`),                                                                // junit/maven "Failures: 1", "Errors: 2"
		regexp.MustCompile(`(?i)\b[1-9][0-9]*[ \t]+(\w+[ \t]+)?(failing|failures?)\b`),                                                      // mocha "1 failing", "2 tests failing", "1 failure"
		regexp.MustCompile(`(?im)\b[1-9][0-9]*[ \t]+errors?([ \t]*([.,;:)]|\r?$)|[ \t]+(in|during|generated|found|occurred|and)\b)`),        // "1 error in 0.1s", "2 errors and 1 warning" — not "2 error paths"
		regexp.MustCompile(`(?m)^(ERROR|Killed)\b|^Traceback \(most recent call last\)`),                                                    // pytest ERROR lines, OOM kill, Python traceback
		regexp.MustCompile(`\bSegmentation fault\b|\berror\[E[0-9]+\]|\bpanicked at\b|\bException in thread\b|(?i)\bunhandled exception\b`), // crashes, rustc, panics
		regexp.MustCompile(`\b[A-Z][A-Za-z]*(Error|Exception)\b( \[\w+\])?:`),                                                               // TypeError:, AssertionError [ERR_ASSERTION]:
		regexp.MustCompile(`(?m)^[^\s:]+\.go:[0-9]+(:[0-9]+)?: |(?i)\b[1-9][0-9]*[ \t]+issues?:`),                                           // go build/vet/lint diagnostics, golangci-lint "1 issues:"
		regexp.MustCompile(`(?m)^[ \t]*not ok\b`),                                                                                           // TAP, including indented subtests
		regexp.MustCompile(`\bError[ \t]+[1-9][0-9]*\b`),                                                                                    // make "*** [test] Error 2"
		regexp.MustCompile(`\berror (TS|CS)[0-9]+`),                                                                                         // tsc, MSBuild
		regexp.MustCompile(`(?i)\bnpm (ERR!|error)`),
		regexp.MustCompile(`(?i)\berror:`),
	}
	// lineFailurePatterns are anchored tool-output shapes (mr-b08): each matches from the START of a line
	// (multiline ^) in a fixed tool format. Where the phrase can also appear in prose it must be preceded by
	// a fixed token (a shell tag, or a program/path plus "line N:") with its own bounds pinned, so prose that
	// merely names the phrase - "covers the permission-denied path", "command not found handling is covered"
	// - never reads as a failure; where the phrase IS the whole line (a bare "Failures:" header), the shape is
	// that line alone. (failurePatterns also holds a few line-anchored shapes; this list groups the added
	// ones.) A shape that cannot be pinned this tightly (a bare "timed out", a non-shell tool's EACCES, an
	// errored/crashed count with no fixed prologue) is deliberately NOT a pattern: prefer an evidence receipt.
	lineFailurePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^panic: `),              // Go panic ("panic: send on closed channel")
		regexp.MustCompile(`(?m)^WARNING: DATA RACE\b`), // Go race detector
		// SIGABRT: glibc's bare "Aborted" / "Aborted (core dumped)", macOS/BSD's "Abort trap", or a shell's
		// "<shell>: [line N: |N:] PID Aborted ...".
		regexp.MustCompile(`(?m)^\s*Aborted(?:\s+\(core dumped\))?\s*\r?$`),
		regexp.MustCompile(`(?mi)^\s*Abort trap\b`),
		regexp.MustCompile(`(?mi)^\s*Bus error(?:\s+\(core dumped\))?\s*\r?$`),           // SIGBUS
		regexp.MustCompile(`(?mi)^\s*Illegal instruction(?:\s+\(core dumped\))?\s*\r?$`), // SIGILL
		regexp.MustCompile(`(?m)^\s*(?:` + shellTag + `): (?:line [0-9]+: |[0-9]+: )?[0-9]+ Aborted\b`),
		regexp.MustCompile(`(?mi)^\s*(?:` + shellTag + `): abort\b`), // zsh's lowercase SIGABRT ("zsh: abort ./prog")
		regexp.MustCompile(`(?m)^\s*make(?:\[[0-9]+\])?: \*\*\* `),   // make fatal ("No rule to make target")
		regexp.MustCompile(`(?m)^fatal: `),                           // git fatal
		// Missing command / EACCES as a shell reports it. The tag is a shell (or a script) name, so a prose
		// line like "Note: permission denied ..." is not read as a shell error.
		//   bash/zsh: "bash: [line N:] cmd: command not found" / "...: cmd: Permission denied"
		//   shell:    "sh: 1: cmd: not found" / "sh: 1: path: Permission denied"
		//   zsh:      "zsh: command not found: cmd" / "zsh: permission denied: path"
		regexp.MustCompile(`(?mi)^\s*(?:` + shellTag + `): (?:line [0-9]+: |[0-9]+: )?\S+: command not found\s*\r?$`),
		regexp.MustCompile(`(?mi)^\s*(?:` + shellTag + `): (?:line [0-9]+: |[0-9]+: )?\S+: permission denied\b`),
		regexp.MustCompile(`(?mi)^\s*(?:` + shellTag + `): (?:[0-9]+: )?\S+: not found\s*\r?$`),
		regexp.MustCompile(`(?mi)^\s*(?:` + shellTag + `):(?:[0-9]+:)?\s+command not found: \S+`),
		regexp.MustCompile(`(?mi)^\s*(?:` + shellTag + `):(?:[0-9]+:)?\s+permission denied: \S+`),
		regexp.MustCompile(`(?mi)^\S+@\S+: permission denied \(publickey[^)]*\)`), // ssh/scp
		regexp.MustCompile(`(?m)^(?:Command|Process) terminated by signal\b`),     // signal kill (GNU time / runner)
		// pytest's "no tests ran in Ns" (exit 5), bare or '='-padded ("===== no tests ran in 0.0s =====").
		regexp.MustCompile(`(?m)^=*\s*no tests ran in [0-9]`),
		regexp.MustCompile(`(?m)^No tests found, exiting with code [1-9]`),                   // jest (NOT passWithNoTests code 0)
		regexp.MustCompile(`(?m)^Jest: [^\n]*coverage threshold[^\n]*not met`),               // jest coverage gate
		regexp.MustCompile(`(?m)^(?:ESLint found )?too many warnings \(maximum: [0-9]+\)`),   // eslint --max-warnings N
		regexp.MustCompile(`(?m)^would reformat \S`),                                         // black --check
		regexp.MustCompile(`(?m)^\[warn\] Code style issues found\b`),                        // prettier --check
		regexp.MustCompile(`(?m)^[0-9]+ files? inspected, [1-9][0-9]* offenses? detected\b`), // rubocop summary
		regexp.MustCompile(`(?m)^\s*Failures?:\s*\r?$`),                                      // rspec bare "Failures:" header
		regexp.MustCompile(`(?m)^\s*[0-9]+\) Failure:`),                                      // minitest numbered "1) Failure:"
		regexp.MustCompile(`(?mi)^\s*[{,]?\s*"exit_?code"\s*[:=]\s*"?-?[1-9]`),               // JSON `"exitCode": 1`
	}
	// zeroClause and zeroLabel report that nothing failed; they are neutralized before failurePatterns run. Both are
	// narrow on purpose, so they can never swallow a real failure:
	//   - zeroClause: 0/no/none, optionally "of N" or "/N", one noun from a fixed list, then fail, failed, failing,
	//     failures or errors, STARTING a clause (line start, comma, semicolon, bracket or pipe) and ending one (a
	//     delimiter or line end, optionally after "in <duration>" or "out of N"): "…, 0 failed", "no tests failed",
	//     "none of the checks failed", "0 of 10 failed in 1s", bun "0 fail", ctest "0 tests failed out of 5".
	//     "shard 0 failed", "Passed: 0 Failed: 3" and "0 passed 3 failed" stay failures;
	//   - zeroLabel: a label with a zero count that ends there — "Failed: 0, Passed: 5", "failed=0 skipped=0",
	//     "# fail 0", "Errors: 0" — never "Error: 0 is not a valid port". A following "key=" is kept (${1}), so
	//     "failed=0 errors=3" still reads the errors.
	zeroClause = regexp.MustCompile(`(?im)(^|[,;(|])[ \t]*(0|no|none)([ \t]+of([ \t]+the)?([ \t]+[0-9]+)?|/[0-9]+)?[ \t]+((tests?|checks?|specs?|examples?|cases?|suites?)[ \t]+)?(fail|failed|failing|failures?|errors?)([ \t]+in[ \t]+[0-9.]+[mµn]?s|[ \t]+out of[ \t]+[0-9]+)?[ \t]*([,;.)(|!]|\r?$)`)
	zeroLabel  = regexp.MustCompile(`(?m)\b(?:(?i:failed|failures?|errors)[ \t]*[:=]|fail[ \t]*[:=]?)[ \t]*0(?:[ \t]*(?:[,;)|]|\r?$)|[ \t]+(\w+=))`)
	// proseFail is the one exemption the base reader lacked (mr-r3y): a lower-case "fail" in a prose sentence — after a
	// hypothetical or negated modal, as in a test's name ("should fail (3 ms)", "must fail", "doesn't fail"), or after a
	// subject word and before against/without ("the new tests fail against origin/main", "fail without the fix"). Not
	// "to fail" ("continues to fail" reports a failure; only "expected to fail" is hypothetical), nor "fail before".
	// Never "did/does fail" (a report, rewritten to "failed" first),
	// never upper or title case (a verdict), never after ":" or "=" ("Status: fail on windows"). It is neutralized after
	// a counted "fail" ("2 tests fail on windows") has already been read as a failure.
	proseFail = regexp.MustCompile(`\b(` + modalVerb + `)[ \t]+fail\b|(\w[ \t]+)fail[ \t]+(against|without)\b`)
	// modalFail is proseFail's modal half, removed before countFail reads a count: "1 should fail on main" counts nothing.
	modalFail = regexp.MustCompile(`\b(` + modalVerb + `)[ \t]+fail\b`)
	// reportedFail is "did/does/do fail": a report that something failed, never prose to exempt.
	reportedFail = regexp.MustCompile(`(?i)\b(did|does|do)[ \t]+fail\b`)
	// countFail is a counted "fail" ("3 tests fail and 9 pass", "1 test fails"): read before proseFail neutralizes.
	countFail = regexp.MustCompile(`(?i)\b[1-9][0-9]*[ \t]+(\w+[ \t]+)?fails?\b`)
	// expectedFailures is Python unittest's passing "OK (expected failures=1)": a count of failures that were expected.
	expectedFailures = regexp.MustCompile(`(?i)\bexpected failures?[ \t]*=[ \t]*[0-9]+`)
	// ansiEscape is a terminal colour/control sequence: removed first, since "\x1b[31mFAIL" has no word boundary.
	ansiEscape = regexp.MustCompile(`\x1b\[[0-9;:?]*[ -/]*[@-~]`) // ";" or ":" separates parameters ("\x1b[38:2:255:0:0m")
)

func Parse(data []byte) (Bundle, error) {
	return ParseWithOptions(data, ParseOptions{})
}

func ParseWithOptions(data []byte, options ParseOptions) (Bundle, error) {
	scanner := jsonl.NewScanner(bytes.NewReader(data))
	var receipts []Receipt
	receiptLines := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		receipt, ok, err := parseReceiptLine([]byte(line), options)
		if err != nil {
			return Bundle{}, err
		}
		if !ok {
			continue
		}
		receiptLines++
		receipts = append(receipts, receipt)
	}
	if err := scanner.Err(); err != nil {
		return Bundle{}, err
	}
	if receiptLines > 0 {
		return Bundle{Receipts: receipts}, nil
	}
	return parseFreeform(data), nil
}

func parseReceiptLine(line []byte, options ParseOptions) (Receipt, bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		if bytes.Contains(line, []byte("schemaVersion")) {
			return Receipt{}, false, err
		}
		return Receipt{}, false, nil
	}
	if _, ok := raw["schemaVersion"]; !ok {
		return Receipt{}, false, nil
	}
	var receipt Receipt
	if err := json.Unmarshal(line, &receipt); err != nil {
		return Receipt{}, false, err
	}
	if receipt.SchemaVersion != 1 {
		return Receipt{}, false, fmt.Errorf("unsupported evidence schemaVersion %d", receipt.SchemaVersion)
	}
	if _, ok := raw["exitCode"]; !ok {
		return Receipt{}, false, errors.New("evidence receipt missing exitCode")
	}
	if strings.TrimSpace(receipt.Summary) == "" {
		return Receipt{}, false, errors.New("evidence receipt missing summary")
	}
	if receipt.Kind == "" {
		receipt.Kind = ReceiptKindValidation
	}
	if options.Strict {
		if !options.Now.IsZero() && options.MaxAge > 0 {
			finished := receipt.FinishedAt
			if finished.IsZero() {
				finished = receipt.StartedAt
			}
			if finished.IsZero() {
				return Receipt{}, false, errors.New("strict evidence receipt missing timestamp")
			}
			if options.Now.Sub(finished) > options.MaxAge {
				return Receipt{}, false, errors.New("strict evidence receipt is stale")
			}
		}
	}
	return receipt, true, nil
}

func parseFreeform(data []byte) Bundle {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return Bundle{FreeformText: text, Fallback: true}
	}
	plain := ansiEscape.ReplaceAllString(text, "")
	exitCode := 1
	if hasSuccessSignal(plain) && !hasFailureSignal(plain) {
		exitCode = 0
	}
	return Bundle{
		Receipts: []Receipt{{
			SchemaVersion: 1,
			Kind:          ReceiptKindValidation,
			ExitCode:      exitCode,
			Summary:       firstFreeformSummary(text),
		}},
		FreeformText: text,
		Fallback:     true,
	}
}

func hasSuccessSignal(text string) bool {
	for _, pattern := range successPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func hasFailureSignal(text string) bool {
	text = expectedFailures.ReplaceAllString(text, " ")
	text = zeroClause.ReplaceAllString(text, "${1} ")
	text = zeroLabel.ReplaceAllString(text, " ${1}")
	if countFail.MatchString(modalFail.ReplaceAllString(text, " ")) {
		return true
	}
	text = reportedFail.ReplaceAllString(text, " failed")
	text = proseFail.ReplaceAllString(text, "${2} ")
	// The two slices are scanned identically; failurePatterns holds the base shapes (some also line-anchored)
	// and lineFailurePatterns the mr-b08 anchored shapes.
	for _, patterns := range [][]*regexp.Regexp{failurePatterns, lineFailurePatterns} {
		for _, pattern := range patterns {
			if pattern.MatchString(text) {
				return true
			}
		}
	}
	return false
}

func firstFreeformSummary(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return "freeform evidence"
}

func (bundle Bundle) HasSuccessfulValidation(kind Kind) bool {
	if kind == KindCICheck {
		return bundle.allCIChecksSuccessful()
	}
	if bundle.hasFailedValidation(kind) {
		return false
	}
	for _, receipt := range bundle.Receipts {
		if receipt.ExitCode != 0 {
			continue
		}
		if receipt.Kind != ReceiptKindValidation && receipt.Kind != ReceiptKindCICheck {
			continue
		}
		if receiptMatchesKind(receipt, kind) {
			return true
		}
	}
	return false
}

func (bundle Bundle) hasFailedValidation(kind Kind) bool {
	for _, receipt := range bundle.Receipts {
		if receipt.ExitCode == 0 {
			continue
		}
		if receipt.Kind != ReceiptKindValidation && receipt.Kind != ReceiptKindCICheck {
			continue
		}
		if kind == "" || kind == KindGeneric || receiptMatchesKind(receipt, kind) {
			return true
		}
	}
	return false
}

func (bundle Bundle) allCIChecksSuccessful() bool {
	seen := false
	for _, receipt := range bundle.Receipts {
		if receipt.Kind != ReceiptKindCICheck {
			continue
		}
		seen = true
		if receipt.ExitCode != 0 {
			return false
		}
	}
	return seen
}

func (bundle Bundle) ValidationSummaries() []string {
	var summaries []string
	for _, receipt := range bundle.Receipts {
		if receipt.Kind != ReceiptKindValidation && receipt.Kind != ReceiptKindCICheck {
			continue
		}
		prefix := "structured validation"
		if bundle.Fallback {
			prefix = "freeform fallback validation (best-effort; prefer an evidence receipt)"
		}
		status := fmt.Sprintf("exit %d", receipt.ExitCode)
		if receipt.Kind == ReceiptKindCICheck {
			prefix = "structured ci-check"
		}
		summary := strings.TrimSpace(receipt.Summary)
		if summary == "" {
			summary = strings.Join(receipt.Command, " ")
		}
		summaries = append(summaries, fmt.Sprintf("%s: %s (%s)", prefix, summary, status))
	}
	return summaries
}

// encodeReceiptLine is a seam over the JSON encoder so a test can force the (otherwise unreachable,
// since Receipt is always marshalable) encode failure JSONL propagates.
var encodeReceiptLine = func(w io.Writer, receipt Receipt) error {
	return json.NewEncoder(w).Encode(receipt)
}

func (bundle Bundle) JSONL() ([]byte, error) {
	var out bytes.Buffer
	for _, receipt := range bundle.Receipts {
		if err := encodeReceiptLine(&out, receipt); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func receiptMatchesKind(receipt Receipt, kind Kind) bool {
	if kind == "" || kind == KindGeneric {
		return true
	}
	if kind == KindCICheck {
		return receipt.Kind == ReceiptKindCICheck
	}
	text := strings.ToLower(strings.Join(append([]string{receipt.Summary}, receipt.Command...), " "))
	switch kind {
	case KindTests:
		return strings.Contains(text, "test") || strings.Contains(text, "go test") || strings.Contains(text, "pytest") || strings.Contains(text, "vitest") || strings.Contains(text, "jest")
	case KindBuild:
		return strings.Contains(text, "build")
	case KindTypecheck:
		return strings.Contains(text, "tsc") || strings.Contains(text, "typecheck")
	case KindCoverage:
		return strings.Contains(text, "coverage")
	default:
		return strings.Contains(text, strings.ToLower(string(kind)))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
