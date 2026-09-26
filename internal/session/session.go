// Package session binds a host session to the worktree its work is actually in.
//
// A Stop hook only knows the directory the host launched it in. Codex (and any host whose session
// is opened on one checkout while the agent works in a sibling worktree via `cd`) reports the
// launch checkout both as the hook's working directory and as the payload's `cwd` — captured on
// 2026-09-26: session opened on /Users/dsifry/Developer/thread (main), work in
// /Users/dsifry/Developer/thread-keeper-mvp. The hook therefore evaluated main on every turn and
// blocked on main's genuinely-unreviewed files, which the session had no reason to touch and no
// way to clear: a livelock the yield had to paper over.
//
// Nothing in the host payload names the worktree, so the session has to say so, once:
// `metareview session bind <session-id> <worktree>`. The hook already receives the session id, so
// it can quote the exact command in its block reason; the agent never has to discover an id the
// host may not expose to it.
//
// A binding SELECTS a checkout; it never exempts one. The bound worktree's own pending reviews
// block exactly as they would if the host had launched there, and the hook names the checkout it
// evaluated, so a binding chosen to dodge a blocker is visible in the transcript. Bindings are
// accepted only for worktrees of the SAME repository (same git common directory) and are
// re-validated on every resolve, so neither a stale nor an edited binding can point the gate at an
// unrelated, passing repository.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/repo"
)

// SchemaVersion is the version of the binding file.
const SchemaVersion = 1

var (
	// ErrInvalidSessionID is returned for an id that could not safely name a file.
	ErrInvalidSessionID = errors.New("invalid session id")
	// ErrNotSameRepository is returned when the target is a checkout of a different repository.
	ErrNotSameRepository = errors.New("not a worktree of this repository")
)

// Session ids are host-generated (Codex and Claude Code both use UUIDs). The id becomes a file
// name, so anything that could traverse or hide is refused rather than escaped.
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Binding is the recorded choice, stored under the git common directory so every worktree of the
// repository — including the launch checkout the hook runs in — sees the same answer.
type Binding struct {
	SchemaVersion int    `json:"schemaVersion"`
	SessionID     string `json:"sessionId"`
	Worktree      string `json:"worktree"`
	Branch        string `json:"branch,omitempty"`
	BoundAt       string `json:"boundAt,omitempty"`
	BoundBy       string `json:"boundBy,omitempty"`
}

// Resolution is where a session's review state should be evaluated.
type Resolution struct {
	// Dir is the checkout root to evaluate: the bound worktree, or the start's own checkout.
	Dir string
	// Bound reports whether Dir came from a valid binding.
	Bound bool
	// Warning explains a binding that exists but was not used. Empty when there is nothing to say.
	Warning string
}

// gitDeadline bounds each git call. Resolve runs inside a synchronous Stop hook, which the host
// waits on; a wedged git must not hold session end.
var gitDeadline = 10 * time.Second

func gitOut(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- args are literals and validated paths
	cmd.Dir = dir
	// A GIT_DIR/GIT_WORK_TREE inherited from an enclosing git process would override cmd.Dir and
	// answer for the wrong repository — the very confusion this package exists to remove.
	cmd.Env = scrubGitEnv(os.Environ())
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("git %s timed out after %s", args[0], gitDeadline)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func scrubGitEnv(env []string) []string {
	kept := env[:0:0]
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_PREFIX":
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}

// checkout is a working tree and the common git directory it belongs to, both as git reports them
// (absolute and symlink-resolved), so two spellings of one path compare equal.
type checkout struct {
	top, common string
}

func inspect(dir string) (checkout, error) {
	if _, err := os.Stat(dir); err != nil {
		return checkout{}, err
	}
	out, err := gitOut(dir, "rev-parse", "--show-toplevel", "--path-format=absolute", "--git-common-dir")
	top, common, ok := strings.Cut(out, "\n")
	if err != nil || !ok {
		return checkout{}, fmt.Errorf("%s is not inside a git working tree", dir)
	}
	return checkout{top: top, common: common}, nil
}

// listed reports whether top is one of the working trees git itself records for dir's repository.
// Sharing the common directory is not enough: a directory holding only a `.git` FILE that points
// at the repository shares it too, without being a worktree anyone created.
func listed(dir, top string) bool {
	out, _ := gitOut(dir, "worktree", "list", "--porcelain")
	for _, line := range strings.Split(out, "\n") {
		if path, ok := strings.CutPrefix(line, "worktree "); ok && path == top {
			return true
		}
	}
	return false
}

func bindingPath(common, sessionID string) string {
	return filepath.Join(common, "metareview", "sessions", sessionID+".json")
}

func validID(sessionID string) error {
	if !sessionIDPattern.MatchString(sessionID) {
		return fmt.Errorf("%w %q: expected 1-128 characters of letters, digits, '.', '_' or '-'", ErrInvalidSessionID, sessionID)
	}
	return nil
}

// Bind records that sessionID's work is in the worktree containing target. start is any directory
// inside the repository (the command's working directory); target must be a checkout of the same
// repository. by is audit metadata only — who claims to have bound it.
func Bind(start, sessionID, target, by string, now time.Time) (Binding, error) {
	if err := validID(sessionID); err != nil {
		return Binding{}, err
	}
	here, err := inspect(start)
	if err != nil {
		return Binding{}, fmt.Errorf("metareview session bind must run inside the repository: %w", err)
	}
	// Relative to the command's working directory, which is not always the process's.
	if !filepath.IsAbs(target) {
		target = filepath.Join(start, target)
	}
	there, err := inspect(target)
	if err != nil {
		return Binding{}, err
	}
	if there.common != here.common {
		return Binding{}, fmt.Errorf("%s: %w (its git directory is %s, this repository's is %s)", there.top, ErrNotSameRepository, there.common, here.common)
	}
	if !listed(start, there.top) {
		return Binding{}, fmt.Errorf("%s: %w (`git worktree list` does not include it)", there.top, ErrNotSameRepository)
	}
	branch, _ := gitOut(there.top, "rev-parse", "--abbrev-ref", "HEAD")
	b := Binding{
		SchemaVersion: SchemaVersion,
		SessionID:     sessionID,
		Worktree:      there.top,
		Branch:        branch,
		BoundAt:       now.UTC().Format(time.RFC3339),
		BoundBy:       by,
	}
	data, _ := json.MarshalIndent(b, "", "  ") // a struct of strings and an int always marshals
	if err := writeAtomic(bindingPath(here.common, sessionID), append(data, '\n')); err != nil {
		return Binding{}, err
	}
	return b, nil
}

// writeAtomic replaces path with data via a uniquely named temp file and a rename, so a hook
// resolving concurrently reads either the old binding or the new one, never a torn write.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp-%d-%d", path, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0o644); err != nil { // #nosec G306 -- a path, not a secret
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Unbind removes sessionID's binding. removed is false when there was none.
func Unbind(start, sessionID string) (removed bool, err error) {
	if err := validID(sessionID); err != nil {
		return false, err
	}
	here, err := inspect(start)
	if err != nil {
		return false, fmt.Errorf("metareview session unbind must run inside the repository: %w", err)
	}
	err = os.Remove(bindingPath(here.common, sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Resolve answers where sessionID's review state lives, starting from the host's directory.
//
// It never fails: every path that cannot use a binding falls back to the start's own checkout —
// exactly the behaviour before bindings existed — and a binding that exists but cannot be used is
// reported in Warning rather than dropped silently.
func Resolve(start, sessionID string) Resolution {
	fallback := Resolution{Dir: repo.RootOr(start)}
	if sessionID == "" || validID(sessionID) != nil {
		return fallback
	}
	here, err := inspect(start)
	if err != nil {
		return fallback
	}
	path := bindingPath(here.common, sessionID)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fallback
	}
	unusable := func(why string) Resolution {
		fallback.Warning = fmt.Sprintf("session %s has a binding that cannot be used (%s); evaluating %s instead. Rebind with `metareview session bind %s <worktree>` or remove it with `metareview session unbind %s`.",
			sessionID, why, fallback.Dir, sessionID, sessionID)
		return fallback
	}
	if err != nil {
		return unusable(err.Error())
	}
	var b Binding
	if err := json.Unmarshal(data, &b); err != nil {
		return unusable("unreadable binding file " + path)
	}
	if b.SessionID != sessionID || b.Worktree == "" {
		return unusable("binding file " + path + " does not name this session's worktree")
	}
	there, err := inspect(b.Worktree)
	if err != nil {
		return unusable("bound worktree " + b.Worktree + " is gone: " + err.Error())
	}
	if there.common != here.common || !listed(start, there.top) {
		return unusable("bound path " + b.Worktree + " is " + ErrNotSameRepository.Error())
	}
	return Resolution{Dir: there.top, Bound: true}
}
