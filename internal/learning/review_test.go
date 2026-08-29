package learning

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ReviewOptions.HomeDir exists so session-history discovery is deterministic under test - its own
// comment says that without it "coverage of the discovery paths then tracks whatever session data
// the machine happens to hold". Nothing set it and no test exercised it, so the determinism it
// promises was not delivered anywhere: the paths it controls were covered only by whatever the
// machine running CI happened to have in its real home.
func TestRunPostMergeDiscoversSessionHistoryUnderHomeDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".metareview"), 0o755); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@x")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	base := "HEAD"
	home := t.TempDir()
	transcript := filepath.Join(home, ".claude", "projects", "p")
	if err := os.MkdirAll(transcript, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"timestamp":"2026-08-29T00:00:00Z","type":"user","message":{"role":"user","content":"SENTINEL-FROM-FIXTURE-HOME"}}`
	if err := os.WriteFile(filepath.Join(transcript, "a.jsonl"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := RunPostMerge(root, ReviewOptions{
		PostMergePR: "21",
		Base:        base,
		HomeDir:     home,
		Now:         time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunPostMerge: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(res.AcceptedRel)))
	if err != nil {
		t.Fatalf("read accepted log: %v", err)
	}
	if !strings.Contains(string(body), "Session history: available") {
		t.Errorf("the fixture home's session history was not discovered, so HomeDir does not steer discovery:\n%s", body)
	}

	// The other direction, or the assertion above could pass on the machine's real home: an empty
	// HomeDir must produce no session history at all.
	empty := t.TempDir()
	res2, err := RunPostMerge(root, ReviewOptions{PostMergePR: "22", Base: base, HomeDir: empty,
		Now: time.Date(2026, 8, 29, 13, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("RunPostMerge (empty home): %v", err)
	}
	body2, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(res2.AcceptedRel)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body2), "Session history: available") {
		t.Errorf("an empty HomeDir still found session history, so discovery is reading past it:\n%s", body2)
	}
}
