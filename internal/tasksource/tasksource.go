package tasksource

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Source struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Path  string `json:"path,omitempty"`
}

func Resolve(root, target string) (Source, error) {
	if strings.TrimSpace(target) == "" {
		return Source{}, fmt.Errorf("task target is required")
	}
	if looksPathLike(target) {
		return resolveMarkdown(root, target)
	}
	if source, ok, err := resolveBeads(root, target); ok || err != nil {
		return source, err
	}
	return Source{
		Kind:  "advisory",
		ID:    target,
		Title: target,
		Body:  "Advisory task target: " + target,
	}, nil
}

func resolveBeads(root, id string) (Source, bool, error) {
	path := filepath.Join(root, ".beads", "issues.jsonl")
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return Source{}, false, nil
	}
	if err != nil {
		return Source{}, false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var issue map[string]any
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			return Source{}, false, err
		}
		issueID := stringField(issue, "id")
		if issueID != id {
			continue
		}
		return Source{
			Kind:  "beads",
			ID:    issueID,
			Title: firstNonEmpty(stringField(issue, "title"), stringField(issue, "summary"), issueID),
			Body:  beadsBody(issue),
			Path:  ".beads/issues.jsonl",
		}, true, nil
	}
	if err := scanner.Err(); err != nil {
		return Source{}, false, err
	}
	return Source{}, false, nil
}

func resolveMarkdown(root, target string) (Source, error) {
	path, rel, err := containedPath(root, target)
	if err != nil {
		return Source{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Source{}, fmt.Errorf("task path not found: %s", target)
	}
	if !info.Mode().IsRegular() {
		return Source{}, fmt.Errorf("task path is not a regular file: %s", target)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return Source{}, err
	}
	body := string(bytes)
	return Source{
		Kind:  "markdown",
		ID:    rel,
		Title: markdownTitle(body, rel),
		Body:  body,
		Path:  rel,
	}, nil
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
		return "", "", fmt.Errorf("task path is outside repository root: %s", target)
	}
	realRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", "", err
	}
	realCandidate, err := filepath.EvalSymlinks(candidateAbs)
	if err != nil {
		if os.IsNotExist(err) {
			rel, relErr := filepath.Rel(rootAbs, candidateAbs)
			if relErr != nil {
				return "", "", relErr
			}
			return candidateAbs, filepath.ToSlash(rel), nil
		}
		return "", "", err
	}
	if realCandidate != realRoot && !strings.HasPrefix(realCandidate, realRoot+string(filepath.Separator)) {
		return "", "", fmt.Errorf("task path is outside repository root: %s", target)
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return "", "", err
	}
	return candidateAbs, filepath.ToSlash(rel), nil
}

func looksPathLike(target string) bool {
	return filepath.IsAbs(target) ||
		strings.Contains(target, "/") ||
		strings.Contains(target, `\`) ||
		strings.HasSuffix(strings.ToLower(target), ".md")
}

func markdownTitle(body, fallback string) string {
	for _, line := range strings.Split(body, "\n") {
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
			return value
		}
	}
	return ""
}

func beadsBody(issue map[string]any) string {
	parts := make([]string, 0, 6)
	for _, key := range []string{"description", "body", "content", "summary", "acceptance", "details", "status"} {
		value := stringField(issue, key)
		if value == "" {
			continue
		}
		if key == "status" {
			parts = append(parts, "Status: "+value)
			continue
		}
		parts = append(parts, value)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
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
