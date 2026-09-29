package state

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// appendChildEnv makes the test binary a child that appends one row and exits (see TestAppendJSONLChild).
const appendChildEnv = "METAREVIEW_APPEND_CHILD"

type bigRow struct {
	Writer  int    `json:"writer"`
	Payload string `json:"payload"`
}

// TestAppendJSONLChild is the child process body: it appends one row far larger than an atomic pipe write.
func TestAppendJSONLChild(t *testing.T) {
	spec := os.Getenv(appendChildEnv)
	if spec == "" {
		t.Skip("child process only")
	}
	path, id, _ := strings.Cut(spec, "|")
	var n int
	if _, err := fmt.Sscan(id, &n); err != nil {
		t.Fatal(err)
	}
	if err := AppendJSONL(path, bigRow{Writer: n, Payload: strings.Repeat(string(rune('a'+n%26)), 256<<10)}); err != nil {
		t.Fatal(err)
	}
}

// AC-5.5 (#180): 100 concurrent AppendJSONL calls from separate processes to one file yield 100 intact,
// parseable rows.
func TestAppendJSONLFromConcurrentProcesses(t *testing.T) {
	if os.Getenv(appendChildEnv) != "" {
		t.Skip("parent only")
	}
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	const writers = 100
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestAppendJSONLChild$", "-test.count=1")
			cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s|%d", appendChildEnv, path, i))
			if out, err := cmd.CombinedOutput(); err != nil {
				errs <- fmt.Errorf("writer %d: %v\n%s", i, err, out)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	seen := map[int]bool{}
	for scanner.Scan() {
		var row bigRow
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatalf("a torn row: %v", err)
		}
		want := strings.Repeat(string(rune('a'+row.Writer%26)), 256<<10)
		if row.Payload != want || seen[row.Writer] {
			t.Fatalf("row of writer %d is corrupt or repeated", row.Writer)
		}
		seen[row.Writer] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != writers {
		t.Fatalf("got %d intact rows, want %d", len(seen), writers)
	}
}

// A failed lock fails the append, which writes nothing.
func TestAppendJSONLLockFailure(t *testing.T) {
	saved := appendLock
	t.Cleanup(func() { appendLock = saved })
	boom := errors.New("boom")
	appendLock = func(int, int) error { return boom }
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	if err := AppendJSONL(path, bigRow{}); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if raw, _ := os.ReadFile(path); len(raw) != 0 {
		t.Fatalf("nothing may be written: %q", raw)
	}
}
