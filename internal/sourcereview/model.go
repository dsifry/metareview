package sourcereview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Runner runs one logged-in CLI. stdin is the prompt for claude and codex;
// grok reads the prompt from the --prompt-file file and stdin is nil. When ctx
// ends, the process and its children are killed.
type Runner interface {
	Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)
}

// OSRunner is the real process runner. It does not read an API key and does not
// add a token to the command.
type OSRunner struct{}

// waitDelay bounds how long Run waits for output pipes after the process group
// is killed (the internal/fsm/cmdexec pattern).
const waitDelay = 2 * time.Second

const (
	grokModelID  = "grok-4.7"
	reviewEffort = "medium"
)

func (OSRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	// Review only the supplied source. Starting in the host checkout injects
	// unrelated project rules and skills into the model's conversation.
	dir, err := os.MkdirTemp("", "metareview-workspace-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"CLAUDE_CODE_SAFE_MODE=1",
		// Claude gives this environment variable precedence over --effort.
		"CLAUDE_CODE_EFFORT_LEVEL="+reviewEffort,
		// Selecting a different Grok model after initialization re-renders its
		// coding-agent system prompt, discarding --system-prompt-override.
		// Make the requested model the session default before initialization.
		fmt.Sprintf(`GROK_CONFIG={"models":{"default":%q}}`, grokModelID),
		"GROK_CLAUDE_AGENTS_ENABLED=false", "GROK_CLAUDE_RULES_ENABLED=false", "GROK_CLAUDE_SKILLS_ENABLED=false",
		"GROK_CURSOR_AGENTS_ENABLED=false", "GROK_CURSOR_RULES_ENABLED=false", "GROK_CURSOR_SKILLS_ENABLED=false",
	)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		// Codex puts the turn failure on stdout and a snapshot warning on
		// stderr. Keep both so a non-zero exit records the CLI's reason.
		msg := strings.TrimSpace(stderr.String())
		if out := strings.TrimSpace(stdout.String()); out != "" {
			if msg != "" {
				msg = msg + "\n" + out
			} else {
				msg = out
			}
		}
		if msg == "" {
			msg = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("%s: %s", name, msg)
	}
	return stdout.Bytes(), nil
}

// ModelID is the id shown on the page and passed to the CLI.
func ModelID(model string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "opus":
		return "opus", nil
	case "astra":
		return "gpt-6-astra", nil
	case "grok":
		return grokModelID, nil
	default:
		return "", fmt.Errorf("unknown model %q (want astra, opus, or grok)", model)
	}
}

// Every model gets the same review setup: this system prompt, this answer
// schema, no tools, one answer. The prompt text and the parser are shared too,
// so the models are compared on the same job.

// findingsSchema constrains the answer to {"findings":[...]} with all eight
// fields present and no others. Codex's strict schema mode rejects an object
// without "additionalProperties": false.
const findingsSchema = `{"type":"object","properties":{"findings":{"type":"array","items":{"type":"object","properties":{"tag":{"type":"string","enum":["bug","advisory"]},"file":{"type":"string"},"start_line":{"type":"integer"},"end_line":{"type":"integer"},"issue":{"type":"string"},"consequence":{"type":"string"},"confidence":{"type":"integer"},"severity":{"type":"string","enum":["P0","P1","P2","P3"]}},"required":["tag","file","start_line","end_line","issue","consequence","confidence","severity"],"additionalProperties":false}}},"required":["findings"],"additionalProperties":false}`

// reviewSystem replaces each CLI's agent system prompt. Without it Grok tries
// to call tools and --max-turns 1 cancels the turn.
const reviewSystem = "You are a code reviewer. You have no tools. Everything you need is in the user message. Answer in a single message."

// claudeArgs runs Claude as one turn with no tools, no MCP servers, and no
// saved session, at medium effort to match Grok's --reasoning-effort medium.
// The prompt is on stdin.
func claudeArgs() []string {
	return []string{
		"-p", "--model", "opus", "--output-format", "json",
		"--tools", "", "--system-prompt", reviewSystem, "--json-schema", findingsSchema,
		"--strict-mcp-config", "--no-session-persistence",
		"--effort", reviewEffort,
	}
}

// codexArgs runs Codex with the shared schema, a read-only sandbox, and no
// saved session. The prompt is on stdin ("-"). Codex has no switch to remove
// its tools or replace its system prompt, so it is the one model that differs.
func codexArgs(schemaFile string) []string {
	return []string{
		"exec", "--json", "-m", "gpt-6-astra",
		"-s", "read-only", "--ephemeral", "--skip-git-repo-check", "--output-schema", schemaFile, "-",
	}
}

// grokArgs runs Grok as one turn with no tools. -p hands a large prompt to
// Grok's agent as an excerpt plus a file to read back with tools, which made
// one hh prompt take 11 model calls; --prompt-file with --verbatim sends it whole.
// Effort is shared with Claude. Speed and independently verified quality are
// measured together; a lower raw finding count alone does not establish a loss.
// Grok treats --tools "" as the default toolset. Use a non-empty allowlist,
// then deny that tool and the always-on MCP dispatchers. Deny takes precedence,
// leaving no tools without maintaining an exhaustive list of built-in tools.
func grokArgs(promptFile string) []string {
	return []string{
		"--prompt-file", promptFile, "--verbatim",
		"-m", grokModelID, "--output-format", "json",
		"--json-schema", findingsSchema,
		"--tools", "read_file", "--disallowed-tools", "read_file,search_tool,use_tool",
		"--max-turns", "1", "--no-subagents", "--disable-web-search",
		"--system-prompt-override", reviewSystem,
		"--reasoning-effort", reviewEffort,
	}
}

// Temp-file seams. Production uses the os functions. Tests replace them to
// cover the error returns.
var (
	createTemp = os.CreateTemp
	removeFile = os.Remove
)

// writeTempFile writes data to a new file in os.TempDir(), which is never
// inside the reviewed tree or the output directory.
func writeTempFile(pattern string, data []byte) (string, error) {
	f, err := createTemp("", pattern)
	if err != nil {
		return "", err
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = removeFile(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// Call is the one function that runs claude, codex, or grok. The prompt text
// and the parser stay the same; only the CLI and the model id change.
func Call(ctx context.Context, runner Runner, model string, prompt []byte) (string, error) {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "opus":
		out, err := runner.Run(ctx, "claude", claudeArgs(), prompt)
		if err != nil {
			return "", err
		}
		return claudeModelText(out)
	case "astra":
		schema, err := writeTempFile("metareview-schema-*.json", []byte(findingsSchema))
		if err != nil {
			return "", err
		}
		defer func() { _ = removeFile(schema) }()
		out, err := runner.Run(ctx, "codex", codexArgs(schema), prompt)
		if err != nil {
			return "", err
		}
		return codexModelText(out)
	case "grok":
		path, err := writeTempFile("metareview-grok-prompt-*.txt", prompt)
		if err != nil {
			return "", err
		}
		defer func() { _ = removeFile(path) }()
		out, err := runner.Run(ctx, "grok", grokArgs(path), nil)
		if err != nil {
			return "", err
		}
		return grokModelText(out)
	default:
		return "", fmt.Errorf("unknown model %q", model)
	}
}

func claudeModelText(stdout []byte) (string, error) {
	var doc struct {
		IsError          bool            `json:"is_error"`
		StopReason       string          `json:"stop_reason"`
		Result           *string         `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal(stdout, &doc); err != nil {
		return "", fmt.Errorf("missing model text: %w", err)
	}
	if doc.IsError {
		msg := ""
		if doc.Result != nil {
			msg = *doc.Result
		}
		return "", fmt.Errorf("claude: is_error: %s", msg)
	}
	if doc.StopReason == "max_tokens" {
		return "", errors.New("claude: incomplete review: max_tokens")
	}
	if len(doc.StructuredOutput) > 0 && !bytes.Equal(doc.StructuredOutput, []byte("null")) {
		if doc.StructuredOutput[0] != '{' {
			return "", errors.New("claude: structured_output must be an object")
		}
		return string(doc.StructuredOutput), nil
	}
	if doc.Result == nil || *doc.Result == "" {
		return "", errors.New("missing model text")
	}
	return *doc.Result, nil
}

func codexModelText(stdout []byte) (string, error) {
	var text string
	found := false
	for _, line := range strings.Split(string(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev struct {
			Type string `json:"type"`
			Item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev.Type == "item.completed" && ev.Item.Type == "agent_message" {
			text = ev.Item.Text
			found = true
		}
	}
	if !found || text == "" {
		return "", errors.New("missing model text")
	}
	return text, nil
}

func grokModelText(stdout []byte) (string, error) {
	var doc struct {
		Text                  *string `json:"text"`
		StopReason            string  `json:"stopReason"`
		StructuredOutputError string  `json:"structuredOutputError"`
	}
	if err := json.Unmarshal(stdout, &doc); err != nil {
		return "", fmt.Errorf("missing model text: %w", err)
	}
	if doc.StopReason == "max_tokens" {
		return "", errors.New("grok: incomplete review: max_tokens")
	}
	if doc.StructuredOutputError != "" {
		return "", fmt.Errorf("grok: %s", doc.StructuredOutputError)
	}
	if doc.Text == nil || *doc.Text == "" {
		return "", errors.New("missing model text")
	}
	return *doc.Text, nil
}
