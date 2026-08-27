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
}

// OverrideGrant is the acknowledgement from outside the workflow.
type OverrideGrant struct {
	By     string
	Reason string
	Now    string
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
	return mutateFinding(root, findingID, func(record *Record) error {
		if record.Status != "open" {
			return fmt.Errorf("finding %s is %s, not open", findingID, record.Status)
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
// (a human overriding without a prior agent escalation) or a pending request
// filed by someone else. It refuses a grant from the actor that requested it:
// requesting and acknowledging are separate roles by design.
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
	return mutateFinding(root, findingID, func(record *Record) error {
		if record.Status != "open" && record.Status != StatusOverridePending {
			return fmt.Errorf("finding %s is %s and cannot be overridden", findingID, record.Status)
		}
		if strings.EqualFold(by, record.OverrideRequestedBy) {
			return fmt.Errorf("%s requested this override and cannot also grant it; acknowledgement comes from outside the workflow", by)
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

func mutateFinding(root, findingID string, apply func(*Record) error) error {
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
	if index < 0 {
		return fmt.Errorf("finding %s not found", findingID)
	}
	if err := apply(&records[index]); err != nil {
		return err
	}
	if err := saveRecords(path, records); err != nil {
		return err
	}
	return RenderIndexWithRecords(root, records)
}

// overrideLines renders the process-exception section of the findings index.
func overrideLines(records []Record) []string {
	var lines []string
	for _, record := range records {
		switch record.Status {
		case StatusOverridePending:
			lines = append(lines, withEscalation(fmt.Sprintf("- %s [pending] %s — requested by %s at %s: %s",
				record.ID, record.Title, record.OverrideRequestedBy, record.OverrideRequestedAt, record.OverrideRequestReason), record))
		case StatusOverridden:
			detail := fmt.Sprintf("- %s [granted] %s — granted by %s at %s: %s",
				record.ID, record.Title, record.OverrideGrantedBy, record.OverrideGrantedAt, record.OverrideGrantReason)
			if record.OverrideRequestedBy != "" {
				detail += fmt.Sprintf(" (requested by %s at %s: %s)",
					record.OverrideRequestedBy, record.OverrideRequestedAt, record.OverrideRequestReason)
			}
			lines = append(lines, withEscalation(detail, record))
		}
	}
	return lines
}

// withEscalation appends the escalation context when the record carries one, so
// the index shows why the workflow was stepped outside of and not just that it was.
func withEscalation(detail string, record Record) string {
	if record.OverrideEscalation == "" {
		return detail
	}
	return detail + fmt.Sprintf(" [escalation: %s]", record.OverrideEscalation)
}
