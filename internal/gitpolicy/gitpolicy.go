// Package gitpolicy owns the ONE .gitignore block that keeps metareview's ephemeral per-clone state out of a
// consumer's commits while leaving the durable learning state committable. Both `setup` (on git-hook install)
// and `learning` (on post-merge) ensure this block. It is a zero-dependency leaf so either can import it
// without a cycle, and sharing one source keeps them from writing DIVERGENT marker comments that would orphan
// each other's line — the reason this was extracted from two copies.
package gitpolicy

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// tempFile is the slice of *os.File that atomicWrite needs. It exists so the file-creation seam below can
// hand back a fake whose Write/Close fail on demand — a small write to a real fresh temp file never fails, so
// those defensive branches are only reachable through dependency injection.
type tempFile interface {
	io.Writer
	Close() error
	Name() string
}

// File-operation seams. Production wires the real os functions; tests override these to inject a failure at
// each step of the atomic write, the same way the rest of metareview seams `git`.
var (
	fsCreateTemp = func(dir, pattern string) (tempFile, error) { return os.CreateTemp(dir, pattern) }
	fsChmod      = os.Chmod
	fsRename     = os.Rename
)

// Marker is the comment line that identifies metareview's block in a .gitignore.
const Marker = "# metareview: keep ephemeral review state local while syncing durable learning state"

// Lines is the full block: ignore everything under .metareview/, then re-include the durable learning files
// (knowledge index, calibration, learning-runs) so a team can sync them. The ephemeral state — runs.jsonl,
// findings.jsonl, runs/, shards/, the materialized git-hooks — stays ignored (subsumed by .metareview/*).
var Lines = []string{
	Marker,
	".metareview/*",
	"!.metareview/knowledge/",
	".metareview/knowledge/*",
	"!.metareview/knowledge/metareview.jsonl",
	"!.metareview/calibration.jsonl",
	"!.metareview/learning-runs.jsonl",
}

// Present reports whether <root>/.gitignore already carries metareview's block (identified by its Marker).
// setup folds this into the "already installed" decision so a reinstall RESTORES a missing/blanket block
// instead of short-circuiting — otherwise an already-installed repo whose hooks are current never gets the
// ephemeral-state ignore.
func Present(root string) bool {
	b, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return false
	}
	return strings.Contains(string(b), Marker)
}

// Ensure writes the block to <root>/.gitignore idempotently. It STRIPS any prior copy of the policy lines
// (and a bare `.metareview/` blanket ignore) and re-appends the current block at the end, so re-runs never
// duplicate or orphan a line, and a pre-existing blanket `.metareview/` is UPGRADED to the allowlist (which
// makes the durable files committable again). Every other line is preserved in order. It is non-destructive:
// gitignore never untracks a file the consumer already committed. The replacement is written ATOMICALLY (temp
// file + rename) so a crash or write error cannot leave a truncated .gitignore that has lost unrelated rules.
func Ensure(root string) error {
	path := filepath.Join(root, ".gitignore")
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	lines := retained(string(b))
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "") // a blank line before our block, when the file has other content
	}
	lines = append(lines, Lines...)
	return atomicWrite(path, []byte(strings.Join(lines, "\n")+"\n"))
}

// atomicWrite replaces path in one step: it writes a temp file in the SAME directory (so the final rename is
// atomic on one filesystem) and renames it over path. A failure before the rename leaves the original intact.
func atomicWrite(path string, content []byte) error {
	tmp, err := fsCreateTemp(filepath.Dir(path), ".gitignore-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // a no-op once the rename has moved it away
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := fsChmod(tmpName, 0o644); err != nil { //nolint:gosec // repo-local .gitignore, world-readable by design
		return err
	}
	return fsRename(tmpName, path)
}

// retained returns the .gitignore lines that are NOT metareview's policy (so re-adding the block cannot
// duplicate it), with the trailing blank lines trimmed. A bare `.metareview/` blanket ignore is treated as
// policy and stripped, so Ensure replaces it with the allowlist rather than leaving the durable files ignored.
func retained(content string) []string {
	policy := map[string]bool{".metareview/": true}
	for _, l := range Lines {
		policy[l] = true
	}
	out := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if policy[strings.TrimSpace(line)] {
			continue
		}
		out = append(out, line)
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}
