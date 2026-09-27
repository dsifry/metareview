package setup

import (
	"errors"
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
	if got := withStopGateOptIn(registered, true); !got.OptedIn || got.Remediation != "" {
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
