# Bounded Review FSM Escalation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. When metaswarm is present, metaswarm remains the lifecycle owner; use this plan inside its decomposition, execution, and PR-shepherding flow. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement bounded metareview gate attempts so repeated blocker-fix-review loops escalate instead of running indefinitely, while preserving strict blocker behavior.

**Architecture:** Add a shared `internal/runchain` package that reads `.metareview/runs.jsonl`, validates `--previous-run`, computes attempt metadata, and blocks same-target restarts after `ESCALATED`. Wire task-done, epic-ready, and pr-ready through that package, then extend findings classification, review discovery, and PR evidence so blocker/advisory/follow-up counts and escalation state are visible.

**Tech Stack:** Go 1.x, shell integration tests under `tests/go`, existing Markdown review artifacts, existing JSONL state helpers in `internal/state`.

---

## Source Spec

- Spec: `docs/specs/2026-05-28-bounded-review-fsm-escalation.md`
- Artifact review: `docs/metareview/reviews/mrv-20260529-025714176590000-artifact-2026-05-28-bounded-review-fsm-escalation-940432d8.md`
- Current task-done evidence: `docs/metareview/reviews/mrv-20260529-033535838240000-task-done-2026-05-28-bounded-review-fsm-escalation-940432d8.md`
- Current pr-ready evidence: `docs/metareview/reviews/mrv-20260529-033557589618000-pr-ready-branch-10d735e5.md`

## File Structure

- Create `internal/runchain/runchain.go`: shared attempt-chain reader and validator.
- Create `internal/runchain/runchain_test.go`: unit tests for attempt calculation, max-attempt inheritance, previous-run validation, and escalated restart blocking.
- Modify `cmd/metareview/main.go`: add `--max-attempts <n>` parsing for task-done, epic-ready, and pr-ready.
- Modify `internal/taskdone/review.go`: add options/result/run-record fields and call `runchain.Resolve` before writing artifacts.
- Modify `internal/epicready/review.go`: same wiring as task-done.
- Modify `internal/prready/review.go`: same wiring as task-done, plus pass run metadata into PR evidence.
- Modify `internal/findings/findings.go`: normalize public finding classes and count blocker/advisory/follow-up records.
- Modify `internal/findings/findings_test.go`: classification and count tests.
- Modify `internal/reviewlog/reviewlog.go`: treat `ESCALATED` as unresolved, attach run metadata from `.metareview/runs.jsonl`, and expose run-chain/warning summaries for generated logs.
- Modify `internal/reviewlog/reviewlog_test.go`: `ESCALATED`, attempt metadata, full run-chain, and unknown-classification warning tests.
- Modify `internal/prready/evidence.go`: render attempt count and escalation state in PR evidence.
- Modify `internal/prready/evidence_test.go`: focused PR evidence rendering tests.
- Modify `tests/go/test-task-done-review.sh`: bounded attempt integration tests for task-done.
- Modify `tests/go/test-epic-ready-review.sh`: first/second/third attempt integration tests for epic-ready.
- Modify `tests/go/test-pr-ready-review.sh`: first/second/third attempt integration tests for pr-ready.
- Modify rubrics and docs listed in the spec: `rubrics/task-done-review-rubric.md`, `rubrics/epic-ready-review-rubric.md`, `rubrics/pr-ready-review-rubric.md`, `skills/review-task-done/SKILL.md`, `skills/review-epic-ready/SKILL.md`, `skills/review-pr-ready/SKILL.md`, `docs/quickstart.md`, `docs/README.codex.md`, `docs/README.claude.md`, `INSTALL.md`, and `docs/integrations/metaswarm.md`.

## Work Units

### Task 1: Shared Run-Chain State

**Files:**
- Create: `internal/runchain/runchain_test.go`
- Create: `internal/runchain/runchain.go`

- [ ] **Step 1: Write failing unit tests for attempt calculation and escalation blocking**

Create `internal/runchain/runchain_test.go`:

```go
package runchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveStartsFirstAttempt(t *testing.T) {
	root := t.TempDir()
	decision, err := Resolve(root, Options{
		Scope:       "task-done",
		Target:      map[string]string{"type": "path", "id": "docs/spec.md"},
		MaxAttempts: 0,
	})
	if err != nil {
		t.Fatalf("resolve first attempt: %v", err)
	}
	if decision.AttemptNumber != 1 || decision.MaxAttempts != DefaultMaxAttempts {
		t.Fatalf("unexpected first attempt decision: %+v", decision)
	}
}

func TestResolveFollowsPreviousRunChainAndInheritsRootMax(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":2}`)
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-a","attemptNumber":2,"maxAttempts":2}`)
	decision, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-b",
	})
	if err != nil {
		t.Fatalf("resolve previous chain: %v", err)
	}
	if decision.AttemptNumber != 3 || decision.MaxAttempts != 2 || len(decision.Chain) != 2 || decision.RootRun.ID != "mrv-a" {
		t.Fatalf("expected inherited third attempt from root chain, got %+v", decision)
	}
}

func TestResolveRejectsMissingPreviousRun(t *testing.T) {
	root := t.TempDir()
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-missing",
	})
	if err == nil || !strings.Contains(err.Error(), "previous run mrv-missing not found") {
		t.Fatalf("expected missing previous-run error, got %v", err)
	}
}

func TestResolveRejectsMismatchedPreviousRunTarget(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/other.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-a",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match task-done docs/spec.md") {
		t.Fatalf("expected mismatched target error, got %v", err)
	}
}

func TestResolveRejectsMismatchedPreviousRunScope(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"epic-ready","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-a",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match task-done docs/spec.md") {
		t.Fatalf("expected mismatched scope error, got %v", err)
	}
}


func TestResolveRejectsMismatchedAncestorTarget(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/other.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`)
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-a","attemptNumber":2,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-b",
	})
	if err == nil || !strings.Contains(err.Error(), "ancestor run mrv-a does not match task-done docs/spec.md") {
		t.Fatalf("expected mismatched ancestor error, got %v", err)
	}
}

func TestResolveRejectsBrokenPreviousRunChain(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-missing","attemptNumber":2,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-b",
	})
	if err == nil || !strings.Contains(err.Error(), "previous run chain missing mrv-missing") {
		t.Fatalf("expected broken chain error, got %v", err)
	}
}

func TestResolveRejectsPreviousRunCycle(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-b","attemptNumber":1,"maxAttempts":3}`)
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","previousRunId":"mrv-a","attemptNumber":2,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-b",
	})
	if err == nil || !strings.Contains(err.Error(), "previous run chain cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestChainToReturnsRootToLeaf(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`)
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"ESCALATED","previousRunId":"mrv-a","attemptNumber":2,"maxAttempts":3}`)
	records, err := ReadRuns(root)
	if err != nil {
		t.Fatalf("read runs: %v", err)
	}
	chain, err := ChainTo(records, "mrv-b")
	if err != nil {
		t.Fatalf("chain to run: %v", err)
	}
	if len(chain) != 2 || chain[0].ID != "mrv-a" || chain[1].ID != "mrv-b" {
		t.Fatalf("expected root-to-leaf chain, got %+v", chain)
	}
}

func TestResolveRejectsEscalatedPreviousRun(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"ESCALATED","attemptNumber":3,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-a",
	})
	if err == nil || !strings.Contains(err.Error(), "already escalated") {
		t.Fatalf("expected escalated previous-run error, got %v", err)
	}
}

func TestResolveRejectsOlderPreviousRunAfterSameTargetEscalated(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":2}`)
	writeRun(t, root, `{"id":"mrv-b","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"ESCALATED","previousRunId":"mrv-a","attemptNumber":2,"maxAttempts":2}`)
	_, err := Resolve(root, Options{
		Scope:         "task-done",
		Target:        map[string]string{"type": "path", "id": "docs/spec.md"},
		PreviousRunID: "mrv-a",
	})
	if err == nil || !strings.Contains(err.Error(), "same target already escalated in run mrv-b") {
		t.Fatalf("expected older previous-run bypass to fail, got %v", err)
	}
}

func TestResolveRejectsSameTargetRestartAfterEscalation(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"ESCALATED","attemptNumber":3,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:  "task-done",
		Target: map[string]string{"type": "path", "id": "docs/spec.md"},
	})
	if err == nil || !strings.Contains(err.Error(), "same target already escalated") {
		t.Fatalf("expected same-target escalated restart error, got %v", err)
	}
}

func TestResolveRejectsSameTargetRestartAfterAnyPriorEscalation(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, `{"id":"mrv-a","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"ESCALATED","attemptNumber":3,"maxAttempts":3}`)
	writeRun(t, root, `{"id":"mrv-manual","scope":"task-done","target":{"type":"path","id":"docs/spec.md"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`)
	_, err := Resolve(root, Options{
		Scope:  "task-done",
		Target: map[string]string{"type": "path", "id": "docs/spec.md"},
	})
	if err == nil || !strings.Contains(err.Error(), "same target already escalated in run mrv-a") {
		t.Fatalf("expected older escalated run to block same-target restart, got %v", err)
	}
}

func TestResolveRejectsInvalidMaxAttempts(t *testing.T) {
	root := t.TempDir()
	_, err := Resolve(root, Options{
		Scope:       "task-done",
		Target:      map[string]string{"type": "path", "id": "docs/spec.md"},
		MaxAttempts: -1,
	})
	if err == nil || !strings.Contains(err.Error(), "max attempts must be at least 1") {
		t.Fatalf("expected invalid max attempts error, got %v", err)
	}
}

func writeRun(t *testing.T, root, line string) {
	t.Helper()
	path := filepath.Join(root, ".metareview", "runs.jsonl")
	if err := appendLine(path, line); err != nil {
		t.Fatal(err)
	}
}

func appendLine(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(line + "\n")
	return err
}
```

- [ ] **Step 2: Run the new tests to verify RED**

Run:

```bash
go test ./internal/runchain
```

Expected: FAIL because production symbols such as `Resolve`, `Options`, `ReadRuns`, `ChainTo`, and `DefaultMaxAttempts` do not exist yet.

- [ ] **Step 3: Implement minimal run-chain resolver**

Create `internal/runchain/runchain.go`:

```go
package runchain

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const DefaultMaxAttempts = 3

type Options struct {
	Scope         string
	Target        map[string]string
	PreviousRunID string
	MaxAttempts   int
}

type Decision struct {
	AttemptNumber int
	MaxAttempts   int
	PreviousRun   *Record
	RootRun       *Record
	Chain         []Record
}

type Record struct {
	ID            string            `json:"id"`
	Scope         string            `json:"scope"`
	Target        map[string]string `json:"target"`
	Status        string            `json:"status"`
	Verdict       string            `json:"verdict"`
	PreviousRunID string            `json:"previousRunId"`
	AttemptNumber int               `json:"attemptNumber"`
	MaxAttempts   int               `json:"maxAttempts"`
	BlockingFindingCount int        `json:"blockingFindingCount"`
	AdvisoryFindingCount int        `json:"advisoryFindingCount"`
	FollowUpFindingCount int        `json:"followUpFindingCount"`
	WarningFindingCount  int        `json:"warningFindingCount"`
	EscalationReason     string     `json:"escalationReason"`
}

func Resolve(root string, options Options) (Decision, error) {
	if strings.TrimSpace(options.Scope) == "" {
		return Decision{}, fmt.Errorf("scope is required")
	}
	if len(options.Target) == 0 {
		return Decision{}, fmt.Errorf("target is required")
	}
	if options.MaxAttempts < 0 {
		return Decision{}, fmt.Errorf("max attempts must be at least 1")
	}
	records, err := ReadRuns(root)
	if err != nil {
		return Decision{}, err
	}
	if options.PreviousRunID != "" {
		chain, err := resolveChain(records, options.PreviousRunID)
		if err != nil {
			return Decision{}, err
		}
		previous := chain[len(chain)-1]
		for _, ancestor := range chain {
			if ancestor.Scope != options.Scope || !sameTarget(ancestor.Target, options.Target) {
				return Decision{}, fmt.Errorf("ancestor run %s does not match %s %s", ancestor.ID, options.Scope, targetID(options.Target))
			}
		}
		if strings.EqualFold(previous.Verdict, "ESCALATED") {
			return Decision{}, fmt.Errorf("previous run %s already escalated", options.PreviousRunID)
		}
		if escalated, ok := escalatedForTarget(records, options.Scope, options.Target); ok && escalated.ID != previous.ID {
			return Decision{}, fmt.Errorf("same target already escalated in run %s", escalated.ID)
		}
		root := chain[0]
		max := root.MaxAttempts
		if max == 0 {
			max = effectiveMax(options.MaxAttempts)
		}
		return Decision{AttemptNumber: previous.AttemptNumber + 1, MaxAttempts: max, PreviousRun: &previous, RootRun: &root, Chain: chain}, nil
	}
	if escalated, ok := escalatedForTarget(records, options.Scope, options.Target); ok {
		return Decision{}, fmt.Errorf("same target already escalated in run %s", escalated.ID)
	}
	return Decision{AttemptNumber: 1, MaxAttempts: effectiveMax(options.MaxAttempts)}, nil
}

func ReadRuns(root string) ([]Record, error) {
	path := filepath.Join(root, ".metareview", "runs.jsonl")
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var records []Record
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		if record.AttemptNumber == 0 {
			record.AttemptNumber = 1
		}
		if record.MaxAttempts == 0 {
			record.MaxAttempts = DefaultMaxAttempts
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}

func ChainTo(records []Record, runID string) ([]Record, error) {
	byID := map[string]Record{}
	for _, record := range records {
		byID[record.ID] = record
	}
	var reversed []Record
	seen := map[string]bool{}
	for id := runID; id != ""; {
		if seen[id] {
			return nil, fmt.Errorf("previous run chain cycle at %s", id)
		}
		seen[id] = true
		record, ok := byID[id]
		if !ok {
			if id == runID {
				return nil, fmt.Errorf("previous run %s not found", id)
			}
			return nil, fmt.Errorf("previous run chain missing %s", id)
		}
		reversed = append(reversed, record)
		id = record.PreviousRunID
	}
	chain := make([]Record, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		chain = append(chain, reversed[i])
	}
	return chain, nil
}

func resolveChain(records []Record, previousRunID string) ([]Record, error) {
	return ChainTo(records, previousRunID)
}

func escalatedForTarget(records []Record, scope string, target map[string]string) (Record, bool) {
	for _, record := range records {
		if record.Scope == scope && sameTarget(record.Target, target) {
			if strings.EqualFold(record.Verdict, "ESCALATED") {
				return record, true
			}
		}
	}
	return Record{}, false
}

func sameTarget(a, b map[string]string) bool {
	return reflect.DeepEqual(a, b)
}

func targetID(target map[string]string) string {
	if target["id"] != "" {
		return target["id"]
	}
	return target["path"]
}

func effectiveMax(value int) int {
	if value > 0 {
		return value
	}
	return DefaultMaxAttempts
}
```

- [ ] **Step 4: Format runchain files**

Run:

```bash
gofmt -w internal/runchain/runchain.go internal/runchain/runchain_test.go
```

Expected: exits 0 and only formatting changes are applied.

- [ ] **Step 5: Run GREEN**

Run:

```bash
go test ./internal/runchain
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/runchain/runchain.go internal/runchain/runchain_test.go
git commit -m "test: add bounded review run-chain resolver"
```

### Task 2: Finding Classification Counts

**Files:**
- Modify: `internal/findings/findings.go`
- Modify: `internal/findings/findings_test.go`

- [ ] **Step 1: Write failing classification tests**

Add to `internal/findings/findings_test.go`:

```go
func TestClassCountsSeparateBlockersAdvisoriesAndFollowups(t *testing.T) {
	root := t.TempDir()
	run := Run{ID: "mrv-counts", Scope: "task-done", Target: map[string]string{"type": "path", "id": "docs/task.md"}, RepoRoot: root, GitHead: "abc"}
	advisory := unsafeEval("Prefer a narrower helper.")
	advisory.Classification = "ADVISORY"
	advisory.Severity = "medium"
	advisory.Fingerprint = "advisory:narrow-helper"
	followup := unsafeEval("Adjacent cleanup should be tracked later.")
	followup.Classification = "FOLLOW_UP"
	followup.Severity = "low"
	followup.Fingerprint = "follow-up:cleanup"

	result, err := Reconcile(root, run, []Input{advisory, followup}, Options{})
	if err != nil {
		t.Fatalf("reconcile non-blocking classes: %v", err)
	}
	persisted, err := readJSONL(findingsPath(root))
	if err != nil {
		t.Fatalf("read persisted findings: %v", err)
	}
	if persisted[0].Classification != "advisory" || persisted[1].Classification != "follow-up" {
		t.Fatalf("expected canonical persisted classes, got %q and %q", persisted[0].Classification, persisted[1].Classification)
	}
	counts := CountByClass(result.Findings)
	if counts.Blocking != 0 || counts.Advisory != 1 || counts.FollowUp != 1 {
		t.Fatalf("unexpected class counts: %+v", counts)
	}
	if result.OpenBlockingCount != 0 {
		t.Fatalf("non-blocking classes should not block, got %d", result.OpenBlockingCount)
	}
}

func TestUnknownClassificationIsNonBlockingWarning(t *testing.T) {
	root := t.TempDir()
	run := Run{ID: "mrv-unknown", Scope: "task-done", Target: map[string]string{"type": "path", "id": "docs/task.md"}, RepoRoot: root, GitHead: "abc"}
	unknown := unsafeEval("Unknown class should not silently block.")
	unknown.Classification = "novel"
	unknown.Severity = "high"
	unknown.Fingerprint = "unknown:classification"

	result, err := Reconcile(root, run, []Input{unknown}, Options{})
	if err != nil {
		t.Fatalf("reconcile unknown class: %v", err)
	}
	persisted, err := readJSONL(findingsPath(root))
	if err != nil {
		t.Fatalf("read persisted findings: %v", err)
	}
	if persisted[0].Classification != "warning" {
		t.Fatalf("expected canonical warning class, got %q", persisted[0].Classification)
	}
	counts := CountByClass(result.Findings)
	if counts.Blocking != 0 || counts.Warnings != 1 {
		t.Fatalf("unknown class should be a warning, got %+v", counts)
	}
}

func TestPublicBlockerPersistsAsCanonicalBlockingClass(t *testing.T) {
	root := t.TempDir()
	run := Run{ID: "mrv-blocker", Scope: "task-done", Target: map[string]string{"type": "path", "id": "docs/task.md"}, RepoRoot: root, GitHead: "abc"}
	blocker := unsafeEval("Public BLOCKER must remain blocking even without high severity.")
	blocker.Classification = "BLOCKER"
	blocker.Severity = "medium"
	blocker.Fingerprint = "blocker:contract"

	result, err := Reconcile(root, run, []Input{blocker}, Options{})
	if err != nil {
		t.Fatalf("reconcile public blocker: %v", err)
	}
	persisted, err := readJSONL(findingsPath(root))
	if err != nil {
		t.Fatalf("read persisted findings: %v", err)
	}
	if persisted[0].Classification != "spec-contract" {
		t.Fatalf("expected canonical blocking class, got %q", persisted[0].Classification)
	}
	if counts := CountByClass(result.Findings); counts.Blocking != 1 {
		t.Fatalf("public blocker must remain blocking, got %+v", counts)
	}
	if counts := CountByClass(persisted); counts.Blocking != 1 {
		t.Fatalf("persisted blocker must reload as blocking, got %+v", counts)
	}
}

func TestLegacyBlockingClassificationKeepsSeveritySemantics(t *testing.T) {
	records := []Record{
		{Classification: "blocking", Severity: "medium"},
		{Classification: "blocking", Severity: "low"},
		{Classification: "blocking", Severity: "high"},
		{Classification: "blocking", Severity: "critical"},
		{Classification: "spec-contract", Severity: "low"},
	}
	counts := CountByClass(records)
	if counts.Blocking != 3 || counts.Warnings != 2 {
		t.Fatalf("legacy blocking severity semantics changed: %+v", counts)
	}
}

func TestReconcileFixesAncestorChainFindingWhenAbsentFromCurrentRun(t *testing.T) {
	root := t.TempDir()
	target := map[string]string{"type": "path", "id": "docs/task.md"}
	runA := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	repeated := unsafeEval("eval remains")
	repeated.Classification = "BLOCKER"
	repeated.Fingerprint = "blocker:eval"
	if _, err := Reconcile(root, runA, []Input{repeated}, Options{}); err != nil {
		t.Fatalf("seed first run: %v", err)
	}

	runB := Run{ID: "mrv-b", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "bbb"}
	result, err := Reconcile(root, runB, []Input{repeated}, Options{PreviousRunID: "mrv-a", PreviousRunIDs: []string{"mrv-a"}})
	if err != nil {
		t.Fatalf("reconcile repeated blocker: %v", err)
	}
	if counts := CountByClass(result.OpenFindings); counts.Blocking != 1 {
		t.Fatalf("repeated blocker should remain open in chain counts, got %+v", counts)
	}

	runC := Run{ID: "mrv-c", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "ccc"}
	result, err = Reconcile(root, runC, nil, Options{PreviousRunID: "mrv-b", PreviousRunIDs: []string{"mrv-a", "mrv-b"}})
	if err != nil {
		t.Fatalf("reconcile fixed ancestor blocker: %v", err)
	}
	if result.OpenBlockingCount != 0 || len(result.OpenFindings) != 0 {
		t.Fatalf("ancestor blocker should be fixed when absent from current run, got count=%d open=%+v", result.OpenBlockingCount, result.OpenFindings)
	}
	persisted, err := readJSONL(findingsPath(root))
	if err != nil {
		t.Fatalf("read persisted findings: %v", err)
	}
	if persisted[0].Status != "fixed" || persisted[0].FixedInRunID != "mrv-c" {
		t.Fatalf("expected ancestor finding fixed in mrv-c, got %+v", persisted[0])
	}
}

func TestReconcileReportsOlderOpenSameTargetFindings(t *testing.T) {
	root := t.TempDir()
	target := map[string]string{"type": "path", "id": "docs/task.md"}
	runA := Run{ID: "mrv-a", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "aaa"}
	blocker := unsafeEval("eval remains")
	blocker.Classification = "BLOCKER"
	blocker.Fingerprint = "blocker:eval"
	if _, err := Reconcile(root, runA, []Input{blocker}, Options{}); err != nil {
		t.Fatalf("seed first run: %v", err)
	}

	runB := Run{ID: "mrv-b", Scope: "task-done", Target: target, RepoRoot: root, GitHead: "bbb"}
	result, err := Reconcile(root, runB, nil, Options{})
	if err != nil {
		t.Fatalf("reconcile same target without chain: %v", err)
	}
	if counts := CountByClass(result.OpenFindings); counts.Blocking != 1 {
		t.Fatalf("older same-target blocker must still drive gate verdict, got %+v", counts)
	}
}
```

- [ ] **Step 2: Run RED**

Run:

```bash
go test ./internal/findings
```

Expected: FAIL because `CountByClass`, warning counting, canonical persisted classifications, legacy blocking severity semantics, chain-aware finding closure, and `OpenFindings` do not exist.

- [ ] **Step 3: Implement classification normalization and counts**

In `internal/findings/findings.go`, add:

```go
type ClassCounts struct {
	Blocking int
	Advisory int
	FollowUp int
	Warnings int
}

func CountByClass(records []Record) ClassCounts {
	var counts ClassCounts
	for _, record := range records {
		switch classForCount(record.Classification, record.Severity) {
		case "blocking":
			counts.Blocking++
		case "advisory":
			counts.Advisory++
		case "follow-up":
			counts.FollowUp++
		case "warning":
			counts.Warnings++
		}
	}
	return counts
}

func canonicalClass(classification string) string {
	class := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(classification, "_", "-")))
	switch class {
	case "blocker", "spec-contract":
		return "spec-contract"
	case "blocking":
		return "blocking"
	case "advisory":
		return "advisory"
	case "follow-up", "followup":
		return "follow-up"
	default:
		return "warning"
	}
}

func classForCount(classification, severity string) string {
	switch canonicalClass(classification) {
	case "spec-contract":
		return "blocking"
	case "blocking":
		switch strings.ToLower(strings.TrimSpace(severity)) {
		case "critical", "high":
			return "blocking"
		default:
			return "warning"
		}
	case "advisory":
		return "advisory"
	case "follow-up":
		return "follow-up"
	default:
		return "warning"
	}
}
```

Extend `Options` and `Result`:

```go
type Options struct {
	PreviousRunID  string
	PreviousRunIDs []string
}

type Result struct {
	Findings          []Record `json:"findings"`
	NewFindings       []Record `json:"newFindings"`
	OpenFindings      []Record `json:"openFindings"`
	OpenBlockingCount int      `json:"openBlockingCount"`
}
```

Add helpers:

```go
func previousRunSet(options Options) map[string]bool {
	ids := map[string]bool{}
	if options.PreviousRunID != "" {
		ids[options.PreviousRunID] = true
	}
	for _, id := range options.PreviousRunIDs {
		if id != "" {
			ids[id] = true
		}
	}
	return ids
}

func openForTarget(records []Record, target any) []Record {
	open := make([]Record, 0, len(records))
	for _, record := range records {
		if record.Status == "open" && sameTarget(firstTarget(record.Target, target), target) {
			open = append(open, record)
		}
	}
	return open
}
```

In `Reconcile`, replace the single immediate-previous fix condition with the previous-run set:

```go
previousRuns := previousRunSet(options)
...
if previousRuns[record.RunID] &&
	sameTarget(firstTarget(record.Target, run.Target), run.Target) &&
	record.Status == "open" &&
	record.Fingerprint != "" &&
	!currentFingerprints[record.Fingerprint] {
	record.Status = "fixed"
	record.FixedInRunID = run.ID
	record.UpdatedAt = now
	record.GitHead = run.GitHead
}
```

After `all := append(updated, newRecords...)`, compute unresolved same-target findings and return them:

```go
openFindings := openForTarget(all, run.Target)
openCounts := CountByClass(openFindings)
...
return Result{
	Findings:          activeCurrent,
	NewFindings:       newRecords,
	OpenFindings:      openFindings,
	OpenBlockingCount: openCounts.Blocking,
}, nil
```

Update `unresolvedBlockingFrom` to call `classForCount`:

```go
if classForCount(record.Classification, record.Severity) == "blocking" {
	blockers = append(blockers, record)
}
```

Update `normalize` so persisted records use the canonical class:

```go
	record.Classification = canonicalClass(input.Classification)
```

- [ ] **Step 4: Run GREEN**

Run:

```bash
go test ./internal/findings
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/findings/findings.go internal/findings/findings_test.go
git commit -m "feat: classify metareview findings by blocker state"
```

### Task 3: Lifecycle Gate Attempt Metadata

**Files:**
- Modify: `cmd/metareview/main.go`
- Modify: `internal/taskdone/review.go`
- Create: `internal/taskdone/review_markdown_test.go`
- Modify: `internal/epicready/review.go`
- Create: `internal/epicready/review_markdown_test.go`
- Modify: `internal/prready/review.go`
- Create: `internal/prready/review_markdown_test.go`
- Modify: `tests/go/test-task-done-review.sh`
- Modify: `tests/go/test-epic-ready-review.sh`
- Modify: `tests/go/test-pr-ready-review.sh`

- [ ] **Step 1: Write failing task-done integration test**

Append to `tests/go/test-task-done-review.sh` after the first blocker/pass scenario:

```bash
mkdir -p "$TMP/default-escalate/lib" "$TMP/default-escalate/.beads"
cd "$TMP/default-escalate"
git init -q
git config user.email test-user
git config user.name "Test User"
printf '{"id":"task-default","title":"Default bounded parser","description":"Parse expressions without executing input","acceptance":["no eval"]}\n' > .beads/issues.jsonl
printf "'use strict';\nmodule.exports = input => JSON.parse(input);\n" > lib/parser.js
git add .
git commit -qm "initial"
default_base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => eval(input);\n" > lib/parser.js
git add .
git commit -qm "unsafe"

set +e
"$TMP/metareview" review task-done task-default --base "$default_base" > "$TMP/default-1.out" 2>"$TMP/default-1.err"
code=$?
set -e
test "$code" -eq 1
default_run_1="$(node -e "const fs=require('fs'); const lines=fs.readFileSync('.metareview/runs.jsonl','utf8').trim().split('\n').map(JSON.parse); const r=lines.at(-1); if (r.attemptNumber!==1 || r.maxAttempts!==3 || r.verdict!=='NEEDS_REVISION') process.exit(1); console.log(r.id)")"

set +e
"$TMP/metareview" review task-done task-default --base "$default_base" --previous-run "$default_run_1" > "$TMP/default-2.out" 2>"$TMP/default-2.err"
code=$?
set -e
test "$code" -eq 1
default_run_2="$(node -e "const fs=require('fs'); const lines=fs.readFileSync('.metareview/runs.jsonl','utf8').trim().split('\n').map(JSON.parse); const r=lines.at(-1); if (r.attemptNumber!==2 || r.maxAttempts!==3 || r.previousRunId!=='$default_run_1' || r.verdict!=='NEEDS_REVISION') process.exit(1); console.log(r.id)")"

set +e
"$TMP/metareview" review task-done task-default --base "$default_base" --previous-run "$default_run_2" > "$TMP/default-3.out" 2>"$TMP/default-3.err"
code=$?
set -e
test "$code" -eq 1
default_run_3="$(node -e "const fs=require('fs'); const lines=fs.readFileSync('.metareview/runs.jsonl','utf8').trim().split('\n').map(JSON.parse); const r=lines.at(-1); if (r.attemptNumber!==3 || r.maxAttempts!==3 || r.previousRunId!=='$default_run_2' || r.verdict!=='ESCALATED' || r.status!=='escalated') process.exit(1); console.log(r.id)")"
grep -q "ESCALATED" "$(cat "$TMP/default-3.out")"

mkdir -p "$TMP/escalate/lib" "$TMP/escalate/.beads"
cd "$TMP/escalate"
git init -q
git config user.email test-user
git config user.name "Test User"
printf '{"id":"task-1","title":"Bounded unsafe parser","description":"Parse expressions without executing input","acceptance":["no eval"]}\n' > .beads/issues.jsonl
printf "'use strict';\nmodule.exports = input => JSON.parse(input);\n" > lib/parser.js
git add .
git commit -qm "initial"
escalate_base="$(git rev-parse HEAD)"
printf "'use strict';\nmodule.exports = input => eval(input);\n" > lib/parser.js
git add .
git commit -qm "unsafe"

set +e
"$TMP/metareview" review task-done task-1 --base "$escalate_base" --max-attempts 2 > "$TMP/escalate-1.out" 2>"$TMP/escalate-1.err"
code=$?
set -e
test "$code" -eq 1
first_escalate_run="$(node -e "const fs=require('fs'); const lines=fs.readFileSync('.metareview/runs.jsonl','utf8').trim().split('\n').map(JSON.parse); const r=lines.at(-1); if (r.attemptNumber!==1 || r.maxAttempts!==2 || r.verdict!=='NEEDS_REVISION') process.exit(1); console.log(r.id)")"

set +e
"$TMP/metareview" review task-done task-1 --base "$escalate_base" --previous-run "$first_escalate_run" > "$TMP/escalate-2.out" 2>"$TMP/escalate-2.err"
code=$?
set -e
test "$code" -eq 1
second_escalate_run="$(node -e "const fs=require('fs'); const lines=fs.readFileSync('.metareview/runs.jsonl','utf8').trim().split('\n').map(JSON.parse); const r=lines.at(-1); if (r.attemptNumber!==2 || r.maxAttempts!==2 || r.previousRunId!=='$first_escalate_run' || r.verdict!=='ESCALATED' || r.status!=='escalated' || !r.escalationReason) process.exit(1); console.log(r.id)")"
grep -q "ESCALATED" "$(cat "$TMP/escalate-2.out")"
grep -q "## Run Chain" "$(cat "$TMP/escalate-2.out")"
grep -q "$first_escalate_run" "$(cat "$TMP/escalate-2.out")"
post_escalation_count="$(find docs/metareview/reviews -type f 2>/dev/null | wc -l | tr -d ' ')"
post_escalation_context_count="$(find docs/metareview/context -type f 2>/dev/null | wc -l | tr -d ' ')"
post_escalation_runs_hash="$(shasum .metareview/runs.jsonl | awk '{print $1}')"
post_escalation_findings_hash="$(shasum .metareview/findings.jsonl | awk '{print $1}')"

set +e
"$TMP/metareview" review task-done task-1 --base "$escalate_base" --previous-run "$second_escalate_run" > "$TMP/escalate-3.out" 2>"$TMP/escalate-3.err"
code=$?
set -e
test "$code" -ne 0
test "$post_escalation_count" = "$(find docs/metareview/reviews -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$post_escalation_context_count" = "$(find docs/metareview/context -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$post_escalation_runs_hash" = "$(shasum .metareview/runs.jsonl | awk '{print $1}')"
test "$post_escalation_findings_hash" = "$(shasum .metareview/findings.jsonl | awk '{print $1}')"
grep -q "already escalated" "$TMP/escalate-3.err"

set +e
"$TMP/metareview" review task-done task-1 --base "$escalate_base" --previous-run "$first_escalate_run" > "$TMP/escalate-ancestor.out" 2>"$TMP/escalate-ancestor.err"
code=$?
set -e
test "$code" -ne 0
test "$post_escalation_count" = "$(find docs/metareview/reviews -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$post_escalation_context_count" = "$(find docs/metareview/context -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$post_escalation_runs_hash" = "$(shasum .metareview/runs.jsonl | awk '{print $1}')"
test "$post_escalation_findings_hash" = "$(shasum .metareview/findings.jsonl | awk '{print $1}')"
grep -q "same target already escalated" "$TMP/escalate-ancestor.err"

set +e
"$TMP/metareview" review task-done task-1 --base "$escalate_base" > "$TMP/escalate-restart.out" 2>"$TMP/escalate-restart.err"
code=$?
set -e
test "$code" -ne 0
test "$post_escalation_count" = "$(find docs/metareview/reviews -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$post_escalation_context_count" = "$(find docs/metareview/context -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$post_escalation_runs_hash" = "$(shasum .metareview/runs.jsonl | awk '{print $1}')"
test "$post_escalation_findings_hash" = "$(shasum .metareview/findings.jsonl | awk '{print $1}')"
grep -q "same target already escalated" "$TMP/escalate-restart.err"

before_count="$post_escalation_count"
before_context_count="$post_escalation_context_count"
before_runs_hash="$post_escalation_runs_hash"
before_findings_hash="$post_escalation_findings_hash"
set +e
"$TMP/metareview" review task-done task-1 --base "$escalate_base" --max-attempts 0 > "$TMP/invalid-max.out" 2>"$TMP/invalid-max.err"
code=$?
set -e
after_count="$(find docs/metareview/reviews -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$code" -ne 0
test "$before_count" = "$after_count"
test "$before_context_count" = "$(find docs/metareview/context -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$before_runs_hash" = "$(shasum .metareview/runs.jsonl | awk '{print $1}')"
test "$before_findings_hash" = "$(shasum .metareview/findings.jsonl | awk '{print $1}')"
grep -q "max attempts must be an integer greater than 0" "$TMP/invalid-max.err"

set +e
"$TMP/metareview" review task-done task-1 --base "$escalate_base" --previous-run mrv-missing > "$TMP/missing-previous.out" 2>"$TMP/missing-previous.err"
code=$?
set -e
test "$code" -ne 0
test "$before_count" = "$(find docs/metareview/reviews -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$before_context_count" = "$(find docs/metareview/context -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$before_runs_hash" = "$(shasum .metareview/runs.jsonl | awk '{print $1}')"
test "$before_findings_hash" = "$(shasum .metareview/findings.jsonl | awk '{print $1}')"
grep -q "previous run mrv-missing not found" "$TMP/missing-previous.err"

node -e "const fs=require('fs'); fs.appendFileSync('.metareview/runs.jsonl', JSON.stringify({id:'mrv-other-target',scope:'task-done',target:{type:'path',id:'other-task'},verdict:'NEEDS_REVISION',attemptNumber:1,maxAttempts:3})+'\\n')"
mismatch_count="$(find docs/metareview/reviews -type f 2>/dev/null | wc -l | tr -d ' ')"
mismatch_context_count="$(find docs/metareview/context -type f 2>/dev/null | wc -l | tr -d ' ')"
mismatch_runs_hash="$(shasum .metareview/runs.jsonl | awk '{print $1}')"
mismatch_findings_hash="$(shasum .metareview/findings.jsonl | awk '{print $1}')"
set +e
"$TMP/metareview" review task-done task-1 --base "$escalate_base" --previous-run mrv-other-target > "$TMP/mismatch-previous.out" 2>"$TMP/mismatch-previous.err"
code=$?
set -e
test "$code" -ne 0
test "$mismatch_count" = "$(find docs/metareview/reviews -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$mismatch_context_count" = "$(find docs/metareview/context -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$mismatch_runs_hash" = "$(shasum .metareview/runs.jsonl | awk '{print $1}')"
test "$mismatch_findings_hash" = "$(shasum .metareview/findings.jsonl | awk '{print $1}')"
grep -q "does not match task-done" "$TMP/mismatch-previous.err"

node -e "const fs=require('fs'); fs.appendFileSync('.metareview/runs.jsonl', JSON.stringify({id:'mrv-other-scope',scope:'epic-ready',target:{type:'path',id:'docs/spec.md'},verdict:'NEEDS_REVISION',attemptNumber:1,maxAttempts:3})+'\\n')"
scope_count="$(find docs/metareview/reviews -type f 2>/dev/null | wc -l | tr -d ' ')"
scope_context_count="$(find docs/metareview/context -type f 2>/dev/null | wc -l | tr -d ' ')"
scope_runs_hash="$(shasum .metareview/runs.jsonl | awk '{print $1}')"
scope_findings_hash="$(shasum .metareview/findings.jsonl | awk '{print $1}')"
set +e
"$TMP/metareview" review task-done task-1 --base "$escalate_base" --previous-run mrv-other-scope > "$TMP/mismatch-scope.out" 2>"$TMP/mismatch-scope.err"
code=$?
set -e
test "$code" -ne 0
test "$scope_count" = "$(find docs/metareview/reviews -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$scope_context_count" = "$(find docs/metareview/context -type f 2>/dev/null | wc -l | tr -d ' ')"
test "$scope_runs_hash" = "$(shasum .metareview/runs.jsonl | awk '{print $1}')"
test "$scope_findings_hash" = "$(shasum .metareview/findings.jsonl | awk '{print $1}')"
grep -q "does not match task-done" "$TMP/mismatch-scope.err"
```

Create `internal/taskdone/review_markdown_test.go`:

```go
package taskdone

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
)

func TestReviewMarkdownSeparatesNonBlockingFindings(t *testing.T) {
	records := []findings.Record{
		{Reviewer: "code-quality-reviewer", Classification: "advisory", Severity: "medium", Title: "Prefer helper", Finding: "Helper would reduce duplication."},
		{Reviewer: "architecture-reviewer", Classification: "follow-up", Severity: "low", Title: "Track cleanup", Finding: "Cleanup belongs in a later target."},
		{Reviewer: "security-reviewer", Classification: "warning", Severity: "high", Title: "Unknown class", Finding: "Unknown classification was downgraded to warning."},
	}
	md := reviewMarkdown("mrv-task", "task-1", "ctx.md", "", "gate", "PASS_ADVISORY", records, reviewMetadata{AdvisoryFindingCount: 1, FollowUpFindingCount: 1, WarningFindingCount: 1})
	if strings.Contains(md, "| code-quality-reviewer | NEEDS_REVISION | 1 |") || strings.Contains(md, "| architecture-reviewer | NEEDS_REVISION | 1 |") {
		t.Fatalf("non-blocking findings must not render as blocking reviewer failures:\n%s", md)
	}
	for _, required := range []string{"| code-quality-reviewer | PASS_ADVISORY | 0 | Prefer helper |", "## Advisory Findings", "## Follow-up Findings", "## Warnings", "Unknown classification was downgraded to warning."} {
		if !strings.Contains(md, required) {
			t.Fatalf("review markdown missing %q:\n%s", required, md)
		}
	}
}

func TestVerdictForNonBlockingFindingsIsPassAdvisory(t *testing.T) {
	counts := findings.ClassCounts{Advisory: 1, FollowUp: 1}
	verdict, status, blocking, reason := verdictForCounts(counts, "gate", 1, 3)
	if verdict != "PASS_ADVISORY" || status != "passed" || blocking || reason != "" {
		t.Fatalf("non-blocking findings must produce PASS_ADVISORY, got verdict=%s status=%s blocking=%v reason=%q", verdict, status, blocking, reason)
	}
}
```

Create `internal/epicready/review_markdown_test.go`:

```go
package epicready

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
)

func TestReviewMarkdownSeparatesNonBlockingFindings(t *testing.T) {
	records := []findings.Record{
		{Reviewer: "acceptance-reviewer", Classification: "advisory", Severity: "medium", Title: "Prefer helper", Finding: "Helper would reduce duplication."},
		{Reviewer: "architecture-reviewer", Classification: "follow-up", Severity: "low", Title: "Track cleanup", Finding: "Cleanup belongs in a later target."},
		{Reviewer: "intent-preservation-reviewer", Classification: "warning", Severity: "high", Title: "Unknown class", Finding: "Unknown classification was downgraded to warning."},
	}
	md := reviewMarkdown("mrv-epic", "epic-1", "ctx.md", "", "gate", "PASS_ADVISORY", records, reviewMetadata{AdvisoryFindingCount: 1, FollowUpFindingCount: 1, WarningFindingCount: 1})
	if strings.Contains(md, "| acceptance-reviewer | NEEDS_REVISION | 1 |") || strings.Contains(md, "| architecture-reviewer | NEEDS_REVISION | 1 |") {
		t.Fatalf("non-blocking findings must not render as blocking reviewer failures:\n%s", md)
	}
	for _, required := range []string{"| acceptance-reviewer | PASS_ADVISORY | 0 | Prefer helper |", "## Advisory Findings", "## Follow-up Findings", "## Warnings", "Unknown classification was downgraded to warning."} {
		if !strings.Contains(md, required) {
			t.Fatalf("review markdown missing %q:\n%s", required, md)
		}
	}
}

func TestVerdictForNonBlockingFindingsIsPassAdvisory(t *testing.T) {
	counts := findings.ClassCounts{Advisory: 1, FollowUp: 1}
	verdict, status, blocking, reason := verdictForCounts(counts, "gate", 1, 3)
	if verdict != "PASS_ADVISORY" || status != "passed" || blocking || reason != "" {
		t.Fatalf("non-blocking findings must produce PASS_ADVISORY, got verdict=%s status=%s blocking=%v reason=%q", verdict, status, blocking, reason)
	}
}
```

Create `internal/prready/review_markdown_test.go`:

```go
package prready

import (
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/findings"
)

func TestReviewMarkdownSeparatesNonBlockingFindings(t *testing.T) {
	records := []findings.Record{
		{Reviewer: "validation-reviewer", Classification: "advisory", Severity: "medium", Title: "Prefer helper", Finding: "Helper would reduce duplication."},
		{Reviewer: "architecture-reviewer", Classification: "follow-up", Severity: "low", Title: "Track cleanup", Finding: "Cleanup belongs in a later target."},
		{Reviewer: "code-quality-reviewer", Classification: "warning", Severity: "high", Title: "Unknown class", Finding: "Unknown classification was downgraded to warning."},
	}
	md := reviewMarkdown("mrv-pr", "ctx.md", "", "gate", "PASS_ADVISORY", records, "final PR evidence", reviewMetadata{AdvisoryFindingCount: 1, FollowUpFindingCount: 1, WarningFindingCount: 1})
	if strings.Contains(md, "| validation-reviewer | NEEDS_REVISION | 1 |") || strings.Contains(md, "| architecture-reviewer | NEEDS_REVISION | 1 |") {
		t.Fatalf("non-blocking findings must not render as blocking reviewer failures:\n%s", md)
	}
	for _, required := range []string{"| validation-reviewer | PASS_ADVISORY | 0 | Prefer helper |", "## Advisory Findings", "## Follow-up Findings", "## Warnings", "Unknown classification was downgraded to warning.", "final PR evidence"} {
		if !strings.Contains(md, required) {
			t.Fatalf("review markdown missing %q:\n%s", required, md)
		}
	}
}

func TestVerdictForNonBlockingFindingsIsPassAdvisory(t *testing.T) {
	counts := findings.ClassCounts{Advisory: 1, FollowUp: 1}
	verdict, status, blocking, reason := verdictForCounts(counts, "gate", 1, 3)
	if verdict != "PASS_ADVISORY" || status != "passed" || blocking || reason != "" {
		t.Fatalf("non-blocking findings must produce PASS_ADVISORY, got verdict=%s status=%s blocking=%v reason=%q", verdict, status, blocking, reason)
	}
}
```

- [ ] **Step 2: Run RED for task-done integration**

Run:

```bash
bash tests/go/test-task-done-review.sh
go test ./internal/taskdone ./internal/epicready ./internal/prready
```

Expected: FAIL because `--max-attempts` is unknown, run records lack attempt fields, invalid previous-run failures still write artifacts, mismatched-scope previous runs still write artifacts, already-escalated failures still write artifacts, older previous-run bypass after escalation is still accepted, escalated logs lack a run-chain section, non-blocking records still render as blocking reviewer failures, or non-blocking findings aggregate as `PASS` instead of `PASS_ADVISORY`.

- [ ] **Step 3: Add `--max-attempts` parsing**

In `cmd/metareview/main.go`, extend only the task-done option switch with:

```go
case "--max-attempts":
	options.MaxAttempts = mustPositiveInt(flagValue(args, i, "--max-attempts"), "--max-attempts")
	i++
```

Add helper functions near `flagValue`:

```go
func mustPositiveInt(value, name string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		fmt.Fprintf(os.Stderr, "%s must be an integer greater than 0\n", name)
		os.Exit(2)
	}
	return parsed
}
```

Add import:

```go
import "strconv"
```

Do not add the epic-ready or pr-ready `--max-attempts` switches yet; those options structs are extended in Step 6 before their parser branches are wired. This keeps the Step 5 RED failures focused on missing epic/pr behavior instead of a compile error.

- [ ] **Step 4: Wire task-done through runchain**

In `internal/taskdone/review.go`:

Add import:

```go
	"github.com/dsifry/metareview/internal/runchain"
```

Extend `Options`:

```go
	MaxAttempts   int
```

Extend `Result`:

```go
	AttemptNumber int
	MaxAttempts   int
```

Replace the existing `runRecord.PreviousRunID *string` field with a string field, then extend `runRecord` with the attempt/count fields:

```go
	AttemptNumber        int    `json:"attemptNumber"`
	MaxAttempts          int    `json:"maxAttempts"`
	PreviousRunID        string `json:"previousRunId,omitempty"`
	BlockingFindingCount int    `json:"blockingFindingCount"`
	AdvisoryFindingCount int    `json:"advisoryFindingCount"`
	FollowUpFindingCount int    `json:"followUpFindingCount"`
	WarningFindingCount  int    `json:"warningFindingCount"`
	EscalationReason     string `json:"escalationReason"`
```

Remove `optionalString(options.PreviousRunID)` at the record call site and assign `PreviousRunID: options.PreviousRunID` directly. After all lifecycle packages use the string field, remove their now-unused `optionalString` helpers.

After `targetRecord` is built, before snapshots are captured, call:

```go
	chain, err := runchain.Resolve(root, runchain.Options{
		Scope:         "task-done",
		Target:        targetRecord,
		PreviousRunID: options.PreviousRunID,
		MaxAttempts:   options.MaxAttempts,
	})
	if err != nil {
		return Result{}, err
	}
	previousRunIDs := make([]string, 0, len(chain.Chain))
	for _, link := range chain.Chain {
		previousRunIDs = append(previousRunIDs, link.ID)
	}
```

Pass `previousRunIDs` into finding reconciliation:

```go
	reconciled, err := findings.Reconcile(root, run, rawFindings, findings.Options{
		PreviousRunID:  options.PreviousRunID,
		PreviousRunIDs: previousRunIDs,
	})
```

Add helper and replace verdict calculation with calls to it:

```go
func verdictForCounts(counts findings.ClassCounts, gateEffect string, attemptNumber, maxAttempts int) (string, string, bool, string) {
	blocking := counts.Blocking > 0
	nonBlocking := counts.Advisory > 0 || counts.FollowUp > 0 || counts.Warnings > 0
	if blocking && attemptNumber >= maxAttempts {
		reason := fmt.Sprintf("blocking findings remain after attempt %d of %d", attemptNumber, maxAttempts)
		return "ESCALATED", "escalated", true, reason
	}
	if blocking {
		return "NEEDS_REVISION", "needs-revision", true, ""
	}
	if gateEffect == "advisory" || nonBlocking {
		return "PASS_ADVISORY", "passed", false, ""
	}
	return "PASS", "passed", false, ""
}
```

Then after `counts := findings.CountByClass(reconciled.OpenFindings)`. Lifecycle verdicts must use unresolved same-target findings after reconciliation, not only findings emitted by the current run, so an older open blocker cannot be hidden by deduplication:

```go
			verdict, status, blocking, escalationReason := verdictForCounts(counts, gateEffect, chain.AttemptNumber, chain.MaxAttempts)
```

Populate `result`:

```go
		result.Verdict = verdict
		result.Blocking = blocking
		result.AttemptNumber = chain.AttemptNumber
		result.MaxAttempts = chain.MaxAttempts
```

Populate `record`:

```go
			AttemptNumber:        chain.AttemptNumber,
			MaxAttempts:          chain.MaxAttempts,
			PreviousRunID:        options.PreviousRunID,
			BlockingFindingCount: counts.Blocking,
			AdvisoryFindingCount: counts.Advisory,
			FollowUpFindingCount: counts.FollowUp,
			WarningFindingCount:  counts.Warnings,
			EscalationReason:     escalationReason,
```

Update the task-done review Markdown builder before running GREEN. In `internal/taskdone/review.go`, add a local metadata type:

```go
type reviewMetadata struct {
	AttemptNumber        int
	MaxAttempts          int
	RunChain             []runchain.Record
	BlockingFindingCount int
	AdvisoryFindingCount int
	FollowUpFindingCount int
	WarningFindingCount  int
}
```

Change the task-done signature to:

```go
func reviewMarkdown(runID, target, contextRel, previousRun, gateEffect, verdict string, records []findings.Record, meta reviewMetadata) string
```

Build `meta` from `chain` and `counts` at the task-done call site. Pass `reconciled.OpenFindings` to `reviewMarkdown` so unresolved prior blockers remain visible in the review log. Update task-done `reviewerTable` so advisory, follow-up, and warning records do not become blocker rows:

```go
func reviewerTable(records []findings.Record) string {
	lines := make([]string, 0, len(reviewerNames))
	for _, reviewer := range reviewerNames {
		var blockers, nonBlockers []string
		for _, record := range records {
			if record.Reviewer != reviewer {
				continue
			}
			counts := findings.CountByClass([]findings.Record{record})
			if counts.Blocking > 0 {
				blockers = append(blockers, record.Title)
			} else {
				nonBlockers = append(nonBlockers, record.Title)
			}
		}
		verdict := "PASS"
		note := "No blocking findings."
		if len(blockers) > 0 {
			verdict = "NEEDS_REVISION"
			note = strings.Join(blockers, "; ")
		} else if len(nonBlockers) > 0 {
			verdict = "PASS_ADVISORY"
			note = strings.Join(nonBlockers, "; ")
		}
		lines = append(lines, fmt.Sprintf("| %s | %s | %d | %s |", reviewer, verdict, len(blockers), note))
	}
	return strings.Join(lines, "\n")
}
```

Replace the single `## Findings` section with `classifiedFindingsMarkdown(records)` that renders separate `## Blocking Findings`, `## Advisory Findings`, `## Follow-up Findings`, and `## Warnings` sections. Unknown classifications already persisted as `warning` must appear in `## Warnings`.

Append run-chain metadata in the same lifecycle builders before returning the Markdown:

```go
func runChainMarkdown(runID, verdict string, meta reviewMetadata) string {
	if verdict != "ESCALATED" {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("\n## Run Chain\n\n")
	for _, link := range meta.RunChain {
		builder.WriteString(fmt.Sprintf("- %s: %s attempt %d/%d\n", link.ID, link.Verdict, link.AttemptNumber, link.MaxAttempts))
	}
	builder.WriteString(fmt.Sprintf("- %s: %s attempt %d/%d\n", runID, verdict, meta.AttemptNumber, meta.MaxAttempts))
	builder.WriteString("\n## Unresolved Blocker Summary\n\n")
	builder.WriteString(fmt.Sprintf("- Blocking: %d\n- Advisory: %d\n- Follow-up: %d\n- Warnings: %d\n", meta.BlockingFindingCount, meta.AdvisoryFindingCount, meta.FollowUpFindingCount, meta.WarningFindingCount))
	return builder.String()
}
```

- [ ] **Step 5: Add failing epic-ready and pr-ready shell checks before wiring those gates**

Add compact first/second/third escalation checks to:

- `tests/go/test-epic-ready-review.sh`
- `tests/go/test-pr-ready-review.sh`

Add two fixtures to each script:

- A default-cap fixture with no `--max-attempts` flag: attempt 1 returns `NEEDS_REVISION` with `maxAttempts: 3`, attempt 2 returns `NEEDS_REVISION` with `maxAttempts: 3`, and attempt 3 returns `ESCALATED` with `status: escalated`.
- A compact `--max-attempts 2` fixture: the first run returns `NEEDS_REVISION`, the second run returns `ESCALATED`, and `previousRunId` is set to the first run.

For both fixtures, use the same state-integrity pattern from the task-done test: capture review count, context count, `.metareview/runs.jsonl` hash, and `.metareview/findings.jsonl` hash after escalation, then assert unchanged state for `--max-attempts 0`, missing `--previous-run`, mismatched-target `--previous-run`, same-target/different-scope `--previous-run`, `--previous-run` pointing to an `ESCALATED` run, `--previous-run` pointing to an older non-escalated ancestor after a later same-target `ESCALATED` run, and same-target restart after `ESCALATED`.

Run:

```bash
bash tests/go/test-epic-ready-review.sh
bash tests/go/test-pr-ready-review.sh
```

Expected: FAIL because epic-ready and pr-ready do not parse `--max-attempts`, do not preserve default `maxAttempts: 3` first/second/third behavior, do not write `previousRunId`, do not render run chains, do not preserve warning counts, do not fail before artifact creation on invalid, mismatched-scope, already-escalated, or older-ancestor-after-escalation previous-run state, and their Markdown builders have not yet been updated with review metadata.

- [ ] **Step 6: Mirror runchain wiring in epic-ready and pr-ready**

Apply the same `Options`, `Result`, `runRecord`, `runchain.Resolve`, `previousRunIDs` extraction, chain-aware `findings.Reconcile` options, verdict calculation from `findings.CountByClass(reconciled.OpenFindings)`, and count fields to:

- `internal/epicready/review.go` with scope `epic-ready`
- `internal/prready/review.go` with scope `pr-ready`

In both files, replace the existing `runRecord.PreviousRunID *string` field with `PreviousRunID string`, assign `PreviousRunID: options.PreviousRunID` directly, and remove the now-unused `optionalString` helper.

After adding `MaxAttempts int` to `internal/epicready.Options` and `internal/prready.Options`, extend the epic-ready and pr-ready option switches in `cmd/metareview/main.go` with:

```go
case "--max-attempts":
	options.MaxAttempts = mustPositiveInt(flagValue(args, i, "--max-attempts"), "--max-attempts")
	i++
```

Add the same `reviewMetadata` type, classified findings rendering, non-blocking `reviewerTable` behavior, and `runChainMarkdown` helper from task-done to both packages. Change the epic-ready signature to:

```go
func reviewMarkdown(runID, target, contextRel, previousRun, gateEffect, verdict string, records []findings.Record, meta reviewMetadata) string
```

Change the pr-ready signature to:

```go
func reviewMarkdown(runID, contextRel, previousRun, gateEffect, verdict string, records []findings.Record, prEvidence string, meta reviewMetadata) string
```

Build `meta` from the resolved chain and open-finding counts at both epic-ready and pr-ready call sites before writing review Markdown. Pass `reconciled.OpenFindings` to the Markdown builders so unresolved blockers from earlier runs in the same target remain visible and continue to block.

For pr-ready, target stays:

```go
targetRecord := map[string]string{"type": "branch", "id": firstNonEmpty(git.Branch, git.HeadSHA)}
```

For pr-ready, use a two-phase evidence flow:

1. Before reviewers run, build `preReviewEvidence := RenderEvidence(...)` from prior task/epic logs, unresolved blockers, validation evidence, and GitHub context. Pass this to `reviewerContext` and `contextMarkdown`.
2. After `findings.Reconcile` and verdict/count calculation, build `finalPREvidence := RenderEvidence(...)` with the same prior evidence plus `CurrentReview` populated from the actual current run ID, verdict, review path, blocker state, attempt number, max attempts, and finding counts.
3. Write `finalPREvidence` into the final pr-ready review log. The context pack can keep `preReviewEvidence` because it is reviewer input, not final evidence.

- [ ] **Step 7: Run task-done GREEN**

Run:

```bash
bash tests/go/test-task-done-review.sh
```

Expected: PASS.

- [ ] **Step 8: Run lifecycle shell tests**

Run:

```bash
bash tests/go/test-task-done-review.sh
bash tests/go/test-epic-ready-review.sh
bash tests/go/test-pr-ready-review.sh
go test ./internal/taskdone ./internal/epicready ./internal/prready
```

Expected: all PASS.

- [ ] **Step 9: Commit**

```bash
git add cmd/metareview/main.go internal/taskdone/review.go internal/taskdone/review_markdown_test.go internal/epicready/review.go internal/epicready/review_markdown_test.go internal/prready/review.go internal/prready/review_markdown_test.go tests/go/test-task-done-review.sh tests/go/test-epic-ready-review.sh tests/go/test-pr-ready-review.sh
git commit -m "feat: bound metareview lifecycle gate attempts"
```

### Task 4: Review Log Metadata And PR Evidence

**Files:**
- Modify: `internal/reviewlog/reviewlog.go`
- Modify: `internal/reviewlog/reviewlog_test.go`
- Modify: `internal/prready/evidence.go`
- Modify: `internal/prready/evidence_test.go`
- Modify: `tests/go/test-epic-ready-review.sh`
- Modify: `tests/go/test-pr-ready-review.sh`

- [ ] **Step 1: Write failing reviewlog tests**

Add to `internal/reviewlog/reviewlog_test.go`:

```go
// Add strings to the existing import block.

func TestEscalatedVerdictIsUnresolved(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "ESCALATED", ""))

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs) != 1 || !logs[0].HasUnresolvedBlockers {
		t.Fatalf("expected ESCALATED to be unresolved: %+v", logs)
	}
}

func TestDiscoverMergesRunAttemptMetadata(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "ESCALATED", ""))
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"),
		`{"id":"mrv-root","scope":"task-done","target":{"type":"path","id":"task-1"},"verdict":"NEEDS_REVISION","attemptNumber":1,"maxAttempts":3}`+"\n"+
			`{"id":"mrv-task","scope":"task-done","target":{"type":"path","id":"task-1"},"verdict":"ESCALATED","previousRunId":"mrv-root","attemptNumber":3,"maxAttempts":3,"blockingFindingCount":1,"advisoryFindingCount":2,"followUpFindingCount":1}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	log := logs[0]
	if log.AttemptNumber != 3 || log.MaxAttempts != 3 || log.BlockingFindingCount != 1 || log.AdvisoryFindingCount != 2 || log.FollowUpFindingCount != 1 {
		t.Fatalf("expected run metadata merged into summary: %+v", log)
	}
	if len(log.RunChain) != 2 || log.RunChain[0].ID != "mrv-root" || log.RunChain[1].ID != "mrv-task" {
		t.Fatalf("expected full run chain in summary: %+v", log.RunChain)
	}
}

func TestDiscoverSurfacesUnknownClassificationWarning(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "metareview", "reviews", "task.md"), reviewMarkdown("mrv-task", "task-1", "PASS_ADVISORY", ""))
	mustWrite(t, filepath.Join(root, ".metareview", "runs.jsonl"), `{"id":"mrv-task","attemptNumber":1,"maxAttempts":3,"warningFindingCount":1}`+"\n")

	logs, err := ForTarget(root, "task-1")
	if err != nil {
		t.Fatalf("target logs: %v", err)
	}
	if len(logs[0].Warnings) != 1 || !strings.Contains(logs[0].Warnings[0], "unknown finding classification") {
		t.Fatalf("expected unknown-classification warning in review log summary: %+v", logs[0].Warnings)
	}
}
```

Append this gate-boundary regression to `tests/go/test-epic-ready-review.sh` after the existing `incomplete-child-artifact` scenario:

```bash
repo="$TMP/escalated-child"
mkdir -p "$repo/.beads" "$repo/docs/metareview/reviews"
cd "$repo"
git init -q
git config user.email test-user
git config user.name "Test User"
{
  write_issue '{"id":"epic-escalated","title":"Escalated child blockers","description":"Must not pass while a child is escalated.","children":["task-1"]}'
  write_issue '{"id":"task-1","title":"Child task","description":"Implement child safely."}'
} > .beads/issues.jsonl
cat > docs/metareview/reviews/child-escalated.md <<'REVIEW'
# metareview: task-done review

Run ID: `mrv-child-escalated`

Target: `task-1`

## Verdict

ESCALATED

## Findings

### mrvf-child-escalated-001: Existing escalated blocker
REVIEW
printf "initial\n" > docs/change.md
git add .
git commit -qm "initial"
base="$(git rev-parse HEAD)"
printf "updated\n" > docs/change.md
git add .
git commit -qm "change"

set +e
"$TMP/metareview" review epic-ready epic-escalated --base "$base" > "$TMP/escalated-child.out" 2>"$TMP/escalated-child.err"
code=$?
set -e
test "$code" -eq 1
escalated_child_review="$(cat "$TMP/escalated-child.out")"
grep -q "Unresolved child blockers" "$repo/$escalated_child_review"
grep -q "ESCALATED" "$repo/$escalated_child_review"
```

Append this gate-boundary regression to `tests/go/test-pr-ready-review.sh` after the existing `unresolved` scenario:

```bash
repo="$TMP/escalated-review"
init_repo "$repo"
base="$(git rev-parse HEAD)"
mkdir -p docs/metareview/reviews
cat > docs/metareview/reviews/task-escalated.md <<'REVIEW'
# metareview: task-done review

Run ID: `mrv-task-escalated`

Target: `task-1`

## Verdict

ESCALATED

## Findings

### mrvf-task-escalated-001: Existing escalated blocker
REVIEW
printf "'use strict';\nmodule.exports = input => JSON.parse(input); // escalated branch\n" > lib/parser.js
git add .
git commit -qm "branch change"
printf "bash tests/run-all.sh exited 0\n" > "$TMP/escalated-evidence.md"
set +e
"$TMP/metareview" review pr-ready --base "$base" --evidence "$TMP/escalated-evidence.md" > "$TMP/escalated-review.out" 2>"$TMP/escalated-review.err"
code=$?
set -e
test "$code" -eq 1
escalated_review="$(cat "$TMP/escalated-review.out")"
grep -q "Unresolved review blockers" "$repo/$escalated_review"
grep -q "ESCALATED" "$repo/$escalated_review"
```

- [ ] **Step 2: Run reviewlog RED**

Run:

```bash
go test ./internal/reviewlog
bash tests/go/test-epic-ready-review.sh
bash tests/go/test-pr-ready-review.sh
```

Expected: FAIL because `ESCALATED` is not unresolved, downstream gates do not yet fail on child/prior `ESCALATED` logs, summary metadata fields do not exist, run-chain summaries are not built, and unknown-classification warnings are not surfaced.

- [ ] **Step 3: Extend reviewlog summary and run metadata merge**

In `internal/reviewlog/reviewlog.go`, extend `Summary`:

```go
	AttemptNumber        int `json:"attemptNumber,omitempty"`
	MaxAttempts          int `json:"maxAttempts,omitempty"`
	BlockingFindingCount int `json:"blockingFindingCount,omitempty"`
	AdvisoryFindingCount int `json:"advisoryFindingCount,omitempty"`
	FollowUpFindingCount int `json:"followUpFindingCount,omitempty"`
	WarningFindingCount  int `json:"warningFindingCount,omitempty"`
	RunChain             []RunLink `json:"runChain,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
```

Add run-link summary type:

```go
type RunLink struct {
	ID            string `json:"id"`
	Verdict       string `json:"verdict"`
	AttemptNumber int    `json:"attemptNumber"`
	MaxAttempts   int    `json:"maxAttempts"`
}
```

Extend `verdictIsUnresolved`:

```go
case "", "NOT_REVIEWED", "ESCALATE", "ESCALATED", "NEEDS_REVISION":
	return true
```

Import the shared run-chain package instead of defining a second run-record schema:

```go
	"github.com/dsifry/metareview/internal/runchain"
```

In `Discover`, read runs once and merge by `RunID`:

```go
runs, err := runchain.ReadRuns(root)
if err != nil {
	return nil, err
}
...
mergeRunMetadata(&summary, runs)
```

`mergeRunMetadata` must call `runchain.ChainTo(runs, summary.RunID)`, reject cycles by returning a warning instead of panicking, attach `RunChain` root-to-leaf, copy attempt/count metadata from the current run, and append a generic non-blocking warning summary when `WarningFindingCount > 0`.

- [ ] **Step 4: Write failing PR evidence tests**

Append to `internal/prready/evidence_test.go`:

```go
func TestRenderEvidenceIncludesAttemptCountsAndEscalation(t *testing.T) {
	body := RenderEvidence(EvidenceInput{
		Summary:    "branch summary",
		Validation: []string{"go test ./... exited 0"},
		TaskReviews: []ReviewEvidence{{
			Target:                "task-1",
			Verdict:               "ESCALATED",
			Path:                  "docs/metareview/reviews/task.md",
			HasUnresolvedBlockers: true,
			AttemptNumber:         3,
			MaxAttempts:           3,
			BlockingFindingCount:  1,
			AdvisoryFindingCount:  2,
			FollowUpFindingCount:  1,
		}},
		CurrentReview: &ReviewEvidence{
			Target:                "current branch",
			Verdict:               "ESCALATED",
			Path:                  "docs/metareview/reviews/pr.md",
			HasUnresolvedBlockers: true,
			AttemptNumber:         2,
			MaxAttempts:           2,
			BlockingFindingCount:  1,
			AdvisoryFindingCount:  1,
		},
	})
	if !strings.Contains(body, "task-1: ESCALATED with unresolved blockers attempt 3/3 findings: blocking 1, advisory 2, follow-up 1") ||
		!strings.Contains(body, "current branch: ESCALATED with unresolved blockers attempt 2/2 findings: blocking 1, advisory 1, follow-up 0") {
		t.Fatalf("expected attempt count and escalation in evidence:\n%s", body)
	}
}
```

- [ ] **Step 5: Run PR evidence RED**

Run:

```bash
go test ./internal/prready
```

Expected: FAIL because `ReviewEvidence` lacks attempt fields and renderer omits counts.

- [ ] **Step 6: Extend PR evidence structs and renderer**

In `internal/prready/evidence.go`, extend `ReviewEvidence`:

```go
	AttemptNumber        int
	MaxAttempts          int
	BlockingFindingCount int
	AdvisoryFindingCount int
	FollowUpFindingCount int
```

Extend `EvidenceInput`:

```go
	CurrentReview *ReviewEvidence
```

Update `FromReviewLog`:

```go
		AttemptNumber:        log.AttemptNumber,
		MaxAttempts:          log.MaxAttempts,
		BlockingFindingCount: log.BlockingFindingCount,
		AdvisoryFindingCount: log.AdvisoryFindingCount,
		FollowUpFindingCount: log.FollowUpFindingCount,
```

Update `reviewList`:

```go
if review.AttemptNumber > 0 && review.MaxAttempts > 0 {
	status += fmt.Sprintf(" attempt %d/%d", review.AttemptNumber, review.MaxAttempts)
}
if review.BlockingFindingCount > 0 || review.AdvisoryFindingCount > 0 || review.FollowUpFindingCount > 0 {
	status += fmt.Sprintf(" findings: blocking %d, advisory %d, follow-up %d", review.BlockingFindingCount, review.AdvisoryFindingCount, review.FollowUpFindingCount)
}
```

Update `RenderEvidence` to render the actual current run when present:

```go
if input.CurrentReview != nil {
	builder.WriteString("### Current PR Review\n\n")
	builder.WriteString(reviewList([]ReviewEvidence{*input.CurrentReview}, "No current review evidence.") + "\n\n")
}
```

Add import:

```go
import "fmt"
```

- [ ] **Step 7: Run GREEN**

Run:

```bash
go test ./internal/reviewlog ./internal/prready
bash tests/go/test-epic-ready-review.sh
bash tests/go/test-pr-ready-review.sh
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/reviewlog/reviewlog.go internal/reviewlog/reviewlog_test.go internal/prready/evidence.go internal/prready/evidence_test.go tests/go/test-epic-ready-review.sh tests/go/test-pr-ready-review.sh
git commit -m "feat: expose escalated review metadata in PR evidence"
```

### Task 5: Documentation And Skills

**Files:**
- Modify: `rubrics/task-done-review-rubric.md`
- Modify: `rubrics/epic-ready-review-rubric.md`
- Modify: `rubrics/pr-ready-review-rubric.md`
- Modify: `skills/review-task-done/SKILL.md`
- Modify: `skills/review-epic-ready/SKILL.md`
- Modify: `skills/review-pr-ready/SKILL.md`
- Modify: `docs/quickstart.md`
- Modify: `docs/README.codex.md`
- Modify: `docs/README.claude.md`
- Modify: `INSTALL.md`
- Modify: `docs/integrations/metaswarm.md`
- Modify: `tests/manifest/test-skills.sh`
- Modify: `tests/manifest/test-manifests.sh`
- Modify: `tests/go/test-cli-baseline.sh`

- [ ] **Step 1: Write failing manifest assertions**

In `tests/manifest/test-skills.sh`, add checks:

```bash
grep -q -- '--max-attempts' skills/review-task-done/SKILL.md
grep -q 'ESCALATED is blocking' skills/review-task-done/SKILL.md
grep -q 'same scope and target' skills/review-task-done/SKILL.md
grep -q 'Advisory and follow-up findings do not extend the repair loop' skills/review-pr-ready/SKILL.md
grep -q 'new blockers may only cover the original contract, unresolved prior blockers, or regressions from the latest fix' rubrics/task-done-review-rubric.md
grep -q 'new blockers may only cover the original contract, unresolved prior blockers, or regressions from the latest fix' rubrics/epic-ready-review-rubric.md
grep -q 'new blockers may only cover the original contract, unresolved prior blockers, or regressions from the latest fix' rubrics/pr-ready-review-rubric.md
```

In `tests/manifest/test-manifests.sh`, add checks:

```bash
for doc in docs/quickstart.md docs/README.codex.md docs/README.claude.md INSTALL.md docs/integrations/metaswarm.md; do
  grep -q 'ESCALATED' "$doc"
  grep -q 'max attempts' "$doc"
done
grep -q 'validationAttemptNumber' docs/integrations/metaswarm.md
grep -q 'maxValidationAttempts' docs/integrations/metaswarm.md
grep -q 'latest validation command and exit code' docs/integrations/metaswarm.md
grep -q 'latest failing evidence path or log excerpt' docs/integrations/metaswarm.md
grep -q 'ESCALATED work-unit status' docs/integrations/metaswarm.md
```

In `tests/go/test-cli-baseline.sh`, add:

```bash
printf '%s\n' "$help" | grep -q -- '--max-attempts <n>'
```

- [ ] **Step 2: Run RED**

Run:

```bash
bash tests/manifest/test-skills.sh
bash tests/manifest/test-manifests.sh
bash tests/go/test-cli-baseline.sh
```

Expected: FAIL because docs and rubrics do not yet mention bounded attempts or re-review scope limits, and CLI help does not yet show `--max-attempts <n>`.

- [ ] **Step 3: Update skill command blocks**

In the three review skills, update command syntax to include:

```bash
[--max-attempts <n>]
```

Add required workflow text:

```markdown
Re-run blockers with `--previous-run`, but stop after the configured max attempts. `ESCALATED` is blocking and is not a pass. A same scope and target cannot restart after `ESCALATED`; continuing requires a new task/spec target. Advisory and follow-up findings do not extend the repair loop unless the human creates a new target with widened scope.
```

In `cmd/metareview/main.go`, update the lifecycle usage lines to include `--max-attempts <n>`:

```go
metareview review task-done <task-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
metareview review epic-ready <epic-id-or-path> [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>]
metareview review pr-ready [--base <ref>] [--previous-run <run-id>] [--max-attempts <n>] [--evidence <path>] [--github-pr <number>]
```

- [ ] **Step 4: Update reviewer rubrics and docs**

In each review rubric, add this re-review guard:

```markdown
On re-review, new blockers may only cover the original contract, unresolved prior blockers, or regressions from the latest fix. Do not expand the target with unrelated improvements, style preferences, adjacent refactors, or newly imagined scope; record those as advisory or follow-up findings instead.
```

Then add the same bounded-loop language to:

- `docs/quickstart.md`
- `docs/README.codex.md`
- `docs/README.claude.md`
- `INSTALL.md`
- `docs/integrations/metaswarm.md`

For `docs/integrations/metaswarm.md`, also state:

```markdown
Metareview caps review-gate attempts. Metaswarm owns validation-only attempt tracking in execution state, with the same default cap of 3.

When validation fails before a metareview gate runs, metaswarm should persist these execution-state fields in its own state file, such as `.beads/context/execution-state.md`:

- `validationAttemptNumber`
- `maxValidationAttempts`, default `3`
- latest validation command and exit code
- latest failing evidence path or log excerpt
- `ESCALATED` work-unit status when validation failures exhaust the budget
```

- [ ] **Step 5: Run GREEN**

Run:

```bash
bash tests/manifest/test-skills.sh
bash tests/manifest/test-manifests.sh
bash tests/go/test-cli-baseline.sh
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/metareview/main.go rubrics/task-done-review-rubric.md rubrics/epic-ready-review-rubric.md rubrics/pr-ready-review-rubric.md skills/review-task-done/SKILL.md skills/review-epic-ready/SKILL.md skills/review-pr-ready/SKILL.md docs/quickstart.md docs/README.codex.md docs/README.claude.md INSTALL.md docs/integrations/metaswarm.md tests/manifest/test-skills.sh tests/manifest/test-manifests.sh tests/go/test-cli-baseline.sh
git commit -m "docs: document bounded metareview repair loops"
```

### Task 6: Full Verification And Metareview Gates

**Files:**
- No source file changes unless verification reveals a defect.
- Generated review artifacts under `docs/metareview/context/` and `docs/metareview/reviews/`.

- [ ] **Step 1: Run full test suite**

Run:

```bash
go test ./...
bash tests/run-all.sh
git diff --check
```

Expected:

- `go test ./...` exits 0
- `bash tests/run-all.sh` exits 0
- `git diff --check` exits 0

- [ ] **Step 2: Create evidence file**

Run:

```bash
tmp_evidence="$(mktemp)"
printf '%s\n' \
  'Verification evidence:' \
  '- go test ./... exited 0' \
  '- bash tests/run-all.sh exited 0' \
  '- git diff --check exited 0' > "$tmp_evidence"
```

- [ ] **Step 3: Run task-done review**

Run:

```bash
go run ./cmd/metareview review task-done docs/specs/2026-05-28-bounded-review-fsm-escalation.md --base origin/main --evidence "$tmp_evidence"
```

Expected: exits 0 and generated review log verdict is `PASS` or `PASS_ADVISORY` with zero blockers.

- [ ] **Step 4: Run PR-ready review**

Run:

```bash
go run ./cmd/metareview review pr-ready --base origin/main --evidence "$tmp_evidence"
```

Expected: exits 0 and generated review log verdict is `PASS` or `PASS_ADVISORY` with zero blockers.

- [ ] **Step 5: Commit generated review artifacts**

```bash
git add docs/metareview/context docs/metareview/reviews
git commit -m "chore: record bounded FSM review evidence"
```

Do not add `.metareview/findings.jsonl`, `.metareview/runs.jsonl`, generated binaries, or `docs/metareview/FINDINGS.md` unless a separate intentional documentation change modifies an already tracked durable findings summary.
