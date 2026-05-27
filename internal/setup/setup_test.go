package setup

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCheckPreservesRepoFieldsAndAddsPrerequisiteStatuses(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "docs", "SERVICE_INVENTORY.md"), "# Services\n")

	report := Check(root, Options{
		ExecutablePath: "/usr/local/bin/metareview",
		HomeDir:        t.TempDir(),
		LookupPath: func(name string) (string, error) {
			switch name {
			case "git":
				return "/usr/bin/git", nil
			case "go":
				return "/usr/local/go/bin/go", nil
			default:
				return "", errors.New("missing")
			}
		},
	})

	if report.Mode != "standalone-minimal" {
		t.Fatalf("expected backward-compatible mode, got %s", report.Mode)
	}
	if !report.Capabilities.Git || !report.Capabilities.ServiceInventory || report.Files.ServiceInventory == nil {
		t.Fatalf("expected backward-compatible repo fields, got %+v", report)
	}
	if !report.Prerequisites.Git.Present || report.Prerequisites.Git.Path != "/usr/bin/git" {
		t.Fatalf("git status missing: %+v", report.Prerequisites.Git)
	}
	if !report.Prerequisites.Go.Present || report.Prerequisites.Go.Path != "/usr/local/go/bin/go" {
		t.Fatalf("go status missing: %+v", report.Prerequisites.Go)
	}
	if report.Prerequisites.Superpowers.Present {
		t.Fatalf("superpowers should be reported missing in fixture: %+v", report.Prerequisites.Superpowers)
	}
	if report.Install.Path != "/usr/local/bin/metareview" {
		t.Fatalf("install path mismatch: %+v", report.Install)
	}
	if !report.Standalone.AdvisoryOnly || report.Standalone.FullMetaswarmReady {
		t.Fatalf("standalone readiness mismatch: %+v", report.Standalone)
	}
	if !reflect.DeepEqual(report.Standalone.MissingForFullMetaswarm, []string{"beads", "metaswarm", "superpowers"}) {
		t.Fatalf("missing full prerequisites mismatch: %+v", report.Standalone.MissingForFullMetaswarm)
	}
	if report.Prerequisites.Superpowers.Action == "" || report.Prerequisites.Beads.Action == "" {
		t.Fatalf("missing prerequisites should include actions: %+v", report.Prerequisites)
	}
}

func TestBootstrapDryRunReturnsStandaloneActionsWithoutMutatingRepo(t *testing.T) {
	root := t.TempDir()
	before := snapshotNames(t, root)

	plan, err := BootstrapPrereqs(root, BootstrapOptions{DryRun: true})
	if err != nil {
		t.Fatalf("BootstrapPrereqs dry-run returned error: %v", err)
	}

	if !plan.DryRun {
		t.Fatalf("expected dry-run plan: %+v", plan)
	}
	joined := strings.Join(plan.Actions, "\n")
	for _, expected := range []string{
		"Install Superpowers",
		"Install Beads",
		"Install metaswarm",
		"No changes made",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("bootstrap plan missing %q:\n%s", expected, joined)
		}
	}
	after := snapshotNames(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("dry-run mutated repo. before=%+v after=%+v", before, after)
	}
}

func TestBootstrapRequiresExplicitConfirmation(t *testing.T) {
	_, err := BootstrapPrereqs(t.TempDir(), BootstrapOptions{})
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("expected confirmation error, got %v", err)
	}
}

func snapshotNames(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func mustWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
