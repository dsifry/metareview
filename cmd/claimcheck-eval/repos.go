package main

// Issue #146: the repo-side structural pass over the corpus. The harnesseval lab caches
// PR diffs but not corpus clones, so the pass takes a -repos directory of operator-provided
// clones (one per PR repo), pins each PR's head rev by fetching its pull ref, and runs the
// same two-stage search the product adjudicator runs (judge.RepoTestEvidence) behind real
// git seams. The matrix then separates diff-only from repo-only evidence — the question
// the A/B re-judge (model spend, still open in the issue) will answer against verdicts.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// gitRunner is the command seam: trimmed stdout, git's exit code, and an error only for
// a failed start (the process never ran). git's nonzero exits carry meaning at the call
// sites (1 = no grep matches), so they travel as code, not error.
type gitRunner func(ctx context.Context, dir string, args ...string) (string, int, error)

func realGit(ctx context.Context, dir string, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return strings.TrimSpace(out.String()), ee.ExitCode(), nil
		}
		return "", -1, err
	}
	return strings.TrimSpace(out.String()), 0, nil
}

// prURL parses the harnesseval record URLs: github.com/<org>/<repo>/pull/<n>.
var prURL = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/pull/(\d+)$`)

// repoPass holds one resolved clone per PR URL: the directory and the pinned head rev.
type repoPass struct {
	reposDir string
	runGit   gitRunner
	revs     map[string]string // url → head rev
	dirs     map[string]string // url → clone dir
}

// reportGit is report's git seam: the pass is built through it so a test can drive the
// per-claim search-error path without a corrupted clone. Production uses realGit.
var reportGit gitRunner = realGit

// resolveRepos pins a head rev for every record URL that has a clone. Missing clones and
// unparseable URLs are returned as one error naming each — the operator fetches what they
// were told about, exactly like the missing-diff disclosure. A failed fetch or rev-parse
// for an EXISTING clone is also a miss (pinned to nothing): measuring the wrong rev would
// be worse than measuring none.
func resolveRepos(o options, records []record, runGit gitRunner) (*repoPass, error) {
	p := &repoPass{reposDir: o.repos, runGit: runGit, revs: map[string]string{}, dirs: map[string]string{}}
	urls := map[string]bool{}
	for _, r := range records {
		urls[r.URL] = true
	}
	var missing []string
	for u := range urls {
		m := prURL.FindStringSubmatch(u)
		if m == nil {
			missing = append(missing, u)
			continue
		}
		org, repo, n := m[1], m[2], m[3]
		dir := ""
		for _, cand := range []string{
			joinDir(o.repos, org, repo),
			joinDir(o.repos, org+"-"+repo),
		} {
			if dirExists(cand) {
				dir = cand
				break
			}
		}
		if dir == "" {
			missing = append(missing, fmt.Sprintf("%s/%s#%s", org, repo, n))
			continue
		}
		ctx := context.Background()
		if _, code, err := runGit(ctx, dir, "fetch", "-q", "origin", "pull/"+n+"/head"); err != nil || code != 0 {
			missing = append(missing, fmt.Sprintf("%s/%s#%s (fetch failed)", org, repo, n))
			continue
		}
		rev, code, err := runGit(ctx, dir, "rev-parse", "FETCH_HEAD")
		if err != nil || code != 0 || rev == "" {
			missing = append(missing, fmt.Sprintf("%s/%s#%s (rev-parse failed)", org, repo, n))
			continue
		}
		p.revs[u], p.dirs[u] = rev, dir
	}
	if len(missing) > 0 {
		return p, fmt.Errorf("no corpus clone for %d PR(s): %s", len(missing), strings.Join(missing, ", "))
	}
	return p, nil
}

// rev returns the pinned head rev for a URL, empty when unresolved.
func (p *repoPass) rev(url string) string { return p.revs[url] }

// search runs the product's repo-side search for one claim at the URL's pinned head.
func (p *repoPass) search(url string, f run.Finding) (judge.RepoEvidence, error) {
	rev, dir := p.revs[url], p.dirs[url]
	if rev == "" || dir == "" {
		return judge.RepoEvidence{Ran: false}, nil
	}
	ctx := context.Background()
	return judge.RepoTestEvidence(p.grepHead(ctx, dir, rev), p.showHead(ctx, dir, rev), f, judge.MaxGapEvidenceFiles)
}

// grepHead mirrors the adjudicator's seam: git grep at the pinned rev, with the rev:
// prefix stripped and exit 1 kept distinct from failure.
func (p *repoPass) grepHead(ctx context.Context, dir, rev string) judge.GrepPaths {
	return func(pattern string) ([]string, error) {
		out, code, err := p.runGit(ctx, dir, "grep", "-l", "-i", "-E", pattern, rev)
		if err != nil {
			return nil, err
		}
		if code != 0 && code != 1 {
			return nil, fmt.Errorf("git grep -l -i -E at %s failed with exit code %d", rev, code)
		}
		prefix := rev + ":"
		var paths []string
		for _, line := range strings.Split(out, "\n") {
			if line = strings.TrimSpace(line); line == "" {
				continue
			}
			paths = append(paths, strings.TrimPrefix(line, prefix))
		}
		return paths, nil
	}
}

// showHead mirrors showFile: absence (not in the tree) stays distinct from failure, so a
// missing blob never reads as a read blob.
func (p *repoPass) showHead(ctx context.Context, dir, rev string) judge.ShowHead {
	return func(path string) ([]byte, bool, error) {
		listed, code, err := p.runGit(ctx, dir, "ls-tree", "--name-only", "-z", rev, "--", path)
		if err != nil {
			return nil, false, err
		}
		if code != 0 {
			return nil, false, fmt.Errorf("git ls-tree %s -- %s failed with exit code %d", rev, path, code)
		}
		if strings.Trim(listed, "\x00") == "" {
			return nil, false, nil
		}
		body, code, err := p.runGit(ctx, dir, "cat-file", "blob", rev+":"+path)
		if err != nil {
			return nil, false, err
		}
		if code != 0 {
			return nil, false, fmt.Errorf("git cat-file blob %s:%s failed with exit code %d", rev, path, code)
		}
		return []byte(body), true, nil
	}
}

func joinDir(parts ...string) string { return strings.Join(parts, "/") }

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
