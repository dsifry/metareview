package evidence

import (
	"context"
	"errors"
	"testing"
)

func TestImportGitHubChecksCreatesCIReceipts(t *testing.T) {
	bundle, err := ImportGitHubChecks(context.Background(), "3", GitHubCheckOptions{
		Repo: "dsifry/metareview",
		Runner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			assertContainsArg(t, args, "name,state,bucket,startedAt,completedAt,link,workflow")
			return []byte(`[
				{"name":"go test","bucket":"pass","state":"SUCCESS"},
				{"name":"lint","bucket":"fail","state":"FAILURE"},
				{"name":"deploy","bucket":"pending","state":"PENDING"},
				{"name":"docs","bucket":"skipping","state":"SKIPPED"}
			]`), nil
		},
	})
	if err != nil {
		t.Fatalf("import checks: %v", err)
	}
	if len(bundle.Receipts) != 4 {
		t.Fatalf("expected four receipts: %+v", bundle.Receipts)
	}
	if bundle.HasSuccessfulValidation(KindCICheck) {
		t.Fatalf("mixed failed/pending/skipped checks must not satisfy ci-check validation: %+v", bundle.Receipts)
	}
	for _, receipt := range bundle.Receipts {
		if receipt.Summary == "lint fail" && receipt.ExitCode == 0 {
			t.Fatalf("failed check must not be successful: %+v", receipt)
		}
		if receipt.Summary == "deploy pending" && receipt.ExitCode == 0 {
			t.Fatalf("pending check must not be successful: %+v", receipt)
		}
		if receipt.Summary == "docs skipping" && receipt.ExitCode == 0 {
			t.Fatalf("skipped check must not be successful: %+v", receipt)
		}
	}
}

func TestImportGitHubChecksPassesWhenAllChecksSucceed(t *testing.T) {
	bundle, err := ImportGitHubChecks(context.Background(), "3", GitHubCheckOptions{
		Runner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte(`[
				{"name":"go test","bucket":"pass"},
				{"name":"lint","state":"SUCCESS"}
			]`), nil
		},
	})
	if err != nil {
		t.Fatalf("import checks: %v", err)
	}
	if !bundle.HasSuccessfulValidation(KindCICheck) {
		t.Fatalf("all-success checks should satisfy ci-check validation: %+v", bundle.Receipts)
	}
}

func TestImportGitHubChecksParsesPendingJSONDespiteExitCode(t *testing.T) {
	bundle, err := ImportGitHubChecks(context.Background(), "3", GitHubCheckOptions{
		Runner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte(`[{"name":"slow check","bucket":"pending","state":"PENDING"}]`), errors.New("exit status 8")
		},
	})
	if err != nil {
		t.Fatalf("pending checks with JSON should import as receipts, not unavailable context: %v", err)
	}
	if len(bundle.Receipts) != 1 || bundle.Receipts[0].ExitCode == 0 {
		t.Fatalf("pending check should import as failed receipt: %+v", bundle.Receipts)
	}
	if bundle.HasSuccessfulValidation(KindCICheck) {
		t.Fatalf("pending check set must not satisfy validation: %+v", bundle.Receipts)
	}
}

func TestImportGitHubChecksReportsUnavailableContext(t *testing.T) {
	_, err := ImportGitHubChecks(context.Background(), "3", GitHubCheckOptions{
		Runner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return nil, errors.New("gh auth required")
		},
	})
	if err == nil {
		t.Fatalf("expected unavailable gh context error")
	}
}

func assertContainsArg(t *testing.T, args []string, expected string) {
	t.Helper()
	for _, arg := range args {
		if arg == expected {
			return
		}
	}
	t.Fatalf("missing arg %q in %v", expected, args)
}
