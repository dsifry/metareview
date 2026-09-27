package judge

import (
	"os"
	"path/filepath"
)

// userCacheDir is os.UserCacheDir, a seam so the fallback paths are testable.
var userCacheDir = os.UserCacheDir

// isolatedDir makes the private, empty working directory a judge CLI runs in, and returns its
// cleanup. A headless CLI still loads the project configuration of the directory it starts in
// (Claude Code's .claude/settings.json hooks and CLAUDE.md; Codex's AGENTS.md), and neither
// disabling tools nor a read-only sandbox switches that off. Starting inside the repository under
// review would let that repository run code, or rewrite the judge's instructions, before the
// judge reads a byte of the prompt. The prompt arrives on stdin, so the CLI needs no files of the
// repository at all. A var so the failure path is testable.
//
// The directory lives under the user's own cache directory and nowhere else: Codex walks up from
// its working directory to a project root and loads the repo-scoped skills it finds on the way
// (project_doc_max_bytes=0 does not switch those off), so under a shared temp dir another local
// user could plant a .git root and skills in /tmp. There is deliberately no temp-dir fallback:
// without a usable cache directory the attempt fails, and the CLI judges need $HOME for their
// OAuth session anyway.
var isolatedDir = func() (string, func(), error) {
	cache, err := userCacheDir()
	if err != nil {
		return "", func() {}, err
	}
	base := filepath.Join(cache, "metareview", "judge")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", func() {}, err
	}
	dir, err := os.MkdirTemp(base, "metareview-judge-")
	if err != nil {
		return "", func() {}, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}
