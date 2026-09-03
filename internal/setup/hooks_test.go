package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// isolatedGit is a GitRunner pinned to a temp HOME with no system config, so a test never reads or writes the
// developer's real global/system git config — the conflict logic reads the EFFECTIVE hooksPath (local >
// global > system), which would otherwise be contaminated by the ambient environment.
func isolatedGit(home string) GitRunner {
	return func(root string, args ...string) ([]byte, error) {
		c := exec.Command("git", append([]string{"-C", root}, args...)...) // #nosec G204 -- test-only, literal args
		c.Env = append(os.Environ(), "HOME="+home, "GIT_CONFIG_NOSYSTEM=1", "XDG_CONFIG_HOME="+filepath.Join(home, "xdg"))
		return c.Output()
	}
}

// tempRepo makes a throwaway repo whose HOME is the repo itself (so `git config --global` is isolated), and
// returns (root, git) where git is the isolated runner to pass to the functions under test.
func tempRepo(t *testing.T) (string, GitRunner) {
	t.Helper()
	root := t.TempDir()
	g := isolatedGit(root)
	if out, err := g(root, "init", "-q", "-b", "main"); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return root, g
}

func hooksPath(t *testing.T, root string, g GitRunner) string {
	t.Helper()
	out, _ := g(root, "config", "--local", "--get", "core.hooksPath")
	return strings.TrimSpace(string(out))
}

// A clean repo installs cleanly: no conflicts, Apply sets core.hooksPath to hooks/git, a second Plan reports
// AlreadyDone, and Uninstall reverses it.
func TestHookInstallCleanRepoRoundTrips(t *testing.T) {
	root, g := tempRepo(t)
	target, _ := filepath.Abs(filepath.Join(root, ".metareview", "git-hooks"))

	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AlreadyDone || len(plan.Conflicts) != 0 || plan.Target != target || plan.Current != "" {
		t.Fatalf("clean repo plan wrong: %+v (want target %s, no conflicts, unset current)", plan, target)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatalf("apply on a clean repo must succeed: %v", err)
	}
	if got := hooksPath(t, root, g); got != target {
		t.Fatalf("core.hooksPath = %q, want %q", got, target)
	}
	if plan2, _ := PlanHookInstall(root, g); !plan2.AlreadyDone {
		t.Fatalf("a second plan must report AlreadyDone, got %+v", plan2)
	}
	changed, err := UninstallHookInstall(root, g)
	if err != nil || !changed {
		t.Fatalf("uninstall of our own hooksPath must change it: changed=%v err=%v", changed, err)
	}
	if got := hooksPath(t, root, g); got != "" {
		t.Fatalf("core.hooksPath should be unset after uninstall, got %q", got)
	}
}

// A LOCAL core.hooksPath set to a different path is a conflict: Plan reports it, Apply refuses without force,
// and Uninstall refuses to touch a value it did not set.
func TestHookInstallRefusesForeignLocalHooksPath(t *testing.T) {
	root, g := tempRepo(t)
	foreign := filepath.Join(root, "my-own-hooks")
	if _, err := g(root, "config", "--local", "core.hooksPath", foreign); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) == 0 || !strings.Contains(plan.Conflicts[0], foreign) {
		t.Fatalf("a foreign local core.hooksPath must be a conflict naming it: %+v", plan)
	}
	// A LOCAL conflict must NOT be described as global/system — that wording is reserved for a value we do
	// not own locally. (Pins the `if local == ""` scope-naming branch.)
	if strings.Contains(plan.Conflicts[0], "a global or system") {
		t.Fatalf("a LOCAL conflict must not be worded as global/system: %+v", plan)
	}
	if err := ApplyHookInstall(root, plan, false, g); err == nil {
		t.Fatal("apply must refuse a conflict without force")
	}
	if got := hooksPath(t, root, g); got != foreign {
		t.Fatalf("a refused install must not change core.hooksPath; got %q", got)
	}
	if err := ApplyHookInstall(root, plan, true, g); err != nil {
		t.Fatalf("apply with force must override the conflict: %v", err)
	}
	if got := hooksPath(t, root, g); got != plan.Target {
		t.Fatalf("forced install must set our target; got %q", got)
	}
	// Uninstall refuses a foreign value it did not set.
	_, _ = g(root, "config", "--local", "core.hooksPath", foreign)
	if changed, err := UninstallHookInstall(root, g); err == nil || changed {
		t.Fatalf("uninstall must refuse a foreign hooksPath: changed=%v err=%v", changed, err)
	}
	if got := hooksPath(t, root, g); got != foreign {
		t.Fatalf("a refused uninstall must not change core.hooksPath; got %q", got)
	}
}

// A GLOBAL core.hooksPath is in effect for the repo, so it must be a conflict too — reading only --local
// silently overrode a user's global hooks (adversarial-review Finding 1).
func TestHookInstallRefusesForeignGlobalHooksPath(t *testing.T) {
	root, g := tempRepo(t)
	foreign := filepath.Join(root, "global-hooks")
	if _, err := g(root, "config", "--global", "core.hooksPath", foreign); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) == 0 || !strings.Contains(plan.Conflicts[0], foreign) || !strings.Contains(plan.Conflicts[0], "a global or system") {
		t.Fatalf("a foreign GLOBAL core.hooksPath must be a conflict worded as global/system: %+v", plan)
	}
	if err := ApplyHookInstall(root, plan, false, g); err == nil {
		t.Fatal("apply must refuse a global conflict without force")
	}
	if got := hooksPath(t, root, g); got != "" {
		t.Fatalf("a refused install must not set a LOCAL core.hooksPath; got %q", got)
	}
}

// Active (non-sample) hooks in .git/hooks are a conflict, because core.hooksPath would silently bypass them.
func TestHookInstallDetectsActiveGitHooks(t *testing.T) {
	root, g := tempRepo(t)
	hookFile := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hookFile, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) == 0 || !strings.Contains(plan.Conflicts[0], "pre-commit") {
		t.Fatalf("an active .git/hooks/pre-commit must be a conflict: %+v", plan)
	}
	// A .sample is NOT active and must not conflict.
	if err := os.Remove(hookFile); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(hookFile+".sample", []byte("#!/bin/sh\n"), 0o755)
	if plan2, _ := PlanHookInstall(root, g); len(plan2.Conflicts) != 0 {
		t.Fatalf(".sample hooks must not conflict: %+v", plan2)
	}
}

// PlanHookInstall must FAIL CLOSED when it cannot inspect the repo: a non-git directory made config/hook
// reads error out, and the old code read those errors as "unset/empty" and returned a conflict-free plan —
// which would let an install proceed where it could not see existing hooks (CodeRabbit Finding: hooks.go:56).
func TestPlanHookInstallFailsClosedOnNonRepo(t *testing.T) {
	root := t.TempDir() // deliberately NOT a git repo
	if _, err := PlanHookInstall(root, isolatedGit(root)); err == nil {
		t.Fatal("PlanHookInstall on a non-git dir must fail closed (error), not return a conflict-free plan")
	}
}

// ApplyHookInstall must re-inspect immediately before writing: a foreign core.hooksPath that appears AFTER
// planning (e.g. across an interactive prompt) must be refused without --force, never silently shadowed
// (CodeRabbit Finding: hooks.go:99).
func TestApplyHookInstallRevalidatesBeforeWrite(t *testing.T) {
	root, g := tempRepo(t)
	plan, err := PlanHookInstall(root, g) // clean plan, no conflicts
	if err != nil {
		t.Fatal(err)
	}
	// A foreign value appears between planning and applying.
	if _, err := g(root, "config", "--local", "core.hooksPath", ".githooks-foreign"); err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err == nil {
		t.Fatal("apply must re-check and refuse a conflict that appeared after planning")
	}
	if got := hooksPath(t, root, g); got != ".githooks-foreign" {
		t.Fatalf("a refused revalidation must not overwrite the value set meanwhile; got %q", got)
	}
	// --force still overrides the changed state.
	if err := ApplyHookInstall(root, plan, true, g); err != nil {
		t.Fatalf("--force must install over the changed state: %v", err)
	}
}

// Passing a nil GitRunner must fall back to the REAL git binary (the `if git == nil` default). The negation
// mutant leaves git nil and the next call nil-derefs, so exercising the nil path across all four entry points
// kills those guards. HOME/config are isolated via t.Setenv so the real runner is deterministic.
func TestHookInstallNilGitUsesRealRunner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	plan, err := PlanHookInstall(root, nil) // nil → realGitRunner
	if err != nil {
		t.Fatal(err)
	}
	if plan.AlreadyDone || len(plan.Conflicts) != 0 {
		t.Fatalf("clean repo via the real runner must have no conflicts: %+v", plan)
	}
	if err := ApplyHookInstall(root, plan, false, nil); err != nil { // nil → realGitRunner
		t.Fatal(err)
	}
	st, err := UninstallPreview(root, nil) // nil → realGitRunner
	if err != nil || !st.WouldChange {
		t.Fatalf("after install, preview via the real runner must report WouldChange: %+v err=%v", st, err)
	}
	if changed, err := UninstallHookInstall(root, nil); err != nil || !changed { // nil → realGitRunner
		t.Fatalf("uninstall via the real runner must change it: changed=%v err=%v", changed, err)
	}
}

// UninstallPreview is read-only truth for the uninstall CLI: it fails closed on a non-repo, and its
// WouldChange is true ONLY when core.hooksPath is metareview's own hooks/git — false when unset or foreign.
func TestUninstallPreviewStates(t *testing.T) {
	// Non-repo → fail closed (pins the rev-parse guard).
	if _, err := UninstallPreview(t.TempDir(), isolatedGit(t.TempDir())); err == nil {
		t.Fatal("UninstallPreview on a non-git dir must fail closed")
	}
	root, g := tempRepo(t)
	target, _ := filepath.Abs(filepath.Join(root, ".metareview", "git-hooks"))
	// Unset → nothing to change.
	if st, err := UninstallPreview(root, g); err != nil || st.WouldChange || st.Current != "" {
		t.Fatalf("unset core.hooksPath: WouldChange must be false; %+v err=%v", st, err)
	}
	// A foreign value → not ours, so WouldChange must be false.
	if _, err := g(root, "config", "--local", "core.hooksPath", filepath.Join(root, "not-ours")); err != nil {
		t.Fatal(err)
	}
	if st, err := UninstallPreview(root, g); err != nil || st.WouldChange {
		t.Fatalf("a foreign core.hooksPath must not be WouldChange: %+v err=%v", st, err)
	}
	// Our own hooks/git → WouldChange true.
	if _, err := g(root, "config", "--local", "core.hooksPath", target); err != nil {
		t.Fatal(err)
	}
	if st, err := UninstallPreview(root, g); err != nil || !st.WouldChange || st.Current != target {
		t.Fatalf("metareview's own hooks/git must be WouldChange with Current set: %+v err=%v", st, err)
	}
}

// The gate must reach a CONSUMER repo that has no committed hooks/git of its own: install MATERIALIZES the
// embedded scripts into .metareview/git-hooks (executable) and points core.hooksPath there, so the gate is
// genuinely active — not the old bug where core.hooksPath named a non-existent dir and the CLI still said
// "active". Uninstall then removes the materialized dir.
func TestHookInstallMaterializesHooksInConsumerRepo(t *testing.T) {
	root, g := tempRepo(t)
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".metareview", "git-hooks")
	for _, name := range []string{"pre-push", "post-commit"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("hook %s must be materialized in a consumer repo: %v", name, err)
		}
		if info.Mode()&0o100 == 0 {
			t.Fatalf("materialized hook %s must be executable; mode=%v", name, info.Mode())
		}
	}
	if got := hooksPath(t, root, g); got != plan.Target {
		t.Fatalf("core.hooksPath = %q, want the materialized target %q", got, plan.Target)
	}
	if _, err := UninstallHookInstall(root, g); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("uninstall must remove the materialized hooks dir; stat err = %v", err)
	}
}

// An older metareview clone whose core.hooksPath still points at the LEGACY committed hooks/git is ours, not
// foreign: install UPGRADES it to the materialized target instead of refusing, and uninstall removes it too.
func TestHookInstallUpgradesLegacyTarget(t *testing.T) {
	root, g := tempRepo(t)
	legacy, _ := filepath.Abs(filepath.Join(root, "hooks", "git"))
	// A GENUINE metareview legacy install: hooks/git holds metareview's own pre-push (content marker), which
	// is how ownership is decided — not the path alone.
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "pre-push"), []byte("#!/usr/bin/env bash\n# metareview review gate --push\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := g(root, "config", "--local", "core.hooksPath", legacy); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) != 0 {
		t.Fatalf("the legacy metareview target must not be a conflict: %+v", plan)
	}
	if plan.AlreadyDone {
		t.Fatal("the legacy target is not AlreadyDone — it must upgrade to the materialized target")
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	if got := hooksPath(t, root, g); got != plan.Target {
		t.Fatalf("install must upgrade the legacy value to the new target; got %q want %q", got, plan.Target)
	}
	// Uninstall recognises the legacy value as ours too.
	if _, err := g(root, "config", "--local", "core.hooksPath", legacy); err != nil {
		t.Fatal(err)
	}
	if changed, err := UninstallHookInstall(root, g); err != nil || !changed {
		t.Fatalf("uninstall must remove the legacy value: changed=%v err=%v", changed, err)
	}
}

// If the hook scripts cannot be materialized (here: .metareview is a FILE, so mkdir fails), install must
// FAIL and must NOT set core.hooksPath — never leave the gate pointing at an empty/absent dir while claiming
// it is active (the exact consumer-repo bug this whole change fixes).
func TestApplyHookInstallFailsWhenHooksCannotMaterialize(t *testing.T) {
	root, g := tempRepo(t)
	if err := os.WriteFile(filepath.Join(root, ".metareview"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err == nil {
		t.Fatal("install must fail when the hook scripts cannot be written")
	}
	if got := hooksPath(t, root, g); got != "" {
		t.Fatalf("a failed materialize must not set core.hooksPath; got %q", got)
	}
}

// A consumer repo's OWN hooks/git must NOT be claimed as metareview's: ownership is the pre-push content
// marker, not the path. Install refuses (conflict) and uninstall leaves it (CodeRabbit Major: hooks.go:210).
func TestHookInstallDoesNotClaimForeignLegacyHooks(t *testing.T) {
	root, g := tempRepo(t)
	foreign, _ := filepath.Abs(filepath.Join(root, "hooks", "git"))
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "pre-push"), []byte("#!/bin/sh\n# the repo's own release hook\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := g(root, "config", "--local", "core.hooksPath", foreign); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) == 0 {
		t.Fatalf("a foreign hooks/git must be a conflict, not claimed as ours: %+v", plan)
	}
	if err := ApplyHookInstall(root, plan, false, g); err == nil {
		t.Fatal("install must refuse a foreign hooks/git without --force")
	}
	if changed, err := UninstallHookInstall(root, g); err == nil || changed {
		t.Fatalf("uninstall must refuse a foreign hooks/git: changed=%v err=%v", changed, err)
	}
}

// A second install when the materialized scripts have been DELETED (they are git-ignored, so this is
// ordinary) must REMATERIALIZE them — not report "already installed" over an inert gate (Cursor High:
// hooks.go:128).
func TestReinstallRematerializesMissingHooks(t *testing.T) {
	root, g := tempRepo(t)
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".metareview", "git-hooks")
	if err := os.RemoveAll(dir); err != nil { // the scripts vanish, core.hooksPath still points here
		t.Fatal(err)
	}
	plan2, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.AlreadyDone {
		t.Fatal("with the materialized scripts gone, install must NOT report AlreadyDone")
	}
	if err := ApplyHookInstall(root, plan2, false, g); err != nil {
		t.Fatal(err)
	}
	if !hooksMaterialized(dir) {
		t.Fatal("reinstall must rematerialize the missing hooks")
	}
}

// A clone bitten by the ORIGINAL bug has core.hooksPath pointing at a NON-EXISTENT hooks/git. That is not a
// consumer's hook set to protect, so re-install must RECLAIM it (no conflict, no --force) and rematerialize
// — otherwise those broken clones stay ungated forever (Cursor High: hooks.go:228).
func TestHookInstallRecoversBrokenLegacyInstall(t *testing.T) {
	root, g := tempRepo(t)
	brokenLegacy, _ := filepath.Abs(filepath.Join(root, "hooks", "git")) // deliberately never created
	if _, err := g(root, "config", "--local", "core.hooksPath", brokenLegacy); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) != 0 {
		t.Fatalf("a non-existent legacy hooks/git must not be a conflict: %+v", plan)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatalf("re-install must recover a broken install without --force: %v", err)
	}
	if got := hooksPath(t, root, g); got != plan.Target {
		t.Fatalf("recovery must point core.hooksPath at the materialized target; got %q", got)
	}
	if !hooksMaterialized(plan.Target) {
		t.Fatal("recovery must materialize the hooks")
	}
}

// A consumer's OWN hooks/git that EXISTS but has no metareview pre-push (e.g. it holds only commit-msg) must
// NOT be claimed — install refuses (conflict), uninstall leaves it. Only an entirely-absent dir is reclaimed
// (CodeRabbit/Cursor: hooks.go — do not claim an existing legacy dir without the marker).
func TestHookInstallDoesNotClaimConsumerDirWithoutPrePush(t *testing.T) {
	root, g := tempRepo(t)
	consumer, _ := filepath.Abs(filepath.Join(root, "hooks", "git"))
	if err := os.MkdirAll(consumer, 0o755); err != nil {
		t.Fatal(err)
	}
	// The consumer's own hook — no pre-push at all.
	if err := os.WriteFile(filepath.Join(consumer, "commit-msg"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := g(root, "config", "--local", "core.hooksPath", consumer); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) == 0 {
		t.Fatalf("an existing consumer hooks/git without our marker must be a conflict: %+v", plan)
	}
	if changed, err := UninstallHookInstall(root, g); err == nil || changed {
		t.Fatalf("uninstall must not touch a consumer's hooks/git: changed=%v err=%v", changed, err)
	}
}

// Install keeps the per-clone materialized hooks out of a consumer's commits by adding a .gitignore entry
// (the docs promise they are not committed). It is idempotent and a no-op when already ignored (Cursor:
// INSTALL.md — consumer gitignore does not cover hooks).
func TestHookInstallGitignoresMaterializedHooks(t *testing.T) {
	root, g := tempRepo(t) // fresh repo: no .gitignore yet
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	if _, err := g(root, "check-ignore", "-q", ".metareview/git-hooks"); err != nil {
		t.Fatalf("install must cause .metareview/git-hooks to be git-ignored: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if strings.Count(string(body), ".metareview/git-hooks/") != 1 {
		t.Fatalf(".gitignore must have exactly one entry; got %q", body)
	}
	// Reinstall (scripts removed → not AlreadyDone) must NOT duplicate the entry.
	if err := os.RemoveAll(plan.Target); err != nil {
		t.Fatal(err)
	}
	plan2, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan2, false, g); err != nil {
		t.Fatal(err)
	}
	body2, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if strings.Count(string(body2), ".metareview/git-hooks/") != 1 {
		t.Fatalf(".gitignore entry must not be duplicated on reinstall; got %q", body2)
	}
}
