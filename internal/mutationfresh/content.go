package mutationfresh

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
)

// Content is the code under review (spec §6.2): the working tree for task-done and epic-ready, and
// HEAD for pr-ready unless --include-working-tree.
type Content interface {
	// Read returns the content of every path that exists as a regular file or symlink; any other
	// path is omitted (its digest is Absent). A read error other than absence stops the review.
	Read(paths []string) (map[string]Entry, error)
	// Paths lists every present path, excluding gitlinks.
	Paths() ([]string, error)
	// Head reports HEAD mode, in which an attested untracked path absent from HEAD is not a change.
	Head() bool
}

// runGit runs git in root. A variable so a test can make one subcommand fail.
var runGit = func(root string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("mutationfresh: git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func splitZ(out []byte) []string {
	var parts []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

type worktree struct{ root string }

// Worktree reads the working tree under root.
func Worktree(root string) Content { return worktree{root: root} }

func (worktree) Head() bool { return false }

// readEntry reads one path: a symlink's link text (never followed) or a regular file's bytes.
// Missing paths, paths under a file, and anything else (a directory) are absent.
func readEntry(full string) (Entry, bool, error) {
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(full)
		return Entry{Data: []byte(target), Symlink: true}, true, err
	}
	if !info.Mode().IsRegular() {
		return Entry{}, false, nil
	}
	data, err := os.ReadFile(full)
	return Entry{Data: data}, true, err
}

func (w worktree) Read(paths []string) (map[string]Entry, error) {
	out := map[string]Entry{}
	for _, p := range paths {
		e, ok, err := readEntry(filepath.Join(w.root, filepath.FromSlash(p)))
		if err != nil {
			return nil, fmt.Errorf("mutationfresh: reading %s: %w", p, err)
		}
		if ok {
			out[p] = e
		}
	}
	return out, nil
}

// Paths is `git ls-files -c -o --exclude-standard --deduplicate` (spec §6.3). A gitlink or a nested
// repository checks out as a directory, which is not content.
func (w worktree) Paths() ([]string, error) {
	listed, err := runGit(w.root, nil, "ls-files", "-c", "-o", "--exclude-standard", "--deduplicate", "-z")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range splitZ(listed) {
		if info, err := os.Lstat(filepath.Join(w.root, filepath.FromSlash(p))); err == nil && info.IsDir() {
			continue
		}
		out = append(out, strings.TrimSuffix(p, "/"))
	}
	return out, nil
}

type head struct {
	root  string
	modes map[string]string // path → git mode at HEAD
	cache map[string]Entry  // content already read in this review
}

// Head reads the committed tree at HEAD through the repository's filters (spec §6.2).
func Head(root string) Content { return &head{root: root} }

func (*head) Head() bool { return true }

func (h *head) load() error {
	if h.modes != nil {
		return nil
	}
	out, err := runGit(h.root, nil, "ls-tree", "-r", "-z", "--full-tree", "HEAD")
	if err != nil {
		return err
	}
	h.modes = map[string]string{}
	h.cache = map[string]Entry{}
	for _, line := range splitZ(out) {
		// "<mode> <type> <object>\t<path>"
		h.modes[line[strings.IndexByte(line, '\t')+1:]] = line[:strings.IndexByte(line, ' ')]
	}
	return nil
}

// Paths is `git ls-tree -r HEAD` without gitlinks (mode 160000), sorted.
func (h *head) Paths() ([]string, error) {
	if err := h.load(); err != nil {
		return nil, err
	}
	var out []string
	for p, mode := range h.modes {
		if mode != "160000" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// headReaders bounds the git processes that read HEAD content at once.
const headReaders = 16

// Read is `git cat-file --filters HEAD:<path>` per present path (spec §6.2). Batch mode cannot be used:
// with --filters its header reports the unfiltered size while the content is filtered, so the output
// cannot be split. Paths are read once per review (Build classifies every report against one Content)
// and in parallel. A symlink's blob is its link text; gitlinks are absent.
func (h *head) Read(paths []string) (map[string]Entry, error) {
	if err := h.load(); err != nil {
		return nil, err
	}
	todo := map[string]bool{}
	for _, p := range paths {
		_, cached := h.cache[p]
		if mode, ok := h.modes[p]; ok && mode != "160000" && !cached {
			todo[p] = true
		}
	}
	if err := h.fill(todo); err != nil {
		return nil, err
	}
	out := map[string]Entry{}
	for _, p := range paths {
		if e, ok := h.cache[p]; ok {
			out[p] = e
		}
	}
	return out, nil
}

// fill reads the given paths into the cache through at most headReaders git processes.
func (h *head) fill(todo map[string]bool) error {
	type result struct {
		path string
		data []byte
		err  error
	}
	jobs := make(chan string)
	results := make(chan result, len(todo))
	var wg sync.WaitGroup
	for w := 0; w < min(headReaders, len(todo)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				data, err := runGit(h.root, nil, "cat-file", "--filters", "HEAD:"+p)
				results <- result{path: p, data: data, err: err}
			}
		}()
	}
	for p := range todo {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	close(results)
	for r := range results { // every reader has finished, so returning early leaks nothing
		if r.err != nil {
			return r.err
		}
		h.cache[r.path] = Entry{Data: r.data, Symlink: h.modes[r.path] == "120000"}
	}
	return nil
}
