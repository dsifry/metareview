package mutation

import (
	"os"
	"testing"
)

// Reads the actual gremlins reports written during the 2026-08-29 timeout spike, if they are
// still on this machine, and shows what our scoring says about each. Skips when they are absent,
// so it never fails on another machine — its value is the record it prints, not an assertion.
func TestAgainstTheRealSpikeReports(t *testing.T) {
	for _, f := range []string{"/tmp/g-c2.json", "/tmp/g-c5.json", "/tmp/g-c10.json", "/tmp/g-c20.json"} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Skipf("spike report %s not present on this machine", f)
		}
		r, err := ParseGremlins(data, "./internal/findings")
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		s := r.Score()
		t.Logf("%-16s killed=%3d survived=%2d uncovered=%d UNRESOLVED=%3d efficacy=%6.2f%% complete=%v findings=%d",
			f, s.Killed, s.Survived, s.Uncovered, s.Unresolved, s.Efficacy*100, s.Complete(), len(r.Findings()))
	}
}
