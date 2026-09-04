package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendJSONLMarshalError(t *testing.T) {
	// A channel cannot be JSON-marshaled, so AppendJSONL must surface the marshal error.
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	if err := AppendJSONL(path, make(chan int)); err == nil {
		t.Fatal("AppendJSONL must return the json.Marshal error for an unmarshalable record")
	}
}

func TestAppendJSONLMkdirError(t *testing.T) {
	// A parent path that is a regular file makes MkdirAll fail.
	root := t.TempDir()
	fileAsParent := filepath.Join(root, "notadir")
	if err := os.WriteFile(fileAsParent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendJSONL(filepath.Join(fileAsParent, "sub", "f.jsonl"), map[string]string{"id": "x"}); err == nil {
		t.Fatal("AppendJSONL must fail when the parent path is a file")
	}
}

func TestAppendJSONLOpenError(t *testing.T) {
	// Opening a directory for writing fails.
	dir := t.TempDir()
	if err := AppendJSONL(dir, map[string]string{"id": "x"}); err == nil {
		t.Fatal("AppendJSONL must fail when the path is a directory")
	}
}

func TestReadJSONLUnmarshalError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(path, []byte("{not valid json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadJSONL[map[string]string](path); err == nil {
		t.Fatal("ReadJSONL must surface a malformed JSON line")
	}
}

func TestReadJSONLMissingIsEmpty(t *testing.T) {
	got, err := ReadJSONL[map[string]string](filepath.Join(t.TempDir(), "absent.jsonl"))
	if err != nil || got != nil {
		t.Fatalf("a missing file must be (nil,nil), got %v %v", got, err)
	}
}

func TestReadJSONLSkipsBlankLinesAndReadsRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	if err := os.WriteFile(path, []byte(`{"id":"a"}`+"\n\n"+`{"id":"b"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadJSONL[map[string]string](path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 2 || got[0]["id"] != "a" || got[1]["id"] != "b" {
		t.Fatalf("blank line must be skipped, records preserved: %+v", got)
	}
}

func TestReadJSONLSurfacesOpenError(t *testing.T) {
	orig := openFile
	openFile = func(string) (*os.File, error) { return nil, os.ErrPermission }
	defer func() { openFile = orig }()
	if _, err := ReadJSONL[map[string]string]("whatever"); err == nil {
		t.Fatal("ReadJSONL must surface a non-NotExist open error")
	}
}
