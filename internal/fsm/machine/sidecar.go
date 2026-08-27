package machine

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

var sidecarName = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// ValidSidecarName reports whether name may be stored beside audit.jsonl.
func ValidSidecarName(name string) bool {
	return sidecarName.MatchString(name) && !strings.HasPrefix(name, "audit.") && name != "lock"
}

func sidecarErr(reason, detail string) error {
	return errs.E(CodeSidecar, detail, "reason", reason)
}

func checkSidecarArgs(runID, name string) error {
	if err := run.ValidateRunID(runID); err != nil {
		return sidecarErr("path", "invalid run id")
	}
	if !ValidSidecarName(name) {
		return sidecarErr("name", "invalid sidecar name "+name)
	}
	return nil
}

// FSSidecar stores sidecars under <root>/.metareview/runs/<id>/. Open is
// the file seam (nil → os.OpenFile); tests inject failing files.
type FSSidecar struct {
	Root string
	Open func(path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error)
}

func (f FSSidecar) path(runID, name string) string {
	return filepath.Join(f.Root, ".metareview", "runs", runID, name)
}

func (f FSSidecar) open(path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
	if f.Open != nil {
		return f.Open(path, flag, perm)
	}
	return os.OpenFile(path, flag, perm)
}

// Write creates the file exclusively (0600, O_NOFOLLOW); the run directory
// must already exist (run.Create makes it).
func (f FSSidecar) Write(runID, name string, b []byte) error {
	if err := checkSidecarArgs(runID, name); err != nil {
		return err
	}
	fh, err := f.open(f.path(runID, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return sidecarErr("exists", name+" already exists")
		}
		return sidecarErr("path", err.Error())
	}
	_, werr := fh.Write(b)
	if err := errors.Join(werr, fh.Close()); err != nil {
		return sidecarErr("path", err.Error())
	}
	return nil
}

// Read returns at most run.MaxPayload bytes.
func (f FSSidecar) Read(runID, name string) ([]byte, error) {
	if err := checkSidecarArgs(runID, name); err != nil {
		return nil, err
	}
	fh, err := f.open(f.path(runID, name), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, sidecarErr("missing", name+" is missing")
		}
		return nil, sidecarErr("path", err.Error())
	}
	defer fh.Close()
	b, err := io.ReadAll(io.LimitReader(fh, run.MaxPayload+1))
	if err != nil {
		return nil, sidecarErr("path", err.Error())
	}
	if len(b) > run.MaxPayload {
		return nil, sidecarErr("too_large", name+" exceeds MaxPayload")
	}
	return b, nil
}

// List names the sidecars of a run (never audit.* or lock).
func (f FSSidecar) List(runID string) ([]string, error) {
	if err := run.ValidateRunID(runID); err != nil {
		return nil, sidecarErr("path", "invalid run id")
	}
	entries, err := os.ReadDir(filepath.Dir(f.path(runID, "x")))
	if err != nil {
		return nil, sidecarErr("missing", err.Error())
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() && ValidSidecarName(e.Name()) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// MemSidecar is the in-memory Sidecar for tests.
type MemSidecar struct {
	mu    sync.Mutex
	files map[string][]byte
}

func (m *MemSidecar) key(runID, name string) string { return runID + "/" + name }

// Write stores b exclusively.
func (m *MemSidecar) Write(runID, name string, b []byte) error {
	if err := checkSidecarArgs(runID, name); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.files == nil {
		m.files = map[string][]byte{}
	}
	if _, ok := m.files[m.key(runID, name)]; ok {
		return sidecarErr("exists", name+" already exists")
	}
	m.files[m.key(runID, name)] = append([]byte(nil), b...)
	return nil
}

// Read returns a copy.
func (m *MemSidecar) Read(runID, name string) ([]byte, error) {
	if err := checkSidecarArgs(runID, name); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[m.key(runID, name)]
	if !ok {
		return nil, sidecarErr("missing", name+" is missing")
	}
	if len(b) > run.MaxPayload {
		return nil, sidecarErr("too_large", name+" exceeds MaxPayload")
	}
	return append([]byte(nil), b...), nil
}

// List names a run's sidecars.
func (m *MemSidecar) List(runID string) ([]string, error) {
	if err := run.ValidateRunID(runID); err != nil {
		return nil, sidecarErr("path", "invalid run id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for k := range m.files {
		if strings.HasPrefix(k, runID+"/") {
			out = append(out, strings.TrimPrefix(k, runID+"/"))
		}
	}
	sort.Strings(out)
	return out, nil
}

// Delete removes a sidecar (tests simulate crashes/edits with it).
func (m *MemSidecar) Delete(runID, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, m.key(runID, name))
}

// Put overwrites a sidecar (tests simulate edits with it).
func (m *MemSidecar) Put(runID, name string, b []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.files == nil {
		m.files = map[string][]byte{}
	}
	m.files[m.key(runID, name)] = append([]byte(nil), b...)
}
