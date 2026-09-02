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
	target, _ := filepath.Abs(filepath.Join(root, "hooks", "git"))

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
	if len(plan.Conflicts) == 0 || !strings.Contains(plan.Conflicts[0], foreign) || !strings.Contains(plan.Conflicts[0], "global") {
		t.Fatalf("a foreign GLOBAL core.hooksPath must be a conflict naming it as global: %+v", plan)
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
