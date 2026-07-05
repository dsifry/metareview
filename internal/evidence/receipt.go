package evidence

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
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
	failurePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(exit(ed)?|exit code)\s+[1-9][0-9]*\b`),
		regexp.MustCompile(`(?i)\bFAIL\b`),
		regexp.MustCompile(`(?i)\bfailed\b`),
		regexp.MustCompile(`(?i)\berror:`),
	}
)

func Parse(data []byte) (Bundle, error) {
	return ParseWithOptions(data, ParseOptions{})
}

func ParseWithOptions(data []byte, options ParseOptions) (Bundle, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var receipts []Receipt
	jsonLines := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		jsonLines++
		receipt, err := parseReceiptLine([]byte(line), options)
		if err != nil {
			return Bundle{}, err
		}
		receipts = append(receipts, receipt)
	}
	if err := scanner.Err(); err != nil {
		return Bundle{}, err
	}
	if jsonLines > 0 {
		return Bundle{Receipts: receipts}, nil
	}
	return parseFreeform(data), nil
}

func parseReceiptLine(line []byte, options ParseOptions) (Receipt, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return Receipt{}, err
	}
	var receipt Receipt
	if err := json.Unmarshal(line, &receipt); err != nil {
		return Receipt{}, err
	}
	if receipt.SchemaVersion != 1 {
		return Receipt{}, fmt.Errorf("unsupported evidence schemaVersion %d", receipt.SchemaVersion)
	}
	if _, ok := raw["exitCode"]; !ok {
		return Receipt{}, errors.New("evidence receipt missing exitCode")
	}
	if strings.TrimSpace(receipt.Summary) == "" {
		return Receipt{}, errors.New("evidence receipt missing summary")
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
				return Receipt{}, errors.New("strict evidence receipt missing timestamp")
			}
			if options.Now.Sub(finished) > options.MaxAge {
				return Receipt{}, errors.New("strict evidence receipt is stale")
			}
		}
	}
	return receipt, nil
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
	if kind == KindGeneric && bundle.hasFailedCICheck() {
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

func (bundle Bundle) hasFailedCICheck() bool {
	for _, receipt := range bundle.Receipts {
		if receipt.Kind == ReceiptKindCICheck && receipt.ExitCode != 0 {
			return true
		}
	}
	return false
}

func (bundle Bundle) onlyCIChecks() bool {
	if len(bundle.Receipts) == 0 {
		return false
	}
	for _, receipt := range bundle.Receipts {
		if receipt.Kind != ReceiptKindCICheck {
			return false
		}
	}
	return true
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

func (bundle Bundle) JSONL() ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	for _, receipt := range bundle.Receipts {
		if err := encoder.Encode(receipt); err != nil {
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
