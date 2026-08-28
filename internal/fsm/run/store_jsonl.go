package run

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// jsonlStore is the on-disk RunStore: <root>/.metareview/runs/<id>/audit.jsonl (§5).
type jsonlStore struct {
	root string
	opts Options
	mu   sync.Mutex
	held map[string]*os.File // locks issued by this process
}

// NewJSONLStore returns the on-disk store rooted at the repository root.
func NewJSONLStore(root string, opts Options) RunStore {
	return &jsonlStore{root: root, opts: opts, held: map[string]*os.File{}}
}

func (s *jsonlStore) Root() string { return s.root }

func (s *jsonlStore) MaxEvents() int { return s.opts.maxEvents() }

// TornFiles lists audit.torn-*.bin for a run with their sha256 and size, sorted by name.
func (s *jsonlStore) TornFiles(runID string) ([]TornFile, error) {
	if err := s.validate(runID); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.runDir(runID))
	if err != nil {
		return nil, storeErrf(CodeRunNotFound, 0, runID)
	}
	out := []TornFile{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "audit.torn-") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.runDir(runID), e.Name()))
		if err != nil {
			return nil, pathErr(0, err)
		}
		out = append(out, TornFile{Name: e.Name(), SHA256: LineHash(raw), Bytes: int64(len(raw))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *jsonlStore) runsDir() string { return filepath.Join(s.root, ".metareview", "runs") }

func (s *jsonlStore) runDir(id string) string { return filepath.Join(s.runsDir(), id) }

func (s *jsonlStore) auditPath(id string) string { return filepath.Join(s.runDir(id), "audit.jsonl") }

// pathErr converts an OS error into ERR_STORE_PATH; nil stays nil.
func pathErr(seq int64, err error) error {
	if err == nil {
		return nil
	}
	return storeErrf(CodeStorePath, seq, err.Error())
}

// firstErr returns the first non-nil error.
func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// checkComponents verifies that every path component below root, when present, is a real directory.
func (s *jsonlStore) checkComponents(rel ...string) error {
	cur := s.root
	for _, c := range rel {
		cur = filepath.Join(cur, c)
		fi, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			return nil // remaining components do not exist yet
		}
		if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
			return storeErrf(CodeStorePath, 0, cur+": not a real directory")
		}
	}
	return nil
}

func (s *jsonlStore) validate(id string) error {
	if err := ValidateRunID(id); err != nil {
		return storeErrf(CodeStorePath, 0, err.Error())
	}
	return s.checkComponents(".metareview", "runs", id)
}

// ensureRuns creates .metareview/runs (0700) and its self-ignoring .gitignore (temp + rename).
func (s *jsonlStore) ensureRuns() error {
	err := os.MkdirAll(s.runsDir(), 0o700)
	gi := filepath.Join(s.runsDir(), ".gitignore")
	if cur, rerr := os.ReadFile(gi); err == nil && rerr == nil && string(cur) == "*\n" {
		return nil
	}
	if err == nil {
		err = os.WriteFile(gi+".tmp", []byte("*\n"), 0o600)
	}
	if err == nil {
		err = os.Rename(gi+".tmp", gi)
	}
	return pathErr(0, err)
}

func openNoFollow(path string, flag int, mode os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flag|syscall.O_NOFOLLOW, mode)
}

func fsyncDir(path string) {
	if d, err := os.Open(path); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// writeLine writes line+"\n" and fsyncs; it returns the first error.
func writeLine(f *os.File, line []byte) error {
	_, werr := f.Write(append(line, '\n'))
	return firstErr(werr, f.Sync())
}

func (s *jsonlStore) Create(runID string, first Event) (FoldState, error) {
	if err := s.validate(runID); err != nil {
		return FoldState{}, err
	}
	line, st, err := prepareCreate(runID, first)
	if err != nil {
		return FoldState{}, err
	}
	if err := s.ensureRuns(); err != nil {
		return FoldState{}, err
	}
	dir := s.runDir(runID)
	merr := os.Mkdir(dir, 0o700)
	if os.IsExist(merr) {
		merr = nil
	}
	f, oerr := openNoFollow(s.auditPath(runID), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(oerr) {
		return FoldState{}, storeErrf(CodeRunExists, 0, runID)
	}
	if err := pathErr(1, firstErr(merr, oerr)); err != nil {
		return FoldState{}, err
	}
	werr := writeLine(f, line)
	_ = f.Close()
	fsyncDir(dir)
	fsyncDir(s.runsDir())
	return zeroOnErr(st, werr), pathErr(1, werr)
}

func zeroOnErr(st FoldState, err error) FoldState {
	if err != nil {
		return FoldState{}
	}
	return st
}

// readRaw reads the audit file, refusing symlinked components and a symlinked audit file.
func (s *jsonlStore) readRaw(runID string) ([]byte, error) {
	if err := s.validate(runID); err != nil {
		return nil, err
	}
	f, err := openNoFollow(s.auditPath(runID), os.O_RDONLY, 0)
	if os.IsNotExist(err) {
		return nil, storeErrf(CodeRunNotFound, 0, runID)
	}
	if err != nil {
		return nil, pathErr(0, err)
	}
	// Read-only: a Close error here tells the caller nothing it can act on.
	defer func() { _ = f.Close() }()
	var buf strings.Builder
	chunk := make([]byte, 64<<10)
	for {
		n, rerr := f.Read(chunk)
		buf.Write(chunk[:n])
		if rerr != nil {
			break
		}
	}
	return []byte(buf.String()), nil
}

func (s *jsonlStore) EventsWithLines(runID string) (Log, [][]byte, error) {
	raw, err := s.readRaw(runID)
	if err != nil {
		return Log{}, nil, err
	}
	lines, torn := splitLines(raw)
	log, err := parseLines(lines)
	log.Torn = torn
	return log, lines, err
}

func (s *jsonlStore) Events(runID string) (Log, error) {
	log, _, err := s.EventsWithLines(runID)
	return log, err
}

func (s *jsonlStore) holds(runID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.held[runID]
	return ok
}

func (s *jsonlStore) Append(runID string, st FoldState, ev Event) (FoldState, error) {
	raw, err := s.readRaw(runID) // validates the id and reports a missing run first
	if err != nil {
		return FoldState{}, err
	}
	if !s.holds(runID) {
		return FoldState{}, storeErrf(CodeRunLocked, st.Seq, "lock not held by this process")
	}
	lines, torn := splitLines(raw)
	line, next, err := prepareAppend(lines, torn, st, ev, s.opts.maxEvents())
	if err != nil {
		return FoldState{}, err
	}
	f, err := openNoFollow(s.auditPath(runID), os.O_WRONLY|os.O_APPEND, 0)
	if err == nil {
		err = writeLine(f, line)
		_ = f.Close()
	}
	return zeroOnErr(next, err), pathErr(next.Seq, err)
}

func (s *jsonlStore) RepairTail(runID string) error {
	raw, err := s.readRaw(runID)
	if err != nil {
		return err
	}
	if !s.holds(runID) {
		return storeErrf(CodeRunLocked, 0, "lock not held by this process")
	}
	lines, torn := splitLines(raw)
	if torn == nil {
		return storeErrf(CodeAuditNotTorn, int64(len(lines)), "audit ends with a newline")
	}
	dir := s.runDir(runID)
	nextSeq := int64(len(lines)) + 1
	side := filepath.Join(dir, fmt.Sprintf("audit.torn-%d-%d.bin", nextSeq, time.Now().UnixNano()))
	f, err := openNoFollow(side, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		_, werr := f.Write(torn.Bytes)
		err = firstErr(werr, f.Sync())
		_ = f.Close()
	}
	fsyncDir(dir)
	if err == nil && torn.Offset == 0 {
		err = s.removeNeverCreated(runID, dir, side)
	} else if err == nil {
		err = truncateTo(s.auditPath(runID), torn.Offset)
	}
	return pathErr(nextSeq, err)
}

// removeNeverCreated preserves the torn bytes under runs/.torn and removes a run that never durably
// existed (no complete first line). The lock file is unlinked last.
func (s *jsonlStore) removeNeverCreated(runID, dir, side string) error {
	tornDir := filepath.Join(s.runsDir(), ".torn")
	err := os.MkdirAll(tornDir, 0o700)
	if err == nil {
		err = os.Rename(side, filepath.Join(tornDir, fmt.Sprintf("%s-%d.bin", runID, time.Now().UnixNano())))
	}
	fsyncDir(tornDir)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "lock" {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	_ = os.Remove(filepath.Join(dir, "lock"))
	err = firstErr(err, os.Remove(dir))
	fsyncDir(s.runsDir())
	return err
}

// truncateTo truncates the audit file through a descriptor opened without following symlinks.
func truncateTo(path string, offset int64) error {
	f, err := openNoFollow(path, os.O_WRONLY, 0)
	if err == nil {
		err = firstErr(f.Truncate(offset), f.Sync())
		_ = f.Close()
	}
	return err
}

func (s *jsonlStore) List() ([]RunSummary, error) {
	if err := s.checkComponents(".metareview", "runs"); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.runsDir())
	if os.IsNotExist(err) {
		return []RunSummary{}, nil
	}
	if err != nil {
		return nil, pathErr(0, err)
	}
	list := []RunSummary{}
	for _, e := range entries {
		id := e.Name()
		if !e.IsDir() || ValidateRunID(id) != nil {
			continue
		}
		sidecars := 0
		files, _ := os.ReadDir(s.runDir(id))
		for _, f := range files {
			if strings.HasPrefix(f.Name(), "audit.torn-") {
				sidecars++
			}
		}
		log, err := s.Events(id)
		list = append(list, summarize(id, log, err, sidecars))
	}
	sortSummaries(list)
	return list, nil
}

func (s *jsonlStore) Lock(runID string) (func(), error) {
	if err := s.validate(runID); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(s.auditPath(runID)); err != nil {
		return nil, storeErrf(CodeRunNotFound, 0, runID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.held[runID]; ok {
		return nil, storeErrf(CodeRunLocked, 0, "already held by this process")
	}
	f, err := openNoFollow(filepath.Join(s.runDir(runID), "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err == nil {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	}
	if errors.Is(err, syscall.EWOULDBLOCK) {
		_ = f.Close()
		return nil, storeErrf(CodeRunLocked, 0, "held by another process")
	}
	if err != nil {
		return nil, pathErr(0, err)
	}
	s.held[runID] = f
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if h, ok := s.held[runID]; ok {
			_ = syscall.Flock(int(h.Fd()), syscall.LOCK_UN)
			_ = h.Close()
			delete(s.held, runID)
		}
	}, nil
}
