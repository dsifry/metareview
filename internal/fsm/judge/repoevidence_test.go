package judge

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/run"
)

// Issue #146: the repository-side covering-test search. The diff-only search misses a
// covering test that lives outside every changed hunk; RepoTestEvidence searches the
// repository at the pinned head. Both seams are injected — the judge package never
// spawns git (the command-seam DI pattern), production wires them in internal/fsm/cli.

// fakeRepo is a hermetic double for both seams.
type fakeRepo struct {
	grepPattern string
	grepPaths   []string
	grepErr     error
	files       map[string]string
	showErrOn   map[string]bool
}

func (f *fakeRepo) grep(pattern string) ([]string, error) {
	f.grepPattern = pattern
	if f.grepErr != nil {
		return nil, f.grepErr
	}
	return f.grepPaths, nil
}

func (f *fakeRepo) show(path string) ([]byte, bool, error) {
	if f.showErrOn[path] {
		return nil, false, errFakeShow
	}
	c, ok := f.files[path]
	return []byte(c), ok, nil
}

var errFakeShow = errFake{}

type errFake struct{}

func (errFake) Error() string { return "fake show failure" }

func finding() run.Finding {
	return run.Finding{File: "app/models/topic_embed.rb", IssueText: "per-host category assignment has no test verifying an embedded topic's category"}
}

// The core #146 scenario: the covering spec exists in the repo but is NOT in the diff.
func TestRepoTestEvidenceFindsOutOfDiffCoveringTest(t *testing.T) {
	repo := &fakeRepo{
		grepPaths: []string{
			"app/models/topic_embed.rb",
			"spec/fabricators/embeddable_host_fabricator.rb",
			"spec/models/unrelated_spec.rb",
			"spec/models/topic_embed_spec.rb",
			"app/models/embeddable_host.rb",
		},
		files: map[string]string{
			"spec/models/topic_embed_spec.rb": "RSpec.describe TopicEmbed do\n  let!(:embeddable_host) { Fabricate(:embeddable_host) }\n  it 'assigns category from the host' do\n    expect(post.topic.category).to eq(embeddable_host.category)\n  end\nend\n",
			"spec/models/unrelated_spec.rb":   "RSpec.describe Unrelated do\n  it 'caches the setting' do\n  end\nend\n",
		},
	}
	ev, err := RepoTestEvidence(repo.grep, repo.show, finding(), 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	if len(ev.Evidence) != 1 || ev.Evidence[0].Path != "spec/models/topic_embed_spec.rb" {
		t.Fatalf("Evidence = %+v; want only the covering spec", ev.Evidence)
	}
	if got := ev.Content["spec/models/topic_embed_spec.rb"]; !strings.Contains(got, "embeddable_host") {
		t.Errorf("content missing for the admitted path: %q", got)
	}
	if _, ok := ev.Content["spec/models/unrelated_spec.rb"]; ok {
		t.Errorf("content carried for a path that was never admitted: %v", ev.Content)
	}
	// The grep pattern came from the claim's subject tokens, case-insensitive alternation.
	if !strings.Contains(repo.grepPattern, "topic_embed") || !strings.HasPrefix(repo.grepPattern, "(") {
		t.Errorf("grep pattern = %q", repo.grepPattern)
	}
}

// Non-test paths, support paths and the finding's own file never become candidates.
func TestRepoTestEvidenceFiltersCandidates(t *testing.T) {
	repo := &fakeRepo{
		grepPaths: []string{
			"b/app/models/topic_embed.rb", // own file, diff-header spelling
			"spec/fabricators/embeddable_host_fabricator.rb",
			"lib/authorize.rb",
		},
		files: map[string]string{
			"lib/authorize.rb": "def embeddable_host; end\n",
		},
	}
	ev, err := RepoTestEvidence(repo.grep, repo.show, finding(), 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	if len(ev.Evidence) != 0 || len(ev.Content) != 0 {
		t.Errorf("Evidence = %+v, content = %v; want none (no test-shaped candidate)", ev.Evidence, ev.Content)
	}
}

// The seam errors fail open at the caller's boundary: RepoTestEvidence reports the error,
// it never reports "searched and found nothing".
func TestRepoTestEvidenceFailsOpenOnSeamErrors(t *testing.T) {
	if _, err := RepoTestEvidence((&fakeRepo{grepErr: errFake{}}).grep, (&fakeRepo{}).show, finding(), 3); err == nil {
		t.Error("grep seam error swallowed; want error")
	}
	repo := &fakeRepo{grepPaths: []string{"spec/x_spec.rb"}, showErrOn: map[string]bool{"spec/x_spec.rb": true}}
	if _, err := RepoTestEvidence(repo.grep, repo.show, finding(), 3); err == nil {
		t.Error("show seam error swallowed; want error (skipping a candidate biases toward confirming absence)")
	}
}

// A path the grep reports but that is absent at head is skipped, not an error.
func TestRepoTestEvidenceSkipsAbsentPath(t *testing.T) {
	repo := &fakeRepo{grepPaths: []string{"spec/gone_spec.rb", "spec/here_spec.rb"},
		files: map[string]string{"spec/here_spec.rb": "it 'works' do\n  expect(post.topic.category).to be_present\nend\n"}}
	ev, err := RepoTestEvidence(repo.grep, repo.show, finding(), 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	if len(ev.Evidence) != 1 || ev.Evidence[0].Path != "spec/here_spec.rb" {
		t.Errorf("Evidence = %+v; want only spec/here_spec.rb", ev.Evidence)
	}
}

// A claim with no subject tokens searches nothing and calls no seam.
func TestRepoTestEvidenceNoSubject(t *testing.T) {
	calls := 0
	grep := func(string) ([]string, error) { calls++; return nil, nil }
	ev, err := RepoTestEvidence(grep, (&fakeRepo{}).show, run.Finding{IssueText: "no tests"}, 3)
	if err != nil || len(ev.Evidence) != 0 || len(ev.Content) != 0 {
		t.Errorf("= %+v, %v; want empty, nil", ev, err)
	}
	if calls != 0 {
		t.Errorf("grep called %d times for a subjectless claim", calls)
	}
}

// max <= 0 means MaxGapEvidenceFiles; per-file content is line-capped deterministically.
func TestRepoTestEvidenceCapAndContentBound(t *testing.T) {
	var files []string
	for i := 0; i < 5; i++ {
		files = append(files, "test/t"+string(rune('a'+i))+"_test.go")
	}
	repo := &fakeRepo{grepPaths: files}
	repo.files = map[string]string{}
	for _, p := range files {
		repo.files[p] = "func TestParseFoo(t *testing.T) {}\n"
	}
	ev, err := RepoTestEvidence(repo.grep, repo.show, run.Finding{File: "pkg/parse.go", IssueText: "parseFoo has no test coverage"}, 0)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	if len(ev.Evidence) != MaxGapEvidenceFiles {
		t.Errorf("default cap = %d, want %d", len(ev.Evidence), MaxGapEvidenceFiles)
	}
	// content is capped at MaxRepoEvidenceLines per file
	long := map[string]string{"spec/long_spec.rb": strings.Repeat("expect(post.topic.category).to be_present\n", MaxRepoEvidenceLines+50)}
	repo2 := &fakeRepo{grepPaths: []string{"spec/long_spec.rb"}, files: long}
	ev2, err := RepoTestEvidence(repo2.grep, repo2.show, finding(), 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	lines := strings.Count(ev2.Content["spec/long_spec.rb"], "\n")
	if lines != MaxRepoEvidenceLines {
		t.Errorf("content lines = %d, want %d (capped)", lines, MaxRepoEvidenceLines)
	}
}

// Determinism: identical seams yield identical results, whatever order grep reports.
func TestRepoTestEvidenceDeterministic(t *testing.T) {
	mk := func(order []string) *fakeRepo {
		r := &fakeRepo{grepPaths: order, files: map[string]string{
			"spec/a_spec.rb": "let!(:embeddable_host) { Fabricate(:embeddable_host) }\n",
			"spec/b_spec.rb": "it 'mentions embeddable_host too' do\nend\n",
		}}
		return r
	}
	a, err := RepoTestEvidence(mk([]string{"spec/b_spec.rb", "spec/a_spec.rb"}).grep, mk([]string{"spec/b_spec.rb", "spec/a_spec.rb"}).show, finding(), 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	b, err := RepoTestEvidence(mk([]string{"spec/a_spec.rb", "spec/b_spec.rb"}).grep, mk([]string{"spec/a_spec.rb", "spec/b_spec.rb"}).show, finding(), 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	if len(a.Evidence) != len(b.Evidence) {
		t.Fatalf("evidence counts differ: %d vs %d", len(a.Evidence), len(b.Evidence))
	}
	for i := range a.Evidence {
		if a.Evidence[i].Path != b.Evidence[i].Path || a.Evidence[i].Score != b.Evidence[i].Score || !reflect.DeepEqual(a.Evidence[i].Tokens, b.Evidence[i].Tokens) {
			t.Errorf("evidence[%d] differs: %+v vs %+v", i, a.Evidence[i], b.Evidence[i])
		}
	}
}

// The Evidence shape stays the claimcheck type so the audit trail and the eval share it.
func TestRepoTestEvidenceShape(t *testing.T) {
	repo := &fakeRepo{grepPaths: []string{"spec/models/topic_embed_spec.rb"},
		files: map[string]string{"spec/models/topic_embed_spec.rb": "RSpec.describe TopicEmbed do\n  expect(post.topic.category).to eq(host.category)\nend\n"}}
	ev, err := RepoTestEvidence(repo.grep, repo.show, finding(), 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	if len(ev.Evidence) != 1 || ev.Evidence[0].Path != "spec/models/topic_embed_spec.rb" {
		t.Fatalf("evidence = %+v; want the spec as claimcheck.Evidence", ev.Evidence)
	}
}

// Nil seams disable the search (mock and judge-less runs keep today's behavior).
func TestRepoTestEvidenceNilSeams(t *testing.T) {
	if ev, err := RepoTestEvidence(nil, (&fakeRepo{}).show, finding(), 3); err != nil || len(ev.Evidence) != 0 {
		t.Errorf("nil grep = %+v, %v; want empty, nil", ev, err)
	}
	if ev, err := RepoTestEvidence((&fakeRepo{}).grep, nil, finding(), 3); err != nil || len(ev.Evidence) != 0 {
		t.Errorf("nil show = %+v, %v; want empty, nil", ev, err)
	}
}

// A finding filed against a test-shaped file skips that file among the candidates even
// when grep reports it under a different spelling ("./x" vs "x").
func TestRepoTestEvidenceSkipsOwnTestShapedFile(t *testing.T) {
	repo := &fakeRepo{
		grepPaths: []string{"./spec/foo_spec.rb", "spec/bar_spec.rb"},
		files: map[string]string{
			"spec/bar_spec.rb": "it 'mentions foo_spec' do\n  expect(post.topic.category).to be_present\nend\n",
		},
	}
	ev, err := RepoTestEvidence(repo.grep, repo.show, run.Finding{File: "spec/foo_spec.rb", IssueText: "foo_spec contains no real assertion for topic category"}, 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	for _, e := range ev.Evidence {
		if e.Path == "spec/foo_spec.rb" || e.Path == "./spec/foo_spec.rb" {
			t.Errorf("own file admitted: %+v", ev.Evidence)
		}
	}
	if len(ev.Evidence) != 1 || ev.Evidence[0].Path != "spec/bar_spec.rb" {
		t.Errorf("Evidence = %+v; want only spec/bar_spec.rb", ev.Evidence)
	}
}

// More candidates than MaxRepoCandidates: reads are capped, deterministically by path.
func TestRepoTestEvidenceCandidateCap(t *testing.T) {
	repo := &fakeRepo{grepPaths: nil, files: map[string]string{}}
	for i := 0; i < MaxRepoCandidates+10; i++ {
		p := "spec/c" + string(rune('a'+i%26)) + string(rune('0'+i/26)) + "_spec.rb"
		repo.grepPaths = append(repo.grepPaths, p)
		repo.files[p] = "it 'mentions topic category' do\nend\n"
	}
	ev, err := RepoTestEvidence(repo.grep, repo.show, finding(), 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	if len(ev.Evidence) != 3 {
		t.Errorf("evidence = %d, want the cap of 3", len(ev.Evidence))
	}
	for _, e := range ev.Evidence {
		if e.Path > "spec/c"+string(rune('a'+MaxRepoCandidates%26))+string(rune('0'+MaxRepoCandidates/26))+"_spec.rb" {
			t.Errorf("candidate beyond the read cap admitted: %s", e.Path)
		}
	}
}

// The candidate cap must not be alphabetical: grep recalls by content, and the 65th
// path alphabetically can be the strongest subject match. Candidates are ranked by how
// many subject tokens their PATH carries (strong tokens weigh more) before truncation,
// so the reads spend themselves on the likeliest covering tests.
func TestRepoTestEvidenceCandidateCapPrefersSubjectPaths(t *testing.T) {
	repo := &fakeRepo{grepPaths: nil, files: map[string]string{}}
	// 64 alphabetically-early candidates whose content merely mentions the weak subject
	for i := 0; i < MaxRepoCandidates; i++ {
		p := "spec/a" + string(rune('a'+i%26)) + string(rune('0'+i/26)) + "_spec.rb"
		repo.grepPaths = append(repo.grepPaths, p)
		repo.files[p] = "it 'mentions the widget is untested here' do\nend\n"
	}
	// the strong-subject match sorts LAST, its path names the subject, and its content
	// is a strong-token hit (TopicEmbed → topic_embed) so it outranks the weak matches
	strong := "spec/topic_embed_spec.rb"
	repo.grepPaths = append(repo.grepPaths, strong)
	repo.files[strong] = "expect(TopicEmbed.category_for).to be_present\n"
	ev, err := RepoTestEvidence(repo.grep, repo.show, run.Finding{File: "app/models/topic_embed.rb", IssueText: "the widget is untested"}, 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	for _, e := range ev.Evidence {
		if e.Path == strong {
			return
		}
	}
	t.Errorf("the path-named candidate was truncated away: %+v", ev.Evidence)
}

// A small repo body is injected as-is (clipBody's no-op path), and path ranking weighs
// weak token matches in the path too.
func TestRepoTestEvidenceSmallBodyAndWeakPathScore(t *testing.T) {
	repo := &fakeRepo{grepPaths: []string{"spec/widget_spec.rb"},
		files: map[string]string{"spec/widget_spec.rb": "expect(widget).to be_present\nit is untested\n"}}
	ev, err := RepoTestEvidence(repo.grep, repo.show, run.Finding{File: "app/models/topic_embed.rb", IssueText: "the widget is untested"}, 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	if len(ev.Evidence) != 1 {
		t.Fatalf("evidence = %+v", ev.Evidence)
	}
	if !strings.Contains(ev.Content["spec/widget_spec.rb"], "expect(widget)") || len(ev.Content["spec/widget_spec.rb"]) > 100 {
		t.Errorf("a small body must be injected whole: %q", ev.Content["spec/widget_spec.rb"])
	}
}

// Path ranking weighs weak token matches too: a path carrying a weak subject token
// outranks one carrying none when the cap must cut.
func TestRepoTestEvidenceCapRanksWeakPathMatches(t *testing.T) {
	repo := &fakeRepo{grepPaths: nil, files: map[string]string{}}
	for i := 0; i < MaxRepoCandidates; i++ {
		p := "spec/b" + string(rune('a'+i%26)) + string(rune('0'+i/26)) + "_spec.rb"
		repo.grepPaths = append(repo.grepPaths, p)
		repo.files[p] = "it 'mentions the widget here' do\nend\n" // one weak token: never admitted
	}
	widget := "spec/widget_edge_spec.rb"
	repo.grepPaths = append(repo.grepPaths, widget)
	repo.files[widget] = "it 'mentions the widget is untested here' do\nend\n"
	// alphabetically widget_edge sorts last and falls past the cap; path ranking keeps it.
	// Its content is also the only two-token match, so the evidence names it.
	ev, err := RepoTestEvidence(repo.grep, repo.show, run.Finding{File: "pkg/parse.go", IssueText: "the widget is untested"}, 1)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	if len(ev.Evidence) != 1 || ev.Evidence[0].Path != widget {
		t.Errorf("evidence = %+v; want the weak-path-named candidate", ev.Evidence)
	}
}

// The admission match must run over the file's real content, not a 200-line prefix: a
// covering assertion at line 250 of a long spec is recalled by grep in full, and the
// 200-line injection cap must not decide whether the file is admitted at all.
func TestRepoTestEvidenceMatchesBeyondTheInjectionCap(t *testing.T) {
	long := ""
	for i := 0; i < MaxRepoEvidenceLines+50; i++ {
		long += "filler line that mentions nothing\n"
	}
	long += "expect(widget.polish).to eq(true)\n" // the covering line, past the cap
	repo := &fakeRepo{grepPaths: []string{"spec/long_spec.rb"}, files: map[string]string{"spec/long_spec.rb": long}}
	ev, err := RepoTestEvidence(repo.grep, repo.show, run.Finding{File: "app/models/widget.rb", IssueText: "the widget polish path is untested"}, 3)
	if err != nil {
		t.Fatalf("RepoTestEvidence: %v", err)
	}
	if len(ev.Evidence) != 1 {
		t.Fatalf("evidence = %+v; want the long spec admitted on its full content", ev.Evidence)
	}
	// the injected content stays line-capped
	if lines := strings.Count(ev.Content["spec/long_spec.rb"], "\n"); lines > MaxRepoEvidenceLines {
		t.Errorf("injected content = %d lines; want <= %d", lines, MaxRepoEvidenceLines)
	}
}
