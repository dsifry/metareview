package prready

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dsifry/metareview/internal/findings"
)

// prready-1: verdictForCounts's ESCALATED branch and its `attemptNumber >= maxAttempts` boundary were
// unpinned — the only call in the suite was (gate, 1, 3) with no blockers. These cases pin every arm,
// including the >= boundary (attempt == max escalates; attempt == max-1 does not).
func TestVerdictForCounts(t *testing.T) {
	blk := findings.ClassCounts{Blocking: 1}
	adv := findings.ClassCounts{Advisory: 1}
	cases := []struct {
		name         string
		counts       findings.ClassCounts
		gateEffect   string
		attempt, max int
		verdict      string
		blocking     bool
		reasonEmpty  bool
	}{
		{"escalate at boundary", blk, "gate", 3, 3, "ESCALATED", true, false},
		{"escalate past boundary", blk, "gate", 4, 3, "ESCALATED", true, false},
		{"below boundary needs revision", blk, "gate", 2, 3, "NEEDS_REVISION", true, true},
		{"first attempt needs revision", blk, "gate", 1, 3, "NEEDS_REVISION", true, true},
		{"advisory findings pass_advisory", adv, "gate", 1, 3, "PASS_ADVISORY", false, true},
		{"advisory gate effect pass_advisory", findings.ClassCounts{}, "advisory", 1, 3, "PASS_ADVISORY", false, true},
		{"clean gated pass", findings.ClassCounts{}, "gate", 1, 3, "PASS", false, true},
	}
	for _, c := range cases {
		verdict, _, blocking, reason := verdictForCounts(c.counts, c.gateEffect, c.attempt, c.max)
		if verdict != c.verdict || blocking != c.blocking || (reason == "") != c.reasonEmpty {
			t.Errorf("%s: got (%q, blocking=%v, reason=%q), want (%q, blocking=%v, reasonEmpty=%v)",
				c.name, verdict, blocking, reason, c.verdict, c.blocking, c.reasonEmpty)
		}
	}
}

// prready-3: readEvidence truncated at a raw byte boundary (text[:12000]), which can split a multi-byte
// UTF-8 rune and emit an invalid byte into the context pack.
func TestReadEvidenceTruncatesAtRuneBoundary(t *testing.T) {
	// A 2-byte rune straddling byte 12000: 11999 ASCII bytes, then "é" (0xC3 0xA9) at bytes 11999-12000.
	content := strings.Repeat("a", 11999) + "é" + strings.Repeat("b", 100)
	path := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readEvidence(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 12000 {
		t.Fatalf("evidence not truncated: %d bytes", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncated evidence is not valid UTF-8 (rune split at the byte boundary)")
	}
	// Empty path and short content are pass-throughs.
	if s, _ := readEvidence(""); s != "" {
		t.Fatalf("empty path should yield empty string, got %q", s)
	}
	// A short file passes through unchanged.
	short := filepath.Join(t.TempDir(), "short.txt")
	if err := os.WriteFile(short, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s, _ := readEvidence(short); s != "hi" {
		t.Fatalf("short content should pass through, got %q", s)
	}
	// A missing path surfaces the read error.
	if _, err := readEvidence(filepath.Join(t.TempDir(), "does-not-exist.txt")); err == nil {
		t.Fatal("reading a missing evidence file should error")
	}
}
