package lensoutput

import (
	"encoding/json"
	"testing"
)

// payloadEntry builds one payload entry from a base valid finding plus overrides; with no
// overrides it is exactly one valid entry.
func payload(t *testing.T, entries ...map[string]any) []byte {
	t.Helper()
	if len(entries) == 0 {
		entries = []map[string]any{{}}
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		base := map[string]any{
			"tag": "bug", "file": "a.go", "start_line": 2, "end_line": 3,
			"issue":       "the lookup misses deleted users",
			"consequence": "500 on every deleted user",
			"confidence":  75, "severity": "P1",
		}
		for k, v := range e {
			base[k] = v
		}
		out = append(out, base)
	}
	b, err := json.Marshal(map[string]any{"findings": out})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestValidatePayloadBuckets(t *testing.T) {
	rows := []struct {
		name    string
		payload []byte
		stats   Stats
		kept    int
	}{
		{"valid", payload(t), Stats{Kept: 1}, 1},
		{"empty-findings", []byte(`{"findings":[]}`), Stats{}, 0},
		{
			"schema-top-level-not-object",
			[]byte(`["not", "an", "object"]`),
			Stats{Schema: 1}, 0,
		},
		{
			"schema-findings-not-a-list",
			[]byte(`{"findings":"nope"}`),
			Stats{Schema: 1}, 0,
		},
		{
			"schema-entry-type-error",
			payload(t, map[string]any{"start_line": "two"}),
			Stats{Schema: 1}, 0,
		},
		{
			// A MISSING field is schema (the lab's KeyError bucket), distinct from the
			// present-but-empty case below (enum).
			"schema-missing-issue",
			[]byte(`{"findings":[{"tag":"bug","file":"a.go","start_line":2,"end_line":3,"consequence":"500s","confidence":75,"severity":"P1"}]}`),
			Stats{Schema: 1}, 0,
		},
		{
			"enum-bad-tag",
			payload(t, map[string]any{"tag": "smell"}),
			Stats{Enum: 1}, 0,
		},
		{
			"enum-bad-severity",
			payload(t, map[string]any{"severity": "critical"}),
			Stats{Enum: 1}, 0,
		},
		{
			"enum-confidence-out-of-range",
			payload(t, map[string]any{"confidence": 140}),
			Stats{Enum: 1}, 0,
		},
		{
			"enum-empty-issue",
			payload(t, map[string]any{"issue": "  "}),
			Stats{Enum: 1}, 0,
		},
		{
			"enum-empty-consequence",
			payload(t, map[string]any{"consequence": ""}),
			Stats{Enum: 1}, 0,
		},
		{
			"enum-line-range",
			payload(t, map[string]any{"start_line": 0}),
			Stats{Enum: 1}, 0,
		},
		{
			// An entry with an empty file passes the enum bucket and dies in the anchor
			// bucket — the lab's bucketing, preserved deliberately (see package doc).
			"anchor-empty-file",
			payload(t, map[string]any{"file": ""}),
			Stats{Anchor: 1}, 0,
		},
		{
			"anchor-file-not-in-diff",
			payload(t, map[string]any{"file": "elsewhere.go"}),
			Stats{Anchor: 1}, 0,
		},
		{
			"anchor-out-of-slack",
			payload(t, map[string]any{"start_line": 300, "end_line": 301}),
			Stats{Anchor: 1}, 0,
		},
		{
			"suppression-below-floor",
			payload(t, map[string]any{"confidence": 40}),
			Stats{Suppression: 1}, 0,
		},
		{
			"suppression-p0-overrides-floor",
			payload(t, map[string]any{"confidence": 10, "severity": "P0"}),
			Stats{Kept: 1}, 1,
		},
		{
			"mixed",
			payload(t,
				map[string]any{"tag": "advisory", "start_line": 48, "end_line": 51},
				map[string]any{"confidence": 25},
				map[string]any{"file": "gone.go"},
				map[string]any{"tag": "note"},
				map[string]any{"end_line": "x"},
			),
			Stats{Enum: 1, Anchor: 1, Suppression: 1, Schema: 1, Kept: 1}, 1,
		},
	}
	for _, r := range rows {
		kept, stats := ValidatePayload(r.payload, testDiff)
		if stats != r.stats {
			t.Errorf("%s: stats %+v want %+v", r.name, stats, r.stats)
		}
		if len(kept) != r.kept {
			t.Errorf("%s: kept %d want %d", r.name, len(kept), r.kept)
		}
	}
}

func TestValidatePayloadKeptEntryShape(t *testing.T) {
	// The kept entry is the trimmed, typed finding, and ToCandidate round-trips it into
	// the FSM shape with the canonical [TAG] text.
	kept, stats := ValidatePayload(payload(t, map[string]any{"tag": "advisory", "issue": "  padded  "}), testDiff)
	if stats.Kept != 1 || len(kept) != 1 {
		t.Fatalf("stats %+v kept %+v", stats, kept)
	}
	f := kept[0]
	if f.Issue != "padded" || f.Tag != TagAdvisory {
		t.Errorf("kept entry: %+v", f)
	}
	if f.CanonicalText() != "[ADVISORY] padded 500 on every deleted user" {
		t.Errorf("canonical: %q", f.CanonicalText())
	}
}
