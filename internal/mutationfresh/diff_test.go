package mutationfresh

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The harness runs the same vectors (templates/mutation-incremental/lib/diff.mjs), so the gate and
// the planner agree on which lines an edit touched.
func TestLineDiffVectors(t *testing.T) {
	data, err := os.ReadFile("../../testdata/mutation-incremental/diff-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		MaxEditDistance int `json:"maxEditDistance"`
		Cases           []struct {
			Name  string `json:"name"`
			Old   string `json:"old"`
			New   string `json:"new"`
			Hunks []hunk `json:"hunks"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if vectors.MaxEditDistance != maxEditDistance {
		t.Fatalf("maxEditDistance %d, vectors say %d", maxEditDistance, vectors.MaxEditDistance)
	}
	for _, c := range vectors.Cases {
		got, ok := lineDiff(c.Old, c.New)
		if !ok || len(got)+len(c.Hunks) > 0 && !reflect.DeepEqual(got, c.Hunks) {
			t.Errorf("%s: got %+v (%v), want %+v", c.Name, got, ok, c.Hunks)
		}
	}
}

func TestLineDiffGivesUpAboveTheEditDistance(t *testing.T) {
	var a, b strings.Builder
	for i := 0; i <= maxEditDistance; i++ {
		fmt.Fprintf(&a, "a%d\n", i)
		fmt.Fprintf(&b, "b%d\n", i)
	}
	if _, ok := lineDiff(a.String(), b.String()); ok {
		t.Error("a diff above the edit distance must give up")
	}
}

// Spec §11.4.3: ranges overlap for a changed hunk; a pure insertion intersects a mutant that spans
// the insertion point.
func TestHunkIntersects(t *testing.T) {
	for _, c := range []struct {
		start, end int
		h          hunk
		want       bool
	}{
		{1, 5, hunk{OldStart: 2, OldEnd: 2, NewStart: 2, NewEnd: 2}, true},
		{3, 3, hunk{OldStart: 2, OldEnd: 2, NewStart: 2, NewEnd: 2}, false},
		{1, 5, hunk{OldStart: 6, OldEnd: 5, NewStart: 6, NewEnd: 6}, false},
		{1, 5, hunk{OldStart: 3, OldEnd: 2, NewStart: 3, NewEnd: 3}, true},
	} {
		if got := hunkIntersects(c.start, c.end, c.h); got != c.want {
			t.Errorf("[%d,%d] %+v = %v", c.start, c.end, c.h, got)
		}
	}
}
