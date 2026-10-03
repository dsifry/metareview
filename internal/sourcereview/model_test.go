package sourcereview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRunner records calls. It is safe for concurrent calls: n is the call's
// start order, active/maxActive count calls running at the same time, and wait
// (when set) runs inside the call before reply so a test can block on ctx.
type fakeRunner struct {
	mu        sync.Mutex
	calls     []fakeCall
	active    int
	maxActive int
	reply     func(n int, name string, args []string, stdin []byte) ([]byte, error)
	wait      func(ctx context.Context, prompt []byte) error
	onRun     func()
}

type fakeCall struct {
	name  string
	args  []string
	stdin []byte
	// prompt is the prompt the CLI received: stdin, or the --prompt-file bytes.
	prompt []byte
}

func (f *fakeRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	if f.onRun != nil {
		f.onRun()
	}
	prompt := append([]byte(nil), stdin...)
	for i, a := range args {
		if a == "--prompt-file" && i+1 < len(args) {
			prompt, _ = os.ReadFile(args[i+1])
		}
	}
	f.mu.Lock()
	n := len(f.calls)
	f.calls = append(f.calls, fakeCall{name, append([]string(nil), args...), append([]byte(nil), stdin...), prompt})
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	}()
	if f.wait != nil {
		if err := f.wait(ctx, prompt); err != nil {
			return nil, err
		}
	}
	if f.reply == nil {
		return nil, errors.New("no reply")
	}
	return f.reply(n, name, args, stdin)
}

func (f *fakeRunner) callList() []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeCall(nil), f.calls...)
}

var bg = context.Background()

func TestModelID(t *testing.T) {
	cases := map[string]string{"opus": "opus", " astra ": "gpt-6-astra", "GROK": "grok-4.7"}
	for in, want := range cases {
		got, err := ModelID(in)
		if err != nil || got != want {
			t.Fatalf("%q -> %q %v", in, got, err)
		}
	}
	if _, err := ModelID("sonnet"); err == nil {
		t.Fatal("expected unknown model")
	}
}

func TestCallShapes(t *testing.T) {
	prompt := []byte("review this")
	for _, model := range []string{"opus", "astra", "grok"} {
		var schemaFile string
		fr := &fakeRunner{reply: func(_ int, _ string, args []string, _ []byte) ([]byte, error) {
			switch model {
			case "opus":
				return []byte(`{"is_error":false,"result":"{\"findings\":[]}"}`), nil
			case "astra":
				// The schema file holds the shared schema during the call.
				schemaFile = args[len(args)-2]
				if got, err := os.ReadFile(schemaFile); err != nil || string(got) != findingsSchema {
					t.Fatalf("codex schema file %s: %v %q", schemaFile, err, got)
				}
				return []byte("not json\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"first\"}}\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"{\\\"findings\\\":[]}\"}}\n"), nil
			default:
				return []byte(`{"text":"{\"findings\":[]}"}`), nil
			}
		}}
		text, err := Call(bg, fr, model, prompt)
		if err != nil {
			t.Fatal(model, err)
		}
		if model == "astra" {
			if text != `{"findings":[]}` {
				t.Fatalf("astra text %q", text)
			}
		} else if text != `{"findings":[]}` {
			t.Fatalf("%s text %q", model, text)
		}
		c := fr.calls[0]
		switch model {
		case "opus":
			want := []string{"-p", "--model", "opus", "--output-format", "json", "--tools", "", "--system-prompt", sharedSystem, "--json-schema", findingsSchema, "--strict-mcp-config", "--no-session-persistence", "--effort", "medium"}
			if c.name != "claude" || !reflectArgs(c.args, want) || !bytes.Equal(c.stdin, prompt) {
				t.Fatalf("claude call %+v", c)
			}
		case "astra":
			want := []string{"exec", "--json", "-m", "gpt-6-astra", "-s", "read-only", "--ephemeral", "--skip-git-repo-check", "--output-schema", schemaFile, "-"}
			if c.name != "codex" || !reflectArgs(c.args, want) || !bytes.Equal(c.stdin, prompt) {
				t.Fatalf("codex call %+v", c)
			}
			if filepath.Dir(schemaFile) != filepath.Clean(os.TempDir()) {
				t.Fatalf("schema file %s not in os.TempDir()", schemaFile)
			}
			if _, err := os.Stat(schemaFile); !os.IsNotExist(err) {
				t.Fatalf("schema file %s left behind", schemaFile)
			}
		case "grok":
			if c.name != "grok" || len(c.args) < 2 || !reflectArgs(c.args, grokWant(c.args[1])) || c.stdin != nil || !bytes.Equal(c.prompt, prompt) {
				t.Fatalf("grok call %+v", c)
			}
		}
	}
}

// sharedSystem is the system prompt every model gets, spelled out so a change
// to reviewSystem is a visible test change.
const sharedSystem = "You are a code reviewer. You have no tools. Everything you need is in the user message. Answer in a single message."

// grokWant is the exact Grok argv the goal names, for a given prompt file.
func grokWant(promptFile string) []string {
	return []string{
		"--prompt-file", promptFile, "--verbatim",
		"-m", "grok-4.7", "--output-format", "json",
		"--json-schema", findingsSchema,
		"--tools", "read_file", "--disallowed-tools", "read_file,search_tool,use_tool",
		"--max-turns", "1", "--no-subagents", "--disable-web-search",
		"--system-prompt-override", sharedSystem,
		"--reasoning-effort", "medium",
	}
}

func TestGrokPromptFile(t *testing.T) {
	prompt := []byte("review this ✓\n")
	tree, out := t.TempDir(), t.TempDir()
	replies := map[string]func() ([]byte, error){
		"success":      func() ([]byte, error) { return []byte(`{"text":"{\"findings\":[]}"}`), nil },
		"nonzero":      func() ([]byte, error) { return nil, errors.New("grok: exit status 1") },
		"cannot-start": func() ([]byte, error) { return nil, errors.New(`exec: "grok": executable file not found in $PATH`) },
	}
	for name, reply := range replies {
		var path string
		fr := &fakeRunner{reply: func(_ int, _ string, args []string, _ []byte) ([]byte, error) {
			path = args[1]
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, prompt) {
				t.Fatalf("%s: prompt file during the call: %v %q", name, err, got)
			}
			for _, dir := range []string{tree, out} {
				if strings.HasPrefix(path, dir) {
					t.Fatalf("%s: prompt file %s inside %s", name, path, dir)
				}
			}
			if filepath.Dir(path) != filepath.Clean(os.TempDir()) {
				t.Fatalf("%s: prompt file %s not in os.TempDir()", name, path)
			}
			return reply()
		}}
		_, err := Call(bg, fr, "grok", prompt)
		if (name == "success") != (err == nil) {
			t.Fatalf("%s: err %v", name, err)
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("%s: prompt file %s left behind", name, path)
		}
	}
	// A real process that cannot start also removes the file.
	before, _ := filepath.Glob(filepath.Join(os.TempDir(), "metareview-grok-prompt-*"))
	t.Setenv("PATH", t.TempDir())
	if _, err := Call(bg, OSRunner{}, "grok", prompt); err == nil {
		t.Fatal("grok should not start with an empty PATH")
	}
	after, _ := filepath.Glob(filepath.Join(os.TempDir(), "metareview-grok-prompt-*"))
	if len(after) > len(before) {
		t.Fatalf("prompt file left behind: %v", after)
	}
}

func TestFindingsSchema(t *testing.T) {
	var s struct {
		Type                 string `json:"type"`
		Required             []string
		AdditionalProperties *bool `json:"additionalProperties"`
		Properties           struct {
			Findings struct {
				Type  string `json:"type"`
				Items struct {
					Required             []string                   `json:"required"`
					AdditionalProperties *bool                      `json:"additionalProperties"`
					Properties           map[string]json.RawMessage `json:"properties"`
				} `json:"items"`
			} `json:"findings"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(findingsSchema), &s); err != nil {
		t.Fatal(err)
	}
	// Codex's strict mode rejects any object that allows extra properties.
	if s.AdditionalProperties == nil || *s.AdditionalProperties || s.Properties.Findings.Items.AdditionalProperties == nil || *s.Properties.Findings.Items.AdditionalProperties {
		t.Fatal("every object must set additionalProperties to false")
	}
	want := []string{"tag", "file", "start_line", "end_line", "issue", "consequence", "confidence", "severity"}
	if !reflectArgs(s.Properties.Findings.Items.Required, want) || s.Properties.Findings.Type != "array" {
		t.Fatalf("schema required %v", s.Properties.Findings.Items.Required)
	}
	for _, f := range want {
		if _, ok := s.Properties.Findings.Items.Properties[f]; !ok {
			t.Fatalf("schema has no property %s", f)
		}
	}
	for f, typ := range map[string]string{"start_line": "integer", "end_line": "integer", "confidence": "integer"} {
		if !strings.Contains(string(s.Properties.Findings.Items.Properties[f]), `"`+typ+`"`) {
			t.Fatalf("%s is not %s", f, typ)
		}
	}
	for f, enum := range map[string]string{"tag": `["bug","advisory"]`, "severity": `["P0","P1","P2","P3"]`} {
		if !strings.Contains(string(s.Properties.Findings.Items.Properties[f]), enum) {
			t.Fatalf("%s enum", f)
		}
	}
}

func TestWritePromptFileErrors(t *testing.T) {
	prevCreate, prevRemove := createTemp, removeFile
	t.Cleanup(func() { createTemp, removeFile = prevCreate, prevRemove })
	createTemp = func(string, string) (*os.File, error) { return nil, errors.New("create") }
	for _, model := range []string{"grok", "astra"} {
		fr := &fakeRunner{}
		if _, err := Call(bg, fr, model, []byte("p")); err == nil || !strings.Contains(err.Error(), "create") || len(fr.callList()) != 0 {
			t.Fatalf("%s: %v", model, err)
		}
	}
	// A file that is already closed fails Write; the partial file is removed.
	var made string
	createTemp = func(dir, pattern string) (*os.File, error) {
		f, err := prevCreate(dir, pattern)
		if err != nil {
			return nil, err
		}
		made = f.Name()
		_ = f.Close()
		return f, nil
	}
	fr := &fakeRunner{}
	if _, err := Call(bg, fr, "grok", []byte("p")); err == nil {
		t.Fatal("write to a closed file")
	}
	if len(fr.callList()) != 0 {
		t.Fatal("grok ran without a prompt file")
	}
	if _, err := os.Stat(made); !os.IsNotExist(err) {
		t.Fatal("partial prompt file left behind")
	}
}

func reflectArgs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestCallErrors(t *testing.T) {
	fr := &fakeRunner{reply: func(int, string, []string, []byte) ([]byte, error) {
		return nil, errors.New("executable file not found")
	}}
	if _, err := Call(bg, fr, "opus", []byte("p")); err == nil {
		t.Fatal("start failure")
	}
	fr.reply = func(int, string, []string, []byte) ([]byte, error) {
		return []byte(`{"is_error":true,"result":"boom"}`), nil
	}
	if _, err := Call(bg, fr, "opus", []byte("p")); err == nil || !strings.Contains(err.Error(), "is_error") {
		t.Fatal(err)
	}
	fr.reply = func(int, string, []string, []byte) ([]byte, error) {
		return []byte(`{"is_error":false}`), nil
	}
	if _, err := Call(bg, fr, "opus", []byte("p")); err == nil {
		t.Fatal("missing result")
	}
	fr.reply = func(int, string, []string, []byte) ([]byte, error) {
		return []byte(`{"is_error":false,"result":""}`), nil
	}
	if _, err := Call(bg, fr, "opus", []byte("p")); err == nil {
		t.Fatal("empty result")
	}
	fr.reply = func(int, string, []string, []byte) ([]byte, error) {
		return []byte("not-json"), nil
	}
	if _, err := Call(bg, fr, "opus", []byte("p")); err == nil {
		t.Fatal("bad claude json")
	}
	if _, err := Call(bg, fr, "grok", []byte("p")); err == nil {
		t.Fatal("bad grok json")
	}
	fr.reply = func(int, string, []string, []byte) ([]byte, error) {
		return []byte(`{"text":""}`), nil
	}
	if _, err := Call(bg, fr, "grok", []byte("p")); err == nil {
		t.Fatal("empty grok text")
	}
	fr.reply = func(int, string, []string, []byte) ([]byte, error) {
		return []byte(`{}`), nil
	}
	if _, err := Call(bg, fr, "grok", []byte("p")); err == nil {
		t.Fatal("nil grok text")
	}
	fr.reply = func(int, string, []string, []byte) ([]byte, error) {
		return []byte("{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"\"}}\n"), nil
	}
	if _, err := Call(bg, fr, "astra", []byte("p")); err == nil {
		t.Fatal("empty codex text")
	}
	fr.reply = func(int, string, []string, []byte) ([]byte, error) {
		return []byte("nope\n"), nil
	}
	if _, err := Call(bg, fr, "astra", []byte("p")); err == nil {
		t.Fatal("no agent message")
	}
	if _, err := Call(bg, fr, "nope", []byte("p")); err == nil {
		t.Fatal("unknown model")
	}
	fr.reply = func(int, string, []string, []byte) ([]byte, error) {
		return nil, errors.New("exit status 1")
	}
	if _, err := Call(bg, fr, "astra", []byte("p")); err == nil {
		t.Fatal("nonzero")
	}
	if _, err := Call(bg, fr, "grok", []byte("p")); err == nil {
		t.Fatal("grok nonzero")
	}
}

func TestModelTextRejectsIncompleteAnswer(t *testing.T) {
	for _, c := range []struct {
		model string
		raw   string
	}{
		{"opus", `{"stop_reason":"max_tokens","result":"{\"findings\":[]}"}`},
		{"grok", `{"stopReason":"max_tokens","text":"{\"findings\":[]}"}`},
		{"grok", `{"structuredOutputError":"model did not produce structured output","text":"{\"findings\":[]}"}`},
	} {
		t.Run(c.model+c.raw, func(t *testing.T) {
			fr := &fakeRunner{reply: func(int, string, []string, []byte) ([]byte, error) {
				return []byte(c.raw), nil
			}}
			if text, err := Call(bg, fr, c.model, []byte("review source")); err == nil || text != "" {
				t.Fatalf("incomplete response accepted: text=%q err=%v", text, err)
			}
		})
	}
}

func TestOSRunner(t *testing.T) {
	out, err := OSRunner{}.Run(bg, "echo", []string{"hi"}, nil)
	if err != nil || !bytes.Contains(out, []byte("hi")) {
		t.Fatalf("echo: %v %q", err, out)
	}
	out, err = OSRunner{}.Run(bg, "cat", nil, []byte("yo"))
	if err != nil || !bytes.Equal(out, []byte("yo")) {
		t.Fatalf("cat: %v %q", err, out)
	}
	if _, err := (OSRunner{}).Run(bg, "false", nil, nil); err == nil {
		t.Fatal("false should fail")
	}
	if _, err := (OSRunner{}).Run(bg, "sh", []string{"-c", "echo boom >&2; exit 1"}, nil); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatal(err)
	}
	if _, err := (OSRunner{}).Run(bg, "sh", []string{"-c", "echo usage-limit; exit 1"}, nil); err == nil || !strings.Contains(err.Error(), "usage-limit") {
		t.Fatal(err)
	}
	if _, err := (OSRunner{}).Run(bg, "sh", []string{"-c", "echo out-line; echo err-line >&2; exit 1"}, nil); err == nil || !strings.Contains(err.Error(), "err-line") || !strings.Contains(err.Error(), "out-line") {
		t.Fatal(err)
	}
	if _, err := (OSRunner{}).Run(bg, "no-such-binary-xyz", nil, nil); err == nil {
		t.Fatal("missing binary")
	}
	// exec.Command is used; a lookup failure is *exec.Error, not ExitError.
	if _, err := exec.LookPath("echo"); err != nil {
		t.Fatal(err)
	}
}

func TestOSRunnerIsolatesWorkspace(t *testing.T) {
	t.Setenv("GROK_CLAUDE_SKILLS_ENABLED", "true")
	t.Setenv("CLAUDE_CODE_SAFE_MODE", "0")
	t.Setenv("CLAUDE_CODE_EFFORT_LEVEL", "high")
	t.Setenv("GROK_CONFIG", `{"models":{"default":"a-different-model"}}`)
	for _, exit := range []string{"0", "1"} {
		out, err := OSRunner{}.Run(bg, "sh", []string{"-c", `pwd; printf '%s\n' "$GROK_CLAUDE_SKILLS_ENABLED" "$CLAUDE_CODE_SAFE_MODE" "$GROK_CONFIG" "$CLAUDE_CODE_EFFORT_LEVEL"; exit "$1"`, "sh", exit}, nil)
		if (err == nil) != (exit == "0") {
			t.Fatalf("exit %s: %v", exit, err)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) != 5 || !strings.HasPrefix(filepath.Base(lines[0]), "metareview-workspace-") || lines[1] != "false" || lines[2] != "1" || lines[3] != `{"models":{"default":"grok-4.7"}}` || lines[4] != "medium" {
			t.Fatalf("unexpected workspace or environment: %q", out)
		}
		if _, err := os.Stat(lines[0]); !os.IsNotExist(err) {
			t.Fatalf("workspace remains after exit %s: %v", exit, err)
		}
	}
}

func TestOSRunnerWorkspaceFailure(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	if _, err := (OSRunner{}).Run(bg, "echo", []string{"not started"}, nil); err == nil {
		t.Fatal("workspace creation failure must fail the call")
	}
}

func TestOSRunnerKillsProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(bg, 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := OSRunner{}.Run(ctx, "sh", []string{"-c", `sleep 30 & echo $! > "$0"; wait`, pidFile}, nil)
	if err == nil {
		t.Fatal("expected a killed process")
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("kill took %s", took)
	}
	raw, rerr := os.ReadFile(pidFile)
	if rerr != nil {
		t.Fatal(rerr)
	}
	pid, perr := strconv.Atoi(strings.TrimSpace(string(raw)))
	if perr != nil {
		t.Fatal(perr)
	}
	deadline := time.Now().Add(5 * time.Second)
	for exec.Command("kill", "-0", strconv.Itoa(pid)).Run() == nil {
		if time.Now().After(deadline) {
			t.Fatalf("child %d still running", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestClaudeIsErrorWithoutResult(t *testing.T) {
	if _, err := claudeModelText([]byte(`{"is_error":true}`)); err == nil {
		t.Fatal("expected is_error")
	}
}

func TestCodexSkipsBlank(t *testing.T) {
	raw := "\n  \n{\"type\":\"turn.completed\"}\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"{\\\"findings\\\":[]}\"}}\n"
	text, err := codexModelText([]byte(raw))
	if err != nil || text != `{"findings":[]}` {
		t.Fatalf("%q %v", text, err)
	}
	b, err := json.Marshal(map[string]string{"text": "ok"})
	if err != nil || !bytes.Contains(b, []byte("ok")) {
		t.Fatal(err)
	}
}
