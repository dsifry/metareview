# metareview task-done context

Run ID: `mrv-20260705-035540088635000-task-done-issue-2-wu2-review-fix-05b5e486`

## Task

Advisory task target: issue-2-wu2-review-fix

## Git

- Base: `6b7fd70ee60a18e144e4d3bbd10f8dc636c6b4dc`
- Head: `6b7fd70ee60a18e144e4d3bbd10f8dc636c6b4dc`
- Branch: `codex/issue-2-wu2`
- Gate effect: `gate`

## Changed Files

- internal/evidence/receipt.go
- internal/evidence/receipt_test.go

## Diff

```diff


diff --git a/internal/evidence/receipt.go b/internal/evidence/receipt.go
index fed5f5d..a603240 100644
--- a/internal/evidence/receipt.go
+++ b/internal/evidence/receipt.go
@@ -189,7 +189,7 @@ func (bundle Bundle) HasSuccessfulValidation(kind Kind) bool {
     if kind == KindCICheck {
         return bundle.allCIChecksSuccessful()
     }
-    if kind == KindGeneric && bundle.hasFailedCICheck() {
+    if bundle.hasFailedValidation(kind) {
         return false
     }
     for _, receipt := range bundle.Receipts {
@@ -206,27 +206,21 @@ func (bundle Bundle) HasSuccessfulValidation(kind Kind) bool {
     return false
 }

-func (bundle Bundle) hasFailedCICheck() bool {
+func (bundle Bundle) hasFailedValidation(kind Kind) bool {
     for _, receipt := range bundle.Receipts {
-        if receipt.Kind == ReceiptKindCICheck && receipt.ExitCode != 0 {
+        if receipt.ExitCode == 0 {
+            continue
+        }
+        if receipt.Kind != ReceiptKindValidation && receipt.Kind != ReceiptKindCICheck {
+            continue
+        }
+        if kind == "" || kind == KindGeneric || receiptMatchesKind(receipt, kind) {
             return true
         }
     }
     return false
 }

-func (bundle Bundle) onlyCIChecks() bool {
-    if len(bundle.Receipts) == 0 {
-        return false
-    }
-    for _, receipt := range bundle.Receipts {
-        if receipt.Kind != ReceiptKindCICheck {
-            return false
-        }
-    }
-    return true
-}
-
 func (bundle Bundle) allCIChecksSuccessful() bool {
     seen := false
     for _, receipt := range bundle.Receipts {
diff --git a/internal/evidence/receipt_test.go b/internal/evidence/receipt_test.go
index 488b51e..ed841b9 100644
--- a/internal/evidence/receipt_test.go
+++ b/internal/evidence/receipt_test.go
@@ -102,3 +102,19 @@ func TestFailedCICheckPreventsGenericValidation(t *testing.T) {
         t.Fatalf("specific local test validation should remain visible")
     }
 }
+
+func TestFailedValidationReceiptPreventsGenericValidation(t *testing.T) {
+    input := []byte(
+        `{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"exitCode":0,"summary":"go test ./... exited 0"}` + "\n" +
+            `{"schemaVersion":1,"kind":"validation","command":["go","vet","./..."],"exitCode":1,"summary":"go vet ./... exited 1"}` + "\n")
+    bundle, err := Parse(input)
+    if err != nil {
+        t.Fatalf("parse mixed receipts: %v", err)
+    }
+    if bundle.HasSuccessfulValidation(KindGeneric) {
+        t.Fatalf("failed validation receipt must prevent generic validation")
+    }
+    if !bundle.HasSuccessfulValidation(KindTests) {
+        t.Fatalf("specific successful test validation should remain visible")
+    }
+}

```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

{"schemaVersion":1,"kind":"validation","command":["go","test","./internal/evidence","./internal/reviewers","./internal/prready"],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T03:54:59.154851Z","finishedAt":"2026-07-05T03:54:59.708729Z","stdoutSha256":"dceae5a37833eeb38b7a1a552b6c9a2b64a74f26418e0be42462685f0dc8ce4d","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./internal/evidence ./internal/reviewers ./internal/prready exited 0"}
{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T03:54:59.786691Z","finishedAt":"2026-07-05T03:55:00.12423Z","stdoutSha256":"c5bf51b8d16faee511cf027a21bdbde318264886e4aa534c49c531face18b4e1","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./... exited 0"}
{"schemaVersion":1,"kind":"validation","command":["bash","tests/run-all.sh"],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T03:55:00.230396Z","finishedAt":"2026-07-05T03:55:21.113989Z","stdoutSha256":"7ffeffbe5589bd2c366fad83e21410548fc41b6d228ad6c2a00bd6b210ece612","stderrSha256":"2bdaf97dbb5c8ec8319c8b5e0a4fc379528af40b511befe365922dbf0e376f6b","summary":"bash tests/run-all.sh exited 0"}
{"schemaVersion":1,"kind":"validation","command":["git","diff","--check"],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T03:55:21.206662Z","finishedAt":"2026-07-05T03:55:21.227089Z","stdoutSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"git diff --check exited 0"}
