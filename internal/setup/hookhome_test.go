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

// #173: the hook scripts are materialized under the user's data home, named by their content, never inside a
// checkout — so no checkout's location can strand core.hooksPath.
func TestHookLocationIsUserLevelAndContentAddressed(t *testing.T) {
	xdg := isolateHooksHome(t)
	target, err := hookTargetDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(target) != filepath.Join(xdg, "metareview", "git-hooks") || !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(filepath.Base(target)) {
		t.Fatalf("target = %q, want <XDG_DATA_HOME>/metareview/git-hooks/<16 hex>", target)
	}
	if again, _ := hookTargetDir(t.TempDir()); again != target {
		t.Fatalf("the location must not depend on the checkout: %q vs %q", again, target)
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
	if _, err := hookTargetDir(""); err == nil {
		t.Fatal("no data home must be an error")
	}
	if _, err := PlanHookInstall(t.TempDir(), nil); err == nil {
		t.Fatal("PlanHookInstall must surface a missing data home")
	}
}

func TestHookContentIDSurfacesAnUnreadableAsset(t *testing.T) {
	orig := readHookAsset
	t.Cleanup(func() { readHookAsset = orig })
	readHookAsset = func(string) ([]byte, error) { return nil, errors.New("gone") }
	if _, err := hookTargetDir(""); err == nil {
		t.Fatal("an unreadable embedded hook must fail the target lookup")
	}
}

// Materializing into the shared directory is atomic per script and cleans up after a failed step.
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

// AC-2.6: a pre-#173 install (core.hooksPath → <checkout>/.metareview/git-hooks) is metareview's, reported stale by
// setup --check, and migrated by install, which then removes the per-checkout copy.
func TestInstallMigratesThePerCheckoutLocation(t *testing.T) {
	isolateHooksHome(t)
	root, g := tempRepo(t)
	old := filepath.Join(root, ".metareview", "git-hooks")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"pre-push", "post-commit"} {
		_ = os.WriteFile(filepath.Join(old, n), []byte(ourGatePrePush), 0o755)
	}
	if _, err := g(root, "config", "--local", "core.hooksPath", old); err != nil {
		t.Fatal(err)
	}
	st := gitGateStatus(root, g)
	if st.Installed || !st.Stale || st.Location != hookTarget(t) || !strings.Contains(st.Remediation, "migrate") {
		t.Fatalf("setup --check must report the earlier location as stale: %+v", st)
	}
	plan, err := PlanHookInstall(root, g)
	if err != nil || len(plan.Conflicts) != 0 {
		t.Fatalf("the per-checkout location is metareview's, not a conflict: %+v %v", plan, err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	if got := hooksPath(t, root, g); got != hookTarget(t) {
		t.Fatalf("core.hooksPath = %q, want %q", got, hookTarget(t))
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("the migrated per-checkout copy must be removed: %v", err)
	}
	if st := gitGateStatus(root, g); !st.Installed || st.Stale || st.Location != hookTarget(t) {
		t.Fatalf("after migration the gate is installed at its location: %+v", st)
	}
}

// Scripts from another metareview version (another content id) are ours; a foreign dir named like ours is not.
func TestAnotherContentIDIsOursAndUninstallKeepsSharedScripts(t *testing.T) {
	xdg := isolateHooksHome(t)
	root, g := tempRepo(t)
	older := filepath.Join(xdg, "metareview", "git-hooks", "0000000000000000")
	if err := os.MkdirAll(older, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(older, "pre-push"), []byte(ourGatePrePush), 0o755)
	if !isOurHookPath(root, older, hookTarget(t)) {
		t.Fatal("another metareview version's scripts are ours")
	}
	foreign := filepath.Join(t.TempDir(), ".metareview", "git-hooks")
	_ = os.MkdirAll(foreign, 0o755)
	_ = os.WriteFile(filepath.Join(foreign, "pre-push"), []byte("#!/bin/sh\necho husky\n"), 0o755)
	if isOurHookPath(root, foreign, hookTarget(t)) {
		t.Fatal("a dir named like ours whose pre-push is not metareview's gate is foreign")
	}
	// Uninstall from the older id: unsets, and leaves the shared directory for other repositories.
	if _, err := g(root, "config", "--local", "core.hooksPath", older); err != nil {
		t.Fatal(err)
	}
	if changed, err := UninstallHookInstall(root, g); err != nil || !changed {
		t.Fatalf("uninstall: %v %v", changed, err)
	}
	if _, err := os.Stat(filepath.Join(older, "pre-push")); err != nil {
		t.Fatal("a shared content-id dir must survive uninstall")
	}
	// Uninstall from the per-checkout location removes that copy: it was this repository's alone.
	perCheckout := filepath.Join(root, ".metareview", "git-hooks")
	_ = os.MkdirAll(perCheckout, 0o755)
	_ = os.WriteFile(filepath.Join(perCheckout, "pre-push"), []byte(ourGatePrePush), 0o755)
	_, _ = g(root, "config", "--local", "core.hooksPath", perCheckout)
	if changed, err := UninstallHookInstall(root, g); err != nil || !changed {
		t.Fatalf("uninstall per-checkout: %v %v", changed, err)
	}
	if _, err := os.Stat(perCheckout); !os.IsNotExist(err) {
		t.Fatal("uninstall must remove the per-checkout copy")
	}
}

// AC-2.4: moving the main checkout (mv + git worktree repair) leaves a linked worktree's hooks resolving to the
// materialized scripts, so its pushes stay gated. With the pre-#173 per-checkout location git silently ran none.
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
	plan, err := PlanHookInstall(wt, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(wt, plan, false, g); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(base, "main-moved")
	if err := os.Rename(main, moved); err != nil {
		t.Fatal(err)
	}
	run(moved, "worktree", "repair", wt)
	hooks := run(wt, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	want, _ := filepath.EvalSymlinks(hookTarget(t))
	if got, _ := filepath.EvalSymlinks(hooks); got != want {
		t.Fatalf("after moving the main checkout the linked worktree's hooks resolve to %q, want %q", hooks, hookTarget(t))
	}
	if !hooksCurrent(hooks) {
		t.Fatal("the hook scripts must still be in place after the move")
	}
}
