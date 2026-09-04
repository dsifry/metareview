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
