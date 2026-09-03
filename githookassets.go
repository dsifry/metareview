// Package metareview (the module root) exists only to embed the git-native review-gate hook scripts, so
// `metareview setup --install-hooks` can materialize them into ANY repository. Before this, the installer
// pointed core.hooksPath at <repo>/hooks/git, which exists only in metareview's OWN checkout — in a consumer
// repo that directory is absent, git found no hooks, and the gate was silently inert while the CLI reported
// it "active". The scripts are embedded (compiled into the binary) rather than looked up on disk so the same
// bytes ship through every install path: npm, the Claude/Codex plugin, and a source checkout.
//
// This file deliberately declares ONLY the embedded FS and no functions: a package with no executable
// statements never enters the coverage profile, which keeps the module-root package out of the per-package
// coverage floor (its label differs between the two coverage gates). The committed hooks/git/ and
// hooks/session-start-check.sh files remain the single source of truth — this root file is the only place a
// //go:embed directive can reach them, since embed paths resolve against the embedding file's directory.
package metareview

import "embed"

// GitHookAssets holds the embedded hook scripts, addressed by their repo-relative path
// (e.g. "hooks/git/pre-push"). Read one with GitHookAssets.ReadFile(name).
//
//go:embed hooks/git/pre-push hooks/git/post-commit hooks/session-start-check.sh
var GitHookAssets embed.FS
