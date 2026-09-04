package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setup --check must report the git-native gate's install state. A fresh repo → not installed; after
// `--install-hooks` → installed and current. This is the push-time enforcement setup --check previously
// omitted entirely.
func TestGitGateStatusReportsInstallState(t *testing.T) {
	root, g := tempRepo(t)

	before := gitGateStatus(root, g)
	if before.Installed {
		t.Fatalf("a fresh repo must report the git gate as NOT installed: %+v", before)
	}
	if before.Remediation == "" {
		t.Fatal("a not-installed git gate must carry remediation naming `setup --install-hooks`")
	}

	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}

	after := gitGateStatus(root, g)
	if !after.Installed {
		t.Fatalf("after --install-hooks the git gate must report installed: %+v", after)
	}
	if after.HooksPath == "" {
		t.Fatal("an installed git gate must report its hooksPath")
	}

	// Stale scripts (byte-drift from the embed) must read as NOT installed — the same currency PlanHookInstall
	// enforces, surfaced in the report so a reader isn't told an out-of-date gate is fine.
	stalePrePush := filepath.Join(root, ".metareview", "git-hooks", "pre-push")
	if err := os.WriteFile(stalePrePush, []byte("#!/usr/bin/env bash\n# STALE\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if gitGateStatus(root, g).Installed {
		t.Fatal("a stale (content-drifted) materialized hook must report the git gate as not current/installed")
	}
}

// The push gate is active via the pre-push HOOK regardless of the .gitignore block. Reporting installed must
// read the hook state (HooksCurrent), not AlreadyDone (which #96 also gated on the gitpolicy block) — else a
// working gate with the ignore line missing wrongly reads not-installed. (Cursor: git gate under-reports.)
func TestGitGateStatusInstalledEvenWhenGitignoreBlockMissing(t *testing.T) {
	root, g := tempRepo(t)
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	// The user deleted metareview's .gitignore block; the hooks are untouched and still gate `git push`.
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !gitGateStatus(root, g).Installed {
		t.Fatal("the push gate is active via the hook regardless of the .gitignore block; must report installed")
	}
}

// When core.hooksPath is foreign (a conflict `setup --install-hooks` would REFUSE without --force),
// gitGateStatus must surface the conflict and name --force, not the generic install advice. (CodeRabbit +
// Cursor: conflicts dropped.)
func TestGitGateStatusReportsConflict(t *testing.T) {
	root, g := tempRepo(t)
	if _, err := g(root, "config", "--local", "core.hooksPath", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	got := gitGateStatus(root, g)
	if got.Installed {
		t.Fatalf("a foreign core.hooksPath must not report the gate installed: %+v", got)
	}
	if !strings.Contains(got.Remediation, "--force") || !strings.Contains(got.Remediation, "will not override") {
		t.Fatalf("conflict remediation must name the conflict reason and --force, got: %q", got.Remediation)
	}
}

// The git-gate acknowledgment must apply to EVERY inactive-Stop-hook state, not just the bare one: a present-
// but-unregistered pre-finish.sh + an installed git gate must still say the push gate enforces. (CodeRabbit:
// report the installed git gate in every inactive Stop-hook state.)
func TestEnforcementScriptPresentStillAcknowledgesGitGate(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hooks", "pre-finish.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := enforcementStatus(root, t.TempDir(), "", true) // script present, unregistered → inactive; git gate installed
	if got.Active {
		t.Fatalf("precondition: no Stop hook registered, so inactive: %+v", got)
	}
	if !strings.Contains(got.Remediation, "pre-push gate is installed") {
		t.Fatalf("a present-but-unregistered script + installed git gate must acknowledge the push gate: %q", got.Remediation)
	}
	if !strings.Contains(got.Remediation, "never runs") {
		t.Fatalf("must keep the Stop-hook detail (script present but not registered): %q", got.Remediation)
	}
}

// A repo that is not a usable git repository reports not-installed with a reason, never an error/panic.
func TestGitGateStatusOnNonRepo(t *testing.T) {
	got := gitGateStatus(t.TempDir(), nil)
	if got.Installed {
		t.Fatal("a non-repo must not report the git gate installed")
	}
	if got.Remediation == "" {
		t.Fatal("a non-repo must explain why the gate cannot be installed")
	}
}

// The enforcement remediation must ACKNOWLEDGE an installed git gate: with no Stop hook but the git gate
// installed, it must not claim "nothing stops a host" (the push gate does). Without the git gate, the
// original "nothing stops" message stands.
func TestEnforcementRemediationAcknowledgesGitGate(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()

	withGate := enforcementStatus(root, home, "", true)
	if withGate.Active {
		t.Fatalf("precondition: no Stop hook should be active here: %+v", withGate)
	}
	if strings.Contains(withGate.Remediation, "nothing stops") {
		t.Fatalf("with the git gate installed, remediation must not claim nothing stops a host: %q", withGate.Remediation)
	}
	if !strings.Contains(withGate.Remediation, "pre-push gate is installed") {
		t.Fatalf("remediation should acknowledge the installed push gate: %q", withGate.Remediation)
	}

	withoutGate := enforcementStatus(root, home, "", false)
	if !strings.Contains(withoutGate.Remediation, "nothing stops") {
		t.Fatalf("with no git gate and no Stop hook, remediation must say nothing stops the host: %q", withoutGate.Remediation)
	}
}
