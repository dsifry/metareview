# metareview task-done context

Run ID: `mrv-20260705-035954884586000-task-done-issue-2-wu2-prready-filter-fix-d4005ab8`

## Task

Advisory task target: issue-2-wu2-prready-filter-fix

## Git

- Base: `23b607af2f0bd2cbcfb81e284ad710bbfc6fbb67`
- Head: `23b607af2f0bd2cbcfb81e284ad710bbfc6fbb67`
- Branch: `codex/issue-2-wu2`
- Gate effect: `gate`

## Changed Files

- internal/gitcontext/gitcontext.go
- internal/gitcontext/gitcontext_test.go
- internal/prready/review.go
- internal/taskdone/review.go

## Diff

```diff


diff --git a/internal/gitcontext/gitcontext.go b/internal/gitcontext/gitcontext.go
index deef102..a4bde40 100644
--- a/internal/gitcontext/gitcontext.go
+++ b/internal/gitcontext/gitcontext.go
@@ -40,6 +40,14 @@ type Context struct {
 }

 func Collect(root, requestedBase string) (Context, error) {
+    return collect(root, requestedBase, nil)
+}
+
+func CollectWithExcludes(root, requestedBase string, excludes []string) (Context, error) {
+    return collect(root, requestedBase, excludes)
+}
+
+func collect(root, requestedBase string, excludes []string) (Context, error) {
     base, err := resolveBase(root, requestedBase)
     if err != nil {
         return Context{}, err
@@ -48,20 +56,20 @@ func Collect(root, requestedBase string) (Context, error) {
     if err != nil {
         return Context{}, err
     }
-    diff, diffTruncated, err := limitedGit(root, "diff", base+"..HEAD")
+    diff, diffTruncated, err := limitedGit(root, withPathspec([]string{"diff", base + "..HEAD"}, excludes)...)
     if err != nil {
         return Context{}, err
     }
-    stagedDiff, stagedDiffTruncated, err := limitedGit(root, "diff", "--cached")
+    stagedDiff, stagedDiffTruncated, err := limitedGit(root, withPathspec([]string{"diff", "--cached"}, excludes)...)
     if err != nil {
         return Context{}, err
     }
-    workingTreeDiff, workingTreeDiffTruncated, err := limitedGit(root, "diff")
+    workingTreeDiff, workingTreeDiffTruncated, err := limitedGit(root, withPathspec([]string{"diff"}, excludes)...)
     if err != nil {
         return Context{}, err
     }
-    workingTreeFiles := splitLines(tryGit(root, "diff", "--name-only"))
-    untrackedFiles := splitLines(tryGit(root, "ls-files", "--others", "--exclude-standard"))
+    workingTreeFiles := splitLines(tryGit(root, withPathspec([]string{"diff", "--name-only"}, excludes)...))
+    untrackedFiles := splitLines(tryGit(root, withPathspec([]string{"ls-files", "--others", "--exclude-standard"}, excludes)...))
     untrackedExcerpts, err := readUntrackedExcerpts(root, untrackedFiles)
     if err != nil {
         return Context{}, err
@@ -71,14 +79,14 @@ func Collect(root, requestedBase string) (Context, error) {
         HeadSHA:                  head,
         Branch:                   tryGit(root, "branch", "--show-current"),
         StatusShort:              tryGit(root, "status", "--short"),
-        ChangedFiles:             splitLines(tryGit(root, "diff", "--name-only", base+"..HEAD")),
-        StagedFiles:              splitLines(tryGit(root, "diff", "--cached", "--name-only")),
+        ChangedFiles:             splitLines(tryGit(root, withPathspec([]string{"diff", "--name-only", base + "..HEAD"}, excludes)...)),
+        StagedFiles:              splitLines(tryGit(root, withPathspec([]string{"diff", "--cached", "--name-only"}, excludes)...)),
         UnstagedFiles:            workingTreeFiles,
         WorkingTreeFiles:         workingTreeFiles,
         UntrackedFiles:           untrackedFiles,
-        DiffStat:                 tryGit(root, "diff", "--stat", base+"..HEAD"),
-        StagedStat:               tryGit(root, "diff", "--cached", "--stat"),
-        WorkingTreeStat:          tryGit(root, "diff", "--stat"),
+        DiffStat:                 tryGit(root, withPathspec([]string{"diff", "--stat", base + "..HEAD"}, excludes)...),
+        StagedStat:               tryGit(root, withPathspec([]string{"diff", "--cached", "--stat"}, excludes)...),
+        WorkingTreeStat:          tryGit(root, withPathspec([]string{"diff", "--stat"}, excludes)...),
         Diff:                     diff,
         DiffTruncated:            diffTruncated,
         StagedDiff:               stagedDiff,
@@ -89,6 +97,21 @@ func Collect(root, requestedBase string) (Context, error) {
     }, nil
 }

+func withPathspec(args []string, excludes []string) []string {
+    if len(excludes) == 0 {
+        return args
+    }
+    out := append([]string{}, args...)
+    out = append(out, "--", ".")
+    for _, exclude := range excludes {
+        exclude = strings.TrimSpace(exclude)
+        if exclude != "" {
+            out = append(out, ":(exclude)"+exclude)
+        }
+    }
+    return out
+}
+
 func resolveBase(root, requestedBase string) (string, error) {
     if requestedBase != "" {
         if err := validateRef(requestedBase); err != nil {
diff --git a/internal/gitcontext/gitcontext_test.go b/internal/gitcontext/gitcontext_test.go
index 1563e7c..a308725 100644
--- a/internal/gitcontext/gitcontext_test.go
+++ b/internal/gitcontext/gitcontext_test.go
@@ -1,9 +1,75 @@
 package gitcontext

-import "testing"
+import (
+    "os"
+    "os/exec"
+    "path/filepath"
+    "strings"
+    "testing"
+)

 func TestMaxDiffBytesAccommodatesMediumDeletionReviews(t *testing.T) {
     if maxDiffBytes < 100_000 {
         t.Fatalf("maxDiffBytes = %d, want at least 100000 for medium deletion reviews", maxDiffBytes)
     }
 }
+
+func TestCollectWithExcludesKeepsGeneratedArtifactsOutOfTruncation(t *testing.T) {
+    root := t.TempDir()
+    git := func(args ...string) {
+        t.Helper()
+        cmd := exec.Command("git", args...)
+        cmd.Dir = root
+        if out, err := cmd.CombinedOutput(); err != nil {
+            t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
+        }
+    }
+    writeFile := func(rel, content string) {
+        t.Helper()
+        path := filepath.Join(root, filepath.FromSlash(rel))
+        if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
+            t.Fatalf("mkdir %s: %v", rel, err)
+        }
+        if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
+            t.Fatalf("write %s: %v", rel, err)
+        }
+    }
+
+    git("init", "-b", "main")
+    git("config", "user.email", "test@example.com")
+    git("config", "user.name", "Test User")
+    writeFile("internal/evidence/receipt.go", "package evidence\n")
+    git("add", ".")
+    git("commit", "-m", "base")
+    base := strings.TrimSpace(commandOutput(t, root, "git", "rev-parse", "HEAD"))
+
+    writeFile("internal/evidence/receipt.go", "package evidence\n\nfunc marker() {}\n")
+    writeFile("docs/metareview/context/generated.md", strings.Repeat("generated artifact\n", maxDiffBytes/10))
+    git("add", ".")
+    git("commit", "-m", "change")
+
+    context, err := CollectWithExcludes(root, base, []string{"docs/metareview/**"})
+    if err != nil {
+        t.Fatalf("collect filtered context: %v", err)
+    }
+    if context.DiffTruncated {
+        t.Fatalf("generated artifact diff should not force truncation:\n%s", context.DiffStat)
+    }
+    if strings.Contains(context.Diff, "generated artifact") {
+        t.Fatalf("generated artifact leaked into filtered diff")
+    }
+    if len(context.ChangedFiles) != 1 || context.ChangedFiles[0] != "internal/evidence/receipt.go" {
+        t.Fatalf("changed files = %#v, want only real code file", context.ChangedFiles)
+    }
+}
+
+func commandOutput(t *testing.T, dir, name string, args ...string) string {
+    t.Helper()
+    cmd := exec.Command(name, args...)
+    cmd.Dir = dir
+    out, err := cmd.Output()
+    if err != nil {
+        t.Fatalf("%s %s: %v", name, strings.Join(args, " "), err)
+    }
+    return string(out)
+}
diff --git a/internal/prready/review.go b/internal/prready/review.go
index f3cde6b..a03e8c4 100644
--- a/internal/prready/review.go
+++ b/internal/prready/review.go
@@ -81,7 +81,7 @@ func Create(root string, options Options) (Result, error) {
         now = time.Now()
     }
     report := repo.Detect(root)
-    git, err := gitcontext.Collect(root, options.Base)
+    git, err := gitcontext.CollectWithExcludes(root, options.Base, generatedMetareviewPathExcludes())
     if err != nil {
         return Result{}, err
     }
@@ -654,6 +654,10 @@ func isGeneratedMetareviewPath(path string) bool {
         strings.HasPrefix(path, "docs/metareview/")
 }

+func generatedMetareviewPathExcludes() []string {
+    return []string{".metareview", ".metareview/**", "docs/metareview", "docs/metareview/**"}
+}
+
 func readEvidence(path string) (string, error) {
     if path == "" {
         return "", nil
diff --git a/internal/taskdone/review.go b/internal/taskdone/review.go
index c436300..3558b48 100644
--- a/internal/taskdone/review.go
+++ b/internal/taskdone/review.go
@@ -81,7 +81,7 @@ func Create(root, target string, options Options) (Result, error) {
     if err != nil {
         return Result{}, err
     }
-    git, err := gitcontext.Collect(root, options.Base)
+    git, err := gitcontext.CollectWithExcludes(root, options.Base, generatedMetareviewPathExcludes())
     if err != nil {
         return Result{}, err
     }
@@ -311,6 +311,10 @@ func isGeneratedMetareviewPath(path string) bool {
         strings.HasPrefix(path, "docs/metareview/")
 }

+func generatedMetareviewPathExcludes() []string {
+    return []string{".metareview", ".metareview/**", "docs/metareview", "docs/metareview/**"}
+}
+
 func reviewerKnowledge(context knowledge.Context) reviewers.KnowledgeContext {
     facts := make([]reviewers.KnowledgeFact, 0, len(context.Facts))
     for _, fact := range context.Facts {

```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

{"schemaVersion":1,"kind":"validation","command":["go","test","./internal/gitcontext","./internal/taskdone","./internal/prready","./internal/evidence","./internal/reviewers"],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T03:59:27.562894Z","finishedAt":"2026-07-05T03:59:28.429344Z","stdoutSha256":"f5413311b2c06fb457afe52e0fd66834882494ce2dbaa2bdcbbb11c4e201605e","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./internal/gitcontext ./internal/taskdone ./internal/prready ./internal/evidence ./internal/reviewers exited 0"}
{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T03:59:28.497102Z","finishedAt":"2026-07-05T03:59:29.592747Z","stdoutSha256":"29fc89f1f5242b5c574574ce22c14d44e34c47e2f2ac03ca780b246bbbaf97a3","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./... exited 0"}
{"schemaVersion":1,"kind":"validation","command":["bash","tests/run-all.sh"],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T03:59:29.697376Z","finishedAt":"2026-07-05T03:59:50.547398Z","stdoutSha256":"0bdda0f128910bddc6a6d260a87f88c16ddf0f70c833d5a728794d27f23c9bd5","stderrSha256":"2bdaf97dbb5c8ec8319c8b5e0a4fc379528af40b511befe365922dbf0e376f6b","summary":"bash tests/run-all.sh exited 0"}
{"schemaVersion":1,"kind":"validation","command":["git","diff","--check"],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T03:59:50.617851Z","finishedAt":"2026-07-05T03:59:50.635148Z","stdoutSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"git diff --check exited 0"}
