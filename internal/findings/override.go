package findings

import (
	"fmt"
	"sort"
	"strings"
)

// Override statuses. A finding is either open, fixed, or has left the normal
// workflow: override-pending records that an out-of-workflow escalation happened
// and still blocks; overridden records that an authority outside the workflow
// acknowledged it and stops blocking.
const (
	StatusOverridePending = "override-pending"
	StatusOverridden      = "overridden"
)

// minOverrideReason is the shortest reason worth recording. An override that
// cannot be explained in a sentence is not an override, it is a shrug.
const minOverrideReason = 12

// Storage seam. The override commands read and write findings through these so
// their failure paths are testable without depending on filesystem permissions
// (which do not fail for a root test runner).
var (
	loadRecords = readJSONL
	saveRecords = writeJSONL
)

// OverrideRequest is filed by whoever stepped outside the workflow — typically
// the orchestrating agent, which may request but never grant.
//
// By is audit metadata: it records who claims to have acted, and nothing here
// authenticates it. The enforceable half of the boundary is that the actor who
// requested an exception cannot also acknowledge it (see GrantOverride).
type OverrideRequest struct {
	By         string
	Reason     string
	Escalation string
	Now        string
	// Subject is the row to add when neither the ledger nor a committed review log knows the ID: an abandoned FSM run's
	// closure (#179, AbandonedRunRecord). Nil for a finding.
	Subject *Record
}

// OverrideGrant is the acknowledgement from outside the workflow.
type OverrideGrant struct {
	By     string
	Reason string
	Now    string
	// Subject is as OverrideRequest's.
	Subject *Record
}

// Blocks reports whether a status still holds a gate closed. A pending override
// blocks by design: the workflow was stepped outside of and nobody has
// acknowledged it yet.
func Blocks(status string) bool {
	return status == "open" || status == StatusOverridePending
}

// RequestOverride marks a finding as a recorded, unacknowledged process exception.
func RequestOverride(root, findingID string, request OverrideRequest) error {
	if strings.TrimSpace(request.By) == "" {
		return fmt.Errorf("override request needs an actor (--by)")
	}
	if len(strings.TrimSpace(request.Reason)) < minOverrideReason {
		return fmt.Errorf("override request needs a reason of at least %d characters", minOverrideReason)
	}
	now := strings.TrimSpace(request.Now)
	if now == "" {
		return fmt.Errorf("override request needs a timestamp")
	}
	return mutateFinding(root, findingID, now, request.Subject, func(record *Record) error {
		// A FIXED finding enters the two-phase flow only when the request references the
		// escalation whose hard stop it asks to lift (request.Escalation — issue #147):
		// the run-level stop can outlive the finding-level fix, and the recorded request
		// is what makes the later grant two-phase (requester ≠ grantor).
		fixedWithEscalation := (record.Status == "fixed" || supersededFreshness(*record)) && strings.TrimSpace(request.Escalation) != ""
		reclose := staleClosure(*record, request.Subject)
		if record.Status != "open" && !fixedWithEscalation && !reclose {
			return fmt.Errorf("finding %s is %s, not open", findingID, record.Status)
		}
		if reclose || IsRunClosure(*record) && request.Subject != nil {
			record.RunUpdated = request.Subject.RunUpdated
			record.OverrideGrantedBy, record.OverrideGrantedAt, record.OverrideGrantReason = "", "", ""
		}
		record.Status = StatusOverridePending
		record.OverrideRequestedBy = strings.TrimSpace(request.By)
		record.OverrideRequestReason = strings.TrimSpace(request.Reason)
		record.OverrideRequestedAt = now
		record.OverrideEscalation = strings.TrimSpace(request.Escalation)
		record.UpdatedAt = now
		return nil
	})
}

// GrantOverride acknowledges the exception. It accepts an open finding directly
// (a human overriding without a prior agent escalation), a pending request
// filed by someone else, or a FIXED finding whose run-level escalation persists —
// lifting that escalation is a human decision the grant records (issue #147: the
// run's hard stop outlives the finding-level fix). It refuses a grant from the
// actor that requested it: requesting and acknowledging are separate roles by
// design.
//
// By is audit metadata, not authentication — a local CLI has no authority to
// verify an identity — so this enforces the boundary against the accidental
// case, not against an actor that deliberately misreports itself.
func GrantOverride(root, findingID string, grant OverrideGrant) error {
	if strings.TrimSpace(grant.By) == "" {
		return fmt.Errorf("override grant needs an actor (--by)")
	}
	if len(strings.TrimSpace(grant.Reason)) < minOverrideReason {
		return fmt.Errorf("override grant needs a reason of at least %d characters", minOverrideReason)
	}
	by := strings.TrimSpace(grant.By)
	now := strings.TrimSpace(grant.Now)
	if now == "" {
		return fmt.Errorf("override grant needs a timestamp")
	}
	return mutateFinding(root, findingID, now, grant.Subject, func(record *Record) error {
		// A FIXED finding enters the two-phase flow only when a REQUEST referencing the
		// escalation was already filed (record.OverrideEscalation — issue #147): the
		// run-level stop can outlive the finding-level fix, and lifting it is the human
		// decision the grant records — with requester ≠ grantor enforced below.
		fixedWithEscalation := (record.Status == "fixed" || supersededFreshness(*record)) && strings.TrimSpace(record.OverrideEscalation) != ""
		reclose := staleClosure(*record, grant.Subject)
		if record.Status != "open" && !fixedWithEscalation && record.Status != StatusOverridePending && !reclose {
			return fmt.Errorf("finding %s is %s and cannot be overridden", findingID, record.Status)
		}
		if reclose {
			// The run moved on since its last closure: this grant is a new decision, not a second acknowledgement.
			record.OverrideRequestedBy, record.OverrideRequestedAt, record.OverrideRequestReason = "", "", ""
		}
		if strings.EqualFold(by, record.OverrideRequestedBy) {
			return fmt.Errorf("%s requested this override and cannot also grant it; acknowledgement comes from outside the workflow", by)
		}
		if IsRunClosure(*record) && grant.Subject != nil {
			record.RunUpdated = grant.Subject.RunUpdated // the grant closes the run as it stands now
		}
		record.Status = StatusOverridden
		record.OverrideGrantedBy = by
		record.OverrideGrantReason = strings.TrimSpace(grant.Reason)
		record.OverrideGrantedAt = now
		record.UpdatedAt = now
		return nil
	})
}

// ListOverrides returns every requested or granted override, newest first.
func ListOverrides(root string) ([]Record, error) {
	records, err := loadRecords(findingsPath(root))
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0)
	for _, record := range records {
		if record.Status == StatusOverridePending || record.Status == StatusOverridden {
			out = append(out, record)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// PendingOverrides returns the exceptions nobody has acknowledged yet. CI should
// refuse to pass while this is non-empty.
func PendingOverrides(root string) ([]Record, error) {
	all, err := ListOverrides(root)
	if err != nil {
		return nil, err
	}
	pending := make([]Record, 0, len(all))
	for _, record := range all {
		if record.Status == StatusOverridePending {
			pending = append(pending, record)
		}
	}
	return pending, nil
}

// mutateFinding applies an override transition to one ledger row. The blockers of the committed review logs that list the
// finding and are missing from the ledger — the finding itself, when it exists only in those logs — are imported first
// (#188), so the escalation path reaches every blocker the gates read and no grant retires a blocker nobody saw; nothing
// is written unless the transition applies. An ID nothing knows takes subject, when it names that ID.
func mutateFinding(root, findingID, now string, subject *Record, apply func(*Record) error) error {
	path := findingsPath(root)
	records, err := loadRecords(path)
	if err != nil {
		return err
	}
	index := -1
	for i, record := range records {
		if record.ID == findingID {
			index = i
			break
		}
	}
	known := make(map[string]bool, len(records))
	for _, record := range records {
		known[record.ID] = true
	}
	imported, err := committedFindings(root, findingID, now, known)
	if err != nil {
		return err
	}
	if index < 0 && (len(imported) == 0 || imported[0].ID != findingID) {
		if subject == nil || subject.ID != findingID {
			return fmt.Errorf("finding %s not found", findingID)
		}
		imported = []Record{*subject}
	}
	if index < 0 {
		index = len(records)
	}
	records = append(records, imported...)
	if err := apply(&records[index]); err != nil {
		return err
	}
	if err := saveRecords(path, records); err != nil {
		return err
	}
	return RenderIndexWithRecords(root, records)
}

// supersededFreshness: a freshness row that fresh evidence replaced. Like a fixed row, it takes an
// override only with an escalation, so a stale-only escalation (spec §6.8) can still be lifted.
func supersededFreshness(record Record) bool {
	return record.Status == StatusSuperseded && IsFreshnessFingerprint(record.Fingerprint)
}

// overrideLines renders the process-exception section of the findings index.
func overrideLines(records []Record) []string {
	var lines []string
	for _, record := range records {
		// Free-text fields are flattened to the canonical single physical line every emitted
		// index entry uses: the committed index is re-read line-by-line by carryOverLines, so
		// an embedded newline in a title or reason would make one entry span lines and only
		// its first line carry back (see the findings.go blocker-bullet comment).
		title, reqReason := singleLine(record.Title), singleLine(record.OverrideRequestReason)
		reqBy, grantedBy := singleLine(record.OverrideRequestedBy), singleLine(record.OverrideGrantedBy)
		reqAt, grantedAt := singleLine(record.OverrideRequestedAt), singleLine(record.OverrideGrantedAt)
		switch record.Status {
		case StatusOverridePending:
			lines = append(lines, withEscalation(fmt.Sprintf("- %s [pending] %s — requested by %s at %s: %s",
				record.ID, title, reqBy, reqAt, reqReason), record))
		case StatusSuperseded:
			if record.OverrideRequestedBy != "" {
				lines = append(lines, withEscalation(fmt.Sprintf("- %s [superseded] %s — requested by %s at %s: %s",
					record.ID, title, reqBy, reqAt, reqReason), record))
			}
		case StatusOverridden:
			detail := fmt.Sprintf("- %s [granted] %s — granted by %s at %s: %s",
				record.ID, title, grantedBy, grantedAt, singleLine(record.OverrideGrantReason))
			if record.OverrideRequestedBy != "" {
				detail += fmt.Sprintf(" (requested by %s at %s: %s)",
					reqBy, reqAt, reqReason)
			}
			lines = append(lines, withEscalation(detail, record))
		}
	}
	return lines
}

// withEscalation appends the escalation context when the record carries one, so
// the index shows why the workflow was stepped outside of and not just that it was. The
// escalation is free text (a CLI --escalation value) and is flattened to the canonical
// single physical line every emitted entry uses — see the findings.go blocker-bullet comment.
func withEscalation(detail string, record Record) string {
	if record.OverrideEscalation == "" {
		return detail
	}
	return detail + fmt.Sprintf(" [escalation: %s]", singleLine(record.OverrideEscalation))
}

// AbandonedRunFingerprintPrefix marks the ledger row that closes an abandoned FSM run (#179).
const AbandonedRunFingerprintPrefix = "fsm:abandoned-run:"

// AbandonedRunRecord is the ledger row through which an abandoned FSM run is closed (#179): the run's ID, taken through
// the ordinary override flow — a request does not close it, the requester cannot grant it, a grant needs a reason — and
// rendered under Process Overrides. It is bookkeeping, not a finding: advisory, so no review gate counts it, while the
// run itself keeps blocking `status` until the grant. Branch and head are the run's own (its init), so the row is
// scoped with the run. updated is the run's last event: a grant closes the run only while it is still there.
func AbandonedRunRecord(runID, description, branch, head, updated, now string) Record {
	return Record{
		SchemaVersion:  1,
		ID:             runID,
		RunID:          runID,
		Scope:          "fsm-run",
		Reviewer:       "fsm",
		Severity:       "low",
		Classification: "advisory",
		Status:         "open",
		Title:          "Abandoned FSM run " + description,
		Fingerprint:    AbandonedRunFingerprintPrefix + runID,
		Target:         map[string]string{"type": "fsm-run", "id": runID},
		CreatedAt:      now,
		UpdatedAt:      now,
		GitHead:        head,
		Branch:         branch,
		RunUpdated:     updated,
	}
}

// IsRunClosure reports whether a ledger row is an abandoned FSM run's closure row.
func IsRunClosure(record Record) bool {
	return strings.HasPrefix(record.Fingerprint, AbandonedRunFingerprintPrefix) && record.ID == strings.TrimPrefix(record.Fingerprint, AbandonedRunFingerprintPrefix)
}

// staleClosure reports whether record is a run closure (#179), granted or requested, whose run has moved on since —
// resumed, and left again — so it may be requested or granted afresh: the old grant no longer closes the run (without
// this the row could never be reopened), and an old request no longer describes it, so a grant is taken as a fresh,
// direct decision on the run as it now stands rather than as the acknowledgement of a request about another state.
func staleClosure(record Record, subject *Record) bool {
	return IsRunClosure(record) && (record.Status == StatusOverridden || record.Status == StatusOverridePending) &&
		subject != nil && subject.ID == record.ID && subject.RunUpdated != record.RunUpdated
}
