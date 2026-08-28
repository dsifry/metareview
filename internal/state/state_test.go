package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
