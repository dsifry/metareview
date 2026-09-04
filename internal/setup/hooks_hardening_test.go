package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	hookassets "github.com/dsifry/metareview"
)

// embeddedHook returns the current embedded body of a materialized hook, so a test can assert the on-disk
// script matches (or has drifted from) what the binary would write.
func embeddedHook(t *testing.T, name string) []byte {
	t.Helper()
	src, ok := gitHookScripts[name]
	if !ok {
		t.Fatalf("no embedded source for hook %q", name)
	}
	b, err := hookassets.GitHookAssets.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Gap A: a materialized script whose CONTENT has drifted from the current embed (an upgraded binary, older
// on-disk hook) must NOT report AlreadyDone, and re-install must REFRESH it — otherwise a hook fix (e.g. the
// #82 per-ref gate) never reaches an already-installed repo.
func TestReinstallRefreshesStaleHookContent(t *testing.T) {
	root, g := tempRepo(t)
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".metareview", "git-hooks")
	prePush := filepath.Join(dir, "pre-push")
	// Simulate an OLDER materialized hook (present + executable, but different content than the current embed).
	if err := os.WriteFile(prePush, []byte("#!/usr/bin/env bash\n# STALE OLD VERSION — no per-ref gate\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan2, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.AlreadyDone {
		t.Fatal("a stale-content hook must NOT report AlreadyDone — the fix would never reach an upgraded repo")
	}
	if err := ApplyHookInstall(root, plan2, false, g); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(prePush)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, embeddedHook(t, "pre-push")) {
		t.Fatalf("re-install must refresh the stale hook to the current embed;\n got: %q", got)
	}
}

// Gap A: --force must ALWAYS re-materialize, even when the plan reports AlreadyDone — the explicit override for
// "rewrite the scripts now" (e.g. a hand-tampered or partially-updated hook).
func TestForceReinstallRematerializesEvenWhenAlreadyDone(t *testing.T) {
	root, g := tempRepo(t)
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	// A fresh plan over the CURRENT install reports AlreadyDone.
	done, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if !done.AlreadyDone {
		t.Fatal("precondition: a current install should report AlreadyDone")
	}
	prePush := filepath.Join(root, ".metareview", "git-hooks", "pre-push")
	if err := os.WriteFile(prePush, []byte("tampered\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// force=true must rewrite despite AlreadyDone.
	if err := ApplyHookInstall(root, done, true, g); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(prePush)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, embeddedHook(t, "pre-push")) {
		t.Fatalf("--force must re-materialize even when AlreadyDone; got %q", got)
	}
}

// Gap B: install must gitignore metareview's EPHEMERAL per-clone state (runs.jsonl, findings.jsonl, git-hooks),
// while leaving the DURABLE learning/knowledge state committable — so a consumer's `git add -A` never commits
// HEAD-bound markers, and a team can still sync calibration.
func TestInstallGitignoresEphemeralStateAllowlist(t *testing.T) {
	root, g := tempRepo(t)
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	ignored := func(p string) bool {
		_, err := g(root, "check-ignore", "-q", p)
		return err == nil
	}
	for _, p := range []string{".metareview/runs.jsonl", ".metareview/findings.jsonl", ".metareview/git-hooks", ".metareview/shards/x"} {
		if !ignored(p) {
			t.Fatalf("ephemeral state %q must be gitignored after install", p)
		}
	}
	for _, p := range []string{".metareview/knowledge/metareview.jsonl", ".metareview/learning-runs.jsonl", ".metareview/calibration.jsonl"} {
		if ignored(p) {
			t.Fatalf("durable learning state %q must remain committable (not ignored)", p)
		}
	}
}

// Gap B for an ALREADY-installed repo: if the hook scripts are byte-current but the .gitignore block is
// missing (an install that predates Gap B, or the user deleted the block), a reinstall must NOT report
// AlreadyDone — it must fall through and restore the block. Without folding gitpolicy.Present into the
// AlreadyDone decision, the ephemeral-state ignore would never reach these repos.
func TestReinstallRestoresMissingGitignoreBlock(t *testing.T) {
	root, g := tempRepo(t)
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-Gap-B install: hooks current, but the block removed from .gitignore.
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan2, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.AlreadyDone {
		t.Fatal("current hooks but a missing gitignore block must NOT be AlreadyDone — the block would never be restored")
	}
	if err := ApplyHookInstall(root, plan2, false, g); err != nil {
		t.Fatal(err)
	}
	if _, err := g(root, "check-ignore", "-q", ".metareview/runs.jsonl"); err != nil {
		t.Fatalf("reinstall must restore the ephemeral-state ignore: %v", err)
	}
}
