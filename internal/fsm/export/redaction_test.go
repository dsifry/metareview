package export

import (
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
		tag := strings.Split(f.Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
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
