package main

import (
	"os"
	"testing"
)

// TestMain points XDG_DATA_HOME at a throwaway directory for the whole package: `setup --install-hooks` materializes
// the hook scripts under the user's data home (#173), and no test may write into the real one.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "metareview-cli-xdg-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("XDG_DATA_HOME", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
