package export

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/kind"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
	"github.com/dsifry/metareview/workflows"
)

const runA = "mrv-export-a-000001"

// ---- fakes ---------------------------------------------------------------------------------------

type memFile struct {
	buf   bytes.Buffer
	fs    *memFS
	path  string
	wfail error
}

func (f *memFile) Write(b []byte) (int, error) {
	if f.wfail != nil {
		return 0, f.wfail
	}
	return f.buf.Write(b)
}
func (f *memFile) Close() error { f.fs.files[f.path] = f.buf.Bytes(); return f.fs.closeErr }

type memFS struct {
	files    map[string][]byte
	dirs     map[string]bool
	symlinks map[string]bool
	opens    []string // "path flag perm"
	mkdirs   []string
	mkdirErr error
	openErr  error
	lstatErr error
	readErr  error
	writeErr error
	closeErr error
}

func newFS() *memFS {
	return &memFS{files: map[string][]byte{}, dirs: map[string]bool{}, symlinks: map[string]bool{}}
}

type fakeInfo struct{ symlink bool }

func (fakeInfo) Name() string { return "" }
func (fakeInfo) Size() int64  { return 0 }
func (f fakeInfo) Mode() os.FileMode {
	if f.symlink {
		return os.ModeSymlink
	}
	return os.ModeDir
}
func (fakeInfo) ModTime() time.Time { return time.Time{} }
func (fakeInfo) IsDir() bool        { return true }
func (fakeInfo) Sys() any           { return nil }

type fakeEntry struct{ name string }

func (e fakeEntry) Name() string             { return e.name }
func (fakeEntry) IsDir() bool                { return false }
func (fakeEntry) Type() os.FileMode          { return 0 }
func (fakeEntry) Info() (os.FileInfo, error) { return fakeInfo{}, nil }
func (m *memFS) MkdirAll(dir string, perm os.FileMode) error {
	m.mkdirs = append(m.mkdirs, dir+" "+perm.String())
	if m.mkdirErr != nil {
		return m.mkdirErr
	}
	m.dirs[dir] = true
	return nil
}
func (m *memFS) OpenFile(path string, flag int, perm os.FileMode) (io.WriteCloser, error) {
	m.opens = append(m.opens, path+" "+itoa(flag)+" "+perm.String())
	if m.openErr != nil {
		return nil, m.openErr
	}
	return &memFile{fs: m, path: path, wfail: m.writeErr}, nil
}
func (m *memFS) Lstat(path string) (os.FileInfo, error) {
	if m.lstatErr != nil {
		return nil, m.lstatErr
	}
	if m.symlinks[path] {
		return fakeInfo{symlink: true}, nil
	}
	if m.dirs[path] {
		return fakeInfo{}, nil
	}
	return nil, os.ErrNotExist
}
func (m *memFS) ReadDir(dir string) ([]os.DirEntry, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	var out []os.DirEntry
	for p := range m.files {
		if filepath.Dir(p) == dir {
			out = append(out, fakeEntry{filepath.Base(p)})
		}
	}
	return out, nil
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }

// ---- a fold-valid run with a marker in every redacted field ---------------------------------------

const (
	secret = "sekret-value"
	head0  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	head1  = "cccccccccccccccccccccccccccccccccccccccc"
)

func out(raw string) run.NodeOutputData { return run.NodeOutputData{Output: json.RawMessage(raw)} }
func delta(raw string, d run.Delta) run.DeltaAppliedData {
	return run.DeltaAppliedData{Delta: d, OutputHash: run.OutputHash([]byte(raw))}
}

// markedLog: discover → adjudicate → fix → verify → done with markers in vars, argv, file_hashes, paths, tree status,
// gate detail, cmd streams, converge reason, warn detail, record data, and a cmd_call/overflow_handler.
func markedLog(t *testing.T) *run.Builder {
	t.Helper()
	b := run.NewBuilder(runA)
	b.Init(run.InitData{
		RunID: runA, Workflow: "sdlc-loop", WorkflowHash: "wh", WorkflowSource: "embedded", RepoMode: "advisory", RepoRoot: "/repo", WorkDir: "/repo/wt",
		BaseSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Head: head0, InitialState: "discover", Lineage: []string{}, Goldens: []run.Golden{{Comment: "golden text"}},
		Vars:        map[string]string{"SECRET": secret, "REV": "ab", "TARGET": "/repo/tools/run.sh", "JUDGE": "judge-model", "OTHER": "twelve-chars"},
		AllowedCmds: []run.AllowedCmd{{Name: "notify", Argv: []string{"/home/dave/bin/tool", "--token=" + secret, "/repo/tools/run.sh", "src/x"}, FileHashes: map[string]string{"/home/dave/bin/tool": "h1", "/repo/tools/run.sh": "h2", "/repo/" + secret: "h3", "/repo/$SECRET": "h4"}}},
	})
	b.Event(run.TypeTree, run.TreeData{Head: head0, TreeHash: "t0", Status: "1 .M N... 100644 100644 100644 abc def porcelain-file.go\n? untracked-marker.txt\n"})
	b.Event(run.TypeWarn, run.WarnData{Code: "UNSANCTIONED_EDIT", Detail: "warn-marker porcelain"})
	b.Event(run.TypeNeedsInput, run.EmptyData{}, run.WithNode("discover"))
	b.Event(run.TypeNodeOutput, out(`{"findings":[{"issue_text":"issue kept"}]}`), run.WithNode("discover"))
	b.Event(run.TypeDeltaApplied, delta(`{"findings":[{"issue_text":"issue kept"}]}`, run.Delta{Findings: []run.Finding{{IssueText: "issue kept"}}}), run.WithNode("discover"))
	b.Event(run.TypeRecord, run.RecordData{Name: "note", Data: json.RawMessage(`{"note":"record-marker kept"}`)})
	b.Event(run.TypeGate, run.GateData{Name: "findings_nonempty", Passed: true})
	b.Event(run.TypeTransition, run.TransitionData{From: "discover", To: "adjudicate", Gate: "findings_nonempty", Head: head0})
	b.Event(run.TypeLLMCall, run.LLMCallData{Kind: "adjudicate", Model: "judge-model", Effort: "low", Index: 0, InputHash: "ih", Verdict: json.RawMessage(`{"is_real":true,"reasoning":"reasoning kept"}`), Confidence: 0.9, Error: ""}, run.WithNode("adjudicate"))
	b.Event(run.TypeNodeOutput, out(`{"confirmed":[{"id":"b1","desc":"desc kept"}],"rejected":[{"id":"b2","desc":"rejected desc kept"}]}`), run.WithNode("adjudicate"))
	b.Event(run.TypeDeltaApplied, delta(`{"confirmed":[{"id":"b1","desc":"desc kept"}],"rejected":[{"id":"b2","desc":"rejected desc kept"}]}`, run.Delta{Confirmed: []run.Bug{{ID: "b1", Desc: "desc kept", Verdict: "real_but_ungold", Confidence: 0.9}}}), run.WithNode("adjudicate"))
	b.Event(run.TypeGate, run.GateData{Name: "confirmed_nonempty", Passed: true})
	b.Event(run.TypeTransition, run.TransitionData{From: "adjudicate", To: "fix", Gate: "confirmed_nonempty", ToKind: run.KindAgentEdit, Head: head0})
	b.Event(run.TypeNeedsInput, run.EmptyData{}, run.WithNode("fix"))
	b.Event(run.TypeGate, run.GateData{Name: "commit_exists", Passed: false, Error: &run.GateError{Code: "ERR_NO_COMMIT", Gate: "commit_exists", Detail: "0 commits since x; clean=false\n--- status ---\n1 .M N... 100644 100644 100644 abc def porcelain-diff-file.go\n--- working diff ---\ndiff-marker\n", DetailTruncated: true}})
	b.Event(run.TypeNodeOutput, out(`{"commit":"c1"}`), run.WithNode("fix"))
	b.Event(run.TypeDeltaApplied, delta(`{"commit":"c1"}`, run.Delta{Commit: "c1"}), run.WithNode("fix"))
	b.Event(run.TypeTree, run.TreeData{Head: head1, TreeHash: "t1", Status: ""})
	b.Event(run.TypeGate, run.GateData{Name: "commit_exists", Passed: true})
	b.Event(run.TypeTransition, run.TransitionData{From: "fix", To: "verify", Gate: "commit_exists", Head: head1})
	b.Event(run.TypeLLMCall, run.LLMCallData{Kind: "still-present", Model: "judge-model", Effort: "low", Index: 0, InputHash: "ih2", Verdict: json.RawMessage(`{"still_present":false}`), Error: "ERR_JUDGE_RESPONSE"}, run.WithNode("verify"))
	b.Event(run.TypeNodeOutput, out(`{"status":[{"id":"b1","still_present":false}]}`), run.WithNode("verify"))
	b.Event(run.TypeDeltaApplied, delta(`{"status":[{"id":"b1","still_present":false}]}`, run.Delta{Status: []run.BugStatus{{ID: "b1", StillPresent: false, Confidence: 0.8}}}), run.WithNode("verify"))
	b.Event(run.TypeCmdCall, run.CmdCallData{Name: "notify", Argv: []string{"/home/dave/bin/tool", "--token=" + secret, "/repo/tools/run.sh", "src/x"}, InputHash: "x", Stdout: "stdout-marker", Stderr: "stderr-marker"})
	b.Event(run.TypeConverge, run.ConvergeData{Atom: "cmd:notify", Class: run.OutcomeCustom, Stop: false, Reason: "cmd-reason-marker"})
	b.Event(run.TypeConverge, run.ConvergeData{Atom: "all_fixed", Class: run.OutcomeFixed, Stop: true, Reason: "0 unfixed kept"})
	b.Event(run.TypeGate, run.GateData{Name: "all_fixed", Passed: true})
	b.Event(run.TypeTransition, run.TransitionData{From: "verify", To: "done", Gate: "all_fixed", Outcome: run.OutcomeFixed, Head: head1})
	b.Event(run.TypeOverflowHandler, run.OverflowHandlerData{Name: "notify", Argv: []string{"--token=" + secret}, InputHash: "y", Stdout: "handler-stdout-marker", Stderr: ""})
	return b
}

func seedRun(t *testing.T, store run.RunStore, id string, evs []run.Event) {
	t.Helper()
	st, err := store.Create(id, evs[0])
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := store.Lock(id)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	for _, ev := range evs[1:] {
		if st, err = store.Append(id, st, ev); err != nil {
			t.Fatalf("seed seq %d: %v", ev.Seq, err)
		}
	}
}

type harness struct {
	store   run.RunStore
	sidecar *machine.MemSidecar
	fs      *memFS
	deps    Deps
	raw     []byte
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	reg, err := kind.New(kind.Deps{})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{store: run.NewMemStore(run.Options{}), sidecar: &machine.MemSidecar{}, fs: newFS()}
	h.raw, _ = workflows.Read("sdlc-loop")
	h.deps = Deps{Store: h.store, Sidecar: h.sidecar, Kinds: reg, FS: h.fs, RepoRoot: "/repo", Home: "/home/dave",
		Clock: func() run.Time { return run.Time{Time: time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)} }}
	seedRun(t, h.store, runA, markedLog(t).Events())
	h.sidecar.Put(runA, machine.SidecarWorkflow, h.raw)
	h.sidecar.Put(runA, "notes.md", []byte("sidecar bytes"))
	return h
}

func allBytes(fs *memFS) string {
	var b strings.Builder
	for _, v := range fs.files {
		b.Write(v)
	}
	return b.String()
}

func TestF8Redaction(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	m, err := Export(ctx, h.deps, runA, Options{})
	if err != nil {
		t.Fatal(err)
	}
	out := "/repo/docs/metareview/fsm/" + runA
	every := allBytes(h.fs)
	for _, marker := range []string{secret, "/home/dave", "warn-marker", "stdout-marker", "stderr-marker", "handler-stdout-marker", "cmd-reason-marker", "diff-marker", "porcelain-file.go \n", "/repo/tools/run.sh"} {
		if strings.Contains(every, marker) {
			t.Fatalf("marker %q leaked", marker)
		}
	}
	for _, kept := range []string{"issue kept", "desc kept", "rejected desc kept", "reasoning kept", "record-marker kept", "0 unfixed kept", "golden text", "$SECRET", "$TARGET", "~/bin/tool", "porcelain-file.go", "untracked-marker.txt", "porcelain-diff-file.go", "judge-model", `"exported":true`, "sidecar bytes", `"redacted":true`} {
		if !strings.Contains(every, kept) {
			t.Fatalf("expected %q in the bundle", kept)
		}
	}
	// positive payload assertions
	audit := string(h.fs.files[filepath.Join(out, "audit.redacted.jsonl")])
	lines := strings.Split(strings.TrimSuffix(audit, "\n"), "\n")
	if len(lines) != m.Events {
		t.Fatalf("line count %d vs %d", len(lines), m.Events)
	}
	var initLine map[string]any
	_ = json.Unmarshal([]byte(lines[0]), &initLine)
	data := initLine["data"].(map[string]any)
	cmd := data["allowed_cmds"].([]any)[0].(map[string]any)
	argv := cmd["argv"].([]any)
	if argv[0] != "~/bin/tool" || argv[1] != "--token=$SECRET" || argv[2] != "$TARGET" || argv[3] != "src/x" {
		t.Fatalf("argv: %v", argv)
	}
	fh := cmd["file_hashes"].([]any)
	if len(fh) != 4 || fh[0].(map[string]any)["path"] != "$SECRET" || fh[1].(map[string]any)["path"] != "$SECRET" || fh[2].(map[string]any)["path"] != "$TARGET" || fh[3].(map[string]any)["path"] != "~/bin/tool" {
		t.Fatalf("file_hashes (collisions kept, sorted): %v", fh)
	}
	if data["repo_root"] != "." || data["work_dir"] != "wt" || data["exported"] != true || !strings.HasPrefix(data["vars"].(map[string]any)["SECRET"].(string), "sha256:") || data["vars"].(map[string]any)["REV"] == "ab" {
		t.Fatalf("init fields: %v", data)
	}
	if !strings.Contains(lines[0], `"redacted":["exported","vars","allowed_cmds","repo_root","work_dir"]`) || !strings.Contains(lines[0], `"seq":1`) {
		t.Fatalf("init markers: %s", lines[0])
	}
	// unredacted lines are byte-identical to the source and verify pairwise
	_, src, _ := h.store.EventsWithLines(runA)
	redactedSet := map[int64]bool{}
	for _, s := range m.Redacted {
		redactedSet[s] = true
	}
	for i, line := range lines {
		seq := int64(i + 1)
		if !redactedSet[seq] && line != string(src[i]) {
			t.Fatalf("unredacted line %d differs", seq)
		}
		if i > 0 && !redactedSet[seq] && !redactedSet[seq-1] {
			var ev run.Event
			_ = json.Unmarshal([]byte(line), &ev)
			if ev.Prev != run.LineHash(src[i-1]) {
				t.Fatalf("pairwise chain at %d", seq)
			}
		}
	}
	// tree.status is a sorted path list; gate detail summary carries the porcelain paths; cmd streams emptied
	for _, line := range lines {
		var ev map[string]any
		_ = json.Unmarshal([]byte(line), &ev)
		d := ev["data"].(map[string]any)
		switch ev["type"] {
		case "tree":
			if st, ok := d["status"].([]any); !ok || (len(st) != 0 && len(st) != 2) {
				t.Fatalf("tree status: %v", d["status"])
			} else if len(st) == 2 && (st[0] != "porcelain-file.go" || st[1] != "untracked-marker.txt") {
				t.Fatalf("tree paths: %v", st)
			}
		case "gate":
			if e, ok := d["error"].(map[string]any); ok {
				if e["detail"] != "" || e["detail_truncated"] != true {
					t.Fatalf("gate detail: %v", e)
				}
				files := e["detail_summary"].(map[string]any)["files"].([]any)
				if len(files) != 1 || files[0] != "porcelain-diff-file.go" || e["detail_summary"].(map[string]any)["truncated"] != true {
					t.Fatalf("detail_summary: %v", e["detail_summary"])
				}
			}
		case "cmd_call", "overflow_handler":
			if d["stdout"] != "" || d["stderr"] != "" || d["stdout_truncated"] != true || d["stderr_truncated"] != true {
				t.Fatalf("streams: %v", d)
			}
		case "converge":
			if d["atom"] == "cmd:notify" && d["reason"] != "" || d["atom"] == "all_fixed" && d["reason"] != "0 unfixed kept" {
				t.Fatalf("converge: %v", d)
			}
		case "node_output":
			if _, ok := d["output"].(map[string]any); !ok || len(d["output"].(map[string]any)) == 0 {
				t.Fatalf("known-kind outputs are kept: %v", d)
			}
		}
	}
	// snapshot: redacted projection with kept fields and the marker
	var snap map[string]any
	_ = json.Unmarshal(h.fs.files[filepath.Join(out, "snapshot.json")], &snap)
	if snap["redacted"] != true || snap["head"] != head1 || snap["outcome"] != "fixed" || snap["repo_root"] != "." || snap["work_dir"] != "wt" || len(snap["goldens"].([]any)) != 1 || len(snap["findings"].([]any)) != 1 || len(snap["node_outputs"].(map[string]any)) != 4 || snap["schemaVersion"] != float64(1) || snap["stop_reason"] != "all_fixed" {
		t.Fatalf("snapshot: %v", snap)
	}
	if ts, ok := snap["tree_status"].([]any); !ok || len(ts) != 0 {
		t.Fatalf("tree_status list: %v", snap["tree_status"])
	}
	// manifest
	if m.SchemaVersion != 1 || m.RunID != runA || m.Workflow != "sdlc-loop" || m.WorkflowHash != "wh" || m.WorkflowSource != "embedded" || m.SourceHead != head1 || m.ChainHead == "" || m.IncludeVars || m.Chain != "redacted" || len(m.Records) != 1 || len(m.Sidecars) != 2 || len(m.TornFiles) != 0 || len(m.OriginChecks) != 0 || m.Bytes <= 0 {
		t.Fatalf("manifest: %+v", m)
	}
	var mj Manifest
	if err := json.Unmarshal(h.fs.files[filepath.Join(out, "manifest.json")], &mj); err != nil || mj.ChainHead != m.ChainHead {
		t.Fatalf("manifest.json: %v", err)
	}
	if string(h.fs.files[filepath.Join(out, "workflow.yaml")]) != string(h.raw) || string(h.fs.files[filepath.Join(out, "notes.md")]) != "sidecar bytes" {
		t.Fatal("sidecars copied")
	}
	// destination flags and perms: verify O_NOFOLLOW is present (symlink-attack control)
	expectedFlags := os.O_WRONLY | os.O_CREATE | os.O_EXCL | syscall.O_NOFOLLOW
	for _, o := range h.fs.opens {
		parts := strings.Fields(o)
		if len(parts) < 3 {
			t.Fatalf("open string malformed: %s", o)
		}
		flagStr := parts[len(parts)-2]
		permStr := parts[len(parts)-1]
		var flag int
		if _, err := fmt.Sscanf(flagStr, "%d", &flag); err != nil {
			t.Fatalf("cannot parse flag %q in %s: %v", flagStr, o, err)
		}
		if flag != expectedFlags {
			t.Fatalf("open flags mismatch: got %d (0x%x), expected %d (0x%x) with O_NOFOLLOW=%d; string: %s", flag, flag, expectedFlags, expectedFlags, syscall.O_NOFOLLOW, o)
		}
		if permStr != "-rw-------" {
			t.Fatalf("open perm mismatch: got %s, expected -rw-------; string: %s", permStr, o)
		}
		if flag&syscall.O_NOFOLLOW == 0 {
			t.Fatalf("O_NOFOLLOW missing from flags in %s", o)
		}
	}
	if len(h.fs.mkdirs) != 1 || !strings.HasSuffix(h.fs.mkdirs[0], out+" -rwx------") {
		t.Fatalf("mkdir: %v", h.fs.mkdirs)
	}
	// include vars: values present, argv untouched, explicit --out required
	h2 := newHarness(t)
	if _, err := Export(ctx, h2.deps, runA, Options{IncludeVars: true}); !errs.Is(err, CodeExportDest) || errs.As(err).Fields["reason"] != "include_vars_default" {
		t.Fatalf("include-vars default: %v", err)
	}
	m2, err := Export(ctx, h2.deps, runA, Options{IncludeVars: true, Out: "/tmp/x/bundle"})
	if err != nil || !m2.IncludeVars {
		t.Fatal(err)
	}
	all2 := allBytes(h2.fs)
	if !strings.Contains(all2, `"SECRET":"`+secret+`"`) || !strings.Contains(all2, "--token="+secret) {
		t.Fatal("include-vars must keep values")
	}
	// legacy init without workflow_source → manifest "" (the recorder maps it; the exporter reports the stored value)
	if m.WorkflowSource == "" {
		t.Fatal("source")
	}
}

func TestF8Refusals(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	out := "/repo/docs/metareview/fsm/" + runA
	// MaxBytes refusal writes nothing
	_, err := Export(ctx, h.deps, runA, Options{MaxBytes: 10})
	if !errs.Is(err, CodeExportTooLarge) || len(h.fs.opens) != 0 || len(h.fs.mkdirs) != 0 {
		t.Fatalf("max bytes: %v %v", err, h.fs.opens)
	}
	// intermediate symlink component
	h.fs.dirs["/repo"] = true
	h.fs.symlinks["/repo/docs"] = true
	if _, err := Export(ctx, h.deps, runA, Options{}); !errs.Is(err, CodeExportDest) || errs.As(err).Fields["reason"] != "symlink" || errs.As(err).Fields["path"] != "/repo/docs" {
		t.Fatalf("symlink: %v", err)
	}
	delete(h.fs.symlinks, "/repo/docs")
	// non-empty destination
	h.fs.dirs["/repo"], h.fs.dirs["/repo/docs"], h.fs.dirs["/repo/docs/metareview"], h.fs.dirs["/repo/docs/metareview/fsm"], h.fs.dirs[out] = true, true, true, true, true
	h.fs.files[filepath.Join(out, "stale")] = []byte("x")
	if _, err := Export(ctx, h.deps, runA, Options{}); !errs.Is(err, CodeExportDest) || errs.As(err).Fields["reason"] != "not_empty" {
		t.Fatalf("not empty: %v", err)
	}
	delete(h.fs.files, filepath.Join(out, "stale"))
	// FS failures pass through
	h.fs.lstatErr = errors.New("lstat failed")
	if _, err := Export(ctx, h.deps, runA, Options{}); err == nil || err.Error() != "lstat failed" {
		t.Fatalf("lstat: %v", err)
	}
	h.fs.lstatErr = nil
	h.fs.readErr = errors.New("readdir failed")
	if _, err := Export(ctx, h.deps, runA, Options{}); err == nil || err.Error() != "readdir failed" {
		t.Fatalf("readdir: %v", err)
	}
	h.fs.readErr = nil
	h.fs.mkdirErr = errors.New("mkdir failed")
	if _, err := Export(ctx, h.deps, runA, Options{}); err == nil || err.Error() != "mkdir failed" {
		t.Fatalf("mkdir: %v", err)
	}
	h.fs.mkdirErr = nil
	h.fs.openErr = errors.New("open failed")
	if _, err := Export(ctx, h.deps, runA, Options{}); err == nil || err.Error() != "open failed" {
		t.Fatalf("open: %v", err)
	}
	h.fs.openErr = nil
	h.fs.writeErr = errors.New("write failed")
	if _, err := Export(ctx, h.deps, runA, Options{}); err == nil || err.Error() != "write failed" {
		t.Fatalf("write: %v", err)
	}
	h.fs.writeErr = nil
	h.fs.files = map[string][]byte{}
	h.fs.closeErr = errors.New("close failed")
	if _, err := Export(ctx, h.deps, runA, Options{}); err == nil || err.Error() != "close failed" {
		t.Fatalf("close: %v", err)
	}
	h.fs.closeErr = nil
	// run/store/sidecar failures
	if _, err := Export(ctx, h.deps, "../x", Options{}); !errs.Is(err, run.CodeRunNotFound) {
		t.Fatalf("bad id: %v", err)
	}
	if _, err := Export(ctx, h.deps, "mrv-missing-0000001", Options{}); err == nil {
		t.Fatal("missing run")
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Export(cctx, h.deps, runA, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx: %v", err)
	}
	// unparseable sidecar, missing sidecar, unknown kind fails closed, torn files listed, origin checks reported
	h3 := newHarness(t)
	h3.sidecar.Put(runA, machine.SidecarWorkflow, []byte("workflow: ["))
	if _, err := Export(ctx, h3.deps, runA, Options{}); err == nil {
		t.Fatal("bad sidecar")
	}
	h4 := newHarness(t)
	h4.sidecar = &machine.MemSidecar{}
	h4.deps.Sidecar = h4.sidecar
	if _, err := Export(ctx, h4.deps, runA, Options{}); err == nil {
		t.Fatal("missing sidecar")
	}
	h5 := newHarness(t)
	changed := strings.Replace(string(h5.raw), "discover:   {kind: review-lenses,", "discover:   {kind: mystery,", 1)
	h5.sidecar.Put(runA, machine.SidecarWorkflow, []byte(changed))
	h5.deps.Kinds = &mysteryRegistry{Registry: h5.deps.Kinds}
	if _, err := Export(ctx, h5.deps, runA, Options{}); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(h5.fs.files["/repo/docs/metareview/fsm/"+runA+"/audit.redacted.jsonl"]), "\n") {
		var ev map[string]any
		_ = json.Unmarshal([]byte(line), &ev)
		if ev["type"] == "node_output" && ev["node"] == "discover" {
			if o := ev["data"].(map[string]any)["output"].(map[string]any); len(o) != 0 || !strings.Contains(line, `"redacted":["output"]`) {
				t.Fatalf("unknown kind must fail closed: %s", line)
			}
		}
	}
	var snap map[string]any
	_ = json.Unmarshal(h5.fs.files["/repo/docs/metareview/fsm/"+runA+"/snapshot.json"], &snap)
	if len(snap["node_outputs"].(map[string]any)["discover@0"].(map[string]any)) != 0 {
		t.Fatal("snapshot output for an unknown kind must be empty")
	}
	// a fold error on the stored log passes through
	h6 := newHarness(t)
	h6.deps.Store = &badStore{RunStore: h6.store}
	if _, err := Export(ctx, h6.deps, runA, Options{}); err == nil {
		t.Fatal("fold error")
	}
	h6.deps.Store = &badStore{RunStore: h6.store, tornErr: errors.New("torn list failed")}
	if _, err := Export(ctx, h6.deps, runA, Options{}); err == nil || err.Error() != "torn list failed" {
		t.Fatalf("torn files: %v", err)
	}
	h7 := newHarness(t)
	h7.deps.Sidecar = &listFailSidecar{Sidecar: h7.sidecar}
	if _, err := Export(ctx, h7.deps, runA, Options{}); err == nil || err.Error() != "list failed" {
		t.Fatalf("sidecar list: %v", err)
	}
	h7.deps.Sidecar = &listFailSidecar{Sidecar: h7.sidecar, names: []string{machine.SidecarWorkflow, "ghost"}}
	if _, err := Export(ctx, h7.deps, runA, Options{}); err == nil {
		t.Fatal("ghost sidecar read")
	}
	// a run without record events reports an empty records list
	h8 := newHarness(t)
	evs := markedLog(t).Events()
	var noRec []run.Event
	b := run.NewBuilder("mrv-export-b-000001")
	var init run.InitData
	_ = json.Unmarshal(evs[0].Data, &init)
	init.RunID = "mrv-export-b-000001"
	b.Init(init)
	noRec = b.Events()
	seedRun(t, h8.store, "mrv-export-b-000001", noRec)
	h8.sidecar.Put("mrv-export-b-000001", machine.SidecarWorkflow, h8.raw)
	if m, err := Export(ctx, h8.deps, "mrv-export-b-000001", Options{}); err != nil || len(m.Records) != 0 || len(m.Redacted) != 1 {
		t.Fatalf("no records: %v %+v", err, m)
	}
	// event/snapshot branches on crafted inputs: mock + origin stamps, unknown node, last_error, cmd stop reason
	w, _ := workflow.Parse(h8.raw, workflow.Options{Kinds: h8.deps.Kinds.Info()})
	rd := &redactor{snap: run.Snapshot{RepoRoot: "/repo", WorkDir: "/repo", LastError: &run.GateError{Detail: "secret-detail"}, StopReason: "cmd:notify: plateau", NodeOutputs: map[string]json.RawMessage{"ghost@0": json.RawMessage(`{"x":1}`)}}, w: w, repoRoot: "/repo"}
	if rd.kindOf("ghost") != "" {
		t.Fatal("unknown node")
	}
	var snap2 map[string]any
	_ = json.Unmarshal(rd.snapshot(), &snap2)
	if snap2["last_error"].(map[string]any)["detail"] != "" || snap2["stop_reason"] != "" || len(snap2["node_outputs"].(map[string]any)["ghost@0"].(map[string]any)) != 0 {
		t.Fatalf("snapshot branches: %v", snap2)
	}
	line, fields := rd.event(run.Event{Type: run.TypeWarn, State: "discover", Node: "n", Mock: true, Origin: &run.Origin{RunID: "p", Seq: 2, Version: 1, Hash: "h"}, Data: json.RawMessage(`{"code":"X","detail":"d"}`)})
	if len(fields) != 1 || !strings.Contains(string(line), `"mock":true`) || !strings.Contains(string(line), `"origin":{`) || !strings.Contains(string(line), `"node":"n"`) || !strings.Contains(string(line), `"state":"discover"`) {
		t.Fatalf("stamps: %s", line)
	}
	if line, fields := rd.event(run.Event{Type: run.TypeWarn, Data: json.RawMessage(`{"code":"X"}`)}); line != nil || fields != nil {
		t.Fatal("a warn without detail is unredacted")
	}
	// helpers
	if p := statusPaths(""); len(p) != 0 {
		t.Fatal("empty status")
	}
	if p := statusBlock("no block"); len(p) != 0 {
		t.Fatal("no block")
	}
	r := &redactor{snap: run.Snapshot{RepoRoot: "/repo"}, repoRoot: "/repo", home: "/home/dave"}
	if r.rootPath("/elsewhere") != "<outside>" || r.rootPath("/repo") != "." || r.path("/elsewhere/x") != "/elsewhere/x" || r.path("/home/dave") != "~" {
		t.Fatal("paths")
	}
}

func TestOSFS(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	var fs FS = OSFS{}
	if err := fs.MkdirAll(filepath.Join(dir, "a", "b"), 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "a", "b", "f")
	if err := writeFile(fs, p, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v %v", fi, err)
	}
	if err := writeFile(fs, p, []byte("y")); err == nil {
		t.Fatal("O_EXCL: existing file must refuse")
	}
	_ = os.Symlink(p, filepath.Join(dir, "link"))
	if fi, err := fs.Lstat(filepath.Join(dir, "link")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("lstat symlink")
	}
	if entries, err := fs.ReadDir(filepath.Join(dir, "a", "b")); err != nil || len(entries) != 1 {
		t.Fatal("readdir")
	}
	// end to end against the real filesystem
	h := newHarness(t)
	h.deps.FS = OSFS{}
	h.deps.RepoRoot = dir
	if _, err := Export(context.Background(), h.deps, runA, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "metareview", "fsm", runA, "manifest.json")); err != nil {
		t.Fatal(err)
	}
}

// ---- more fakes ------------------------------------------------------------------------------------

type mysteryRegistry struct{ machine.Registry }

func (m *mysteryRegistry) Info() map[string]workflow.KindInfo {
	info := m.Registry.Info()
	info["mystery"] = info["review-lenses"]
	return info
}

type badStore struct {
	run.RunStore
	tornErr error
}

func (b *badStore) EventsWithLines(id string) (run.Log, [][]byte, error) {
	if b.tornErr != nil {
		return b.RunStore.EventsWithLines(id)
	}
	log, lines, err := b.RunStore.EventsWithLines(id)
	log.Events[1].Data = json.RawMessage(`{"bogus":1}`)
	return log, lines, err
}
func (b *badStore) TornFiles(id string) ([]run.TornFile, error) {
	if b.tornErr != nil {
		return nil, b.tornErr
	}
	return b.RunStore.TornFiles(id)
}

type listFailSidecar struct {
	machine.Sidecar
	names []string
}

func (l *listFailSidecar) List(string) ([]string, error) {
	if l.names == nil {
		return nil, errors.New("list failed")
	}
	return l.names, nil
}
