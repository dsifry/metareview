package evidence

import (
	"testing"
	"time"
)

func TestParseJSONLReceipts(t *testing.T) {
	input := []byte(`{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"cwd":"/repo","exitCode":0,"startedAt":"2026-07-04T12:00:00Z","finishedAt":"2026-07-04T12:00:02Z","summary":"go test ./... exited 0"}` + "\n")
	bundle, err := Parse(input)
	if err != nil {
		t.Fatalf("parse receipts: %v", err)
	}
	if !bundle.HasSuccessfulValidation(KindTests) {
		t.Fatalf("expected successful validation: %+v", bundle)
	}
}

func TestFailedReceiptDoesNotPassValidation(t *testing.T) {
	input := []byte(`{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":1,"summary":"tests passed before final failure"}` + "\n")
	bundle, err := Parse(input)
	if err != nil {
		t.Fatalf("parse receipts: %v", err)
	}
	if bundle.HasSuccessfulValidation(KindTests) {
		t.Fatalf("failed receipt must not pass validation")
	}
}

func TestFreeformFallbackRecognizesCommonSuccess(t *testing.T) {
	for _, text := range []string{
		"ok  \tgithub.com/dsifry/metareview/internal/reviewlog\t0.132s",
		"npm run build exited 0",
		"tsc --noEmit exited 0",
		"coverage gate passed: 95.4%",
	} {
		bundle, err := Parse([]byte(text))
		if err != nil {
			t.Fatalf("parse fallback %q: %v", text, err)
		}
		if !bundle.HasSuccessfulValidation(KindGeneric) {
			t.Fatalf("expected fallback validation for %q", text)
		}
		if !bundle.Fallback {
			t.Fatalf("expected fallback marker for %q", text)
		}
	}
}

func TestMalformedReceiptFailsStrictMode(t *testing.T) {
	input := []byte(`{"schemaVersion":1,"kind":"validation","summary":"missing exit code"}` + "\n")
	if _, err := Parse(input); err == nil {
		t.Fatalf("default parse should fail missing exitCode")
	}
	if _, err := ParseWithOptions(input, ParseOptions{Strict: true}); err == nil {
		t.Fatalf("strict parse should fail missing exitCode")
	}
}

func TestMalformedReceiptMissingSummaryFails(t *testing.T) {
	input := []byte(`{"schemaVersion":1,"kind":"validation","exitCode":0}` + "\n")
	if _, err := Parse(input); err == nil {
		t.Fatalf("parse should fail missing summary")
	}
}

func TestStaleReceiptFailsStrictMode(t *testing.T) {
	input := []byte(`{"schemaVersion":1,"kind":"validation","exitCode":0,"finishedAt":"2026-07-04T12:00:00Z","summary":"go test ./... exited 0"}` + "\n")
	_, err := ParseWithOptions(input, ParseOptions{
		Strict: true,
		Now:    time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
		MaxAge: time.Hour,
	})
	if err == nil {
		t.Fatalf("strict parse should fail stale receipt")
	}
}

func TestContradictoryReceiptUsesExitCodeAsAuthority(t *testing.T) {
	input := []byte(`{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":1,"summary":"all tests passed"}` + "\n")
	bundle, err := Parse(input)
	if err != nil {
		t.Fatalf("parse receipts: %v", err)
	}
	if bundle.HasSuccessfulValidation(KindGeneric) {
		t.Fatalf("exitCode 1 must not satisfy validation")
	}
}

func TestFailedCICheckPreventsGenericValidation(t *testing.T) {
	input := []byte(
		`{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":0,"summary":"go test ./... exited 0"}` + "\n" +
			`{"schemaVersion":1,"kind":"ci-check","command":["gh","pr","checks","3"],"exitCode":1,"summary":"lint fail"}` + "\n")
	bundle, err := Parse(input)
	if err != nil {
		t.Fatalf("parse mixed receipts: %v", err)
	}
	if bundle.HasSuccessfulValidation(KindGeneric) {
		t.Fatalf("failed ci-check must prevent generic validation")
	}
	if !bundle.HasSuccessfulValidation(KindTests) {
		t.Fatalf("specific local test validation should remain visible")
	}
}

func TestFailedValidationReceiptPreventsGenericValidation(t *testing.T) {
	input := []byte(
		`{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":0,"summary":"go test ./... exited 0"}` + "\n" +
			`{"schemaVersion":1,"kind":"validation","command":["go","vet","./..."],"exitCode":1,"summary":"go vet ./... exited 1"}` + "\n")
	bundle, err := Parse(input)
	if err != nil {
		t.Fatalf("parse mixed receipts: %v", err)
	}
	if bundle.HasSuccessfulValidation(KindGeneric) {
		t.Fatalf("failed validation receipt must prevent generic validation")
	}
	if !bundle.HasSuccessfulValidation(KindTests) {
		t.Fatalf("specific successful test validation should remain visible")
	}
}
