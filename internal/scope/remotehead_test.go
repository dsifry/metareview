package scope

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemoteHeadNamesTheDefault is mr-as8 (10): a remote's own default branch is what its refs/remotes/<r>/HEAD
// points at, read from the one ref listing Load already makes; only a remote with no HEAD falls back to its main and
// master. A HEAD aimed outside its own remote proves nothing and is ignored.
func TestRemoteHeadNamesTheDefault(t *testing.T) {
	orig := forkPoint
	t.Cleanup(func() { forkPoint = orig })
	forkPoint = func(string) (string, bool, error) { return "base", true, nil }
	var rangeArgs string
	calls := 0
	git := func(dir string, args ...string) (string, error) {
		switch args[0] {
		case "for-each-ref":
			return "refs/heads/feat \n" +
				"refs/remotes/origin/HEAD refs/remotes/origin/develop\n" +
				"refs/remotes/origin/develop \n" +
				"refs/remotes/origin/main \n" + // origin's HEAD says develop: its main is an ordinary branch
				"refs/remotes/team/alice/HEAD refs/remotes/origin/main\n" + // points outside its remote: ignored
				"refs/remotes/team/alice/master ", nil
		case "rev-list":
			rangeArgs = strings.Join(args, " ")
		}
		return fakeGit(&calls, "", 0)(dir, args...)
	}
	s := Load("/repo", git)
	if !s.known || !s.branches["feat"] || len(s.branches) != 1 {
		t.Fatalf("Load = %+v", s)
	}
	if rangeArgs != "rev-list base..HEAD --not refs/remotes/origin/develop refs/remotes/team/alice/master --" {
		t.Fatalf("range args: %q", rangeArgs)
	}
}

// TestSlashNamedRemoteAgainstARealRepository is mr-as8 (9) and (10) in real git: a remote named team/alice whose HEAD
// names develop. A branch cut from that remote's fresh develop while local main lags keeps a merged branch's commits
// (already on team/alice/develop) out of its range, and a commit of its own stays in.
func TestSlashNamedRemoteAgainstARealRepository(t *testing.T) {
	base := t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	env = append(env, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_EDITOR=true")
	gitIn := func(dir string, args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = env
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	upstream := filepath.Join(base, "upstream")
	root := filepath.Join(base, "work")
	for _, d := range []string{upstream, root} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(upstream, "init", "-q", "-b", "develop")
	gitIn(upstream, "commit", "-q", "--allow-empty", "-m", "base")
	gitIn(root, "init", "-q", "-b", "main")
	gitIn(root, "remote", "add", "team/alice", upstream)
	gitIn(root, "fetch", "-q", "team/alice")
	gitIn(root, "remote", "set-head", "team/alice", "develop")
	gitIn(root, "switch", "-q", "-c", "main", "--force", "team/alice/develop")
	// A branch merged upstream after local main was last updated.
	gitIn(upstream, "commit", "-q", "--allow-empty", "-m", "merged work")
	merged := gitIn(upstream, "rev-parse", "HEAD")
	gitIn(root, "fetch", "-q", "team/alice")
	gitIn(root, "switch", "-q", "-c", "feat", "team/alice/develop")
	gitIn(root, "commit", "-q", "--allow-empty", "-m", "mine")
	mine := gitIn(root, "rev-parse", "HEAD")

	s := Load(root, nil)
	if !s.known || s.Current != "feat" {
		t.Fatalf("Load = %+v", s)
	}
	if s.inRange[merged] || !s.inRange[mine] {
		t.Fatalf("the remote default's commits must stay out of the range and this branch's own in: merged=%v mine=%v",
			s.inRange[merged], s.inRange[mine])
	}
}
