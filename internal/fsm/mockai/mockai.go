// Package mockai loads scenario files that script the judge and the
// sanctioned-command runner for tests and rehearsals. A scenario executes
// nothing; runs driven by one are stamped Mock in every event.
package mockai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/dsifry/metareview/internal/fsm/cmdexec"
	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/judge"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// Error codes.
const (
	CodeMockInvalid    = "ERR_MOCK_INVALID"
	CodeMockUnscripted = judge.CodeMockUnscripted
	FileName           = "judge.yaml"
)

type file struct {
	Calls []callRow `yaml:"calls"`
	Cmds  []cmdRow  `yaml:"cmds"`
}

type callRow struct {
	Kind            string `yaml:"kind"`
	Node            string `yaml:"node"`
	Iter            int    `yaml:"iter"`
	Index           int    `yaml:"index"`
	Raw             string `yaml:"raw"`
	Tokens          tokRow `yaml:"tokens"`
	ExpectModel     string `yaml:"expect_model"`
	ExpectInputHash string `yaml:"expect_input_hash"`
	Error           string `yaml:"error"`
}

type tokRow struct {
	Input       int64 `yaml:"input"`
	CacheRead   int64 `yaml:"cache_read"`
	CacheCreate int64 `yaml:"cache_create"`
	Output      int64 `yaml:"output"`
	Reasoning   int64 `yaml:"reasoning"`
}

type cmdRow struct {
	Name   string `yaml:"name"`
	Call   int    `yaml:"call"`
	Stdout string `yaml:"stdout"`
	Stderr string `yaml:"stderr"`
	Exit   int    `yaml:"exit"`
	Repeat bool   `yaml:"repeat"`
}

// Scenario is a loaded scenario directory.
type Scenario struct {
	hash   string
	script judge.Script
	cmds   []cmdRow
}

// Load reads <dir>/judge.yaml strictly.
func Load(dir string) (*Scenario, error) {
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return nil, errs.E(CodeMockInvalid, err.Error(), "dir", dir)
	}
	var f file
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, errs.E(CodeMockInvalid, err.Error(), "dir", dir)
	}
	s := &Scenario{script: judge.Script{Calls: map[judge.ScriptKey]judge.ScriptRow{}}}
	for i, c := range f.Calls {
		key := judge.ScriptKey{Kind: c.Kind, Node: c.Node, Iter: c.Iter, Index: c.Index}
		if _, dup := s.script.Calls[key]; dup {
			return nil, errs.E(CodeMockInvalid, fmt.Sprintf("calls[%d] duplicates (%s,%s,%d,%d)", i, c.Kind, c.Node, c.Iter, c.Index), "dir", dir)
		}
		tok := run.TokenTotals{Input: c.Tokens.Input, CacheRead: c.Tokens.CacheRead, CacheCreate: c.Tokens.CacheCreate, Output: c.Tokens.Output, Reasoning: c.Tokens.Reasoning}
		if tok.Negative() || tok.TooLarge() {
			return nil, errs.E(CodeMockInvalid, fmt.Sprintf("calls[%d] has invalid tokens", i), "dir", dir)
		}
		s.script.Calls[key] = judge.ScriptRow{Raw: c.Raw, Tokens: tok, ExpectModel: c.ExpectModel, ExpectInputHash: c.ExpectInputHash, Error: c.Error}
	}
	seen := map[string]bool{}
	for i, c := range f.Cmds {
		k := fmt.Sprintf("%s#%d", c.Name, c.Call)
		if c.Name == "" || c.Call < 0 || seen[k] {
			return nil, errs.E(CodeMockInvalid, fmt.Sprintf("cmds[%d] is unnamed, negative, or duplicates %s", i, k), "dir", dir)
		}
		seen[k] = true
	}
	s.cmds = f.Cmds
	sum := sha256.Sum256(raw)
	s.hash = hex.EncodeToString(sum[:])
	return s, nil
}

// Hash is sha256 of the judge.yaml bytes (the run's Mock identity).
func (s *Scenario) Hash() string { return s.hash }

// Script is the judge script.
func (s *Scenario) Script() judge.Script { return s.script }

// Runner returns the fake command runner: rows match by name and the
// durable ordinal (first `call == Ordinal`, else the first `repeat` row with
// `call <= Ordinal`).
func (s *Scenario) Runner() cmdexec.Runner { return fakeRunner{rows: s.cmds} }

type fakeRunner struct{ rows []cmdRow }

func (f fakeRunner) Run(_ context.Context, sp cmdexec.Spec) (cmdexec.Result, error) {
	var repeat *cmdRow
	for i := range f.rows {
		r := &f.rows[i]
		if r.Name != sp.Name {
			continue
		}
		if r.Call == sp.Ordinal {
			return result(r), nil
		}
		if r.Repeat && r.Call <= sp.Ordinal && repeat == nil {
			repeat = r
		}
	}
	if repeat != nil {
		return result(repeat), nil
	}
	return cmdexec.Result{ExitCode: -1}, errs.E(CodeMockUnscripted, fmt.Sprintf("no scripted cmd row for %s at ordinal %d", sp.Name, sp.Ordinal), "name", sp.Name, "ordinal", fmt.Sprint(sp.Ordinal))
}

func result(r *cmdRow) cmdexec.Result {
	return cmdexec.Result{Stdout: []byte(r.Stdout), Stderr: []byte(r.Stderr), ExitCode: r.Exit, Duration: time.Millisecond}
}

// LoadHash is the machine.Deps.MockLoad adapter.
func LoadHash(dir string) (string, error) {
	s, err := Load(dir)
	if err != nil {
		return "", err
	}
	return s.Hash(), nil
}
