# metareview task-done context

Run ID: `mrv-20260705-040500203996000-task-done-issue-2-wu2-cursor-followup-fix-e87ecaa9`

## Task

Advisory task target: issue-2-wu2-cursor-followup-fix

## Git

- Base: `2a1cc5451f412dfe9fc4b5b6eb8b2d23cd5615a6`
- Head: `2a1cc5451f412dfe9fc4b5b6eb8b2d23cd5615a6`
- Branch: `codex/issue-2-wu2`
- Gate effect: `gate`

## Changed Files

- cmd/metareview/main.go
- internal/evidence/receipt.go
- internal/evidence/receipt_test.go
- tests/go/test-evidence.sh

## Diff

```diff


diff --git a/cmd/metareview/main.go b/cmd/metareview/main.go
index afa59c5..bdf660c 100644
--- a/cmd/metareview/main.go
+++ b/cmd/metareview/main.go
@@ -304,11 +304,20 @@ func handleEvidence(args []string) {
             fmt.Fprintln(os.Stderr, "Usage: metareview evidence run -- <command> [args...]")
             os.Exit(2)
         }
-        receipt, err := evidence.Run(context.Background(), args[separator+1:], evidence.RunOptions{})
-        exitOnErr(err)
+        receipt, runErr := evidence.Run(context.Background(), args[separator+1:], evidence.RunOptions{})
+        if runErr != nil && receipt.SchemaVersion == 0 {
+            exitOnErr(runErr)
+        }
         bytes, err := json.Marshal(receipt)
         exitOnErr(err)
         fmt.Println(string(bytes))
+        if runErr != nil {
+            fmt.Fprintln(os.Stderr, runErr)
+            if receipt.ExitCode != 0 {
+                os.Exit(receipt.ExitCode)
+            }
+            os.Exit(1)
+        }
         if receipt.ExitCode != 0 {
             os.Exit(receipt.ExitCode)
         }
@@ -337,12 +346,24 @@ func handleEvidence(args []string) {
         bytes, err := bundle.JSONL()
         exitOnErr(err)
         fmt.Print(string(bytes))
+        if bundleExitCode(bundle) != 0 {
+            os.Exit(1)
+        }
     default:
         fmt.Fprintf(os.Stderr, "Unknown evidence command: %s\n", args[0])
         os.Exit(2)
     }
 }

+func bundleExitCode(bundle evidence.Bundle) int {
+    for _, receipt := range bundle.Receipts {
+        if receipt.ExitCode != 0 {
+            return 1
+        }
+    }
+    return 0
+}
+
 func mustCwd() string {
     cwd, err := os.Getwd()
     exitOnErr(err)
diff --git a/internal/evidence/receipt.go b/internal/evidence/receipt.go
index a603240..1ee641d 100644
--- a/internal/evidence/receipt.go
+++ b/internal/evidence/receipt.go
@@ -76,45 +76,54 @@ func Parse(data []byte) (Bundle, error) {
 func ParseWithOptions(data []byte, options ParseOptions) (Bundle, error) {
     scanner := bufio.NewScanner(bytes.NewReader(data))
     var receipts []Receipt
-    jsonLines := 0
+    receiptLines := 0
     for scanner.Scan() {
         line := strings.TrimSpace(scanner.Text())
         if line == "" || !strings.HasPrefix(line, "{") {
             continue
         }
-        jsonLines++
-        receipt, err := parseReceiptLine([]byte(line), options)
+        receipt, ok, err := parseReceiptLine([]byte(line), options)
         if err != nil {
             return Bundle{}, err
         }
+        if !ok {
+            continue
+        }
+        receiptLines++
         receipts = append(receipts, receipt)
     }
     if err := scanner.Err(); err != nil {
         return Bundle{}, err
     }
-    if jsonLines > 0 {
+    if receiptLines > 0 {
         return Bundle{Receipts: receipts}, nil
     }
     return parseFreeform(data), nil
 }

-func parseReceiptLine(line []byte, options ParseOptions) (Receipt, error) {
+func parseReceiptLine(line []byte, options ParseOptions) (Receipt, bool, error) {
     var raw map[string]json.RawMessage
     if err := json.Unmarshal(line, &raw); err != nil {
-        return Receipt{}, err
+        if bytes.Contains(line, []byte("schemaVersion")) {
+            return Receipt{}, false, err
+        }
+        return Receipt{}, false, nil
+    }
+    if _, ok := raw["schemaVersion"]; !ok {
+        return Receipt{}, false, nil
     }
     var receipt Receipt
     if err := json.Unmarshal(line, &receipt); err != nil {
-        return Receipt{}, err
+        return Receipt{}, false, err
     }
     if receipt.SchemaVersion != 1 {
-        return Receipt{}, fmt.Errorf("unsupported evidence schemaVersion %d", receipt.SchemaVersion)
+        return Receipt{}, false, fmt.Errorf("unsupported evidence schemaVersion %d", receipt.SchemaVersion)
     }
     if _, ok := raw["exitCode"]; !ok {
-        return Receipt{}, errors.New("evidence receipt missing exitCode")
+        return Receipt{}, false, errors.New("evidence receipt missing exitCode")
     }
     if strings.TrimSpace(receipt.Summary) == "" {
-        return Receipt{}, errors.New("evidence receipt missing summary")
+        return Receipt{}, false, errors.New("evidence receipt missing summary")
     }
     if receipt.Kind == "" {
         receipt.Kind = ReceiptKindValidation
@@ -126,14 +135,14 @@ func parseReceiptLine(line []byte, options ParseOptions) (Receipt, error) {
                 finished = receipt.StartedAt
             }
             if finished.IsZero() {
-                return Receipt{}, errors.New("strict evidence receipt missing timestamp")
+                return Receipt{}, false, errors.New("strict evidence receipt missing timestamp")
             }
             if options.Now.Sub(finished) > options.MaxAge {
-                return Receipt{}, errors.New("strict evidence receipt is stale")
+                return Receipt{}, false, errors.New("strict evidence receipt is stale")
             }
         }
     }
-    return receipt, nil
+    return receipt, true, nil
 }

 func parseFreeform(data []byte) Bundle {
diff --git a/internal/evidence/receipt_test.go b/internal/evidence/receipt_test.go
index ed841b9..8e994cd 100644
--- a/internal/evidence/receipt_test.go
+++ b/internal/evidence/receipt_test.go
@@ -47,6 +47,19 @@ func TestFreeformFallbackRecognizesCommonSuccess(t *testing.T) {
     }
 }

+func TestFreeformFallbackIgnoresBracePrefixedNonReceiptLines(t *testing.T) {
+    bundle, err := Parse([]byte("note: command logged structured-ish text\n{this was not a receipt}\nbash tests/run-all.sh exited 0"))
+    if err != nil {
+        t.Fatalf("parse fallback with brace-prefixed note: %v", err)
+    }
+    if !bundle.HasSuccessfulValidation(KindGeneric) {
+        t.Fatalf("expected freeform fallback validation: %+v", bundle)
+    }
+    if !bundle.Fallback {
+        t.Fatalf("expected fallback marker")
+    }
+}
+
 func TestMalformedReceiptFailsStrictMode(t *testing.T) {
     input := []byte(`{"schemaVersion":1,"kind":"validation","summary":"missing exit code"}` + "\n")
     if _, err := Parse(input); err == nil {
diff --git a/tests/go/test-evidence.sh b/tests/go/test-evidence.sh
index 007ecb3..baead47 100644
--- a/tests/go/test-evidence.sh
+++ b/tests/go/test-evidence.sh
@@ -15,3 +15,44 @@ if (receipt.exitCode !== 0) throw new Error("exit code mismatch");
 if (!receipt.summary.includes("go test ./internal/evidence")) throw new Error("summary missing command");
 if (!receipt.stdoutSha256 || !receipt.stderrSha256) throw new Error("missing output hashes");
 NODE
+
+set +e
+missing_output="$(go run ./cmd/metareview evidence run -- __metareview_missing_command__ 2>/tmp/metareview-evidence-missing.err)"
+missing_status=$?
+set -e
+if [ "$missing_status" -eq 0 ]; then
+  echo "missing command evidence run should exit nonzero"
+  exit 1
+fi
+RECEIPT="$missing_output" node - <<'NODE'
+const receipt = JSON.parse(process.env.RECEIPT);
+if (receipt.schemaVersion !== 1) throw new Error("schema version mismatch");
+if (receipt.kind !== "validation") throw new Error("kind mismatch");
+if (receipt.exitCode === 0) throw new Error("missing command should have nonzero exit code");
+if (!receipt.summary.includes("__metareview_missing_command__")) throw new Error("summary missing command");
+NODE
+
+fakebin="$(mktemp -d)"
+trap 'rm -rf "$fakebin"' EXIT
+cat > "$fakebin/gh" <<'SH'
+#!/usr/bin/env bash
+printf '[{"name":"lint","bucket":"fail","state":"FAILURE"}]\n'
+exit 1
+SH
+chmod +x "$fakebin/gh"
+
+set +e
+import_output="$(PATH="$fakebin:$PATH" go run ./cmd/metareview evidence import --github-checks 4 2>/tmp/metareview-evidence-import.err)"
+import_status=$?
+set -e
+if [ "$import_status" -eq 0 ]; then
+  echo "failed CI import should exit nonzero"
+  exit 1
+fi
+RECEIPT="$import_output" node - <<'NODE'
+const lines = process.env.RECEIPT.trim().split(/\n+/);
+if (lines.length !== 1) throw new Error("expected one imported receipt");
+const receipt = JSON.parse(lines[0]);
+if (receipt.kind !== "ci-check") throw new Error("kind mismatch");
+if (receipt.exitCode === 0) throw new Error("failed check should have nonzero exit code");
+NODE

```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

{"schemaVersion":1,"kind":"validation","command":["go","test","./internal/evidence","./internal/reviewers","./internal/prready","./internal/gitcontext","./internal/taskdone"],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T04:04:33.000264Z","finishedAt":"2026-07-05T04:04:33.528881Z","stdoutSha256":"2b22b91019cb3a9adf88eafb87feee5db61e23964f1b9a99dc16283de23400c8","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./internal/evidence ./internal/reviewers ./internal/prready ./internal/gitcontext ./internal/taskdone exited 0"}
{"schemaVersion":1,"kind":"validation","command":["go","test","./..."],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T04:04:33.654142Z","finishedAt":"2026-07-05T04:04:33.918059Z","stdoutSha256":"c5bf51b8d16faee511cf027a21bdbde318264886e4aa534c49c531face18b4e1","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"go test ./... exited 0"}
{"schemaVersion":1,"kind":"validation","command":["bash","tests/run-all.sh"],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T04:04:33.988661Z","finishedAt":"2026-07-05T04:04:55.905968Z","stdoutSha256":"7ffeffbe5589bd2c366fad83e21410548fc41b6d228ad6c2a00bd6b210ece612","stderrSha256":"2bdaf97dbb5c8ec8319c8b5e0a4fc379528af40b511befe365922dbf0e376f6b","summary":"bash tests/run-all.sh exited 0"}
{"schemaVersion":1,"kind":"validation","command":["git","diff","--check"],"cwd":"/Users/dsifry/Developer/metareview/.worktrees/issue-2-wu2","exitCode":0,"startedAt":"2026-07-05T04:04:55.981412Z","finishedAt":"2026-07-05T04:04:55.995824Z","stdoutSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","stderrSha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","summary":"git diff --check exited 0"}
