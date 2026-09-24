package metareview

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The workflow template's static contract (spec §5.7, §7.2, §11.2) and its two verdict shell steps,
// executed with fake exit codes.

type wfStep struct {
	Name            string            `yaml:"name"`
	ID              string            `yaml:"id"`
	If              string            `yaml:"if"`
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	With            map[string]any    `yaml:"with"`
	Env             map[string]string `yaml:"env"`
	ContinueOnError bool              `yaml:"continue-on-error"`
}

type wfJob struct {
	If          string            `yaml:"if"`
	Needs       string            `yaml:"needs"`
	Permissions map[string]string `yaml:"permissions"`
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress bool   `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Outputs map[string]string `yaml:"outputs"`
	Env     map[string]string `yaml:"env"`
	Steps   []wfStep          `yaml:"steps"`
}

type workflow struct {
	On   map[string]any    `yaml:"on"`
	Env  map[string]string `yaml:"env"`
	Jobs map[string]wfJob  `yaml:"jobs"`
}

const workflowPath = "templates/mutation-incremental/github-workflow.yml"

func loadWorkflow(t *testing.T) (workflow, string) {
	t.Helper()
	src, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	var wf workflow
	if err := yaml.Unmarshal(src, &wf); err != nil {
		t.Fatal(err)
	}
	return wf, string(src)
}

func stepIndex(job wfJob, match func(wfStep) bool) int {
	return slices.IndexFunc(job.Steps, match)
}

func named(name string) func(wfStep) bool { return func(s wfStep) bool { return s.Name == name } }

func TestMutationWorkflowStatic(t *testing.T) {
	wf, src := loadWorkflow(t)
	if len(wf.On) != 2 || wf.On["push"] == nil || !mapHas(wf.On, "pull_request") {
		t.Errorf("triggers must be push and pull_request only: %v", wf.On)
	}
	if !strings.Contains(src, "\npermissions: {}\n") {
		t.Error("workflow-level permissions must be {}")
	}
	if strings.Contains(src, "--threshold") {
		t.Error("no --threshold flag anywhere (decision 15)")
	}
	wantPerms := map[string]string{"incremental-pr": "read", "pr-full": "read", "incremental-main": "write", "full": "write"}
	if len(wf.Jobs) != len(wantPerms) {
		t.Fatalf("jobs = %d, want %d", len(wf.Jobs), len(wantPerms))
	}
	if _, ok := wf.Env["MUTATION_STATE_TOKEN"]; ok {
		t.Error("MUTATION_STATE_TOKEN must not be workflow-level")
	}
	var savedKeys []string
	var cachePaths []string
	for name, job := range wf.Jobs {
		if got := job.Permissions["contents"]; got != wantPerms[name] || len(job.Permissions) != 1 {
			t.Errorf("%s: permissions %v, want contents: %s only", name, job.Permissions, wantPerms[name])
		}
		if len(job.Env) != 0 {
			t.Errorf("%s: no job-level env (the token is per step)", name)
		}
		if !strings.HasPrefix(job.Steps[0].Uses, "actions/checkout@") {
			t.Errorf("%s: checkout must be the first step", name)
		}
		install := stepIndex(job, named("install"))
		for i, s := range job.Steps {
			if strings.HasPrefix(s.Uses, "actions/checkout@") && (s.With["fetch-depth"] != 0 || s.With["persist-credentials"] != false) {
				t.Errorf("%s: checkout needs fetch-depth: 0 and persist-credentials: false", name)
			}
			if strings.Contains(s.Run, "${{") {
				t.Errorf("%s/%s: pass outputs through env:, never ${{ }} inside run:", name, s.Name)
			}
			remote := strings.Contains(s.Run, " fetch-state") || strings.Contains(s.Run, " publish-state")
			if _, has := s.Env["MUTATION_STATE_TOKEN"]; has != remote {
				t.Errorf("%s/%s: MUTATION_STATE_TOKEN only (and always) on fetch-state/publish-state steps", name, s.Name)
			}
			if remote && s.ContinueOnError {
				t.Errorf("%s/%s: publish/fetch steps are never continue-on-error", name, s.Name)
			}
			if strings.Contains(s.Run, " fetch-state") && i < install {
				t.Errorf("%s: checkout and project setup precede fetch-state", name)
			}
			if strings.Contains(s.Run, " run --mode") && !s.ContinueOnError {
				t.Errorf("%s/%s: run steps are continue-on-error (a later step decides)", name, s.Name)
			}
			if strings.HasPrefix(s.Run, `node "$MUTATION_CLI" summary`) && s.If != "always()" {
				t.Errorf("%s/%s: the summary is if: always()", name, s.Name)
			}
			if strings.Contains(s.Run, `"$MUTATION_CLI" summary`) && !strings.Contains(s.Run, "|| true") {
				t.Errorf("%s/%s: the summary never fails the job", name, s.Name)
			}
			if strings.HasPrefix(s.Uses, "actions/cache/") {
				cachePaths = append(cachePaths, s.With["path"].(string))
				if strings.HasPrefix(s.Uses, "actions/cache/save@") {
					savedKeys = append(savedKeys, s.With["key"].(string))
					if !strings.Contains(s.If, "hashFiles('.mutation/attestation.json') != ''") {
						t.Errorf("%s/%s: cache saves require the attestation", name, s.Name)
					}
				}
			}
			if strings.Contains(s.Run, " publish-state") {
				save := stepIndex(job, func(x wfStep) bool { return strings.HasPrefix(x.Uses, "actions/cache/save@") && x.If == s.If })
				if save < 0 || save > i {
					t.Errorf("%s/%s: the cache is saved (same condition) before publishing", name, s.Name)
				}
			}
			if strings.HasPrefix(s.Uses, "actions/cache/") && strings.HasPrefix(name, "incremental-pr") && !strings.Contains(s.With["key"].(string), "mutation-pr-${{ github.event.pull_request.number }}-") {
				t.Errorf("%s: N in PR cache keys is github.event.pull_request.number", name)
			}
		}
	}
	for _, p := range cachePaths {
		if p != cachePaths[0] {
			t.Errorf("every cache step uses the identical path list (F10): %q vs %q", p, cachePaths[0])
		}
	}
	slices.Sort(savedKeys)
	if len(slices.Compact(slices.Clone(savedKeys))) != len(savedKeys) {
		t.Errorf("every saved cache key is distinct: %v", savedKeys)
	}

	pr, prFull, main, full := wf.Jobs["incremental-pr"], wf.Jobs["pr-full"], wf.Jobs["incremental-main"], wf.Jobs["full"]
	if !pr.Concurrency.CancelInProgress || main.Concurrency.CancelInProgress || full.Concurrency.CancelInProgress || prFull.Concurrency.CancelInProgress {
		t.Error("only incremental-pr cancels in progress")
	}
	if prFull.Needs != "incremental-pr" || !strings.Contains(prFull.If, "needs.incremental-pr.outputs.route == 'sweep'") || !strings.Contains(prFull.If, "exit_code == '1'") {
		t.Errorf("pr-full needs incremental-pr and runs only on a sweep route with exit 0/1: %q", prFull.If)
	}
	if strings.Contains(stepsText(prFull), "MUTATION_STATE_TOKEN") || strings.Contains(stepsText(prFull), "publish-state") {
		t.Error("pr-full has no token and no publish step")
	}
	if full.Needs != "incremental-main" || !strings.Contains(full.If, "!cancelled()") || !strings.Contains(full.If, "needs.incremental-main.outputs.pending_full == 'true'") {
		t.Errorf("full needs incremental-main, runs when not cancelled and pending: %q", full.If)
	}
	catchup, fullRun := stepIndex(full, named("catch-up")), stepIndex(full, named("full run"))
	if catchup < 0 || fullRun < catchup {
		t.Error("the full job runs the catch-up before run --mode full")
	}
	publishCatchup := full.Steps[stepIndex(full, named("publish catch-up"))]
	if !strings.Contains(publishCatchup.If, "steps.catchup.outputs.pending_full == 'false'") {
		t.Error("the catch-up is published only when it cleared pending")
	}
	if !strings.Contains(stepsText(main), "publish-state --kind inc") || strings.Contains(main.Steps[stepIndex(main, named("summary"))].Run, "--job pr") {
		t.Error("incremental-main publishes inc; its summary omits main's score (--job main)")
	}
	for _, key := range []string{"exit_code", "pending_full"} {
		if main.Outputs[key] == "" {
			t.Errorf("incremental-main exposes %s", key)
		}
	}
	for _, key := range []string{"exit_code", "route"} {
		if pr.Outputs[key] == "" {
			t.Errorf("incremental-pr exposes %s", key)
		}
	}
}

func mapHas(m map[string]any, key string) bool { _, ok := m[key]; return ok }

func stepsText(job wfJob) string {
	out, _ := yaml.Marshal(job.Steps)
	return string(out)
}

func runVerdict(t *testing.T, script string, env map[string]string) int {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		t.Fatal(err)
	}
	return 0
}

func TestMutationWorkflowVerdicts(t *testing.T) {
	wf, _ := loadWorkflow(t)
	verdict := func(job string) string {
		j := wf.Jobs[job]
		return j.Steps[stepIndex(j, named("verdict"))].Run
	}
	// incremental-pr (spec §11.2): a sweep route is green (pr-full decides), fail and any other
	// non-zero exit are red, an empty exit code is red.
	for _, c := range []struct {
		exit, route string
		want        int
	}{
		{"0", "verdict", 0}, {"1", "verdict", 1}, {"0", "sweep", 0}, {"1", "sweep", 0},
		{"0", "fail", 1}, {"1", "fail", 1}, {"", "", 1}, {"2", "", 1}, {"3", "", 1}, {"4", "", 1}, {"130", "", 1},
	} {
		if got := runVerdict(t, verdict("incremental-pr"), map[string]string{"EXIT_CODE": c.exit, "ROUTE": c.route}); got != c.want {
			t.Errorf("incremental-pr exit=%q route=%q: got %d, want %d", c.exit, c.route, got, c.want)
		}
	}
	// full (spec §5.7 step 5): the full run's exit code when that step ran, else the catch-up's.
	for _, c := range []struct {
		outcome, fullExit, catchupExit string
		want                           int
	}{
		{"skipped", "", "0", 0}, {"skipped", "", "1", 1}, {"skipped", "", "", 1},
		{"success", "0", "0", 0}, {"failure", "1", "0", 1}, {"failure", "", "0", 1}, {"failure", "4", "4", 1},
	} {
		env := map[string]string{"FULL_OUTCOME": c.outcome, "FULL_EXIT": c.fullExit, "CATCHUP_EXIT": c.catchupExit}
		if got := runVerdict(t, verdict("full"), env); got != c.want {
			t.Errorf("full outcome=%q full=%q catch-up=%q: got %d, want %d", c.outcome, c.fullExit, c.catchupExit, got, c.want)
		}
	}
	for _, job := range []string{"incremental-main", "pr-full"} {
		for _, c := range []struct {
			exit string
			want int
		}{{"0", 0}, {"1", 1}, {"", 1}, {"4", 1}} {
			if got := runVerdict(t, verdict(job), map[string]string{"EXIT_CODE": c.exit}); got != c.want {
				t.Errorf("%s exit=%q: got %d, want %d", job, c.exit, got, c.want)
			}
		}
	}
}
