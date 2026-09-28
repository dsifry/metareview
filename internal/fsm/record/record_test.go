package record

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/jsonl"
	"github.com/dsifry/metareview/internal/runchain"
	"github.com/dsifry/metareview/internal/state"
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
	raw, err := os.ReadFile(filepath.Join(root, "metareview", "runs.jsonl"))
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
	want := `{"schemaVersion":1,"id":"mrv-root-000000001","scope":"fsm-sdlc-loop","target":{"id":"sdlc-loop@bbbbbbbbbbbb","type":"fsm"},"status":"passed","verdict":"PASS","executionMode":"fsm","attemptNumber":1,"maxAttempts":3,"baseSha":"` + base + `","headSha":"` + head + `","createdAt":"2026-08-27T01:02:03Z","updatedAt":"2026-08-27T04:05:06.000000007Z","repoRoot":"/repo","contextPackPath":"","reviewLogPath":"","mock":false,"outcome":"fixed","fsmRunDir":"metareview/runs/mrv-root-000000001/","workflowHash":"wh","workflowSource":"embedded","escalationReason":""}`
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
	recs, err := runchain.ReadRuns(asCheckoutLedger(t, root))
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
	dir := filepath.Join(root, "metareview")
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
	frag, err := os.ReadFile(filepath.Join(root, "metareview", "runs", ".torn", "runs.jsonl-42"))
	if err != nil || string(frag) != "{\"schemaVersion\":1,\"id\":\"mrv-torn" {
		t.Fatalf("fragment preserved: %q %v", frag, err)
	}
	if ok, err := Exists(root, "mrv-root-000000001"); err != nil || !ok {
		t.Fatal("exists after repair")
	}
	if recs, err := runchain.ReadRuns(asCheckoutLedger(t, root)); err != nil || len(recs) != 2 || recs[0].Verdict != "ESCALATED" {
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
	_ = os.RemoveAll(filepath.Join(root, "metareview", "runs"))
	if err := term(context.Background(), view("mrv-root-000000002", run.OutcomeFixed, nil)); err != nil {
		t.Fatal(err)
	}
	got = lines(t, root)
	if len(got) != 2 || got[0] != legacy {
		t.Fatalf("newline-less legacy row kept: %v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "metareview", "runs", ".torn")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("nothing must be moved for a decodable tail")
	}
	if recs, err := runchain.ReadRuns(asCheckoutLedger(t, root)); err != nil || len(recs) != 2 || recs[0].Verdict != "ESCALATED" {
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
	// metareview as a regular file → MkdirAll ENOTDIR
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "metareview"), []byte("x"), 0o644)
	if err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); err == nil {
		t.Fatal("ENOTDIR")
	}
	if _, err := Exists(root, "x"); err == nil {
		t.Fatal("Exists ENOTDIR")
	}
	// runs.jsonl as a directory → open fails
	root = t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "metareview", "runs.jsonl"), 0o755)
	if err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); err == nil {
		t.Fatal("directory as file")
	}
	// the torn directory cannot be created (runs is a file)
	root = t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "metareview"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "metareview", "runs"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "metareview", "runs.jsonl"), []byte("{\"torn"), 0o644)
	if err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); err == nil {
		t.Fatal("torn dir")
	}
	// the torn fragment cannot be written (unwritable .torn dir)
	if os.Getuid() != 0 {
		root = t.TempDir()
		_ = os.MkdirAll(filepath.Join(root, "metareview", "runs", ".torn"), 0o700)
		_ = os.WriteFile(filepath.Join(root, "metareview", "runs.jsonl"), []byte("{\"torn"), 0o644)
		_ = os.Chmod(filepath.Join(root, "metareview", "runs", ".torn"), 0)
		err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil))
		_ = os.Chmod(filepath.Join(root, "metareview", "runs", ".torn"), 0o700)
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
	_ = os.MkdirAll(filepath.Join(root, "metareview"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "metareview", "runs.jsonl"), append([]byte(`{"id":"`+strings.Repeat("x", 1<<20)+`"}`), '\n'), 0o644)
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

// runs.jsonl has two writers: record.appendRow here, and state.AppendJSONL used
// by the artifact, epic, task-done and pr-ready gates. appendRow held an
// advisory flock, but an advisory lock only excludes writers that also take it,
// and state.AppendJSONL takes none — it relies on O_APPEND, which makes the
// kernel place each write at the end atomically. appendRow opened without
// O_APPEND and positioned with Seek(0,2), so a write landing between that seek
// and its own Write overwrote the row instead of following it.
func TestRunsJSONLSurvivesInterleavedWriters(t *testing.T) {
	root := t.TempDir()
	const rows = 40

	var wg sync.WaitGroup
	for i := 0; i < rows; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_ = appendRow(root, Row{ID: fmt.Sprintf("mrv-record-%02d", i), HeadSHA: "h", WorkflowHash: "w"})
				return
			}
			_ = state.AppendJSONL(path(root), map[string]string{"id": fmt.Sprintf("mrv-state-%02d", i)})
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(path(root))
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("interleaved writes produced an unparseable line: %q", line)
		}
		seen++
	}
	if seen != rows {
		t.Fatalf("rows lost to interleaved writers: %d of %d survived", seen, rows)
	}
}

// runs.jsonl is written by internal/runchain and read by both runchain and this package.
// runchain sizes its scanner MaxLineBytes+2 because bufio counts the terminator; readRows
// passed the bare cap, so a row runchain writes and reads back fine made Exists fail with
// "token too long" - and it fails for the WHOLE file, so one long row hid every other run.
func TestExistsReadsAnExactlyMaxLengthRow(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "metareview"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	row := Row{SchemaVersion: 1, ID: "mrv-long-row", Scope: "fsm", Status: "passed", Target: map[string]string{"pad": ""}}
	encode := func(r Row) []byte {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}
	if pad := jsonl.MaxLineBytes - len(encode(row)); pad > 0 {
		row.Target["pad"] = strings.Repeat("x", pad)
	}
	line := encode(row)
	if len(line) != jsonl.MaxLineBytes {
		t.Fatalf("fixture is %d bytes, want exactly %d", len(line), jsonl.MaxLineBytes)
	}
	// a second row after it: a cap failure takes down the whole file, not just the long line
	second := append(encode(Row{SchemaVersion: 1, ID: "mrv-short-row", Scope: "fsm", Status: "passed"}), '\n')
	body := append(append(line, '\n'), second...)
	if err := os.WriteFile(filepath.Join(root, "metareview", "runs.jsonl"), body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, id := range []string{"mrv-long-row", "mrv-short-row"} {
		found, err := Exists(root, id)
		if err != nil {
			t.Fatalf("Exists(%s): %v", id, err)
		}
		if !found {
			t.Errorf("Exists(%s) = false, want true", id)
		}
	}
}

// asCheckoutLedger copies the common-dir ledger into a scratch checkout's .metareview/runs.jsonl, where the existing
// runchain decoder reads, so the tests keep proving a terminal row is a runs.jsonl row every reader understands.
func asCheckoutLedger(t *testing.T, common string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(common, "metareview", "runs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	checkout := t.TempDir()
	if err := os.MkdirAll(filepath.Join(checkout, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, ".metareview", "runs.jsonl"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return checkout
}

// #173: a 0.13.x store kept the FSM's terminal rows in the main checkout's .metareview/runs.jsonl, among that
// checkout's own review rows. Migration copies the FSM rows (scope fsm-*) into the common-dir ledger so run ids stay
// unique across it; it leaves the review rows alone, is idempotent, and reports an id whose row differs.
func TestMigrateLegacyRows(t *testing.T) {
	ctx := context.Background()
	checkout, common := t.TempDir(), t.TempDir()
	// A legacy ledger: one FSM row written the 0.13.x way, one review row, one conflicting FSM row.
	old := t.TempDir()
	if err := Terminal(old, fixedClock)(ctx, view("mrv-root-000000001", run.OutcomeFixed, []string{})); err != nil {
		t.Fatal(err)
	}
	fsmRow, _ := os.ReadFile(filepath.Join(old, "metareview", "runs.jsonl"))
	// 0.13.x named the run dir relative to the main checkout.
	fsmRow = []byte(strings.Replace(string(fsmRow), `"fsmRunDir":"metareview/runs/`, `"fsmRunDir":".metareview/runs/`, 1))
	if !strings.Contains(string(fsmRow), `".metareview/runs/`) {
		t.Fatalf("fixture must carry a 0.13.x fsmRunDir: %s", fsmRow)
	}
	legacy := string(fsmRow) + `{"schemaVersion":1,"id":"mrv-review-1","scope":"pr-ready","verdict":"PASS"}` + "\n"
	_ = os.MkdirAll(filepath.Join(checkout, ".metareview"), 0o755)
	if err := os.WriteFile(filepath.Join(checkout, ".metareview", "runs.jsonl"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	copied, conflicts, err := MigrateLegacyRows(checkout, common)
	if err != nil || len(copied) != 1 || copied[0] != "mrv-root-000000001" || len(conflicts) != 0 {
		t.Fatalf("migrate: %v %v %v", copied, conflicts, err)
	}
	if ok, _ := Exists(common, "mrv-root-000000001"); !ok {
		t.Fatal("the FSM row must be in the common-dir ledger")
	}
	// A migrated row names its run where it now lives, like a row written after #173 (relative to the common dir).
	if rows, _, err := readRowsFile(path(common)); err != nil || len(rows) != 1 || rows[0].FSMRunDir != "metareview/runs/mrv-root-000000001/" {
		t.Fatalf("migrated row FSMRunDir: %+v %v", rows, err)
	}
	if ok, _ := Exists(common, "mrv-review-1"); ok {
		t.Fatal("a review row belongs to its checkout and must not be copied")
	}
	if copied, _, err := MigrateLegacyRows(checkout, common); err != nil || len(copied) != 0 {
		t.Fatalf("re-running must be a no-op: %v %v", copied, err)
	}
	// A legacy FSM row whose id the ledger holds for a different head is a conflict, reported, not merged.
	conflicting := strings.Replace(string(fsmRow), `"headSha":"`, `"headSha":"f`, 1)
	_ = os.WriteFile(filepath.Join(checkout, ".metareview", "runs.jsonl"), []byte(conflicting), 0o644)
	if _, conflicts, err := MigrateLegacyRows(checkout, common); err != nil || len(conflicts) != 1 {
		t.Fatalf("conflict: %v %v", conflicts, err)
	}
	// No legacy ledger, or an unreadable one.
	if copied, conflicts, err := MigrateLegacyRows(t.TempDir(), common); err != nil || len(copied)+len(conflicts) != 0 {
		t.Fatalf("no legacy ledger: %v %v %v", copied, conflicts, err)
	}
	bad := t.TempDir()
	_ = os.MkdirAll(filepath.Join(bad, ".metareview", "runs.jsonl"), 0o755)
	if _, _, err := MigrateLegacyRows(bad, common); err == nil {
		t.Fatal("an unreadable legacy ledger must surface")
	}
	notDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(notDir, ".metareview"), []byte("x"), 0o644) // stat fails with ENOTDIR
	if _, _, err := MigrateLegacyRows(notDir, common); err == nil {
		t.Fatal("a legacy ledger that cannot be stat'd must surface")
	}
	// The common ledger cannot be written.
	blocked := t.TempDir()
	_ = os.WriteFile(filepath.Join(blocked, "metareview"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(checkout, ".metareview", "runs.jsonl"), fsmRow, 0o644)
	if _, _, err := MigrateLegacyRows(checkout, blocked); err == nil {
		t.Fatal("an unreadable ledger must surface")
	}
	if os.Getuid() != 0 {
		readOnly := t.TempDir()
		_ = os.MkdirAll(filepath.Join(readOnly, "metareview"), 0o500)
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(readOnly, "metareview"), 0o700) })
		if _, _, err := MigrateLegacyRows(checkout, readOnly); err == nil {
			t.Fatal("an unwritable ledger must surface")
		}
	}
}

// The legacy ledger is the main checkout's live review ledger (lock-free writers), so a malformed line in it must not
// stop migration — its FSM rows are copied, the rest skipped — and once migrated it is not re-read on every fsm
// command: a stamp of its size skips it until an older binary appends more (#173 review).
// A run MigrateLegacyRuns left behind as a collision keeps its row behind too: the store's run of that id is a
// different one, whose own terminal row must not be pre-empted by the legacy row.
func TestMigrateLegacyRowsSkipsCollidedRuns(t *testing.T) {
	ctx := context.Background()
	checkout, common, old := t.TempDir(), t.TempDir(), t.TempDir()
	if err := Terminal(old, fixedClock)(ctx, view("mrv-collide-00000001", run.OutcomeFixed, []string{})); err != nil {
		t.Fatal(err)
	}
	row, _ := os.ReadFile(filepath.Join(old, "metareview", "runs.jsonl"))
	_ = os.MkdirAll(filepath.Join(checkout, ".metareview"), 0o755)
	if err := os.WriteFile(filepath.Join(checkout, ".metareview", "runs.jsonl"), row, 0o644); err != nil {
		t.Fatal(err)
	}
	copied, conflicts, err := MigrateLegacyRows(checkout, common, "mrv-collide-00000001")
	if err != nil || len(copied)+len(conflicts) != 0 {
		t.Fatalf("a collided run's row must be skipped: %v %v %v", copied, conflicts, err)
	}
	if ok, _ := Exists(common, "mrv-collide-00000001"); ok {
		t.Fatal("the collided id must stay free for the store's own run")
	}
}

func TestMigrateLegacyRowsIsLenientAndRunsOnce(t *testing.T) {
	ctx := context.Background()
	checkout, common := t.TempDir(), t.TempDir()
	old := t.TempDir()
	if err := Terminal(old, fixedClock)(ctx, view("mrv-root-000000001", run.OutcomeFixed, []string{})); err != nil {
		t.Fatal(err)
	}
	fsmRow, _ := os.ReadFile(filepath.Join(old, "metareview", "runs.jsonl"))
	legacy := filepath.Join(checkout, ".metareview", "runs.jsonl")
	_ = os.MkdirAll(filepath.Dir(legacy), 0o755)
	_ = os.WriteFile(legacy, []byte("not json\n"+string(fsmRow)+"{\"torn"), 0o644)
	copied, _, err := MigrateLegacyRows(checkout, common)
	if err != nil || len(copied) != 1 {
		t.Fatalf("a malformed legacy line must not stop migration: %v %v", copied, err)
	}
	// Migrated: the next call does not read the legacy file at all (here: unreadable, and still no error).
	if os.Getuid() != 0 {
		_ = os.Chmod(legacy, 0)
		t.Cleanup(func() { _ = os.Chmod(legacy, 0o644) })
		if copied, _, err := MigrateLegacyRows(checkout, common); err != nil || len(copied) != 0 {
			t.Fatalf("a migrated legacy ledger must be skipped: %v %v", copied, err)
		}
		_ = os.Chmod(legacy, 0o644)
	}
	// An older binary appends another FSM row: the size changed, so it is migrated too.
	other := strings.Replace(string(fsmRow), "mrv-root-000000001", "mrv-root-000000002", 1)
	f, _ := os.OpenFile(legacy, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("\n" + other)
	_ = f.Close()
	if copied, _, err := MigrateLegacyRows(checkout, common); err != nil || len(copied) != 1 || copied[0] != "mrv-root-000000002" {
		t.Fatalf("rows appended after migration must be picked up: %v %v", copied, err)
	}
}
