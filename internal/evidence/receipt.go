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
	// the word "fail" in prose (mr-r3y: "the new tests fail against origin/main" means they CATCH the regression): a
	// lower-case "fail" counts only beside a count ("1 fail", "# fail 1", "fail: 2"). A clause reporting that nothing
	// failed is neutralized first (zeroFailures).
	failurePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(exit(ed)?|exit code|exit status)\s+[1-9][0-9]*\b`),
		regexp.MustCompile(`\bFAIL(URES?)?\b`),                                                 // FAIL, BUILD FAILURE, FAILURES! (upper case)
		regexp.MustCompile(`(?i)\bfailed\b`),                                                   // any form: "Failed: 1", "Command failed.", "go vet failed"
		regexp.MustCompile(`(?i)\b(failures?|errors?)[ \t]*[:=][ \t]*[1-9]`),                   // junit/maven "Failures: 1", "Errors: 2"
		regexp.MustCompile(`(?i)\b[1-9][0-9]*[ \t]+(\w+[ \t]+)?(failing|failures?|errors?)\b`), // mocha "1 failing", "2 tests failing", "1 error"
		regexp.MustCompile(`(?i)\b[1-9][0-9]*[ \t]+fail\b`),                                    // bun "1 fail"
		regexp.MustCompile(`(?i)\bfail[ \t]*[:=]?[ \t]*[1-9]`),                                 // node:test "# fail 1", "fail: 2"
		regexp.MustCompile(`(?m)^not ok\b`),                                                    // TAP
		regexp.MustCompile(`\bError[ \t]+[1-9][0-9]*\b`),                                       // make "*** [test] Error 2"
		regexp.MustCompile(`\berror (TS|CS)[0-9]+`),                                            // tsc, MSBuild
		regexp.MustCompile(`(?i)\bnpm (ERR!|error)`),
		regexp.MustCompile(`(?i)\berror:`),
	}
	// zeroFailures is a clause reporting that nothing failed — "0 failed", "no tests failed", "none of the checks
	// failed", "0 of 10 failed", "0/10 failed" — neutralized before failurePatterns run. It is deliberately narrow, so
	// it can never swallow a real failure: the clause is bounded on both sides (line start or end, a comma, semicolon or
	// bracket), stays on one line, and allows only one noun from a fixed list between the zero and the failure word.
	// "Passed: 0 Failed: 3", "0 passed 3 failed" and "exited 0 build failed" are therefore left as failures.
	zeroFailures = regexp.MustCompile(`(?im)(^|[,;(]|[ \t])(0|no|none)([ \t]+of([ \t]+the)?([ \t]+[0-9]+)?|/[0-9]+)?[ \t]+((tests?|checks?|specs?|examples?|cases?|suites?)[ \t]+)?(failed|failing|failures?|errors?)[ \t]*([,;.)!]|$)`)
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
	exitCode := 1
	if hasSuccessSignal(text) && !hasFailureSignal(text) {
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
	text = zeroFailures.ReplaceAllString(text, "${1} ")
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
