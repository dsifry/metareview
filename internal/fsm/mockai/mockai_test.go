package mockai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsifry/metareview/internal/fsm/cmdexec"
	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/run"
)

const good = `calls:
  - {kind: adjudicate, node: adjudicate, iter: 0, index: 0, raw: '{"reasoning":"r","is_real":true,"confidence":0.9}', tokens: {input: 10, output: 5, cache_read: 1, cache_create: 2, reasoning: 3}, expect_model: gpt-5.2, expect_input_hash: abc}
  - {kind: match, node: adjudicate, iter: 1, index: 3, error: ERR_JUDGE_HTTP}
cmds:
  - {name: notify, call: 0, stdout: '{"stop": false, "reason": ""}', stderr: "", exit: 0}
  - {name: notify, call: 1, stdout: '{"stop": true, "reason": "plateau"}', exit: 0, repeat: true}
  - {name: other, call: 0, stdout: "x", exit: 3}
`

func write(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestS1Load(t *testing.T) {
	dir := write(t, good)
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(good))
	if s.Hash() != hex.EncodeToString(sum[:]) {
		t.Fatal("hash is over the file bytes")
	}
	h, err := LoadHash(dir)
	if err != nil || h != s.Hash() {
		t.Fatal("LoadHash")
	}
	// a comment edit moves the hash
	dir2 := write(t, good+"# edited\n")
	if h2, _ := LoadHash(dir2); h2 == h {
		t.Fatal("comment edit must change the hash")
	}
	row := s.Script().Calls[judge.ScriptKey{Kind: "adjudicate", Node: "adjudicate", Iter: 0, Index: 0}]
	if row.Raw == "" || row.Tokens != (run.TokenTotals{Input: 10, CacheRead: 1, CacheCreate: 2, Output: 5, Reasoning: 3}) || row.ExpectModel != "gpt-5.2" || row.ExpectInputHash != "abc" {
		t.Fatalf("row: %+v", row)
	}
	if e := s.Script().Calls[judge.ScriptKey{Kind: "match", Node: "adjudicate", Iter: 1, Index: 3}]; e.Error != "ERR_JUDGE_HTTP" {
		t.Fatal("error row")
	}
	bad := map[string]string{
		"unknown-key":   "calls:\n  - {kind: match, node: n, iter: 0, index: 0, raw: x, zzz: 1}\n",
		"dup-call":      "calls:\n  - {kind: match, node: n, iter: 0, index: 0, raw: x}\n  - {kind: match, node: n, iter: 0, index: 0, raw: y}\n",
		"bad-tokens":    "calls:\n  - {kind: match, node: n, iter: 0, index: 0, raw: x, tokens: {input: -1}}\n",
		"malformed":     "calls: [\n",
		"dup-cmd":       "cmds:\n  - {name: a, call: 0}\n  - {name: a, call: 0}\n",
		"unnamed-cmd":   "cmds:\n  - {call: 0}\n",
		"negative-call": "cmds:\n  - {name: a, call: -1}\n",
		"dup-yaml-key":  "calls: []\ncalls: []\n",
	}
	for name, content := range bad {
		if _, err := Load(write(t, content)); !errs.Is(err, CodeMockInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Load(t.TempDir()); !errs.Is(err, CodeMockInvalid) {
		t.Fatal("missing file")
	}
	if _, err := LoadHash(t.TempDir()); !errs.Is(err, CodeMockInvalid) {
		t.Fatal("LoadHash missing")
	}
}

func TestS3Runner(t *testing.T) {
	s, err := Load(write(t, good))
	if err != nil {
		t.Fatal(err)
	}
	r := s.Runner()
	ctx := context.Background()
	res, err := r.Run(ctx, cmdexec.Spec{Name: "notify", Ordinal: 0, Argv: []string{"/anything"}})
	if err != nil || string(res.Stdout) != `{"stop": false, "reason": ""}` || res.ExitCode != 0 {
		t.Fatalf("ordinal 0: %+v %v", res, err)
	}
	res, _ = r.Run(ctx, cmdexec.Spec{Name: "notify", Ordinal: 1})
	if string(res.Stdout) != `{"stop": true, "reason": "plateau"}` {
		t.Fatal("ordinal 1")
	}
	res, _ = r.Run(ctx, cmdexec.Spec{Name: "notify", Ordinal: 7})
	if string(res.Stdout) != `{"stop": true, "reason": "plateau"}` {
		t.Fatal("repeat row covers later ordinals")
	}
	res, err = r.Run(ctx, cmdexec.Spec{Name: "other", Ordinal: 0})
	if err != nil || string(res.Stdout) != "x" || res.ExitCode != 3 || res.Duration == 0 {
		t.Fatalf("other: %+v %v", res, err)
	}
	if _, err := r.Run(ctx, cmdexec.Spec{Name: "other", Ordinal: 1}); !errs.Is(err, CodeMockUnscripted) || errs.As(err).Field("ordinal") != "1" {
		t.Fatalf("no repeat: %v", err)
	}
	if _, err := r.Run(ctx, cmdexec.Spec{Name: "nope", Argv: []string{"/notify"}}); !errs.Is(err, CodeMockUnscripted) {
		t.Fatal("keyed by Spec.Name, never argv")
	}
}
