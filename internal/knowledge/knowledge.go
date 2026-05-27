package knowledge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/repo"
)

type Context struct {
	ServiceInventoryPath string `json:"serviceInventoryPath,omitempty"`
	ServiceInventory     string `json:"serviceInventory,omitempty"`
	Facts                []Fact `json:"facts"`
}

type Fact struct {
	Source string `json:"source"`
	Line   int    `json:"line"`
	Text   string `json:"text"`
}

func Collect(root string) (Context, error) {
	var context Context
	if path := repo.FindServiceInventory(root); path != nil {
		fullPath, err := containedPath(root, *path)
		if err != nil {
			return Context{}, err
		}
		bytes, err := os.ReadFile(fullPath)
		if err != nil {
			return Context{}, err
		}
		context.ServiceInventoryPath = *path
		context.ServiceInventory = string(bytes)
	}
	facts, err := collectFacts(root)
	if err != nil {
		return Context{}, err
	}
	context.Facts = facts
	return context, nil
}

func collectFacts(root string) ([]Fact, error) {
	dir := filepath.Join(root, ".beads", "knowledge")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Fact{}, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var facts []Fact
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(".beads", "knowledge", entry.Name()))
		fullPath, err := containedPath(root, rel)
		if err != nil {
			return nil, err
		}
		file, err := os.Open(fullPath)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			facts = append(facts, Fact{Source: rel, Line: lineNumber, Text: factText(line)})
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
	}
	return facts, nil
}

func containedPath(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidateAbs, err := filepath.Abs(filepath.Join(rootAbs, filepath.Clean(rel)))
	if err != nil {
		return "", err
	}
	if candidateAbs != rootAbs && !strings.HasPrefix(candidateAbs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("knowledge path is outside repository root: %s", rel)
	}
	realRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", err
	}
	realCandidate, err := filepath.EvalSymlinks(candidateAbs)
	if err != nil {
		return "", err
	}
	if realCandidate != realRoot && !strings.HasPrefix(realCandidate, realRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("knowledge path is outside repository root: %s", rel)
	}
	return candidateAbs, nil
}

func factText(line string) string {
	var object map[string]any
	if err := json.Unmarshal([]byte(line), &object); err != nil {
		return line
	}
	for _, key := range []string{"fact", "text", "summary", "body"} {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return line
}
