package setup

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/dsifry/metareview/internal/repo"
)

var ErrConfirmationRequired = errors.New("setup bootstrap prerequisites requires --confirm-bootstrap-prereqs")

type LookupPathFunc func(string) (string, error)

type Options struct {
	ExecutablePath string
	HomeDir        string
	LookupPath     LookupPathFunc
}

type Report struct {
	Mode          string              `json:"mode"`
	Capabilities  repo.Capabilities   `json:"capabilities"`
	Files         repo.Files          `json:"files"`
	Prerequisites Prerequisites       `json:"prerequisites"`
	Install       InstallStatus       `json:"install"`
	Standalone    StandaloneReadiness `json:"standalone"`
}

type Prerequisites struct {
	Superpowers ToolStatus `json:"superpowers"`
	Beads       ToolStatus `json:"beads"`
	Metaswarm   ToolStatus `json:"metaswarm"`
	Go          ToolStatus `json:"go"`
	Git         ToolStatus `json:"git"`
}

type ToolStatus struct {
	Present bool   `json:"present"`
	Path    string `json:"path,omitempty"`
	Action  string `json:"action,omitempty"`
}

type InstallStatus struct {
	Path string `json:"path"`
}

type StandaloneReadiness struct {
	AdvisoryOnly             bool     `json:"advisoryOnly"`
	FullMetaswarmReady       bool     `json:"fullMetaswarmReady"`
	MissingForFullMetaswarm  []string `json:"missingForFullMetaswarm"`
	FullMetaswarmDescription string   `json:"fullMetaswarmDescription"`
}

type BootstrapOptions struct {
	DryRun  bool
	Confirm bool
}

type BootstrapPlan struct {
	DryRun  bool
	Actions []string
}

func Check(root string, options Options) Report {
	base := repo.Detect(root)
	lookup := options.LookupPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	home := options.HomeDir
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	prereqs := Prerequisites{
		Superpowers: superpowersStatus(root, home),
		Beads:       beadsStatus(root, lookup, base.Capabilities.Beads),
		Metaswarm:   metaswarmStatus(root, base.Capabilities.Metaswarm),
		Go:          commandStatus(lookup, "go", "Install Go 1.22+ and ensure go is on PATH."),
		Git:         commandStatus(lookup, "git", "Install git and ensure git is on PATH."),
	}
	missing := missingFullMetaswarmPrereqs(prereqs)

	return Report{
		Mode:          base.Mode,
		Capabilities:  base.Capabilities,
		Files:         base.Files,
		Prerequisites: prereqs,
		Install:       InstallStatus{Path: options.ExecutablePath},
		Standalone: StandaloneReadiness{
			AdvisoryOnly:             len(missing) > 0,
			FullMetaswarmReady:       len(missing) == 0,
			MissingForFullMetaswarm:  missing,
			FullMetaswarmDescription: "Full metaswarm mode requires Superpowers, Beads, and metaswarm; advisory review can run with git and Go.",
		},
	}
}

func BootstrapPrereqs(root string, options BootstrapOptions) (BootstrapPlan, error) {
	plan := BootstrapPlan{
		DryRun: options.DryRun,
		Actions: []string{
			"Install Superpowers: enable the Superpowers plugin/skills for the current agent runtime.",
			"Install Beads: install the bd CLI and initialize .beads/ in this repository.",
			"Install metaswarm: install from ../metaswarm when available or from https://github.com/dsifry/metaswarm.",
		},
	}
	if options.DryRun {
		plan.Actions = append(plan.Actions, "No changes made (dry run).")
		return plan, nil
	}
	if !options.Confirm {
		return plan, ErrConfirmationRequired
	}
	plan.Actions = append(plan.Actions, "Confirmation supplied; automated prerequisite installation is not implemented in this release.")
	return plan, nil
}

func superpowersStatus(root, home string) ToolStatus {
	candidates := []string{
		filepath.Join(root, ".claude", "plugins", "superpowers"),
		filepath.Join(root, ".codex", "plugins", "superpowers"),
		filepath.Join(root, ".agents", "plugins", "superpowers"),
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".claude", "plugins", "superpowers"),
			filepath.Join(home, ".codex", "plugins", "cache", "claude-plugins-official", "superpowers"),
		)
	}
	for _, candidate := range candidates {
		if isDir(candidate) {
			return ToolStatus{Present: true, Path: candidate}
		}
	}
	return ToolStatus{Action: "Install or enable the Superpowers plugin/skills for the current agent runtime."}
}

func beadsStatus(root string, lookup LookupPathFunc, repoHasBeads bool) ToolStatus {
	if repoHasBeads {
		return ToolStatus{Present: true, Path: filepath.Join(root, ".beads")}
	}
	if path, err := lookup("bd"); err == nil {
		return ToolStatus{Present: true, Path: path}
	}
	return ToolStatus{Action: "Install Beads and run bd init for full metaswarm mode."}
}

func metaswarmStatus(root string, repoHasMetaswarm bool) ToolStatus {
	if repoHasMetaswarm {
		return ToolStatus{Present: true, Path: root}
	}
	for _, candidate := range []string{
		filepath.Clean(filepath.Join(root, "..", "metaswarm")),
		filepath.Join(root, ".claude", "plugins", "metaswarm"),
		filepath.Join(root, ".codex", "plugins", "metaswarm"),
	} {
		if isDir(candidate) {
			return ToolStatus{Present: true, Path: candidate}
		}
	}
	return ToolStatus{Action: "Install metaswarm from ../metaswarm or https://github.com/dsifry/metaswarm."}
}

func commandStatus(lookup LookupPathFunc, name, action string) ToolStatus {
	path, err := lookup(name)
	if err != nil {
		return ToolStatus{Action: action}
	}
	return ToolStatus{Present: true, Path: path}
}

func missingFullMetaswarmPrereqs(prereqs Prerequisites) []string {
	missing := []string{}
	if !prereqs.Beads.Present {
		missing = append(missing, "beads")
	}
	if !prereqs.Metaswarm.Present {
		missing = append(missing, "metaswarm")
	}
	if !prereqs.Superpowers.Present {
		missing = append(missing, "superpowers")
	}
	sort.Strings(missing)
	return missing
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
