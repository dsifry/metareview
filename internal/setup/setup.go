package setup

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
	Mode          string            `json:"mode"`
	Capabilities  repo.Capabilities `json:"capabilities"`
	Files         repo.Files        `json:"files"`
	Prerequisites Prerequisites     `json:"prerequisites"`
	Install       InstallStatus     `json:"install"`
	Enforcement   EnforcementStatus `json:"enforcement"`
	// GitGate is the push-time enforcement (the pre-push review gate), distinct from Enforcement's
	// session-completion Stop hook. Reporting both keeps setup --check from understating the posture.
	GitGate    GitGateStatus       `json:"gitGate"`
	Standalone StandaloneReadiness `json:"standalone"`
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
	// Version and VersionOK are reported for tools with a minimum this project
	// actually depends on. INSTALL.md tells the operator that setup --check
	// reports git >= MinGitVersion, so it has to.
	Version   string `json:"version,omitempty"`
	VersionOK bool   `json:"versionOk,omitempty"`
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
		Go:          commandStatus(lookup, "go", "Install Go 1.26+ and ensure go is on PATH."),
		Git:         gitStatus(lookup, nil),
	}
	missing := missingFullMetaswarmPrereqs(prereqs)

	gitGate := gitGateStatus(root, nil)

	return Report{
		Mode:          base.Mode,
		Capabilities:  base.Capabilities,
		Files:         base.Files,
		Prerequisites: prereqs,
		Install:       InstallStatus{Path: options.ExecutablePath},
		Enforcement:   enforcementStatus(root, home, pluginRoot(home), gitGate.Installed),
		GitGate:       gitGate,
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

// MinGitVersion is the git `metareview fsm` needs, as INSTALL.md states.
const MinGitVersion = "2.31"

// gitVersionOutput returns `git version` output; a seam so the check is testable
// without depending on whatever git the machine happens to carry.
type gitVersionOutput func() (string, error)

// gitStatus is commandStatus for git, plus the version check INSTALL.md promises.
// Presence and version are reported separately: a git that is installed but too
// old is Present with VersionOK false, which is a different problem from absent.
func gitStatus(lookup LookupPathFunc, version gitVersionOutput) ToolStatus {
	status := commandStatus(lookup, "git", "Install git and ensure git is on PATH.")
	if !status.Present {
		return status
	}
	if version == nil {
		version = func() (string, error) {
			out, err := exec.Command(status.Path, "version").Output()
			return string(out), err
		}
	}
	out, err := version()
	if err != nil {
		status.Action = "Could not read the git version; metareview fsm needs git >= " + MinGitVersion + "."
		return status
	}
	v := parseGitVersion(out)
	if v == "" {
		status.Action = "Could not parse the git version; metareview fsm needs git >= " + MinGitVersion + "."
		return status
	}
	status.Version = v
	if !atLeastVersion(v, MinGitVersion) {
		status.Action = "Upgrade git: metareview fsm needs git >= " + MinGitVersion + ", found " + v + "."
		return status
	}
	status.VersionOK = true
	return status
}

// parseGitVersion pulls the number out of "git version 2.39.5 (Apple Git-154)".
func parseGitVersion(out string) string {
	fields := strings.Fields(strings.TrimSpace(out))
	for i, f := range fields {
		if f == "version" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// atLeastVersion compares dotted numeric versions field by field; a non-numeric
// field ends the comparison rather than guessing at it.
func atLeastVersion(have, min string) bool {
	hp, mp := strings.Split(have, "."), strings.Split(min, ".")
	for i := range mp {
		if i >= len(hp) {
			return false
		}
		h, err1 := strconv.Atoi(hp[i])
		m, err2 := strconv.Atoi(mp[i])
		if err1 != nil || err2 != nil {
			return false
		}
		if h != m {
			return h > m
		}
	}
	return true
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

// pluginRoot locates a plugin installation of metareview, when there is one.
//
// It does NOT walk up from the executable. That walk asked "is there a hooks/hooks.json above
// me", which is true in an ordinary source checkout — this repository ships that manifest — so
// `./bin/metareview setup --check` certified enforcement as active although no host had ever
// loaded the plugin. Reporting a gate as live because its manifest exists on disk is the same
// mistake as reporting it live because a script exists on disk, which this check was written to
// stop.
//
// Two things are evidence, and both mean a host really loaded it:
//
//   - CLAUDE_PLUGIN_ROOT, which the host sets for a plugin hook.
//   - An installed plugin under ~/.claude/plugins. The walk never looked there, so a genuine
//     plugin install combined with a metareview on PATH still reported inactive and advised the
//     operator to install the plugin they had already installed.
func pluginRoot(home string) string {
	if env := strings.TrimSpace(os.Getenv("CLAUDE_PLUGIN_ROOT")); env != "" {
		return env
	}
	if home == "" {
		return ""
	}
	// Installed plugins live one or two levels down (<plugins>/<name> or
	// <plugins>/<marketplace>/<name>), so both shapes are checked rather than guessing one.
	base := filepath.Join(home, ".claude", "plugins")
	for _, pattern := range []string{
		filepath.Join(base, "*", "hooks", "hooks.json"),
		filepath.Join(base, "*", "*", "hooks", "hooks.json"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			// It must be METAREVIEW's manifest, not any plugin that happens to register a Stop
			// hook: another plugin's gate says nothing about whether this one runs.
			if ours, _ := stopHookCommandsScoped(m, false, filepath.Dir(filepath.Dir(m))); ours {
				return filepath.Dir(filepath.Dir(m))
			}
		}
	}
	return ""
}
