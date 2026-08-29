package judge

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// CodexPrefix marks a model judged through the Codex CLI rather than over HTTP.
//
// The CLI holds the credential — a ChatGPT OAuth session under ~/.codex — so
// this provider needs no API key of its own, and metareview never sees a token.
const CodexPrefix = "codex/"

// CodexBin is the executable name; the caller resolves it on PATH.
const CodexBin = "codex"

// CodexExec runs one codex invocation. stdout is the `--json` event stream, and
// code is the process exit status. err is non-nil only when the process could
// not be run at all — a model that refuses is a clean exit with an error event.
// dir is the working directory the CLI runs in. Empty means inherit the caller's, which is
// the historical behaviour; a materialized sandbox (internal/fsm/sandbox) narrows a judge to
// the evidence it was given, since the CLI can read and execute inside whatever it inherits.
type CodexExec func(ctx context.Context, dir string, args []string, stdin string) (stdout []byte, code int, err error)

// codexEfforts is the CLI's reasoning-effort enum, which is wider than the HTTP
// providers'. Taken from the API's own rejection message for an invalid value.
var codexEfforts = map[string]bool{
	"none": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true,
}

// codexJudge answers Requests by shelling out to the Codex CLI.
type codexJudge struct {
	exec    CodexExec
	nonce   func() string
	clock   Clock
	workDir string // empty: inherit the caller's directory
}

// Call renders the same prompts the HTTP providers use and parses the same way,
// so a verdict is comparable no matter which provider produced it.
func (j *codexJudge) Call(ctx context.Context, r Request) (v Verdict, err error) {
	v = Verdict{Kind: r.Kind, Model: r.Model, Effort: r.Effort, InputHash: InputHash(r.Input)}
	if err := validateCodex(r.Model, r.Effort, r.Calibration); err != nil {
		return v, err
	}
	system, user, err := RenderPrompt(r.Kind, r.Input, r.Fence, r.Calibration, j.nonce())
	if err != nil {
		return v, err
	}
	start := j.clock.Now()
	defer func() { v.Duration = j.clock.Now().Sub(start) }()

	args := []string{
		"exec", "--json",
		"--sandbox", "read-only", // a judge reads; it must never edit the tree it is judging
		"--skip-git-repo-check", // judging is not tied to a repository
		"--color", "never",
		"-m", wireModel(r.Model),
		"-c", "model_reasoning_effort=" + r.Effort,
		"-", // the prompt arrives on stdin, never as an argv the process table would show
	}
	// The same attempt ceiling, per-attempt deadline and backoff as the HTTP arm.
	// This provider had none of the three: a bare caller context with no deadline,
	// so a codex exec that never returns stalls the run forever, and a single
	// attempt, so one transient failure was fatal. Whatever retrying the CLI does
	// internally is its own business; the timeout has to exist here because
	// nothing else in the process is going to impose one.
	prompt := system + "\n\n" + user
	var lastErr error
	for attempt := 0; attempt < MaxAttempts; attempt++ {
		// Checked before every attempt, including the first. Relying on the
		// select below made the guarantee incidental: with a real clock the
		// backoff timer is slow enough that ctx.Done() always wins, but attempt 0
		// skips the select entirely, so an already-cancelled caller still spawned
		// one codex exec. Explicit beats accidental.
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
		actx, cancel := context.WithTimeout(ctx, AttemptTimeout)
		stdout, code, execErr := j.exec(actx, j.workDir, args, prompt)
		cancel()

		text, tokens, found := parseCodexEvents(stdout)
		v.Tokens = v.Tokens.Add(tokens)
		switch {
		case execErr != nil:
			lastErr = errs.E(CodeJudgeTransport, "codex could not be run: "+execErr.Error(), "provider", "codex")
		case code != 0:
			lastErr = errs.E(CodeJudgeTransport, "codex exited "+itoa(code), "provider", "codex", "exit", itoa(code))
		case !found:
			lastErr = errs.E(CodeJudgeResponse, "codex produced no agent message", "provider", "codex")
		default:
			v.Raw = text
			v.Parsed, v.Decision, v.Confidence, v.ParseError = Parse(r.Kind, text)
			return v, nil
		}
	}
	return v, lastErr
}

// validateCodex is validate's codex arm: no key, and the CLI's wider effort set.
func validateCodex(model, effort string, calibration bool) error {
	// route strips the prefix with trimPrefixFold, so an empty model has to be detected the
	// same way: comparing against the literal lowercase prefix let "Codex/" through and the
	// CLI was spawned with an empty -m value.
	if _, over := run.CapText(model, run.MaxShort); over || trimPrefixFold(model, CodexPrefix) == "" {
		return errs.E(CodeJudgeModel, "model id is empty or exceeds MaxShort", "model", model, "reason", "length")
	}
	if !codexEfforts[effort] {
		return errs.E(CodeJudgeEffortUnsupported, "unknown effort "+effort, "effort", effort, "provider", "codex")
	}
	if calibration && effort != CalibrationEff {
		return errs.E(CodeJudgeEffortUnsupported, "calibration requires effort medium", "effort", effort, "reason", "calibration")
	}
	return nil
}

// codexEvent is the subset of the `--json` stream this provider reads.
type codexEvent struct {
	Type string `json:"type"`
	Item struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
	Usage struct {
		Input       int64 `json:"input_tokens"`
		Cached      int64 `json:"cached_input_tokens"`
		CacheWrite  int64 `json:"cache_write_input_tokens"`
		Output      int64 `json:"output_tokens"`
		ReasoningOu int64 `json:"reasoning_output_tokens"`
	} `json:"usage"`
}

// parseCodexEvents pulls the last agent message and the turn's token usage out
// of the event stream. A line that is not JSON is ignored rather than fatal:
// the stream is a CLI's stdout, and a future version may add lines this build
// does not know.
func parseCodexEvents(stdout []byte) (text string, tokens run.TokenTotals, found bool) {
	for _, line := range strings.Split(string(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev codexEvent
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "item.completed":
			if ev.Item.Type == "agent_message" {
				text, found = ev.Item.Text, true
			}
		case "turn.completed":
			// input_tokens is the whole prompt with cached_input_tokens a subset of it, and
			// output_tokens the whole completion with reasoning a subset - the same convention
			// the HTTP arm subtracts for (judge.go). TokenTotals.Total() sums every field and
			// feeds the budget convergence atom, so the categories are made disjoint here;
			// assigning both raw bills the cached half twice and trips budget early.
			tokens = run.TokenTotals{
				Input:       clampTok(ev.Usage.Input - ev.Usage.Cached),
				CacheRead:   clampTok(ev.Usage.Cached),
				CacheCreate: clampTok(ev.Usage.CacheWrite),
				Output:      clampTok(ev.Usage.Output - ev.Usage.ReasoningOu),
				Reasoning:   clampTok(ev.Usage.ReasoningOu),
			}
		}
	}
	return text, tokens, found
}

// itoa avoids pulling strconv in for two call sites.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
