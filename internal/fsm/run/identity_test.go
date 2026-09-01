package run

import (
	"strings"
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
	// The digit placeholder must SURVIVE the punctuation pass: a number is canonicalized to a "#"
	// token, not deleted, so a text mentioning a count stays distinct from one that does not.
	if NormalizeText("error at 42") == NormalizeText("error at") {
		t.Error("a number must not vanish — the # placeholder must survive normalization")
	}
	if !strings.Contains(NormalizeText("error at 42"), "#") {
		t.Errorf("the digit placeholder must be present: %q", NormalizeText("error at 42"))
	}
	if NormalizeText("line 42") != NormalizeText("line 51") {
		t.Error("two line numbers must canonicalize equal (T2)")
	}
}

// The file half of the key must use the repo's definition of "same file" (judge.NormalizePath):
// git a/ b/ diff prefixes, a leading ./, and path.Clean all canonicalize, or a re-discovered fault
// splits across rounds — the recall failure this freeze exists to prevent.
func TestFindingKeyCanonicalizesFileSpellings(t *testing.T) {
	want := FindingKey("internal/foo.go", "x")
	for _, alias := range []string{"a/internal/foo.go", "b/internal/foo.go", "./internal/foo.go", "internal/./foo.go", "internal/bar/../foo.go"} {
		if FindingKey(alias, "x") != want {
			t.Errorf("file spelling %q must canonicalize to the same key", alias)
		}
	}
	// SameFault's same-file continuity must also see aliases as one file.
	src := "the mock-scenario refusal is a duplicated guard only one test pins"
	reword := "a duplicated guard that only one test pins the mock-scenario refusal"
	if !SameFault("a/x.go", src, "x.go", reword) {
		t.Error("continuity must treat git-prefixed and bare spellings as the same file")
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

func TestSameFault(t *testing.T) {
	src := "the mock-scenario refusal is a duplicated guard only one test pins"
	// Exact identity (T1–T3) is the same fault regardless of file-similarity math.
	if !SameFault("a.go", "Deleting a.go:12 passes", "a.go", "[shard-1] deleting a.go:99 passes") {
		t.Error("a T2/T3 rewording in the same file must be the same fault via the exact key")
	}
	// Same file + a genuine rewording above τ is the same fault (continuity).
	reword := "a duplicated guard that only one test pins the mock-scenario refusal"
	if !SameFault("a.go", src, "a.go", reword) {
		t.Errorf("a same-file rewording at/above τ must be the same fault (sim=%.2f)", Similarity(src, reword))
	}
	// Same file but a DIFFERENT fault below τ is not (precision).
	if SameFault("a.go", src, "a.go", "the loader runs twice on init, doubling startup cost") {
		t.Error("a different fault below τ must not be merged")
	}
	// A similar rewording in a DIFFERENT file is not the same fault (the file component binds).
	if SameFault("a.go", src, "b.go", reword) {
		t.Error("continuity must be scoped to the same file")
	}
	// Two fileless findings never match by similarity alone (only by exact key).
	if SameFault("", src, "", reword) {
		t.Error("a fileless finding must not match by similarity")
	}
}

// --- §5.2 identity-scheme migration (versioned derivation, migrate-on-read) ---

// FindingKeyForScheme must reproduce each historical derivation exactly, so a persisted id minted
// under an older scheme can be recognized and translated forward from the retained (file,text).
func TestFindingKeyForSchemeReproducesHistory(t *testing.T) {
	// Scheme 0 is the pre-T0.1 text-only sha1 — file IGNORED — so its value is pinned to the
	// historical BugID output and is identical across files.
	if got := FindingKeyForScheme(0, "any/file.go", "x"); got != "11f6ad8ec52a" {
		t.Fatalf("scheme 0 must reproduce the historical text-only id, got %q", got)
	}
	if FindingKeyForScheme(0, "a.go", "x") != FindingKeyForScheme(0, "b.go", "x") {
		t.Error("scheme 0 ignores the file (text-only), so two files share the id")
	}
	// The current scheme is file-aware and equals FindingKey.
	if FindingKeyForScheme(FindingScheme, "a.go", "x") != FindingKey("a.go", "x") {
		t.Error("the current scheme must equal FindingKey")
	}
	if FindingKeyForScheme(FindingScheme, "a.go", "x") == FindingKeyForScheme(FindingScheme, "b.go", "x") {
		t.Error("the current scheme is file-aware")
	}
	// The change of derivation is real: the same finding hashes differently under 0 and the current.
	if FindingKeyForScheme(0, "a.go", "x") == FindingKeyForScheme(FindingScheme, "a.go", "x") {
		t.Error("scheme 0 and the current scheme must differ, or there is nothing to migrate")
	}
}

// MigrateFindingID translates a persisted id forward and reports whether it moved, so an override or
// Unproven gap keyed on the old id is re-found by translation, never orphaned (§5.2(a)). Dropping the
// scheme switch (FindingKeyForScheme ignoring the scheme) makes changed always false and orphans the
// override — the mutation this test kills.
func TestMigrateFindingIDResolvesAnOldKeyedOverride(t *testing.T) {
	file, text := "internal/fsm/kind/kind.go", "the guard is unpinned"
	// A run persisted under scheme 0 recorded an override keyed on the old finding id.
	oldID := FindingKeyForScheme(0, file, text)
	override := map[string]string{oldID: "granted: accepted risk"}
	// The new binary computes the current id; looking the override up under it would orphan it,
	// unless the migration translates the old id space forward from the retained (file,text).
	newID, changed := MigrateFindingID(0, file, text)
	if !changed {
		t.Fatal("migrating a scheme-0 id to the current file-aware scheme must report a change")
	}
	if newID != FindingKey(file, text) {
		t.Fatalf("migration must yield the current-scheme id, got %q", newID)
	}
	if _, orphaned := override[newID]; orphaned {
		t.Fatal("test bug: the new id must not already be present in the old-keyed map")
	}
	// Migrate-on-read: rebuild the override under the current id space from the retained (file,text).
	migrated := map[string]string{}
	for old, reason := range override {
		if FindingKeyForScheme(0, file, text) != old {
			t.Fatalf("the persisted old id must reproduce from retained text")
		}
		cur, _ := MigrateFindingID(0, file, text)
		migrated[cur] = reason
	}
	if _, ok := migrated[newID]; !ok {
		t.Fatal("the override must resolve under the current id after migration")
	}
	// An id already at the current scheme does not move.
	if _, changed := MigrateFindingID(FindingScheme, file, text); changed {
		t.Error("an id already at the current scheme must not report a change")
	}
}

// --- The T0.1 frozen-floor gate over the pre-locked labeled set ---

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
		// G4–G12: nine more real findings, each with one faithful T4 rewording (recall samples). These
		// broaden the T4 recall estimate to 12 sources; precision is measured at scale on the 274 real
		// same-file corpus pairs (see docs/specs/2026-09-01-t0.1-*; that data is local/gitignored).
		g4T4("G4", "docs/specs/2026-08-27-metareview-0.9.0-fsm-fork.md",
			"Shipped fsm diff JSON carries a dead field whose doc comment asserts a property it does not have, and the spec's CallRow contract does not declare it. machine.CallRow.Index (diff.go:29-33) is exported.",
			"The shipped fsm diff JSON exposes machine.CallRow.Index (diff.go:29-33), an exported field the spec's CallRow contract never declares and whose doc comment claims a property it lacks, a dead field on the wire."),
		g4T4("G5", "docs/tasks/m0-fsm-run-persistence.md",
			"Both task documents in this shard assert an acceptance criterion that is false at the reviewed head. m0-fsm-run-persistence.md:11 says bash tests/coverage.sh passes: internal/fsm/run and workflows at 100%.",
			"At the reviewed head the acceptance criterion in both task docs is untrue: m0-fsm-run-persistence.md:11 claims bash tests/coverage.sh passes with internal/fsm/run and workflows at 100%, but that does not hold."),
		g4T4("G6", "internal/fsm/export/export.go",
			"export.go:37 claims FS is the destination seam: every write goes through it so tests assert flags and perms literally. No test asserts the flags. The only assertion over the recorded opens is export_test.go:348.",
			"The doc at export.go:37 says every write goes through the FS seam so tests check flags and perms literally, yet no test actually asserts the flags; export_test.go:348 is the sole check over recorded opens and it does not."),
		g4T4("G7", "internal/fsm/kind/kind.go",
			"match-then-adjudicate's executor omits the spec-mandated self-Decode (spec 4.2 step 3) and justifies the omission with a false claim: validity by construction: the pre-flight bounds the size.",
			"The match-then-adjudicate executor skips the self-Decode that spec 4.2 step 3 requires, defending the skip with an incorrect validity by construction claim about the pre-flight bounding the size."),
		g4T4("G8", "internal/reviewers/taskdone.go",
			"The docs/ lint exemption is bypassable by a filename containing a space, which is the exact blind spot the change claims to have closed. Real git emits +++ b/<path> trailing TAB whenever the path contains a space.",
			"A path with a space slips past the docs/ lint exemption, the very gap the change says it fixed, because git appends a trailing TAB to +++ b/<path> for spaced paths."),
		g4T4("G9", "internal/reviewlog/reviewlog.go",
			"Four user-facing docs changed on this branch tell an agent to run FIVE artifact-review lenses; the shipped code requires EIGHT and the gate that reads the resulting log fails until all eight are present.",
			"Four docs on this branch instruct an agent to run five artifact-review lenses, but the code demands eight and the log gate stays red until all eight appear; the docs and the gate disagree."),
		g4T4("G10", "0,",
			"A stray, empty, tracked file named 0, sits at the repository root and is part of this branch's diff. It was committed in cde8d0a alongside real fixes and is almost certainly the residue of a mistyped shell redirect.",
			"An empty tracked file called 0, was committed at the repo root in cde8d0a bundled with real fixes, almost certainly a fat-fingered shell redirect, and it rides in this branch's diff."),
		g4T4("G11", "cli/metareview.js",
			"The npm launcher's go-run fallback silently reviews the wrong repository. cli/metareview.js:15 sets options = cwd: process.cwd(), stdio: inherit and the packaged-binary branch at :17-19 keeps it, but the fallback differs.",
			"In the npm launcher, the go-run fallback reviews the wrong repo without warning: metareview.js:15 sets cwd to process.cwd() and the packaged-binary path at :17-19 keeps it, but the fallback branch does not."),
		g4T4("G12", "cmd/metareview/main.go",
			"A relative --shard-result / --cross-shard-result path is accepted by the CLI and then silently thrown away by the ingester, producing a misleading missing shard result failure. main.go:619-633 mustResult.",
			"The CLI accepts a relative --shard-result / --cross-shard-result path but the ingester quietly drops it (main.go:619-633), yielding a confusing missing shard result error."),
	}
}

// g4T4 builds a source-plus-one-T4 recall group.
func g4T4(name, file, source, t4 string) group {
	return group{name, file, source, []variant{{"T4", file, t4}}}
}

// The frozen-floor gate (spike §7; τ=0.35 signed off 2026-09-01). It asserts, over the pre-locked
// labeled set, that the frozen algorithm still meets every floor: the exact key canonicalizes
// T1–T3 at 100%, SameFault (identity ∨ same-file Jaccard≥τ) carries T4 recall ≥ 90%, and no T5
// different-fault negative is ever merged (precision). A regression in NormalizeText, FindingKey, or
// τ reddens here. (Precision at scale — 274 real same-file pairs, max 0.30 — is documented evidence;
// that corpus is local/gitignored so cannot be a committed assertion.)
func TestT0IdentityMeetsFrozenFloors(t *testing.T) {
	exact := map[string]struct{ hit, total int }{}
	t4hit, t4total, t5neg := 0, 0, 0
	for _, g := range labeledGroups() {
		for _, v := range g.vars {
			switch v.class {
			case "T1", "T2", "T3": // canonicalized by the exact key
				e := exact[v.class]
				e.total++
				if FindingKey(v.file, v.text) == FindingKey(g.file, g.source) {
					e.hit++
				}
				exact[v.class] = e
			case "T4": // genuine rewording — caught by the continuity relation, not the exact key
				t4total++
				if SameFault(g.file, g.source, v.file, v.text) {
					t4hit++
				}
			case "T5": // different fault sharing vocabulary — must NEVER merge (precision)
				if SameFault(g.file, g.source, v.file, v.text) {
					t5neg++
					t.Errorf("PRECISION FAIL: negative %s-%s was merged with its source", g.name, v.class)
				}
			}
		}
	}
	for _, c := range []string{"T1", "T2", "T3"} {
		if e := exact[c]; e.hit != e.total {
			t.Errorf("%s recall floor: exact key merged %d/%d, want 100%%", c, e.hit, e.total)
		}
	}
	// T4 recall floor: ≥ 90% (Dave-set). With 12 samples that is ≥ 11.
	if t4hit*100 < 90*t4total {
		t.Errorf("T4 recall floor: SameFault caught %d/%d = %.0f%%, want ≥ 90%%", t4hit, t4total, float64(t4hit)/float64(t4total)*100)
	}
	if t5neg != 0 {
		t.Errorf("precision floor: %d negative(s) merged, want 0", t5neg)
	}
	t.Logf("frozen floors met: T1–T3 exact=100%%, T4 recall=%d/%d, precision negatives=%d", t4hit, t4total, t5neg)
}
