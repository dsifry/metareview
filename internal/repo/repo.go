package repo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Capabilities struct {
	Git              bool `json:"git"`
	Beads            bool `json:"beads"`
	Metaswarm        bool `json:"metaswarm"`
	ServiceInventory bool `json:"serviceInventory"`
	MetareviewState  bool `json:"metareviewState"`
}

type Files struct {
	ServiceInventory *string `json:"serviceInventory"`
}

type Report struct {
	Mode         string       `json:"mode"`
	Capabilities Capabilities `json:"capabilities"`
	Files        Files        `json:"files"`
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, rel))
	return err == nil
}

func readIfFile(root, rel string) string {
	bytes, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return ""
	}
	return string(bytes)
}

func hasMetaswarmInstructionMarker(text string) bool {
	normalized := strings.ToLower(text)
	for _, marker := range []string{"does not use metaswarm", "not using metaswarm", "without metaswarm", "no metaswarm"} {
		if strings.Contains(normalized, marker) {
			return false
		}
	}
	return strings.Contains(normalized, "uses metaswarm") ||
		strings.Contains(normalized, "metaswarm workflows") ||
		strings.Contains(normalized, "metaswarm")
}

func hasMetaswarmMarker(root string) bool {
	instructions := strings.Join([]string{
		readIfFile(root, "AGENTS.md"),
		readIfFile(root, "CLAUDE.md"),
		readIfFile(root, "GEMINI.md"),
	}, "\n")
	return hasMetaswarmInstructionMarker(instructions) ||
		exists(root, ".claude/plugins/metaswarm") ||
		exists(root, ".codex/plugins/metaswarm") ||
		exists(root, "docs/metaswarm") ||
		exists(root, ".beads/context/project-context.md")
}

func FindServiceInventory(root string) *string {
	for _, candidate := range []string{
		"docs/SERVICE_INVENTORY.md",
		"SERVICE_INVENTORY.md",
		"docs/service-inventory.md",
		"docs/architecture/SERVICE_INVENTORY.md",
	} {
		if exists(root, candidate) {
			value := candidate
			return &value
		}
	}
	return nil
}

func Detect(root string) Report {
	serviceInventory := FindServiceInventory(root)
	capabilities := Capabilities{
		Git:              exists(root, ".git"),
		Beads:            exists(root, ".beads") || exists(root, ".beads/issues.jsonl"),
		Metaswarm:        hasMetaswarmMarker(root),
		ServiceInventory: serviceInventory != nil,
		MetareviewState:  exists(root, ".metareview"),
	}
	mode := "advisory"
	if capabilities.Metaswarm {
		mode = "metaswarm-extension"
	} else if capabilities.Beads {
		mode = "standalone-full"
	} else if capabilities.Git {
		mode = "standalone-minimal"
	}
	return Report{Mode: mode, Capabilities: capabilities, Files: Files{ServiceInventory: serviceInventory}}
}

func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
