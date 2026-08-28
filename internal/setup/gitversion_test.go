package setup

import (
	"errors"
	"strings"
	"testing"
)

// INSTALL.md tells the operator that `metareview fsm` needs git >= 2.31 and that
// `metareview setup --check` reports it. It did not: the check was a PATH
// presence test and ToolStatus carried no version at all, so an operator on an
// older git got a clean report and then a confusing FSM failure.
func TestGitStatusReportsTheVersionAndTheMinimum(t *testing.T) {
	cases := []struct {
		name       string
		out        string
		err        error
		wantOK     bool
		wantAction string
	}{
		{"new enough", "git version 2.43.0\n", nil, true, ""},
		{"exactly the minimum", "git version 2.31.0\n", nil, true, ""},
		{"too old", "git version 2.30.2\n", nil, false, "2.31"},
		{"apple flavoured", "git version 2.39.5 (Apple Git-154)\n", nil, true, ""},
		{"unparseable", "not a version banner\n", nil, false, "2.31"},
		{"cannot run", "", errors.New("boom"), false, "2.31"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := gitStatus(func(string) (string, error) { return "/usr/bin/git", nil },
				func() (string, error) { return c.out, c.err })
			if !got.Present {
				t.Fatalf("git is on PATH, so Present must stay true: %+v", got)
			}
			if got.VersionOK != c.wantOK {
				t.Fatalf("VersionOK = %v, want %v (%+v)", got.VersionOK, c.wantOK, got)
			}
			if c.wantOK {
				if got.Version == "" {
					t.Fatalf("a usable git must report its version: %+v", got)
				}
				if got.Action != "" {
					t.Fatalf("no action needed when the version is fine: %+v", got)
				}
				return
			}
			if !strings.Contains(got.Action, c.wantAction) {
				t.Fatalf("Action %q must name the minimum %q", got.Action, c.wantAction)
			}
		})
	}
}

// A git that is not installed at all keeps the existing behaviour.
func TestGitStatusAbsentIsUnchanged(t *testing.T) {
	got := gitStatus(func(string) (string, error) { return "", errors.New("not found") }, nil)
	if got.Present || got.Action == "" {
		t.Fatalf("an absent git must report Present=false with an action: %+v", got)
	}
}
