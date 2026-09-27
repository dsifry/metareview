package machine

import (
	"os"
	"path/filepath"
	"testing"
)

// #173: under the common-dir store, sidecars live beside the run's audit: <common>/metareview/runs/<id>/<name>.
func TestFSSidecarCommonDirLayout(t *testing.T) {
	common := t.TempDir()
	const id = "mrv-sidecar-common1"
	if err := os.MkdirAll(filepath.Join(common, "metareview", "runs", id), 0o700); err != nil {
		t.Fatal(err)
	}
	f := FSSidecar{Root: common, CommonDir: true}
	if err := f.Write(id, SidecarWorkflow, []byte("wf")); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(common, "metareview", "runs", id, SidecarWorkflow)); err != nil || string(b) != "wf" {
		t.Fatalf("sidecar at the common-dir layout: %q %v", b, err)
	}
}
