package evidence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A committed receipt must not leak the operator's absolute path/username (issue #80): inside a repo the CWD
// is stored relative to the repo root, and never as an absolute path.
func TestReceiptCWDIsRedactedToRepoRelative(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	receipt, err := Run(context.Background(), []string{"sh", "-c", "exit 0"}, RunOptions{CWD: dir})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if receipt.CWD != "." {
		t.Fatalf("CWD at the repo root should be %q, got %q", ".", receipt.CWD)
	}
	if strings.HasPrefix(receipt.CWD, "/") || strings.Contains(receipt.CWD, dir) {
		t.Fatalf("receipt leaks an absolute path: %q", receipt.CWD)
	}
}

// redactCWD directly: empty stays empty, and a path under $HOME (outside any repo) collapses to "~/…".
func TestRedactCWDHomeAndEmpty(t *testing.T) {
	if got := redactCWD(""); got != "" {
		t.Fatalf("empty cwd should stay empty, got %q", got)
	}
	home := t.TempDir() // no repo markers above it, so the repo-relative branch won't fire
	t.Setenv("HOME", home)
	got := redactCWD(filepath.Join(home, "work", "proj"))
	if got != "~/work/proj" {
		t.Fatalf("a path under $HOME should collapse to %q, got %q", "~/work/proj", got)
	}
	if strings.Contains(got, home) {
		t.Fatalf("redaction still leaks the home path: %q", got)
	}
}

// Outside any repo (no markers, not under home), only the leaf directory is kept — never the full path.
func TestReceiptCWDFallsBackToLeaf(t *testing.T) {
	dir := t.TempDir() // /var/folders/... on macOS: no repo markers, not under $HOME
	receipt, err := Run(context.Background(), []string{"sh", "-c", "exit 0"}, RunOptions{CWD: dir})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(receipt.CWD, string(filepath.Separator)) || receipt.CWD != filepath.Base(dir) {
		t.Fatalf("CWD should collapse to the leaf %q, got %q", filepath.Base(dir), receipt.CWD)
	}
}

func TestRunCapturesSuccessfulCommandReceipt(t *testing.T) {
	receipt, err := Run(context.Background(), []string{"sh", "-c", "printf hello"}, RunOptions{})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if receipt.ExitCode != 0 {
		t.Fatalf("expected exit 0: %+v", receipt)
	}
	if receipt.StdoutSHA256 == "" || receipt.StderrSHA256 == "" {
		t.Fatalf("expected output hashes: %+v", receipt)
	}
	if receipt.Summary != "sh -c printf hello exited 0" {
		t.Fatalf("unexpected summary: %q", receipt.Summary)
	}
}

func TestRunReturnsReceiptForFailedCommand(t *testing.T) {
	receipt, err := Run(context.Background(), []string{"sh", "-c", "printf passed; exit 7"}, RunOptions{})
	if err != nil {
		t.Fatalf("nonzero exit should still return a receipt, not an error: %v", err)
	}
	if receipt.ExitCode != 7 {
		t.Fatalf("expected exit 7: %+v", receipt)
	}
	bundle := Bundle{Receipts: []Receipt{receipt}}
	if bundle.HasSuccessfulValidation(KindGeneric) {
		t.Fatalf("failed command receipt must not validate")
	}
}
