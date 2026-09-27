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
// The directory lives under the user's own cache directory, not a shared temp dir: Codex walks up
// from its working directory to a .git root and loads the repo-scoped skills it finds there, so
// under a world-writable /tmp another local user could plant both. The temp dir is only the
// fallback when there is no usable cache directory (no $HOME).
var isolatedDir = func() (string, func(), error) {
	base := os.TempDir()
	if cache, err := userCacheDir(); err == nil {
		if b := filepath.Join(cache, "metareview", "judge"); os.MkdirAll(b, 0o700) == nil {
			base = b
		}
	}
	dir, err := os.MkdirTemp(base, "metareview-judge-")
	if err != nil {
		return "", func() {}, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}
