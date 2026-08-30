package export

import (
	"encoding/json"
	"github.com/dsifry/metareview/internal/fsm/kind"
	"github.com/dsifry/metareview/internal/fsm/workflow"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/run"
)

// Spec 3 §5 requires the export to fail closed: a field added to run.Snapshot
// must not reach an evidence bundle before someone has decided whether it is
// safe to export. redactor.snapshot() is a hand-written if-chain, so nothing
// structural enforced that — a new field would export verbatim with the whole
// suite green and the coverage gate satisfied, because coverage cannot see a
// field it was never told about.
//
// This test is that enforcement. Every JSON field of run.Snapshot must appear
// in exactly one of the two sets below. Adding a field to the struct and not to
// a set fails here, which is the point: the failure is the question "is this
// safe to export?" being asked of the author.

// transformed are the fields redactor.snapshot() rewrites, blanks, or hashes.
var transformed = map[string]string{
	"vars":         "hashed unless --include-vars",
	"allowed_cmds": "rewritten by allowedCmds",
	"repo_root":    "replaced with a root-relative placeholder",
	"work_dir":     "replaced with a root-relative placeholder",
	"tree_status":  "reduced to paths, dropping the diff body",
	"last_error":   "detail blanked",
	"stop_reason":  "blanked when it names a cmd",
	"node_outputs": "emptied for kinds the exporter does not know",
	"pins":         "from/to source fragments hashed; file and test name kept",
	"unproven":     "same Pin shape as pins: from/to source fragments hashed, file and test kept",
}

// verbatim are the fields deliberately exported as they stand. Each entry is a
// decision that the value carries no repository content or secret.
var verbatim = map[string]string{
	"schemaVersion":    "format version",
	"run_id":           "generated identifier",
	"parent_run_id":    "generated identifier",
	"lineage":          "generated identifiers",
	"forked_at_seq":    "counter",
	"created_at":       "timestamp",
	"seq":              "counter",
	"workflow":         "workflow name, not repository content",
	"workflow_hash":    "hash",
	"workflow_source":  "embedded or path-derived label",
	"calibration":      "flag",
	"mock":             "flag",
	"mock_tainted":     "flag",
	"repo_mode":        "enum",
	"cmds_sha256":      "hash",
	"state":            "workflow state name",
	"state_kind":       "node kind",
	"outcome":          "enum",
	"iteration":        "counter",
	"base_sha":         "commit id",
	"head":             "commit id",
	"fix_entry_head":   "commit id",
	"tree_hash":        "hash",
	"goldens":          "supplied by the operator, already their own data",
	"prev_fixed":       "a count of fixed bugs at the previous loop boundary; carries no content",
	"findings":         "review findings, the point of the bundle",
	"confirmed":        "review findings, the point of the bundle",
	"all_found":        "review findings, the point of the bundle",
	"status":           "per-finding status list",
	"unfixed":          "counter",
	"prev_unfixed":     "counter",
	"tokens":           "counters",
	"applied":          "node keys, no payload",
	"nodes_run":        "node names",
	"overflow_handled": "flag",
	"warnings":         "structured warnings, already redacted at the source",
}

func TestEverySnapshotFieldIsClassifiedForExport(t *testing.T) {
	var unclassified []string
	fields := reflect.VisibleFields(reflect.TypeOf(run.Snapshot{}))
	seen := map[string]bool{}
	for _, f := range fields {
		if !f.IsExported() {
			continue // unexported fields are not marshalled at all
		}
		tag := strings.Split(f.Tag.Get("json"), ",")[0]
		if tag == "-" {
			continue // explicitly excluded from the wire
		}
		if tag == "" {
			// An exported field with no json tag is still marshalled, under its
			// Go name. Skipping it here was the hole in this guard: precisely the
			// field nobody had thought about was the one it ignored. Classify it
			// under the name it will actually be serialised as.
			tag = f.Name
		}
		seen[tag] = true
		_, isTransformed := transformed[tag]
		_, isVerbatim := verbatim[tag]
		if isTransformed && isVerbatim {
			t.Fatalf("%s is listed as both transformed and verbatim", tag)
		}
		if !isTransformed && !isVerbatim {
			unclassified = append(unclassified, tag)
		}
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Fatalf("run.Snapshot fields not classified for export: %s\n"+
			"Add each to `transformed` (redactor.snapshot rewrites it) or to `verbatim` "+
			"(it is safe to export as-is), with the reason. A field in neither would be "+
			"exported unredacted without anyone deciding that it should be.",
			strings.Join(unclassified, ", "))
	}
	// The reverse direction: a set entry naming a field that no longer exists is
	// a stale decision, and would hide the next field that reuses the name.
	for _, set := range []map[string]string{transformed, verbatim} {
		for name := range set {
			if !seen[name] {
				t.Fatalf("%q is classified for export but is not a field of run.Snapshot", name)
			}
		}
	}
}

// A pin's from/to are literal source fragments — repository content by the same definition that
// makes tree_status get reduced to paths. The export keeps what identifies the proof (the file
// and the test name) and hashes the code itself, so a holder of the source can still confirm the
// export describes their tree without the export carrying that source.
func TestExportHashesPinSourceFragments(t *testing.T) {
	rd := &redactor{
		snap: run.Snapshot{
			RepoRoot: "/repo", WorkDir: "/repo",
			Pins: []run.Pin{{
				File: "internal/secret/algo.go",
				From: "if key == expected { return true }",
				To:   "if key != expected { return true }",
				Test: "TestKeyComparison",
			}},
			// Same Pin shape, carried across iterations rather than within one. It holds source
			// fragments for exactly the same reason and must be redacted the same way: a field
			// that merely inherits the treatment by resemblance is a field that exports the
			// repository's code in the clear the day someone forgets.
			Unproven: []run.Pin{{
				File: "internal/secret/algo.go",
				From: "for i := 0; i < len(secretTable); i++ {",
				To:   "for i := 0; i < 0; i++ {",
				Test: "TestTableWalk",
			}},
		},
		repoRoot: "/repo",
	}
	var m map[string]any
	if err := json.Unmarshal(rd.snapshot(), &m); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	for key, test := range map[string]string{"pins": "TestKeyComparison", "unproven": "TestTableWalk"} {
		pins, ok := m[key].([]any)
		if !ok || len(pins) != 1 {
			t.Fatalf("%s missing from the export: %v", key, m[key])
		}
		pin := pins[0].(map[string]any)
		if pin["file"] != "internal/secret/algo.go" || pin["test"] != test {
			t.Errorf("%s: the export must keep what identifies the proof: %v", key, pin)
		}
		for _, field := range []string{"from", "to"} {
			got, _ := pin[field].(string)
			for _, leak := range []string{"key", "expected", "secretTable"} {
				if strings.Contains(got, leak) {
					t.Errorf("%s.%s still carries source (%q): %q", key, field, leak, got)
				}
			}
			if !strings.HasPrefix(got, "sha256:") || len(got) != len("sha256:")+64 {
				t.Errorf("%s.%s = %q, want a sha256: prefixed digest", key, field, got)
			}
		}
		// Distinct fragments must hash distinctly, or the export cannot distinguish two pins.
		if pin["from"] == pin["to"] {
			t.Errorf("%s: from and to hashed to the same value", key)
		}
	}
}

// Two more places pin source reaches an export, neither covered by hashing the snapshot's copy.
//
// agent-edit is a knownKind, so its node_output is kept whole — and that output CARRIES the pins,
// with from/to as literal source. And delta_applied had no case in event() at all, which was
// harmless while a Delta held only findings and ids, and stopped being harmless the moment it
// gained pins and pin_results. Fixing the dropped-pins defect is what makes that line actually
// carry source, so the fix widened the leak rather than closing it.
func TestPinSourceIsRedactedOnEveryExportPath(t *testing.T) {
	const secret = "if key == expectedSecret { return true }"
	rd := &redactor{snap: run.Snapshot{RepoRoot: "/repo"}, repoRoot: "/repo", w: pinWorkflow(t)}

	nodeOut := run.Event{Type: run.TypeNodeOutput, Node: "fix", Data: json.RawMessage(
		`{"output":{"commit":"abc1234","pins":[{"file":"internal/secret/algo.go","from":` +
			mustJSON(secret) + `,"to":"if key != expectedSecret { return true }","test":"TestKey"}]}}`)}
	line, fields := rd.event(nodeOut)
	if line == nil {
		t.Fatal("agent-edit output carrying pins was exported unchanged")
	}
	if strings.Contains(string(line), "expectedSecret") {
		t.Errorf("node_output still carries source: %s", line)
	}
	if !strings.Contains(string(line), "sha256:") || len(fields) == 0 {
		t.Errorf("the fragments must be hashed and the redaction recorded: %s %v", line, fields)
	}
	// The path and test name stay: they identify what was proven without reproducing the code.
	if !strings.Contains(string(line), "internal/secret/algo.go") || !strings.Contains(string(line), "TestKey") {
		t.Errorf("the export lost what identifies the proof: %s", line)
	}

	delta := run.Event{Type: run.TypeDeltaApplied, Node: "prove", Data: json.RawMessage(
		`{"findings":[],"pins":[{"file":"a.go","from":` + mustJSON(secret) + `,"to":"y","test":"T"}],` +
			`"pin_results":[{"pin":{"file":"a.go","from":` + mustJSON(secret) + `,"to":"y","test":"T"},` +
			`"outcome":"survived","detail":"a.go: breaking ` + secret + ` left the tests passing"}],"output_hash":"h"}`)}
	line, fields = rd.event(delta)
	if line == nil {
		t.Fatal("a delta_applied event carrying pins was exported unchanged")
	}
	if strings.Contains(string(line), "expectedSecret") {
		t.Errorf("delta_applied still carries source: %s", line)
	}
	if len(fields) == 0 {
		t.Error("the redaction was not recorded")
	}
}

func mustJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// pinWorkflow is the smallest workflow with an agent-edit node named "fix", so kindOf resolves it
// to a known kind and the node_output branch keeps the output whole — the condition under which
// the leak existed.
func pinWorkflow(t *testing.T) *workflow.Workflow {
	t.Helper()
	src := []byte(`workflow: t
version: 1
vars: {}
states: [fix, done, failed]
transitions:
  - {from: fix, to: done, gate: commit_exists, outcome: fixed}
nodes:
  fix: {kind: agent-edit}
convergence:
  any: [{max_iterations: 1}]
`)
	kinds, err := kind.New(kind.Deps{})
	if err != nil {
		t.Fatal(err)
	}
	w, err := workflow.Parse(src, workflow.Options{Kinds: kinds.Info()})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return w
}
