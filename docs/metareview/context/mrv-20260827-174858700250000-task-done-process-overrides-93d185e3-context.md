# metareview task-done context

Run ID: `mrv-20260827-174858700250000-task-done-process-overrides-93d185e3`

## Task

# Process overrides — recording out-of-workflow escalations

A blocking finding is normally cleared by fixing it. There was no way to record the other case: the
workflow being deliberately stepped outside of. In practice that case kept arising (a review chain
exhausting its attempts overnight, a branch structurally unable to pass a gate yet), and it was
improvised by hand-editing a review log's `## Verdict` line — untracked, unattributable, and
indistinguishable afterwards from a review that genuinely passed.

This adds the exception as a first-class, analysable object.

- `internal/findings`: statuses `override-pending` and `overridden`; `RequestOverride`, `GrantOverride`,
  `ListOverrides`, `PendingOverrides`; `Blocks(status)` as the single blocking rule (`open` and
  `override-pending` block; `overridden` does not). Provenance fields record actor, timestamp, reason and
  escalation context for both halves. An override is never a fix — `fixedInRunId` stays empty — so
  post-merge learning can separate exceptions from resolutions.
- Reconciliation: a recurring fingerprint reuses its overridden record instead of spawning a duplicate,
  so a persistent condition does not need re-overriding on every run.
- `internal/reviewlog` uses the same blocking rule.
- `docs/metareview/FINDINGS.md` gains a "Process Overrides" section; an override is never silent.
- `cmd/metareview`: `override request|grant|list [--pending]`. `--pending` exits 1 while any exception is
  unacknowledged, which is the CI hook.
- `CLAUDE.md`, `AGENTS.md`, CHANGELOG document the flow and the separation of requesting from granting.

The separation is the point: requesting is available to whoever drives the run, including an orchestrating
agent; granting must come from outside the workflow. A reviewing agent never clears its own findings.

Done when `go test ./...`, `tests/run-all.sh` (including `tests/go/test-override.sh`) and `go vet` pass.


## Git

- Base: `61ffdf722cca6df64bc57924922f43c0f2a0d7ef`
- Head: `7726c151d86f41b55a40ca7ec8b9c60bacdde186`
- Branch: `override-findings`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `34461`
- Filtered diff bytes: `34461`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `97609b6bf2e5ea79`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- AGENTS.md
- CHANGELOG.md
- CLAUDE.md
- cmd/metareview/main.go
- docs/tasks/process-overrides.md
- internal/findings/findings.go
- internal/findings/override.go
- internal/findings/override_test.go
- internal/reviewlog/reviewlog.go
- tests/go/test-override.sh
- tests/run-all.sh

### Shards
- shard-01: AGENTS.md, CHANGELOG.md, CLAUDE.md, cmd/metareview/main.go, docs/tasks/process-overrides.md, internal/findings/findings.go, internal/findings/override.go, internal/findings/override_test.go, internal/reviewlog/reviewlog.go, tests/go/test-override.sh, tests/run-all.sh

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- AGENTS.md
- CHANGELOG.md
- CLAUDE.md
- cmd/metareview/main.go
- docs/tasks/process-overrides.md
- internal/findings/findings.go
- internal/findings/override.go
- internal/findings/override_test.go
- internal/reviewlog/reviewlog.go
- tests/go/test-override.sh
- tests/run-all.sh

## Diff

````diff
diff --git a/AGENTS.md b/AGENTS.md
index b4fa7ce..ebde2e4 100644
--- a/AGENTS.md
+++ b/AGENTS.md
@@ -29,6 +29,20 @@ Lifecycle gate verdicts have this contract:
 
 Advisory notes can be recorded for later, but blockers are current work.
 
+When a blocker cannot be fixed and the workflow is deliberately stepped outside of, record the exception
+instead of working around it:
+
+- `metareview override request <finding-id> --reason "<text>" [--escalation "<text>"]` — available to
+  whoever drives the run, including an orchestrating agent. It does **not** clear the gate: the finding
+  keeps blocking and `metareview override list --pending` exits nonzero, so CI stays red.
+- `metareview override grant <finding-id> --reason "<text>"` — the acknowledgement, and it must come from
+  outside the workflow (a human, or an authority explicitly designated as one). A reviewing agent never
+  grants an override on findings it produced.
+
+Both halves record actor, timestamp and reason and are rendered under "Process Overrides" in
+`docs/metareview/FINDINGS.md`. An override is never a fix: `fixedInRunId` stays empty, so exceptions can be
+analysed separately from resolutions.
+
 ## Evidence Policy
 
 For task-done and pr-ready reviews, provide evidence:
diff --git a/CHANGELOG.md b/CHANGELOG.md
index bf97bfd..f198220 100644
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -1,5 +1,17 @@
 # Changelog
 
+## Unreleased
+
+### Added
+
+- **Process overrides.** `metareview override request|grant|list` records that the review workflow was
+  deliberately stepped outside of. Requesting is available to whoever drives the run (an orchestrating
+  agent included) and does not clear the gate — the finding keeps blocking and `override list --pending`
+  exits nonzero, so CI stays red until an authority outside the workflow grants it. Both halves record
+  actor, timestamp and reason; overrides render under "Process Overrides" in `docs/metareview/FINDINGS.md`
+  and never read as fixes (`fixedInRunId` stays empty), so exceptions can be analysed separately from
+  resolutions.
+
 ## 0.8.2 - 2026-08-26
 
 0.8.2 adds **orchestrator discipline** guidance to the review-artifact skill: the orchestrator
diff --git a/CLAUDE.md b/CLAUDE.md
index 9fea063..b5e914e 100644
--- a/CLAUDE.md
+++ b/CLAUDE.md
@@ -29,6 +29,26 @@ Before saying work is done, run the appropriate metareview gate.
 
 Exit handling: `0` means verify `PASS`/`PASS_ADVISORY` with zero blockers; `1` with a review path means follow that log; nonzero without a path means read stderr.
 
+## Process Overrides
+
+A blocking finding is normally cleared by fixing it. When that is not possible and the workflow is
+deliberately stepped outside of — an escalation — record it rather than working around it:
+
+```bash
+metareview override request <finding-id> --reason "<why the workflow was exited>" [--escalation "<context>"]
+metareview override grant   <finding-id> --reason "<why the exception is accepted>"
+metareview override list [--pending]
+```
+
+- **Requesting** is available to whoever is driving the run, including an orchestrating agent. It does
+  **not** clear the gate: the finding keeps blocking and `override list --pending` exits nonzero, so CI
+  stays red.
+- **Granting** is the acknowledgement, and must come from outside the workflow — a human, or an authority
+  explicitly designated as such. A reviewing agent never grants an override on its own findings.
+- Both halves are recorded with actor, timestamp and reason, rendered under "Process Overrides" in
+  `docs/metareview/FINDINGS.md`, and an override is never a fix (`fixedInRunId` stays empty), so post-merge
+  learning can analyse exceptions separately from resolutions.
+
 ## Lifecycle Placement
 
 - Before implementing a plan or spec: review the artifact.
diff --git a/cmd/metareview/main.go b/cmd/metareview/main.go
index a55c0e3..4232266 100644
--- a/cmd/metareview/main.go
+++ b/cmd/metareview/main.go
@@ -6,6 +6,7 @@ import (
 	"errors"
 	"fmt"
 	"os"
+	"os/exec"
 	"strconv"
 	"strings"
 	"time"
@@ -14,6 +15,7 @@ import (
 	"github.com/dsifry/metareview/internal/contextpack"
 	"github.com/dsifry/metareview/internal/epicready"
 	"github.com/dsifry/metareview/internal/evidence"
+	"github.com/dsifry/metareview/internal/findings"
 	"github.com/dsifry/metareview/internal/gitcontext"
 	"github.com/dsifry/metareview/internal/learning"
 	"github.com/dsifry/metareview/internal/prready"
@@ -30,6 +32,9 @@ Usage:
   metareview setup --check
   metareview setup --bootstrap-prereqs --dry-run
   metareview status
+  metareview override request <finding-id> --reason "<text>" [--by <who>] [--escalation "<text>"]
+  metareview override grant <finding-id> --reason "<text>" [--by <who>]
+  metareview override list [--pending]
   metareview context build <path>
   metareview context diff [--base <ref>]
   metareview evidence run -- <command> [args...]
@@ -44,6 +49,9 @@ Commands:
   setup --check              Detect repository mode and prerequisites without writing files
   setup --bootstrap-prereqs  Print or execute prerequisite bootstrap actions
   status                     Print repository review capability status
+  override request           Record an out-of-workflow escalation against a finding (still blocks)
+  override grant             Acknowledge a process exception from outside the workflow (stops blocking)
+  override list              List process exceptions; --pending exits 1 while any are unacknowledged
   context build <path>       Build a Markdown context pack for an artifact
   context diff               Print git diff context as JSON
   evidence run               Run a command and print a structured JSON receipt
@@ -94,6 +102,11 @@ func main() {
 		return
 	}
 
+	if args[0] == "override" {
+		handleOverride(args[1:])
+		return
+	}
+
 	if len(args) == 3 && args[0] == "context" && args[1] == "build" {
 		result, err := contextpack.Build(mustCwd(), args[2], time.Now())
 		exitOnErr(err)
@@ -451,3 +464,110 @@ func present(value bool) string {
 	}
 	return "missing"
 }
+
+// handleOverride implements the process-exception commands. An override records
+// that the workflow was deliberately stepped outside of: requesting is available
+// to whoever is driving the run, granting is the acknowledgement from outside it,
+// and CI stays red while any request is unacknowledged.
+func handleOverride(args []string) {
+	if len(args) == 0 {
+		fmt.Fprintln(os.Stderr, "Usage: metareview override request|grant|list")
+		os.Exit(2)
+	}
+	root := mustCwd()
+	switch args[0] {
+	case "list":
+		pendingOnly := false
+		for _, arg := range args[1:] {
+			if arg == "--pending" {
+				pendingOnly = true
+			}
+		}
+		records, err := findings.ListOverrides(root)
+		exitOnErr(err)
+		pending := 0
+		for _, record := range records {
+			if record.Status == findings.StatusOverridePending {
+				pending++
+			}
+			if pendingOnly && record.Status != findings.StatusOverridePending {
+				continue
+			}
+			printOverride(record)
+		}
+		if len(records) == 0 {
+			fmt.Println("no process overrides recorded")
+		}
+		if pendingOnly && pending > 0 {
+			fmt.Fprintf(os.Stderr, "%d override(s) awaiting acknowledgement\n", pending)
+			os.Exit(1)
+		}
+	case "request", "grant":
+		if len(args) < 2 {
+			fmt.Fprintf(os.Stderr, "Usage: metareview override %s <finding-id> --reason \"<text>\"\n", args[0])
+			os.Exit(2)
+		}
+		id := args[1]
+		reason, by, escalation := "", "", ""
+		for i := 2; i < len(args); i++ {
+			switch args[i] {
+			case "--reason":
+				if i+1 < len(args) {
+					reason = args[i+1]
+					i++
+				}
+			case "--by":
+				if i+1 < len(args) {
+					by = args[i+1]
+					i++
+				}
+			case "--escalation":
+				if i+1 < len(args) {
+					escalation = args[i+1]
+					i++
+				}
+			default:
+				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", args[i])
+				os.Exit(2)
+			}
+		}
+		if by == "" {
+			by = defaultActor()
+		}
+		now := time.Now().UTC().Format(time.RFC3339)
+		if args[0] == "request" {
+			exitOnErr(findings.RequestOverride(root, id, findings.OverrideRequest{
+				By: by, Reason: reason, Escalation: escalation, Now: now,
+			}))
+			fmt.Printf("%s: override requested by %s (still blocking until granted)\n", id, by)
+			return
+		}
+		exitOnErr(findings.GrantOverride(root, id, findings.OverrideGrant{By: by, Reason: reason, Now: now}))
+		fmt.Printf("%s: override granted by %s\n", id, by)
+	default:
+		fmt.Fprintln(os.Stderr, "Usage: metareview override request|grant|list")
+		os.Exit(2)
+	}
+}
+
+func printOverride(record findings.Record) {
+	switch record.Status {
+	case findings.StatusOverridePending:
+		fmt.Printf("%s  pending  %s\n    requested by %s at %s: %s\n",
+			record.ID, record.Title, record.OverrideRequestedBy, record.OverrideRequestedAt, record.OverrideRequestReason)
+	default:
+		fmt.Printf("%s  granted  %s\n    granted by %s at %s: %s\n",
+			record.ID, record.Title, record.OverrideGrantedBy, record.OverrideGrantedAt, record.OverrideGrantReason)
+	}
+}
+
+// defaultActor identifies who is acting, from git config.
+func defaultActor() string {
+	cmd := exec.Command("git", "config", "user.email")
+	cmd.Dir = mustCwd()
+	out, err := cmd.Output()
+	if err != nil {
+		return ""
+	}
+	return strings.TrimSpace(string(out))
+}
diff --git a/docs/tasks/process-overrides.md b/docs/tasks/process-overrides.md
new file mode 100644
index 0000000..a4d951a
--- /dev/null
+++ b/docs/tasks/process-overrides.md
@@ -0,0 +1,27 @@
+# Process overrides — recording out-of-workflow escalations
+
+A blocking finding is normally cleared by fixing it. There was no way to record the other case: the
+workflow being deliberately stepped outside of. In practice that case kept arising (a review chain
+exhausting its attempts overnight, a branch structurally unable to pass a gate yet), and it was
+improvised by hand-editing a review log's `## Verdict` line — untracked, unattributable, and
+indistinguishable afterwards from a review that genuinely passed.
+
+This adds the exception as a first-class, analysable object.
+
+- `internal/findings`: statuses `override-pending` and `overridden`; `RequestOverride`, `GrantOverride`,
+  `ListOverrides`, `PendingOverrides`; `Blocks(status)` as the single blocking rule (`open` and
+  `override-pending` block; `overridden` does not). Provenance fields record actor, timestamp, reason and
+  escalation context for both halves. An override is never a fix — `fixedInRunId` stays empty — so
+  post-merge learning can separate exceptions from resolutions.
+- Reconciliation: a recurring fingerprint reuses its overridden record instead of spawning a duplicate,
+  so a persistent condition does not need re-overriding on every run.
+- `internal/reviewlog` uses the same blocking rule.
+- `docs/metareview/FINDINGS.md` gains a "Process Overrides" section; an override is never silent.
+- `cmd/metareview`: `override request|grant|list [--pending]`. `--pending` exits 1 while any exception is
+  unacknowledged, which is the CI hook.
+- `CLAUDE.md`, `AGENTS.md`, CHANGELOG document the flow and the separation of requesting from granting.
+
+The separation is the point: requesting is available to whoever drives the run, including an orchestrating
+agent; granting must come from outside the workflow. A reviewing agent never clears its own findings.
+
+Done when `go test ./...`, `tests/run-all.sh` (including `tests/go/test-override.sh`) and `go vet` pass.
diff --git a/internal/findings/findings.go b/internal/findings/findings.go
index 89a5c35..17c3183 100644
--- a/internal/findings/findings.go
+++ b/internal/findings/findings.go
@@ -68,10 +68,20 @@ type Record struct {
 	Fingerprint        string     `json:"fingerprint"`
 	Target             any        `json:"target"`
 	FixedInRunID       string     `json:"fixedInRunId,omitempty"`
-	CreatedAt          string     `json:"createdAt"`
-	UpdatedAt          string     `json:"updatedAt"`
-	RepoRoot           string     `json:"repoRoot"`
-	GitHead            string     `json:"gitHead"`
+
+	// Process-exception provenance (see override.go). An override is never a fix:
+	// FixedInRunID stays empty.
+	OverrideRequestedBy   string `json:"overrideRequestedBy,omitempty"`
+	OverrideRequestedAt   string `json:"overrideRequestedAt,omitempty"`
+	OverrideRequestReason string `json:"overrideRequestReason,omitempty"`
+	OverrideEscalation    string `json:"overrideEscalation,omitempty"`
+	OverrideGrantedBy     string `json:"overrideGrantedBy,omitempty"`
+	OverrideGrantedAt     string `json:"overrideGrantedAt,omitempty"`
+	OverrideGrantReason   string `json:"overrideGrantReason,omitempty"`
+	CreatedAt             string `json:"createdAt"`
+	UpdatedAt             string `json:"updatedAt"`
+	RepoRoot              string `json:"repoRoot"`
+	GitHead               string `json:"gitHead"`
 }
 
 type Result struct {
@@ -121,7 +131,7 @@ func Reconcile(root string, run Run, current []Input, options Options) (Result,
 
 	activeExisting := map[string]bool{}
 	for _, record := range updated {
-		if record.Status == "open" && record.Fingerprint != "" && sameRunTarget(record, run) {
+		if record.Status != "fixed" && record.Fingerprint != "" && sameRunTarget(record, run) {
 			activeExisting[record.Fingerprint] = true
 		}
 	}
@@ -215,7 +225,13 @@ func RenderIndexWithRecords(root string, records []Record) error {
 		}
 		body = strings.Join(lines, "\n")
 	}
-	return os.WriteFile(path, []byte("# metareview Findings\n\n"+body+"\n"), 0o644)
+	document := "# metareview Findings\n\n" + body + "\n"
+	if overrides := overrideLines(records); len(overrides) > 0 {
+		document += "\n## Process Overrides\n\n" +
+			"Deliberate exceptions to the review workflow. Pending entries still block CI.\n\n" +
+			strings.Join(overrides, "\n") + "\n"
+	}
+	return os.WriteFile(path, []byte(document), 0o644)
 }
 
 func UnresolvedBlocking(root string) ([]Record, error) {
@@ -261,7 +277,7 @@ func normalize(run Run, finding Input, index int, createdAt string) Record {
 func unresolvedBlockingFrom(records []Record) []Record {
 	blockers := make([]Record, 0)
 	for _, record := range records {
-		if record.Status != "open" {
+		if !Blocks(record.Status) {
 			continue
 		}
 		if classForCount(record.Classification, record.Severity) == "blocking" {
@@ -334,7 +350,7 @@ func classForCount(classification, severity string) string {
 func openForRun(records []Record, run Run) []Record {
 	open := make([]Record, 0, len(records))
 	for _, record := range records {
-		if record.Status == "open" && sameRunTarget(record, run) {
+		if Blocks(record.Status) && sameRunTarget(record, run) {
 			open = append(open, record)
 		}
 	}
diff --git a/internal/findings/override.go b/internal/findings/override.go
new file mode 100644
index 0000000..3f1299d
--- /dev/null
+++ b/internal/findings/override.go
@@ -0,0 +1,164 @@
+package findings
+
+import (
+	"fmt"
+	"sort"
+	"strings"
+)
+
+// Override statuses. A finding is either open, fixed, or has left the normal
+// workflow: override-pending records that an out-of-workflow escalation happened
+// and still blocks; overridden records that an authority outside the workflow
+// acknowledged it and stops blocking.
+const (
+	StatusOverridePending = "override-pending"
+	StatusOverridden      = "overridden"
+)
+
+// minOverrideReason is the shortest reason worth recording. An override that
+// cannot be explained in a sentence is not an override, it is a shrug.
+const minOverrideReason = 12
+
+// OverrideRequest is filed by whoever stepped outside the workflow — typically
+// the orchestrating agent, which may request but never grant.
+type OverrideRequest struct {
+	By         string
+	Reason     string
+	Escalation string
+	Now        string
+}
+
+// OverrideGrant is the acknowledgement from outside the workflow.
+type OverrideGrant struct {
+	By     string
+	Reason string
+	Now    string
+}
+
+// Blocks reports whether a status still holds a gate closed. A pending override
+// blocks by design: the workflow was stepped outside of and nobody has
+// acknowledged it yet.
+func Blocks(status string) bool {
+	return status == "open" || status == StatusOverridePending
+}
+
+// RequestOverride marks a finding as a recorded, unacknowledged process exception.
+func RequestOverride(root, findingID string, request OverrideRequest) error {
+	if strings.TrimSpace(request.By) == "" {
+		return fmt.Errorf("override request needs an actor (--by)")
+	}
+	if len(strings.TrimSpace(request.Reason)) < minOverrideReason {
+		return fmt.Errorf("override request needs a reason of at least %d characters", minOverrideReason)
+	}
+	return mutateFinding(root, findingID, func(record *Record) error {
+		if record.Status != "open" {
+			return fmt.Errorf("finding %s is %s, not open", findingID, record.Status)
+		}
+		record.Status = StatusOverridePending
+		record.OverrideRequestedBy = strings.TrimSpace(request.By)
+		record.OverrideRequestReason = strings.TrimSpace(request.Reason)
+		record.OverrideRequestedAt = request.Now
+		record.OverrideEscalation = strings.TrimSpace(request.Escalation)
+		record.UpdatedAt = request.Now
+		return nil
+	})
+}
+
+// GrantOverride acknowledges the exception. It accepts an open finding directly
+// (a human overriding without a prior agent escalation) or a pending request.
+func GrantOverride(root, findingID string, grant OverrideGrant) error {
+	if strings.TrimSpace(grant.By) == "" {
+		return fmt.Errorf("override grant needs an actor (--by)")
+	}
+	if len(strings.TrimSpace(grant.Reason)) < minOverrideReason {
+		return fmt.Errorf("override grant needs a reason of at least %d characters", minOverrideReason)
+	}
+	return mutateFinding(root, findingID, func(record *Record) error {
+		if record.Status != "open" && record.Status != StatusOverridePending {
+			return fmt.Errorf("finding %s is %s and cannot be overridden", findingID, record.Status)
+		}
+		record.Status = StatusOverridden
+		record.OverrideGrantedBy = strings.TrimSpace(grant.By)
+		record.OverrideGrantReason = strings.TrimSpace(grant.Reason)
+		record.OverrideGrantedAt = grant.Now
+		record.UpdatedAt = grant.Now
+		return nil
+	})
+}
+
+// ListOverrides returns every requested or granted override, newest first.
+func ListOverrides(root string) ([]Record, error) {
+	records, err := readJSONL(findingsPath(root))
+	if err != nil {
+		return nil, err
+	}
+	out := make([]Record, 0)
+	for _, record := range records {
+		if record.Status == StatusOverridePending || record.Status == StatusOverridden {
+			out = append(out, record)
+		}
+	}
+	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
+	return out, nil
+}
+
+// PendingOverrides returns the exceptions nobody has acknowledged yet. CI should
+// refuse to pass while this is non-empty.
+func PendingOverrides(root string) ([]Record, error) {
+	all, err := ListOverrides(root)
+	if err != nil {
+		return nil, err
+	}
+	pending := make([]Record, 0, len(all))
+	for _, record := range all {
+		if record.Status == StatusOverridePending {
+			pending = append(pending, record)
+		}
+	}
+	return pending, nil
+}
+
+func mutateFinding(root, findingID string, apply func(*Record) error) error {
+	path := findingsPath(root)
+	records, err := readJSONL(path)
+	if err != nil {
+		return err
+	}
+	index := -1
+	for i, record := range records {
+		if record.ID == findingID {
+			index = i
+			break
+		}
+	}
+	if index < 0 {
+		return fmt.Errorf("finding %s not found", findingID)
+	}
+	if err := apply(&records[index]); err != nil {
+		return err
+	}
+	if err := writeJSONL(path, records); err != nil {
+		return err
+	}
+	return RenderIndexWithRecords(root, records)
+}
+
+// overrideLines renders the process-exception section of the findings index.
+func overrideLines(records []Record) []string {
+	var lines []string
+	for _, record := range records {
+		switch record.Status {
+		case StatusOverridePending:
+			lines = append(lines, fmt.Sprintf("- %s [pending] %s — requested by %s: %s",
+				record.ID, record.Title, record.OverrideRequestedBy, record.OverrideRequestReason))
+		case StatusOverridden:
+			detail := fmt.Sprintf("- %s [granted] %s — granted by %s: %s",
+				record.ID, record.Title, record.OverrideGrantedBy, record.OverrideGrantReason)
+			if record.OverrideRequestedBy != "" {
+				detail += fmt.Sprintf(" (requested by %s: %s)", record.OverrideRequestedBy, record.OverrideRequestReason)
+			}
+			lines = append(lines, detail)
+		}
+	}
+	return lines
+}
diff --git a/internal/findings/override_test.go b/internal/findings/override_test.go
new file mode 100644
index 0000000..4925cbe
--- /dev/null
+++ b/internal/findings/override_test.go
@@ -0,0 +1,247 @@
+package findings
+
+import (
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+)
+
+func seedRecord(t *testing.T, root string, record Record) {
+	t.Helper()
+	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
+		t.Fatal(err)
+	}
+	if err := writeJSONL(findingsPath(root), []Record{record}); err != nil {
+		t.Fatal(err)
+	}
+}
+
+func openBlocker(id string) Record {
+	return Record{
+		SchemaVersion:  1,
+		ID:             id,
+		RunID:          "mrv-run-1",
+		Scope:          "pr-ready",
+		Reviewer:       "architecture-reviewer",
+		Severity:       "high",
+		Classification: "blocking",
+		Status:         "open",
+		Title:          "Review context risk",
+		Fingerprint:    "pr:architecture:context-risk",
+		Target:         map[string]string{"type": "branch", "id": "feature"},
+		CreatedAt:      "2026-08-27T00:00:00Z",
+		UpdatedAt:      "2026-08-27T00:00:00Z",
+	}
+}
+
+func loadOne(t *testing.T, root string) Record {
+	t.Helper()
+	records, err := readJSONL(findingsPath(root))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(records) != 1 {
+		t.Fatalf("want exactly one record, got %d", len(records))
+	}
+	return records[0]
+}
+
+func TestRequestOverrideMarksPendingAndKeepsBlocking(t *testing.T) {
+	root := t.TempDir()
+	seedRecord(t, root, openBlocker("mrvf-1"))
+
+	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
+		By:         "orchestrator",
+		Reason:     "chain exhausted at three attempts; blockers closed in the final revision",
+		Escalation: "artifact review chain exhausted (attempt 3 of 3)",
+		Now:        "2026-08-27T01:00:00Z",
+	}); err != nil {
+		t.Fatal(err)
+	}
+	record := loadOne(t, root)
+	if record.Status != StatusOverridePending {
+		t.Fatalf("status = %q, want %q", record.Status, StatusOverridePending)
+	}
+	if record.OverrideRequestedBy != "orchestrator" || record.OverrideRequestReason == "" || record.OverrideRequestedAt == "" {
+		t.Fatalf("request provenance is incomplete: %+v", record)
+	}
+	if record.OverrideEscalation == "" {
+		t.Fatal("the escalation context must be recorded")
+	}
+	// A pending override still blocks: CI must stay red until it is granted.
+	if len(unresolvedBlockingFrom([]Record{record})) != 1 {
+		t.Fatal("a pending override must still count as a blocker")
+	}
+}
+
+func TestGrantOverrideClearsBlockingWithAttribution(t *testing.T) {
+	root := t.TempDir()
+	seedRecord(t, root, openBlocker("mrvf-1"))
+	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
+		By: "orchestrator", Reason: "three lens passes were enough evidence to proceed", Now: "2026-08-27T01:00:00Z",
+	}); err != nil {
+		t.Fatal(err)
+	}
+	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
+		By: "dsifry@warmstart.ai", Reason: "reviewed the evidence and accept the exception", Now: "2026-08-27T02:00:00Z",
+	}); err != nil {
+		t.Fatal(err)
+	}
+	record := loadOne(t, root)
+	if record.Status != StatusOverridden {
+		t.Fatalf("status = %q, want %q", record.Status, StatusOverridden)
+	}
+	if record.OverrideGrantedBy != "dsifry@warmstart.ai" || record.OverrideGrantReason == "" || record.OverrideGrantedAt == "" {
+		t.Fatalf("grant provenance is incomplete: %+v", record)
+	}
+	if record.OverrideRequestedBy != "orchestrator" {
+		t.Fatal("granting must not erase who requested the override")
+	}
+	if len(unresolvedBlockingFrom([]Record{record})) != 0 {
+		t.Fatal("a granted override must stop blocking")
+	}
+	if record.FixedInRunID != "" {
+		t.Fatal("an override is not a fix; fixedInRunId must stay empty")
+	}
+}
+
+func TestGrantWithoutRequestIsAllowed(t *testing.T) {
+	root := t.TempDir()
+	seedRecord(t, root, openBlocker("mrvf-1"))
+	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
+		By: "dsifry@warmstart.ai", Reason: "accepted directly; no agent escalation preceded it", Now: "2026-08-27T02:00:00Z",
+	}); err != nil {
+		t.Fatal(err)
+	}
+	if loadOne(t, root).Status != StatusOverridden {
+		t.Fatal("a human may override an open finding directly")
+	}
+}
+
+func TestOverrideRequiresSubstantiveReasonAndActor(t *testing.T) {
+	root := t.TempDir()
+	seedRecord(t, root, openBlocker("mrvf-1"))
+	if err := RequestOverride(root, "mrvf-1", OverrideRequest{By: "orchestrator", Reason: "because", Now: "t"}); err == nil {
+		t.Fatal("a trivial reason must be refused")
+	}
+	if err := RequestOverride(root, "mrvf-1", OverrideRequest{Reason: "a perfectly good explanation of the exception", Now: "t"}); err == nil {
+		t.Fatal("an override with no actor must be refused")
+	}
+	if err := GrantOverride(root, "mrvf-1", OverrideGrant{By: "someone", Reason: "ok", Now: "t"}); err == nil {
+		t.Fatal("a trivial grant reason must be refused")
+	}
+	if loadOne(t, root).Status != "open" {
+		t.Fatal("a refused override must not change the record")
+	}
+}
+
+func TestOverrideUnknownOrClosedFinding(t *testing.T) {
+	root := t.TempDir()
+	seedRecord(t, root, openBlocker("mrvf-1"))
+	if err := RequestOverride(root, "missing", OverrideRequest{By: "a", Reason: "a good reason for the exception", Now: "t"}); err == nil {
+		t.Fatal("an unknown finding id must be refused")
+	}
+	fixed := openBlocker("mrvf-2")
+	fixed.Status = "fixed"
+	if err := writeJSONL(findingsPath(root), []Record{fixed}); err != nil {
+		t.Fatal(err)
+	}
+	if err := RequestOverride(root, "mrvf-2", OverrideRequest{By: "a", Reason: "a good reason for the exception", Now: "t"}); err == nil {
+		t.Fatal("overriding an already-closed finding must be refused")
+	}
+}
+
+// A recurring condition must not spawn a duplicate record beside its override,
+// and must not silently reopen.
+func TestReconcileKeepsOverrideWhenTheFindingRecurs(t *testing.T) {
+	root := t.TempDir()
+	seedRecord(t, root, openBlocker("mrvf-1"))
+	if err := GrantOverride(root, "mrvf-1", OverrideGrant{
+		By: "dsifry@warmstart.ai", Reason: "accepted while the underlying condition persists", Now: "2026-08-27T02:00:00Z",
+	}); err != nil {
+		t.Fatal(err)
+	}
+	run := Run{ID: "mrv-run-2", Scope: "pr-ready", Target: map[string]string{"type": "branch", "id": "feature"}, RepoRoot: root}
+	same := Input{
+		Reviewer: "architecture-reviewer", Severity: "high", Classification: "blocking",
+		Title: "Review context risk", Fingerprint: "pr:architecture:context-risk",
+	}
+	if _, err := Reconcile(root, run, []Input{same}, Options{}); err != nil {
+		t.Fatal(err)
+	}
+	records, err := readJSONL(findingsPath(root))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(records) != 1 {
+		t.Fatalf("the recurring finding must reuse its overridden record, got %d records", len(records))
+	}
+	if records[0].Status != StatusOverridden {
+		t.Fatalf("status = %q, want the override to persist", records[0].Status)
+	}
+}
+
+func TestOverridesAreRenderedInTheIndex(t *testing.T) {
+	root := t.TempDir()
+	seedRecord(t, root, openBlocker("mrvf-1"))
+	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
+		By: "orchestrator", Reason: "the reviewer was asleep and the evidence was sufficient", Now: "2026-08-27T01:00:00Z",
+	}); err != nil {
+		t.Fatal(err)
+	}
+	records, err := readJSONL(findingsPath(root))
+	if err != nil {
+		t.Fatal(err)
+	}
+	if err := RenderIndexWithRecords(root, records); err != nil {
+		t.Fatal(err)
+	}
+	body, err := os.ReadFile(filepath.Join(root, "docs", "metareview", "FINDINGS.md"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	text := string(body)
+	if !strings.Contains(text, "Process Overrides") {
+		t.Fatalf("FINDINGS.md must surface overrides:\n%s", text)
+	}
+	for _, want := range []string{"pending", "orchestrator", "the reviewer was asleep"} {
+		if !strings.Contains(text, want) {
+			t.Fatalf("FINDINGS.md is missing %q:\n%s", want, text)
+		}
+	}
+}
+
+func TestListOverrides(t *testing.T) {
+	root := t.TempDir()
+	seedRecord(t, root, openBlocker("mrvf-1"))
+	if err := RequestOverride(root, "mrvf-1", OverrideRequest{
+		By: "orchestrator", Reason: "an explanation long enough to be meaningful", Now: "t",
+	}); err != nil {
+		t.Fatal(err)
+	}
+	all, err := ListOverrides(root)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(all) != 1 || all[0].Status != StatusOverridePending {
+		t.Fatalf("ListOverrides = %+v", all)
+	}
+	pending, err := PendingOverrides(root)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(pending) != 1 {
+		t.Fatalf("PendingOverrides = %+v", pending)
+	}
+	if err := GrantOverride(root, "mrvf-1", OverrideGrant{By: "human", Reason: "acknowledged the exception", Now: "t"}); err != nil {
+		t.Fatal(err)
+	}
+	pending, err = PendingOverrides(root)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(pending) != 0 {
+		t.Fatalf("a granted override must not stay pending: %+v", pending)
+	}
+}
diff --git a/internal/reviewlog/reviewlog.go b/internal/reviewlog/reviewlog.go
index 760f44a..7beeb1e 100644
--- a/internal/reviewlog/reviewlog.go
+++ b/internal/reviewlog/reviewlog.go
@@ -10,6 +10,7 @@ import (
 	"sort"
 	"strings"
 
+	"github.com/dsifry/metareview/internal/findings"
 	"github.com/dsifry/metareview/internal/runchain"
 )
 
@@ -293,7 +294,8 @@ func readFindings(root string) ([]findingRecord, error) {
 }
 
 func isOpenBlocker(record findingRecord) bool {
-	if record.Status != "open" {
+	// A pending override still blocks; a granted one does not.
+	if !findings.Blocks(record.Status) {
 		return false
 	}
 	if record.Classification == "spec-contract" {
diff --git a/tests/go/test-override.sh b/tests/go/test-override.sh
new file mode 100755
index 0000000..bfae786
--- /dev/null
+++ b/tests/go/test-override.sh
@@ -0,0 +1,61 @@
+#!/usr/bin/env bash
+# Process overrides: an agent may record an out-of-workflow escalation, only an
+# authority outside the workflow may acknowledge it, and CI stays red until then.
+set -euo pipefail
+
+ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
+TMP="$(mktemp -d)"
+trap 'rm -rf "$TMP"' EXIT
+
+(cd "$ROOT" && go build -o "$TMP/metareview" ./cmd/metareview)
+
+REPO="$TMP/repo"
+mkdir -p "$REPO/.metareview"
+cd "$REPO"
+git init -q -b main
+git config user.email tester@example.com
+git config user.name "Test User"
+
+cat > .metareview/findings.jsonl <<'EOF'
+{"schemaVersion":1,"id":"mrvf-1","runId":"mrv-1","scope":"pr-ready","reviewer":"architecture-reviewer","severity":"high","classification":"blocking","status":"open","title":"Review context risk","finding":"","expected":"","found":"","evidence":[],"recommendation":"","owner":"","knowledgeCandidate":false,"beadsFollowupId":null,"fingerprint":"pr:architecture:context-risk","target":{"type":"branch","id":"feature"},"createdAt":"2026-08-27T00:00:00Z","updatedAt":"2026-08-27T00:00:00Z"}
+EOF
+
+fail() { echo "FAIL: $1" >&2; exit 1; }
+
+# A reason that explains nothing is refused, and the record is untouched.
+if "$TMP/metareview" override request mrvf-1 --by orchestrator --reason "nope" >/dev/null 2>&1; then
+  fail "a trivial reason must be refused"
+fi
+grep -q '"status":"open"' .metareview/findings.jsonl || fail "a refused override changed the record"
+
+# The orchestrator may request; the finding still blocks.
+"$TMP/metareview" override request mrvf-1 --by orchestrator \
+  --reason "chain exhausted overnight; three lens passes were sufficient evidence" \
+  --escalation "artifact review chain exhausted (attempt 3 of 3)" >/dev/null
+grep -q '"status":"override-pending"' .metareview/findings.jsonl || fail "request did not mark the finding pending"
+grep -q '"overrideEscalation"' .metareview/findings.jsonl || fail "the escalation context was not recorded"
+
+# CI hook: pending overrides keep the pipeline red.
+if "$TMP/metareview" override list --pending >/dev/null 2>&1; then
+  fail "override list --pending must exit nonzero while an override is unacknowledged"
+fi
+
+# Acknowledgement from outside the workflow clears it, with attribution.
+"$TMP/metareview" override grant mrvf-1 --reason "reviewed the evidence and accept the exception" >/dev/null
+grep -q '"status":"overridden"' .metareview/findings.jsonl || fail "grant did not mark the finding overridden"
+grep -q '"overrideGrantedBy":"tester@example.com"' .metareview/findings.jsonl || fail "the granting actor was not recorded"
+grep -q '"fixedInRunId"' .metareview/findings.jsonl && fail "an override must never read as a fix"
+
+"$TMP/metareview" override list --pending >/dev/null || fail "a granted override must not stay pending"
+
+# The exception is visible, with both halves of its provenance.
+grep -q "## Process Overrides" docs/metareview/FINDINGS.md || fail "FINDINGS.md does not surface overrides"
+grep -q "granted by tester@example.com" docs/metareview/FINDINGS.md || fail "FINDINGS.md omits the granting actor"
+grep -q "requested by orchestrator" docs/metareview/FINDINGS.md || fail "FINDINGS.md omits the requesting actor"
+
+# An already-overridden finding cannot be re-overridden.
+if "$TMP/metareview" override request mrvf-1 --by orchestrator --reason "trying to override this a second time" >/dev/null 2>&1; then
+  fail "an overridden finding must not accept a new request"
+fi
+
+echo "test-override: ok"
diff --git a/tests/run-all.sh b/tests/run-all.sh
index d23c251..4423464 100755
--- a/tests/run-all.sh
+++ b/tests/run-all.sh
@@ -16,6 +16,7 @@ if [ -f tests/go/test-git-context.sh ]; then bash tests/go/test-git-context.sh;
 if [ -f tests/go/test-task-source.sh ]; then bash tests/go/test-task-source.sh; fi
 if [ -f tests/go/test-knowledge-context.sh ]; then bash tests/go/test-knowledge-context.sh; fi
 if [ -f tests/go/test-findings.sh ]; then bash tests/go/test-findings.sh; fi
+if [ -f tests/go/test-override.sh ]; then bash tests/go/test-override.sh; fi
 if [ -f tests/go/test-taskdone-reviewers.sh ]; then bash tests/go/test-taskdone-reviewers.sh; fi
 if [ -f tests/go/test-task-done-review.sh ]; then bash tests/go/test-task-done-review.sh; fi
 if [ -f tests/go/test-reviewlog.sh ]; then bash tests/go/test-reviewlog.sh; fi



````

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"cwd":"/private/tmp/claude-501/-Users-dsifry-Developer-metareview/1ce9905e-9420-455e-83c9-fbfa8a0bf8ce/scratchpad/wt-override","exitCode":0,"startedAt":"2026-08-27T17:48:51.764522Z","finishedAt":"2026-08-27T17:48:51.898702Z","stdoutSha256":"a37581d02bda3a379a662ef1d715a64f3c679e9fcc8d215c759c69b860c37e45","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./... exited 0"}

