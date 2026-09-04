package state

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello World.md":       "hello-world",      // lowercased, extension stripped, spaces → -
		"UPPER":                "upper",            // lowercase, no extension
		"a.b.c":                "a-b",              // only the LAST dot is treated as an extension
		"  !!!  ":              "target",           // nothing slug-worthy → fallback
		"internal/foo_bar":     "internal-foo-bar", // separators collapse to a single -
		"--leading-trailing--": "leading-trailing", // outer dashes trimmed
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	// A value longer than the 48-char cap is truncated (and dash-trimmed), never longer than 48.
	long := Slugify(strings.Repeat("a", 60) + " tail")
	if len(long) > 48 {
		t.Fatalf("Slugify must cap at 48 chars, got %d (%q)", len(long), long)
	}
}

func TestRunID(t *testing.T) {
	at := time.Date(2026, 9, 4, 8, 30, 15, 123456789, time.UTC)
	got := RunID("pr-ready", "current branch", at)
	// Contract: mrv-<utc stamp>-<scope slug>-<target-base slug>-<8 hex of target hash>.
	re := regexp.MustCompile(`^mrv-20260904-083015123456789-pr-ready-current-branch-[0-9a-f]{8}$`)
	if !re.MatchString(got) {
		t.Fatalf("RunID = %q, does not match the documented shape", got)
	}
	// Deterministic in the target: same inputs → same id; a different target → a different hash suffix.
	if RunID("pr-ready", "current branch", at) != got {
		t.Fatal("RunID must be deterministic for identical inputs")
	}
	if RunID("pr-ready", "other target", at) == got {
		t.Fatal("a different target must change the id")
	}
}

func TestFindingID(t *testing.T) {
	if got := FindingID("mrv-20260904-abc", 7); got != "mrvf-20260904-abc-007" {
		t.Fatalf("FindingID = %q, want mrvf-20260904-abc-007", got)
	}
	// An index wider than the pad width is not truncated.
	if got := FindingID("mrv-x", 1234); got != "mrvf-x-1234" {
		t.Fatalf("FindingID = %q, want mrvf-x-1234", got)
	}
}

func TestLeftPad(t *testing.T) {
	cases := map[int]string{5: "005", 42: "042", 100: "100", 1234: "1234"}
	for in, want := range cases {
		if got := leftPad(in, 3); got != want {
			t.Errorf("leftPad(%d,3) = %q, want %q", in, got, want)
		}
	}
}
