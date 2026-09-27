package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stopGate(t *testing.T, root string, g GitRunner) string {
	t.Helper()
	out, _ := g(root, "config", "--local", "--get", StopGateKey)
	return strings.TrimSpace(string(out))
}

// #194: the plugin registers the Stop hook in every session, and the hook gates only a repository that opted in.
// `setup --install-hooks` is the opt-in: it records metareview.stopGate=true locally; uninstall removes it.
func TestHookInstallRecordsTheStopGateOptIn(t *testing.T) {
	root, g := tempRepo(t)
	plan, err := PlanHookInstall(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	if got := stopGate(t, root, g); got != "true" {
		t.Fatalf("install must record the opt-in, got %q", got)
	}
	if changed, err := UninstallHookInstall(root, g); err != nil || !changed {
		t.Fatalf("uninstall: %v %v", changed, err)
	}
	if got := stopGate(t, root, g); got != "" {
		t.Fatalf("uninstall must remove the opt-in, got %q", got)
	}
}

// A gate installed before #194 has current hooks but no opt-in: it is not AlreadyDone, so re-running
// `setup --install-hooks` records the opt-in and the Stop gate keeps working after the upgrade.
func TestPreOptInInstallIsNotAlreadyDone(t *testing.T) {
	root, g := tempRepo(t)
	plan, _ := PlanHookInstall(root, g)
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	if _, err := g(root, "config", "--local", "--unset", StopGateKey); err != nil {
		t.Fatal(err)
	}
	plan2, err := PlanHookInstall(root, g)
	if err != nil || plan2.AlreadyDone {
		t.Fatalf("an install without the opt-in must not be AlreadyDone: %+v %v", plan2, err)
	}
	if err := ApplyHookInstall(root, plan2, false, g); err != nil {
		t.Fatal(err)
	}
	if got := stopGate(t, root, g); got != "true" {
		t.Fatalf("re-install must record the opt-in, got %q", got)
	}
}

// A failure to record the opt-in fails the install: reporting the gate installed while the Stop hook stays inert
// would be the silent non-enforcement this layer exists to prevent.
func TestHookInstallFailsWhenTheOptInCannotBeRecorded(t *testing.T) {
	root, g := tempRepo(t)
	failing := func(r string, args ...string) ([]byte, error) {
		if len(args) == 4 && args[0] == "config" && args[2] == StopGateKey {
			return nil, errors.New("boom")
		}
		return g(r, args...)
	}
	plan, _ := PlanHookInstall(root, failing)
	if err := ApplyHookInstall(root, plan, false, failing); err == nil || !strings.Contains(err.Error(), "opt-in") {
		t.Fatalf("err = %v, want the opt-in failure", err)
	}
}

// setup --check (#194): a registered Stop hook gates only a repository that opted in, so the report says so and
// names the opt-in; an opted-in repository is reported as before.
func TestEnforcementReportsTheStopGateOptIn(t *testing.T) {
	registered := EnforcementStatus{Active: true, Source: "/p/hooks/hooks.json", ScriptPresent: true}
	got := withStopGateOptIn(registered, false)
	if got.OptedIn || !strings.Contains(got.Remediation, "has not opted in") || !strings.Contains(got.Remediation, "setup --install-hooks") {
		t.Fatalf("not opted in: %+v", got)
	}
	if got := withStopGateOptIn(registered, true); !got.OptedIn || !got.Active || got.Remediation != "" {
		t.Fatalf("opted in: %+v", got)
	}
	// An unregistered hook keeps its own remediation.
	unregistered := EnforcementStatus{Remediation: "No Stop hook is registered"}
	if got := withStopGateOptIn(unregistered, false); got.Remediation != "No Stop hook is registered" {
		t.Fatalf("unregistered: %+v", got)
	}
}

func TestStopGateOptedIn(t *testing.T) {
	root, g := tempRepo(t)
	if stopGateOptedIn(root, g) {
		t.Fatal("a fresh repository has not opted in")
	}
	if _, err := g(root, "config", "--local", StopGateKey, "true"); err != nil {
		t.Fatal(err)
	}
	if !stopGateOptedIn(root, g) {
		t.Fatal("metareview.stopGate=true is the opt-in")
	}
	if stopGateOptedIn(t.TempDir(), nil) {
		t.Fatal("a directory outside any repository has not opted in")
	}
}

// The standalone opt-in (#194 review): a repository whose own hook manager owns core.hooksPath (husky, lefthook,
// beads) cannot take `setup --install-hooks` without --force, but can still opt into the Stop gate.
func TestEnableAndDisableStopGate(t *testing.T) {
	root, g := tempRepo(t)
	if _, err := g(root, "config", "--local", "core.hooksPath", ".husky"); err != nil {
		t.Fatal(err)
	}
	if err := EnableStopGate(root, g); err != nil {
		t.Fatal(err)
	}
	if got := stopGate(t, root, g); got != "true" {
		t.Fatalf("enable must record the opt-in, got %q", got)
	}
	if got := hooksPath(t, root, g); got != ".husky" {
		t.Fatalf("enable must not touch core.hooksPath, got %q", got)
	}
	if changed, err := DisableStopGate(root, g); err != nil || !changed {
		t.Fatalf("disable: %v %v", changed, err)
	}
	if changed, err := DisableStopGate(root, g); err != nil || changed {
		t.Fatalf("disabling twice changes nothing: %v %v", changed, err)
	}
	if got := stopGate(t, root, g); got != "" {
		t.Fatalf("disable must remove the opt-in, got %q", got)
	}
	nonRepo := t.TempDir()
	if err := EnableStopGate(nonRepo, isolatedGit(nonRepo)); err == nil {
		t.Fatal("enable outside a repository must fail")
	}
	if _, err := DisableStopGate(nonRepo, isolatedGit(nonRepo)); err == nil {
		t.Fatal("disable outside a repository must fail")
	}
	if err := EnableStopGate(root, nil); err != nil { // nil runner uses real git
		t.Fatal(err)
	}
}

// A repository that has not opted in has no active Stop gate, whatever is registered (#194 AC3).
func TestUnoptedRegisteredHookIsInactive(t *testing.T) {
	got := withStopGateOptIn(EnforcementStatus{Active: true, Source: "/p/hooks/hooks.json", ScriptPresent: true}, false)
	if got.Active || got.Source == "" {
		t.Fatalf("a registered hook without the opt-in must report inactive (keeping its source): %+v", got)
	}
}

// setup --check wires the opt-in into its report.
func TestCheckReportsTheOptIn(t *testing.T) {
	root, g := tempRepo(t)
	if Check(root, Options{}).Enforcement.OptedIn {
		t.Fatal("a fresh repository has not opted in")
	}
	if err := EnableStopGate(root, g); err != nil {
		t.Fatal(err)
	}
	if !Check(root, Options{}).Enforcement.OptedIn {
		t.Fatal("setup --check must report the recorded opt-in")
	}
}

// The hook and the installer must agree on the key: a drifted constant makes install report success while the
// hook stays inert everywhere.
func TestHookReadsTheStopGateKey(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "hooks", "pre-finish.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "--get "+StopGateKey+" ") {
		t.Fatalf("hooks/pre-finish.sh does not read %s", StopGateKey)
	}
}

func TestStopGateTogglesEdgeCases(t *testing.T) {
	root, g := tempRepo(t)
	failing := func(r string, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "config" && args[len(args)-1] == "true" {
			return nil, errors.New("boom")
		}
		return g(r, args...)
	}
	if err := EnableStopGate(root, failing); err == nil || !strings.Contains(err.Error(), "opt-in") {
		t.Fatalf("a failed write must surface: %v", err)
	}
	if _, err := DisableStopGate(root, nil); err != nil { // nil runner uses real git
		t.Fatal(err)
	}
}

// Uninstall must not report success while the opt-in survives: only git's missing-key exit (5) is benign when
// unsetting it; any other failure is returned (CodeRabbit on #195).
func TestUninstallSurfacesAFailedOptInRemoval(t *testing.T) {
	root, g := tempRepo(t)
	plan, _ := PlanHookInstall(root, g)
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	failing := func(r string, args ...string) ([]byte, error) {
		if len(args) == 4 && args[2] == "--unset-all" && args[3] == StopGateKey {
			return nil, errors.New("could not lock config file")
		}
		return g(r, args...)
	}
	if changed, err := UninstallHookInstall(root, failing); err == nil || changed || !strings.Contains(err.Error(), "opt-in") {
		t.Fatalf("a failed opt-in removal must surface and change nothing: %v %v", changed, err)
	}
	// Nothing was taken apart, so the same command can finish the job once the failure clears.
	if got := hooksPath(t, root, g); got != plan.Target {
		t.Fatalf("core.hooksPath must survive a failed opt-in removal, got %q", got)
	}
	if changed, err := UninstallHookInstall(root, g); err != nil || !changed || stopGate(t, root, g) != "" {
		t.Fatalf("the retry must complete the uninstall: %v %v", changed, err)
	}
	plan, _ = PlanHookInstall(root, g)
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	// A pre-#194 install has no opt-in to remove: git's missing-key exit is not a failure.
	if _, err := g(root, "config", "--local", "core.hooksPath", plan.Target); err != nil {
		t.Fatal(err)
	}
	if _, err := g(root, "config", "--local", "--unset", StopGateKey); err != nil {
		t.Fatal(err)
	}
	if changed, err := UninstallHookInstall(root, g); err != nil || !changed {
		t.Fatalf("uninstalling without an opt-in: %v %v", changed, err)
	}
}

// If core.hooksPath cannot be unset after the opt-in is gone, the error surfaces; the push gate stays installed
// without the opt-in, which the Stop hook announces rather than hides, and a retry finishes the job.
func TestUninstallSurfacesAFailedHooksPathRemoval(t *testing.T) {
	root, g := tempRepo(t)
	plan, _ := PlanHookInstall(root, g)
	if err := ApplyHookInstall(root, plan, false, g); err != nil {
		t.Fatal(err)
	}
	failing := func(r string, args ...string) ([]byte, error) {
		if len(args) == 4 && args[2] == "--unset" && args[3] == "core.hooksPath" {
			return nil, errors.New("could not lock config file")
		}
		return g(r, args...)
	}
	if changed, err := UninstallHookInstall(root, failing); err == nil || changed {
		t.Fatalf("a failed core.hooksPath removal must surface: %v %v", changed, err)
	}
	if changed, err := UninstallHookInstall(root, g); err != nil || !changed {
		t.Fatalf("the retry must complete the uninstall: %v %v", changed, err)
	}
}

// Duplicate opt-in lines (a hand-edited config): plain --unset exits 5 and changes nothing — the same code as a
// missing key — so uninstall and --disable-stop-gate must remove EVERY value (CodeRabbit on #195).
func TestOptInRemovalClearsDuplicateValues(t *testing.T) {
	for _, remove := range []func(string, GitRunner) error{
		func(root string, g GitRunner) error { _, err := UninstallHookInstall(root, g); return err },
		func(root string, g GitRunner) error { _, err := DisableStopGate(root, g); return err },
	} {
		root, g := tempRepo(t)
		plan, _ := PlanHookInstall(root, g)
		if err := ApplyHookInstall(root, plan, false, g); err != nil {
			t.Fatal(err)
		}
		if _, err := g(root, "config", "--local", "--add", StopGateKey, "true"); err != nil {
			t.Fatal(err)
		}
		if err := remove(root, g); err != nil {
			t.Fatal(err)
		}
		if got := stopGate(t, root, g); got != "" {
			t.Fatalf("every opt-in value must be removed, got %q", got)
		}
	}
}
