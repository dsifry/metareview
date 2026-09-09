package judge

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// ClaudeCLIPrefix marks a model judged through the Claude Code CLI rather than
// over HTTP.
//
// The CLI holds the credential — the user's logged-in Claude session — so this
// provider needs no API key of its own, and metareview never sees a token.
// "claude-cli/" rather than "claude/" because every bare claude id routes to
// the Anthropic HTTP API and must keep doing so.
const ClaudeCLIPrefix = "claude-cli/"

// ClaudeBin is the executable name; the caller resolves it on PATH.
const ClaudeBin = "claude"

// ClaudeExec runs one claude invocation. stdout is the --output-format json
// document, and code is the process exit status. err is non-nil only when the
// process could not be run at all. dir is the working directory the CLI runs
// in (empty inherits the caller's); the signature matches CodexExec so the two
// CLI seams stay interchangeable at the wiring layer.
type ClaudeExec func(ctx context.Context, dir string, args []string, stdin string) (stdout []byte, code int, err error)

// claudeEfforts is the CLI's --effort enum, which is wider than the HTTP
// providers' (max exists here and not there). Taken from `claude --help`.
var claudeEfforts = map[string]bool{
	"low": true, "medium": true, "high": true, "xhigh": true, "max": true,
}

// claudeJudge answers Requests by shelling out to the Claude Code CLI. It
// mirrors codexJudge: the same prompts, the same parsing, the same retry ladder.
type claudeJudge struct {
	exec           ClaudeExec
	nonce          func() string
	clock          Clock
	attemptTimeout time.Duration // zero: the AttemptTimeout default
}

// timeout is the per-attempt timeout: the configured override, or the AttemptTimeout default.
func (j *claudeJudge) timeout() time.Duration {
	if j.attemptTimeout > 0 {
		return j.attemptTimeout
	}
	return AttemptTimeout
}

// Call renders the same prompts the HTTP providers use and parses the same way,
// so a verdict is comparable no matter which provider produced it.
func (j *claudeJudge) Call(ctx context.Context, r Request) (v Verdict, err error) {
	v = Verdict{Kind: r.Kind, Model: r.Model, Effort: r.Effort, InputHash: InputHash(r.Input)}
	if err := validateClaude(r.Model, r.Effort, r.Calibration); err != nil {
		return v, err
	}
	system, user, err := RenderPrompt(r.Kind, r.Input, r.Fence, r.Calibration, j.nonce())
	if err != nil {
		return v, err
	}
	start := j.clock.Now()
	defer func() { v.Duration = j.clock.Now().Sub(start) }()

	args := []string{
		"-p", // print mode: one turn, no interactive UI
		"--model", wireModel(r.Model),
		"--effort", r.Effort,
		"--output-format", "json",
		"--max-turns", "1", // a judge answers; it must never start a tool loop
		// The system prompt must always be passed. Without it `claude -p` can
		// silently fall back to Haiku for the work turn even with --model set —
		// the judge would then be a different model than the one recorded in the
		// verdict (the harnesseval reference implementation's hard-won lesson).
		"--append-system-prompt", system,
		// The user prompt arrives on stdin, never as an argv the process table
		// would show; the system prompt above is a static template, but the
		// user prompt carries the diff under judgment.
	}
	// The same attempt ceiling, per-attempt deadline and backoff as the HTTP and
	// codex arms: explicit beats accidental, whatever retrying the CLI does
	// internally is its own business.
	var lastErr error
	for attempt := 0; attempt < MaxAttempts; attempt++ {
		// Checked before every attempt, including the first (see codexJudge).
		if err := ctx.Err(); err != nil {
			return v, err
		}
		v.Attempts = attempt + 1
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return v, ctx.Err()
			case <-j.clock.After(backoff(classBackoff, attempt-1)):
			}
		}
		actx, cancel := context.WithTimeout(ctx, j.timeout())
		stdout, code, execErr := j.exec(actx, "", args, user)
		cancel()

		text, tokens, found, transient := parseClaudeResult(stdout)
		v.Tokens = v.Tokens.Add(tokens)
		switch {
		case execErr != nil:
			lastErr = errs.E(CodeJudgeTransport, "claude could not be run: "+execErr.Error(), "provider", "claude-cli")
		case code != 0:
			lastErr = errs.E(CodeJudgeTransport, "claude exited "+itoa(code), "provider", "claude-cli", "exit", itoa(code))
		case transient:
			// The silent failure mode: `claude -p` exits 0 and reports a 5xx
			// overload as the result string ("API Error: 529 ..."). Treated as
			// text it would parse as a verdict's raw reply and fail closed for
			// the wrong reason, or worse record a bogus parse-error verdict;
			// treated as what it is — a transient server error — the ladder
			// retries it exactly like a transport failure.
			lastErr = errs.E(CodeJudgeTransport, "claude reported a transient API error in its result", "provider", "claude-cli")
		case !found:
			lastErr = errs.E(CodeJudgeResponse, "claude produced no result", "provider", "claude-cli")
		default:
			v.Raw = text
			v.Parsed, v.Decision, v.Confidence, v.ParseError = Parse(r.Kind, text)
			return v, nil
		}
	}
	return v, lastErr
}

// validateClaude is validate's claude arm: no key, and the CLI's wider effort set.
func validateClaude(model, effort string, calibration bool) error {
	// route strips the prefix with trimPrefixFold, so an empty model has to be
	// detected the same way (see validateCodex for the case-folding trap).
	if _, over := run.CapText(model, run.MaxShort); over || trimPrefixFold(model, ClaudeCLIPrefix) == "" {
		return errs.E(CodeJudgeModel, "model id is empty or exceeds MaxShort", "model", model, "reason", "length")
	}
	if !claudeEfforts[effort] {
		return errs.E(CodeJudgeEffortUnsupported, "unknown effort "+effort, "effort", effort, "provider", "claude-cli")
	}
	if calibration && effort != CalibrationEff {
		return errs.E(CodeJudgeEffortUnsupported, "calibration requires effort medium", "effort", effort, "reason", "calibration")
	}
	return nil
}

// claudeResult is the subset of the --output-format json document this provider
// reads: the result text, the error flag, and the turn's usage.
type claudeResult struct {
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
	Usage   *struct {
		Input      int64 `json:"input_tokens"`
		CacheCre   int64 `json:"cache_creation_input_tokens"`
		CacheRead  int64 `json:"cache_read_input_tokens"`
		Output     int64 `json:"output_tokens"`
		ThinkingOf *struct {
			Thinking int64 `json:"thinking_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

// parseClaudeResult pulls the result text and the turn's token usage out of the
// CLI's JSON document. found is false for a body that is not JSON, declares
// is_error, or carries no result text. transient is true when the result string
// is a server-side "API Error: 5.." report (see Call).
//
// The CLI reports usage in the Anthropic API's convention, live-verified
// 2026-09-09: input_tokens is the UNCACHED prompt (9 for a two-word prompt) with
// the ~15k system-prompt/tool scaffolding tax split across cache_read and
// cache_creation, and output_tokens is the whole completion with thinking a
// subset of it. TokenTotals.Total() sums every field, so the categories are
// made disjoint here exactly as the codex arm does; summing only input+output
// would report the scaffolding tax as zero and undercount ~4000x.
func parseClaudeResult(stdout []byte) (text string, tokens run.TokenTotals, found, transient bool) {
	var r claudeResult
	if json.Unmarshal(stdout, &r) != nil || r.IsError {
		return "", run.TokenTotals{}, false, false
	}
	if r.Usage != nil {
		var thinking int64
		if r.Usage.ThinkingOf != nil {
			thinking = r.Usage.ThinkingOf.Thinking
		}
		tokens = run.TokenTotals{
			Input:       clampTok(r.Usage.Input),
			CacheRead:   clampTok(r.Usage.CacheRead),
			CacheCreate: clampTok(r.Usage.CacheCre),
			Output:      clampTok(r.Usage.Output - thinking),
			Reasoning:   clampTok(thinking),
		}
	}
	text = r.Result
	return text, tokens, text != "", strings.HasPrefix(strings.TrimSpace(text), "API Error: 5")
}
