# metareview task-done context

Run ID: `mrv-20260827-104831818298000-task-done-m7-fork-record-export-9dc62119`

## Task

# M7 — fork/resume, diff, export, runs.jsonl record

Implements spec 3 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md`): the `run` amendments (`WorkflowSource`,
`TornFiles`/`MaxEvents`/`Counted`, incomplete-fork rule), `machine.Fork`/`VerifyOrigin`/`DiffRuns` + `machine.Decision` +
`ERR_FORK_INCOMPLETE`, `kind.Decision` + judge-less registries, `internal/fsm/record` (terminal recorder, `Exists`,
torn-safe writer), `internal/fsm/export` (redaction table, redacted snapshot, manifest, `FS` seam).

Done when every touched `internal/fsm/*` package is at exactly 100% statement coverage (`tests/coverage.sh`) and
`go vet` is clean.


## Git

- Base: `484b1507429a8b89f26aa7eff99e70fdfb1370fa`
- Head: `a5a78ee6f1fa0a93669f61553b4ad57b7621ee40`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `14251`
- Filtered diff bytes: `14251`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `abbab6069bd7e5b6`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/tasks/m7-fork-record-export.md
- internal/fsm/run/event.go
- internal/fsm/run/fold.go
- internal/fsm/run/fold_test.go
- internal/fsm/run/store.go
- internal/fsm/run/store_jsonl.go
- internal/fsm/run/store_mem.go
- internal/fsm/run/store_test.go
- internal/fsm/run/types.go

### Shards
- shard-01: docs/tasks/m7-fork-record-export.md, internal/fsm/run/event.go, internal/fsm/run/fold.go, internal/fsm/run/fold_test.go, internal/fsm/run/store.go, internal/fsm/run/store_jsonl.go, internal/fsm/run/store_mem.go, internal/fsm/run/store_test.go, internal/fsm/run/types.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- internal/fsm/run/event.go
- internal/fsm/run/fold.go
- internal/fsm/run/fold_test.go
- internal/fsm/run/store.go
- internal/fsm/run/store_jsonl.go
- internal/fsm/run/store_mem.go
- internal/fsm/run/store_test.go
- internal/fsm/run/types.go
- docs/tasks/m7-fork-record-export.md

## Diff

```diff
diff --git a/internal/fsm/run/event.go b/internal/fsm/run/event.go
index 86c6d30..6d0d98f 100644
--- a/internal/fsm/run/event.go
+++ b/internal/fsm/run/event.go
@@ -50,26 +50,27 @@ var EventTypes = []string{TypeInit, TypeTree, TypeNeedsInput, TypeNodeOutput, Ty
 
 // InitData is the seq-1 payload.
 type InitData struct {
-	RunID        string            `json:"run_id"`
-	CreatedAt    Time              `json:"created_at"`
-	Workflow     string            `json:"workflow"`
-	WorkflowHash string            `json:"workflow_hash"`
-	Vars         map[string]string `json:"vars"`
-	Calibration  bool              `json:"calibration"`
-	Mock         string            `json:"mock,omitempty"`
-	RepoMode     string            `json:"repo_mode"`
-	AllowedCmds  []AllowedCmd      `json:"allowed_cmds"`
-	CmdsSHA256   string            `json:"cmds_sha256,omitempty"`
-	RepoRoot     string            `json:"repo_root"`
-	WorkDir      string            `json:"work_dir"`
-	BaseSHA      string            `json:"base_sha"`
-	Head         string            `json:"head"`
-	InitialState State             `json:"initial_state"`
-	InitialKind  Kind              `json:"initial_kind,omitempty"`
-	Goldens      []Golden          `json:"goldens"`
-	ParentRunID  string            `json:"parent_run_id,omitempty"`
-	Lineage      []string          `json:"lineage"`
-	ForkedAtSeq  int64             `json:"forked_at_seq,omitempty"`
+	RunID          string            `json:"run_id"`
+	CreatedAt      Time              `json:"created_at"`
+	Workflow       string            `json:"workflow"`
+	WorkflowHash   string            `json:"workflow_hash"`
+	WorkflowSource string            `json:"workflow_source,omitempty"` // embedded|path|"" (spec 3 r5; "" = legacy)
+	Vars           map[string]string `json:"vars"`
+	Calibration    bool              `json:"calibration"`
+	Mock           string            `json:"mock,omitempty"`
+	RepoMode       string            `json:"repo_mode"`
+	AllowedCmds    []AllowedCmd      `json:"allowed_cmds"`
+	CmdsSHA256     string            `json:"cmds_sha256,omitempty"`
+	RepoRoot       string            `json:"repo_root"`
+	WorkDir        string            `json:"work_dir"`
+	BaseSHA        string            `json:"base_sha"`
+	Head           string            `json:"head"`
+	InitialState   State             `json:"initial_state"`
+	InitialKind    Kind              `json:"initial_kind,omitempty"`
+	Goldens        []Golden          `json:"goldens"`
+	ParentRunID    string            `json:"parent_run_id,omitempty"`
+	Lineage        []string          `json:"lineage"`
+	ForkedAtSeq    int64             `json:"forked_at_seq,omitempty"`
 }
 
 // TreeData is a working-tree snapshot carrier.
diff --git a/internal/fsm/run/fold.go b/internal/fsm/run/fold.go
index fe6cbca..74c69e7 100644
--- a/internal/fsm/run/fold.go
+++ b/internal/fsm/run/fold.go
@@ -289,6 +289,7 @@ func (st *FoldState) applyInit(d *InitData) {
 	st.RunID, st.ParentRunID, st.ForkedAtSeq, st.CreatedAt = d.RunID, d.ParentRunID, d.ForkedAtSeq, d.CreatedAt
 	st.Lineage = nonNilStrings(d.Lineage)
 	st.Workflow, st.WorkflowHash, st.Calibration, st.Mock = d.Workflow, d.WorkflowHash, d.Calibration, d.Mock
+	st.WorkflowSource = d.WorkflowSource
 	st.Vars = cloneStringMap(d.Vars)
 	if st.Vars == nil {
 		st.Vars = map[string]string{}
@@ -500,7 +501,7 @@ func argvOK(argv []string) bool {
 func withinCaps(p any) bool {
 	switch d := p.(type) {
 	case *InitData:
-		if !shortOK(d.RunID, d.Workflow, d.WorkflowHash, d.Mock, d.RepoMode, d.CmdsSHA256, d.RepoRoot, d.WorkDir, d.BaseSHA, d.Head, string(d.InitialState), string(d.InitialKind), d.ParentRunID) || !shortOK(d.Lineage...) {
+		if !shortOK(d.RunID, d.Workflow, d.WorkflowHash, d.Mock, d.RepoMode, d.CmdsSHA256, d.RepoRoot, d.WorkDir, d.BaseSHA, d.Head, string(d.InitialState), string(d.InitialKind), d.ParentRunID, d.WorkflowSource) || !shortOK(d.Lineage...) {
 			return false
 		}
 		if len(d.Vars) > MaxVars || len(d.Goldens) > MaxGoldens || len(d.AllowedCmds) > MaxAllowedCmds {
diff --git a/internal/fsm/run/fold_test.go b/internal/fsm/run/fold_test.go
index c867e03..585c1f3 100644
--- a/internal/fsm/run/fold_test.go
+++ b/internal/fsm/run/fold_test.go
@@ -1092,3 +1092,30 @@ func TestFoldTokensNegativeAndNextIndex(t *testing.T) {
 		t.Fatal(err)
 	}
 }
+
+func TestWorkflowSourceFolds(t *testing.T) {
+	d := baseInit()
+	d.WorkflowSource = "path"
+	b := NewBuilder(runA)
+	b.Init(d)
+	snap, err := Fold(b.Events())
+	if err != nil || snap.WorkflowSource != "path" {
+		t.Fatalf("workflow_source must fold: %v %+v", err, snap.WorkflowSource)
+	}
+	if snap.Clone().WorkflowSource != "path" {
+		t.Fatal("Clone must copy WorkflowSource")
+	}
+	// legacy init without the field decodes and folds as ""
+	b2 := NewBuilder(runA)
+	b2.Init(baseInit())
+	if snap, err := Fold(b2.Events()); err != nil || snap.WorkflowSource != "" {
+		t.Fatalf("legacy: %v %q", err, snap.WorkflowSource)
+	}
+	// cap: over MaxShort is refused at fold
+	d.WorkflowSource = strings.Repeat("x", MaxShort+1)
+	b3 := NewBuilder(runA)
+	b3.Init(d)
+	if _, err := Fold(b3.Events()); err == nil {
+		t.Fatal("over-cap workflow_source must be refused")
+	}
+}
diff --git a/internal/fsm/run/store.go b/internal/fsm/run/store.go
index 1eb84bb..02c35a3 100644
--- a/internal/fsm/run/store.go
+++ b/internal/fsm/run/store.go
@@ -35,6 +35,13 @@ type RunSummary struct {
 	Error       string
 }
 
+// TornFile describes one audit.torn-*.bin left by RepairTail (spec 3 r5: listed by exports, never copied).
+type TornFile struct {
+	Name   string
+	SHA256 string
+	Bytes  int64
+}
+
 // Options configures a store.
 type Options struct {
 	MaxEvents int // zero → DefaultMaxEvents
@@ -57,8 +64,13 @@ type RunStore interface {
 	List() ([]RunSummary, error)
 	Lock(runID string) (unlock func(), err error)
 	Root() string
+	TornFiles(runID string) ([]TornFile, error) // sorted by name; empty for the memory store
+	MaxEvents() int                             // the effective cap (Options.MaxEvents or DefaultMaxEvents)
 }
 
+// Counted reports whether an event type counts toward MaxEvents (exported for spec 3's in-memory fork check).
+func Counted(t string) bool { return countedType(t) }
+
 func storeErrf(code string, seq int64, detail string) *StoreError {
 	return &StoreError{Code: code, Seq: seq, Detail: detail}
 }
@@ -191,9 +203,20 @@ func summarize(runID string, log Log, err error, sidecars int) RunSummary {
 	}
 	s.Workflow, s.CreatedAt, s.State, s.Outcome = snap.Workflow, snap.CreatedAt, snap.State, snap.Outcome
 	s.ParentRunID, s.Mock, s.MockTainted = snap.ParentRunID, snap.Mock, snap.MockTainted
+	if incompleteFork(snap) {
+		s.Error = "incomplete fork: run " + runID + " of " + snap.ParentRunID + " has no rebaseline"
+	}
 	return s
 }
 
+// incompleteFork is spec 3 r5 §2 step 8: a forked child whose step-8 write did not finish.
+func incompleteFork(snap Snapshot) bool {
+	if snap.ParentRunID == "" {
+		return false
+	}
+	return snap.Seq <= snap.ForkedAtSeq || (snap.Seq == snap.ForkedAtSeq+1 && snap.StateKind == KindAgentEdit)
+}
+
 func sortSummaries(list []RunSummary) {
 	sort.Slice(list, func(i, j int) bool {
 		if !list[i].CreatedAt.Equal(list[j].CreatedAt.Time) {
diff --git a/internal/fsm/run/store_jsonl.go b/internal/fsm/run/store_jsonl.go
index d389f56..2e75c00 100644
--- a/internal/fsm/run/store_jsonl.go
+++ b/internal/fsm/run/store_jsonl.go
@@ -5,6 +5,7 @@ import (
 	"fmt"
 	"os"
 	"path/filepath"
+	"sort"
 	"strings"
 	"sync"
 	"syscall"
@@ -26,6 +27,32 @@ func NewJSONLStore(root string, opts Options) RunStore {
 
 func (s *jsonlStore) Root() string { return s.root }
 
+func (s *jsonlStore) MaxEvents() int { return s.opts.maxEvents() }
+
+// TornFiles lists audit.torn-*.bin for a run with their sha256 and size, sorted by name.
+func (s *jsonlStore) TornFiles(runID string) ([]TornFile, error) {
+	if err := s.validate(runID); err != nil {
+		return nil, err
+	}
+	entries, err := os.ReadDir(s.runDir(runID))
+	if err != nil {
+		return nil, storeErrf(CodeRunNotFound, 0, runID)
+	}
+	out := []TornFile{}
+	for _, e := range entries {
+		if e.IsDir() || !strings.HasPrefix(e.Name(), "audit.torn-") {
+			continue
+		}
+		raw, err := os.ReadFile(filepath.Join(s.runDir(runID), e.Name()))
+		if err != nil {
+			return nil, pathErr(0, err)
+		}
+		out = append(out, TornFile{Name: e.Name(), SHA256: LineHash(raw), Bytes: int64(len(raw))})
+	}
+	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
+	return out, nil
+}
+
 func (s *jsonlStore) runsDir() string { return filepath.Join(s.root, ".metareview", "runs") }
 
 func (s *jsonlStore) runDir(id string) string { return filepath.Join(s.runsDir(), id) }
diff --git a/internal/fsm/run/store_mem.go b/internal/fsm/run/store_mem.go
index 749d340..56bfb88 100644
--- a/internal/fsm/run/store_mem.go
+++ b/internal/fsm/run/store_mem.go
@@ -22,6 +22,17 @@ func NewMemStore(opts Options) RunStore {
 
 func (s *memStore) Root() string { return "" }
 
+func (s *memStore) MaxEvents() int { return s.opts.maxEvents() }
+
+func (s *memStore) TornFiles(runID string) ([]TornFile, error) {
+	s.mu.Lock()
+	defer s.mu.Unlock()
+	if _, err := s.get(runID); err != nil {
+		return nil, err
+	}
+	return []TornFile{}, nil
+}
+
 func (s *memStore) get(runID string) (*memRun, error) {
 	if err := ValidateRunID(runID); err != nil {
 		return nil, storeErrf(CodeStorePath, 0, err.Error())
diff --git a/internal/fsm/run/store_test.go b/internal/fsm/run/store_test.go
index f4dd7cd..aa10971 100644
--- a/internal/fsm/run/store_test.go
+++ b/internal/fsm/run/store_test.go
@@ -846,3 +846,83 @@ func TestOracle(t *testing.T) {
 		t.Fatalf("edit seq: %+v", se)
 	}
 }
+
+// ---- spec 3 r5 owned amendments -----------------------------------------------------------------
+
+func TestCountedAndMaxEvents(t *testing.T) {
+	for _, typ := range []string{TypeNeedsInput, TypeNodeOutput, TypeDeltaApplied, TypeLLMCall, TypeTokens, TypeRecord, TypeTree} {
+		if !Counted(typ) {
+			t.Fatalf("%s must be counted", typ)
+		}
+	}
+	for _, typ := range []string{TypeInit, TypeTransition, TypeGate, TypeCmdCall, TypeConverge, TypeFork, TypeFixBaseline, TypeWarn, TypeOverflowHandler} {
+		if Counted(typ) {
+			t.Fatalf("%s must not be counted", typ)
+		}
+	}
+	if NewMemStore(Options{MaxEvents: 5}).MaxEvents() != 5 || NewJSONLStore(t.TempDir(), Options{MaxEvents: 7}).MaxEvents() != 7 {
+		t.Fatal("MaxEvents must echo Options")
+	}
+	if NewMemStore(Options{}).MaxEvents() != DefaultMaxEvents || NewJSONLStore(t.TempDir(), Options{}).MaxEvents() != DefaultMaxEvents {
+		t.Fatal("zero → DefaultMaxEvents")
+	}
+}
+
+func TestTornFiles(t *testing.T) {
+	mem := NewMemStore(Options{})
+	seed(t, mem, happyLog().Events())
+	if files, err := mem.TornFiles(runA); err != nil || len(files) != 0 {
+		t.Fatalf("mem store has no torn files: %v %v", files, err)
+	}
+	if _, err := mem.TornFiles("../x"); err == nil {
+		t.Fatal("invalid id must be refused")
+	}
+	if _, err := mem.TornFiles(runB); err == nil {
+		t.Fatal("unknown run must be refused")
+	}
+	s := NewJSONLStore(t.TempDir(), Options{})
+	seed(t, s, happyLog().Events())
+	dir := filepath.Join(s.Root(), ".metareview", "runs", runA)
+	if files, err := s.TornFiles(runA); err != nil || len(files) != 0 {
+		t.Fatalf("no torn files yet: %v %v", files, err)
+	}
+	_ = os.WriteFile(filepath.Join(dir, "audit.torn-9-2.bin"), []byte("zz"), 0o600)
+	_ = os.WriteFile(filepath.Join(dir, "audit.torn-9-1.bin"), []byte("{\"torn"), 0o600)
+	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600)
+	files, err := s.TornFiles(runA)
+	if err != nil || len(files) != 2 {
+		t.Fatalf("torn files: %+v %v", files, err)
+	}
+	want := TornFile{Name: "audit.torn-9-1.bin", SHA256: LineHash([]byte("{\"torn")), Bytes: 6}
+	if files[0] != want || files[1].Name != "audit.torn-9-2.bin" || files[1].Bytes != 2 {
+		t.Fatalf("literal: %+v", files)
+	}
+	if _, err := s.TornFiles("../x"); err == nil {
+		t.Fatal("invalid id must be refused")
+	}
+	if _, err := s.TornFiles(runB); err == nil {
+		t.Fatal("unknown run must be refused")
+	}
+	_ = os.Chmod(filepath.Join(dir, "audit.torn-9-2.bin"), 0)
+	if _, err := s.TornFiles(runA); err == nil && os.Getuid() != 0 {
+		t.Fatal("unreadable torn file must error")
+	}
+}
+
+func TestSummarizeIncompleteFork(t *testing.T) {
+	parent := happyLog().Events()
+	mk := func(to int64, kind Kind, extra ...string) Log {
+		b := NewBuilder(runB)
+		evs := b.Copy(parent, 2, to, runB, func(d *InitData) { d.ForkedAtSeq = to })
+		_ = kind
+		return Log{Events: evs}
+	}
+	// no rebaseline tree: Seq == ForkedAtSeq → incomplete
+	if s := summarize(runB, mk(3, ""), nil, 0); !strings.Contains(s.Error, "incomplete fork") {
+		t.Fatalf("expected incomplete fork, got %+v", s)
+	}
+	// a root run is never incomplete
+	if s := summarize(runA, Log{Events: parent[:3]}, nil, 0); s.Error != "" {
+		t.Fatalf("root must not be flagged: %+v", s)
+	}
+}
diff --git a/internal/fsm/run/types.go b/internal/fsm/run/types.go
index 56f1abd..04eff73 100644
--- a/internal/fsm/run/types.go
+++ b/internal/fsm/run/types.go
@@ -197,6 +197,7 @@ type Snapshot struct {
 	Seq             int64                      `json:"seq"`
 	Workflow        string                     `json:"workflow"`
 	WorkflowHash    string                     `json:"workflow_hash"`
+	WorkflowSource  string                     `json:"workflow_source,omitempty"`
 	Vars            map[string]string          `json:"vars"`
 	Calibration     bool                       `json:"calibration"`
 	Mock            string                     `json:"mock,omitempty"`


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

