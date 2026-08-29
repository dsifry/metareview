package jsonl

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A line of exactly MaxLineBytes must scan, with either terminator. Passing the bare cap
// instead of +2 fails both cases, which is how internal/fsm/record came to be unable to
// read a runs.jsonl line internal/runchain writes.
func TestScannerAcceptsAnExactlyMaxLengthLine(t *testing.T) {
	line := strings.Repeat("x", MaxLineBytes)
	for _, tc := range []struct{ name, terminator string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			sc := NewScanner(strings.NewReader(line + tc.terminator))
			if !sc.Scan() {
				t.Fatalf("a %d-byte line did not scan: %v", MaxLineBytes, sc.Err())
			}
			if got := strings.TrimRight(sc.Text(), "\r"); len(got) != MaxLineBytes {
				t.Fatalf("got %d bytes, want %d", len(got), MaxLineBytes)
			}
			if err := sc.Err(); err != nil {
				t.Fatalf("scan error: %v", err)
			}
		})
	}
}

// The package doc claims to cover every JSONL reader in the repository. That claim was false for
// months - four of nine readers ran bufio's 64 KiB default, and session-history discovery aborted
// `metareview learn --post-merge` on any real Claude Code transcript - so the claim is checked
// rather than asserted. A bare bufio.NewScanner anywhere in internal/ (outside this package,
// which builds the configured one) fails here.
func TestNoBareScannerOutsideThisPackage(t *testing.T) {
	root := ".."
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.Contains(filepath.ToSlash(path), "/jsonl/") {
			return nil // this package is where the configured scanner is built
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), "bufio.NewScanner(") {
			offenders = append(offenders, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Errorf("bare bufio.NewScanner (64 KiB default) found in %v; use jsonl.NewScanner, or narrow this test if the file genuinely reads something other than JSONL", offenders)
	}
}
