package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// #173: each repository's hook scripts are materialized under the user's data home, in a dir named by an id kept in
// the repository's own config — never inside a checkout, so no checkout's location can strand core.hooksPath.
func TestHookLocationIsUserLevelPerRepository(t *testing.T) {
	xdg := isolateHooksHome(t)
	root, g := tempRepo(t)
	other, og := tempRepo(t)
	target := hookTarget(t, root, g)
	if filepath.Dir(target) != filepath.Join(xdg, "metareview", "git-hooks") || !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(filepath.Base(target)) {
		t.Fatalf("target = %q, want <XDG_DATA_HOME>/metareview/git-hooks/<16 hex>", target)
	}
	if again := hookTarget(t, root, g); again != target {
		t.Fatalf("a read-only plan must be stable before install: %q vs %q", again, target)
	}
	if hookTarget(t, other, og) == target {
		t.Fatal("two repositories must not share a hook dir")
	}
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	if out, _ := g(root, "config", "--local", "--get", HooksIDKey); strings.TrimSpace(string(out)) != filepath.Base(target) {
		t.Fatalf("install must record %s = %s, got %q", HooksIDKey, filepath.Base(target), out)
	}

	// XDG_DATA_HOME unset or relative → ~/.local/share; no home → an error, never a guess.
	home := t.TempDir()
	for _, v := range []string{"", "relative/data"} {
		t.Setenv("XDG_DATA_HOME", v)
		orig := userHomeDir
		userHomeDir = func() (string, error) { return home, nil }
		got, err := hooksHome()
		userHomeDir = orig
		if err != nil || got != filepath.Join(home, ".local", "share", "metareview", "git-hooks") {
			t.Fatalf("XDG_DATA_HOME=%q: got %q %v", v, got, err)
		}
	}
	orig := userHomeDir
	t.Cleanup(func() { userHomeDir = orig })
	userHomeDir = func() (string, error) { return "", errors.New("no home") }
	if _, err := PlanHookInstall(root, g); err == nil {
		t.Fatal("PlanHookInstall must surface a missing data home")
	}
}

// A recorded id that is not one metareview writes is ignored in favour of the derived one; a repository git cannot
// name the common directory of is an error.
func TestRepoHooksIDIgnoresJunkAndSurfacesGitErrors(t *testing.T) {
	isolateHooksHome(t)
	root, g := tempRepo(t)
	derived := hookTarget(t, root, g)
	if _, err := g(root, "config", "--local", HooksIDKey, "../../etc"); err != nil {
		t.Fatal(err)
	}
	if got := hookTarget(t, root, g); got != derived {
		t.Fatalf("a junk %s must be ignored: got %q want %q", HooksIDKey, got, derived)
	}
	broken := func(root string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "rev-parse" && strings.Contains(strings.Join(args, " "), "git-common-dir") {
			return nil, errors.New("no common dir")
		}
		return g(root, args...)
	}
	if _, err := repoHooksID(root, broken); err == nil {
		t.Fatal("an unresolvable common dir must be an error")
	}
	if _, err := PlanHookInstall(root, broken); err == nil {
		t.Fatal("PlanHookInstall must surface it")
	}
	if _, err := UninstallPreview(root, broken); err == nil {
		t.Fatal("UninstallPreview must surface it")
	}
	if _, err := UninstallHookInstall(root, broken); err == nil {
		t.Fatal("UninstallHookInstall must surface it")
	}
}

// Materializing is atomic per script and cleans up after a failed step.
func TestMaterializeHooksCleansUpOnFailure(t *testing.T) {
	dir := t.TempDir()
	orig := osChmod
	t.Cleanup(func() { osChmod = orig })
	osChmod = func(string, os.FileMode) error { return errors.New("chmod failed") }
	if err := materializeHooks(dir); err == nil {
		t.Fatal("a failed chmod must fail materialization")
	}
	osChmod = orig
	// A directory where the temporary script goes: the write itself fails.
	for _, n := range []string{"pre-push", "post-commit"} {
		blocker := filepath.Join(dir, fmt.Sprintf("%s.tmp-%d", n, os.Getpid()))
		_ = os.Mkdir(blocker, 0o755)
		_ = os.WriteFile(filepath.Join(blocker, "x"), []byte("x"), 0o644)
	}
	if err := materializeHooks(dir); err == nil {
		t.Fatal("a failed write must fail materialization")
	}
	for _, n := range []string{"pre-push", "post-commit"} {
		_ = os.RemoveAll(filepath.Join(dir, fmt.Sprintf("%s.tmp-%d", n, os.Getpid())))
	}
	if err := os.Mkdir(filepath.Join(dir, "pre-push"), 0o755); err != nil { // a directory in the way of the rename
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "pre-push", "x"), []byte("x"), 0o644) // non-empty: the rename cannot replace it
	if err := materializeHooks(dir); err == nil {
		t.Fatal("a failed rename must fail materialization")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("a failed step must not leave %s behind", e.Name())
		}
	}
}

// ourGatePrePush is a pre-push body carrying metareview's ownership marker.
const ourGatePrePush = "#!/bin/sh\n# metareview review gate --push\nexit 0\n"

func writeGateDir(t *testing.T, dir string, extra ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range append([]string{"pre-push", "post-commit"}, extra...) {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(ourGatePrePush), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// AC-2.6: a pre-#173 install (core.hooksPath → <checkout>/.metareview/git-hooks) is metareview's, reported stale by
// setup --check, and migrated by install, which then removes the per-checkout copy.
func TestInstallMigratesThePerCheckoutLocation(t *testing.T) {
	isolateHooksHome(t)
	root, g := tempRepo(t)
	old := filepath.Join(root, ".metareview", "git-hooks")
	writeGateDir(t, old)
	if _, err := g(root, "config", "--local", "core.hooksPath", old); err != nil {
		t.Fatal(err)
	}
	st := gitGateStatus(root, g)
	if st.Installed || !st.Stale || st.Location != hookTarget(t, root, g) || !strings.Contains(st.Remediation, "migrate") {
		t.Fatalf("setup --check must report the earlier location as stale: %+v", st)
	}
	plan, err := PlanHookInstall(root, g)
	if err != nil || len(plan.Conflicts) != 0 {
		t.Fatalf("the per-checkout location is metareview's, not a conflict: %+v %v", plan, err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	if got := hooksPath(t, root, g); got != hookTarget(t, root, g) {
		t.Fatalf("core.hooksPath = %q, want %q", got, hookTarget(t, root, g))
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("the migrated per-checkout copy must be removed: %v", err)
	}
	if st := gitGateStatus(root, g); !st.Installed || st.Stale {
		t.Fatalf("after migration the gate is installed at its location: %+v", st)
	}
}

// Moving core.hooksPath off an earlier location would silently stop any other hook kept there: that is a conflict.
func TestMigrationRefusesToDropOtherHooks(t *testing.T) {
	isolateHooksHome(t)
	root, g := tempRepo(t)
	old := filepath.Join(root, ".metareview", "git-hooks")
	writeGateDir(t, old, "commit-msg")
	_, _ = g(root, "config", "--local", "core.hooksPath", old)
	plan, err := PlanHookInstall(root, g)
	if err != nil || len(plan.Conflicts) != 1 || !strings.Contains(plan.Conflicts[0], "commit-msg") {
		t.Fatalf("a hook metareview does not own must block the move: %+v %v", plan, err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err == nil {
		t.Fatal("install must refuse without --force")
	}
	if _, err := os.Stat(filepath.Join(old, "commit-msg")); err != nil {
		t.Fatal("the other hook must survive the refusal")
	}
}

// A user-level dir materialized under ANOTHER data home (XDG_DATA_HOME differs between shells) is still ours, by
// shape and marker; a dir of the same shape without the gate is not.
func TestAHookDirFromAnotherDataHomeIsOurs(t *testing.T) {
	isolateHooksHome(t)
	root, g := tempRepo(t)
	elsewhere := filepath.Join(t.TempDir(), "metareview", "git-hooks", "0123456789abcdef")
	writeGateDir(t, elsewhere)
	if !isOurHookPath(root, elsewhere, hookTarget(t, root, g)) {
		t.Fatal("a metareview hook dir under another data home is ours")
	}
	impostor := filepath.Join(t.TempDir(), "metareview", "git-hooks", "fedcba9876543210")
	_ = os.MkdirAll(impostor, 0o755)
	_ = os.WriteFile(filepath.Join(impostor, "pre-push"), []byte("#!/bin/sh\necho mine\n"), 0o755)
	if isOurHookPath(root, impostor, hookTarget(t, root, g)) {
		t.Fatal("a dir shaped like ours whose pre-push is not the gate is foreign")
	}
	foreign := filepath.Join(t.TempDir(), ".metareview", "git-hooks")
	_ = os.MkdirAll(foreign, 0o755)
	_ = os.WriteFile(filepath.Join(foreign, "pre-push"), []byte("#!/bin/sh\necho husky\n"), 0o755)
	if isOurHookPath(root, foreign, hookTarget(t, root, g)) {
		t.Fatal("a per-checkout-shaped dir whose pre-push is not the gate is foreign")
	}
}

// Uninstall takes metareview's scripts out of the repository's hook dir and forgets the id, but never removes a hook
// someone else put there; an otherwise empty dir goes.
func TestUninstallKeepsOtherHooksAndForgetsTheID(t *testing.T) {
	isolateHooksHome(t)
	root, g := tempRepo(t)
	plan, _ := PlanHookInstall(root, g)
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	target := plan.Target
	if err := os.WriteFile(filepath.Join(target, "commit-msg"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if changed, err := UninstallHookInstall(root, g); err != nil || !changed {
		t.Fatalf("uninstall: %v %v", changed, err)
	}
	if _, err := os.Stat(filepath.Join(target, "pre-push")); !os.IsNotExist(err) {
		t.Fatal("uninstall must remove metareview's scripts")
	}
	if _, err := os.Stat(filepath.Join(target, "commit-msg")); err != nil {
		t.Fatal("uninstall must keep a hook it does not own")
	}
	if out, _ := g(root, "config", "--local", "--get", HooksIDKey); strings.TrimSpace(string(out)) != "" {
		t.Fatalf("uninstall must forget %s, got %q", HooksIDKey, out)
	}
	// Reinstall and uninstall with nothing else there: the dir goes too.
	_ = os.Remove(filepath.Join(target, "commit-msg"))
	plan, _ = PlanHookInstall(root, g)
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	if _, err := UninstallHookInstall(root, g); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plan.Target); !os.IsNotExist(err) {
		t.Fatal("an emptied hook dir must be removed")
	}
}

// AC-2.4 and AC-2.3 (hook half): install from the MAIN checkout, then move it and repair the linked worktree. Before
// #173 core.hooksPath named <main>/.metareview/git-hooks, so the move silently ungated the worktree. The recorded id
// keeps the location, and `git clean -fdX` in the moved checkout touches no hook.
func TestMovingTheMainCheckoutKeepsLinkedWorktreesGated(t *testing.T) {
	isolateHooksHome(t)
	base, _ := filepath.EvalSymlinks(t.TempDir())
	main := filepath.Join(base, "main")
	g := isolatedGit(base)
	run := func(dir string, args ...string) string {
		t.Helper()
		out, err := g(dir, args...)
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	run(main, "init", "-q", "-b", "main")
	run(main, "commit", "-q", "--allow-empty", "-m", "base")
	wt := filepath.Join(base, "wt")
	run(main, "worktree", "add", "-q", "-b", "feat", wt)
	plan, err := PlanHookInstall(main, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(main, plan, false, g); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(base, "main-moved")
	if err := os.Rename(main, moved); err != nil {
		t.Fatal(err)
	}
	run(moved, "worktree", "repair", wt)
	run(moved, "clean", "-fdX")
	want, _ := filepath.EvalSymlinks(plan.Target)
	for _, dir := range []string{wt, moved} {
		hooks := run(dir, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
		if got, _ := filepath.EvalSymlinks(hooks); got != want {
			t.Fatalf("after moving the main checkout, %s's hooks resolve to %q, want %q", dir, hooks, want)
		}
	}
	if !hooksCurrent(plan.Target) {
		t.Fatal("the hook scripts must still be in place after the move and git clean")
	}
	if again, _ := hookTargetDir(moved, g); again != plan.Target {
		t.Fatalf("the recorded id must survive the move: %q vs %q", again, plan.Target)
	}
}
