package setup

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain points XDG_DATA_HOME at a throwaway directory for the whole package, so no test materializes hook
// scripts into the real user's ~/.local/share (#173: the hooks are user-level).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "metareview-setup-xdg-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("XDG_DATA_HOME", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// hookTarget is where this binary materializes the hook scripts, the value core.hooksPath should hold.
func hookTarget(t *testing.T) string {
	t.Helper()
	dir, err := hookTargetDir("")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(dir)
}

// isolateHooksHome gives one test its own XDG_DATA_HOME, for a test that tampers with the materialized scripts or
// needs them absent: the package-wide directory is shared by every test.
func isolateHooksHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	return dir
}
