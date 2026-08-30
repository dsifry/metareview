package epicready

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The CLI validates --mutation-report at parse time so a typo fails fast, which leaves this the
// library's own guard: a report that cannot be read must stop the review, never be skipped.
// A skipped report is indistinguishable from a package with no survivors, so the failure mode of
// getting this wrong is a gate that passes because it looked at less.
func TestAnUnreadableMutationReportStopsTheReview(t *testing.T) {
	if _, err := mutationContextFor(nil); err != nil {
		t.Errorf("no reports at all is the ordinary case: %v", err)
	}
	for name, path := range map[string]string{
		"missing file":  filepath.Join(t.TempDir(), "absent.json"),
		"not a report":  writeTemp(t, "this is a test log, not a report"),
		"empty file":    writeTemp(t, ""),
		"files is text": writeTemp(t, `{"files":"none"}`),
	} {
		ctx, err := mutationContextFor([]string{path})
		if err == nil {
			t.Errorf("%s: must be an error, got %d reports", name, len(ctx.Reports))
			continue
		}
		if !strings.Contains(err.Error(), "mutation") {
			t.Errorf("%s: the error must say what could not be read: %v", name, err)
		}
	}
}

// A readable report reaches the reviewers, so the review actually acts on it.
func TestAReadableMutationReportReachesTheReviewers(t *testing.T) {
	path := writeTemp(t, `{"go_module":"m","files":[{"file_name":"a.go","mutations":[{"type":"T","status":"LIVED","line":3}]}]}`)
	ctx, err := mutationContextFor([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.Reports) != 1 || len(ctx.Findings()) != 1 {
		t.Fatalf("the surviving mutant did not reach the reviewers: %+v", ctx)
	}
}

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
