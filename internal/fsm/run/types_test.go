package run

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// R14: Time marshals as UTC RFC3339Nano, unmarshals only the Z form, and reports zero.
func TestTime(t *testing.T) {
	loc := time.FixedZone("X", 3*3600)
	tm := Time{time.Date(2026, 8, 26, 12, 0, 0, 5, loc)}
	b, err := json.Marshal(tm)
	if err != nil || string(b) != `"2026-08-26T09:00:00.000000005Z"` {
		t.Fatalf("marshal = %s (%v)", b, err)
	}
	var back Time
	if err := json.Unmarshal(b, &back); err != nil || !back.Equal(tm.Time) {
		t.Fatalf("round trip: %v %v", back, err)
	}
	if err := json.Unmarshal([]byte(`"2026-08-26T12:00:00+03:00"`), &back); err == nil {
		t.Fatalf("offset form must be rejected")
	}
	if err := json.Unmarshal([]byte(`"not-a-timeZ"`), &back); err == nil {
		t.Fatalf("unparseable Z string must be rejected")
	}
	if err := json.Unmarshal([]byte(`123`), &back); err == nil {
		t.Fatalf("non-string must be rejected")
	}
	if !(Time{}).IsZero() || tm.IsZero() {
		t.Fatalf("IsZero")
	}
}

// R9: run id validation and construction.
func TestRunIDs(t *testing.T) {
	good := RunID("sdlc-loop", time.Date(2026, 8, 26, 1, 2, 3, 4, time.UTC))
	if !strings.HasPrefix(good, "mrv-20260826-010203") || !strings.Contains(good, "fsm-sdlc-loop") {
		t.Fatalf("RunID = %q", good)
	}
	if err := ValidateRunID(good); err != nil {
		t.Fatalf("valid id rejected: %v", err)
	}
	for _, bad := range []string{"", "mrv-", "mrv-abc", "x-" + strings.Repeat("a", 10), "mrv-a/b" + strings.Repeat("c", 10), "mrv-.." + strings.Repeat("c", 10), "mrv-" + strings.Repeat("a", 201), "mrv-é" + strings.Repeat("a", 10)} {
		if err := ValidateRunID(bad); err == nil {
			t.Fatalf("accepted bad id %q", bad)
		}
	}
	if err := ValidateRunID("mrv-" + strings.Repeat("a", 8)); err != nil {
		t.Fatalf("minimum length rejected: %v", err)
	}
}

// R5: Clone is a deep copy through every slice element, map value, pointer target and RawMessage.
func TestCloneIsDeep(t *testing.T) {
	one := 1
	orig := Snapshot{
		Vars:        map[string]string{"k": "v"},
		AllowedCmds: []AllowedCmd{{Name: "n", Argv: []string{"a"}, FileHashes: map[string]string{"p": "h"}}},
		Lineage:     []string{"p"},
		Goldens:     []Golden{{Comment: "c"}},
		Findings:    []Finding{{IssueText: "i"}},
		Confirmed:   []Bug{{ID: "b", GoldenIdx: &one}},
		AllFound:    []Bug{{ID: "b", GoldenIdx: &one}},
		Status:      []BugStatus{{ID: "b"}},
		PrevUnfixed: &one,
		NodeOutputs: map[string]json.RawMessage{"n@0": json.RawMessage(`{"x":1}`)},
		Applied:     map[string]bool{"n@0": true},
		NodesRun:    []string{"n"},
		LastError:   &GateError{Code: "c"},
		Warnings:    []string{"w"},
		Pins:        []Pin{{File: "a.go", From: "x", To: "y", Test: "T"}},
		Unproven:    []Pin{{File: "u.go", From: "p", To: "q", Test: "U"}},
	}
	c := orig.Clone()
	if !reflect.DeepEqual(c, orig) {
		t.Fatalf("clone not equal")
	}
	// mutate the clone in place; original must be untouched
	c.Vars["k"] = "changed"
	c.AllowedCmds[0].Argv[0] = "changed"
	c.AllowedCmds[0].FileHashes["p"] = "changed"
	c.Lineage[0] = "changed"
	c.Goldens[0].Comment = "changed"
	c.Findings[0].IssueText = "changed"
	*c.Confirmed[0].GoldenIdx = 9
	*c.PrevUnfixed = 7
	c.NodeOutputs["n@0"][0] = '['
	c.Applied["n@0"] = false
	c.NodesRun[0] = "changed"
	c.LastError.Code = "changed"
	c.Warnings[0] = "changed"
	c.Pins[0].File = "changed"
	c.Unproven[0].File = "changed"
	if orig.Vars["k"] != "v" || orig.AllowedCmds[0].Argv[0] != "a" || orig.AllowedCmds[0].FileHashes["p"] != "h" ||
		orig.Lineage[0] != "p" || orig.Goldens[0].Comment != "c" || orig.Findings[0].IssueText != "i" ||
		*orig.Confirmed[0].GoldenIdx != 1 || *orig.AllFound[0].GoldenIdx != 1 || *orig.PrevUnfixed != 1 ||
		string(orig.NodeOutputs["n@0"]) != `{"x":1}` || !orig.Applied["n@0"] || orig.NodesRun[0] != "n" ||
		orig.LastError.Code != "c" || orig.Warnings[0] != "w" || orig.Pins[0].File != "a.go" ||
		orig.Unproven[0].File != "u.go" {
		t.Fatalf("clone shares storage with original: %+v", orig)
	}
	// and the other direction
	d := orig.Clone()
	orig.Warnings[0] = "again"
	if d.Warnings[0] != "w" {
		t.Fatalf("original mutation leaked into clone")
	}
	// reflect walk: every slice/map/pointer/RawMessage field of Snapshot was exercised above
	rt := reflect.TypeOf(Snapshot{})
	exercised := map[string]bool{"Vars": true, "AllowedCmds": true, "Lineage": true, "Goldens": true, "Findings": true, "Confirmed": true, "AllFound": true, "Status": true, "PrevUnfixed": true, "NodeOutputs": true, "Applied": true, "NodesRun": true, "LastError": true, "Warnings": true, "Pins": true, "Unproven": true}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		switch f.Type.Kind() {
		case reflect.Slice, reflect.Map, reflect.Pointer:
			if !exercised[f.Name] {
				t.Fatalf("Clone test does not exercise reference field %s", f.Name)
			}
		}
	}
	nilSnap := Snapshot{}.Clone()
	if nilSnap.PrevUnfixed != nil || nilSnap.LastError != nil {
		t.Fatalf("nil pointers must stay nil")
	}
}

// R1b mirror rows: SnapshotEqualIgnoringSeq is true only when Seq differs, false for any other field.
func TestSnapshotEqualIgnoringSeq(t *testing.T) {
	a := Snapshot{Seq: 1, State: "s", Warnings: []string{}, Vars: map[string]string{}}
	b := a.Clone()
	b.Seq = 99
	if !SnapshotEqualIgnoringSeq(a, b) {
		t.Fatalf("Seq-only difference must be equal")
	}
	rt := reflect.TypeOf(Snapshot{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Name == "Seq" {
			continue
		}
		c := a.Clone()
		v := reflect.ValueOf(&c).Elem().Field(i)
		switch f.Type.Kind() {
		case reflect.String:
			v.SetString("different")
		case reflect.Int, reflect.Int64:
			v.SetInt(v.Int() + 1)
		case reflect.Bool:
			v.SetBool(!v.Bool())
		case reflect.Slice:
			v.Set(reflect.Append(v, reflect.Zero(f.Type.Elem())))
		case reflect.Map:
			m := reflect.MakeMap(f.Type)
			m.SetMapIndex(reflect.ValueOf("k"), reflect.Zero(f.Type.Elem()))
			v.Set(m)
		case reflect.Pointer:
			v.Set(reflect.New(f.Type.Elem()))
		case reflect.Struct:
			if f.Type == reflect.TypeOf(Time{}) {
				v.Set(reflect.ValueOf(Time{time.Unix(1, 0)}))
			} else {
				v.Set(reflect.ValueOf(TokenTotals{Input: 1}))
			}
		default:
			t.Fatalf("unhandled field kind %s for %s", f.Type.Kind(), f.Name)
		}
		if SnapshotEqualIgnoringSeq(a, c) {
			t.Fatalf("difference in %s not detected", f.Name)
		}
	}
}

// R13: every reason maps to a code per the §2.4 table; String forms are stable.
func TestReasonCodeTable(t *testing.T) {
	want := map[string]string{
		ReasonEmpty: CodeAuditEmpty, ReasonVersion: CodeAuditVersion,
		ReasonFirstNotInit: CodeAuditInvalid, ReasonSecondInit: CodeAuditInvalid, ReasonSeqGap: CodeAuditInvalid,
		ReasonUnknownType: CodeAuditInvalid, ReasonBadPayload: CodeAuditInvalid, ReasonOversize: CodeAuditInvalid,
		ReasonOutputAfterDelta: CodeAuditInvalid, ReasonDeltaWithoutOutput: CodeAuditInvalid, ReasonSecondDelta: CodeAuditInvalid,
		ReasonOutputHash: CodeAuditInvalid, ReasonStatusNotSubset: CodeAuditInvalid, ReasonStatusIncomplete: CodeAuditInvalid,
		ReasonStatusDuplicate: CodeAuditInvalid, ReasonPostTerminal: CodeAuditInvalid, ReasonProvenance: CodeAuditInvalid,
		ReasonStamp: CodeAuditInvalid, ReasonInitStamp: CodeAuditInvalid, ReasonMockStamp: CodeAuditInvalid,
		ReasonNodeScope: CodeAuditInvalid, ReasonFixBaselineHead: CodeAuditInvalid, ReasonFixBaselineKind: CodeAuditInvalid,
		ReasonFixBaselineOrder: CodeAuditInvalid, ReasonUnsanctionedCmd: CodeAuditInvalid, ReasonBadOutcome: CodeAuditInvalid,
	}
	if len(want) != 26 {
		t.Fatalf("table has %d rows", len(want))
	}
	for reason, code := range want {
		if got := CodeFor(reason); got != code {
			t.Fatalf("CodeFor(%s) = %s want %s", reason, got, code)
		}
	}
	if CodeAuditEmpty != "ERR_AUDIT_EMPTY" || CodeAuditVersion != "ERR_AUDIT_VERSION" || CodeAuditInvalid != "ERR_AUDIT_INVALID" {
		t.Fatalf("code strings")
	}
	fe := &FoldError{Code: CodeAuditInvalid, Reason: ReasonSeqGap, Seq: 3, Type: "gate"}
	if fe.Error() != "ERR_AUDIT_INVALID (seq_gap) at seq 3 type gate" {
		t.Fatalf("FoldError.Error = %q", fe.Error())
	}
	se := &StoreError{Code: CodeRunLocked, Seq: 0, Detail: "held"}
	if se.Error() != "ERR_RUN_LOCKED: held" || se.Unwrap() != nil {
		t.Fatalf("StoreError.Error = %q", se.Error())
	}
	se2 := &StoreError{Code: CodeAppendRejected, Cause: fe}
	if se2.Unwrap() != fe || !strings.Contains(se2.Error(), "seq_gap") {
		t.Fatalf("StoreError wrap: %q", se2.Error())
	}
	for _, c := range []string{CodeStorePath, CodeRunExists, CodeRunNotFound, CodeRunLocked, CodeAuditChain, CodeAuditCAS, CodeAuditTorn, CodeAuditTailChanged, CodeAuditNotTorn, CodeEventTooLarge, CodeAuditFull, CodeAppendRejected} {
		if !strings.HasPrefix(c, "ERR_") {
			t.Fatalf("code %q", c)
		}
	}
}

func TestMarshalCanonicalExported(t *testing.T) {
	if got := string(MarshalCanonical(map[string]string{"a": "<b>"})); got != `{"a":"<b>"}` {
		t.Fatalf("got %s", got)
	}
}

func TestCloneKeepsAllowedCmdScalars(t *testing.T) {
	s := Snapshot{AllowedCmds: []AllowedCmd{{Name: "c", Argv: []string{"/c"}, FileHashes: map[string]string{}, TimeoutMS: 1500, Env: []string{"A"}}}}
	c := s.Clone()
	if c.AllowedCmds[0].TimeoutMS != 1500 || len(c.AllowedCmds[0].Env) != 1 || c.AllowedCmds[0].Env[0] != "A" {
		t.Fatalf("clone dropped fields: %+v", c.AllowedCmds[0])
	}
	c.AllowedCmds[0].Env[0] = "B"
	if s.AllowedCmds[0].Env[0] != "A" {
		t.Fatal("env must be copied")
	}
}

// Valid is what lets a consumer say "I do not recognise this" instead of guessing, so it has to
// be exact: the four outcomes and nothing else. A value from a newer producer, a wrong case, or
// an empty field must all fail it — the executor turns a !Valid outcome into "nothing was
// learned", and that is only safe if Valid never accepts something it should not.
func TestPinOutcomeValid(t *testing.T) {
	for _, o := range []PinOutcome{PinProven, PinSurvived, PinMalformed, PinUnverifiable} {
		if !o.Valid() {
			t.Errorf("%q is part of the schema but Valid() rejects it", o)
		}
	}
	for _, o := range []PinOutcome{"", "ok", "PROVEN", "unusable", "proven "} {
		if o.Valid() {
			t.Errorf("%q is not in the schema but Valid() accepts it", o)
		}
	}
}
