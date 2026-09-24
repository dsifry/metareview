package mutationfresh

import (
	"fmt"
	"sync/atomic"
	"testing"
)

// Final review: HEAD content is read once per path per review (Build classifies several reports
// against one Content), and uncached paths are read in parallel, so a repository with thousands of
// attested files does not pay one sequential git process per file per report.
func TestHeadReadCachesAcrossReportsAndReadsInParallel(t *testing.T) {
	files := map[string]string{}
	var paths []string
	for i := 0; i < 40; i++ {
		p := fmt.Sprintf("f%02d.txt", i)
		files[p] = fmt.Sprintf("content %d", i)
		paths = append(paths, p)
	}
	root := gitRepo(t, files)
	var calls atomic.Int64
	saved := runGit
	t.Cleanup(func() { runGit = saved })
	runGit = func(dir string, stdin []byte, args ...string) ([]byte, error) {
		if args[0] == "cat-file" {
			calls.Add(1)
		}
		return saved(dir, stdin, args...)
	}
	h := Head(root)
	got, err := h.Read(append(paths, paths[0])) // a duplicate is read once
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if string(got[p].Data) != files[p] {
			t.Fatalf("%s = %q, want %q", p, got[p].Data, files[p])
		}
	}
	if calls.Load() != 40 {
		t.Errorf("one read per path, got %d", calls.Load())
	}
	again, err := h.Read(paths[:5])
	if err != nil || len(again) != 5 || calls.Load() != 40 {
		t.Errorf("a second report re-reads nothing: %d entries, %d calls, %v", len(again), calls.Load(), err)
	}
}
