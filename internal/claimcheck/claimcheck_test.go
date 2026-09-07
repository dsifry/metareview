package claimcheck

import (
	"reflect"
	"testing"
)

func TestDetect(t *testing.T) {
	positive := []string{
		"the category argument type changed with zero tests; nothing asserts the created topic's category",
		"No test covers the new parseFoo branch.",
		"The refactored code lacks a spec for host matching.",
		"Coverage is missing for the migration path.",
		"untested error path in LoadConfig",
		"this behavior is untested and uncovered by any assertion",
		"no unit tests exist for the guard",
		"the test file is missing for this controller",
		"there is no spec exercising create/update",
	}
	for _, s := range positive {
		if c, ok := Detect(s); !ok || c != ClassTestingGap {
			t.Errorf("Detect(%q) = %v, %v; want %v, true", s, c, ok, ClassTestingGap)
		}
	}
	negative := []string{
		"The test asserts the wrong constant: it pins 5 but the code requires 8 lenses.",
		"This assertion cannot fail when the code is wrong.",
		"test-name drift: the spec named host_allowed? but exercises record_for_host.",
		"The mock returns null where the real code now returns [].",
	}
	for _, s := range negative {
		if _, ok := Detect(s); ok {
			t.Errorf("Detect(%q) = true; want false (a claim ABOUT tests is not a claim of ABSENCE)", s)
		}
	}
}

func TestIsTestPath(t *testing.T) {
	yes := []string{
		"spec/models/topic_embed_spec.rb",
		"test/javascripts/models/store-test.js.es6",
		"internal/fsm/judge/judge_test.go",
		"__tests__/parse.test.ts",
		"tests/unit/test_config.py",
		"src/foo.spec.jsx",
		"pkg/bar_test.go",
	}
	no := []string{
		"app/models/topic_embed.rb",
		"internal/fsm/judge/judge.go",
		"README.md",
		"test.txt",
		"latest.js",
	}
	for _, p := range yes {
		if !IsTestPath(p) {
			t.Errorf("IsTestPath(%q) = false; want true", p)
		}
	}
	for _, p := range no {
		if IsTestPath(p) {
			t.Errorf("IsTestPath(%q) = true; want false", p)
		}
	}
	// A fabricator lives under spec/ but is test SUPPORT, not a test: IsTestPath is a pure
	// path-shape predicate, so the carve-out is IsSupportPath, which EvidenceFor applies.
	if !IsTestPath("spec/fabricators/embeddable_host_fabricator.rb") {
		t.Error("IsTestPath must be pure path shape; spec/ dir counts")
	}
	if !IsSupportPath("spec/fabricators/embeddable_host_fabricator.rb") {
		t.Error("IsSupportPath(fabricator) = false; want true")
	}
	if IsSupportPath("spec/models/topic_embed_spec.rb") {
		t.Error("IsSupportPath(spec) = true; want false")
	}
}

// The cal.com PR 11059 example from issue #140: the finding claims zero tests for the
// per-host category assignment while the diff adds the assertion to
// spec/models/topic_embed_spec.rb. The contradicting spec must be found, and ranked by
// the tokens that identify the claimed subject.
func TestEvidenceForFindsTheContradictingSpec(t *testing.T) {
	blocks := []Block{
		{Path: "app/models/topic_embed.rb", Added: []string{
			"def category_for(eh)",
			"  eh.try(:category_id) || SiteSetting.embed_category",
			"end",
		}},
		{Path: "spec/models/topic_embed_spec.rb", Added: []string{
			"let!(:embeddable_host) { Fabricate(:embeddable_host) }",
			"expect(post.topic.category).to eq(embeddable_host.category)",
		}},
		{Path: "spec/fabricators/embeddable_host_fabricator.rb", Added: []string{
			"Fabricator(:embeddable_host) do",
			"  category_id { 1 }",
			"end",
		}},
		{Path: "spec/models/site_setting_spec.rb", Added: []string{
			"it \"is uncached\" do",
		}},
	}
	f := Finding{File: "app/models/topic_embed.rb", Text: "per-host category assignment (eh.try(:category_id)) replacing the old global SiteSetting.embed_category has no test verifying an embedded topic's category"}
	ev := EvidenceFor(blocks, f, 4)
	if len(ev) == 0 {
		t.Fatal("EvidenceFor found nothing; the contradicting spec was in the diff")
	}
	if ev[0].Path != "spec/models/topic_embed_spec.rb" {
		t.Errorf("top evidence = %q; want spec/models/topic_embed_spec.rb (all: %+v)", ev[0].Path, ev)
	}
	for _, e := range ev {
		if e.Path == "spec/models/site_setting_spec.rb" {
			t.Errorf("site_setting_spec matched without referencing the subject: %+v", e)
		}
	}
}

// A claim that names a test file that does not exist must NOT be contradicted by the
// diff's other test files unless their added lines reference the claimed subject.
func TestEvidenceForDoesNotContradictOnGenericMentions(t *testing.T) {
	blocks := []Block{
		{Path: "spec/models/unrelated_thing_spec.rb", Added: []string{
			"it \"caches the setting\" do",
		}},
	}
	f := Finding{File: "app/models/embeddable_host.rb", Text: "no spec/models/embeddable_host_spec.rb exists; validation and host_allowed? matching are untested"}
	if ev := EvidenceFor(blocks, f, 4); len(ev) != 0 {
		t.Errorf("EvidenceFor = %+v; want none (no block references embeddable_host)", ev)
	}
}

// The finding's own file is skipped even when it is test-shaped.
func TestEvidenceForSkipsOwnFile(t *testing.T) {
	blocks := []Block{
		{Path: "spec/foo_spec.rb", Added: []string{"it \"foo_spec does x\" do"}},
	}
	f := Finding{File: "spec/foo_spec.rb", Text: "spec/foo_spec.rb contains no real assertion for foo_spec"}
	if ev := EvidenceFor(blocks, f, 4); len(ev) != 0 {
		t.Errorf("EvidenceFor = %+v; want none (own file skipped)", ev)
	}
}

func TestEvidenceForCapsAndOrders(t *testing.T) {
	var blocks []Block
	for i := 0; i < 6; i++ {
		blocks = append(blocks, Block{Path: "test/t" + string(rune('a'+i)) + "_test.go", Added: []string{"func TestParseFoo(t *testing.T) {"}})
	}
	f := Finding{File: "pkg/parse.go", Text: "parseFoo has no test coverage"}
	ev := EvidenceFor(blocks, f, 2)
	if len(ev) != 2 {
		t.Fatalf("cap not applied: %+v", ev)
	}
	if ev[0].Score < ev[1].Score {
		t.Errorf("not ordered by score: %+v", ev)
	}
}

func TestSubjectTokensStrength(t *testing.T) {
	strong, weak := subjectTokens(Finding{File: "app/models/topic_embed.rb",
		Text: "no test verifies eh.try(:category_id) or SiteSetting.embed_category"})
	if !strong["topic_embed"] {
		t.Errorf("own-file stem missing from strong tokens: %v", strong)
	}
	if !strong["category_id"] {
		t.Errorf("snake_case identifier missing from strong tokens: %v", strong)
	}
	if !strong["sitesetting"] {
		t.Errorf("CamelCase identifier missing from strong tokens: %v", strong)
	}
	if len(weak) > 0 {
		t.Errorf("everything distinctive should be strong here; weak = %v", weak)
	}
}

func TestSortedKeysStable(t *testing.T) {
	got := sortedKeys(map[string]bool{"b": true, "a": true, "c": true})
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("sortedKeys = %v", got)
	}
}

// A finding whose only tokens are claim vocabulary ("no tests") yields no subject at all:
// there is nothing to search for, so no evidence.
func TestEvidenceForNoSubjectTokens(t *testing.T) {
	if ev := EvidenceFor([]Block{{Path: "spec/a_spec.rb", Added: []string{"it \"x\" do"}}}, Finding{Text: "no tests"}, 4); ev != nil {
		t.Errorf("EvidenceFor = %+v; want nil (no subject tokens)", ev)
	}
}

// max <= 0 means the default cap; blocks of one path coalesce to one candidate first.
func TestEvidenceForDefaultCap(t *testing.T) {
	var blocks []Block
	for i := 0; i < 6; i++ {
		blocks = append(blocks, Block{Path: "test/x" + string(rune('a'+i)) + "_test.go", Added: []string{"func TestParseFoo(t *testing.T) {"}})
	}
	if ev := EvidenceFor(blocks, Finding{File: "pkg/parse.go", Text: "parseFoo is untested"}, 0); len(ev) != 4 {
		t.Errorf("default cap = %d evidence files, want 4: %+v", len(ev), ev)
	}
	// same-path blocks are one candidate regardless of the cap
	blocks = nil
	for i := 0; i < 6; i++ {
		blocks = append(blocks, Block{Path: "test/x_test.go", Added: []string{"func TestParseFoo" + string(rune('a'+i)) + "(t *testing.T) {"}})
	}
	if ev := EvidenceFor(blocks, Finding{File: "pkg/parse.go", Text: "parseFoo is untested"}, 0); len(ev) != 1 {
		t.Errorf("same-path blocks coalesce to %d files, want 1: %+v", len(ev), ev)
	}
}

// A finding filed against a file with no extension still yields its stem, and a text of
// only underscores (with no file) yields no tokens: the degenerate shapes do not panic
// or misfire.
func TestSubjectTokensDegenerateShapes(t *testing.T) {
	strong, weak := subjectTokens(Finding{Text: "____"})
	if len(strong)+len(weak) != 0 {
		t.Errorf("underscore-only text yielded tokens: strong=%v weak=%v", strong, weak)
	}
	strong, _ = subjectTokens(Finding{File: "Makefile", Text: "no tests for makefile"})
	if !strong["makefile"] {
		t.Errorf("extension-less own file must still yield its stem: %v", strong)
	}
}

// Non-ASCII identifiers take the lowercase continuation path in camelJoin, not a split.
func TestCamelJoinNonASCII(t *testing.T) {
	if j, split := camelJoin("Café"); split || j != "café" {
		t.Errorf("camelJoin(Café) = %q, %v; want café, false (one part)", j, split)
	}
	if j, split := camelJoin("HTTPServer"); !split || j != "http_server" {
		t.Errorf("camelJoin(HTTPServer) = %q, %v; want http_server, true", j, split)
	}
}

// Equal scores fall back to path order, so the evidence ranking is deterministic.
func TestEvidenceForTieBreakIsPathOrder(t *testing.T) {
	blocks := []Block{
		{Path: "test/b_test.go", Added: []string{"func TestParseFooB(t *testing.T) {"}},
		{Path: "test/a_test.go", Added: []string{"func TestParseFooA(t *testing.T) {"}},
	}
	ev := EvidenceFor(blocks, Finding{File: "pkg/parse.go", Text: "parseFoo has no test coverage"}, 4)
	if len(ev) != 2 || ev[0].Path != "test/a_test.go" || ev[1].Path != "test/b_test.go" {
		t.Errorf("tie-break not by path: %+v", ev)
	}
	if ev[0].Score != ev[1].Score {
		t.Errorf("fixture must produce equal scores: %+v", ev)
	}
}

// Unequal scores order by score descending (the primary rank), path order only breaks ties.
func TestEvidenceForOrdersByScore(t *testing.T) {
	blocks := []Block{
		{Path: "test/a_test.go", Added: []string{"func TestParseFoo(t *testing.T) {"}},
		{Path: "test/b_test.go", Added: []string{"// parseFoo and loadFoo", "func TestLoadFoo(t *testing.T) {"}},
	}
	ev := EvidenceFor(blocks, Finding{File: "pkg/parse.go", Text: "loadFoo delegation in parseFoo is untested"}, 4)
	if len(ev) != 2 {
		t.Fatalf("evidence = %+v, want 2 files", ev)
	}
	if ev[0].Path != "test/b_test.go" || ev[0].Score <= ev[1].Score {
		t.Errorf("not score-ranked: %+v", ev)
	}
}

// A finding whose file is spelled with the diff-header "b/" prefix (reviewers quoting diff
// headers produce exactly this) must still skip its own file as evidence.
func TestEvidenceForSkipsOwnFileInAllSpellings(t *testing.T) {
	blocks := []Block{
		{Path: "spec/foo_spec.rb", Added: []string{"it \"foo_spec does x\" do"}},
	}
	for _, own := range []string{"spec/foo_spec.rb", "b/spec/foo_spec.rb", "./spec/foo_spec.rb", "a/spec/foo_spec.rb"} {
		f := Finding{File: own, Text: "spec/foo_spec.rb contains no real assertion for foo_spec"}
		if ev := EvidenceFor(blocks, f, 4); len(ev) != 0 {
			t.Errorf("own file spelled %q was not skipped: %+v", own, ev)
		}
	}
}

// camelJoin must see the hump in non-ASCII initials too: ÜberDriver is a compound.
func TestCamelJoinNonASCIIUpper(t *testing.T) {
	if j, split := camelJoin("ÜberDriver"); !split || j != "über_driver" {
		t.Errorf("camelJoin(ÜberDriver) = %q, %v; want über_driver, true", j, split)
	}
}

// The own-file skip is symmetric: a Block.Path spelled with a diff-header prefix must be
// recognized as the finding's own file too, not admitted as evidence against itself.
func TestEvidenceForOwnFileSymmetricOnBothSides(t *testing.T) {
	blocks := []Block{
		{Path: "b/spec/foo_spec.rb", Added: []string{"it \"foo_spec does x\" do"}},
		{Path: "a/spec/foo_spec.rb", Added: []string{"it \"foo_spec does y\" do"}},
	}
	f := Finding{File: "spec/foo_spec.rb", Text: "spec/foo_spec.rb contains no real assertion for foo_spec"}
	if ev := EvidenceFor(blocks, f, 4); len(ev) != 0 {
		t.Errorf("prefixed own-file blocks were admitted: %+v", ev)
	}
}

// CodeRabbit #145: blocks are coalesced by path before the cap — one multi-hunk test
// file must not consume every evidence slot.
func TestEvidenceForCoalescesBlocksByPath(t *testing.T) {
	var blocks []Block
	for i := 0; i < 5; i++ {
		blocks = append(blocks, Block{Path: "test/same_test.go", Added: []string{"func TestParseFoo" + string(rune('A'+i)) + "(t *testing.T) {"}})
	}
	blocks = append(blocks, Block{Path: "test/other_test.go", Added: []string{"func TestParseFooZ(t *testing.T) {"}})
	ev := EvidenceFor(blocks, Finding{File: "pkg/parse.go", Text: "parseFoo has no test coverage"}, 4)
	if len(ev) != 2 {
		t.Fatalf("evidence = %d files, want 2 (coalesced by path): %+v", len(ev), ev)
	}
	var paths []string
	for _, e := range ev {
		paths = append(paths, e.Path)
	}
	if paths[0] != "test/other_test.go" && paths[1] != "test/same_test.go" {
		t.Errorf("unexpected paths: %v", paths)
	}
	// both files admitted; the multi-block file's hits merged into ONE entry
	if len(ev) != 2 || paths[0] == paths[1] {
		t.Errorf("expected two distinct coalesced paths: %+v", ev)
	}
}

// CodeRabbit #145: support directories are matched case-insensitively, matching IsTestPath.
func TestIsSupportPathIsCaseInsensitive(t *testing.T) {
	for _, p := range []string{"tests/Fixtures/topic_spec.rb", "Spec/Fabricators/x.rb", "test/TestData/y.json"} {
		if !IsSupportPath(p) {
			t.Errorf("IsSupportPath(%q) = false; want true (case-insensitive)", p)
		}
	}
}

// Issue #146, phase 1: EvidenceFor over FULL-FILE blocks. A pre-existing covering test
// outside every changed hunk reaches this package as a Block whose Added carries the
// file's whole content at HEAD; the same admission bar must find it.
func TestEvidenceForOverFullFileBlocks(t *testing.T) {
	// The #146 shape: the diff touches only app code; the covering spec exists in the repo.
	blocks := []Block{
		{Path: "spec/models/topic_embed_spec.rb", Added: []string{
			"# frozen_string_literal: true",
			"require 'rails_helper'",
			"",
			"RSpec.describe TopicEmbed do",
			"  let!(:embeddable_host) { Fabricate(:embeddable_host) }",
			"  it 'assigns the embedded topic's category from the host' do",
			"    expect(post.topic.category).to eq(embeddable_host.category)",
			"  end",
			"end",
		}},
	}
	f := Finding{File: "app/models/topic_embed.rb", Text: "per-host category assignment has no test verifying an embedded topic's category"}
	ev := EvidenceFor(blocks, f, 4)
	if len(ev) != 1 || ev[0].Path != "spec/models/topic_embed_spec.rb" {
		t.Fatalf("EvidenceFor over full-file blocks = %+v; want the covering spec admitted", ev)
	}
	// A full-file support-path file still never admits.
	fixtures := []Block{{Path: "spec/fabricators/embeddable_host_fabricator.rb", Added: []string{
		"Fabricator(:embeddable_host) do",
		"  category_id { 1 }",
		"end",
	}}}
	if ev := EvidenceFor(fixtures, f, 4); len(ev) != 0 {
		t.Errorf("support path admitted from full-file content: %+v", ev)
	}
	// The own-file skip holds for full-file content too.
	own := []Block{{Path: "spec/models/topic_embed_spec.rb", Added: []string{"describe TopicEmbed do"}}}
	if ev := EvidenceFor(own, Finding{File: "spec/models/topic_embed_spec.rb", Text: "topic_embed spec contains no real assertion for embeddable_host"}, 4); len(ev) != 0 {
		t.Errorf("own file admitted from full-file content: %+v", ev)
	}
}

// Issue #146, phase 1: SubjectTokens is the exported wrapper the git layer builds its
// candidate-selection pattern from. Sorted, lowercase, strong-then-weak, no duplicates.
func TestSubjectTokensExported(t *testing.T) {
	strong, weak := SubjectTokens(Finding{File: "app/models/topic_embed.rb",
		Text: "no test verifies eh.try(:category_id) or SiteSetting.embed_category"})
	if !reflect.DeepEqual(strong, []string{"category_id", "embed_category", "sitesetting", "topic_embed"}) {
		t.Errorf("strong = %v", strong)
	}
	if len(weak) != 0 {
		t.Errorf("weak = %v; want empty", weak)
	}
	// A finding whose only subject is a plain identifier yields weak tokens; the own-file
	// stem is still strong, and claim vocabulary in the text is an ordinary identifier.
	strong, weak = SubjectTokens(Finding{File: "app/models/rating.rb", Text: "rating average is untested"})
	if !reflect.DeepEqual(strong, []string{"rating"}) || !reflect.DeepEqual(weak, []string{"average", "rating", "untested"}) {
		t.Errorf("strong = %v, weak = %v", strong, weak)
	}
	// No subject at all.
	strong, weak = SubjectTokens(Finding{Text: "no tests"})
	if len(strong) != 0 || len(weak) != 0 {
		t.Errorf("strong = %v, weak = %v; want both empty", strong, weak)
	}
}
