package covergate

import (
	"errors"
	"strings"
	"testing"
)

const mod = "github.com/dsifry/metareview"

// errReader returns data once, then an error — to exercise the bufio.Scanner error paths.
type errReader struct {
	data string
	done bool
}

func (e *errReader) Read(p []byte) (int, error) {
	if e.done {
		return 0, errors.New("boom")
	}
	e.done = true
	n := copy(p, e.data)
	return n, nil
}

func TestPkgCovPctAndExact(t *testing.T) {
	c := PkgCov{Covered: 3, Total: 4}
	if got := c.Pct(); got != 75 {
		t.Errorf("Pct=%v want 75", got)
	}
	if c.Exact() {
		t.Error("3/4 should not be exact")
	}
	if !(PkgCov{Covered: 4, Total: 4}).Exact() {
		t.Error("4/4 should be exact")
	}
	if (PkgCov{}).Exact() {
		t.Error("0/0 must not be exact (zero-statement guard)")
	}
}

func TestParseProfile(t *testing.T) {
	in := "mode: atomic\n" +
		mod + "/internal/jsonl/jsonl.go:28.2,31.1 3 2\n" +
		mod + "/internal/jsonl/scan.go:10.1,12.2 2 0\n" +
		"\n" + // blank line ignored
		mod + "/internal/fsm/kind/kind.go:1.1,2.2 5 5\n"
	got, err := ParseProfile(strings.NewReader(in), mod)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got["internal/jsonl"] != (PkgCov{Covered: 3, Total: 5}) {
		t.Errorf("jsonl=%+v want {3 5}", got["internal/jsonl"])
	}
	if got["internal/fsm/kind"] != (PkgCov{Covered: 5, Total: 5}) {
		t.Errorf("kind=%+v want {5 5}", got["internal/fsm/kind"])
	}
}

func TestParseProfileErrors(t *testing.T) {
	cases := map[string]string{
		"wrong field count": mod + "/a/b.go:1.1,2.2 3\n",
		"missing colon":     "nocolon 3 2\n",
		"not under module":  "other.com/x/y.go:1.1,2.2 3 2\n",
		"bad stmts":         mod + "/a/b.go:1.1,2.2 x 2\n",
		"bad count":         mod + "/a/b.go:1.1,2.2 3 y\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseProfile(strings.NewReader(in), mod); err == nil {
				t.Fatalf("want error for %q", in)
			}
		})
	}
}

func TestParseProfileScannerError(t *testing.T) {
	if _, err := ParseProfile(&errReader{data: "mode: atomic\n"}, mod); err == nil {
		t.Fatal("want scanner error")
	}
}

// TestGate exercises the require-100 model: required packages must be exactly 100% and present in the
// profile; a package absent from the required set is explicitly excluded and reported without gating.
func TestGate(t *testing.T) {
	in := GateInput{
		Profile: map[string]PkgCov{
			"internal/fsm/kind": {Covered: 5, Total: 5},  // required, exact -> ok
			"internal/fsm/mach": {Covered: 4, Total: 5},  // required, not exact -> FAIL
			"cmd/covergate":     {Covered: 8, Total: 10}, // not required (excluded) -> ok (excluded), even below 100
		},
		Required: []string{"internal/fsm/kind", "internal/fsm/mach", "internal/fsm/absent"}, // absent -> FAIL
	}
	rows, failures := Gate(in)
	// FAILs: mach (not exact), fsm/absent (required but not in profile) = 2
	if failures != 2 {
		t.Fatalf("failures=%d want 2; rows=%+v", failures, rows)
	}
	// sorted by pkg
	for i := 1; i < len(rows); i++ {
		if rows[i-1].Pkg > rows[i].Pkg {
			t.Errorf("rows not sorted: %q > %q", rows[i-1].Pkg, rows[i].Pkg)
		}
	}
	byPkg := map[string]Row{}
	for _, r := range rows {
		byPkg[r.Pkg] = r
	}
	if byPkg["internal/fsm/kind"].Fail {
		t.Error("kind (required, exact) should pass")
	}
	if !byPkg["internal/fsm/mach"].Fail || !strings.Contains(byPkg["internal/fsm/mach"].Status, "every statement") {
		t.Errorf("mach should FAIL not-exact: %+v", byPkg["internal/fsm/mach"])
	}
	if byPkg["cmd/covergate"].Fail || !strings.Contains(byPkg["cmd/covergate"].Status, "excluded") {
		t.Errorf("cmd/covergate (excluded) should pass and read excluded even below 100: %+v", byPkg["cmd/covergate"])
	}
	if !byPkg["internal/fsm/absent"].Fail || byPkg["internal/fsm/absent"].Pct != "absent" {
		t.Errorf("absent required should FAIL and read 'absent': %+v", byPkg["internal/fsm/absent"])
	}
}

func TestGateEmptyRequired(t *testing.T) {
	rows, failures := Gate(GateInput{Profile: map[string]PkgCov{}, Required: nil})
	if failures != 1 {
		t.Fatalf("empty require set should fail once, got %d", failures)
	}
	if rows[0].Pkg != "<require-100>" {
		t.Errorf("want the require-100 sentinel row, got %+v", rows[0])
	}
}
