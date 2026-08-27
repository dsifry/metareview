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

// OverrideRequest is filed by whoever stepped outside the workflow — typically
// the orchestrating agent, which may request but never grant.
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
	return mutateFinding(root, findingID, func(record *Record) error {
		if record.Status != "open" {
			return fmt.Errorf("finding %s is %s, not open", findingID, record.Status)
		}
		record.Status = StatusOverridePending
		record.OverrideRequestedBy = strings.TrimSpace(request.By)
		record.OverrideRequestReason = strings.TrimSpace(request.Reason)
		record.OverrideRequestedAt = request.Now
		record.OverrideEscalation = strings.TrimSpace(request.Escalation)
		record.UpdatedAt = request.Now
		return nil
	})
}

// GrantOverride acknowledges the exception. It accepts an open finding directly
// (a human overriding without a prior agent escalation) or a pending request.
func GrantOverride(root, findingID string, grant OverrideGrant) error {
	if strings.TrimSpace(grant.By) == "" {
		return fmt.Errorf("override grant needs an actor (--by)")
	}
	if len(strings.TrimSpace(grant.Reason)) < minOverrideReason {
		return fmt.Errorf("override grant needs a reason of at least %d characters", minOverrideReason)
	}
	return mutateFinding(root, findingID, func(record *Record) error {
		if record.Status != "open" && record.Status != StatusOverridePending {
			return fmt.Errorf("finding %s is %s and cannot be overridden", findingID, record.Status)
		}
		record.Status = StatusOverridden
		record.OverrideGrantedBy = strings.TrimSpace(grant.By)
		record.OverrideGrantReason = strings.TrimSpace(grant.Reason)
		record.OverrideGrantedAt = grant.Now
		record.UpdatedAt = grant.Now
		return nil
	})
}

// ListOverrides returns every requested or granted override, newest first.
func ListOverrides(root string) ([]Record, error) {
	records, err := readJSONL(findingsPath(root))
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
	records, err := readJSONL(path)
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
	if err := writeJSONL(path, records); err != nil {
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
			lines = append(lines, fmt.Sprintf("- %s [pending] %s — requested by %s: %s",
				record.ID, record.Title, record.OverrideRequestedBy, record.OverrideRequestReason))
		case StatusOverridden:
			detail := fmt.Sprintf("- %s [granted] %s — granted by %s: %s",
				record.ID, record.Title, record.OverrideGrantedBy, record.OverrideGrantReason)
			if record.OverrideRequestedBy != "" {
				detail += fmt.Sprintf(" (requested by %s: %s)", record.OverrideRequestedBy, record.OverrideRequestReason)
			}
			lines = append(lines, detail)
		}
	}
	return lines
}
