// Package export writes a redacted, read-only bundle of one run (spec 3 §5): manifest.json, audit.redacted.jsonl,
// snapshot.json, workflow.yaml and the sidecars. The bundle is evidence for a reader, never a foldable log.
package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/machine"
	"github.com/dsifry/metareview/internal/fsm/run"
	"github.com/dsifry/metareview/internal/fsm/workflow"
)

// Error codes owned here.
const (
	CodeExportDest     = "ERR_EXPORT_DEST"
	CodeExportTooLarge = "ERR_EXPORT_TOO_LARGE"
)

// DefaultMaxBytes caps a bundle unless Options.MaxBytes overrides it.
const DefaultMaxBytes = 5 << 20

// minVarLen is the shortest var value that is substituted in argv (shorter values are exported verbatim).
const minVarLen = 4

// FS is the destination seam: every write goes through it so tests assert flags and perms literally.
type FS interface {
	MkdirAll(dir string, perm os.FileMode) error
	OpenFile(path string, flag int, perm os.FileMode) (io.WriteCloser, error)
	Lstat(path string) (os.FileInfo, error)
	ReadDir(dir string) ([]os.DirEntry, error)
}

// Deps wires Export.
type Deps struct {
	Store    run.RunStore
	Sidecar  machine.Sidecar
	Kinds    machine.Registry
	FS       FS
	Clock    machine.Clock
	RepoRoot string // the path root for relativisation and the default Out
	Home     string // the "~" prefix; "" disables
}

// Options parameterizes Export.
type Options struct {
	Out         string
	IncludeVars bool
	MaxBytes    int64
}

// Manifest is manifest.json.
type Manifest struct {
	SchemaVersion  int                   `json:"schema_version"`
	RunID          string                `json:"run_id"`
	Workflow       string                `json:"workflow"`
	WorkflowHash   string                `json:"workflow_hash"`
	WorkflowSource string                `json:"workflow_source"`
	ExportedAt     run.Time              `json:"exported_at"`
	SourceHead     string                `json:"source_head"`
	ChainHead      string                `json:"chain_head"`
	IncludeVars    bool                  `json:"include_vars"`
	Events         int                   `json:"events"`
	Redacted       []int64               `json:"redacted"`
	Records        []int64               `json:"records"`
	Sidecars       []string              `json:"sidecars"`
	TornFiles      []run.TornFile        `json:"torn_files"`
	OriginChecks   []machine.OriginCheck `json:"origin_checks"`
	Bytes          int64                 `json:"bytes"` // every written file except manifest.json itself
	Chain          string                `json:"chain"` // always "redacted"
}

// Export builds the bundle in memory, checks MaxBytes and the destination, then writes (spec 3 §5).
func Export(ctx context.Context, deps Deps, runID string, opts Options) (Manifest, error) {
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	if err := run.ValidateRunID(runID); err != nil {
		return Manifest{}, errs.E(run.CodeRunNotFound, err.Error(), "detail", runID)
	}
	log, lines, err := deps.Store.EventsWithLines(runID)
	if err != nil {
		return Manifest{}, err
	}
	st, err := run.FoldFull(log.Events)
	if err != nil {
		return Manifest{}, err
	}
	snap := st.Snapshot
	raw, err := deps.Sidecar.Read(runID, machine.SidecarWorkflow)
	if err != nil {
		return Manifest{}, err
	}
	w, err := workflow.Parse(raw, workflow.Options{Kinds: deps.Kinds.Info()})
	if err != nil {
		return Manifest{}, err
	}
	names, err := deps.Sidecar.List(runID)
	if err != nil {
		return Manifest{}, err
	}
	torn, err := deps.Store.TornFiles(runID)
	if err != nil {
		return Manifest{}, err
	}
	out := opts.Out
	if out == "" {
		if opts.IncludeVars {
			return Manifest{}, errs.E(CodeExportDest, "--include-vars needs an explicit --out: cleartext var values never land in the default, committed tree", "reason", "include_vars_default")
		}
		out = filepath.Join(deps.RepoRoot, "docs", "metareview", "fsm", runID)
	}
	max := opts.MaxBytes
	if max <= 0 {
		max = DefaultMaxBytes
	}
	r := &redactor{snap: snap, w: w, includeVars: opts.IncludeVars, repoRoot: deps.RepoRoot, home: deps.Home}
	files := map[string][]byte{}
	var audit []byte
	redactedSeqs := []int64{} // never empty: init always carries the exported marker
	var records []int64
	for i, ev := range log.Events {
		line, fields := r.event(ev)
		if len(fields) == 0 {
			line = lines[i]
		} else {
			redactedSeqs = append(redactedSeqs, ev.Seq)
		}
		if ev.Type == run.TypeRecord {
			records = append(records, ev.Seq)
		}
		audit = append(audit, line...)
		audit = append(audit, '\n')
	}
	files["audit.redacted.jsonl"] = audit
	files["snapshot.json"] = r.snapshot()
	files[machine.SidecarWorkflow] = raw
	for _, n := range names {
		if n == machine.SidecarWorkflow {
			continue
		}
		b, err := deps.Sidecar.Read(runID, n)
		if err != nil {
			return Manifest{}, err
		}
		files[n] = b
	}
	if records == nil {
		records = []int64{}
	}
	m := Manifest{
		SchemaVersion: 1, RunID: runID, Workflow: snap.Workflow, WorkflowHash: snap.WorkflowHash, WorkflowSource: snap.WorkflowSource,
		ExportedAt: deps.Clock(), SourceHead: snap.Head, ChainHead: log.Head, IncludeVars: opts.IncludeVars, Events: len(log.Events),
		Redacted: redactedSeqs, Records: records, Sidecars: names, TornFiles: torn, OriginChecks: machine.VerifyOrigin(ctx, deps.Store, log), Chain: "redacted",
	}
	for _, b := range files {
		m.Bytes += int64(len(b))
	}
	if m.Bytes > max {
		return Manifest{}, errs.E(CodeExportTooLarge, fmt.Sprintf("bundle is %d bytes (max %d)", m.Bytes, max), "bytes", fmt.Sprint(m.Bytes), "max", fmt.Sprint(max))
	}
	files["manifest.json"] = append(run.MarshalCanonical(m), '\n')
	if err := checkDest(deps.FS, out); err != nil {
		return Manifest{}, err
	}
	if err := deps.FS.MkdirAll(out, 0o700); err != nil {
		return Manifest{}, err
	}
	names = make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := writeFile(deps.FS, filepath.Join(out, n), files[n]); err != nil {
			return Manifest{}, err
		}
	}
	return m, nil
}

// checkDest walks every component from the filesystem root down to Out: a symlink refuses, ENOENT on trailing
// components is tolerated (MkdirAll creates them), an existing non-empty Out refuses.
func checkDest(fs FS, out string) error {
	comps := strings.Split(strings.TrimPrefix(filepath.Clean(out), string(filepath.Separator)), string(filepath.Separator))
	p := string(filepath.Separator)
	for _, c := range comps {
		p = filepath.Join(p, c)
		fi, err := fs.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return errs.E(CodeExportDest, "a path component is a symlink: "+p, "reason", "symlink", "path", p)
		}
	}
	entries, err := fs.ReadDir(out)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return errs.E(CodeExportDest, "destination is not empty: "+out, "reason", "not_empty", "path", out)
	}
	return nil
}

func writeFile(fs FS, path string, b []byte) error {
	f, err := fs.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// ---- redaction --------------------------------------------------------------------------------

type redactor struct {
	snap        run.Snapshot
	w           *workflow.Workflow
	includeVars bool
	repoRoot    string
	home        string
	values      []string // var values, longest first (computed lazily)
}

func (r *redactor) varValues() []string {
	if r.values == nil {
		r.values = []string{}
		for _, v := range r.snap.Vars {
			if len(v) >= minVarLen {
				r.values = append(r.values, v)
			}
		}
		sort.Slice(r.values, func(i, j int) bool {
			if len(r.values[i]) != len(r.values[j]) {
				return len(r.values[i]) > len(r.values[j])
			}
			return r.values[i] < r.values[j]
		})
	}
	return r.values
}

func (r *redactor) nameOf(value string) string {
	names := make([]string, 0, 1)
	for k, v := range r.snap.Vars {
		if v == value {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	return names[0]
}

// argv substitutes var values (longest first, single pass per element) then relativises paths.
func (r *redactor) argv(elems []any) []any {
	out := make([]any, len(elems))
	for i, e := range elems {
		s, _ := e.(string)
		out[i] = r.path(r.subst(s))
	}
	return out
}

func (r *redactor) subst(s string) string {
	if r.includeVars {
		return s
	}
	for _, v := range r.varValues() {
		s = strings.ReplaceAll(s, v, "$"+r.nameOf(v))
	}
	return s
}

// path relativises an in-repo absolute path, rewrites a home-prefixed one to ~, and leaves everything else alone.
func (r *redactor) path(s string) string {
	if r.repoRoot != "" && (s == r.repoRoot || strings.HasPrefix(s, r.repoRoot+string(filepath.Separator))) {
		rel, _ := filepath.Rel(r.repoRoot, s)
		return rel
	}
	if r.home != "" && (s == r.home || strings.HasPrefix(s, r.home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(s, r.home)
	}
	return s
}

func (r *redactor) rootPath(s string) string {
	if r.repoRoot != "" && (s == r.repoRoot || strings.HasPrefix(s, r.repoRoot+string(filepath.Separator))) {
		rel, _ := filepath.Rel(r.repoRoot, s)
		return rel
	}
	return "<outside>"
}

func sha(v string) string {
	sum := sha256.Sum256([]byte(v))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// fileHashes turns the map into a sorted list of {path, sha256} with substituted, relativised paths (collisions keep both).
func (r *redactor) fileHashes(m map[string]any) []any {
	out := make([]any, 0, len(m))
	for k, v := range m {
		out = append(out, map[string]any{"path": r.path(r.subst(k)), "sha256": v})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].(map[string]any), out[j].(map[string]any)
		if a["path"] != b["path"] {
			return a["path"].(string) < b["path"].(string)
		}
		return a["sha256"].(string) < b["sha256"].(string)
	})
	return out
}

func (r *redactor) allowedCmds(list []any) {
	for _, c := range list {
		cmd := c.(map[string]any)
		if argv, ok := cmd["argv"].([]any); ok {
			cmd["argv"] = r.argv(argv)
		}
		if fh, ok := cmd["file_hashes"].(map[string]any); ok {
			cmd["file_hashes"] = r.fileHashes(fh)
		}
	}
}

// statusPaths reduces a porcelain status to the sorted list of paths (the last field of each line).
func statusPaths(status string) []string {
	paths := []string{}
	for _, line := range strings.Split(status, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		paths = append(paths, f[len(f)-1])
	}
	sort.Strings(paths)
	return paths
}

func (r *redactor) kindOf(node string) string {
	if n := r.w.NodeFor(run.State(node)); n != nil {
		return n.Kind
	}
	return ""
}

// known kinds keep their outputs whole; anything else fails closed.
var knownKinds = map[string]bool{"review-lenses": true, "match-then-adjudicate": true, "agent-edit": true, "still-present": true, "cmd": true}

// event returns the redacted line for ev and the list of redacted fields (empty → the original line is emitted).
func (r *redactor) event(ev run.Event) ([]byte, []string) {
	var data map[string]any
	_ = json.Unmarshal(ev.Data, &data)
	var fields []string
	mark := func(f string) { fields = append(fields, f) }
	switch ev.Type {
	case run.TypeInit:
		data["exported"] = true
		mark("exported")
		if !r.includeVars {
			if vars, ok := data["vars"].(map[string]any); ok {
				for k, v := range vars {
					vars[k] = sha(v.(string))
				}
				mark("vars")
			}
		}
		if list, ok := data["allowed_cmds"].([]any); ok && len(list) > 0 {
			r.allowedCmds(list)
			mark("allowed_cmds")
		}
		data["repo_root"] = r.rootPath(data["repo_root"].(string))
		data["work_dir"] = r.rootPath(data["work_dir"].(string))
		mark("repo_root")
		mark("work_dir")
	case run.TypeGate:
		if e, ok := data["error"].(map[string]any); ok {
			detail, _ := e["detail"].(string)
			truncated, _ := e["detail_truncated"].(bool)
			if data["name"] == "commit_exists" {
				e["detail_summary"] = map[string]any{"files": statusBlock(detail), "truncated": truncated}
			}
			e["detail"] = ""
			mark("error.detail")
		}
	case run.TypeCmdCall, run.TypeOverflowHandler:
		if argv, ok := data["argv"].([]any); ok {
			data["argv"] = r.argv(argv)
		}
		data["stdout"], data["stderr"] = "", ""
		data["stdout_truncated"], data["stderr_truncated"] = true, true
		mark("argv")
		mark("stdout")
		mark("stderr")
	case run.TypeConverge:
		if atom, _ := data["atom"].(string); strings.HasPrefix(atom, "cmd:") {
			data["reason"] = ""
			mark("reason")
		}
	case run.TypeWarn:
		if _, ok := data["detail"]; ok {
			data["detail"] = ""
			mark("detail")
		}
	case run.TypeNodeOutput:
		if !knownKinds[r.kindOf(ev.Node)] {
			data["output"] = map[string]any{}
			mark("output")
			break
		}
		// A known kind keeps its output whole — but agent-edit's output CARRIES pins, whose
		// from/to are literal source. Hashing the snapshot's copy while leaving this one in the
		// clear made that redaction cosmetic: the same fragments left on the same export, one
		// field over.
		if out, ok := data["output"].(map[string]any); ok && hashPinFragments(out) {
			mark("output.pins")
		}
	case run.TypeTree:
		status, _ := data["status"].(string)
		data["status"] = statusPaths(status)
		mark("status")
	case run.TypeDeltaApplied:
		// There was no case here at all, so the raw line went straight into the audit log. That
		// was harmless while a Delta held only findings and ids; it stopped being harmless when
		// the Delta gained pins and pin_results, and FIXING the dropped-pins defect is what
		// makes this line actually carry source.
		if hashPinFragments(data) {
			mark("pins")
		}
		if results, ok := data["pin_results"].([]any); ok {
			for _, r := range results {
				result, _ := r.(map[string]any)
				hashPinFragments(result)
				// Detail quotes the mutation and up to 400 bytes of test output verbatim.
				if _, present := result["detail"]; present {
					result["detail"] = ""
				}
			}
			mark("pin_results")
		}
	}
	if len(fields) == 0 {
		return nil, nil
	}
	line := map[string]any{"schemaVersion": ev.SchemaVersion, "seq": ev.Seq, "prev": ev.Prev, "at": ev.At, "type": ev.Type, "iter": ev.Iter, "data": data, "redacted": fields}
	if ev.State != "" {
		line["state"] = ev.State
	}
	if ev.Node != "" {
		line["node"] = ev.Node
	}
	if ev.Mock {
		line["mock"] = true
	}
	if ev.Origin != nil {
		line["origin"] = ev.Origin
	}
	return run.MarshalCanonical(line), fields
}

// statusBlock extracts the paths of the porcelain block inside a commit_exists detail.
func statusBlock(detail string) []string {
	_, rest, ok := strings.Cut(detail, "--- status ---\n")
	if !ok {
		return []string{}
	}
	block, _, _ := strings.Cut(rest, "--- working diff ---")
	return statusPaths(block)
}

// snapshot is the redacted projection of the fold, with a top-level redacted marker.
func (r *redactor) snapshot() []byte {
	var m map[string]any
	_ = json.Unmarshal(run.MarshalCanonical(r.snap), &m)
	if !r.includeVars {
		if vars, ok := m["vars"].(map[string]any); ok {
			for k, v := range vars {
				vars[k] = sha(v.(string))
			}
		}
	}
	if list, ok := m["allowed_cmds"].([]any); ok {
		r.allowedCmds(list)
	}
	m["repo_root"] = r.rootPath(r.snap.RepoRoot)
	m["work_dir"] = r.rootPath(r.snap.WorkDir)
	m["tree_status"] = statusPaths(r.snap.TreeStatus)
	// Pins carry literal source fragments in from/to, which is repository content by any
	// definition — the same reason tree_status is reduced to paths. The path and the test name
	// stay, because they identify what was proven without reproducing the code, and the fragments
	// are hashed so a holder of the source can still verify the export describes their tree.
	// unproven holds the same Pin shape and is redacted identically. It is listed here rather
	// than left to inherit the treatment, because a field that carries source fragments and is
	// merely forgotten exports the repository's code in the clear.
	for _, key := range []string{"pins", "unproven"} {
		pins, ok := m[key].([]any)
		if !ok {
			continue
		}
		for _, p := range pins {
			// A nil map from a failed assertion is safe to index in Go and yields nothing, so the
			// unreachable "not a map" branch is left out rather than written and never executed.
			pin, _ := p.(map[string]any)
			for _, field := range []string{"from", "to"} {
				if v, ok := pin[field].(string); ok && v != "" {
					pin[field] = sha(v)
				}
			}
		}
	}
	if le, ok := m["last_error"].(map[string]any); ok {
		le["detail"] = ""
	}
	if strings.HasPrefix(r.snap.StopReason, "cmd:") {
		m["stop_reason"] = ""
	}
	if outs, ok := m["node_outputs"].(map[string]any); ok {
		for key := range outs {
			node, _, _ := strings.Cut(key, "@")
			if !knownKinds[r.kindOf(node)] {
				outs[key] = map[string]any{}
			}
		}
	}
	m["redacted"] = true
	return append(run.MarshalCanonical(m), '\n')
}

// OSFS is the os-backed FS adapter used by the CLI.
type OSFS struct{}

func (OSFS) MkdirAll(dir string, perm os.FileMode) error { return os.MkdirAll(dir, perm) }
func (OSFS) OpenFile(path string, flag int, perm os.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(path, flag, perm)
}
func (OSFS) Lstat(path string) (os.FileInfo, error)    { return os.Lstat(path) }
func (OSFS) ReadDir(dir string) ([]os.DirEntry, error) { return os.ReadDir(dir) }

// hashPinFragments replaces the literal source in every pin reachable from m, in place. It reads
// both a "pins" list and a single embedded "pin", so one function covers the snapshot field, the
// agent-edit node output and a pin result. Reports whether anything was rewritten.
//
// The path and the test name stay: they identify what was proven without reproducing the code,
// which is the same bargain the snapshot redaction already makes.
func hashPinFragments(m map[string]any) bool {
	hashOne := func(pin map[string]any) bool {
		var done bool
		for _, field := range []string{"from", "to"} {
			if v, ok := pin[field].(string); ok && v != "" {
				pin[field] = sha(v)
				done = true
			}
		}
		return done
	}
	var any_ bool
	if pin, ok := m["pin"].(map[string]any); ok {
		any_ = hashOne(pin) || any_
	}
	if pins, ok := m["pins"].([]any); ok {
		for _, p := range pins {
			pin, _ := p.(map[string]any)
			any_ = hashOne(pin) || any_
		}
	}
	return any_
}
