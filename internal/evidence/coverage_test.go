package evidence

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/jsonl"
)

// --- ParseWithOptions: JSONL scanner error ---------------------------------

// A single line larger than the JSONL line cap makes the underlying scanner fail with
// bufio.ErrTooLong; ParseWithOptions must surface that error rather than silently
// falling back to freeform parsing.
func TestParseWithOptionsSurfacesScannerError(t *testing.T) {
	oversized := strings.Repeat("x", jsonl.MaxLineBytes+10)
	if _, err := Parse([]byte(oversized)); err == nil {
		t.Fatalf("expected a scanner error for an oversized line")
	}
}

// --- parseReceiptLine branches ---------------------------------------------

// A brace-prefixed line that mentions schemaVersion but is not valid JSON is a corrupt receipt,
// not free text, so parsing must fail loudly instead of falling back.
func TestMalformedSchemaVersionLineIsAnError(t *testing.T) {
	input := []byte(`{"schemaVersion":}` + "\n")
	if _, err := Parse(input); err == nil {
		t.Fatalf("a malformed line mentioning schemaVersion must error")
	}
}

// Only schemaVersion 1 is supported; any other version is rejected.
func TestUnsupportedSchemaVersionIsRejected(t *testing.T) {
	input := []byte(`{"schemaVersion":2,"exitCode":0,"summary":"future format"}` + "\n")
	if _, err := Parse(input); err == nil {
		t.Fatalf("schemaVersion 2 must be rejected")
	}
}

// A receipt with no explicit kind defaults to a validation receipt and still validates.
func TestReceiptWithoutKindDefaultsToValidation(t *testing.T) {
	input := []byte(`{"schemaVersion":1,"command":["go","test","./..."],"exitCode":0,"summary":"go test ./... exited 0"}` + "\n")
	bundle, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bundle.Receipts) != 1 || bundle.Receipts[0].Kind != ReceiptKindValidation {
		t.Fatalf("missing kind should default to %q: %+v", ReceiptKindValidation, bundle.Receipts)
	}
	if !bundle.HasSuccessfulValidation(KindTests) {
		t.Fatalf("kind-less successful receipt should validate: %+v", bundle.Receipts)
	}
}

// Strict mode with a freshness window rejects a receipt that carries no timestamp at all: we
// cannot prove it is recent, so it must not pass.
func TestStrictModeRejectsReceiptMissingTimestamp(t *testing.T) {
	input := []byte(`{"schemaVersion":1,"exitCode":0,"summary":"no timestamps here"}` + "\n")
	_, err := ParseWithOptions(input, ParseOptions{
		Strict: true,
		Now:    time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
		MaxAge: time.Hour,
	})
	if err == nil {
		t.Fatalf("strict parse should reject a receipt with no timestamp")
	}
}

// Strict mode falls back to startedAt when finishedAt is absent, and a fresh startedAt passes.
func TestStrictModeUsesStartedAtWhenFinishedAtAbsent(t *testing.T) {
	input := []byte(`{"schemaVersion":1,"exitCode":0,"startedAt":"2026-07-05T11:30:00Z","summary":"go test ./... exited 0"}` + "\n")
	bundle, err := ParseWithOptions(input, ParseOptions{
		Strict: true,
		Now:    time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
		MaxAge: time.Hour,
	})
	if err != nil {
		t.Fatalf("a receipt with a fresh startedAt should pass strict mode: %v", err)
	}
	if !bundle.HasSuccessfulValidation(KindTests) {
		t.Fatalf("fresh startedAt receipt should validate: %+v", bundle.Receipts)
	}
}

// A valid JSON object that simply omits schemaVersion is not a receipt and is skipped, so a file
// of such lines falls back to freeform parsing rather than erroring.
func TestValidJSONWithoutSchemaVersionIsSkipped(t *testing.T) {
	bundle, err := Parse([]byte(`{"foo":"bar"}` + "\n" + "go test ./... exited 0"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !bundle.Fallback {
		t.Fatalf("a non-receipt JSON object should not count as a receipt: %+v", bundle)
	}
}

// A line whose schemaVersion is present but whose typed fields have the wrong shape (e.g. a
// string exitCode) is a corrupt receipt and must error.
func TestReceiptWithWronglyTypedFieldIsAnError(t *testing.T) {
	input := []byte(`{"schemaVersion":1,"exitCode":"not-a-number","summary":"x"}` + "\n")
	if _, err := Parse(input); err == nil {
		t.Fatalf("a receipt with a wrongly typed field must error")
	}
}

// --- parseFreeform / signal helpers ----------------------------------------

// Whitespace-only input yields an empty freeform fallback bundle with no receipts.
func TestFreeformWhitespaceOnlyYieldsEmptyFallback(t *testing.T) {
	bundle, err := Parse([]byte("   \n\t\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !bundle.Fallback {
		t.Fatalf("expected fallback marker: %+v", bundle)
	}
	if len(bundle.Receipts) != 0 {
		t.Fatalf("whitespace-only input should produce no receipts: %+v", bundle.Receipts)
	}
	if bundle.FreeformText != "" {
		t.Fatalf("freeform text should be empty: %q", bundle.FreeformText)
	}
}

// Freeform text with no success signal (and no failure signal) is treated as a failed run: the
// synthesized receipt gets a nonzero exit code and does not validate.
func TestFreeformWithoutSuccessSignalDoesNotValidate(t *testing.T) {
	bundle, err := Parse([]byte("just some prose with no recognizable result"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if bundle.HasSuccessfulValidation(KindGeneric) {
		t.Fatalf("text with no success signal must not validate: %+v", bundle.Receipts)
	}
	if len(bundle.Receipts) != 1 || bundle.Receipts[0].ExitCode == 0 {
		t.Fatalf("no-signal freeform should synthesize a failed receipt: %+v", bundle.Receipts)
	}
}

// Freeform text that carries both a success signal and a failure signal is treated as a failure:
// a stray FAIL line must not be masked by an earlier ok line.
func TestFreeformWithMixedSignalsIsFailure(t *testing.T) {
	bundle, err := Parse([]byte("ok  \tgithub.com/foo\t0.1s\nFAIL\tgithub.com/bar\t0.2s"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if bundle.HasSuccessfulValidation(KindGeneric) {
		t.Fatalf("mixed ok/FAIL output must not validate: %+v", bundle.Receipts)
	}
}

// firstFreeformSummary defends against being handed content that is all blank lines by returning a
// neutral placeholder rather than an empty string.
func TestFirstFreeformSummaryFallsBackToPlaceholder(t *testing.T) {
	if got := firstFreeformSummary("\n   \n\t\n"); got != "freeform evidence" {
		t.Fatalf("all-blank input should yield the placeholder, got %q", got)
	}
	if got := firstFreeformSummary("first meaningful line\nsecond"); got != "first meaningful line" {
		t.Fatalf("should return the first non-empty line, got %q", got)
	}
}

// --- HasSuccessfulValidation / hasFailedValidation edge cases ---------------

// A generic validation query skips receipts whose exit code is nonzero and receipts of an
// unrelated kind (e.g. runtime), and returns false when nothing successful matches.
func TestHasSuccessfulValidationSkipsFailedAndUnrelatedReceipts(t *testing.T) {
	bundle := Bundle{Receipts: []Receipt{
		{Kind: ReceiptKindRuntime, ExitCode: 0, Summary: "runtime probe"},
		{Kind: ReceiptKindValidation, ExitCode: 2, Summary: "go build ./... exited 2"},
	}}
	// The only exit-0 receipt is a runtime receipt (wrong kind), and the validation receipt failed.
	if bundle.HasSuccessfulValidation(KindBuild) {
		t.Fatalf("no successful validation receipt of the right kind should exist: %+v", bundle.Receipts)
	}
}

// When no failed receipt of the queried kind exists, HasSuccessfulValidation scans every receipt:
// it skips those with a nonzero exit code and those of an unrelated kind, and returns false when
// no successful receipt of the right kind is found.
func TestHasSuccessfulValidationScansAndReturnsFalse(t *testing.T) {
	bundle := Bundle{Receipts: []Receipt{
		{Kind: ReceiptKindValidation, ExitCode: 1, Summary: "go build ./... exited 1"}, // nonzero exit → skipped
		{Kind: ReceiptKindRuntime, ExitCode: 0, Summary: "runtime probe"},              // wrong kind → skipped
		{Kind: ReceiptKindValidation, ExitCode: 0, Summary: "go vet ./... exited 0"},   // right kind, doesn't match tests
	}}
	// The failing receipt is a build, not a test, so it doesn't count as a failed *test* validation;
	// the scan then finds no successful test receipt and returns false.
	if bundle.HasSuccessfulValidation(KindTests) {
		t.Fatalf("no successful test receipt should be found: %+v", bundle.Receipts)
	}
}

// A ci-check query over a bundle that contains no ci-check receipts is unsatisfied: there is
// nothing to attest.
func TestCICheckValidationRequiresAtLeastOneCICheck(t *testing.T) {
	bundle := Bundle{Receipts: []Receipt{
		{Kind: ReceiptKindValidation, ExitCode: 0, Summary: "go test ./... exited 0"},
	}}
	if bundle.HasSuccessfulValidation(KindCICheck) {
		t.Fatalf("a bundle with no ci-check receipts must not satisfy ci-check validation")
	}
}

// hasFailedValidation ignores a nonzero exit on a runtime receipt (wrong kind), so a passing
// validation receipt alongside a failing runtime receipt still validates.
func TestFailedRuntimeReceiptDoesNotBlockValidation(t *testing.T) {
	bundle := Bundle{Receipts: []Receipt{
		{Kind: ReceiptKindValidation, ExitCode: 0, Summary: "go test ./... exited 0"},
		{Kind: ReceiptKindRuntime, ExitCode: 1, Summary: "runtime probe exited 1"},
	}}
	if !bundle.HasSuccessfulValidation(KindTests) {
		t.Fatalf("a failed runtime receipt must not block a passing test validation: %+v", bundle.Receipts)
	}
}

// --- receiptMatchesKind (all kind branches) ---------------------------------

func TestReceiptMatchesKind(t *testing.T) {
	cases := []struct {
		name    string
		receipt Receipt
		kind    Kind
		want    bool
	}{
		{"empty kind matches anything", Receipt{Summary: "anything"}, "", true},
		{"generic matches anything", Receipt{Summary: "anything"}, KindGeneric, true},
		{"ci-check matches ci-check receipt", Receipt{Kind: ReceiptKindCICheck}, KindCICheck, true},
		{"ci-check rejects validation receipt", Receipt{Kind: ReceiptKindValidation}, KindCICheck, false},
		{"tests match go test", Receipt{Summary: "go test ./... exited 0"}, KindTests, true},
		{"tests match pytest", Receipt{Command: []string{"pytest"}}, KindTests, true},
		{"tests reject unrelated", Receipt{Summary: "linting only"}, KindTests, false},
		{"build matches", Receipt{Summary: "npm run build"}, KindBuild, true},
		{"typecheck matches tsc", Receipt{Summary: "tsc --noEmit"}, KindTypecheck, true},
		{"typecheck matches typecheck word", Receipt{Command: []string{"make", "typecheck"}}, KindTypecheck, true},
		{"coverage matches", Receipt{Summary: "coverage gate 95%"}, KindCoverage, true},
		{"default matches kind substring", Receipt{Summary: "custom lint run"}, Kind("lint"), true},
		{"default rejects when absent", Receipt{Summary: "something else"}, Kind("lint"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := receiptMatchesKind(tc.receipt, tc.kind); got != tc.want {
				t.Fatalf("receiptMatchesKind(%+v, %q) = %v, want %v", tc.receipt, tc.kind, got, tc.want)
			}
		})
	}
}

// --- ValidationSummaries ----------------------------------------------------

// ValidationSummaries renders one human-readable line per validation/ci-check receipt, labels
// freeform-fallback and ci-check receipts distinctly, falls back to the command when the summary
// is blank, and skips receipts of unrelated kinds.
func TestValidationSummaries(t *testing.T) {
	bundle := Bundle{
		Receipts: []Receipt{
			{Kind: ReceiptKindValidation, ExitCode: 0, Summary: "go test ./... exited 0"},
			{Kind: ReceiptKindCICheck, ExitCode: 1, Summary: "lint fail"},
			{Kind: ReceiptKindValidation, ExitCode: 0, Command: []string{"go", "vet", "./..."}},
			{Kind: ReceiptKindRuntime, ExitCode: 0, Summary: "runtime probe"},
		},
	}
	got := bundle.ValidationSummaries()
	want := []string{
		"structured validation: go test ./... exited 0 (exit 0)",
		"structured ci-check: lint fail (exit 1)",
		"structured validation: go vet ./... (exit 0)",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d summaries, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("summary %d = %q, want %q", i, got[i], w)
		}
	}
}

// A freeform-fallback bundle labels its validation summary as a freeform fallback.
func TestValidationSummariesLabelsFreeformFallback(t *testing.T) {
	bundle, err := Parse([]byte("go test ./... exited 0"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	summaries := bundle.ValidationSummaries()
	if len(summaries) != 1 || !strings.HasPrefix(summaries[0], "freeform fallback validation:") {
		t.Fatalf("expected a freeform-fallback-labeled summary, got %v", summaries)
	}
}

// --- JSONL ------------------------------------------------------------------

// JSONL emits one JSON object per line, round-tripping back to equivalent receipts.
func TestJSONLRoundTrips(t *testing.T) {
	bundle := Bundle{Receipts: []Receipt{
		{SchemaVersion: 1, Kind: ReceiptKindValidation, ExitCode: 0, Summary: "go test ./... exited 0"},
		{SchemaVersion: 1, Kind: ReceiptKindCICheck, ExitCode: 1, Summary: "lint fail"},
	}}
	data, err := bundle.JSONL()
	if err != nil {
		t.Fatalf("jsonl: %v", err)
	}
	if lines := strings.Count(strings.TrimRight(string(data), "\n"), "\n"); lines != 1 {
		t.Fatalf("expected 2 JSONL lines (1 separator), got %d: %q", lines+1, data)
	}
	round, err := Parse(data)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(round.Receipts) != 2 || round.Receipts[0].Summary != "go test ./... exited 0" {
		t.Fatalf("round-trip lost receipts: %+v", round.Receipts)
	}
}

// JSONL propagates an encoder failure rather than returning partial output.
func TestJSONLPropagatesEncodeError(t *testing.T) {
	original := encodeReceiptLine
	t.Cleanup(func() { encodeReceiptLine = original })
	encodeReceiptLine = func(io.Writer, Receipt) error { return errors.New("boom") }

	bundle := Bundle{Receipts: []Receipt{{SchemaVersion: 1, Summary: "x"}}}
	if _, err := bundle.JSONL(); err == nil {
		t.Fatalf("JSONL should propagate an encode error")
	}
}

// --- firstNonEmpty ----------------------------------------------------------

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "\tvalue\t", "other"); got != "value" {
		t.Fatalf("expected the first non-blank value trimmed, got %q", got)
	}
	if got := firstNonEmpty("", "   ", "\t"); got != "" {
		t.Fatalf("all-blank inputs should yield an empty string, got %q", got)
	}
}
