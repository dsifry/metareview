package setup

import (
	"encoding/json"
	"os"
	"path"
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
	// ScriptPresent is whether hooks/pre-finish.sh exists AND is executable. Existence alone was
	// not enough: a checkout that lost the mode bit (an archive, a copy, a restrictive umask)
	// reported the script as present while the host could not run it.
	ScriptPresent bool `json:"scriptPresent"`
	// Plugin names the plugin manifest that registers the hook, when that is how it is
	// installed. A correct plugin install writes no settings.json entry at all, so reading only
	// those files reported active:false and advised the operator to install as a plugin — which
	// they had already done.
	Plugin string `json:"pluginManifest,omitempty"`
	// Foreign records a Stop hook that is registered but is not metareview's. It is reported
	// rather than counted, because a linter's Stop hook says nothing about whether the review
	// gate runs, and certifying enforcement on the strength of someone else's hook is the same
	// silent failure this type exists to catch.
	Foreign     string `json:"foreignStopHook,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

// GitGateStatus reports whether the git-native review gate — the pre-push hook that blocks an unreviewed
// push — is installed and current on this clone. This is the PRIMARY, push-time enforcement; the Stop hook
// (EnforcementStatus) is a distinct, additional layer that gates session COMPLETION. setup --check reported
// only the Stop hook, so a repo with the git gate fully installed still read "nothing stops a host... the
// Completion Rule is advisory" — under-stating the posture. Reporting both is what makes the check honest.
type GitGateStatus struct {
	// Installed is true when core.hooksPath points at metareview's materialized, byte-current hook scripts —
	// so `git push` is blocked until the branch is review-clean.
	Installed bool `json:"installed"`
	// HooksPath is this clone's effective core.hooksPath, empty when unset.
	HooksPath   string `json:"hooksPath,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

// gitGateStatus reports the git-native gate's install state, read-only. It reuses PlanHookInstall, whose
// AlreadyDone is true exactly when core.hooksPath is ours AND the materialized scripts are byte-current with
// the embed. A repo that is not usable for git returns not-installed with a reason rather than an error.
func gitGateStatus(root string, git GitRunner) GitGateStatus {
	plan, err := PlanHookInstall(root, git)
	if err != nil {
		return GitGateStatus{Remediation: "Not a usable git repository here, so the git-native review gate cannot be installed."}
	}
	// The gate is ACTIVE when the hooks are ours and current — independent of the .gitignore block, which
	// AlreadyDone also requires. `git push` is gated by the hook whether or not the ignore line exists.
	if plan.HooksCurrent {
		return GitGateStatus{Installed: true, HooksPath: plan.Current}
	}
	// A CONFLICT (a foreign core.hooksPath, or active .git/hooks a redirect would bypass) makes a plain
	// `setup --install-hooks` REFUSE. Surface the reasons so the remediation is actionable, not misleading.
	if len(plan.Conflicts) > 0 {
		return GitGateStatus{
			HooksPath:   plan.Current,
			Remediation: "The git-native review gate is not installed — " + strings.Join(plan.Conflicts, "; ") + ". Resolve that, or run `metareview setup --install-hooks --force` to override.",
		}
	}
	return GitGateStatus{
		HooksPath:   plan.Current,
		Remediation: "The git-native review gate is not installed (or its scripts are stale). Run `metareview setup --install-hooks` so `git push` is blocked until the branch is review-clean.",
	}
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

func enforcementStatus(root, home, pluginRoot string, gitGateInstalled bool) EnforcementStatus {
	s := EnforcementStatus{}
	scriptRoot := root
	// Executable, not merely present: the host runs this file, and a script it cannot execute
	// fails exactly like a missing one while reporting as installed.
	// A plugin install registers through hooks/hooks.json under the PLUGIN root, and its script
	// lives there too, not in the repository being reviewed. Checked first, because it is the
	// installation this project ships and the one the settings files can say nothing about.
	if pluginRoot != "" {
		manifest := filepath.Join(pluginRoot, "hooks", "hooks.json")
		if ours, _ := stopHookCommandsScoped(manifest, false, pluginRoot); ours {
			s.Active, s.Source, s.Plugin = true, manifest, manifest
			scriptRoot = pluginRoot
		}
	}
	if info, err := os.Stat(filepath.Join(scriptRoot, "hooks", "pre-finish.sh")); err == nil &&
		info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
		s.ScriptPresent = true
	}
	for _, path := range hookSettingsFiles(root, home) {
		if s.Active {
			break
		}
		ours, foreign := stopHookCommands(path, root, pluginRoot)
		if ours {
			s.Active, s.Source = true, path
			break
		}
		if foreign != "" && s.Foreign == "" {
			s.Foreign = path
		}
	}
	// The Stop-hook-specific detail. Framed by the git gate below: the two are different enforcement
	// layers. if/else-if rather than a tagless switch: Go's cover profile emits no counter for a
	// tagless-switch case expression, so these boundary mutants would be reported not-covered and stay
	// unkillable (the #104/#106 precedent).
	if !s.Active && s.Foreign != "" {
		s.Remediation = "A Stop hook is registered in " + s.Foreign + " but it does not run metareview, so it does not gate session completion. Add metareview's hook alongside it, or install metareview as a plugin so hooks/hooks.json applies."
	} else if s.Active && !s.ScriptPresent {
		s.Remediation = "A Stop hook is registered in " + s.Source + " but hooks/pre-finish.sh is missing, so the host will report a hook error instead of a review verdict."
	} else if !s.Active && s.ScriptPresent {
		s.Remediation = "hooks/pre-finish.sh exists but no settings file registers it, so it never runs. Add a Stop hook to .claude/settings.json, or install metareview as a plugin so hooks/hooks.json applies."
	} else if !s.Active {
		s.Remediation = "No Stop hook is registered, so session completion (an agent declaring work done) is not gated. Install metareview as a plugin, or add a Stop hook to .claude/settings.json."
	}
	// The git-native gate is the DISTINCT push-time layer. Frame EVERY inactive-Stop-hook state by whether it
	// is installed — so the report never implies pushes are ungated when they are gated, nor claims a push
	// gate that is absent. (An ACTIVE-but-broken Stop hook is handled by its own case and the chmod override.)
	if !s.Active {
		if gitGateInstalled {
			s.Remediation = "The git-native pre-push gate is installed and blocks an unreviewed push; only session completion is not additionally gated. " + s.Remediation
		} else {
			s.Remediation += " The git-native pre-push gate is not installed either, so nothing stops a host from finishing with unresolved blockers — run `metareview setup --install-hooks`."
		}
	}
	if s.Active && !s.ScriptPresent {
		// Overwrites the case above deliberately: an unreadable script is the more specific fact.
		if info, err := os.Stat(filepath.Join(scriptRoot, "hooks", "pre-finish.sh")); err == nil && info.Mode().IsRegular() {
			s.Remediation = "hooks/pre-finish.sh exists but is not executable, so the host cannot run it and will report a hook error instead of a review verdict. chmod +x hooks/pre-finish.sh."
		}
	}
	return s
}

// stopHookCommands reports whether path registers METAREVIEW's Stop hook, and separately whether
// it registers somebody else's.
//
// The distinction is the point. This used to return true for any Stop entry with a non-empty
// command, so an unrelated hook — a formatter, a notifier — certified metareview's enforcement as
// active while no review gate ran at all. That is the same class of silent failure the whole type
// exists to catch, arriving through the check that was supposed to catch it.
//
// A command counts as ours if it invokes the hook script or the binary by name. That is a
// heuristic, and it is deliberately reported rather than assumed: an unrecognised Stop hook
// becomes Foreign, not Active.
func stopHookCommands(path string, roots ...string) (ours bool, foreign string) {
	return stopHookCommandsScoped(path, true, roots...)
}

func stopHookCommandsScoped(path string, allowProject bool, roots ...string) (ours bool, foreign string) {
	data, err := os.ReadFile(path) // #nosec G304 -- a settings path this package constructs
	if err != nil {
		return false, ""
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
		return false, ""
	}
	for _, matcher := range doc.Hooks["Stop"] {
		for _, h := range matcher.Hooks {
			cmd := strings.TrimSpace(h.Command)
			if cmd == "" {
				continue
			}
			if isOursScoped(cmd, allowProject, roots...) {
				return true, ""
			}
			if foreign == "" {
				foreign = cmd
			}
		}
	}
	return false, foreign
}

// isOurs reports whether a Stop-hook command runs metareview's gate, given the roots this check
// is willing to accept a script from. allowProject widens acceptance to the project-root forms;
// the plugin-manifest caller passes false, because a manifest that runs a PROJECT script says
// nothing about whether the plugin it lives in is metareview's.
//
// The command is CLASSIFIED — only the program, or the script operand of a known interpreter, is
// examined. Scanning every token is what let `echo hooks/pre-finish.sh` and `echo pre-finish.sh`
// certify a hook that merely prints the path and never runs the gate. The narrowings, each
// closing a way a previous version certified somebody else's hook:
//
//   - The hook script must be at hooks/pre-finish.sh (a path suffix, not a basename), and under a
//     root this check knows: the accepted roots, the host variables that expand to them, or a
//     relative path the host resolves against the project.
//   - The binary is accepted only as the program being run.
func isOurs(cmd string, roots ...string) bool {
	return isOursScoped(cmd, true, roots...)
}

func isOursScoped(cmd string, allowProject bool, roots ...string) bool {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	// Quotes are STRIPPED throughout, not just trimmed from the ends. A shell-form command like
	// `bash "${CLAUDE_PLUGIN_ROOT}"/hooks/pre-finish.sh` concatenates a quoted variable to an
	// unquoted path, so strings.Fields yields one token with an embedded quote that a
	// trim-the-ends clean left in the middle — and the prefix then failed to match the variable.
	clean := func(t string) string {
		t = strings.NewReplacer(`"`, "", "'", "", "`", "", ")", "").Replace(t)
		return filepath.ToSlash(t)
	}

	// The tokens that are actually EXECUTED, not every argument. The program is always one. If it
	// is a known interpreter, its first non-flag operand is the script it runs; nothing else on
	// the line is. This is what stops `echo hooks/pre-finish.sh` — echo's argument is data, not a
	// program, so it is never classified.
	executed := []string{fields[0]}
	switch path.Base(clean(fields[0])) {
	case "bash", "sh", "zsh", "dash":
		for _, tok := range fields[1:] {
			if strings.HasPrefix(tok, "-") {
				continue
			}
			executed = append(executed, tok)
			break
		}
	}

	// The binary, as the program.
	switch path.Base(clean(fields[0])) {
	case "metareview", "metareview.exe":
		return true
	}

	const rel = "hooks/pre-finish.sh"
	for _, tok := range executed {
		p := clean(tok)
		var prefix string
		// if/else-if rather than a tagless switch so the boundary mutants on these comparisons are
		// killable (the #104/#106 precedent). The separator in "/"+rel is required, though the
		// TrimSuffix would also refuse a match without one (its prefix would then match no accepted
		// root). Kept explicit because the two lines have to agree.
		if p == rel {
			prefix = ""
		} else if strings.HasSuffix(p, "/"+rel) {
			prefix = strings.TrimSuffix(p, "/"+rel)
		} else {
			continue
		}
		switch prefix {
		case "$CLAUDE_PLUGIN_ROOT", "${CLAUDE_PLUGIN_ROOT}":
			return true
		case "$CLAUDE_PROJECT_DIR", "${CLAUDE_PROJECT_DIR}", "", ".":
			// The project-root forms: an absolute project variable, or a relative path the host
			// resolves against the project it is registered in. Refused for plugin-manifest
			// discovery, where a project script proves nothing about the plugin's identity.
			if allowProject {
				return true
			}
		}
		for _, r := range roots {
			if r != "" && prefix == filepath.ToSlash(r) {
				return true
			}
		}
	}
	return false
}
