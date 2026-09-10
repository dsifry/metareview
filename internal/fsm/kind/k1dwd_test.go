package kind

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
)


// TestK1DecodeWithDiff pins the production review-lenses decode: the typed contract enforced
// against the diff the node reviewed. Every bucket (schema/enum/anchor/suppression/kept),
// the canonical candidate forms, the caps, and the WarningEmitter telemetry.
func TestK1DecodeWithDiff(t *testing.T) {
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	k, _ := r.Kind(ReviewLenses)
	dd, ok := k.(machine.DiffDecoder)
	if !ok {
		t.Fatal("review-lenses must implement machine.DiffDecoder")
	}
	diff := machine.Diff{Text: "--- a/f.go\n+++ b/f.go\n@@ -1,4 +1,5 @@\n a\n-b\n+c\n d\n+e\n"}
	entry := func(over string) string {
		e := `{"tag":"bug","file":"f.go","start_line":2,"end_line":2,"issue":"i","consequence":"c","confidence":75,"severity":"P2"}`
		if over != "" {
			e = over
		}
		return e
	}
	dec := func(payload string) (findingsOut, error) {
		out, err := dd.DecodeWithDiff(json.RawMessage(payload), diff)
		if err != nil {
			return findingsOut{}, err
		}
		return out.(findingsOut), nil
	}

	// kept: a valid, in-diff entry
	out, err := dec(`{"findings":[` + entry("") + `]}`)
	if err != nil || len(out.Findings) != 1 || out.stats.Kept != 1 {
		t.Fatalf("kept: %+v err=%v", out, err)
	}
	f := out.Findings[0]
	if f.IssueText != "[BUG] i c" || f.File != "f.go" || f.Line != 2 || f.Severity != "p2" || f.Category != "bug" || f.Source != "lens" {
		t.Fatalf("canonical forms: %+v", f)
	}
	// a clean run emits no warnings
	if w := out.Warnings(); w != nil {
		t.Fatalf("clean run warnings: %v", w)
	}

	// schema: missing field
	out, err = dec(`{"findings":[` + entry(`{"tag":"bug","file":"f.go","start_line":2,"end_line":2,"issue":"i","confidence":75,"severity":"P2"}`) + `]}`)
	if err != nil || out.stats.Schema != 1 || len(out.Findings) != 0 {
		t.Fatalf("schema bucket: %+v err=%v", out.stats, err)
	}
	// schema: wrong type (string where int belongs)
	out, err = dec(`{"findings":[` + entry(`{"tag":"bug","file":"f.go","start_line":"2","end_line":2,"issue":"i","consequence":"c","confidence":75,"severity":"P2"}`) + `]}`)
	if err != nil || out.stats.Schema != 1 {
		t.Fatalf("schema type bucket: %+v err=%v", out.stats, err)
	}
	// enum: bad tag
	out, err = dec(`{"findings":[` + entry(`{"tag":"smell","file":"f.go","start_line":2,"end_line":2,"issue":"i","consequence":"c","confidence":75,"severity":"P2"}`) + `]}`)
	if err != nil || out.stats.Enum != 1 {
		t.Fatalf("enum bucket: %+v err=%v", out.stats, err)
	}
	// enum: inverted range
	out, err = dec(`{"findings":[` + entry(`{"tag":"bug","file":"f.go","start_line":5,"end_line":2,"issue":"i","consequence":"c","confidence":75,"severity":"P2"}`) + `]}`)
	if err != nil || out.stats.Enum != 1 {
		t.Fatalf("enum range bucket: %+v err=%v", out.stats, err)
	}
	// anchor: file not in the diff
	out, err = dec(`{"findings":[` + entry(`{"tag":"bug","file":"elsewhere.go","start_line":2,"end_line":2,"issue":"i","consequence":"c","confidence":75,"severity":"P2"}`) + `]}`)
	if err != nil || out.stats.Anchor != 1 {
		t.Fatalf("anchor bucket: %+v err=%v", out.stats, err)
	}
	// anchor: in the diff's file but far outside every hunk's ±10 slack
	out, err = dec(`{"findings":[` + entry(`{"tag":"bug","file":"f.go","start_line":900,"end_line":901,"issue":"i","consequence":"c","confidence":75,"severity":"P2"}`) + `]}`)
	if err != nil || out.stats.Anchor != 1 {
		t.Fatalf("anchor far bucket: %+v err=%v", out.stats, err)
	}
	// suppression: confidence 40, not P0
	out, err = dec(`{"findings":[` + entry(`{"tag":"advisory","file":"f.go","start_line":2,"end_line":2,"issue":"i","consequence":"c","confidence":40,"severity":"P2"}`) + `]}`)
	if err != nil || out.stats.Suppression != 1 {
		t.Fatalf("suppression bucket: %+v err=%v", out.stats, err)
	}
	// suppression does not eat a P0
	out, err = dec(`{"findings":[` + entry(`{"tag":"bug","file":"f.go","start_line":2,"end_line":2,"issue":"i","consequence":"c","confidence":10,"severity":"P0"}`) + `]}`)
	if err != nil || len(out.Findings) != 1 || out.stats.Kept != 1 {
		t.Fatalf("P0 bypass: %+v err=%v", out.stats, err)
	}
	// mixed: one of each bucket plus one kept — buckets counted, warnings emitted once
	mixed := `{"findings":[` +
		entry("") + `,` + // kept
		entry(`{"tag":"bug","file":"no.go","start_line":1,"end_line":1,"issue":"i","consequence":"c","confidence":75,"severity":"P2"}`) + `,` + // anchor
		entry(`{"tag":"bug","file":"f.go"}`) + `,` + // schema (missing fields)
		entry(`{"tag":"bug","file":"f.go","start_line":0,"end_line":0,"issue":"i","consequence":"c","confidence":75,"severity":"P2"}`) + `,` + // enum (start < 1)
		entry(`{"tag":"advisory","file":"f.go","start_line":2,"end_line":2,"issue":"i","consequence":"c","confidence":30,"severity":"P3"}`) + // suppression
		`]}`
	out, err = dec(mixed)
	if err != nil || len(out.Findings) != 1 || out.stats.Schema != 1 || out.stats.Enum != 1 || out.stats.Anchor != 1 || out.stats.Suppression != 1 || out.stats.Kept != 1 {
		t.Fatalf("mixed buckets: %+v err=%v", out.stats, err)
	}
	ws := out.Warnings()
	if len(ws) != 1 || ws[0].Code != CodeLensValidate || ws[0].Detail != "rejected schema=1 enum=1 anchor=1 suppressed=1 kept=1" {
		t.Fatalf("warnings: %+v", ws)
	}

	// caps: more than MaxDeltaList KEPT entries is a hard error (not a bucket)
	var sb strings.Builder
	sb.WriteString(`{"findings":[`)
	for i := 0; i <= run.MaxDeltaList; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"tag":"bug","file":"f.go","start_line":2,"end_line":2,"issue":"i%d","consequence":"c","confidence":75,"severity":"P2"}`, i)
	}
	sb.WriteString(`]}`)
	_, err = dd.DecodeWithDiff(json.RawMessage(sb.String()), diff)
	if !errs.Is(err, CodeNodeOutputInvalid) || errs.As(err).Field("reason") != "cap" {
		t.Fatalf("kept-count cap: %v", err)
	}

	// shape failures stay hard errors at apply too (unknown top-level field, trailing data)
	if _, err = dd.DecodeWithDiff(json.RawMessage(`{"findings":[],"zzz":1}`), diff); !errs.Is(err, CodeNodeOutputInvalid) {
		t.Fatalf("unknown field: %v", err)
	}
	if _, err = dd.DecodeWithDiff(json.RawMessage(`{"findings":[]} trailing`), diff); !errs.Is(err, CodeNodeOutputInvalid) {
		t.Fatalf("trailing: %v", err)
	}

	// an empty diff anchor-rejects everything (nothing is in the diff) — kept=0, no hard error
	outI, err := dd.DecodeWithDiff(json.RawMessage(`{"findings":[`+entry("")+`]}`), machine.Diff{Text: ""})
	out = outI.(findingsOut)
	if err != nil || out.stats.Anchor != 1 || len(out.Findings) != 0 {
		t.Fatalf("empty diff: %+v err=%v", out.stats, err)
	}
}

// TestK1DecodeWithDiffFieldCap: a kept entry whose canonical "[BUG] issue consequence" text
// exceeds run.MaxText is a hard error at checkFindings (the candidate cap — the typed
// contract validates shape, the FSM caps what it will carry forward).
func TestK1DecodeWithDiffFieldCap(t *testing.T) {
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	k, _ := r.Kind(ReviewLenses)
	dd, _ := k.(machine.DiffDecoder)
	diff := machine.Diff{Text: "--- a/f.go\n+++ b/f.go\n@@ -1,4 +1,5 @@\n a\n-b\n+c\n d\n+e\n"}
	big := strings.Repeat("x", run.MaxText+16)
	payload := `{"findings":[{"tag":"bug","file":"f.go","start_line":2,"end_line":2,"issue":"` + big + `","consequence":"c","confidence":75,"severity":"P2"}]}`
	_, err := dd.DecodeWithDiff(json.RawMessage(payload), diff)
	if !errs.Is(err, CodeNodeOutputInvalid) || errs.As(err).Field("reason") != "cap" {
		t.Fatalf("field cap: %v", err)
	}
}

// TestK1DecodeWithDiffPayloadCap: individually-valid kept findings whose marshaled output
// exceeds run.MaxPayload are rejected whole — the out payload cap fires after every finding
// passed the per-finding caps (250 × ~1.2KB canonical candidates ≈ 300KB > 256KB−128).
func TestK1DecodeWithDiffPayloadCap(t *testing.T) {
	r := mustNew(t, judge.NewMock(judge.Script{}), true)
	k, _ := r.Kind(ReviewLenses)
	dd, _ := k.(machine.DiffDecoder)
	diff := machine.Diff{Text: "--- a/f.go\n+++ b/f.go\n@@ -1,4 +1,5 @@\n a\n-b\n+c\n d\n+e\n"}
	issue := strings.Repeat("y", 1100)
	var sb strings.Builder
	sb.WriteString(`{"findings":[`)
	for i := 0; i < 250; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"tag":"bug","file":"f.go","start_line":2,"end_line":2,"issue":"i%d%s","consequence":"c","confidence":75,"severity":"P2"}`, i, issue)
	}
	sb.WriteString(`]}`)
	_, err := dd.DecodeWithDiff(json.RawMessage(sb.String()), diff)
	if !errs.Is(err, CodeNodeOutputInvalid) || errs.As(err).Field("reason") != "cap" {
		t.Fatalf("payload cap: %v", err)
	}
}
