package cli

import (
	"context"
	"strings"
	"testing"
)

func overrideDeps(env map[string]string) *ctxDeps {
	return &ctxDeps{deps: Deps{Getenv: func(k string) string { return env[k] }}}
}

// The override exists so an operator can retarget the judge without editing a
// workflow. Precedence is flag → environment → the workflow's own var.
func TestJudgeOverridePrecedence(t *testing.T) {
	cases := []struct {
		name                  string
		env                   map[string]string
		modelFlag, effortFlag string
		wantModel, wantEffort string
	}{
		{"nothing set", nil, "", "", "", ""},
		{"env only", map[string]string{EnvJudgeModel: "codex/gpt-5.6-sol", EnvJudgeEffort: "medium"}, "", "", "codex/gpt-5.6-sol", "medium"},
		{"flag beats env", map[string]string{EnvJudgeModel: "codex/old", EnvJudgeEffort: "low"}, "codex/gpt-5.6-sol", "high", "codex/gpt-5.6-sol", "high"},
		{"model only", nil, "codex/gpt-5.6-sol", "", "codex/gpt-5.6-sol", ""},
		{"effort only", nil, "", "max", "", "max"},
		{"blank flag falls through to env", map[string]string{EnvJudgeModel: "codex/env"}, "   ", "", "codex/env", ""},
		{"surrounding space is trimmed", nil, "  codex/gpt-5.6-sol  ", " medium ", "codex/gpt-5.6-sol", "medium"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := overrideDeps(c.env).judgeOverride(c.modelFlag, c.effortFlag)
			if o.Model != c.wantModel || o.Effort != c.wantEffort {
				t.Fatalf("got %+v, want model=%q effort=%q", o, c.wantModel, c.wantEffort)
			}
		})
	}
}

// The override is folded into the run's vars so the model that judged a run
// stays visible in its snapshot; it must not mutate the caller's parsed vars.
func TestApplyJudgeOverrideCopiesAndOverlays(t *testing.T) {
	vars := map[string]string{"JUDGE": "gpt-5.2", "JUDGE_EFFORT": "medium", "REVIEWER": "claude-opus-5"}

	got := overrideDeps(nil).applyJudgeOverride(vars, "codex/gpt-5.6-sol", "")
	if got[JudgeVar] != "codex/gpt-5.6-sol" || got[JudgeEffortVar] != "medium" {
		t.Fatalf("overlay: %+v", got)
	}
	if got["REVIEWER"] != "claude-opus-5" {
		t.Fatalf("unrelated vars must survive: %+v", got)
	}
	if vars[JudgeVar] != "gpt-5.2" {
		t.Fatalf("the caller's map was mutated: %+v", vars)
	}

	// Effort alone still copies, and leaves the model as the workflow set it.
	got = overrideDeps(map[string]string{EnvJudgeEffort: "high"}).applyJudgeOverride(vars, "", "")
	if got[JudgeEffortVar] != "high" || got[JudgeVar] != "gpt-5.2" {
		t.Fatalf("effort-only overlay: %+v", got)
	}

	// With nothing set the same map comes back: no copy, no change.
	same := overrideDeps(nil).applyJudgeOverride(vars, "", "")
	if len(same) != len(vars) || same[JudgeVar] != "gpt-5.2" {
		t.Fatalf("unset override changed the vars: %+v", same)
	}
}

func TestRealCodexExec(t *testing.T) {
	original := codexBin
	defer func() { codexBin = original }()

	t.Run("stdout and stdin", func(t *testing.T) {
		codexBin = "cat"
		out, code, err := realCodexExec(context.Background(), nil, "the prompt")
		if err != nil || code != 0 || strings.TrimSpace(string(out)) != "the prompt" {
			t.Fatalf("out=%q code=%d err=%v", out, code, err)
		}
	})

	t.Run("a non-zero exit is an answer, not a failure to run", func(t *testing.T) {
		codexBin = "sh"
		out, code, err := realCodexExec(context.Background(), []string{"-c", "printf partial; exit 3"}, "")
		if err != nil {
			t.Fatalf("a process that ran must not report err: %v", err)
		}
		if code != 3 || string(out) != "partial" {
			t.Fatalf("code=%d out=%q", code, out)
		}
	})

	t.Run("a missing binary is a failure to run", func(t *testing.T) {
		codexBin = "metareview-no-such-binary"
		_, code, err := realCodexExec(context.Background(), nil, "")
		if err == nil {
			t.Fatal("expected an error when the CLI is not installed")
		}
		if code != 0 {
			t.Fatalf("code must stay 0 when nothing ran, got %d", code)
		}
	})
}

// --calibration pins JUDGE and JUDGE_EFFORT so calibration runs stay comparable,
// and resolve refuses a run that also supplies them. An explicit flag is a real
// conflict and must still be refused; an ambient environment variable is not —
// exporting METAREVIEW_JUDGE_MODEL once made every later calibration run fail
// with an error naming a flag the operator never passed.
func TestCalibrationIgnoresTheEnvironmentButNotAFlag(t *testing.T) {
	env := map[string]string{EnvJudgeModel: "codex/gpt-5.6-sol", EnvJudgeEffort: "max"}
	vars := map[string]string{"REVIEWER": "claude-opus-5"}

	t.Run("environment does not reach a calibration run", func(t *testing.T) {
		got := overrideDeps(env).applyJudgeOverrideFor(vars, "", "", true)
		if _, pinned := got[JudgeVar]; pinned {
			t.Fatalf("the environment overrode a pinned var: %+v", got)
		}
		if _, pinned := got[JudgeEffortVar]; pinned {
			t.Fatalf("the environment overrode a pinned effort: %+v", got)
		}
		if got["REVIEWER"] != "claude-opus-5" {
			t.Fatalf("unrelated vars must survive: %+v", got)
		}
	})

	t.Run("an explicit flag still reaches it, so resolve can refuse", func(t *testing.T) {
		got := overrideDeps(nil).applyJudgeOverrideFor(vars, "codex/gpt-5.6-sol", "", true)
		if got[JudgeVar] != "codex/gpt-5.6-sol" {
			t.Fatalf("an explicit --judge-model must still be passed through so the pin conflict is reported: %+v", got)
		}
	})

	t.Run("outside calibration the environment applies as before", func(t *testing.T) {
		got := overrideDeps(env).applyJudgeOverrideFor(vars, "", "", false)
		if got[JudgeVar] != "codex/gpt-5.6-sol" || got[JudgeEffortVar] != "max" {
			t.Fatalf("normal runs still honour the environment: %+v", got)
		}
	})
}
