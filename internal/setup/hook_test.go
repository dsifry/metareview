package setup

import (
	"os"
	"path/filepath"
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
	got := enforcementStatus(root, t.TempDir())
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
	if got := enforcementStatus(root, t.TempDir()); !got.Active || got.Source == "" {
		t.Errorf("a project-registered Stop hook must be reported active: %+v", got)
	}

	// Registered in the user's own settings instead.
	home := t.TempDir()
	register(t, home, stopHook)
	if got := enforcementStatus(newRepo(t, true), home); !got.Active {
		t.Error("a Stop hook in the user's settings must count")
	}

	// A settings file with hooks but no Stop entry leaves the gate exactly as absent. This is the
	// shape most likely to be mistaken for enforcement, because the file exists and mentions hooks.
	other := newRepo(t, true)
	register(t, other, `{"hooks":{"PreToolUse":[{"matcher":"","hooks":[{"type":"command","command":"x"}]}]}}`)
	if enforcementStatus(other, t.TempDir()).Active {
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
		if enforcementStatus(r, t.TempDir()).Active {
			t.Errorf("%s must not read as a registered gate", name)
		}
	}

	// A registration pointing at a missing script is worse than none: the host reports a hook
	// error rather than a review verdict, so it must be called out separately.
	noScript := newRepo(t, false)
	register(t, noScript, stopHook)
	got = enforcementStatus(noScript, t.TempDir())
	if !got.Active || got.ScriptPresent {
		t.Fatalf("expected registered-but-missing-script: %+v", got)
	}
	if got.Remediation == "" {
		t.Error("a registration with no script must say so")
	}
}
