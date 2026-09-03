package status

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const zeroSHA40 = "0000000000000000000000000000000000000000"

// runReturning builds a RunGit stub that answers `rev-parse HEAD` with head and fails every other call —
// enough for the paths that decide BEFORE delegating to PushGate (they only need HEAD).
func runReturning(head string) RunGit {
	return func(_ string, args ...string) ([]byte, error) {
		if len(args) == 2 && args[0] == "rev-parse" && args[1] == "HEAD" {
			return []byte(head + "\n"), nil
		}
		return nil, errors.New("unexpected git call: " + strings.Join(args, " "))
	}
}

// A pure ref DELETION (all-zero local sha) carries no content, so PushGateForRefs must NOT block it and must
// not even need HEAD — there is nothing to compare against.
func TestPushGateForRefs_DeletionOnly(t *testing.T) {
	stdin := "refs/heads/gone " + zeroSHA40 + " refs/heads/gone deadbeef\n"
	blocked, msg, err := PushGateForRefs("/nope", "", stdin, runReturning("aaaa1111bbbb2222cccc3333dddd4444eeee5555"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Fatalf("a pure deletion must not be blocked; msg=%q", msg)
	}
}

// Empty stdin (nothing to push, or a manual invocation) is not blocked.
func TestPushGateForRefs_EmptyStdin(t *testing.T) {
	blocked, _, err := PushGateForRefs("/nope", "", "\n   \n", runReturning("aaaa"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Fatal("empty stdin must not be blocked")
	}
}

// A pushed non-deletion ref whose local sha differs from the checked-out HEAD is BLOCKED (fail closed): the
// gate measures the checked-out branch and cannot verify a different ref's content (issue #82). The message
// must name the ref and the HEAD, and must not require reaching PushGate (the stub fails every other git call).
func TestPushGateForRefs_NonHeadRefBlocks(t *testing.T) {
	head := "1111111111111111111111111111111111111111"
	pushed := "2222222222222222222222222222222222222222"
	stdin := "refs/heads/evil " + pushed + " refs/heads/main deadbeef\n"
	blocked, msg, err := PushGateForRefs("/root", "", stdin, runReturning(head))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Fatal("a non-HEAD ref must be blocked (fail closed)")
	}
	for _, want := range []string{"PUSH BLOCKED", "refs/heads/main", "refs/heads/evil", pushed[:8], head[:8], "--no-verify"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("block message must contain %q; got:\n%s", want, msg)
		}
	}
}

// A ref that DOES match HEAD alongside one that does NOT: the matching ref must not rescue the push — the
// aggregate blocks on the non-HEAD ref. (A `git push origin HEAD evil:main` must not slip evil past.)
func TestPushGateForRefs_MatchingRefDoesNotRescueMismatch(t *testing.T) {
	head := "1111111111111111111111111111111111111111"
	evil := "2222222222222222222222222222222222222222"
	stdin := "refs/heads/main " + head + " refs/heads/main aaa\n" +
		"refs/heads/evil " + evil + " refs/heads/main bbb\n"
	blocked, msg, err := PushGateForRefs("/root", "", stdin, runReturning(head))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked || !strings.Contains(msg, "refs/heads/evil") {
		t.Fatalf("a co-pushed non-HEAD ref must still block; blocked=%v msg=%q", blocked, msg)
	}
}

// A deletion mixed with a non-HEAD content ref: the deletion is ignored (not a rescue), the non-HEAD ref blocks.
func TestPushGateForRefs_DeletionPlusNonHeadBlocks(t *testing.T) {
	head := "1111111111111111111111111111111111111111"
	evil := "2222222222222222222222222222222222222222"
	stdin := "(delete) " + zeroSHA40 + " refs/heads/gone " + zeroSHA40 + "\n" +
		"refs/heads/evil " + evil + " refs/heads/main bbb\n"
	blocked, _, err := PushGateForRefs("/root", "", stdin, runReturning(head))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Fatal("a deletion must not rescue a co-pushed non-HEAD ref")
	}
}

// A TAG push (remote ref under refs/tags/) is a pointer to commits already gated on their branch, not new
// branch content — and an annotated tag's object sha never equals HEAD. It must NOT be falsely blocked (issue
// #82 acceptance: tags are not falsely blocked), even though its local sha differs from HEAD.
func TestPushGateForRefs_TagPushNotBlocked(t *testing.T) {
	head := "1111111111111111111111111111111111111111"
	tagObj := "2222222222222222222222222222222222222222" // annotated-tag object sha != HEAD
	stdin := "refs/tags/v1.0 " + tagObj + " refs/tags/v1.0 " + zeroSHA40 + "\n"
	blocked, msg, err := PushGateForRefs("/root", "", stdin, runReturning(head))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Fatalf("a tag push must not be falsely blocked; msg=%q", msg)
	}
}

// A tag co-pushed with a non-HEAD BRANCH ref: the tag is skipped, but the branch ref still blocks — skipping
// tags must not become a rescue for a real unreviewed branch push.
func TestPushGateForRefs_TagDoesNotRescueBranchMismatch(t *testing.T) {
	head := "1111111111111111111111111111111111111111"
	evil := "2222222222222222222222222222222222222222"
	stdin := "refs/tags/v1.0 " + evil + " refs/tags/v1.0 " + zeroSHA40 + "\n" +
		"refs/heads/evil " + evil + " refs/heads/main bbb\n"
	blocked, msg, err := PushGateForRefs("/root", "", stdin, runReturning(head))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked || !strings.Contains(msg, "refs/heads/evil") {
		t.Fatalf("a co-pushed tag must not rescue a non-HEAD branch ref; blocked=%v msg=%q", blocked, msg)
	}
}

// If HEAD cannot be resolved, PushGateForRefs must FAIL CLOSED (block) rather than wave the push through: not
// knowing the checked-out sha means it cannot vouch that a pushed ref matches it.
func TestPushGateForRefs_RevParseErrorFailsClosed(t *testing.T) {
	failHead := func(_ string, args ...string) ([]byte, error) { return nil, errors.New("git down") }
	stdin := "refs/heads/x 2222222222222222222222222222222222222222 refs/heads/x aaa\n"
	blocked, msg, err := PushGateForRefs("/root", "", stdin, failHead)
	if err != nil {
		t.Fatalf("a rev-parse failure must be handled as fail-closed, not surfaced as an error: %v", err)
	}
	if !blocked || !strings.Contains(msg, "HEAD") {
		t.Fatalf("an unresolvable HEAD must fail closed; blocked=%v msg=%q", blocked, msg)
	}
}

// A malformed ref line (fewer than 2 fields → no local sha) must FAIL CLOSED, never be silently ignored.
func TestPushGateForRefs_MalformedLineFailsClosed(t *testing.T) {
	blocked, _, err := PushGateForRefs("/root", "", "garbage\n", runReturning("1111"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Fatal("a malformed ref line must fail closed")
	}
}

// isZeroSHA classifies the deletion sentinel. The empty-string case must be false (an empty field is not a
// deletion — it is a malformed line the parser rejects earlier), and a sha with any non-zero digit is not one.
func TestIsZeroSHA(t *testing.T) {
	cases := map[string]bool{"": false, "0": true, zeroSHA40: true, "0000a": false, "deadbeef": false}
	for in, want := range cases {
		if got := isZeroSHA(in); got != want {
			t.Fatalf("isZeroSHA(%q)=%v, want %v", in, got, want)
		}
	}
}

// A 2-field ref line (local-ref + local-sha, no remote-ref) is unusual but parseable: the local sha is known,
// so the gate still judges it (here it mismatches HEAD → blocks) and names the ref as "(unknown remote ref)".
func TestPushGateForRefs_TwoFieldLineParses(t *testing.T) {
	head := "1111111111111111111111111111111111111111"
	stdin := "refs/heads/evil 2222222222222222222222222222222222222222\n"
	blocked, msg, err := PushGateForRefs("/root", "", stdin, runReturning(head))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked || !strings.Contains(msg, "unknown remote ref") {
		t.Fatalf("a 2-field non-HEAD ref must block and name an unknown remote ref; blocked=%v msg=%q", blocked, msg)
	}
}

// A 3-field ref line (local-ref, local-sha, remote-ref — no remote-sha) must use f[2] as the remote ref, NOT
// fall through to "(unknown remote ref)". This pins the len(f) >= 3 boundary: a 2-field line has no remote
// ref, a 3-field line does. (Kills the CONDITIONALS_BOUNDARY mutant at the >= 3 check that the 2- and 4-field
// tests alone leave alive.)
func TestPushGateForRefs_ThreeFieldLineUsesRemoteRef(t *testing.T) {
	head := "1111111111111111111111111111111111111111"
	stdin := "refs/heads/evil 2222222222222222222222222222222222222222 refs/heads/main\n"
	blocked, msg, err := PushGateForRefs("/root", "", stdin, runReturning(head))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Fatal("a 3-field non-HEAD ref must block")
	}
	if !strings.Contains(msg, "refs/heads/main") {
		t.Fatalf("a 3-field line must name f[2] (refs/heads/main) as the remote ref; got:\n%s", msg)
	}
	if strings.Contains(msg, "unknown remote ref") {
		t.Fatalf("a 3-field line has a remote ref (f[2]) and must not report it unknown; got:\n%s", msg)
	}
}

// When every non-deletion pushed sha EQUALS the checked-out HEAD, PushGateForRefs delegates to PushGate — the
// pushed content IS the checked-out branch, so it is gated exactly as before. gateRepo's work branch has an
// unreviewed work.go, so the delegated PushGate blocks with its own "across this branch" message: reaching
// that message proves control passed through to PushGate.
func TestPushGateForRefs_AllMatchDelegatesToPushGate(t *testing.T) {
	root, _, _, head := gateRepo(t)
	stdin := "refs/heads/work " + head + " refs/heads/work oldsha\n"
	blocked, msg, err := PushGateForRefs(root, "", stdin, nil) // nil → realGit, real repo
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked || !strings.Contains(msg, "across this branch") {
		t.Fatalf("an all-HEAD push must delegate to PushGate (which blocks the unreviewed branch); blocked=%v msg=%q", blocked, msg)
	}
}

// The delegation path must also be able to PASS: a branch with no changes (base == head) delegates to PushGate
// and is NOT blocked. This guards against a PushGateForRefs that always blocks once it reaches delegation.
func TestPushGateForRefs_AllMatchDelegatesAndCanPass(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "base")
	git("checkout", "-q", "-b", "work") // no new commit → base(main) == head → empty scope
	head := git("rev-parse", "HEAD")
	stdin := "refs/heads/work " + head + " refs/heads/work oldsha\n"
	blocked, msg, err := PushGateForRefs(root, "", stdin, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Fatalf("an all-HEAD push of a no-change branch must delegate and pass; msg=%q", msg)
	}
}
