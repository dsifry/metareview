package sessionhistory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dsifry/metareview/internal/githubcontext"
)

const MaxSignals = 8
const MaxExcerptRunes = 500
const maxCandidateFiles = 80

type Options struct {
	SessionRoot string
	HomeDir     string
}

type Context struct {
	Available            bool                  `json:"available"`
	UnavailableReason    string                `json:"unavailableReason,omitempty"`
	Signals              []Signal              `json:"signals"`
	IntrospectionRequest *IntrospectionRequest `json:"introspectionRequest,omitempty"`
}

type Signal struct {
	Path       string `json:"path"`
	SourceType string `json:"sourceType"`
	RecordKind string `json:"recordKind"`
	Confidence string `json:"confidence"`
	Timestamp  string `json:"timestamp,omitempty"`
	Excerpt    string `json:"excerpt"`
}

type IntrospectionRequest struct {
	Type       string `json:"type"`
	Confidence string `json:"confidence"`
	Prompt     string `json:"prompt"`
}

type candidate struct {
	path       string
	sourceType string
	recordKind string
	confidence string
	priority   int
}

func Collect(root string, options Options) (Context, error) {
	home, err := homeDir(options.HomeDir)
	if err != nil {
		return Context{}, err
	}
	candidates, rootsFound, err := discoverCandidates(root, home, options.SessionRoot)
	if err != nil {
		return Context{}, err
	}
	if !rootsFound {
		return unavailable("no-session-root", true), nil
	}
	signals, err := collectSignals(root, home, candidates)
	if err != nil {
		return Context{}, err
	}
	if len(signals) == 0 {
		return unavailable("no-usable-session-records", true), nil
	}
	return Context{Available: true, Signals: signals}, nil
}

func discoverCandidates(root, home, explicitRoot string) ([]candidate, bool, error) {
	var candidates []candidate
	rootsFound := false
	if explicitRoot != "" {
		if exists(explicitRoot) {
			rootsFound = true
			found, err := scanSessionPath(explicitRoot, "explicit", 0)
			if err != nil {
				return nil, false, err
			}
			candidates = append(candidates, found...)
		}
	}
	for _, item := range []struct {
		path       string
		sourceType string
	}{
		{filepath.Join(home, ".codex", "session_index.jsonl"), "codex"},
		{filepath.Join(home, ".codex", "history.jsonl"), "codex"},
		{filepath.Join(home, ".codex", "sessions"), "codex"},
		{filepath.Join(home, ".codex", "memories", "rollout_summaries"), "codex"},
		{filepath.Join(home, ".claude", "projects"), "claude"},
		{filepath.Join(home, ".claude", "tasks"), "claude"},
	} {
		if !exists(item.path) {
			continue
		}
		rootsFound = true
		found, err := scanSessionPath(item.path, item.sourceType, 1)
		if err != nil {
			return nil, false, err
		}
		candidates = append(candidates, found...)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		return candidates[i].path < candidates[j].path
	})
	if len(candidates) > maxCandidateFiles {
		candidates = candidates[:maxCandidateFiles]
	}
	return candidates, rootsFound, nil
}

func scanSessionPath(path, sourceType string, priority int) ([]candidate, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if found, ok := candidateFor(path, sourceType, priority); ok {
			return []candidate{found}, nil
		}
		return nil, nil
	}
	var candidates []candidate
	err = filepath.WalkDir(path, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if candidate, ok := candidateFor(path, sourceType, priority); ok {
			candidates = append(candidates, candidate)
		}
		return nil
	})
	return candidates, err
}

func candidateFor(path, sourceType string, priority int) (candidate, bool) {
	lower := strings.ToLower(filepath.ToSlash(path))
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".jsonl" && ext != ".json" && ext != ".md" {
		return candidate{}, false
	}
	recordKind := "raw-transcript"
	if strings.Contains(lower, "rollout_summaries") || strings.Contains(lower, "summary") || ext == ".md" {
		recordKind = "generated-summary"
	}
	kindSuffix := "json"
	if ext == ".jsonl" {
		kindSuffix = "jsonl"
	}
	if recordKind == "generated-summary" {
		kindSuffix = "summary"
	}
	confidence := "medium"
	if recordKind == "raw-transcript" && (ext == ".jsonl" || ext == ".json") {
		confidence = "high"
	}
	return candidate{
		path:       path,
		sourceType: sourceType + "-" + kindSuffix,
		recordKind: recordKind,
		confidence: confidence,
		priority:   priority,
	}, true
}

func collectSignals(root, home string, candidates []candidate) ([]Signal, error) {
	signals := make([]Signal, 0, MaxSignals)
	for _, candidate := range candidates {
		if len(signals) >= MaxSignals {
			break
		}
		signal, ok, err := signalFromCandidate(root, home, candidate)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		signals = append(signals, signal)
	}
	return signals, nil
}

func signalFromCandidate(root, home string, candidate candidate) (Signal, bool, error) {
	ext := strings.ToLower(filepath.Ext(candidate.path))
	switch ext {
	case ".jsonl":
		timestamp, excerpt, ok, err := readJSONLSignal(candidate.path)
		return buildSignal(root, home, candidate, timestamp, excerpt), ok, err
	case ".json":
		timestamp, excerpt, ok, err := readJSONSignal(candidate.path)
		return buildSignal(root, home, candidate, timestamp, excerpt), ok, err
	case ".md":
		bytes, err := os.ReadFile(candidate.path)
		if err != nil {
			return Signal{}, false, err
		}
		excerpt := excerpt(string(bytes))
		return buildSignal(root, home, candidate, "", excerpt), strings.TrimSpace(excerpt) != "", nil
	default:
		return Signal{}, false, nil
	}
}

func readJSONLSignal(path string) (string, string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", false, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		timestamp, text, ok := parseJSONObject(line)
		if ok {
			return timestamp, excerpt(text), true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", false, err
	}
	return "", "", false, nil
}

func readJSONSignal(path string) (string, string, bool, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", "", false, err
	}
	timestamp, text, ok := parseJSONObject(string(bytes))
	if !ok {
		return "", "", false, nil
	}
	return timestamp, excerpt(text), true, nil
}

func parseJSONObject(text string) (string, string, bool) {
	var object map[string]any
	if err := json.Unmarshal([]byte(text), &object); err != nil {
		return "", "", false
	}
	timestamp := firstString(object, "timestamp", "created_at", "createdAt", "time")
	body := firstContentString(object)
	if strings.TrimSpace(body) == "" {
		return timestamp, "", false
	}
	return timestamp, body, true
}

func firstContentString(object map[string]any) string {
	for _, key := range []string{"message", "content", "text", "summary", "event_msg", "turn_context", "response_item"} {
		value, ok := object[key]
		if !ok {
			continue
		}
		if found := contentString(value); found != "" {
			return found
		}
	}
	return ""
}

func contentString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if found := firstContentString(typed); found != "" {
			return found
		}
	case []any:
		for _, item := range typed {
			if found := contentString(item); found != "" {
				return found
			}
		}
	}
	return ""
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildSignal(root, home string, candidate candidate, timestamp, value string) Signal {
	return Signal{
		Path:       displayPath(root, home, candidate.path),
		SourceType: candidate.sourceType,
		RecordKind: candidate.recordKind,
		Confidence: candidate.confidence,
		Timestamp:  timestamp,
		Excerpt:    excerpt(value),
	}
}

func displayPath(root, home, path string) string {
	if rel, ok := relativeTo(root, path); ok {
		return filepath.ToSlash(rel)
	}
	if rel, ok := relativeTo(home, path); ok {
		return filepath.ToSlash(filepath.Join("~", rel))
	}
	return filepath.ToSlash(path)
}

func relativeTo(base, path string) (string, bool) {
	if base == "" {
		return "", false
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", false
	}
	return rel, true
}

func excerpt(value string) string {
	value = strings.TrimSpace(githubcontext.Redact(value))
	runes := []rune(value)
	if len(runes) <= MaxExcerptRunes {
		return value
	}
	truncated := string(runes[:MaxExcerptRunes])
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return strings.TrimSpace(truncated)
}

func unavailable(reason string, introspect bool) Context {
	ctx := Context{Available: false, UnavailableReason: reason}
	if introspect {
		ctx.IntrospectionRequest = &IntrospectionRequest{
			Type:       "active-runtime-session-history-discovery",
			Confidence: "low",
			Prompt:     "Ask the current coding agent/runtime where it stores session/project history, then verify any answer by file existence and schema sampling before using it as review knowledge.",
		}
	}
	return ctx
}

func homeDir(configured string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("session history home unavailable: %w", err)
	}
	return home, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
