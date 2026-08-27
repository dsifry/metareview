# metareview task-done context

Run ID: `mrv-20260827-113912937298000-task-done-m8-cli-suite-docs-46a961bc`

## Task

# M8 — CLI, judge/mockai/converge handoffs, black-box suite, docs

Implements spec 4 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-cli.md`) plus the spec 2/5 handoffs:
`judge.Preflight`, `mockai.MaxFileBytes`, `converge.Describe`; `machine` `OpenOptions`, `Deps.Preflight`, `NodeView`,
`View.Outgoing`, `RecordLLMCall`, `Init` workflow-source stamps; `internal/fsm/cli` (`Deps` seams, `RealDeps`, `Run`,
envelopes, `exitFor`, `StatusLines`, `AgentPrompt`) wired into `cmd/metareview` (`fsm` branch, status section);
`tests/go/test-fsm.sh` over the mock scenarios under `testdata/fsm/scenarios`; `/fsm` skill, `commands/fsm.md`,
`docs/fsm/`, README/INSTALL/quickstart/AGENTS/CLAUDE/CHANGELOG/manifest amendments.

Done when every `internal/fsm/*` package and `workflows/` is at exactly 100% statement coverage and the legacy
packages hold their recorded floor (`tests/coverage.sh`), `tests/run-all.sh` is green, and `go vet` is clean.


## Git

- Base: `cc1f6e2b86979fdc7e2d062b7f90d8e99d3e868e`
- Head: `9d910972f201b95f5ac322541f832885223d148a`
- Branch: ``
- Gate effect: `gate`

## Context Profile

- Raw diff bytes: `11539`
- Filtered diff bytes: `11539`
- Risk level: `none`



## Review Manifest

- Manifest verdict: `NEEDS_REVISION`
- Source manifest hash: `89fd1e16bf57063a`
- Runtime assessment: static-only; runtime not assessed

### Source Paths
- docs/tasks/m8-cli-suite-docs.md
- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/judge/judge.go
- internal/fsm/judge/judge_test.go
- internal/fsm/mockai/mockai.go
- internal/fsm/mockai/mockai_test.go

### Shards
- shard-01: docs/tasks/m8-cli-suite-docs.md, internal/fsm/converge/converge.go, internal/fsm/converge/converge_test.go, internal/fsm/judge/judge.go, internal/fsm/judge/judge_test.go, internal/fsm/mockai/mockai.go, internal/fsm/mockai/mockai_test.go

### Manifest Blockers
- missing shard result for shard-01

## Changed Files

- internal/fsm/converge/converge.go
- internal/fsm/converge/converge_test.go
- internal/fsm/judge/judge.go
- internal/fsm/judge/judge_test.go
- internal/fsm/mockai/mockai.go
- internal/fsm/mockai/mockai_test.go
- docs/tasks/m8-cli-suite-docs.md

## Diff

```diff
diff --git a/internal/fsm/converge/converge.go b/internal/fsm/converge/converge.go
index 1e34295..f28b373 100644
--- a/internal/fsm/converge/converge.go
+++ b/internal/fsm/converge/converge.go
@@ -350,3 +350,50 @@ func (n *not) Evaluate(ctx context.Context, s run.Snapshot) (Result, error) {
 	}
 	return out, nil
 }
+
+// Stats describes a validated convergence tree (spec 5 §2: `fsm converge --check`).
+type Stats struct {
+	Atoms int
+	Depth int
+	Cmds  []string
+}
+
+// Describe validates node like Validate and reports its atom count, nesting depth, and the cmd names it references.
+func Describe(node *yaml.Node, cmdNames []string) (Stats, error) {
+	if err := Validate(node, cmdNames); err != nil {
+		return Stats{}, err
+	}
+	st := Stats{Cmds: []string{}}
+	describe(node, 0, &st)
+	return st, nil
+}
+
+// describe walks a tree Validate already accepted: a scalar is an atom, a `cmd` mapping is a cmd atom, the
+// `max_iterations`/`budget` mappings are atoms, `any`/`all`/`not` recurse.
+func describe(node *yaml.Node, depth int, st *Stats) {
+	if node.Kind == yaml.DocumentNode {
+		describe(node.Content[0], depth, st)
+		return
+	}
+	if depth > st.Depth {
+		st.Depth = depth
+	}
+	if node.Kind == yaml.ScalarNode {
+		st.Atoms++
+		return
+	}
+	key, val := node.Content[0].Value, node.Content[1]
+	switch key {
+	case "any", "all":
+		for _, c := range val.Content {
+			describe(c, depth+1, st)
+		}
+	case "not":
+		describe(val, depth+1, st)
+	case "cmd":
+		st.Atoms++
+		st.Cmds = append(st.Cmds, val.Value)
+	default: // max_iterations, budget
+		st.Atoms++
+	}
+}
diff --git a/internal/fsm/converge/converge_test.go b/internal/fsm/converge/converge_test.go
index 58fbf6d..e084546 100644
--- a/internal/fsm/converge/converge_test.go
+++ b/internal/fsm/converge/converge_test.go
@@ -299,3 +299,19 @@ func TestC4ValidateAndParseErrors(t *testing.T) {
 		t.Fatal("non-string cmd via Parse")
 	}
 }
+
+func TestDescribe(t *testing.T) {
+	st, err := Describe(node(t, "any: [no_fixation_progress, {cmd: notify}, {max_iterations: 5}, {budget: {tokens: 4000000}}, {all: [no_fixation_progress, {not: {cmd: chk}}]}]"), []string{"notify", "chk"})
+	if err != nil || st.Atoms != 6 || st.Depth != 3 || len(st.Cmds) != 2 || st.Cmds[0] != "notify" || st.Cmds[1] != "chk" {
+		t.Fatalf("describe: %+v %v", st, err)
+	}
+	if st, err := Describe(node(t, "no_fixation_progress"), nil); err != nil || st.Atoms != 1 || st.Depth != 0 || len(st.Cmds) != 0 {
+		t.Fatalf("scalar: %+v %v", st, err)
+	}
+	if _, err := Describe(node(t, "any: [{cmd: ghost}]"), []string{"notify"}); err == nil {
+		t.Fatal("undeclared cmd must fail")
+	}
+	if _, err := Describe(node(t, "bogus: 1"), nil); err == nil {
+		t.Fatal("invalid tree")
+	}
+}
diff --git a/internal/fsm/judge/judge.go b/internal/fsm/judge/judge.go
index 7b1f6f7..f3ba609 100644
--- a/internal/fsm/judge/judge.go
+++ b/internal/fsm/judge/judge.go
@@ -219,28 +219,55 @@ func maxTokensFor(kind string, calibration bool) int {
 	return MaxTokensStillPresentProduct
 }
 
-// prepare validates the request and builds the wire body.
-func (j *realJudge) prepare(r Request, system, user string) (request, error) {
-	if _, over := run.CapText(r.Model, run.MaxShort); over || r.Model == "" {
-		return request{}, errs.E(CodeJudgeModel, "model id is empty or exceeds MaxShort", "model", r.Model, "reason", "length")
+// validate is the network-free half of prepare: model routing, effort vocabulary, the calibration pin, the key for
+// the routed provider, and the Anthropic family table. Preflight exposes it to the CLI (spec 5 §8).
+func validate(model, effort string, calibration bool, keys Keys) (provider, error) {
+	if _, over := run.CapText(model, run.MaxShort); over || model == "" {
+		return provUnknown, errs.E(CodeJudgeModel, "model id is empty or exceeds MaxShort", "model", model, "reason", "length")
 	}
-	prov := route(r.Model)
+	prov := route(model)
 	if prov == provUnknown {
-		return request{}, errs.E(CodeJudgeModel, "no provider for model "+r.Model, "model", r.Model)
+		return provUnknown, errs.E(CodeJudgeModel, "no provider for model "+model, "model", model)
+	}
+	if !efforts[effort] {
+		return provUnknown, errs.E(CodeJudgeEffortUnsupported, "unknown effort "+effort, "effort", effort)
 	}
-	if !efforts[r.Effort] {
-		return request{}, errs.E(CodeJudgeEffortUnsupported, "unknown effort "+r.Effort, "effort", r.Effort)
+	if calibration && effort != CalibrationEff {
+		return provUnknown, errs.E(CodeJudgeEffortUnsupported, "calibration requires effort medium", "effort", effort, "reason", "calibration")
+	}
+	switch prov {
+	case provAnthropic:
+		if keys.Anthropic == "" {
+			return provUnknown, errs.E(CodeJudgeKey, "ANTHROPIC_API_KEY is unset", "provider", "anthropic")
+		}
+		if capable, legacy := anthropicFamily(wireModel(model)); !calibration && !capable && !legacy {
+			return provUnknown, errs.E(CodeJudgeModel, "unknown Anthropic model family "+wireModel(model), "model", model, "reason", "unknown_family")
+		}
+	default:
+		if keys.OpenAI == "" {
+			return provUnknown, errs.E(CodeJudgeKey, "OPENAI_API_KEY is unset", "provider", "openai")
+		}
 	}
-	if r.Calibration && r.Effort != CalibrationEff {
-		return request{}, errs.E(CodeJudgeEffortUnsupported, "calibration requires effort medium", "effort", r.Effort, "reason", "calibration")
+	return prov, nil
+}
+
+// Preflight reports the error a Call with these parameters would raise before any network traffic
+// (ERR_JUDGE_MODEL, ERR_JUDGE_EFFORT_UNSUPPORTED, ERR_JUDGE_KEY), or nil.
+func Preflight(model, effort string, calibration bool, keys Keys) error {
+	_, err := validate(model, effort, calibration, keys)
+	return err
+}
+
+// prepare validates the request and builds the wire body.
+func (j *realJudge) prepare(r Request, system, user string) (request, error) {
+	prov, err := validate(r.Model, r.Effort, r.Calibration, j.keys)
+	if err != nil {
+		return request{}, err
 	}
 	maxTok := maxTokensFor(r.Kind, r.Calibration)
 	model := wireModel(r.Model)
 	switch prov {
 	case provAnthropic:
-		if j.keys.Anthropic == "" {
-			return request{}, errs.E(CodeJudgeKey, "ANTHROPIC_API_KEY is unset", "provider", "anthropic")
-		}
 		body := map[string]any{"model": model, "system": system, "messages": []map[string]string{{"role": "user", "content": user}}, "max_tokens": maxTok}
 		lower := strings.ToLower(model)
 		if strings.Contains(lower, "opus-4-5") || strings.Contains(lower, "sonnet-4-5") {
@@ -266,14 +293,9 @@ func (j *realJudge) prepare(r Request, system, user string) (request, error) {
 			default:
 				body["thinking"] = map[string]any{"type": "disabled"}
 			}
-		default:
-			return request{}, errs.E(CodeJudgeModel, "unknown Anthropic model family "+model, "model", r.Model, "reason", "unknown_family")
 		}
 		return request{prov: prov, url: j.urls.Anthropic + "/v1/messages", headers: map[string]string{"x-api-key": j.keys.Anthropic, "anthropic-version": "2023-06-01", "content-type": "application/json"}, body: run.MarshalCanonical(body), maxTokens: maxTok}, nil
 	default:
-		if j.keys.OpenAI == "" {
-			return request{}, errs.E(CodeJudgeKey, "OPENAI_API_KEY is unset", "provider", "openai")
-		}
 		lower := strings.ToLower(model)
 		effort := r.Effort
 		switch {
diff --git a/internal/fsm/judge/judge_test.go b/internal/fsm/judge/judge_test.go
index 24aeeb0..b16a4ed 100644
--- a/internal/fsm/judge/judge_test.go
+++ b/internal/fsm/judge/judge_test.go
@@ -746,3 +746,36 @@ func TestJ9HashesAndCut(t *testing.T) {
 		}
 	}
 }
+
+func TestPreflight(t *testing.T) {
+	both := Keys{Anthropic: "a", OpenAI: "o"}
+	cases := []struct {
+		name, model, effort string
+		cal                 bool
+		keys                Keys
+		code, reason        string
+	}{
+		{"empty model", "", "low", false, both, CodeJudgeModel, "length"},
+		{"unknown provider", "mistral-large", "low", false, both, CodeJudgeModel, ""},
+		{"bad effort", "gpt-5.2", "turbo", false, both, CodeJudgeEffortUnsupported, ""},
+		{"calibration pins medium", "gpt-5.2", "low", true, both, CodeJudgeEffortUnsupported, "calibration"},
+		{"anthropic key", "claude-opus-5", "low", false, Keys{OpenAI: "o"}, CodeJudgeKey, ""},
+		{"openai key", "gpt-5.2", "low", false, Keys{Anthropic: "a"}, CodeJudgeKey, ""},
+		{"unknown family", "claude-zzz-9", "low", false, both, CodeJudgeModel, "unknown_family"},
+		{"unknown family under calibration is accepted", "claude-zzz-9", "medium", true, both, "", ""},
+		{"ok anthropic", "claude-opus-5", "high", false, both, "", ""},
+		{"ok openai", "kimi-k2", "xhigh", false, both, "", ""},
+	}
+	for _, c := range cases {
+		err := Preflight(c.model, c.effort, c.cal, c.keys)
+		if c.code == "" {
+			if err != nil {
+				t.Fatalf("%s: %v", c.name, err)
+			}
+			continue
+		}
+		if !errs.Is(err, c.code) || (c.reason != "" && errs.As(err).Fields["reason"] != c.reason) {
+			t.Fatalf("%s: %v", c.name, err)
+		}
+	}
+}
diff --git a/internal/fsm/mockai/mockai.go b/internal/fsm/mockai/mockai.go
index 76ec7c9..437392d 100644
--- a/internal/fsm/mockai/mockai.go
+++ b/internal/fsm/mockai/mockai.go
@@ -69,12 +69,18 @@ type Scenario struct {
 	cmds   []cmdRow
 }
 
+// MaxFileBytes caps judge.yaml (spec 5 §8): the peek reads it before the run is verified.
+const MaxFileBytes = 512 << 10
+
 // Load reads <dir>/judge.yaml strictly.
 func Load(dir string) (*Scenario, error) {
 	raw, err := os.ReadFile(filepath.Join(dir, FileName))
 	if err != nil {
 		return nil, errs.E(CodeMockInvalid, err.Error(), "dir", dir)
 	}
+	if len(raw) > MaxFileBytes {
+		return nil, errs.E(CodeMockInvalid, "judge.yaml exceeds 512 KB", "dir", dir, "reason", "too_large")
+	}
 	var f file
 	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
 	dec.KnownFields(true)
diff --git a/internal/fsm/mockai/mockai_test.go b/internal/fsm/mockai/mockai_test.go
index 3b8f07f..588787d 100644
--- a/internal/fsm/mockai/mockai_test.go
+++ b/internal/fsm/mockai/mockai_test.go
@@ -1,6 +1,7 @@
 package mockai
 
 import (
+	"bytes"
 	"context"
 	"crypto/sha256"
 	"encoding/hex"
@@ -111,3 +112,14 @@ func TestS3Runner(t *testing.T) {
 		t.Fatal("keyed by Spec.Name, never argv")
 	}
 }
+
+func TestLoadTooLarge(t *testing.T) {
+	dir := t.TempDir()
+	big := append([]byte("judge:\n  - {kind: adjudicate, node: a, iter: 0, index: 0, verdict: {is_real: true}}\n# "), bytes.Repeat([]byte("x"), MaxFileBytes)...)
+	if err := os.WriteFile(filepath.Join(dir, FileName), big, 0o600); err != nil {
+		t.Fatal(err)
+	}
+	if _, err := Load(dir); !errs.Is(err, CodeMockInvalid) || errs.As(err).Fields["reason"] != "too_large" {
+		t.Fatalf("too large: %v", err)
+	}
+}


--- docs/tasks/m8-cli-suite-docs.md
+# M8 — CLI, judge/mockai/converge handoffs, black-box suite, docs
+
+Implements spec 4 r5 (`docs/specs/2026-08-27-metareview-0.9.0-fsm-cli.md`) plus the spec 2/5 handoffs:
+`judge.Preflight`, `mockai.MaxFileBytes`, `converge.Describe`; `machine` `OpenOptions`, `Deps.Preflight`, `NodeView`,
+`View.Outgoing`, `RecordLLMCall`, `Init` workflow-source stamps; `internal/fsm/cli` (`Deps` seams, `RealDeps`, `Run`,
+envelopes, `exitFor`, `StatusLines`, `AgentPrompt`) wired into `cmd/metareview` (`fsm` branch, status section);
+`tests/go/test-fsm.sh` over the mock scenarios under `testdata/fsm/scenarios`; `/fsm` skill, `commands/fsm.md`,
+`docs/fsm/`, README/INSTALL/quickstart/AGENTS/CLAUDE/CHANGELOG/manifest amendments.
+
+Done when every `internal/fsm/*` package and `workflows/` is at exactly 100% statement coverage and the legacy
+packages hold their recorded floor (`tests/coverage.sh`), `tests/run-all.sh` is green, and `go vet` is clean.
```

## Knowledge And Registries

Service inventory: none

No service inventory found.

Knowledge facts:

No Beads knowledge facts found.

## Evidence

# unit statement coverage at 22cd870266b3bd18540b8a18a495fbc834542326 (2026-08-27T11:38:49Z)
ok  	github.com/dsifry/metareview/internal/fsm/cli	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/cmdexec	1.133s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/converge	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/errs	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/export	1.189s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/gate	2.080s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/judge	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/kind	1.496s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/machine	2.137s	coverage: 99.9% of statements
ok  	github.com/dsifry/metareview/internal/fsm/mockai	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/record	2.215s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/run	(cached)	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/internal/fsm/workflow	1.757s	coverage: 100.0% of statements
ok  	github.com/dsifry/metareview/workflows	(cached)	coverage: 100.0% of statements
	github.com/dsifry/metareview/cmd/metareview		coverage: 0.0% of statements

go vet: clean

