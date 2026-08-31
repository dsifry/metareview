package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The check that did not exist. hooks/hooks.json is a plugin manifest, so in a source checkout
// the Stop hook was a file no host ever loaded — the enforcement layer had never run once, and
// `metareview setup` reported every prerequisite satisfied while it hadn't. Nothing could report
// the gap: the hook could not say it was absent, because it was absent.
func TestSetupReportsWhetherEnforcementIsActive(t *testing.T) {
	newRepo := func(t *testing.T, script bool) string {
		t.Helper()
		root := t.TempDir()
		if script {
			if err := os.MkdirAll(filepath.Join(root, "hooks"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "hooks", "pre-finish.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}
	register := func(t *testing.T, dir, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const stopHook = `{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"$CLAUDE_PROJECT_DIR/hooks/pre-finish.sh"}]}]}}`

	// The state this repository was actually in: script present, nothing registering it.
	root := newRepo(t, true)
	got := enforcementStatus(root, t.TempDir(), "")
	if got.Active {
		t.Error("a hook script with no registration is not active")
	}
	if !got.ScriptPresent {
		t.Error("the script is present and must be reported so")
	}
	if got.Remediation == "" {
		t.Error("an inactive gate must say how to turn it on")
	}

	// Registered in project settings.
	register(t, root, stopHook)
	if got := enforcementStatus(root, t.TempDir(), ""); !got.Active || got.Source == "" {
		t.Errorf("a project-registered Stop hook must be reported active: %+v", got)
	}

	// Registered in the user's own settings instead.
	home := t.TempDir()
	register(t, home, stopHook)
	if got := enforcementStatus(newRepo(t, true), home, ""); !got.Active {
		t.Error("a Stop hook in the user's settings must count")
	}

	// A settings file with hooks but no Stop entry leaves the gate exactly as absent. This is the
	// shape most likely to be mistaken for enforcement, because the file exists and mentions hooks.
	other := newRepo(t, true)
	register(t, other, `{"hooks":{"PreToolUse":[{"matcher":"","hooks":[{"type":"command","command":"x"}]}]}}`)
	if enforcementStatus(other, t.TempDir(), "").Active {
		t.Error("hooks that are not Stop hooks do not enforce the Completion Rule")
	}
	for name, body := range map[string]string{
		"empty object":       `{}`,
		"empty Stop list":    `{"hooks":{"Stop":[]}}`,
		"Stop with no entry": `{"hooks":{"Stop":[{"matcher":"","hooks":[]}]}}`,
		"blank command":      `{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"  "}]}]}}`,
		"not json":           `this is not settings`,
	} {
		r := newRepo(t, true)
		register(t, r, body)
		if enforcementStatus(r, t.TempDir(), "").Active {
			t.Errorf("%s must not read as a registered gate", name)
		}
	}

	// A registration pointing at a missing script is worse than none: the host reports a hook
	// error rather than a review verdict, so it must be called out separately.
	noScript := newRepo(t, false)
	register(t, noScript, stopHook)
	got = enforcementStatus(noScript, t.TempDir(), "")
	if !got.Active || got.ScriptPresent {
		t.Fatalf("expected registered-but-missing-script: %+v", got)
	}
	if got.Remediation == "" {
		t.Error("a registration with no script must say so")
	}
}

// This check CERTIFIES the enforcement layer, so a false positive here is worse than no check:
// `metareview setup` reports enforcement active while nothing enforces anything, and the operator
// has been told the opposite of the truth by the component whose whole job is catching that.
func TestEnforcementIsNotCertifiedByHooksThatCannotEnforceIt(t *testing.T) {
	repoWith := func(t *testing.T, mode os.FileMode, settings string) string {
		t.Helper()
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "hooks"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "hooks", "pre-finish.sh"), []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
		if settings != "" {
			if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(settings), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}
	const ours = `{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"$CLAUDE_PROJECT_DIR/hooks/pre-finish.sh"}]}]}}`
	const theirs = `{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"npx prettier --write ."}]}]}}`

	// Somebody else's Stop hook enforces nothing about reviews. It used to set Active, because the
	// check accepted ANY Stop entry with a non-empty command.
	got := enforcementStatus(repoWith(t, 0o755, theirs), t.TempDir(), "")
	if got.Active {
		t.Error("a formatter's Stop hook must not certify metareview's enforcement as active")
	}
	if got.Foreign == "" {
		t.Error("the foreign hook must be reported, not silently ignored")
	}
	if !strings.Contains(got.Remediation, "does not run metareview") {
		t.Errorf("the remediation must say what is actually wrong: %q", got.Remediation)
	}

	// A script the host cannot execute fails exactly like a missing one, and used to report as
	// present because only os.Stat's IsRegular was checked. An archive, a copy or a restrictive
	// umask is enough to lose the bit.
	got = enforcementStatus(repoWith(t, 0o644, ours), t.TempDir(), "")
	if got.ScriptPresent {
		t.Error("a non-executable hook script is not a usable one")
	}
	if !strings.Contains(got.Remediation, "chmod +x") {
		t.Errorf("the remediation must name the fix: %q", got.Remediation)
	}

	// And the working case still works, so the tightening did not just break the check.
	got = enforcementStatus(repoWith(t, 0o755, ours), t.TempDir(), "")
	if !got.Active || !got.ScriptPresent || got.Remediation != "" || got.Foreign != "" {
		t.Errorf("a correctly installed hook must certify cleanly: %+v", got)
	}
}

// A plugin install is the installation this project ships, and it was the one the check could not
// see. hooks/hooks.json registers the Stop hook under CLAUDE_PLUGIN_ROOT and writes no
// settings.json entry at all, so reading only the settings files reported active:false with
// remediation advising the operator to install as a plugin — which they had already done. Being
// told to do the thing you have done is worse than no advice.
func TestAPluginInstallCountsAsEnforcement(t *testing.T) {
	plugin := t.TempDir()
	if err := os.MkdirAll(filepath.Join(plugin, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"${CLAUDE_PLUGIN_ROOT}/hooks/pre-finish.sh"}]}]}}`
	if err := os.WriteFile(filepath.Join(plugin, "hooks", "hooks.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugin, "hooks", "pre-finish.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// The repository under review has no hook of its own — the ordinary case for a plugin user.
	got := enforcementStatus(t.TempDir(), t.TempDir(), plugin)
	if !got.Active {
		t.Error("a plugin install registers the hook and must count as active")
	}
	if got.Plugin == "" {
		t.Error("the report must name the manifest that registered it")
	}
	// The script lives in the PLUGIN, not in the repository being reviewed.
	if !got.ScriptPresent {
		t.Error("the plugin's own script is the one that runs, and it is present")
	}
	if got.Remediation != "" {
		t.Errorf("a working install needs no remediation: %q", got.Remediation)
	}

	// A manifest that registers somebody else's hook is not metareview's enforcement.
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "hooks", "hooks.json"),
		[]byte(`{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"npx prettier ."}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if enforcementStatus(t.TempDir(), t.TempDir(), other).Active {
		t.Error("another plugin's Stop hook must not certify metareview's gate")
	}
}

// The command match is on whole path components, not a substring.
//
// `strings.Contains(cmd, "metareview")` was true for any absolute path under a directory of that
// name — and this project is developed in ~/Developer/metareview, so a formatter's Stop hook in
// this very checkout would have certified the gate as active.
func TestOnlyMetareviewsOwnCommandCertifiesTheGate(t *testing.T) {
	const repoRoot = "/Users/dev/project"
	for cmd, want := range map[string]bool{
		"$CLAUDE_PROJECT_DIR/hooks/pre-finish.sh":     true,
		"${CLAUDE_PLUGIN_ROOT}/hooks/pre-finish.sh":   true,
		"/usr/local/bin/metareview status --json":     true,
		"metareview status --json":                    true,
		"bash /Users/dev/project/hooks/pre-finish.sh": true,
		// The trap: a command that merely lives under a directory called metareview.
		"/Users/dev/Developer/metareview/node_modules/.bin/prettier --write .": false,
		"npx prettier --write .":     false,
		"echo 'metareview is great'": false,
		"":                           false,
		// Named like ours, belonging to nobody we know. Matching the basename alone meant any of
		// these certified a foreign Stop hook as metareview's enforcement — the failure this
		// function exists to prevent, reached through the fix for it.
		"/tmp/pre-finish.sh":               false,
		"echo pre-finish.sh":               false,
		"/opt/other/hooks/pre-finish.sh":   false,
		"bash /tmp/evil/pre-finish.sh":     false,
		"/tmp/hooks/pre-finish.sh --quiet": false,
	} {
		if got := isOurs(cmd, repoRoot); got != want {
			t.Errorf("isOurs(%q) = %v, want %v", cmd, got, want)
		}
	}
}

// A source checkout must NOT be mistaken for a live plugin install, and a real one must be found.
//
// The first version walked up from the executable looking for hooks/hooks.json. This repository
// ships that manifest, so `./bin/metareview setup --check` in an ordinary checkout certified
// enforcement as active although no host had ever loaded the plugin — reporting a gate live
// because its manifest exists on disk, which is the mistake this check exists to stop. The same
// walk never looked under ~/.claude/plugins, so a genuine install plus a PATH binary still
// reported inactive and advised installing the plugin the operator already had.
func TestPluginRootRequiresEvidenceOfAnActualInstall(t *testing.T) {
	// A source checkout: the manifest is present, but nothing loaded it.
	checkout := t.TempDir()
	if err := os.MkdirAll(filepath.Join(checkout, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"${CLAUDE_PLUGIN_ROOT}/hooks/pre-finish.sh"}]}]}}`
	if err := os.WriteFile(filepath.Join(checkout, "hooks", "hooks.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")
	emptyHome := t.TempDir()
	if got := pluginRoot(emptyHome); got != "" {
		t.Errorf("a checkout carrying the manifest is not an install: %q", got)
	}

	// A real install under ~/.claude/plugins is found.
	home := t.TempDir()
	installed := filepath.Join(home, ".claude", "plugins", "metareview")
	if err := os.MkdirAll(filepath.Join(installed, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed, "hooks", "hooks.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := pluginRoot(home); got != installed {
		t.Errorf("pluginRoot = %q, want the installed plugin %q", got, installed)
	}

	// Another plugin's Stop hook is not metareview's enforcement.
	other := t.TempDir()
	stranger := filepath.Join(other, ".claude", "plugins", "someone-else")
	if err := os.MkdirAll(filepath.Join(stranger, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stranger, "hooks", "hooks.json"),
		[]byte(`{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"npx prettier ."}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := pluginRoot(other); got != "" {
		t.Errorf("another plugin's Stop hook is not ours: %q", got)
	}

	// The host's own signal wins, because it means a hook is actually running from there.
	t.Setenv("CLAUDE_PLUGIN_ROOT", checkout)
	if got := pluginRoot(emptyHome); got != checkout {
		t.Errorf("CLAUDE_PLUGIN_ROOT is the host saying it loaded the plugin: %q", got)
	}
}
