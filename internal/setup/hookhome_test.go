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
	if _, err := repoHooksID(root, broken, t.TempDir()); err == nil {
		t.Fatal("an unresolvable common dir must be an error")
	}
	if _, err := PlanHookInstall(root, broken); err == nil {
		t.Fatal("PlanHookInstall must surface it")
	}
	if _, err := UninstallPreview(root, broken); err == nil {
		t.Fatal("UninstallPreview must surface it")
	}
	_, _ = g(root, "config", "--local", "core.hooksPath", derived)
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
	if !st.Installed || !st.Stale || st.Location != hookTarget(t, root, g) || !strings.Contains(st.Remediation, "still gates") || !strings.Contains(st.Remediation, "migrate") {
		t.Fatalf("setup --check must report the earlier location as installed (it still gates) and stale: %+v", st)
	}
	// ...and one that is gone as stale and NOT installed.
	_ = os.RemoveAll(old)
	if st := gitGateStatus(root, g); st.Installed || !st.Stale || strings.Contains(st.Remediation, "still gates") {
		t.Fatalf("an earlier location without its gate is not installed: %+v", st)
	}
	writeGateDir(t, old)
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
	// The old location still gates `git push`: setup --check must say so (installed, stale), with the conflict to
	// resolve before migrating — not "not installed".
	if st := gitGateStatus(root, g); !st.Installed || !st.Stale || !strings.Contains(st.Remediation, "commit-msg") || !strings.Contains(st.Remediation, "still gates") {
		t.Fatalf("a gated earlier location with other hooks is installed and stale: %+v", st)
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

// Uninstall unsets core.hooksPath and leaves the user-level dir — scripts, a hook the user keeps there, and the id —
// so another repository that may run from it keeps its gate, and a reinstall here reuses it.
func TestUninstallLeavesTheUserLevelDirAndReinstallReusesIt(t *testing.T) {
	isolateHooksHome(t)
	root, g := tempRepo(t)
	plan, _ := PlanHookInstall(root, g)
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plan.Target, "commit-msg"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if changed, err := UninstallHookInstall(root, g); err != nil || !changed {
		t.Fatalf("uninstall: %v %v", changed, err)
	}
	if hooksPath(t, root, g) != "" {
		t.Fatal("uninstall must unset core.hooksPath")
	}
	if !hooksCurrent(plan.Target) {
		t.Fatal("uninstall must leave the user-level scripts in place")
	}
	if out, _ := g(root, "config", "--local", "--get", HooksIDKey); strings.TrimSpace(string(out)) != filepath.Base(plan.Target) {
		t.Fatalf("uninstall must keep %s, got %q", HooksIDKey, out)
	}
	again, _ := PlanHookInstall(root, g)
	if err := ApplyHookInstall(root, again, false, g); err != nil {
		t.Fatal(err)
	}
	if again.Target != plan.Target {
		t.Fatal("reinstall must reuse the dir")
	}
	if _, err := os.Stat(filepath.Join(plan.Target, "commit-msg")); err != nil {
		t.Fatal("the user's hook resumes with the reused dir")
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
	run(main, "config", "user.name", "t") // isolatedGit has no global identity (CI cannot infer one)
	run(main, "config", "user.email", "t@example.com")
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
	// The dir still names the old location as its owner: gated, but not done until a re-install records the move.
	after, err := PlanHookInstall(moved, g)
	if err != nil || !after.HooksCurrent || after.AlreadyDone {
		t.Fatalf("after a move: gate current, re-install owed: %+v %v", after, err)
	}
	if err := ApplyHookInstall(moved, after, false, g); err != nil {
		t.Fatal(err)
	}
	if done, _ := PlanHookInstall(moved, g); !done.AlreadyDone || done.Target != plan.Target {
		t.Fatalf("re-install keeps the dir and records the new owner: %+v", done)
	}
}

// A checkout copied with its .git (cp -r, rsync, a restored backup) carries metareview.hooksId and core.hooksPath. It
// must get its own hook dir, and nothing done in the copy may empty the dir its original still runs from.
func TestACopiedCheckoutGetsItsOwnHookDir(t *testing.T) {
	isolateHooksHome(t)
	a, g := tempRepo(t)
	plan, _ := PlanHookInstall(a, g)
	if err := ApplyHookInstall(a, plan, false, g); err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(t.TempDir(), "copy")
	if err := os.CopyFS(b, os.DirFS(a)); err != nil {
		t.Fatal(err)
	}
	if got := hookTarget(t, b, g); got == plan.Target {
		t.Fatal("a copied checkout must not resolve its original's hook dir")
	}
	if st := gitGateStatus(b, g); !st.Installed || !st.Stale {
		t.Fatalf("the copy runs its original's hooks: gated, but stale: %+v", st)
	}
	if _, err := UninstallHookInstall(b, g); err != nil {
		t.Fatal(err)
	}
	if !hooksCurrent(plan.Target) {
		t.Fatal("uninstalling in the copy must not empty the original's hook dir")
	}
	_, _ = g(b, "config", "--local", "core.hooksPath", plan.Target) // as copied again
	bp, _ := PlanHookInstall(b, g)
	if err := ApplyHookInstall(b, bp, false, g); err != nil {
		t.Fatal(err)
	}
	if bp.Target == plan.Target || !hooksCurrent(plan.Target) || !hooksCurrent(bp.Target) {
		t.Fatalf("the copy must move to its own dir and leave the original's: %q vs %q", bp.Target, plan.Target)
	}
	if again, _ := PlanHookInstall(a, g); !again.AlreadyDone {
		t.Fatal("the original stays installed")
	}
}

// The ownership rules, one case each: no owner file (an unrecorded install) or this repository's own is ours; the
// owner of a moved repository (its path gone) is ours; a repository that still exists elsewhere is not.
func TestOwnedBy(t *testing.T) {
	dir := t.TempDir()
	here := t.TempDir()
	if !ownedBy(dir, here) {
		t.Fatal("a dir with no owner file is ours")
	}
	write := func(owner string) { _ = os.WriteFile(filepath.Join(dir, hookOwnerFile), []byte(owner+"\n"), 0o644) }
	write(here)
	if !ownedBy(dir, here) || !ownerIs(dir, here) {
		t.Fatal("our own dir is ours")
	}
	write(filepath.Join(here, "gone"))
	if !ownedBy(dir, here) || ownerIs(dir, here) {
		t.Fatal("a moved repository's dir is ours, but not yet recorded as ours")
	}
	write(t.TempDir())
	if ownedBy(dir, here) {
		t.Fatal("a dir another existing repository owns is not ours")
	}
	releaseHookDir(dir, here, nil) // not a per-checkout dir: never emptied
	if _, err := os.Stat(filepath.Join(dir, hookOwnerFile)); err != nil {
		t.Fatal("releaseHookDir must leave a user-level dir alone")
	}
}

// Install fails, rather than claiming ownership it did not record, when git cannot name the common directory at
// apply time or the owner file cannot be written.
func TestApplyHookInstallOwnerFailures(t *testing.T) {
	isolateHooksHome(t)
	root, g := tempRepo(t)
	target := hookTarget(t, root, g)
	broken := func(r string, args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "git-common-dir") {
			return nil, errors.New("no common dir")
		}
		return g(r, args...)
	}
	if err := ApplyHookInstall(root, HookInstallPlan{Target: target}, true, broken); err == nil {
		t.Fatal("an unresolvable common dir must fail install")
	}
	if err := os.MkdirAll(filepath.Join(target, hookOwnerFile), 0o755); err != nil { // a dir where the file goes
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, HookInstallPlan{Target: target}, true, g); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("an unwritable owner file must fail install, got %v", err)
	}
}

// A NEW repository at a moved repository's old path (`mv repo repo.bak && git clone … repo`) derives from the same
// common-dir path. Whether or not the moved repository re-installed, the new one must not adopt the dir the moved one
// still runs from, and nothing done in it may empty that dir.
func TestANewRepositoryAtAMovedOnesPathGetsItsOwnHookDir(t *testing.T) {
	for _, reinstalledAfterMove := range []bool{true, false} {
		t.Run(fmt.Sprintf("reinstalled=%v", reinstalledAfterMove), func(t *testing.T) {
			isolateHooksHome(t)
			base, _ := filepath.EvalSymlinks(t.TempDir())
			g := isolatedGit(base)
			a := filepath.Join(base, "a")
			if out, err := g(base, "init", "-q", "-b", "main", a); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			plan, _ := PlanHookInstall(a, g)
			if err := ApplyHookInstall(a, plan, false, g); err != nil {
				t.Fatal(err)
			}
			moved := filepath.Join(base, "a.old")
			if err := os.Rename(a, moved); err != nil {
				t.Fatal(err)
			}
			if reinstalledAfterMove {
				mp, _ := PlanHookInstall(moved, g)
				if err := ApplyHookInstall(moved, mp, false, g); err != nil {
					t.Fatal(err)
				}
			}
			if out, err := g(base, "init", "-q", "-b", "main", a); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			np, _ := PlanHookInstall(a, g)
			if np.Target == plan.Target {
				t.Fatal("a new repository at the old path must not adopt the moved repository's hook dir")
			}
			if err := ApplyHookInstall(a, np, false, g); err != nil {
				t.Fatal(err)
			}
			if _, err := UninstallHookInstall(a, g); err != nil {
				t.Fatal(err)
			}
			if !hooksCurrent(plan.Target) {
				t.Fatal("nothing done in the new repository may empty the moved repository's hook dir")
			}
			if hp := hooksPath(t, moved, g); hp != plan.Target {
				t.Fatalf("the moved repository's core.hooksPath must be untouched: %q", hp)
			}
			// Its gate still runs (above). Re-installed, the dir is recorded as its own; if not, its owner file names
			// the path the new repository occupies, so it reads stale until a re-install gives it a dir of its own.
			if st := gitGateStatus(moved, g); !st.Installed || st.Stale == reinstalledAfterMove {
				t.Fatalf("moved repository status: %+v", st)
			}
		})
	}
}

// A repository deleted and re-cloned in place, never uninstalled, cannot be told from a moved one whose old path was
// re-used (the case above), so its old dir — which still holds the gate — is never adopted: each cycle gets a fresh
// dir and never fails. Safety over tidiness: a leaked dir costs a few KB; adopting a live one ungates a repository.
func TestARecloneInPlaceGetsAFreshHookDir(t *testing.T) {
	isolateHooksHome(t)
	base, _ := filepath.EvalSymlinks(t.TempDir())
	g := isolatedGit(base)
	a := filepath.Join(base, "a")
	var first string
	for i := 0; i < 3; i++ {
		_ = os.RemoveAll(a)
		if out, err := g(base, "init", "-q", "-b", "main", a); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		plan, err := PlanHookInstall(a, g)
		if err != nil {
			t.Fatal(err)
		}
		if err := ApplyHookInstall(a, plan, false, g); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = plan.Target
		} else if plan.Target == first {
			t.Fatalf("cycle %d: a dir that still holds the gate must not be adopted", i)
		}
	}
	if !hooksCurrent(first) {
		t.Fatal("the first cycle's dir must be left intact")
	}
}

// Uninstall then reinstall in place keeps a hook the user added to the dir: the reinstall reuses the dir.
func TestUninstallThenReinstallKeepsAUserHookRunning(t *testing.T) {
	isolateHooksHome(t)
	root, g := tempRepo(t)
	plan, _ := PlanHookInstall(root, g)
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plan.Target, "commit-msg"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := UninstallHookInstall(root, g); err != nil {
		t.Fatal(err)
	}
	again, _ := PlanHookInstall(root, g)
	if err := ApplyHookInstall(root, again, false, g); err != nil {
		t.Fatal(err)
	}
	if again.Target != plan.Target {
		t.Fatalf("reinstall must reuse the dir holding the user's hook: %q vs %q", again.Target, plan.Target)
	}
}

// The derived-id search is bounded: with every candidate taken, it is an error, never a reused dir.
func TestDerivedIDSearchIsBounded(t *testing.T) {
	isolateHooksHome(t)
	root, g := tempRepo(t)
	first := hookTarget(t, root, g)
	writeGateDir(t, first) // taken: it still holds the gate
	orig := maxDerivedIDs
	t.Cleanup(func() { maxDerivedIDs = orig })
	maxDerivedIDs = 1
	if _, err := repoHooksID(root, g, filepath.Dir(first)); err == nil {
		t.Fatal("with every candidate id taken, the search must fail")
	}
	maxDerivedIDs = orig
	if next := hookTarget(t, root, g); next == first {
		t.Fatal("an existing dir must never be adopted by a repository with no recorded id")
	}
}

// released: a dir is free only once metareview's scripts are out of it — whatever its owner file says.
func TestReleased(t *testing.T) {
	dir := t.TempDir()
	if !released(dir) {
		t.Fatal("an empty dir is released")
	}
	_ = os.WriteFile(filepath.Join(dir, "commit-msg"), []byte("#!/bin/sh\n"), 0o755)
	if !released(dir) {
		t.Fatal("a dir holding only a user's hook is released")
	}
	_ = os.WriteFile(filepath.Join(dir, "post-commit"), []byte("#!/bin/sh\n"), 0o755)
	if released(dir) {
		t.Fatal("a dir holding any of metareview's scripts is live")
	}
}

// Two live repositories can carry the same hooksId — a copy whose original then moved, or a backup restored over a
// moved repository's old path — and nothing inside either tells them apart. So nothing done in one may delete the
// gate from a user-level dir: uninstall leaves the dir (and the id) alone.
func TestUninstallNeverEmptiesAUserLevelDirAnotherRepositoryRunsFrom(t *testing.T) {
	for _, scenario := range []string{"copy-then-move-original", "restore-backup-over-moved-path"} {
		t.Run(scenario, func(t *testing.T) {
			isolateHooksHome(t)
			base, _ := filepath.EvalSymlinks(t.TempDir())
			g := isolatedGit(base)
			a := filepath.Join(base, "a")
			if out, err := g(base, "init", "-q", "-b", "main", a); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			plan, _ := PlanHookInstall(a, g)
			if err := ApplyHookInstall(a, plan, false, g); err != nil {
				t.Fatal(err)
			}
			var victim, actor string
			switch scenario {
			case "copy-then-move-original": // cp -r a b; mv a c; uninstall in b
				actor, victim = filepath.Join(base, "b"), filepath.Join(base, "c")
				if err := os.CopyFS(actor, os.DirFS(a)); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(a, victim); err != nil {
					t.Fatal(err)
				}
			default: // backup a; mv a b; restore a; uninstall in the restored a
				backup := filepath.Join(base, "backup")
				if err := os.CopyFS(backup, os.DirFS(a)); err != nil {
					t.Fatal(err)
				}
				victim, actor = filepath.Join(base, "b"), a
				if err := os.Rename(a, victim); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(backup, a); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := UninstallHookInstall(actor, g); err != nil {
				t.Fatal(err)
			}
			if !hooksCurrent(plan.Target) {
				t.Fatal("uninstalling in one repository must not delete the gate another runs from")
			}
			if hp := hooksPath(t, victim, g); hp != plan.Target {
				t.Fatalf("the victim's core.hooksPath must be untouched: %q", hp)
			}
		})
	}
}

// Before #173 core.hooksPath was an ABSOLUTE path into the installing checkout, so a copy of such an install (cp -r, a
// restored backup) still points at the ORIGINAL's .metareview/git-hooks. Installing or uninstalling in the copy must
// never delete that dir: only a per-checkout dir of this same repository is released.
func TestACopyNeverReleasesItsOriginalsPerCheckoutDir(t *testing.T) {
	for _, op := range []string{"install", "uninstall"} {
		t.Run(op, func(t *testing.T) {
			isolateHooksHome(t)
			base, _ := filepath.EvalSymlinks(t.TempDir())
			g := isolatedGit(base)
			orig := filepath.Join(base, "orig")
			if out, err := g(base, "init", "-q", "-b", "main", orig); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			old := filepath.Join(orig, ".metareview", "git-hooks") // a pre-#173 install
			writeGateDir(t, old)
			_, _ = g(orig, "config", "--local", "core.hooksPath", old)
			cp := filepath.Join(base, "copy")
			if err := os.CopyFS(cp, os.DirFS(orig)); err != nil {
				t.Fatal(err)
			}
			if op == "install" {
				plan, err := PlanHookInstall(cp, g)
				if err != nil {
					t.Fatal(err)
				}
				if err := ApplyHookInstall(cp, plan, false, g); err != nil {
					t.Fatal(err)
				}
			} else if _, err := UninstallHookInstall(cp, g); err != nil {
				t.Fatal(err)
			}
			if !hooksMaterialized(old) {
				t.Fatalf("%s in the copy must not delete the original's per-checkout hooks", op)
			}
		})
	}
}
