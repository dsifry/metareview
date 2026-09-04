package evidence

import (
	"context"
	"errors"
	"testing"
	"time"
)

// An empty command is a caller error, reported before anything is executed.
func TestRunRejectsEmptyCommand(t *testing.T) {
	if _, err := Run(context.Background(), nil, RunOptions{}); err == nil {
		t.Fatalf("an empty command must be rejected")
	}
}

// When no CWD is supplied, Run falls back to os.Getwd; if that lookup fails, the failure is
// surfaced. The getwd seam lets us exercise this otherwise-unreachable branch.
func TestRunSurfacesGetwdFailure(t *testing.T) {
	original := getwd
	t.Cleanup(func() { getwd = original })
	getwd = func() (string, error) { return "", errors.New("no cwd") }

	if _, err := Run(context.Background(), []string{"true"}, RunOptions{}); err == nil {
		t.Fatalf("a getwd failure with no explicit CWD must be surfaced")
	}
}

// A command whose binary cannot be found returns an error (not an *exec.ExitError), which Run
// surfaces while still reporting a nonzero exit code on the receipt.
func TestRunSurfacesStartFailure(t *testing.T) {
	receipt, err := Run(context.Background(), []string{"metareview-no-such-binary-xyzzy"}, RunOptions{})
	if err == nil {
		t.Fatalf("a missing binary should surface an error")
	}
	if receipt.ExitCode != 1 {
		t.Fatalf("a start failure should report exit code 1, got %d", receipt.ExitCode)
	}
}

// Explicit Env is passed through to the command, and an explicit Now clock is used for the
// receipt timestamps instead of the wall clock.
func TestRunHonorsEnvAndNowClock(t *testing.T) {
	fixed := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	receipt, err := Run(context.Background(),
		[]string{"sh", "-c", `test "$MRV_TEST_ENV" = present`},
		RunOptions{
			Env: []string{"MRV_TEST_ENV=present"},
			Now: func() time.Time { return fixed },
		})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if receipt.ExitCode != 0 {
		t.Fatalf("injected env should reach the command (exit 0 expected): %+v", receipt)
	}
	if !receipt.StartedAt.Equal(fixed) || !receipt.FinishedAt.Equal(fixed) {
		t.Fatalf("injected clock should drive timestamps: %+v", receipt)
	}
}
