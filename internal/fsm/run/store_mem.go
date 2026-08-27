package run

import "sync"

// memRun holds one run's serialized lines; MemStore behaves exactly like the JSONL store over them.
type memRun struct {
	lines  [][]byte
	locked bool
}

// memStore is the in-memory RunStore used by tests and dry runs.
type memStore struct {
	opts Options
	mu   sync.Mutex
	runs map[string]*memRun
}

// NewMemStore returns an in-memory store with the same contract as the JSONL store.
func NewMemStore(opts Options) RunStore {
	return &memStore{opts: opts, runs: map[string]*memRun{}}
}

func (s *memStore) Root() string { return "" }

func (s *memStore) get(runID string) (*memRun, error) {
	if err := ValidateRunID(runID); err != nil {
		return nil, storeErrf(CodeStorePath, 0, err.Error())
	}
	r, ok := s.runs[runID]
	if !ok {
		return nil, storeErrf(CodeRunNotFound, 0, runID)
	}
	return r, nil
}

func (s *memStore) Create(runID string, first Event) (FoldState, error) {
	if err := ValidateRunID(runID); err != nil {
		return FoldState{}, storeErrf(CodeStorePath, 0, err.Error())
	}
	line, st, err := prepareCreate(runID, first)
	if err != nil {
		return FoldState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[runID]; exists {
		return FoldState{}, storeErrf(CodeRunExists, 0, runID)
	}
	s.runs[runID] = &memRun{lines: [][]byte{line}}
	return st, nil
}

func (s *memStore) Append(runID string, st FoldState, ev Event) (FoldState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.get(runID)
	if err != nil {
		return FoldState{}, err
	}
	if !r.locked {
		return FoldState{}, storeErrf(CodeRunLocked, st.Seq, "lock not held by this process")
	}
	line, next, err := prepareAppend(r.lines, nil, st, ev, s.opts.maxEvents())
	if err != nil {
		return FoldState{}, err
	}
	r.lines = append(r.lines, line)
	return next, nil
}

func (s *memStore) EventsWithLines(runID string) (Log, [][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.get(runID)
	if err != nil {
		return Log{}, nil, err
	}
	lines := make([][]byte, len(r.lines))
	for i, l := range r.lines {
		lines[i] = append([]byte{}, l...)
	}
	log, err := parseLines(lines) // lines came through Create/Append, so this cannot fail
	return log, lines, err
}

func (s *memStore) Events(runID string) (Log, error) {
	log, _, err := s.EventsWithLines(runID)
	return log, err
}

func (s *memStore) RepairTail(runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.get(runID)
	if err != nil {
		return err
	}
	if !r.locked {
		return storeErrf(CodeRunLocked, 0, "lock not held by this process")
	}
	return storeErrf(CodeAuditNotTorn, int64(len(r.lines)), "memory store is never torn")
}

func (s *memStore) List() ([]RunSummary, error) {
	s.mu.Lock()
	ids := make([]string, 0, len(s.runs))
	for id := range s.runs {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	list := []RunSummary{}
	for _, id := range ids {
		log, err := s.Events(id)
		list = append(list, summarize(id, log, err, 0))
	}
	sortSummaries(list)
	return list, nil
}

func (s *memStore) Lock(runID string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.get(runID)
	if err != nil {
		return nil, err
	}
	if r.locked {
		return nil, storeErrf(CodeRunLocked, 0, "already held")
	}
	r.locked = true
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		r.locked = false
	}, nil
}
