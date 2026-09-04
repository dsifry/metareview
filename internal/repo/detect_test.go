package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Detect's contract: capability flags reflect the markers on disk and the mode is chosen by the
// documented precedence (metaswarm > beads > git > advisory). This exercises exists()/readIfFile()
// and the Beads `||` across their true and false branches.
func TestDetectCapabilitiesAndMode(t *testing.T) {
	t.Run("bare directory is advisory with nothing enabled", func(t *testing.T) {
		got := Detect(t.TempDir())
		if got.Mode != "advisory" {
			t.Fatalf("mode = %q, want advisory", got.Mode)
		}
		if got.Capabilities != (Capabilities{}) {
			t.Fatalf("expected no capabilities, got %+v", got.Capabilities)
		}
	})

	t.Run("a .git marker gives git + standalone-minimal", func(t *testing.T) {
		root := t.TempDir()
		mkdirAll(t, filepath.Join(root, ".git"))
		got := Detect(root)
		if !got.Capabilities.Git || got.Mode != "standalone-minimal" {
			t.Fatalf("git repo mis-detected: %+v mode=%q", got.Capabilities, got.Mode)
		}
	})

	t.Run("a .beads directory alone enables Beads via the ||", func(t *testing.T) {
		root := t.TempDir()
		mkdirAll(t, filepath.Join(root, ".beads")) // dir present, no issues.jsonl
		got := Detect(root)
		if !got.Capabilities.Beads || got.Mode != "standalone-full" {
			t.Fatalf("beads dir should enable Beads/standalone-full: %+v mode=%q", got.Capabilities, got.Mode)
		}
	})

	t.Run("a CLAUDE.md that names metaswarm enables Metaswarm + metaswarm-extension", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "CLAUDE.md"), "This repo uses metaswarm workflows.\n")
		got := Detect(root)
		if !got.Capabilities.Metaswarm || got.Mode != "metaswarm-extension" {
			t.Fatalf("CLAUDE.md metaswarm marker not detected: %+v mode=%q", got.Capabilities, got.Mode)
		}
	})

	t.Run("a CLAUDE.md that disclaims metaswarm does NOT enable it", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "CLAUDE.md"), "This repo does not use metaswarm.\n")
		if Detect(root).Capabilities.Metaswarm {
			t.Fatal("a disclaimer must not enable Metaswarm")
		}
	})

	t.Run("service inventory and metareview state are surfaced", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "docs", "SERVICE_INVENTORY.md"), "# services\n")
		mkdirAll(t, filepath.Join(root, ".metareview"))
		got := Detect(root)
		if !got.Capabilities.ServiceInventory || got.Files.ServiceInventory == nil || *got.Files.ServiceInventory != "docs/SERVICE_INVENTORY.md" {
			t.Fatalf("service inventory not surfaced: %+v files=%+v", got.Capabilities, got.Files)
		}
		if !got.Capabilities.MetareviewState {
			t.Fatalf("a .metareview directory must set MetareviewState: %+v", got.Capabilities)
		}
	})
}

// Report.JSON round-trips the report as indented JSON a host hook can parse.
func TestReportJSON(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, ".git"))
	b, err := Detect(root).JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if !strings.Contains(string(b), `"mode": "standalone-minimal"`) || !strings.Contains(string(b), `"git": true`) {
		t.Fatalf("unexpected JSON:\n%s", b)
	}
}

// Root surfaces a filepath.Abs failure (via the absPath seam) rather than swallowing it.
func TestRootPropagatesAbsError(t *testing.T) {
	orig := absPath
	absPath = func(string) (string, error) { return "", os.ErrInvalid }
	defer func() { absPath = orig }()
	if _, err := Root("anything"); err == nil {
		t.Fatal("Root must return the filepath.Abs error")
	}
}
