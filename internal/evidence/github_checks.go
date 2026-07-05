package evidence

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type GitHubCheckOptions struct {
	Repo   string
	Runner CommandRunner
}

type ghCheck struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Bucket      string `json:"bucket"`
	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt"`
	Link        string `json:"link"`
	Workflow    string `json:"workflow"`
}

func ImportGitHubChecks(ctx context.Context, pr string, options GitHubCheckOptions) (Bundle, error) {
	runner := options.Runner
	if runner == nil {
		runner = defaultRunner
	}
	args := []string{"pr", "checks", pr, "--json", "name,state,bucket,startedAt,completedAt,link,workflow"}
	if options.Repo != "" {
		args = append(args, "--repo", options.Repo)
	}
	data, err := runner(ctx, "gh", args...)
	if err != nil && len(data) == 0 {
		return Bundle{}, err
	}
	var checks []ghCheck
	if err := json.Unmarshal(data, &checks); err != nil {
		return Bundle{}, err
	}
	receipts := make([]Receipt, 0, len(checks))
	for _, check := range checks {
		exitCode := 1
		if checkSucceeded(check) {
			exitCode = 0
		}
		summary := strings.TrimSpace(check.Name + " " + firstNonEmpty(check.Bucket, check.State))
		receipts = append(receipts, Receipt{
			SchemaVersion: 1,
			Kind:          ReceiptKindCICheck,
			Command:       []string{"gh", "pr", "checks", pr},
			ExitCode:      exitCode,
			Summary:       summary,
			Covers:        []string{"github-check:" + check.Name},
		})
	}
	return Bundle{Receipts: receipts}, nil
}

func checkSucceeded(check ghCheck) bool {
	bucket := strings.ToLower(check.Bucket)
	if bucket != "" {
		return bucket == "pass"
	}
	state := strings.ToLower(check.State)
	return state == "success" || state == "successful" || state == "pass" || state == "passed"
}

func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}
