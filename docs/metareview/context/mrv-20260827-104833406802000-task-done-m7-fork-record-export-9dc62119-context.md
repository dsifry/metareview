# metareview task-done context

Run ID: `mrv-20260827-104833406802000-task-done-m7-fork-record-export-9dc62119`

## Task

# M7 — fork/resume, diff, export, runs.jsonl record

Implements spec 3 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md`): the `run` amendments (`WorkflowSource`,
`TornFiles`/`MaxEvents`/`Counted`, incomplete-fork rule), `machine.Fork`/`VerifyOrigin`/`DiffRuns` + `machine.Decision` +
`ERR_FORK_INCOMPLETE`, `kind.Decision` + judge-less registries, `internal/fsm/record` (terminal recorder, `Exists`,
torn-safe writer), `internal/fsm/export` (redaction table, redacted snapshot, manifest, `FS` seam).

Done when every touched `internal/fsm/*` package is at exactly 100% statement coverage (`tests/coverage.sh`) and
`go vet` is clean.


## Git

- Base: `f9963919de37b37ea33b00f56afbb4b8f1f0251d`
- Head: `5164baf2a9a3c5620388df9bd09dcf342208eae5`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `23240`
- Filtered diff bytes: `23240`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `62e5d051ce03d692`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/tasks/m7-fork-record-export.md
- internal/fsm/record/clock.go
- internal/fsm/record/record.go
- internal/fsm/record/record_test.go

### Shards
- shard-01: docs/tasks/m7-fork-record-export.md, internal/fsm/record/clock.go, internal/fsm/record/record.go, internal/fsm/record/record_test.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- internal/fsm/record/clock.go
- internal/fsm/record/record.go
- internal/fsm/record/record_test.go
- docs/tasks/m7-fork-record-export.md

## Diff

```diff
diff --git a/internal/fsm/record/clock.go b/internal/fsm/record/clock.go
new file mode 100644
index 0000000..dec566d
--- /dev/null
+++ b/internal/fsm/record/clock.go
@@ -0,0 +1,5 @@
+package record
+
+import "time"
+
+func nowNanos() int64 { return time.Now().UnixNano() }
diff --git a/internal/fsm/record/record.go b/internal/fsm/record/record.go
new file mode 100644
index 0000000..a4af487
--- /dev/null
+++ b/internal/fsm/record/record.go
@@ -0,0 +1,234 @@
+// Package record writes the runs.jsonl row for a terminal FSM run (spec 3 §6) and answers the two questions the CLI
+// asks of that file: does a row exist for an id, and the recorder itself as machine.Deps.Terminal.
+package record
+
+import (
+	"bufio"
+	"bytes"
+	"context"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"os"
+	"path/filepath"
+	"strings"
+	"syscall"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/machine"
+	"github.com/dsifry/metareview/internal/fsm/run"
+)
+
+// CodeRunsJSONL is raised for an undecodable or conflicting runs.jsonl line.
+const CodeRunsJSONL = "ERR_RUNS_JSONL"
+
+// Row is the runs.jsonl record for an FSM run. Keys are camelCase (the existing writers' file); new keys are additive.
+type Row struct {
+	SchemaVersion    int               `json:"schemaVersion"`
+	ID               string            `json:"id"`
+	Scope            string            `json:"scope"`
+	Target           map[string]string `json:"target"`
+	Status           string            `json:"status"`
+	Verdict          string            `json:"verdict"`
+	ExecutionMode    string            `json:"executionMode"`
+	PreviousRunID    string            `json:"previousRunId,omitempty"`
+	AttemptNumber    int               `json:"attemptNumber"`
+	MaxAttempts      int               `json:"maxAttempts"`
+	BaseSHA          string            `json:"baseSha"`
+	HeadSHA          string            `json:"headSha"`
+	CreatedAt        string            `json:"createdAt"`
+	UpdatedAt        string            `json:"updatedAt"`
+	RepoRoot         string            `json:"repoRoot"`
+	ContextPackPath  string            `json:"contextPackPath"`
+	ReviewLogPath    string            `json:"reviewLogPath"`
+	Mock             bool              `json:"mock"`
+	Outcome          string            `json:"outcome"`
+	FSMRunDir        string            `json:"fsmRunDir"`
+	WorkflowHash     string            `json:"workflowHash"`
+	WorkflowSource   string            `json:"workflowSource"`
+	EscalationReason string            `json:"escalationReason"`
+}
+
+// Verdicts and statuses (the existing writers' vocabulary).
+const (
+	VerdictPass          = "PASS"
+	VerdictNeedsRevision = "NEEDS_REVISION"
+	VerdictEscalated     = "ESCALATED"
+	StatusPassed         = "passed"
+	StatusNeedsRevision  = "needs-revision"
+	StatusEscalated      = "escalated"
+)
+
+func path(root string) string { return filepath.Join(root, ".metareview", "runs.jsonl") }
+
+// RowFor maps a terminal view to its row (spec 3 §6).
+func RowFor(v machine.View, now run.Time) Row {
+	s := v.Snapshot
+	attempt := machine.Attempt(s)
+	verdict, status := VerdictNeedsRevision, StatusNeedsRevision
+	reason := ""
+	if machine.PassOutcome(s.Outcome) {
+		verdict, status = VerdictPass, StatusPassed
+	} else if attempt >= machine.MaxAttempts {
+		verdict, status = VerdictEscalated, StatusEscalated
+		reason = fmt.Sprintf("attempt %d of a fork lineage ended %s", attempt, s.Outcome)
+	}
+	base := s.BaseSHA
+	if len(base) > 12 {
+		base = base[:12]
+	}
+	source := s.WorkflowSource
+	if source == "" {
+		source = "unknown"
+	}
+	return Row{
+		SchemaVersion: 1, ID: v.RunID, Scope: "fsm-" + s.Workflow, Target: map[string]string{"type": "fsm", "id": s.Workflow + "@" + base},
+		Status: status, Verdict: verdict, ExecutionMode: "fsm", PreviousRunID: s.ParentRunID, AttemptNumber: attempt, MaxAttempts: machine.MaxAttempts,
+		BaseSHA: s.BaseSHA, HeadSHA: s.Head, CreatedAt: s.CreatedAt.UTC().Format(rfc3339Nano), UpdatedAt: now.UTC().Format(rfc3339Nano),
+		RepoRoot: s.RepoRoot, Mock: s.Mock != "" || s.MockTainted, Outcome: string(s.Outcome), FSMRunDir: ".metareview/runs/" + v.RunID + "/",
+		WorkflowHash: s.WorkflowHash, WorkflowSource: source, EscalationReason: reason,
+	}
+}
+
+const rfc3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
+
+// Terminal returns the machine.Deps.Terminal implementation: one row per run, appended at terminal only, idempotent
+// by id; a pre-existing row for the id with a different head or workflow hash is an error, never a silent skip.
+func Terminal(root string, clock func() run.Time) func(context.Context, machine.View) error {
+	return func(ctx context.Context, v machine.View) error {
+		if err := ctx.Err(); err != nil {
+			return err
+		}
+		row := RowFor(v, clock())
+		return appendRow(root, row)
+	}
+}
+
+// Exists reports whether runs.jsonl already holds a row for id (read-only; a torn tail is tolerated, never repaired).
+func Exists(root, id string) (bool, error) {
+	rows, _, err := readRows(root)
+	if err != nil {
+		return false, err
+	}
+	for _, r := range rows {
+		if r.ID == id {
+			return true, nil
+		}
+	}
+	return false, nil
+}
+
+// tail describes what readRows found after the last newline.
+type tail struct {
+	fragment []byte // an undecodable unterminated fragment (a torn append); nil when the file ends cleanly
+	newline  bool   // a decodable newline-less final row was found: the writer must add "\n" first
+	offset   int64  // byte offset where the fragment starts (== file size when there is none)
+}
+
+// readRows decodes runs.jsonl with the tolerant scanner of spec 3 §6: blank lines are skipped, a final line without a
+// trailing newline that decodes is a row, an undecodable unterminated fragment is reported as the tail, any other
+// undecodable line is ERR_RUNS_JSONL{malformed}.
+func readRows(root string) ([]Row, tail, error) {
+	raw, err := os.ReadFile(path(root))
+	if errors.Is(err, os.ErrNotExist) {
+		return nil, tail{}, nil
+	}
+	if err != nil {
+		return nil, tail{}, err
+	}
+	var rows []Row
+	complete := raw
+	t := tail{offset: int64(len(raw))}
+	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
+		i := bytes.LastIndexByte(raw, '\n') + 1
+		last := raw[i:]
+		var r Row
+		if json.Unmarshal(last, &r) == nil {
+			t.newline = true
+			complete = append(append([]byte{}, raw[:i]...), last...)
+			complete = append(complete, '\n')
+		} else {
+			t.fragment, t.offset = last, int64(i)
+			complete = raw[:i]
+		}
+	}
+	sc := bufio.NewScanner(bytes.NewReader(complete))
+	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
+	line := 0
+	for sc.Scan() {
+		line++
+		text := strings.TrimSpace(sc.Text())
+		if text == "" {
+			continue
+		}
+		var r Row
+		if err := json.Unmarshal([]byte(text), &r); err != nil {
+			return nil, tail{}, errs.E(CodeRunsJSONL, "runs.jsonl line "+fmt.Sprint(line)+" is not a JSON object", "line", fmt.Sprint(line), "reason", "malformed")
+		}
+		rows = append(rows, r)
+	}
+	if err := sc.Err(); err != nil {
+		return nil, tail{}, err
+	}
+	return rows, t, nil
+}
+
+// appendRow is record's own writer: flock, read, repair or terminate the tail, check the id, append + fsync.
+func appendRow(root string, row Row) error {
+	p := path(root)
+	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
+		return err
+	}
+	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o644)
+	if err != nil {
+		return err
+	}
+	defer f.Close()
+	if err := flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
+		return err
+	}
+	defer func() { _ = flock(int(f.Fd()), syscall.LOCK_UN) }()
+	rows, t, err := readRows(root)
+	if err != nil {
+		return err
+	}
+	for _, r := range rows {
+		if r.ID != row.ID {
+			continue
+		}
+		if r.HeadSHA != row.HeadSHA || r.WorkflowHash != row.WorkflowHash {
+			return errs.E(CodeRunsJSONL, "a row with this id already exists for a different head or workflow", "line", row.ID, "reason", "id_conflict")
+		}
+		return nil // idempotent
+	}
+	line := run.MarshalCanonical(row)
+	if t.newline {
+		line = append([]byte{'\n'}, line...)
+	}
+	var steps []func() error
+	if t.fragment != nil {
+		torn := filepath.Join(root, ".metareview", "runs", ".torn")
+		steps = append(steps,
+			func() error { return os.MkdirAll(torn, 0o700) },
+			func() error {
+				return os.WriteFile(filepath.Join(torn, fmt.Sprintf("runs.jsonl-%d", nanos())), t.fragment, 0o600)
+			},
+			func() error { return f.Truncate(t.offset) })
+	}
+	steps = append(steps,
+		func() error { _, err := f.Seek(0, 2); return err },
+		func() error { _, err := f.Write(append(line, '\n')); return err },
+		f.Sync)
+	for _, step := range steps {
+		if err := step(); err != nil {
+			return err
+		}
+	}
+	return nil
+}
+
+// flock is the advisory lock seam (tests inject a failing one).
+var flock = syscall.Flock
+
+// nanos is the torn-fragment name clock; tests may override it.
+var nanos = nowNanos
diff --git a/internal/fsm/record/record_test.go b/internal/fsm/record/record_test.go
new file mode 100644
index 0000000..9cb9ecf
--- /dev/null
+++ b/internal/fsm/record/record_test.go
@@ -0,0 +1,298 @@
+package record
+
+import (
+	"context"
+	"encoding/json"
+	"errors"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+	"time"
+
+	"github.com/dsifry/metareview/internal/fsm/errs"
+	"github.com/dsifry/metareview/internal/fsm/machine"
+	"github.com/dsifry/metareview/internal/fsm/run"
+	"github.com/dsifry/metareview/internal/runchain"
+)
+
+const (
+	base = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
+	head = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
+)
+
+func view(id string, outcome run.Outcome, lineage []string) machine.View {
+	return machine.View{RunID: id, Workflow: "sdlc-loop", Snapshot: run.Snapshot{
+		RunID: id, Workflow: "sdlc-loop", WorkflowHash: "wh", WorkflowSource: "embedded", BaseSHA: base, Head: head,
+		RepoRoot: "/repo", Outcome: outcome, Lineage: lineage, CreatedAt: run.Time{Time: time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)},
+	}}
+}
+
+func fixedClock() run.Time { return run.Time{Time: time.Date(2026, 8, 27, 4, 5, 6, 7, time.UTC)} }
+
+func lines(t *testing.T, root string) []string {
+	t.Helper()
+	raw, err := os.ReadFile(filepath.Join(root, ".metareview", "runs.jsonl"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	return strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
+}
+
+// exactKeys is the §6 key set; DisallowUnknownFields rejects extras and the presence check rejects omissions.
+var exactKeys = []string{"schemaVersion", "id", "scope", "target", "status", "verdict", "executionMode", "attemptNumber", "maxAttempts", "baseSha", "headSha", "createdAt", "updatedAt", "repoRoot", "contextPackPath", "reviewLogPath", "mock", "outcome", "fsmRunDir", "workflowHash", "workflowSource", "escalationReason"}
+
+func assertKeys(t *testing.T, line string, previous bool) {
+	t.Helper()
+	dec := json.NewDecoder(strings.NewReader(line))
+	dec.DisallowUnknownFields()
+	var r Row
+	if err := dec.Decode(&r); err != nil {
+		t.Fatalf("extra key: %v", err)
+	}
+	var m map[string]any
+	_ = json.Unmarshal([]byte(line), &m)
+	for _, k := range exactKeys {
+		if _, ok := m[k]; !ok {
+			t.Fatalf("missing key %s", k)
+		}
+	}
+	if _, ok := m["previousRunId"]; ok != previous {
+		t.Fatalf("previousRunId presence: %v", ok)
+	}
+}
+
+func TestF9GoldenRows(t *testing.T) {
+	root := t.TempDir()
+	term := Terminal(root, fixedClock)
+	ctx := context.Background()
+	if err := term(ctx, view("mrv-root-000000001", run.OutcomeFixed, []string{})); err != nil {
+		t.Fatal(err)
+	}
+	want := `{"schemaVersion":1,"id":"mrv-root-000000001","scope":"fsm-sdlc-loop","target":{"id":"sdlc-loop@bbbbbbbbbbbb","type":"fsm"},"status":"passed","verdict":"PASS","executionMode":"fsm","attemptNumber":1,"maxAttempts":3,"baseSha":"` + base + `","headSha":"` + head + `","createdAt":"2026-08-27T01:02:03Z","updatedAt":"2026-08-27T04:05:06.000000007Z","repoRoot":"/repo","contextPackPath":"","reviewLogPath":"","mock":false,"outcome":"fixed","fsmRunDir":".metareview/runs/mrv-root-000000001/","workflowHash":"wh","workflowSource":"embedded","escalationReason":""}`
+	got := lines(t, root)
+	if len(got) != 1 || got[0] != want {
+		t.Fatalf("golden root:\n%s\n%s", got[0], want)
+	}
+	assertKeys(t, got[0], false)
+	// grandchild (attempt 3) ending non-PASS → escalated
+	gc := view("mrv-gc-00000000001", run.OutcomeOverflow, []string{"mrv-root-000000001", "mrv-c1-00000000001"})
+	gc.Snapshot.ParentRunID = "mrv-c1-00000000001"
+	if err := term(ctx, gc); err != nil {
+		t.Fatal(err)
+	}
+	got = lines(t, root)
+	if len(got) != 2 || !strings.Contains(got[1], `"previousRunId":"mrv-c1-00000000001","attemptNumber":3`) || !strings.Contains(got[1], `"status":"escalated","verdict":"ESCALATED"`) || !strings.Contains(got[1], `"escalationReason":"attempt 3 of a fork lineage ended overflow"`) {
+		t.Fatalf("grandchild: %s", got[1])
+	}
+	assertKeys(t, got[1], true)
+	// the existing decoder reads both
+	recs, err := runchain.ReadRuns(root)
+	if err != nil || len(recs) != 2 || recs[1].AttemptNumber != 3 || recs[1].Verdict != "ESCALATED" || recs[0].Scope != "fsm-sdlc-loop" {
+		t.Fatalf("runchain decode: %v %+v", err, recs)
+	}
+	// idempotent on the resume path: same view again → still two rows; parent row first, child row second, then parent again
+	if err := term(ctx, view("mrv-root-000000001", run.OutcomeFixed, []string{})); err != nil || len(lines(t, root)) != 2 {
+		t.Fatalf("idempotent: %v", err)
+	}
+	// Exists
+	for _, c := range []struct {
+		id   string
+		want bool
+	}{{"mrv-root-000000001", true}, {"mrv-gc-00000000001", true}, {"mrv-none-000000001", false}} {
+		if ok, err := Exists(root, c.id); err != nil || ok != c.want {
+			t.Fatalf("exists %s: %v %v", c.id, ok, err)
+		}
+	}
+	if ok, err := Exists(t.TempDir(), "mrv-root-000000001"); err != nil || ok {
+		t.Fatalf("missing file: %v %v", ok, err)
+	}
+}
+
+func TestVerdictMap(t *testing.T) {
+	for _, c := range []struct {
+		outcome run.Outcome
+		lineage int
+		verdict string
+		status  string
+	}{
+		{run.OutcomeFixed, 0, VerdictPass, StatusPassed}, {run.OutcomeClean, 2, VerdictPass, StatusPassed},
+		{run.OutcomeReviewed, 0, VerdictNeedsRevision, StatusNeedsRevision}, {run.OutcomeStalled, 1, VerdictNeedsRevision, StatusNeedsRevision},
+		{run.OutcomeFailed, 0, VerdictNeedsRevision, StatusNeedsRevision}, {run.OutcomeOverflow, 0, VerdictNeedsRevision, StatusNeedsRevision},
+		{run.OutcomeCustom, 0, VerdictNeedsRevision, StatusNeedsRevision},
+		{run.OutcomeFailed, 2, VerdictEscalated, StatusEscalated}, {run.OutcomeReviewed, 3, VerdictEscalated, StatusEscalated},
+		{run.OutcomeFixed, 3, VerdictPass, StatusPassed},
+	} {
+		lin := make([]string, c.lineage)
+		r := RowFor(view("mrv-x-000000000001", c.outcome, lin), fixedClock())
+		if r.Verdict != c.verdict || r.Status != c.status || r.AttemptNumber != c.lineage+1 {
+			t.Fatalf("%s/%d: %+v", c.outcome, c.lineage, r)
+		}
+		if (r.EscalationReason != "") != (c.verdict == VerdictEscalated) {
+			t.Fatalf("reason: %+v", r)
+		}
+	}
+	v := view("mrv-x-000000000001", run.OutcomeFixed, nil)
+	v.Snapshot.MockTainted = true
+	v.Snapshot.WorkflowSource = ""
+	v.Snapshot.BaseSHA = "abc"
+	r := RowFor(v, fixedClock())
+	if !r.Mock || r.WorkflowSource != "unknown" || r.Target["id"] != "sdlc-loop@abc" {
+		t.Fatalf("tainted/unknown/short: %+v", r)
+	}
+	v.Snapshot.MockTainted, v.Snapshot.Mock = false, "m#1234567890abcdef"
+	if !RowFor(v, fixedClock()).Mock {
+		t.Fatal("mock run")
+	}
+}
+
+func TestTornTailWritePath(t *testing.T) {
+	root := t.TempDir()
+	dir := filepath.Join(root, ".metareview")
+	_ = os.MkdirAll(dir, 0o755)
+	p := filepath.Join(dir, "runs.jsonl")
+	// a legacy review row (complete) + a torn fragment
+	legacy := `{"schemaVersion":1,"id":"mrv-legacy-00000001","scope":"task-done","target":{"type":"task","id":"t"},"status":"escalated","verdict":"ESCALATED","previousRunId":"","attemptNumber":3,"maxAttempts":3,"headSha":"` + head + `"}`
+	_ = os.WriteFile(p, []byte(legacy+"\n{\"schemaVersion\":1,\"id\":\"mrv-torn"), 0o644)
+	nanos = func() int64 { return 42 }
+	term := Terminal(root, fixedClock)
+	if err := term(context.Background(), view("mrv-root-000000001", run.OutcomeFixed, nil)); err != nil {
+		t.Fatal(err)
+	}
+	got := lines(t, root)
+	if len(got) != 2 || got[0] != legacy || !strings.Contains(got[1], `"id":"mrv-root-000000001"`) {
+		t.Fatalf("after torn: %v", got)
+	}
+	frag, err := os.ReadFile(filepath.Join(root, ".metareview", "runs", ".torn", "runs.jsonl-42"))
+	if err != nil || string(frag) != "{\"schemaVersion\":1,\"id\":\"mrv-torn" {
+		t.Fatalf("fragment preserved: %q %v", frag, err)
+	}
+	if ok, err := Exists(root, "mrv-root-000000001"); err != nil || !ok {
+		t.Fatal("exists after repair")
+	}
+	if recs, err := runchain.ReadRuns(root); err != nil || len(recs) != 2 || recs[0].Verdict != "ESCALATED" {
+		t.Fatalf("runchain after repair: %v %+v", err, recs)
+	}
+	// Exists tolerates a torn tail without repairing it
+	_ = os.WriteFile(p, []byte(legacy+"\n{\"torn"), 0o644)
+	if ok, err := Exists(root, "mrv-legacy-00000001"); err != nil || !ok {
+		t.Fatalf("exists with torn tail: %v %v", ok, err)
+	}
+	if raw, _ := os.ReadFile(p); !strings.HasSuffix(string(raw), "{\"torn") {
+		t.Fatal("Exists must not repair")
+	}
+	// a newline-less but decodable final row is a row: "\n" is written first, nothing moved
+	_ = os.WriteFile(p, []byte(legacy), 0o644)
+	_ = os.RemoveAll(filepath.Join(root, ".metareview", "runs"))
+	if err := term(context.Background(), view("mrv-root-000000002", run.OutcomeFixed, nil)); err != nil {
+		t.Fatal(err)
+	}
+	got = lines(t, root)
+	if len(got) != 2 || got[0] != legacy {
+		t.Fatalf("newline-less legacy row kept: %v", got)
+	}
+	if _, err := os.Stat(filepath.Join(root, ".metareview", "runs", ".torn")); !errors.Is(err, os.ErrNotExist) {
+		t.Fatal("nothing must be moved for a decodable tail")
+	}
+	if recs, err := runchain.ReadRuns(root); err != nil || len(recs) != 2 || recs[0].Verdict != "ESCALATED" {
+		t.Fatalf("legacy escalation still visible: %v", err)
+	}
+	// blank lines are skipped
+	_ = os.WriteFile(p, []byte("\n"+legacy+"\n\n"), 0o644)
+	if err := term(context.Background(), view("mrv-root-000000003", run.OutcomeFixed, nil)); err != nil {
+		t.Fatal(err)
+	}
+	if ok, _ := Exists(root, "mrv-root-000000003"); !ok {
+		t.Fatal("row after blank lines")
+	}
+	// malformed terminated final line and malformed middle line refuse, nothing written
+	for _, body := range []string{legacy + "\nnot json\n", "not json\n" + legacy + "\n"} {
+		_ = os.WriteFile(p, []byte(body), 0o644)
+		err := term(context.Background(), view("mrv-root-000000004", run.OutcomeFixed, nil))
+		if !errs.Is(err, CodeRunsJSONL) || errs.As(err).Fields["reason"] != "malformed" {
+			t.Fatalf("malformed: %v", err)
+		}
+		if raw, _ := os.ReadFile(p); string(raw) != body {
+			t.Fatal("nothing written on malformed")
+		}
+		if _, err := Exists(root, "x"); !errs.Is(err, CodeRunsJSONL) {
+			t.Fatal("Exists reports malformed")
+		}
+	}
+	// a planted row with the id but a different head → id_conflict
+	planted := strings.Replace(legacy, "mrv-legacy-00000001", "mrv-root-000000009", 1)
+	_ = os.WriteFile(p, []byte(planted+"\n"), 0o644)
+	err = term(context.Background(), view("mrv-root-000000009", run.OutcomeFixed, nil))
+	if !errs.Is(err, CodeRunsJSONL) || errs.As(err).Fields["reason"] != "id_conflict" {
+		t.Fatalf("id_conflict: %v", err)
+	}
+	// idempotency when the matching row is not the last line
+	_ = os.WriteFile(p, nil, 0o644)
+	_ = term(context.Background(), view("mrv-root-000000001", run.OutcomeFixed, nil))
+	_ = term(context.Background(), view("mrv-c1-00000000001", run.OutcomeFixed, []string{"mrv-root-000000001"}))
+	if err := term(context.Background(), view("mrv-root-000000001", run.OutcomeFixed, nil)); err != nil || len(lines(t, root)) != 2 {
+		t.Fatalf("middle-row idempotency: %v %v", err, lines(t, root))
+	}
+}
+
+func TestWriteErrors(t *testing.T) {
+	ctx := context.Background()
+	// .metareview as a regular file → MkdirAll ENOTDIR
+	root := t.TempDir()
+	_ = os.WriteFile(filepath.Join(root, ".metareview"), []byte("x"), 0o644)
+	if err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); err == nil {
+		t.Fatal("ENOTDIR")
+	}
+	if _, err := Exists(root, "x"); err == nil {
+		t.Fatal("Exists ENOTDIR")
+	}
+	// runs.jsonl as a directory → open fails
+	root = t.TempDir()
+	_ = os.MkdirAll(filepath.Join(root, ".metareview", "runs.jsonl"), 0o755)
+	if err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); err == nil {
+		t.Fatal("directory as file")
+	}
+	// the torn directory cannot be created (runs is a file)
+	root = t.TempDir()
+	_ = os.MkdirAll(filepath.Join(root, ".metareview"), 0o755)
+	_ = os.WriteFile(filepath.Join(root, ".metareview", "runs"), []byte("x"), 0o644)
+	_ = os.WriteFile(filepath.Join(root, ".metareview", "runs.jsonl"), []byte("{\"torn"), 0o644)
+	if err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); err == nil {
+		t.Fatal("torn dir")
+	}
+	// the torn fragment cannot be written (unwritable .torn dir)
+	if os.Getuid() != 0 {
+		root = t.TempDir()
+		_ = os.MkdirAll(filepath.Join(root, ".metareview", "runs", ".torn"), 0o700)
+		_ = os.WriteFile(filepath.Join(root, ".metareview", "runs.jsonl"), []byte("{\"torn"), 0o644)
+		_ = os.Chmod(filepath.Join(root, ".metareview", "runs", ".torn"), 0)
+		err := Terminal(root, fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil))
+		_ = os.Chmod(filepath.Join(root, ".metareview", "runs", ".torn"), 0o700)
+		if err == nil {
+			t.Fatal("unwritable torn dir")
+		}
+	}
+	// flock failure
+	orig := flock
+	flock = func(int, int) error { return errors.New("flock failed") }
+	if err := Terminal(t.TempDir(), fixedClock)(ctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); err == nil || err.Error() != "flock failed" {
+		t.Fatalf("flock: %v", err)
+	}
+	flock = orig
+	// a line longer than the scanner buffer
+	root = t.TempDir()
+	_ = os.MkdirAll(filepath.Join(root, ".metareview"), 0o755)
+	_ = os.WriteFile(filepath.Join(root, ".metareview", "runs.jsonl"), append([]byte(`{"id":"`+strings.Repeat("x", 1<<20)+`"}`), '\n'), 0o644)
+	if _, err := Exists(root, "x"); err == nil {
+		t.Fatal("oversized line")
+	}
+	// ctx cancelled
+	cctx, cancel := context.WithCancel(ctx)
+	cancel()
+	if err := Terminal(t.TempDir(), fixedClock)(cctx, view("mrv-x-000000000001", run.OutcomeFixed, nil)); !errors.Is(err, context.Canceled) {
+		t.Fatalf("ctx: %v", err)
+	}
+	if nowNanos() == 0 {
+		t.Fatal("clock")
+	}
+}


--- docs/tasks/m7-fork-record-export.md
+# M7 — fork/resume, diff, export, runs.jsonl record
+
+Implements spec 3 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md`): the `run` amendments (`WorkflowSource`,
+`TornFiles`/`MaxEvents`/`Counted`, incomplete-fork rule), `machine.Fork`/`VerifyOrigin`/`DiffRuns` + `machine.Decision` +
+`ERR_FORK_INCOMPLETE`, `kind.Decision` + judge-less registries, `internal/fsm/record` (terminal recorder, `Exists`,
+torn-safe writer), `internal/fsm/export` (redaction table, redacted snapshot, manifest, `FS` seam).
+
+Done when every touched `internal/fsm/*` package is at exactly 100% statement coverage (`tests/coverage.sh`) and
+`go vet` is clean.
```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence


> metareview@0.8.2 prepack
> npm run build


> metareview@0.8.2 build
> go build -o bin/metareview ./cmd/metareview

cmd/metareview                                      80.4%  ok
internal/artifactreview                             80.4%  ok
internal/contextpack                                76.1%  ok
internal/contextprofile                             84.6%  ok
internal/epicready                                  81.6%  ok
internal/epicsource                                 83.1%  ok
internal/evidence                                   85.2%  ok
internal/findings                                   90.5%  ok
internal/fsm/cmdexec                               100.0%  ok
internal/fsm/converge                              100.0%  ok
internal/fsm/errs                                  100.0%  ok
internal/fsm/export                                100.0%  ok
internal/fsm/gate                                  100.0%  ok
internal/fsm/judge                                 100.0%  ok
internal/fsm/kind                                  100.0%  ok
internal/fsm/machine                               100.0%  ok
internal/fsm/mockai                                100.0%  ok
internal/fsm/record                                100.0%  ok
internal/fsm/run                                   100.0%  ok
internal/fsm/workflow                              100.0%  ok
internal/gitcontext                                 83.7%  ok
internal/githubcontext                              95.9%  ok
internal/integration                               100.0%  ok
internal/knowledge                                  77.8%  ok
internal/learning                                   88.0%  ok
internal/learnsource                                70.8%  ok
internal/markdown                                   70.0%  ok
internal/prready                                    85.7%  ok
internal/repo                                       87.9%  ok
internal/reviewers                                  97.2%  ok
internal/reviewlog                                  90.2%  ok
internal/reviewmanifest                             90.5%  ok
internal/reviewstate                                92.1%  ok
internal/runchain                                   90.1%  ok
internal/sessionhistory                             86.2%  ok
internal/setup                                      88.5%  ok
internal/state                                      81.6%  ok
internal/taskdone                                   87.0%  ok
internal/tasksource                                 79.2%  ok
workflows                                          100.0%  ok
coverage gate passed
coverage exit=0

