// Package lens is the single source of truth for metareview's required artifact-review lens set.
//
// The set is enumerated exactly once, here, in All. Every other spelling derives from it:
//   - internal/fsm/kind      uses the Display names (kind.Lenses)
//   - internal/artifactreview writes the Slugs into the scaffold "Required lenses:" marker
//   - internal/reviewlog      derives its normalized match keys from the Display names
//   - internal/contextpack    lists the Display names as Suggested Reviewers
//
// Adding a lens is a one-line edit to All and every call site updates by construction. This exists
// because those four lists were once hand-maintained copies in three different spellings, and one
// (the context pack) silently drifted — sitting at the original five names for two releases while
// three more lenses were already required. A copy invites that; a single source removes it.
//
// This package is a leaf: it imports nothing from the rest of the tree, so any package can depend
// on it without a cycle.
package lens

// Lens is one required artifact-review lens in its canonical spellings.
type Lens struct {
	// Display is the human-facing name — the reviewer-row label and the Suggested Reviewers entry
	// (e.g. "Scope and alignment"). reviewlog's normalized match key is derived from this.
	Display string
	// Slug is the hyphenated identifier the artifact scaffold writes into its "Required lenses:"
	// marker (e.g. "scope-alignment"). It must normalize to the same key as Display, which
	// reviewlog's canonicalLens fold guarantees; TestEveryLensSlugAndDisplayNormalizeToSameKey
	// enforces it for every entry, including any added later.
	Slug string
}

// All is the required lens set, in dispatch order. This is the ONLY place it is enumerated.
var All = []Lens{
	{"Feasibility", "feasibility"},
	{"Completeness", "completeness"},
	{"Scope and alignment", "scope-alignment"},
	{"Architecture", "architecture"},
	{"Intent preservation", "intent-preservation"},
	{"Security", "security"},
	{"Testing-quality", "testing-quality"},
	{"Data-migration", "data-migration"},
	{"Mechanical-precision", "mechanical-precision"},
}

// Displays returns the Display name of each lens, in order, as a fresh slice.
func Displays() []string {
	out := make([]string, len(All))
	for i, l := range All {
		out[i] = l.Display
	}
	return out
}

// Slugs returns the marker Slug of each lens, in order, as a fresh slice.
func Slugs() []string {
	out := make([]string, len(All))
	for i, l := range All {
		out[i] = l.Slug
	}
	return out
}
