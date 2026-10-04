package judge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// GrokPrefix marks a model judged through the Grok CLI rather than over HTTP.
//
// The CLI holds the credential — the user's logged-in Grok session — so this
// provider needs no API key of its own, and metareview never sees a token.
const GrokPrefix = "grok/"

// GrokBin is the executable name; the caller resolves it on PATH.
const GrokBin = "grok"

// GrokExec runs one grok invocation. stdout is the --output-format json
// document, and code is the process exit status. err is non-nil only when the
// process could not be run at all. dir is the working directory the CLI runs
// in (empty inherits the caller's); the signature matches CodexExec and
// ClaudeExec so the CLI seams stay interchangeable at the wiring layer.
//
// Grok takes its prompt from --prompt-file, not stdin, so stdin is unused; the
// parameter is kept only so the three seams share one shape.
type GrokExec func(ctx context.Context, dir string, args []string, stdin string) (stdout []byte, code int, err error)

// grokEfforts is the CLI's --reasoning-effort enum, narrowed to the judge's own
// effort vocabulary. Grok's canonical levels are none, minimal, low, medium,
// high, xhigh, max (each a distinct tier), and a model accepts only the levels
// its own menu advertises — which Preflight cannot see without a live CLI. The
// judge only ever asks for low..xhigh, and each of those is a canonical tier, so
// this is the union we trust; a per-model menu is a live-check question.
var grokEfforts = map[string]bool{"low": true, "medium": true, "high": true, "xhigh": true}

// grokVerdictField maps a judge kind to the boolean its verdict carries. The
// schema constrains grok to that one field, plus reasoning and confidence.
var grokVerdictField = map[string]string{
	KindMatch:        "match",
	KindAdjudicate:   "is_real",
	KindStillPresent: "still_present",
	KindSymptom:      "matches",
}

// grokSchema is the --json-schema document for one kind, so grok validates its
// answer before we parse it. Parse remains the source of truth; the schema only
// makes a malformed answer less likely. additionalProperties:false because a
// strict structured-output mode rejects an open object, and every property is
// required because the prompt asks for all three.
func grokSchema(kind string) string {
	field := grokVerdictField[kind]
	if field == "" {
		// Parse's default kind. Only reachable if a caller reaches the schema
		// without RenderPrompt, which rejects an unknown kind first.
		field = "still_present"
	}
	return fmt.Sprintf(`{"type":"object","properties":{"reasoning":{"type":"string"},"%s":{"type":"boolean"},"confidence":{"type":"number"}},"required":["reasoning","%s","confidence"],"additionalProperties":false}`, field, field)
}

// grokJudge answers Requests by shelling out to the Grok CLI. It mirrors
// codexJudge: the same prompts, the same parsing, the same retry ladder.
type grokJudge struct {
	exec           GrokExec
	nonce          func() string
	clock          Clock
	attemptTimeout time.Duration // zero: the AttemptTimeout default
}

// timeout is the per-attempt timeout: the configured override, or the AttemptTimeout default.
func (j *grokJudge) timeout() time.Duration {
	if j.attemptTimeout > 0 {
		return j.attemptTimeout
	}
	return AttemptTimeout
}

// Call renders the same prompts the HTTP providers use and parses the same way,
// so a verdict is comparable no matter which provider produced it.
func (j *grokJudge) Call(ctx context.Context, r Request) (v Verdict, err error) {
	v = Verdict{Kind: r.Kind, Model: r.Model, Effort: r.Effort, InputHash: InputHash(r.Input)}
	if err := validateGrok(r.Model, r.Effort, r.Calibration); err != nil {
		return v, err
	}
	system, user, err := RenderPrompt(r.Kind, r.Input, r.Fence, r.Calibration, j.nonce())
	if err != nil {
		return v, err
	}
	start := j.clock.Now()
	defer func() { v.Duration = j.clock.Now().Sub(start) }()

	// The same attempt ceiling, per-attempt deadline and backoff as the HTTP,
	// codex and claude arms.
	var lastErr error
	for attempt := 0; attempt < MaxAttempts; attempt++ {
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
		dir, cleanup, dirErr := isolatedDir()
		if dirErr != nil {
			lastErr = errs.E(CodeJudgeTransport, "grok could not be given an isolated working directory: "+dirErr.Error(), "provider", "grok")
			continue
		}
		promptPath, perr := writeGrokPromptFile(dir, []byte(user))
		if perr != nil {
			cleanup()
			lastErr = errs.E(CodeJudgeTransport, "grok prompt file: "+perr.Error(), "provider", "grok")
			continue
		}
		// --prompt-file with --verbatim sends the prompt whole, from a file, so it
		// never appears in argv; `grok -p` hands a large prompt to Grok's agent as
		// an excerpt it re-reads with tools (#165, G2), so -p is never used here.
		args := []string{
			"--prompt-file", promptPath,
			"--verbatim",
			"-m", wireModel(r.Model),
			"--output-format", "json",
			"--json-schema", grokSchema(r.Kind),
			// A headless grok runs with full tool access by default. A non-empty
			// allowlist (read_file) drops every other built-in, then the denylist
			// removes that one and the always-on MCP dispatchers; when both flags
			// are present the denylist wins, so the net toolset is empty without
			// enumerating every built-in tool by name (#165).
			"--tools", "read_file",
			"--disallowed-tools", "read_file,search_tool,use_tool",
			"--max-turns", "1", // a judge answers; it must never start a tool loop
			"--no-subagents",
			"--disable-web-search",
			"--reasoning-effort", r.Effort,
			"--system-prompt-override", system,
		}
		stdout, code, execErr := func() ([]byte, int, error) {
			defer cleanup() // deferred: a panic in the seam must not leave the directory behind
			defer func() { _ = grokRemoveFile(promptPath) }()
			actx, cancel := context.WithTimeout(ctx, j.timeout())
			defer cancel()
			return j.exec(actx, dir, args, "")
		}()

		text, tokens, found, stopReason, structuredErr := parseGrokResult(stdout)
		v.Tokens = v.Tokens.Add(tokens)
		switch {
		case execErr != nil:
			lastErr = errs.E(CodeJudgeTransport, "grok could not be run: "+execErr.Error(), "provider", "grok")
		case code != 0:
			lastErr = errs.E(CodeJudgeTransport, "grok exited "+itoa(code), "provider", "grok", "exit", itoa(code))
		case stopReason == "max_tokens":
			lastErr = errs.E(CodeJudgeResponse, "grok returned an incomplete review: max_tokens", "provider", "grok", "reason", "max_tokens")
		case stopReason == "refusal":
			lastErr = errs.E(CodeJudgeResponse, "grok refused to answer", "provider", "grok", "reason", "refusal")
		case stopReason == "max_turn_requests":
			lastErr = errs.E(CodeJudgeResponse, "grok reached the turn limit before answering", "provider", "grok", "reason", "max_turn_requests")
		case stopReason == "cancelled":
			lastErr = errs.E(CodeJudgeTransport, "grok cancelled the turn", "provider", "grok")
		case structuredErr != "":
			// A structured-output failure is transport, not judgment (#165 saw it
			// on grok-4.7-build-fast): retry it rather than recording it as text.
			lastErr = errs.E(CodeJudgeTransport, "grok reported a structured-output error: "+structuredErr, "provider", "grok")
		case !found:
			lastErr = errs.E(CodeJudgeResponse, "grok produced no result", "provider", "grok")
		default:
			v.Raw = text
			v.Parsed, v.Decision, v.Confidence, v.ParseError = Parse(r.Kind, text)
			return v, nil
		}
	}
	return v, lastErr
}

// validateGrok is validate's grok arm: no key, and the CLI's effort set.
func validateGrok(model, effort string, calibration bool) error {
	// route strips the prefix with trimPrefixFold, so an empty model has to be
	// detected the same way (see validateCodex for the case-folding trap).
	if _, over := run.CapText(model, run.MaxShort); over || trimPrefixFold(model, GrokPrefix) == "" {
		return errs.E(CodeJudgeModel, "model id is empty or exceeds MaxShort", "model", model, "reason", "length")
	}
	if !grokEfforts[effort] {
		return errs.E(CodeJudgeEffortUnsupported, "unknown effort "+effort, "effort", effort, "provider", "grok")
	}
	if calibration && effort != CalibrationEff {
		return errs.E(CodeJudgeEffortUnsupported, "calibration requires effort medium", "effort", effort, "reason", "calibration")
	}
	return nil
}

// grokUsage is the CLI's usage block. input_tokens is uncached only, and
// output_tokens includes the reasoning tokens as a subset (total_tokens sums
// input + cache_read + cache_creation + output), verified against the headless
// reference. TokenTotals.Total() sums every field, so the buckets are made
// disjoint here exactly as the claude and codex arms do.
type grokUsage struct {
	Input       int64 `json:"input_tokens"`
	CacheRead   int64 `json:"cache_read_input_tokens"`
	CacheCreate int64 `json:"cache_creation_input_tokens"`
	Output      int64 `json:"output_tokens"`
	Reasoning   int64 `json:"reasoning_tokens"`
}

// grokResult is the subset of the --output-format json document this provider
// reads: the answer text, the stop reason, the spend, and (defensively) a
// structured-output failure. --json-schema output rides in text for this
// format, matching #165's measured parse.
type grokResult struct {
	Text                  *string    `json:"text"`
	StopReason            string     `json:"stopReason"`
	StructuredOutputError string     `json:"structuredOutputError"`
	Usage                 *grokUsage `json:"usage"`
}

// parseGrokResult pulls the answer text, the spend, whether an answer was
// present, the stop reason and a structured-output error out of the CLI's JSON
// document. found is false for a body that is not JSON or carries no text; the
// caller distinguishes the stop reasons and a structured-output error before
// deciding the outcome. A body that never reached the model has no usage, so
// the tokens are zero and the caller does not count them as spend.
func parseGrokResult(stdout []byte) (text string, tokens run.TokenTotals, found bool, stopReason, structuredErr string) {
	var r grokResult
	if json.Unmarshal(stdout, &r) != nil {
		return "", run.TokenTotals{}, false, "", ""
	}
	if r.Usage != nil {
		tokens = run.TokenTotals{
			Input:       clampTok(r.Usage.Input),
			CacheRead:   clampTok(r.Usage.CacheRead),
			CacheCreate: clampTok(r.Usage.CacheCreate),
			Output:      clampTok(r.Usage.Output - r.Usage.Reasoning),
			Reasoning:   clampTok(r.Usage.Reasoning),
		}
	}
	if r.Text == nil || *r.Text == "" {
		return "", tokens, false, r.StopReason, r.StructuredOutputError
	}
	return *r.Text, tokens, true, r.StopReason, r.StructuredOutputError
}

// Temp-file seams. Production uses the os functions. Tests replace them to
// cover the error returns.
var (
	grokCreateTemp = os.CreateTemp
	grokRemoveFile = os.Remove
)

// writeGrokPromptFile writes the rendered user prompt to a private (0600) file
// inside dir, the attempt's isolated working directory. The prompt carries the
// diff under judgment, so it must not be world-readable, and it is removed as
// soon as the call returns.
func writeGrokPromptFile(dir string, data []byte) (string, error) {
	f, err := grokCreateTemp(dir, "metareview-grok-prompt-*.txt")
	if err != nil {
		return "", err
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = grokRemoveFile(f.Name())
		return "", err
	}
	return f.Name(), nil
}
