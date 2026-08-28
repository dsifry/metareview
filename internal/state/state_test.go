package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsifry/metareview/internal/jsonl"
)

func TestAppendJSONLSurfacesCloseError(t *testing.T) {
	// A write can fail at Close rather than at Write. This append records every
	// finding and every run, so a swallowed Close error would let metareview
	// report a row as recorded when it never landed on disk.
	original := closeFile
	closeFile = func(f *os.File) error {
		_ = f.Close()
		return errors.New("disk went away")
	}
	defer func() { closeFile = original }()

	path := filepath.Join(t.TempDir(), "runs.jsonl")
	err := AppendJSONL(path, map[string]string{"id": "mrv-1"})
	if err == nil || !strings.Contains(err.Error(), "disk went away") {
		t.Fatalf("Close error was swallowed: %v", err)
	}
}

func TestAppendJSONLKeepsTheWriteErrorWhenCloseAlsoFails(t *testing.T) {
	// Close must not overwrite a more specific earlier failure.
	original := closeFile
	closeFile = func(f *os.File) error {
		_ = f.Close()
		return errors.New("secondary")
	}
	defer func() { closeFile = original }()

	path := filepath.Join(t.TempDir(), "runs.jsonl")
	err := AppendJSONL(path, make(chan int)) // json.Marshal rejects a channel
	if err == nil || strings.Contains(err.Error(), "secondary") {
		t.Fatalf("the marshal error should survive, got: %v", err)
	}
}

// ReadJSONL shares jsonl.NewScanner with every other .jsonl reader; this pins that it
// actually uses it. Without the test, re-introducing a local bufio scanner sized at the
// bare cap regresses silently — a line of exactly MaxLineBytes then fails the whole file.
func TestReadJSONLAcceptsAnExactlyMaxLengthLine(t *testing.T) {
	type row struct {
		ID  string `json:"id"`
		Pad string `json:"pad"`
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "rows.jsonl")
	r := row{ID: "a"}
	encode := func(v row) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}
	if pad := jsonl.MaxLineBytes - len(encode(r)); pad > 0 {
		r.Pad = strings.Repeat("x", pad)
	}
	line := encode(r)
	if len(line) != jsonl.MaxLineBytes {
		t.Fatalf("fixture is %d bytes, want %d", len(line), jsonl.MaxLineBytes)
	}
	body := append(append(line, '\n'), append(encode(row{ID: "b"}), '\n')...)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows, err := ReadJSONL[row](path)
	if err != nil {
		t.Fatalf("ReadJSONL: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != "a" || rows[1].ID != "b" {
		t.Fatalf("got %d rows %+v, want both", len(rows), rows)
	}
}
