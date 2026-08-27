# metareview task-done context

Run ID: `mrv-20260827-090547427693000-task-done-m1-m6-fsm-packages-a99c72f1`

## Task

# M1–M6: internal/fsm core packages

Implement `internal/fsm/{errs,converge,gate,workflow,machine,cmdexec,judge,mockai,kind}` per
`docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md` (r4) and `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
(r5), test-first, under the combined coverage gate (`tests/coverage.sh`), reviewed per commit range (≤ 120 KB each).

## Acceptance

- Every §7/§8 test row has a discriminating test (literal pins; goldens regression-only behind an env flag).
- `go test ./internal/fsm/...` passes; every `internal/fsm/*` package at exactly 100% statements.
- `bash tests/coverage.sh` passes (legacy floor held).
- Dependency direction per spec 2 §1 (machine imports no kinds/judge/cmdexec/workflows).
- Every LLM/shell effect behind an interface; no shell, pinned argv, exact env in `cmdexec`.


## Git

- Base: `54a05913025332c165b6fccf68ea311595885e98`
- Head: `78f74dad699c54a602e35ae85c8b8cdcb6e653e1`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `14612`
- Filtered diff bytes: `14612`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `7fd0d3361b63051b`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- .gitattributes
- docs/tasks/m1-m6-fsm-packages.md
- go.mod
- go.sum
- internal/fsm/gate/git.go
- internal/fsm/judge/testdata/prompts/adjudicate.python.txt
- internal/fsm/judge/testdata/prompts/match.python.txt
- internal/fsm/judge/testdata/prompts/still-present.python.txt
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/types.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/testdata/cmds-preimage.sha256
- internal/fsm/workflow/workflow_test.go

### Shards
- shard-01: .gitattributes, docs/tasks/m1-m6-fsm-packages.md, go.mod, go.sum, internal/fsm/gate/git.go, internal/fsm/judge/testdata/prompts/adjudicate.python.txt, internal/fsm/judge/testdata/prompts/match.python.txt, internal/fsm/judge/testdata/prompts/still-present.python.txt, internal/fsm/machine/machine_test.go, internal/fsm/machine/types.go, internal/fsm/workflow/resolve.go, internal/fsm/workflow/testdata/cmds-preimage.sha256, internal/fsm/workflow/workflow_test.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- .gitattributes
- go.mod
- go.sum
- internal/fsm/gate/git.go
- internal/fsm/judge/testdata/prompts/adjudicate.python.txt
- internal/fsm/judge/testdata/prompts/match.python.txt
- internal/fsm/judge/testdata/prompts/still-present.python.txt
- internal/fsm/machine/machine_test.go
- internal/fsm/machine/types.go
- internal/fsm/workflow/resolve.go
- internal/fsm/workflow/testdata/cmds-preimage.sha256
- internal/fsm/workflow/workflow_test.go
- docs/tasks/m1-m6-fsm-packages.md

## Diff

````diff
diff --git a/.gitattributes b/.gitattributes
new file mode 100644
index 0000000..9897ec3
--- /dev/null
+++ b/.gitattributes
@@ -0,0 +1 @@
+internal/fsm/judge/testdata/prompts/* -text
diff --git a/go.mod b/go.mod
index e9838d5..ac370ae 100644
--- a/go.mod
+++ b/go.mod
@@ -2,4 +2,4 @@ module github.com/dsifry/metareview
 
 go 1.26
 
-require gopkg.in/yaml.v3 v3.0.1 // indirect
+require gopkg.in/yaml.v3 v3.0.1
diff --git a/go.sum b/go.sum
index 4bc0337..a62c313 100644
--- a/go.sum
+++ b/go.sum
@@ -1,3 +1,4 @@
+gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405 h1:yhCVgyC4o1eVCa2tZl7eS0r+SDo693bJlVdllGtEeKM=
 gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405/go.mod h1:Co6ibVJAznAaIkqp8huTwlJQCZ016jof/cbN4VW5Yz0=
 gopkg.in/yaml.v3 v3.0.1 h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=
 gopkg.in/yaml.v3 v3.0.1/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=
diff --git a/internal/fsm/gate/git.go b/internal/fsm/gate/git.go
index 371fee3..2b635c0 100644
--- a/internal/fsm/gate/git.go
+++ b/internal/fsm/gate/git.go
@@ -10,6 +10,7 @@ import (
 	"fmt"
 	"os"
 	"os/exec"
+	"path/filepath"
 	"regexp"
 	"strconv"
 	"strings"
@@ -245,15 +246,12 @@ func (g *execGit) CommonDir(ctx context.Context) (string, error) {
 // changes (not just paths) move the hash. The scratch index lives in the
 // OS temp dir and is removed afterwards.
 func (g *execGit) WorkTree(ctx context.Context) (string, error) {
-	f, err := os.CreateTemp("", "mrv-index-*")
+	dir, err := os.MkdirTemp("", "mrv-index-*")
 	if err != nil {
 		return "", errs.Wrap(errs.E(CodeGit, "scratch index: "+err.Error(), "op", "write-tree"), err)
 	}
-	name := f.Name()
-	_ = f.Close()
-	_ = os.Remove(name)
-	defer os.Remove(name)
-	env := []string{"GIT_INDEX_FILE=" + name}
+	defer os.RemoveAll(dir)
+	env := []string{"GIT_INDEX_FILE=" + filepath.Join(dir, "index")}
 	if _, code, err := g.runEnv(ctx, env, "add", "-A", "--"); err != nil {
 		return "", err
 	} else if code != 0 {
diff --git a/internal/fsm/judge/testdata/prompts/adjudicate.python.txt b/internal/fsm/judge/testdata/prompts/adjudicate.python.txt
new file mode 100644
index 0000000..7a989c1
--- /dev/null
+++ b/internal/fsm/judge/testdata/prompts/adjudicate.python.txt
@@ -0,0 +1,20 @@
+# source: harnesseval@19ff9a8 harnesseval/adjudicate.py:21 sha256=8b7b1897172f2ee101cb363e9d517205edb295b970c8975275b8c01fc64cc784
+You are verifying whether a code review finding identifies a REAL problem in the diff.
+
+Diff (unified):
+```diff
+{diff}
+```
+
+Proposed finding:
+{candidate}
+
+Instructions:
+- Determine if this finding describes a real, verifiable problem present in the diff
+  (a bug, security issue, correctness problem, or a clear defect the code introduces).
+- It is NOT real if it is: a style nit, speculation about code not in the diff, a
+  misreading of the diff, a duplicate of something already fine, or vague/general.
+- Be strict: "real" means a reasonable reviewer would agree the diff has this problem.
+
+Respond with ONLY a JSON object:
+{{"reasoning": "brief explanation grounded in the diff", "is_real": true/false, "confidence": 0.0-1.0}}
\ No newline at end of file
diff --git a/internal/fsm/judge/testdata/prompts/match.python.txt b/internal/fsm/judge/testdata/prompts/match.python.txt
new file mode 100644
index 0000000..9d891f2
--- /dev/null
+++ b/internal/fsm/judge/testdata/prompts/match.python.txt
@@ -0,0 +1,17 @@
+# source: harnesseval@19ff9a8 harnesseval/judge.py:22 sha256=43882bd12af0f8f41cd284ff1dc248919f06895e25cdcd7fd50d7a80c53214fc
+You are evaluating AI code review tools.
+Determine if the candidate issue matches the golden (expected) comment.
+
+Golden Comment (the issue we're looking for):
+{golden_comment}
+
+Candidate Issue (from the tool's review):
+{candidate}
+
+Instructions:
+- Determine if the candidate identifies the SAME underlying issue as the golden comment
+- Accept semantic matches - different wording is fine if it's the same problem
+- Focus on whether they point to the same bug, concern, or code issue
+
+Respond with ONLY a JSON object:
+{{"reasoning": "brief explanation", "match": true/false, "confidence": 0.0-1.0}}
\ No newline at end of file
diff --git a/internal/fsm/judge/testdata/prompts/still-present.python.txt b/internal/fsm/judge/testdata/prompts/still-present.python.txt
new file mode 100644
index 0000000..d4883e9
--- /dev/null
+++ b/internal/fsm/judge/testdata/prompts/still-present.python.txt
@@ -0,0 +1,14 @@
+# source: harnesseval@19ff9a8 harnesseval/sdlc_loop.py:321 sha256=562691a1f484fc034a0e6a9734c3c9621909a2e70a2309755502f7b496763feb
+You are verifying whether a specific bug still exists in the current code.
+
+Original bug description (from a human reviewer):
+{golden_comment}
+
+Current diff (base..HEAD, after fixes were applied):
+```diff
+{repo and _diff(repo, base_ref)[:30000]}
+```
+
+Does the bug described above STILL EXIST in the current code? (True = bug is still present /
+not fixed. False = the bug has been fixed or no longer applies.)
+Respond with ONLY a JSON object: {{"reasoning": "...", "still_present": true/false}}
\ No newline at end of file
diff --git a/internal/fsm/machine/machine_test.go b/internal/fsm/machine/machine_test.go
index aab74a0..c371ffa 100644
--- a/internal/fsm/machine/machine_test.go
+++ b/internal/fsm/machine/machine_test.go
@@ -239,6 +239,14 @@ func TestM2ReviewLoop(t *testing.T) {
 	if v := m.View(); v.NextAction != NextRecord || v.Node.HasOutput {
 		t.Fatalf("view after needs_input: %+v", v)
 	}
+	if r.NeedsInput.Instructions.Input["diff_truncated"] != false {
+		t.Fatal("not truncated")
+	}
+	MaxDiffBytes = 2
+	if r := h.advance(m); r.NeedsInput.Instructions.Input["diff"] != "DI" || r.NeedsInput.Instructions.Input["diff_truncated"] != true {
+		t.Fatalf("truncated diff: %v", r.NeedsInput.Instructions.Input)
+	}
+	MaxDiffBytes = 1 << 20
 	// a second advance and a tokens record do not re-append needs_input
 	h.advance(m)
 	if _, err := m.Record(context.Background(), RecordOptions{Kind: RecordTokens, Data: json.RawMessage(`{"input":7,"output":3}`)}); err != nil {
diff --git a/internal/fsm/machine/types.go b/internal/fsm/machine/types.go
index 9e87efe..fd01205 100644
--- a/internal/fsm/machine/types.go
+++ b/internal/fsm/machine/types.go
@@ -28,8 +28,8 @@ type Diff struct {
 	Truncated bool
 }
 
-// MaxDiffBytes bounds the diff handed to kinds.
-const MaxDiffBytes = 1 << 20
+// MaxDiffBytes bounds the diff handed to kinds (a variable so tests can force truncation).
+var MaxDiffBytes = 1 << 20
 
 // ExecInput is everything a fork executor gets.
 type ExecInput struct {
diff --git a/internal/fsm/workflow/resolve.go b/internal/fsm/workflow/resolve.go
index fa25ea7..82b2795 100644
--- a/internal/fsm/workflow/resolve.go
+++ b/internal/fsm/workflow/resolve.go
@@ -175,9 +175,10 @@ func VerifyCmds(allowed []run.AllowedCmd, workDir string, hash func(string) (str
 	return nil
 }
 
-// FileSHA256 hashes a regular file; directories and missing paths error.
+// FileSHA256 hashes a regular file, following symlinks (a symlinked script is
+// pinned by its target's contents); directories and missing paths error.
 func FileSHA256(path string) (string, error) {
-	st, err := os.Lstat(path)
+	st, err := os.Stat(path)
 	if err != nil {
 		return "", err
 	}
diff --git a/internal/fsm/workflow/testdata/cmds-preimage.sha256 b/internal/fsm/workflow/testdata/cmds-preimage.sha256
index 9e5ebc0..529e116 100644
--- a/internal/fsm/workflow/testdata/cmds-preimage.sha256
+++ b/internal/fsm/workflow/testdata/cmds-preimage.sha256
@@ -1 +1 @@
-d5f22e88404dc2366f7168b75ad299af15f45c53068f0abe4fe39c901e6804e3
+76d37fa8f97d1468432efdc97ba42dc770cbbe032bcf72ca31811af60f2b0f53
diff --git a/internal/fsm/workflow/workflow_test.go b/internal/fsm/workflow/workflow_test.go
index 1d7c438..f07cb29 100644
--- a/internal/fsm/workflow/workflow_test.go
+++ b/internal/fsm/workflow/workflow_test.go
@@ -303,6 +303,51 @@ transitions:
   "*→failed": { on: gate_error }
 `
 
+func TestW2ReservedEnvAndBoundaries(t *testing.T) {
+	for _, name := range []string{"PATH", "HOME", "LANG", "TMPDIR", "BASH_ENV", "ENV", "PYTHONPATH", "PYTHONSTARTUP", "PYTHONHOME", "NODE_OPTIONS", "NODE_PATH", "PERL5OPT", "PERL5LIB", "RUBYOPT", "RUBYLIB", "JAVA_TOOL_OPTIONS", "SHELLOPTS", "PS4", "IFS", "CDPATH", "GLOBIGNORE", "PROMPT_COMMAND", "MRV_RUN_ID", "MRV_X", "LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES", "GIT_DIR", "GIT_CONFIG_COUNT"} {
+		if r, _ := reasonOf(t, edit("env: [SLACK_WEBHOOK]", "env: ["+name+"]")); r != "bad_env" {
+			t.Errorf("reserved %s: %s", name, r)
+		}
+	}
+	// acceptance boundaries
+	for _, ok := range []string{edit("timeout: 30", "timeout: 1"), edit("timeout: 30", "timeout: 3600"), renameState(strings.Repeat("s", 32)), edit("env: [SLACK_WEBHOOK]", "env: [A1, A2, A3, A4, A5, A6, A7, A8, A9, A10, A11, A12, A13, A14, A15, A16]")} {
+		w := mustParse(t, ok)
+		_ = w
+	}
+	if w := mustParse(t, edit("timeout: 30", "timeout: 3600")); w.Cmds["notify"].Timeout != 3600*time.Second {
+		t.Fatal("3600 accepted")
+	}
+	if r, _ := reasonOf(t, renameState(strings.Repeat("s", 33))); r != "bad_state" {
+		t.Fatal("33-char state refused")
+	}
+	var vars []string
+	for i := 0; i < run.MaxVars-3; i++ {
+		vars = append(vars, "V"+strings.Repeat("A", i+1)+": {default: x}")
+	}
+	mustParse(t, edit("vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5} }", "vars: { JUDGE: {required: true}, JUDGE_EFFORT: {required: true}, REVIEWER: {default: claude-opus-5}, "+strings.Join(vars, ", ")+" }"))
+	var cmds []string
+	for i := 0; i < run.MaxAllowedCmds-1; i++ {
+		cmds = append(cmds, "  c"+strings.Repeat("x", i)+": { argv: [a] }")
+	}
+	mustParse(t, edit("  notify: { argv: [bash, ./scripts/notify.sh, --model, $JUDGE], timeout: 30, env: [SLACK_WEBHOOK] }", "  notify: { argv: [bash] }\n"+strings.Join(cmds, "\n")))
+	mustParse(t, edit("argv: [bash, ./scripts/notify.sh, --model, $JUDGE]", "argv: ["+strings.Repeat("a, ", run.MaxArgv-1)+"a]"))
+	// caller beats Default; $1 / ${X} left literal; nested maps not walked
+	w := mustParse(t, edit("lenses: 8 }", "lenses: 8, note: '$1 ${JUDGE} $', deep: {model: $JUDGE} }"))
+	r, eff, err := w.Resolve(map[string]string{"JUDGE": "j", "JUDGE_EFFORT": "e", "REVIEWER": "override"}, false)
+	if err != nil || eff["REVIEWER"] != "override" || r.Nodes["discover"].Model != "override" {
+		t.Fatalf("caller beats default: %v %v", err, eff)
+	}
+	if r.Nodes["discover"].Params["note"] != "$1 ${JUDGE} $" || r.Nodes["discover"].Params["deep"].(map[string]any)["model"] != "$JUDGE" {
+		t.Fatalf("literal/nested: %v", r.Nodes["discover"].Params)
+	}
+}
+
+// renameState renames the adjudicate state (not the kind) in the example.
+func renameState(name string) string {
+	s := strings.ReplaceAll(example, "adjudicate", name)
+	return strings.ReplaceAll(s, "match-then-"+name, "match-then-adjudicate")
+}
+
 func TestW2Warnings(t *testing.T) {
 	src := edit("  - { from: discover, to: done, gate: findings_empty, outcome: clean }\n", "")
 	src = strings.Replace(src, "  - { from: adjudicate, to: done, gate: confirmed_empty, outcome: clean }\n", "", 1)
@@ -436,11 +481,22 @@ func TestW4ResolveCmds(t *testing.T) {
 	if string(canon) != strings.TrimSpace(strings.ReplaceAll(string(fixture), "WORK", work)) {
 		t.Fatalf("preimage drift:\n%s\n%s", canon, fixture)
 	}
-	// the sha over the WORK-substituted fixture must equal CmdsSHA256 (the .sha256 file pins the fixture with WORK=/w)
-	if sha != CmdsSHA256(allowed) {
-		t.Fatal("ResolveCmds sha == CmdsSHA256")
+	// the pinned .sha256 (WORK=/w) must equal CmdsSHA256 of the list resolved in /w through the fakes
+	hashesW := map[string]string{"/bin/bash": "hb", "/w/scripts/notify.sh": "hs"}
+	lookW := func(name string) (string, error) {
+		switch name {
+		case "bash":
+			return "/bin/bash", nil
+		case "/w/scripts/notify.sh":
+			return name, nil
+		}
+		return "", errors.New("nf")
+	}
+	allowedW, shaW, err := ResolveCmds(r, "/w", lookW, hashFor2(hashesW))
+	if err != nil || shaW != wantSha || CmdsSHA256(allowedW) != wantSha {
+		t.Fatalf("pinned preimage sha: %s vs %s (%v)", shaW, wantSha, err)
 	}
-	sum := sha256.Sum256([]byte(strings.TrimSpace(string(fixture))))
+	sum := sha256.Sum256([]byte(strings.ReplaceAll(strings.TrimSpace(string(fixture)), "WORK", "/w")))
 	if hex.EncodeToString(sum[:]) != wantSha {
 		t.Fatalf("fixture sha256 %s != pinned %s", hex.EncodeToString(sum[:]), wantSha)
 	}
@@ -489,6 +545,19 @@ func TestW4ResolveCmds(t *testing.T) {
 	if err := VerifyCmds(allowed, work, hash); !errs.Is(err, CodeCmdChanged) || errs.As(err).Field("reason") != "appeared" || errs.As(err).Field("path") != filepath.Join(work, "--model") {
 		t.Fatalf("appeared: %v", err)
 	}
+	// symlinked script: hashed through the link (target contents), keyed by the argv path
+	link := filepath.Join(work, "scripts", "link.sh")
+	if err := os.Symlink(script, link); err != nil {
+		t.Fatal(err)
+	}
+	if h, err := FileSHA256(link); err != nil || h != hexSum("#!/bin/sh\n") {
+		t.Fatalf("symlink hashed through: %s %v", h, err)
+	}
+	dirLink := filepath.Join(work, "dirlink")
+	_ = os.Symlink(work, dirLink)
+	if _, err := FileSHA256(dirLink); !errs.Is(err, CodeCmdChanged) {
+		t.Fatal("symlink to a directory is irregular")
+	}
 	// FileSHA256: regular, directory, missing
 	h, err := FileSHA256(script)
 	if err != nil || h != hexSum("#!/bin/sh\n") {
@@ -511,6 +580,15 @@ func TestW4ResolveCmds(t *testing.T) {
 	}
 }
 
+func hashFor2(m map[string]string) func(string) (string, error) {
+	return func(p string) (string, error) {
+		if h, ok := m[p]; ok {
+			return h, nil
+		}
+		return "", errors.New("no such file")
+	}
+}
+
 func hexSum(s string) string {
 	sum := sha256.Sum256([]byte(s))
 	return hex.EncodeToString(sum[:])


--- docs/tasks/m1-m6-fsm-packages.md
+# M1–M6: internal/fsm core packages
+
+Implement `internal/fsm/{errs,converge,gate,workflow,machine,cmdexec,judge,mockai,kind}` per
+`docs/specs/2026-08-27-metareview-0.9.0-fsm-core.md` (r4) and `docs/specs/2026-08-27-metareview-0.9.0-fsm-judge-kinds.md`
+(r5), test-first, under the combined coverage gate (`tests/coverage.sh`), reviewed per commit range (≤ 120 KB each).
+
+## Acceptance
+
+- Every §7/§8 test row has a discriminating test (literal pins; goldens regression-only behind an env flag).
+- `go test ./internal/fsm/...` passes; every `internal/fsm/*` package at exactly 100% statements.
+- `bash tests/coverage.sh` passes (legacy floor held).
+- Dependency direction per spec 2 §1 (machine imports no kinds/judge/cmdexec/workflows).
+- Every LLM/shell effect behind an interface; no shell, pinned argv, exact env in `cmdexec`.
````

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

coverage gate run after commit 1d6284b (M4/M5/M6 complete):
internal/markdown                                   70.0%  ok
internal/prready                                    85.7%  ok
internal/repo                                       87.9%  ok
internal/reviewers                                  97.2%  ok
internal/reviewlog                                  90.2%  ok
internal/reviewmanifest                             90.5%  ok
internal/reviewstate                                92.1%  ok
internal/runchain                                   90.1%  ok
internal/sessionhistory                             86.2%  ok
internal/setup                                      88.5%  ok
internal/state                                      81.6%  ok
internal/taskdone                                   87.0%  ok
internal/tasksource                                 79.2%  ok
workflows                                          100.0%  ok
coverage gate passed

[exited with code 0]

