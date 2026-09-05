package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// failGitOn wraps a GitRunner to return err for the first git invocation whose args contain all of
// match; every other call delegates to base.
func failGitOn(base GitRunner, err error, match ...string) GitRunner {
	return func(root string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		all := true
		for _, m := range match {
			if !strings.Contains(joined, m) {
				all = false
				break
			}
		}
		if all {
			return nil, err
		}
		return base(root, args...)
	}
}

// --- materializeHooks / hooksMaterialized / hooksCurrent ---

func TestMaterializeHooksErrors(t *testing.T) {
	// embedded-asset read failure (via the seam).
	dir := t.TempDir()
	origRead := readHookAsset
	t.Cleanup(func() { readHookAsset = origRead })
	readHookAsset = func(string) ([]byte, error) { return nil, errors.New("embed boom") }
	if err := materializeHooks(filepath.Join(dir, "a")); err == nil || !strings.Contains(err.Error(), "embed boom") {
		t.Fatalf("embed read error: %v", err)
	}
	readHookAsset = origRead

	// WriteFile failure: a hook name pre-exists as a directory.
	dir2 := t.TempDir()
	for name := range gitHookScripts {
		if err := os.MkdirAll(filepath.Join(dir2, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := materializeHooks(dir2); err == nil {
		t.Fatal("expected a WriteFile error when a hook name is a directory")
	}

	// Chmod failure (via the seam).
	dir3 := t.TempDir()
	origChmod := osChmod
	t.Cleanup(func() { osChmod = origChmod })
	osChmod = func(string, os.FileMode) error { return errors.New("chmod boom") }
	if err := materializeHooks(dir3); err == nil || !strings.Contains(err.Error(), "chmod boom") {
		t.Fatalf("chmod error: %v", err)
	}
}

func TestHooksMaterializedAndCurrent(t *testing.T) {
	// An empty dir is not materialized.
	if hooksMaterialized(t.TempDir()) {
		t.Error("empty dir should not be materialized")
	}
	// After a real materialize, it is materialized and current.
	dir := t.TempDir()
	if err := materializeHooks(dir); err != nil {
		t.Fatal(err)
	}
	if !hooksMaterialized(dir) || !hooksCurrent(dir) {
		t.Fatal("materialized dir should be materialized and current")
	}
	// hooksCurrent is false when the embed read fails even though the file is present+executable.
	orig := readHookAsset
	t.Cleanup(func() { readHookAsset = orig })
	readHookAsset = func(string) ([]byte, error) { return nil, errors.New("boom") }
	if hooksCurrent(dir) {
		t.Error("hooksCurrent should be false when the embed cannot be read")
	}
}

// --- hookTargetDir error (filepathAbs seam) propagates through Plan/Uninstall ---

func TestHookTargetDirErrorPropagates(t *testing.T) {
	orig := filepathAbs
	t.Cleanup(func() { filepathAbs = orig })
	filepathAbs = func(string) (string, error) { return "", errors.New("abs boom") }
	root := t.TempDir()
	if _, err := PlanHookInstall(root, func(string, ...string) ([]byte, error) { return nil, nil }); err == nil {
		t.Error("PlanHookInstall should surface the hookTargetDir error")
	}
	if _, err := UninstallPreview(root, func(string, ...string) ([]byte, error) { return nil, nil }); err == nil {
		t.Error("UninstallPreview should surface the hookTargetDir error")
	}
	if _, err := UninstallHookInstall(root, func(string, ...string) ([]byte, error) { return nil, nil }); err == nil {
		t.Error("UninstallHookInstall should surface the hookTargetDir error")
	}
}

// --- PlanHookInstall active-hooks inspection error ---

func TestPlanHookInstallActiveHooksError(t *testing.T) {
	root, g := tempRepo(t)
	// git succeeds for rev-parse --git-dir and config reads, but fails resolving the hooks path.
	g2 := failGitOn(g, errors.New("git-path boom"), "--git-path")
	if _, err := PlanHookInstall(root, g2); err == nil || !strings.Contains(err.Error(), "active hooks") {
		t.Fatalf("expected an active-hooks inspection error, got %v", err)
	}
}

// --- ApplyHookInstall branches ---

func TestApplyHookInstallAlreadyDoneNoForce(t *testing.T) {
	root, g := tempRepo(t)
	if err := ApplyHookInstall(root, HookInstallPlan{AlreadyDone: true}, false, g); err != nil {
		t.Fatalf("AlreadyDone + !force should be a no-op, got %v", err)
	}
}

func TestApplyHookInstallRevalidationError(t *testing.T) {
	root, g := tempRepo(t)
	// A fresh PlanHookInstall (rev-parse) fails during revalidation.
	g2 := failGitOn(g, errors.New("revalidate boom"), "rev-parse", "--git-dir")
	if err := ApplyHookInstall(root, HookInstallPlan{}, false, g2); err == nil || !strings.Contains(err.Error(), "revalidating") {
		t.Fatalf("expected a revalidation error, got %v", err)
	}
}

func TestApplyHookInstallFreshAlreadyDone(t *testing.T) {
	root, g := tempRepo(t)
	target, _ := filepath.Abs(filepath.Join(root, ".metareview", "git-hooks"))
	// Install for real so the repo's core.hooksPath already points at target (fresh AlreadyDone),
	// then Apply a stale non-AlreadyDone plan without force.
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	_ = target
	if err := ApplyHookInstall(root, HookInstallPlan{Target: target}, false, g); err != nil {
		t.Fatalf("a stale plan should revalidate to AlreadyDone (nil), got %v", err)
	}
}

func TestApplyHookInstallConfigSetError(t *testing.T) {
	root, g := tempRepo(t)
	target, _ := filepath.Abs(filepath.Join(root, ".metareview", "git-hooks"))
	// force=true skips revalidation; fail the config write that sets core.hooksPath.
	g2 := failGitOn(g, errors.New("set boom"), "core.hooksPath", target)
	if err := ApplyHookInstall(root, HookInstallPlan{Target: target}, true, g2); err == nil || !strings.Contains(err.Error(), "setting core.hooksPath") {
		t.Fatalf("expected a config-set error, got %v", err)
	}
}

func TestApplyHookInstallNotMaterializedAfterWrite(t *testing.T) {
	root, g := tempRepo(t)
	target, _ := filepath.Abs(filepath.Join(root, ".metareview", "git-hooks"))
	orig := applyMaterialize
	t.Cleanup(func() { applyMaterialize = orig })
	// materialize "succeeds" but writes nothing, so the post-write verify fails.
	applyMaterialize = func(string) error { return nil }
	if err := ApplyHookInstall(root, HookInstallPlan{Target: target}, true, g); err == nil || !strings.Contains(err.Error(), "NOT active") {
		t.Fatalf("expected a not-materialized error, got %v", err)
	}
}

// --- UninstallHookInstall branches ---

func TestUninstallHookInstallUnsetAndErrors(t *testing.T) {
	// current == "" -> nothing to do.
	root, g := tempRepo(t)
	if changed, err := UninstallHookInstall(root, g); err != nil || changed {
		t.Fatalf("no core.hooksPath -> (false,nil), got (%v,%v)", changed, err)
	}
	// Install, then fail the --unset.
	if _, err := PlanHookInstall(root, g); err != nil {
		t.Fatal(err)
	}
	plan, _ := PlanHookInstall(root, g)
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	g2 := failGitOn(g, errors.New("unset boom"), "--unset")
	if _, err := UninstallHookInstall(root, g2); err == nil || !strings.Contains(err.Error(), "unset boom") {
		t.Fatalf("expected an unset error, got %v", err)
	}
}

// --- activeGitHooks ReadDir branches ---

func TestActiveGitHooksReadDirBranches(t *testing.T) {
	root, g := tempRepo(t)
	// A --git-path pointing at a nonexistent dir -> nil, nil (nothing to bypass).
	missing := filepath.Join(t.TempDir(), "nope")
	gMissing := gitReturning(g, "--git-path", missing)
	if got, err := activeGitHooks(root, gMissing); err != nil || got != nil {
		t.Fatalf("missing hooks dir -> (nil,nil), got (%v,%v)", got, err)
	}
	// A --git-path pointing at a FILE -> ReadDir returns a non-NotExist error.
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gFile := gitReturning(g, "--git-path", f)
	if _, err := activeGitHooks(root, gFile); err == nil {
		t.Fatal("a file at the hooks path should surface a ReadDir error")
	}
	// git error resolving the path.
	gErr := failGitOn(g, errors.New("path boom"), "--git-path")
	if _, err := activeGitHooks(root, gErr); err == nil || !strings.Contains(err.Error(), "path boom") {
		t.Fatalf("expected a git error, got %v", err)
	}
}

// gitReturning wraps base to return a fixed output (with a trailing newline) for calls matching all
// of `match`, delegating everything else.
func gitReturning(base GitRunner, matchAndOutput ...string) GitRunner {
	match := matchAndOutput[:len(matchAndOutput)-1]
	out := matchAndOutput[len(matchAndOutput)-1]
	return func(root string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		all := true
		for _, m := range match {
			if !strings.Contains(joined, m) {
				all = false
				break
			}
		}
		if all {
			return []byte(out + "\n"), nil
		}
		return base(root, args...)
	}
}

// --- setup.go probes ---

func TestCheckDefaultsAndBootstrapConfirm(t *testing.T) {
	// Check with an empty Options exercises the nil-lookup and empty-home fallbacks.
	_ = Check(t.TempDir(), Options{})
	// BootstrapPrereqs with confirmation (not dry-run) reaches the confirmed-append branch.
	plan, err := BootstrapPrereqs(t.TempDir(), BootstrapOptions{Confirm: true})
	if err != nil {
		t.Fatalf("BootstrapPrereqs confirm: %v", err)
	}
	if len(plan.Actions) == 0 || !strings.Contains(plan.Actions[len(plan.Actions)-1], "Confirmation supplied") {
		t.Fatalf("expected the confirmed-append action: %v", plan.Actions)
	}
}

func TestToolProbes(t *testing.T) {
	root := t.TempDir()
	// superpowersStatus present when a candidate dir exists.
	if err := os.MkdirAll(filepath.Join(root, ".claude", "plugins", "superpowers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if s := superpowersStatus(root, ""); !s.Present {
		t.Error("superpowersStatus should be present with the plugin dir")
	}
	// beadsStatus: repo has beads; or the bd binary is on PATH (fake lookup).
	if s := beadsStatus(root, func(string) (string, error) { return "", errors.New("x") }, true); !s.Present {
		t.Error("beadsStatus repoHasBeads")
	}
	if s := beadsStatus(root, func(string) (string, error) { return "/bin/bd", nil }, false); !s.Present {
		t.Error("beadsStatus bd on PATH")
	}
	// metaswarmStatus: repo has metaswarm; or a candidate dir exists.
	if s := metaswarmStatus(root, true); !s.Present {
		t.Error("metaswarmStatus repoHasMetaswarm")
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude", "plugins", "metaswarm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if s := metaswarmStatus(root, false); !s.Present {
		t.Error("metaswarmStatus candidate dir")
	}
}

func TestGitStatusVersionParsing(t *testing.T) {
	lookup := func(string) (string, error) { return "/usr/bin/git", nil }
	// Unparseable version output.
	if s := gitStatus(lookup, func() (string, error) { return "garbage with no ver token", nil }); s.VersionOK || !strings.Contains(s.Action, "parse") {
		t.Fatalf("unparseable version: %+v", s)
	}
	// parseGitVersion direct: no version field.
	if parseGitVersion("git nope 2.39") != "" {
		t.Error("parseGitVersion should be empty without a 'version' token")
	}
	// atLeastVersion: have shorter than min.
	if atLeastVersion("2", "2.31") {
		t.Error("atLeastVersion(2, 2.31) should be false")
	}
}

func TestPluginRootEnvAndAbsent(t *testing.T) {
	// CLAUDE_PLUGIN_ROOT wins when set.
	t.Setenv("CLAUDE_PLUGIN_ROOT", "/explicit/plugin/root")
	if got := pluginRoot("/home"); got != "/explicit/plugin/root" {
		t.Fatalf("env override: %q", got)
	}
	// Unset + empty home -> "".
	t.Setenv("CLAUDE_PLUGIN_ROOT", "")
	if got := pluginRoot(""); got != "" {
		t.Fatalf("empty home should yield empty plugin root, got %q", got)
	}
	// A home with no metareview manifest -> "".
	if got := pluginRoot(t.TempDir()); got != "" {
		t.Fatalf("home without a manifest should yield empty, got %q", got)
	}
	// A home holding metareview's plugin manifest (a Stop hook running its pre-finish gate) resolves
	// to the plugin dir.
	home := t.TempDir()
	pluginDir := filepath.Join(home, ".claude", "plugins", "metareview")
	script := filepath.Join(pluginDir, "hooks", "pre-finish.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/bin/bash\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"bash ` + script + `"}]}]}}`
	if err := os.WriteFile(filepath.Join(pluginDir, "hooks", "hooks.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := pluginRoot(home); got != pluginDir {
		t.Fatalf("pluginRoot should resolve metareview's manifest to %q, got %q", pluginDir, got)
	}
}

func TestIsOursScopedBashScript(t *testing.T) {
	root := "/repo/plugins/metareview"
	script := filepath.Join(root, "hooks", "pre-finish.sh")
	// A bash-form command whose script sits under a metareview root is ours.
	if !isOursScoped("bash "+script, false, root) {
		t.Errorf("a bash-launched metareview hook should be ours")
	}
	// A leading flag before the script is skipped (the flag-skip continue), then the script matches.
	if !isOursScoped("bash -x "+script, false, root) {
		t.Errorf("a flagged bash command should still resolve to the script")
	}
	// An empty command is not ours.
	if isOursScoped("", false, root) {
		t.Error("an empty command is not ours")
	}
}
