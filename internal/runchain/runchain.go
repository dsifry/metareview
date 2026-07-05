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
	HeadSHA       string
}

type Decision struct {
	AttemptNumber int
	MaxAttempts   int
	PreviousRun   *Record
	RootRun       *Record
	Chain         []Record
	ResetRunIDs   []string
}

type Record struct {
	ID                   string            `json:"id"`
	Scope                string            `json:"scope"`
	Target               map[string]string `json:"target"`
	Status               string            `json:"status"`
	Verdict              string            `json:"verdict"`
	PreviousRunID        string            `json:"previousRunId"`
	AttemptNumber        int               `json:"attemptNumber"`
	MaxAttempts          int               `json:"maxAttempts"`
	HeadSHA              string            `json:"headSha"`
	BlockingFindingCount int               `json:"blockingFindingCount"`
	AdvisoryFindingCount int               `json:"advisoryFindingCount"`
	FollowUpFindingCount int               `json:"followUpFindingCount"`
	WarningFindingCount  int               `json:"warningFindingCount"`
	EscalationReason     string            `json:"escalationReason"`
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
	resetRunIDs := escalatedResetRunIDs(records, options.Scope, options.Target, options.HeadSHA)
	if options.PreviousRunID != "" {
		chain, err := ChainTo(records, options.PreviousRunID)
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
		if escalated, ok := escalatedForTarget(records, options.Scope, options.Target, options.HeadSHA); ok && escalated.ID != previous.ID {
			return Decision{}, fmt.Errorf("same target already escalated in run %s", escalated.ID)
		}
		rootRun := chain[0]
		max := rootRun.MaxAttempts
		if max == 0 {
			max = effectiveMax(options.MaxAttempts)
		}
		return Decision{AttemptNumber: previous.AttemptNumber + 1, MaxAttempts: max, PreviousRun: &previous, RootRun: &rootRun, Chain: chain, ResetRunIDs: resetRunIDs}, nil
	}
	if escalated, ok := escalatedForTarget(records, options.Scope, options.Target, options.HeadSHA); ok {
		return Decision{}, fmt.Errorf("same target already escalated in run %s", escalated.ID)
	}
	return Decision{AttemptNumber: 1, MaxAttempts: effectiveMax(options.MaxAttempts), ResetRunIDs: resetRunIDs}, nil
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

func escalatedForTarget(records []Record, scope string, target map[string]string, headSHA string) (Record, bool) {
	for _, record := range records {
		if record.Scope == scope && sameTarget(record.Target, target) && strings.EqualFold(record.Verdict, "ESCALATED") {
			if strings.TrimSpace(headSHA) != "" && strings.TrimSpace(record.HeadSHA) != "" && strings.TrimSpace(record.HeadSHA) != strings.TrimSpace(headSHA) {
				continue
			}
			return record, true
		}
	}
	return Record{}, false
}

func escalatedResetRunIDs(records []Record, scope string, target map[string]string, headSHA string) []string {
	headSHA = strings.TrimSpace(headSHA)
	if headSHA == "" {
		return nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, record := range records {
		recordHead := strings.TrimSpace(record.HeadSHA)
		if record.Scope == scope &&
			sameTarget(record.Target, target) &&
			strings.EqualFold(record.Verdict, "ESCALATED") &&
			recordHead != "" &&
			recordHead != headSHA {
			chain, err := ChainTo(records, record.ID)
			if err != nil {
				chain = []Record{record}
			}
			for _, link := range chain {
				if !seen[link.ID] {
					ids = append(ids, link.ID)
					seen[link.ID] = true
				}
			}
		}
	}
	return ids
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
