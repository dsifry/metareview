package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// EnforcementStatus reports whether the Stop hook — the thing that makes the Completion Rule a
// gate rather than a sentence in CLAUDE.md — is actually loaded.
//
// It is checked here because nothing checked it anywhere. hooks/hooks.json is a PLUGIN manifest:
// it takes effect only when metareview is installed as a plugin, so in a source checkout the
// hook sat in the repository as a file no host ever read. `metareview setup` reported every
// prerequisite as satisfied while the one component that enforces anything had never run once,
// and the failure was silent in both directions — nothing said the hook was missing, and the
// hook could not say it was missing because it was not there to say it.
type EnforcementStatus struct {
	// Active is true when some settings file this repository can see registers a Stop hook.
	Active bool `json:"active"`
	// Source names where the registration was found, empty when there is none.
	Source string `json:"source,omitempty"`
	// ScriptPresent is whether hooks/pre-finish.sh exists in the checkout. A registration
	// pointing at a missing script is worse than no registration: the host reports a hook error
	// rather than a review verdict.
	ScriptPresent bool   `json:"scriptPresent"`
	Remediation   string `json:"remediation,omitempty"`
}

// hookSettingsFiles are the places a Stop hook can be registered, nearest first. Project settings
// win for a source checkout; the user's own settings cover a plugin or a manual install.
func hookSettingsFiles(root, home string) []string {
	return []string{
		filepath.Join(root, ".claude", "settings.json"),
		filepath.Join(root, ".claude", "settings.local.json"),
		filepath.Join(home, ".claude", "settings.json"),
	}
}

func enforcementStatus(root, home string) EnforcementStatus {
	s := EnforcementStatus{}
	if info, err := os.Stat(filepath.Join(root, "hooks", "pre-finish.sh")); err == nil && info.Mode().IsRegular() {
		s.ScriptPresent = true
	}
	for _, path := range hookSettingsFiles(root, home) {
		if registersStopHook(path) {
			s.Active, s.Source = true, path
			break
		}
	}
	switch {
	case s.Active && !s.ScriptPresent:
		s.Remediation = "A Stop hook is registered in " + s.Source + " but hooks/pre-finish.sh is missing, so the host will report a hook error instead of a review verdict."
	case !s.Active && s.ScriptPresent:
		s.Remediation = "hooks/pre-finish.sh exists but no settings file registers it, so it never runs. Add a Stop hook to .claude/settings.json, or install metareview as a plugin so hooks/hooks.json applies."
	case !s.Active:
		s.Remediation = "No Stop hook is registered, so nothing stops a host from finishing with unresolved blockers. The Completion Rule is advisory until one is."
	}
	return s
}

// registersStopHook reports whether path declares at least one Stop hook. It reads the file
// rather than trusting its presence: an empty settings.json, or one with other hooks and no
// Stop entry, leaves the gate exactly as absent as no file at all.
func registersStopHook(path string) bool {
	data, err := os.ReadFile(path) // #nosec G304 -- a settings path this package constructs
	if err != nil {
		return false
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	for _, matcher := range doc.Hooks["Stop"] {
		for _, h := range matcher.Hooks {
			if strings.TrimSpace(h.Command) != "" {
				return true
			}
		}
	}
	return false
}
