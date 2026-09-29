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

var (
	successPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^ok\s+\S+`),
		regexp.MustCompile(`(?i)\b(go test|tests?|test suite).*\b(pass|passed|ok|exited 0)\b`),
		regexp.MustCompile(`(?i)\b(npm run build|build|tsc --noEmit|typecheck|coverage).*\b(pass|passed|ok|success|exited 0)\b`),
		regexp.MustCompile(`(?i)\bexited 0\b`),
	}
	// failurePatterns read a failure fail-closed: any "failed", and the shapes tools print, count. The one exemption is
	// the word "fail" continuing a prose sentence (mr-r3y: "the new tests fail against origin/main" means they CATCH the
	// regression); "fail" as a verdict — upper case, or followed by a line end, punctuation or a digit ("Result: Fail",
	// "status":"fail", "# fail 1") — counts. A clause reporting that nothing failed is neutralized first (zeroFailures).
	failurePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(exit(ed)?|exit[ _-]?code|exit[ _-]?status|return[ _-]?code|exited with( code| status)?|exit (code|status) (was|is))[ \t]*[:=]?[ \t]*-?[1-9][0-9]*\b|\brc[ \t]*[:=][ \t]*-?[1-9]`),
		regexp.MustCompile(`\bFAIL(URES?)?\b`),                                                                                              // FAIL, BUILD FAILURE, FAILURES! (upper case)
		regexp.MustCompile(`(?m)(^|[^/.\w-])[Ff]ail\b[ \t]*([^ \ta-zA-Z\r\n(/.-]|\.([^\w]|$)|\r?$)`),                                        // a "fail" verdict, not a sentence
		regexp.MustCompile(`(?m)(^|[^/.\w-])[Ff]ail[ \t]*\r?$`),                                                                             // "fail" ending a line
		regexp.MustCompile(`(?i)\b[1-9][0-9]*[ \t]+(\w+[ \t]+)?fails?\b`),                                                                   // a counted "fail": "3 tests fail and 9 pass", "1 test fails"
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
	zeroLabel  = regexp.MustCompile(`(?im)\b(?:failed|failures?|errors?|fail)[ \t]*[:=]?[ \t]*0(?:[ \t]*(?:[,;)|]|\r?$)|[ \t]+(\w+=))`)
	// ansiEscape is a terminal colour/control sequence: removed first, since "\x1b[31mFAIL" has no word boundary.
	ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
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
	text = zeroClause.ReplaceAllString(text, "${1} ")
	text = zeroLabel.ReplaceAllString(text, " ${1}")
	for _, pattern := range failurePatterns {
		if pattern.MatchString(text) {
			return true
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
			prefix = "freeform fallback validation"
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
