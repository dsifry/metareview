package jsonl

import (
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
