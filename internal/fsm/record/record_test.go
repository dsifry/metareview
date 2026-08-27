package record

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/runchain"
)

const (
	base = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	head = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func view(id string, outcome run.Outcome, lineage []string) machine.View {
	return machine.View{RunID: id, Workflow: "sdlc-loop", Snapshot: run.Snapshot{
		RunID: id, Workflow: "sdlc-loop", WorkflowHash: "wh", WorkflowSource: "embedded", BaseSHA: base, Head: head,
		RepoRoot: "/repo", Outcome: outcome, Lineage: lineage, CreatedAt: run.Time{Time: time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)},
	}}
}

func fixedClock() run.Time { return run.Time{Time: time.Date(2026, 8, 27, 4, 5, 6, 7, time.UTC)} }

func lines(t *testing.T, root string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
}

// exactKeys is the §6 key set; DisallowUnknownFields rejects extras and the presence check rejects omissions.
var exactKeys = []string{"schemaVersion", "id", "scope", "target", "status", "verdict", "executionMode", "attemptNumber", "maxAttempts", "baseSha", "headSha", "createdAt", "updatedAt", "repoRoot", "contextPackPath", "reviewLogPath", "mock", "outcome", "fsmRunDir", "workflowHash", "workflowSource", "escalationReason"}

func assertKeys(t *testing.T, line string, previous bool) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(line))
	dec.DisallowUnknownFields()
	var r Row
	if err := dec.Decode(&r); err != nil {
		t.Fatalf("extra key: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(line), &m)
	for _, k := range exactKeys {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing key %s", k)
		}
	}
	if _, ok := m["previousRunId"]; ok != previous {
		t.Fatalf("previousRunId presence: %v", ok)
	}
}

func TestF9GoldenRows(t *testing.T) {
	root := t.TempDir()
	term := Terminal(root, fixedClock)
	ctx := context.Background()
	if err := term(ctx, view("mrv-root-000000001", run.OutcomeFixed, []string{})); err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":1,"id":"mrv-root-000000001","scope":"fsm-sdlc-loop","target":{"id":"sdlc-loop@bbbbbbbbbbbb","type":"fsm"},"status":"passed","verdict":"PASS","executionMode":"fsm","attemptNumber":1,"maxAttempts":3,"baseSha":"` + base + `","headSha":"` + head + `","createdAt":"2026-08-27T01:02:03Z","updatedAt":"2026-08-27T04:05:06.000000007Z","repoRoot":"/repo","contextPackPath":"","reviewLogPath":"","mock":false,"outcome":"fixed","fsmRunDir":".metareview/runs/mrv-root-000000001/","workflowHash":"wh","workflowSource":"embedded","escalationReason":""}`
	got := lines(t, root)
	if len(got) != 1 || got[0] != want {
		t.Fatalf("golden root:\n%s\n%s", got[0], want)
	}
	assertKeys(t, got[0], false)
	// grandchild (attempt 3) ending non-PASS → escalated
	gc := view("mrv-gc-00000000001", run.OutcomeOverflow, []string{"mrv-root-000000001", "mrv-c1-00000000001"})
	gc.Snapshot.ParentRunID = "mrv-c1-00000000001"
	if err := term(ctx, gc); err != nil {
		t.Fatal(err)
	}
	got = lines(t, root)
	if len(got) != 2 || !strings.Contains(got[1], `"previousRunId":"mrv-c1-00000000001","attemptNumber":3`) || !strings.Contains(got[1], `"status":"escalated","verdict":"ESCALATED"`) || !strings.Contains(got[1], `"escalationReason":"attempt 3 of a fork lineage ended overflow"`) {
		t.Fatalf("grandchild: %s", got[1])
	}
	assertKeys(t, got[1], true)
	// the existing decoder reads both
	recs, err := runchain.ReadRuns(root)
	if err != nil || len(recs) != 2 || recs[1].AttemptNumber != 3 || recs[1].Verdict != "ESCALATED" || recs[0].Scope != "fsm-sdlc-loop" {
		t.Fatalf("runchain decode: %v %+v", err, recs)
	}
	// idempotent on the resume path: same view again → still two rows; parent row first, child row second, then parent again
	if err := term(ctx, view("mrv-root-000000001", run.OutcomeFixed, []string{})); err != nil || len(lines(t, root)) != 2 {
		t.Fatalf("idempotent: %v", err)
	}
	// Exists
	for _, c := range []struct {
		id   string
		want bool
	}{{"mrv-root-000000001", true}, {"mrv-gc-00000000001", true}, {"mrv-none-000000001", false}} {
		if ok, err := Exists(root, c.id); err != nil || ok != c.want {
			t.Fatalf("exists %s: %v %v", c.id, ok, err)
		}
	}
	if ok, err := Exists(t.TempDir(), "mrv-root-000000001"); err != nil || ok {
		t.Fatalf("missing file: %v %v", ok, err)
	}
}

func TestVerdictMap(t *testing.T) {
	for _, c := range []struct {
		outcome run.Outcome
		lineage int
		verdict string
		status  string
	}{
		{run.OutcomeFixed, 0, VerdictPass, StatusPassed}, {run.OutcomeClean, 2, VerdictPass, StatusPassed},
		{run.OutcomeReviewed, 0, VerdictNeedsRevision, StatusNeedsRevision}, {run.OutcomeStalled, 1, VerdictNeedsRevision, StatusNeedsRevision},
		{run.OutcomeFailed, 0, VerdictNeedsRevision, StatusNeedsRevision}, {run.OutcomeOverflow, 0, VerdictNeedsRevision, StatusNeedsRevision},
		{run.OutcomeCustom, 0, VerdictNeedsRevision, StatusNeedsRevision},
		{run.OutcomeFailed, 2, VerdictEscalated, StatusEscalated}, {run.OutcomeReviewed, 3, VerdictEscalated, StatusEscalated},
		{run.OutcomeFixed, 3, VerdictPass, StatusPassed},
	} {
		lin := make([]string, c.lineage)
		r := RowFor(view("mrv-x-000000000001", c.outcome, lin), fixedClock())
		if r.Verdict != c.verdict || r.Status != c.status || r.AttemptNumber != c.lineage+1 {
			t.Fatalf("%s/%d: %+v", c.outcome, c.lineage, r)
		}
		if (r.EscalationReason != "") != (c.verdict == VerdictEscalated) {
			t.Fatalf("reason: %+v", r)
		}
	}
	v := view("mrv-x-000000000001", run.OutcomeFixed, nil)
	v.Snapshot.MockTainted = true
	v.Snapshot.WorkflowSource = ""
	v.Snapshot.BaseSHA = "abc"
	r := RowFor(v, fixedClock())
	if !r.Mock || r.WorkflowSource != "unknown" || r.Target["id"] != "sdlc-loop@abc" {
		t.Fatalf("tainted/unknown/short: %+v", r)
	}
	v.Snapshot.MockTainted, v.Snapshot.Mock = false, "m#1234567890abcdef"
	if !RowFor(v, fixedClock()).Mock {
		t.Fatal("mock run")
	}
}

func TestTornTailWritePath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".metareview")
	_ = os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "runs.jsonl")
	// a legacy review row (complete) + a torn fragment
	legacy := `{"schemaVersion":1,"id":"mrv-legacy-00000001","scope":"task-done","target":{"type":"task","id":"t"},"status":"escalated","verdict":"ESCALATED","previousRunId":"","attemptNumber":3,"maxAttempts":3,"headSha":"` + head + `"}`
	_ = os.WriteFile(p, []byte(legacy+"\n{\"schemaVersion\":1,\"id\":\"mrv-torn"), 0o644)
	nanos = func() int64 { return 42 }
	term := Terminal(root, fixedClock)
	if err := term(context.Background(), view("mrv-root-000000001", run.OutcomeFixed, nil)); err != nil {
		t.Fatal(err)
	}
	got := lines(t, root)
	if len(got) != 2 || got[0] != legacy || !strings.Contains(got[1], `"id":"mrv-root-000000001"`) {
		t.Fatalf("after torn: %v", got)
	}
	frag, err := os.ReadFile(filepath.Join(root, ".metareview", "runs", ".torn", "runs.jsonl-42"))
	if err != nil || string(frag) != "{\"schemaVersion\":1,\"id\":\"mrv-torn" {
		t.Fatalf("fragment preserved: %q %v", frag, err)
	}
	if ok, err := Exists(root, "mrv-root-000000001"); err != nil || !ok {
		t.Fatal("exists after repair")
	}
	if recs, err := runchain.ReadRuns(root); err != nil || len(recs) != 2 || recs[0].Verdict != "ESCALATED" {
		t.Fatalf("runchain after repair: %v %+v", err, recs)
	}
	// Exists tolerates a torn tail without repairing it
	_ = os.WriteFile(p, []byte(legacy+"\n{\"torn"), 0o644)
	if ok, err := Exists(root, "mrv-legacy-00000001"); err != nil || !ok {
		t.Fatalf("exists with torn tail: %v %v", ok, err)
	}
	if raw, _ := os.ReadFile(p); !strings.HasSuffix(string(raw), "{\"torn") {
		t.Fatal("Exists must not repair")
	}
	// a newline-less but decodable final row is a row: "\n" is written first, nothing moved
	_ = os.WriteFile(p, []byte(legacy), 0o644)
	_ = os.RemoveAll(filepath.Join(root, ".metareview", "runs"))
	if err := term(context.Background(), view("mrv-root-000000002", run.OutcomeFixed, nil)); err != nil {
		t.Fatal(err)
	}
	got = lines(t, root)
	if len(got) != 2 || got[0] != legacy {
		t.Fatalf("newline-less legacy row kept: %v", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".metareview", "runs", ".torn")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("nothing must be moved for a decodable tail")
	}
	if recs, err := runchain.ReadRuns(root); err != nil || len(recs) != 2 || recs[0].Verdict != "ESCALATED" {
		t.Fatalf("legacy escalation still visible: %v", err)
	}
	// blank lines are skipped
	_ = os.WriteFile(p, []byte("\n"+legacy+"\n\n"), 0o644)
	if err := term(context.Background(), view("mrv-root-000000003", run.OutcomeFixed, nil)); err != nil {
		t.Fatal(err)
	}
	if ok, _ := Exists(root, "mrv-root-000000003"); !ok {
		t.Fatal("row after blank lines")
	}
	// malformed terminated final line and malformed middle line refuse, nothing written
	for _, body := range []string{legacy + "\nnot json\n", "not json\n" + legacy + "\n"} {
		_ = os.WriteFile(p, []byte(body), 0o644)
		err := term(context.Background(), view("mrv-root-000000004", run.OutcomeFixed, nil))
		if !errs.Is(err, CodeRunsJSONL) || errs.As(err).Fields["reason"] != "malformed" {
			t.Fatalf("malformed: %v", err)
		}
		if raw, _ := os.ReadFile(p); string(raw) != body {
			t.Fatal("nothing written on malformed")
		}
		if _, err := Exists(root, "x"); !errs.Is(err, CodeRunsJSONL) {
			t.Fatal("Exists reports malformed")
		}
	}
	// a planted row with the id but a different head → id_conflict
	planted := strings.Replace(legacy, "mrv-legacy-00000001", "mrv-root-000000009", 1)
	_ = os.WriteFile(p, []byte(planted+"\n"), 0o644)
	err = term(context.Background(), view("mrv-root-000000009", run.OutcomeFixed, nil))
	if !errs.Is(err, CodeRunsJSONL) || errs.As(err).Fields["reason"] != "id_conflict" {
		t.Fatalf("id_conflict: %v", err)
	}
	// idempotency when the matching row is not the last line
	_ = os.WriteFile(p, nil, 0o644)
	_ = term(context.Background(), view("mrv-root-000000001", run.OutcomeFixed, nil))
	_ = term(context.Background(), view("mrv-c1-00000000001", run.OutcomeFixed, []string{"mrv-root-000000001"}))
	if err := term(context.Background(), view("mrv-root-000000001", run.OutcomeFixed, nil)); err != nil || len(lines(t, root)) != 2 {
		t.Fatalf("middle-row idempotency: %v %v", err, lines(t, root))
	}
}

func TestWriteErrors(t *testing.T) {
	ctx := context.Background()
	// .metareview as a regular file → MkdirAll ENOTDIR
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, ".metareview"), []byte("x"), 0o644)
	if err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); err == nil {
		t.Fatal("ENOTDIR")
	}
	if _, err := Exists(root, "x"); err == nil {
		t.Fatal("Exists ENOTDIR")
	}
	// runs.jsonl as a directory → open fails
	root = t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".metareview", "runs.jsonl"), 0o755)
	if err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); err == nil {
		t.Fatal("directory as file")
	}
	// the torn directory cannot be created (runs is a file)
	root = t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".metareview"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".metareview", "runs"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, ".metareview", "runs.jsonl"), []byte("{\"torn"), 0o644)
	if err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); err == nil {
		t.Fatal("torn dir")
	}
	// the torn fragment cannot be written (unwritable .torn dir)
	if os.Getuid() != 0 {
		root = t.TempDir()
		_ = os.MkdirAll(filepath.Join(root, ".metareview", "runs", ".torn"), 0o700)
		_ = os.WriteFile(filepath.Join(root, ".metareview", "runs.jsonl"), []byte("{\"torn"), 0o644)
		_ = os.Chmod(filepath.Join(root, ".metareview", "runs", ".torn"), 0)
		err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil))
		_ = os.Chmod(filepath.Join(root, ".metareview", "runs", ".torn"), 0o700)
		if err == nil {
			t.Fatal("unwritable torn dir")
		}
	}
	// flock failure
	orig := flock
	flock = func(int, int) error { return errors.New("flock failed") }
	if err := Terminal(t.TempDir(), fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); err == nil || err.Error() != "flock failed" {
		t.Fatalf("flock: %v", err)
	}
	flock = orig
	// a line longer than the scanner buffer
	root = t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".metareview"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".metareview", "runs.jsonl"), append([]byte(`{"id":"`+strings.Repeat("x", 1<<20)+`"}`), '\n'), 0o644)
	if _, err := Exists(root, "x"); err == nil {
		t.Fatal("oversized line")
	}
	// ctx cancelled
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := Terminal(t.TempDir(), fixedClock)(cctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx: %v", err)
	}
	if nowNanos() == 0 {
		t.Fatal("clock")
	}
}
