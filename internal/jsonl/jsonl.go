// Package jsonl centralises the line cap shared by every reader of the repository's
// .jsonl files (findings.jsonl, runs.jsonl, review logs, run chains).
package jsonl

import (
	"bufio"
	"io"
)

// MaxLineBytes is the JSONL line cap: 1 MiB, not bufio's 64 KiB default. A single record
// can carry long ingested strings, so the default is not enough.
const MaxLineBytes = 1 << 20

// NewScanner returns a bufio.Scanner sized for a MaxLineBytes line.
//
// The token limit is MaxLineBytes+2, not MaxLineBytes. bufio counts the line terminator
// against the limit, so a line of exactly MaxLineBytes needs +1 for a bare LF and +2 for
// CRLF. A reader that passes the bare cap reports "bufio.Scanner: token too long" — for
// the whole file, not just that line — on input another component wrote successfully.
// Callers share one constructor so no reader can get that off-by-two wrong on its own.
func NewScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineBytes+2)
	return sc
}
