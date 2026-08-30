package mutation

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Parse reads one engine's report without being told which engine wrote it.
//
// Detection is on structure, not on a filename or a flag the caller has to get right: the two
// formats disagree about what `files` is — the standard schema keys it by path, gremlins makes it
// an array — so a single probe separates them and neither can be mistaken for the other. A caller
// that already knows the engine can still use ParseStryker or ParseGremlins directly.
func Parse(data []byte, target string) (Report, error) {
	var probe struct {
		Files json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return Report{}, fmt.Errorf("mutation: reading report: %w", err)
	}
	for _, b := range probe.Files {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return ParseStryker(data, target)
		case '[':
			return ParseGremlins(data, target)
		}
		break
	}
	// No `files` at all, or something that is neither. Refusing is the point: a report this code
	// cannot read must not become an empty report, which would score as a clean run.
	return Report{}, fmt.Errorf("mutation: %s is not a report this build can read: expected mutation-testing-report-schema or gremlins JSON", reportName(target))
}

// Load reads and parses one report file.
func Load(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, fmt.Errorf("mutation: reading report: %w", err)
	}
	return Parse(data, path)
}

// LoadAll reads every named report. It stops at the first unreadable one rather than reviewing
// with a partial set: a mutation gate that silently drops a report is a gate that passes because
// it looked at less.
func LoadAll(paths []string) ([]Report, error) {
	out := make([]Report, 0, len(paths))
	for _, p := range paths {
		r, err := Load(p)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func reportName(target string) string {
	if target == "" {
		return "the report"
	}
	return target
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
