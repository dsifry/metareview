package lens

import "testing"

// All is the single source of truth; the accessors must faithfully project it, with no empty or
// duplicate names in either spelling. A drift between Displays()/Slugs() and All, or a copy-paste
// duplicate, fails here.
func TestAllAndAccessorsAreConsistent(t *testing.T) {
	if len(All) == 0 {
		t.Fatal("the lens set must not be empty")
	}
	displays, slugs := Displays(), Slugs()
	if len(displays) != len(All) || len(slugs) != len(All) {
		t.Fatalf("accessor length mismatch: All=%d Displays=%d Slugs=%d", len(All), len(displays), len(slugs))
	}
	seenDisplay, seenSlug := map[string]bool{}, map[string]bool{}
	for i, l := range All {
		if l.Display == "" || l.Slug == "" {
			t.Fatalf("lens %d has an empty field: %+v", i, l)
		}
		if displays[i] != l.Display {
			t.Errorf("Displays()[%d] = %q, want %q", i, displays[i], l.Display)
		}
		if slugs[i] != l.Slug {
			t.Errorf("Slugs()[%d] = %q, want %q", i, slugs[i], l.Slug)
		}
		if seenDisplay[l.Display] {
			t.Errorf("duplicate Display %q", l.Display)
		}
		if seenSlug[l.Slug] {
			t.Errorf("duplicate Slug %q", l.Slug)
		}
		seenDisplay[l.Display], seenSlug[l.Slug] = true, true
	}
}
