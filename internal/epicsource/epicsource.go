package epicsource

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Source struct {
	Kind     string   `json:"kind"`
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Path     string   `json:"path,omitempty"`
	ChildIDs []string `json:"childIds"`
}

func Resolve(root, target string) (Source, error) {
	if strings.TrimSpace(target) == "" {
		return Source{}, fmt.Errorf("epic target is required")
	}
	if looksPathLike(target) {
		return resolveMarkdown(root, target)
	}
	if source, ok, err := resolveBeads(root, target); ok || err != nil {
		return source, err
	}
	return Source{Kind: "advisory", ID: target, Title: target, Body: "Advisory epic target: " + target}, nil
}

func resolveBeads(root, id string) (Source, bool, error) {
	issues, err := readIssues(filepath.Join(root, ".beads", "issues.jsonl"))
	if err != nil {
		return Source{}, false, err
	}
	var epic map[string]any
	for _, issue := range issues {
		if stringField(issue, "id") == id {
			epic = issue
			break
		}
	}
	if epic == nil {
		return Source{}, false, nil
	}
	children := stringSet{}
	children.addAll(stringList(epic, "children"))
	children.addAll(stringList(epic, "depends_on"))
	children.addAll(stringList(epic, "dependencies"))
	for _, issue := range issues {
		if stringField(issue, "parent") == id || stringField(issue, "epic") == id {
			children.add(stringField(issue, "id"))
		}
		for _, dep := range append(stringList(issue, "depends_on"), stringList(issue, "dependencies")...) {
			if dep == id {
				children.add(stringField(issue, "id"))
			}
		}
	}
	childIDs := children.values()
	return Source{
		Kind:     "beads",
		ID:       id,
		Title:    firstNonEmpty(stringField(epic, "title"), stringField(epic, "summary"), id),
		Body:     body(epic),
		Path:     ".beads/issues.jsonl",
		ChildIDs: childIDs,
	}, true, nil
}

func readIssues(path string) ([]map[string]any, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var issues []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var issue map[string]any
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			return nil, err
		}
		issues = append(issues, issue)
	}
	return issues, scanner.Err()
}

func resolveMarkdown(root, target string) (Source, error) {
	path, rel, err := containedPath(root, target)
	if err != nil {
		return Source{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Source{}, fmt.Errorf("epic path not found: %s", target)
	}
	if !info.Mode().IsRegular() {
		return Source{}, fmt.Errorf("epic path is not a regular file: %s", target)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return Source{}, err
	}
	text := string(bytes)
	return Source{Kind: "markdown", ID: rel, Title: markdownTitle(text, rel), Body: text, Path: rel}, nil
}

func containedPath(root, target string) (string, string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	var candidate string
	if filepath.IsAbs(target) {
		candidate = filepath.Clean(target)
	} else {
		candidate = filepath.Join(rootAbs, filepath.Clean(target))
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", err
	}
	if candidateAbs != rootAbs && !strings.HasPrefix(candidateAbs, rootAbs+string(filepath.Separator)) {
		return "", "", fmt.Errorf("epic path is outside repository root: %s", target)
	}
	realRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", "", err
	}
	realCandidate, err := filepath.EvalSymlinks(candidateAbs)
	if err != nil {
		return "", "", err
	}
	if realCandidate != realRoot && !strings.HasPrefix(realCandidate, realRoot+string(filepath.Separator)) {
		return "", "", fmt.Errorf("epic path is outside repository root: %s", target)
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return "", "", err
	}
	return candidateAbs, filepath.ToSlash(rel), nil
}

func looksPathLike(target string) bool {
	return filepath.IsAbs(target) || strings.Contains(target, "/") || strings.Contains(target, `\`) || strings.HasSuffix(strings.ToLower(target), ".md")
}

func stringField(object map[string]any, key string) string {
	value, ok := object[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func stringList(object map[string]any, key string) []string {
	value, ok := object[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	case []any:
		var values []string
		for _, item := range typed {
			text, ok := item.(string)
			if ok && strings.TrimSpace(text) != "" {
				values = append(values, strings.TrimSpace(text))
			}
		}
		return values
	default:
		return nil
	}
}

func body(issue map[string]any) string {
	parts := make([]string, 0, 6)
	for _, key := range []string{"description", "body", "content", "summary", "acceptance", "details", "status"} {
		if value := stringField(issue, key); value != "" {
			if key == "status" {
				parts = append(parts, "Status: "+value)
			} else {
				parts = append(parts, value)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func markdownTitle(text, fallback string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type stringSet map[string]bool

func (s stringSet) add(value string) {
	if strings.TrimSpace(value) != "" {
		s[strings.TrimSpace(value)] = true
	}
}

func (s stringSet) addAll(values []string) {
	for _, value := range values {
		s.add(value)
	}
}

func (s stringSet) values() []string {
	values := make([]string, 0, len(s))
	for value := range s {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}
