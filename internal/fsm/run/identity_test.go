package run

import (
	"fmt"
	"sort"
	"testing"
)

// --- Unit behaviour (the settled parts: T1–T3 canonicalization + precision) ---

func TestNormalizeTextCanonicalizesT1toT3(t *testing.T) {
	base := "Deleting run.go:273-275 passes go test ./..."
	for _, tc := range []struct{ name, variant string }{
		{"T1 case/whitespace", "deleting run.go:273-275   passes GO test ./..."},
		{"T2 line drift", "Deleting run.go:281-283 passes go test ./..."},
		{"T3 shard prefix", "[shard-3a] Deleting run.go:273-275 passes go test ./..."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if NormalizeText(base) != NormalizeText(tc.variant) {
				t.Errorf("%s must normalize equal:\n base=%q\n var =%q", tc.name, NormalizeText(base), NormalizeText(tc.variant))
			}
		})
	}
	// A genuinely different sentence must NOT normalize equal.
	if NormalizeText("the loader runs twice, doubling startup cost") == NormalizeText(base) {
		t.Error("distinct text must not normalize equal")
	}
}

func TestFindingKeyIsFileAware(t *testing.T) {
	// Same file + T1–T3 rewording → same key.
	if FindingKey("a.go", "Deleting a.go:12 passes") != FindingKey("a.go", "[shard-1] deleting a.go:99  passes") {
		t.Error("a T2/T3 rewording in the same file must share the key")
	}
	// Same text, different file → different key (the precision the file component adds).
	if FindingKey("a.go", "same text") == FindingKey("b.go", "same text") {
		t.Error("identical text in different files must not collide")
	}
	// Distinct fault, same file → different key.
	if FindingKey("a.go", "the guard is unpinned") == FindingKey("a.go", "the loader runs twice") {
		t.Error("distinct faults must not collide")
	}
	if len(FindingKey("a.go", "x")) != 12 {
		t.Error("key must be 12 hex chars")
	}
}

func TestSimilarityBounds(t *testing.T) {
	if Similarity("a b c", "a b c") != 1 {
		t.Error("identical → 1")
	}
	if Similarity("a b c", "x y z") != 0 {
		t.Error("disjoint → 0")
	}
	if s := Similarity("a b c d", "a b x y"); s <= 0 || s >= 1 {
		t.Errorf("partial overlap must be strictly between 0 and 1, got %v", s)
	}
	if Similarity("", "") != 1 {
		t.Error("two empty → 1")
	}
}

// --- The T0.1 spike measurement: how far the exact key and a similarity match carry each class ---

type variant struct {
	class string // T1,T2,T3,T4 (faithful) or T5 (negative)
	file  string
	text  string
}
type group struct {
	name   string
	file   string
	source string
	vars   []variant
}

// Dave-validated groups (worksheet 2026-09-01; A1–A4/B1–B4/C1–C4 faithful, A5/B5/C5 different faults).
func labeledGroups() []group {
	return []group{
		{"A", "internal/fsm/cli/run.go",
			"Duplicated guard, tested in one copy only: the init-side refusal of a mock scenario outside the repository is unpinned. Deleting run.go:273-275 passes go test ./...",
			[]variant{
				{"T1", "internal/fsm/cli/run.go", "duplicated guard, tested in one copy only:  the init-side refusal of a mock scenario outside the repository is unpinned.  deleting run.go:273-275 passes go test ./..."},
				{"T2", "internal/fsm/cli/run.go", "Duplicated guard, tested in one copy only: the init-side refusal of a mock scenario outside the repository is unpinned. Deleting run.go:281-283 passes go test ./..."},
				{"T3", "internal/fsm/cli/run.go", "[shard-3a] Duplicated guard, tested in one copy only: the init-side refusal of a mock scenario outside the repository is unpinned. Deleting run.go:273-275 passes go test ./..."},
				{"T4", "internal/fsm/cli/run.go", "The out-of-repo mock-scenario refusal is a duplicated guard that only one test copy pins; removing run.go:273-275 still passes go test ./..."},
				{"T5", "internal/fsm/cli/run.go", "Duplicated guard in run.go: the mock-scenario loader runs twice on init, doubling startup cost."},
			}},
		{"B", "internal/fsm/record/record_test.go",
			"record_test.go:298 is an assertion that cannot fail. nowNanos() is time.Now().UnixNano() (clock.go:5); it is 0 only at the Unix epoch, so if nowNanos() == 0 can never report anything.",
			[]variant{
				{"T1", "internal/fsm/record/record_test.go", "RECORD_TEST.GO:298 is an assertion that cannot fail. nowNanos() is time.Now().UnixNano() (clock.go:5); it is 0 only at the unix epoch, so if nowNanos() == 0 can never report anything."},
				{"T2", "internal/fsm/record/record_test.go", "record_test.go:301 is an assertion that cannot fail. nowNanos() is time.Now().UnixNano() (clock.go:7); it is 0 only at the Unix epoch, so if nowNanos() == 0 can never report anything."},
				{"T4", "internal/fsm/record/record_test.go", "The assertion at record_test.go:298 can never fire: nowNanos() returns time.Now().UnixNano(), which is zero only at the Unix epoch, so the == 0 guard is dead."},
				{"T5", "internal/fsm/record/record_test.go", "record_test.go:298 asserts the wrong property: it checks nowNanos() == 0 where it should assert the clock is monotonic across two reads."},
			}},
		{"C", "internal/markdown/inline.go",
			"fence is built with strings.Repeat at line 22 and then discarded via _ = fence — the closing-fence search compares run lengths numerically and never uses the string, allocating on every header-field parse of every review log.",
			[]variant{
				{"T1", "internal/markdown/inline.go", "fence is built with strings.Repeat at line 22 and then discarded via _ = fence — the closing-fence search compares run lengths numerically and never uses the string, allocating on every header-field parse of every review log."},
				{"T2", "internal/markdown/inline.go", "fence is built with strings.Repeat at line 26 and then discarded via _ = fence — the closing-fence search compares run lengths numerically and never uses the string, allocating on every header-field parse of every review log."},
				{"T4", "internal/markdown/inline.go", "inline.go allocates a fence string via strings.Repeat then throws it away (_ = fence); the closing-fence scan only compares run lengths, so the allocation is dead on every review-log header parse."},
				{"T5", "internal/markdown/inline.go", "inline.go's closing-fence search compares run lengths numerically and misses a fence one backtick longer than the opener."},
			}},
	}
}

// This is the load-bearing spike result. It is a measurement (t.Log), not yet a floor assertion: it
// reports, per class, what the EXACT (file, normalized-text) key merges, and the similarity of each
// faithful (T4) vs negative (T5) variant to its source — the numbers the freeze decision needs.
func TestT0IdentityMeasurement(t *testing.T) {
	groups := labeledGroups()
	exact := map[string]struct{ hit, total int }{}
	var t4sims, t5sims []float64
	for _, g := range groups {
		srcKey := FindingKey(g.file, g.source)
		for _, v := range g.vars {
			same := FindingKey(v.file, v.text) == srcKey
			if v.class != "T5" { // faithful: exact key SHOULD merge
				e := exact[v.class]
				e.total++
				if same {
					e.hit++
				}
				exact[v.class] = e
			} else if same { // negative: exact key must NOT merge (precision)
				t.Errorf("PRECISION FAIL: negative %s-%s shares the exact key with its source", g.name, v.class)
			}
			sim := Similarity(g.source, v.text)
			switch v.class {
			case "T4":
				t4sims = append(t4sims, sim)
			case "T5":
				t5sims = append(t5sims, sim)
			}
		}
	}
	classes := []string{"T1", "T2", "T3", "T4"}
	t.Log("=== EXACT (file, normalized-text) key — recall per class ===")
	for _, c := range classes {
		e := exact[c]
		if e.total > 0 {
			t.Logf("  %s: %d/%d merged by the exact key", c, e.hit, e.total)
		}
	}
	sort.Float64s(t4sims)
	sort.Float64s(t5sims)
	t.Logf("=== SIMILARITY to source ===")
	t.Logf("  T4 (true rewordings, want HIGH): %v  min=%.2f", round2(t4sims), min(t4sims))
	t.Logf("  T5 (different faults,  want LOW): %v  max=%.2f", round2(t5sims), max(t5sims))
	gap := min(t4sims) - max(t5sims)
	t.Logf("  separation (min T4 − max T5): %.2f  → %s", gap, verdict(gap))
}

func round2(xs []float64) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = float64(int(x*100+0.5)) / 100
	}
	return out
}
func min(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs {
		if x < m {
			m = x
		}
	}
	return m
}
func max(xs []float64) float64 {
	m := 0.0
	for _, x := range xs {
		if x > m {
			m = x
		}
	}
	return m
}
func verdict(gap float64) string {
	if gap > 0 {
		return fmt.Sprintf("SEPARABLE: a threshold in (%.2f) window catches every T4 and rejects every T5", gap)
	}
	return "OVERLAP: no single threshold separates T4 from T5 on this sample"
}
