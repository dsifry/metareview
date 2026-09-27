package judge

import "os"

// isolatedDir makes the private, empty working directory a judge CLI runs in, and returns its
// cleanup. A headless CLI still loads the project configuration of the directory it starts in
// (Claude Code's .claude/settings.json hooks and CLAUDE.md; Codex's AGENTS.md), and neither
// disabling tools nor a read-only sandbox switches that off. Starting inside the repository under
// review would let that repository run code, or rewrite the judge's instructions, before the
// judge reads a byte of the prompt. The prompt arrives on stdin, so the CLI needs no files of the
// repository at all. A var so the failure path is testable.
var isolatedDir = func() (string, func(), error) {
	dir, err := os.MkdirTemp("", "metareview-judge-")
	if err != nil {
		return "", func() {}, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}
