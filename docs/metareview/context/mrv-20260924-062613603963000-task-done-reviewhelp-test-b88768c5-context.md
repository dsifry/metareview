# metareview task-done context

Run ID: `mrv-20260924-062613603963000-task-done-reviewhelp-test-b88768c5`

## Task

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `review <subcommand> --help` prints usage and runs nothing. It once ran a full task-done review
// with "--help" as the target, and committed that review log (target `--help`) to main.
func TestReviewHelpPrintsUsageAndRunsNothing(t *testing.T) {
	root := gitRepo(t)
	for _, args := range [][]string{
		{"review", "--help"},
		{"review", "task-done", "--help"},
		{"review", "task-done", "-h"},
		{"review", "task-done", "docs/tasks/t.md", "--base", "main", "--help"},
		{"review", "epic-ready", "--help"},
		{"review", "pr-ready", "--help"},
		{"review", "artifact", "-h"},
		{"review", "record-lenses", "--help"},
	} {
		code, stdout, stderr := runCLI(t, root, nil, args...)
		if code != 0 || !strings.Contains(stdout, "metareview review task-done <task-id-or-path>") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "metareview", "reviews")); !os.IsNotExist(err) {
		t.Errorf("a help request wrote a review log (stat err %v)", err)
	}
}


## Git

- Base: `66e08c95afe22f322e9a131a06610729021defb9`
- Head: `04b6c044ca7168b57ab8f59f42aee391232a2d4b`
- Branch: `fix/review-help`
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `4841`
- Filtered diff bytes: `4841`
- Risk level: `none`

## Context Shard Plan

Not sharded.

## Review Manifest

- Manifest verdict: `PASS`
- Source manifest hash: not sharded
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- .claude-plugin/marketplace.json
- .claude-plugin/plugin.json
- .codex-plugin/plugin.json
- CHANGELOG.md
- cmd/metareview/main.go
- cmd/metareview/reviewhelp_test.go
- internal/version/version.go
- package.json

### Manifest Blockers
No manifest blockers.

## Changed Files

- .claude-plugin/marketplace.json
- .claude-plugin/plugin.json
- .codex-plugin/plugin.json
- CHANGELOG.md
- cmd/metareview/main.go
- cmd/metareview/reviewhelp_test.go
- internal/version/version.go
- package.json

## Diff

```diff
diff --git a/.claude-plugin/marketplace.json b/.claude-plugin/marketplace.json
index 8dc2d15..c814849 100644
--- a/.claude-plugin/marketplace.json
+++ b/.claude-plugin/marketplace.json
@@ -8,7 +8,7 @@
     {
       "name": "metareview",
       "description": "Internal review harness, adversarial gates, and post-merge learning for coding agents",
-      "version": "0.13.0",
+      "version": "0.13.1",
       "source": "./",
       "author": {
         "name": "David Sifry"
diff --git a/.claude-plugin/plugin.json b/.claude-plugin/plugin.json
index 43fa88f..1d10a96 100644
--- a/.claude-plugin/plugin.json
+++ b/.claude-plugin/plugin.json
@@ -1,6 +1,6 @@
 {
   "name": "metareview",
-  "version": "0.13.0",
+  "version": "0.13.1",
   "description": "Go-based metaswarm-compatible internal review harness for plans, specs, decompositions, task-done code review, acceptance evidence, PR readiness, and post-merge learning. Packaged releases use bin/metareview; source checkout mode requires Go 1.26+, plus FSM workflow runs (metareview fsm).",
   "author": {
     "name": "David Sifry"
diff --git a/.codex-plugin/plugin.json b/.codex-plugin/plugin.json
index b17ec80..ea6cc06 100644
--- a/.codex-plugin/plugin.json
+++ b/.codex-plugin/plugin.json
@@ -1,6 +1,6 @@
 {
   "name": "metareview",
-  "version": "0.13.0",
+  "version": "0.13.1",
   "description": "Go-based metaswarm-compatible internal review harness for plans, specs, decompositions, task-done code review, acceptance evidence, PR readiness, and post-merge learning, plus FSM workflow runs (metareview fsm).",
   "author": {
     "name": "David Sifry"
diff --git a/CHANGELOG.md b/CHANGELOG.md
index 9c63814..cf9bc53 100644
--- a/CHANGELOG.md
+++ b/CHANGELOG.md
@@ -1,5 +1,14 @@
 # Changelog
 
+## 0.13.1 - 2026-09-24
+
+### Fixed
+
+- **`metareview review <subcommand> --help` prints usage.** Before, `review task-done --help` and
+  `review epic-ready --help` treated `--help` (or `-h`) as the target and ran, and logged, a full
+  review of it. `pr-ready`, `record-lenses` and a trailing `--help` exited 2 as an unknown option.
+  Any `--help`/`-h` after `review` now prints the usage and runs nothing.
+
 ## 0.13.0 - 2026-09-24
 
 ### Added
diff --git a/cmd/metareview/main.go b/cmd/metareview/main.go
index 7472bf6..7908062 100644
--- a/cmd/metareview/main.go
+++ b/cmd/metareview/main.go
@@ -170,6 +170,13 @@ func dispatch(args []string) {
 		return
 	}
 
+	// `review <subcommand> --help` prints usage and runs nothing: without this, the review
+	// subcommands took "--help" as the target and ran (and logged) a full review of it.
+	if args[0] == "review" && slices.ContainsFunc(args[1:], func(a string) bool { return a == "--help" || a == "-h" }) {
+		printHelp()
+		return
+	}
+
 	if len(args) >= 1 && args[0] == "setup" {
 		handleSetup(args[1:])
 		return
diff --git a/cmd/metareview/reviewhelp_test.go b/cmd/metareview/reviewhelp_test.go
new file mode 100644
index 0000000..101a58a
--- /dev/null
+++ b/cmd/metareview/reviewhelp_test.go
@@ -0,0 +1,32 @@
+package main
+
+import (
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+)
+
+// `review <subcommand> --help` prints usage and runs nothing. It once ran a full task-done review
+// with "--help" as the target, and committed that review log (target `--help`) to main.
+func TestReviewHelpPrintsUsageAndRunsNothing(t *testing.T) {
+	root := gitRepo(t)
+	for _, args := range [][]string{
+		{"review", "--help"},
+		{"review", "task-done", "--help"},
+		{"review", "task-done", "-h"},
+		{"review", "task-done", "docs/tasks/t.md", "--base", "main", "--help"},
+		{"review", "epic-ready", "--help"},
+		{"review", "pr-ready", "--help"},
+		{"review", "artifact", "-h"},
+		{"review", "record-lenses", "--help"},
+	} {
+		code, stdout, stderr := runCLI(t, root, nil, args...)
+		if code != 0 || !strings.Contains(stdout, "metareview review task-done <task-id-or-path>") {
+			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
+		}
+	}
+	if _, err := os.Stat(filepath.Join(root, "docs", "metareview", "reviews")); !os.IsNotExist(err) {
+		t.Errorf("a help request wrote a review log (stat err %v)", err)
+	}
+}
diff --git a/internal/version/version.go b/internal/version/version.go
index 4821ff7..a5a5d30 100644
--- a/internal/version/version.go
+++ b/internal/version/version.go
@@ -1,3 +1,3 @@
 package version
 
-const Version = "0.13.0"
+const Version = "0.13.1"
diff --git a/package.json b/package.json
index 9831afd..8b17590 100644
--- a/package.json
+++ b/package.json
@@ -1,6 +1,6 @@
 {
   "name": "metareview",
-  "version": "0.13.0",
+  "version": "0.13.1",
   "description": "Go-based metaswarm-compatible internal review harness for plans, specs, decompositions, code, acceptance evidence, PR readiness, and post-merge learning",
   "bin": {
     "metareview": "cli/metareview.js"



```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

No external validation evidence supplied.
