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

func TestParseFloor(t *testing.T) {
	in := "# header\n\ncmd/metareview 81.2\ninternal/findings 92.7\n"
	got, err := ParseFloor(strings.NewReader(in))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got["cmd/metareview"] != 81.2 || got["internal/findings"] != 92.7 {
		t.Errorf("got %+v", got)
	}
}

func TestParseFloorErrors(t *testing.T) {
	if _, err := ParseFloor(strings.NewReader("only-one-field\n")); err == nil {
		t.Error("want malformed error")
	}
	if _, err := ParseFloor(strings.NewReader("pkg notanumber\n")); err == nil {
		t.Error("want bad-pct error")
	}
	if _, err := ParseFloor(&errReader{data: "pkg 50\n"}); err == nil {
		t.Error("want scanner error")
	}
}

func TestGate(t *testing.T) {
	in := GateInput{
		Profile: map[string]PkgCov{
			"internal/fsm/kind": {Covered: 5, Total: 5},  // required, exact -> ok
			"internal/fsm/mach": {Covered: 4, Total: 5},  // required, not exact -> FAIL rule 7
			"internal/a":        {Covered: 8, Total: 10}, // 80%, floor 80 -> ok
			"internal/b":        {Covered: 7, Total: 10}, // 70%, floor 80 -> FAIL rule 8
			"internal/c":        {Covered: 5, Total: 10}, // no floor -> FAIL rule 9
		},
		Floor: map[string]float64{
			"internal/a":    80,
			"internal/b":    80,
			"internal/gone": 50, // floored but not in profile -> FAIL rule 12
		},
		Require100: []string{"internal/fsm/kind", "internal/fsm/mach", "internal/fsm/absent"}, // absent -> FAIL rule 11
	}
	rows, failures := Gate(in)
	// FAILs: mach(7), b(8), c(9), fsm/absent(11), gone(12) = 5
	if failures != 5 {
		t.Fatalf("failures=%d want 5; rows=%+v", failures, rows)
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
		t.Error("kind should pass")
	}
	if !byPkg["internal/c"].Fail || !strings.Contains(byPkg["internal/c"].Status, "no floor") {
		t.Errorf("c should FAIL no-floor: %+v", byPkg["internal/c"])
	}
	if byPkg["internal/fsm/absent"].Pct != "absent" {
		t.Errorf("absent required should read 'absent': %+v", byPkg["internal/fsm/absent"])
	}
}

func TestGateEmptyRequired(t *testing.T) {
	rows, failures := Gate(GateInput{Profile: map[string]PkgCov{}, Require100: nil})
	if failures != 1 {
		t.Fatalf("empty require set should fail once, got %d", failures)
	}
	if rows[0].Pkg != "<require-100>" {
		t.Errorf("want the require-100 sentinel row, got %+v", rows[0])
	}
}

func TestUpdateFloor(t *testing.T) {
	profile := map[string]PkgCov{
		"internal/fsm/kind": {Covered: 5, Total: 5}, // required, excluded
		"internal/a":        {Covered: 9, Total: 10},
		"internal/b":        {Covered: 6, Total: 10},
	}
	old := map[string]float64{"internal/a": 80, "internal/b": 50}
	nf, err := UpdateFloor(profile, old, []string{"internal/fsm/kind"}, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := nf["internal/fsm/kind"]; ok {
		t.Error("required package must be excluded from the floor")
	}
	if nf["internal/a"] != 90 || nf["internal/b"] != 60 {
		t.Errorf("regenerated floor wrong: %+v", nf)
	}
}

func TestUpdateFloorRefuseLower(t *testing.T) {
	profile := map[string]PkgCov{"internal/a": {Covered: 7, Total: 10}} // 70%
	old := map[string]float64{"internal/a": 80}                         // would lower 80 -> 70
	if _, err := UpdateFloor(profile, old, nil, false); err == nil {
		t.Fatal("want refuse-lower error")
	}
	nf, err := UpdateFloor(profile, old, nil, true) // allow decrease
	if err != nil {
		t.Fatalf("allowDecrease should succeed: %v", err)
	}
	if nf["internal/a"] != 70 {
		t.Errorf("want 70, got %v", nf["internal/a"])
	}
}

func TestUpdateFloorRefuseVanished(t *testing.T) {
	profile := map[string]PkgCov{"internal/a": {Covered: 9, Total: 10}}
	old := map[string]float64{"internal/a": 80, "internal/gone": 50} // gone not in profile
	_, err := UpdateFloor(profile, old, nil, false)
	if err == nil || !strings.Contains(err.Error(), "internal/gone") {
		t.Fatalf("want vanished-package refusal naming internal/gone, got %v", err)
	}
}

func TestFormatFloor(t *testing.T) {
	out := FormatFloor(map[string]float64{"internal/b": 60, "internal/a": 90})
	if !strings.HasPrefix(out, "# Per-package") {
		t.Error("missing header")
	}
	ai := strings.Index(out, "internal/a 90.0")
	bi := strings.Index(out, "internal/b 60.0")
	if ai < 0 || bi < 0 || ai > bi {
		t.Errorf("floor not sorted/rendered: %q", out)
	}
	// round-trips through ParseFloor
	back, err := ParseFloor(strings.NewReader(out))
	if err != nil || back["internal/a"] != 90 || back["internal/b"] != 60 {
		t.Errorf("round-trip failed: %+v err=%v", back, err)
	}
}
