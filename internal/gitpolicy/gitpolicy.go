// Package gitpolicy owns the ONE .gitignore block that keeps metareview's ephemeral per-clone state out of a
// consumer's commits while leaving the durable learning state committable. Both `setup` (on git-hook install)
// and `learning` (on post-merge) ensure this block. It is a zero-dependency leaf so either can import it
// without a cycle, and sharing one source keeps them from writing DIVERGENT marker comments that would orphan
// each other's line — the reason this was extracted from two copies.
package gitpolicy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// Ensure writes the block to <root>/.gitignore idempotently. It STRIPS any prior copy of the policy lines
// (and a bare `.metareview/` blanket ignore) and re-appends the current block at the end, so re-runs never
// duplicate or orphan a line, and a pre-existing blanket `.metareview/` is UPGRADED to the allowlist (which
// makes the durable files committable again). Every other line is preserved in order. It is non-destructive:
// gitignore never untracks a file the consumer already committed.
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
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644) //nolint:gosec // repo-local .gitignore
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
