package run

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// ---- helpers ------------------------------------------------------------------------------------

// storeErr asserts that err is a StoreError carrying code. Most call sites want
// only the assertion, so this returns nothing: a helper that hands back an error
// nobody reads is indistinguishable, to errcheck and to a reader, from a
// discarded failure.
func storeErr(t *testing.T, err error, code string) {
	t.Helper()
	_ = storeErrE(t, err, code)
}

// storeErrE is storeErr for the call sites that go on to inspect the error.
func storeErrE(t *testing.T, err error, code string) *StoreError {
	t.Helper()
	var se *StoreError
	if !errors.As(err, &se) || se.Code != code {
		t.Fatalf("expected StoreError %s, got %v", code, err)
	}
	return se
}

type storeCase struct {
	name string
	mk   func(t *testing.T) RunStore
	disk bool
}

func stores() []storeCase {
	return []storeCase{
		{"jsonl", func(t *testing.T) RunStore { return NewJSONLStore(t.TempDir(), Options{MaxEvents: 5}) }, true},
		{"mem", func(t *testing.T) RunStore { return NewMemStore(Options{MaxEvents: 5}) }, false},
	}
}

// seed creates a run from the first n events of a builder log through the store's own write path.
func seed(t *testing.T, s RunStore, evs []Event) FoldState {
	t.Helper()
	st, err := s.Create(runIDOf(evs[0]), evs[0])
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(evs) == 1 {
		return st
	}
	unlock, err := s.Lock(runIDOf(evs[0]))
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer unlock()
	for _, ev := range evs[1:] {
		ev.Seq, ev.Prev = 0, "" // the store assigns these
		next, err := s.Append(runIDOf(ev), st, ev)
		if err != nil {
			t.Fatalf("append %s: %v", ev.Type, err)
		}
		st = next
	}
	return st
}

func runIDOf(ev Event) string {
	if ev.Type == TypeInit {
		var d InitData
		_ = json.Unmarshal(ev.Data, &d)
		return d.RunID
	}
	return runA
}

// ---- R7: conformance over both stores -----------------------------------------------------------

func TestStoreCreateAndEvents(t *testing.T) {
	for _, sc := range stores() {
		t.Run(sc.name, func(t *testing.T) {
			s := sc.mk(t)
			evs := happyLog().Events()
			st, err := s.Create(runA, evs[0])
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if st.RunID != runA || st.Seq != 1 || st.ChainHead == "" {
				t.Fatalf("create state: %+v", st)
			}
			log, err := s.Events(runA)
			if err != nil || len(log.Events) != 1 || log.Head != st.ChainHead || log.Torn != nil || log.Version != 1 {
				t.Fatalf("events after create: %+v %v", log, err)
			}
			// refusals
			if _, err := s.Create(runA, evs[0]); err == nil {
				t.Fatalf("second create must fail")
			} else {
				storeErr(t, err, CodeRunExists)
			}
			if _, err := s.Create(runB, evs[1]); err == nil {
				t.Fatalf("non-init create must fail")
			} else if se := storeErrE(t, err, CodeAppendRejected); se.Cause == nil {
				t.Fatalf("rejected create must carry the fold error")
			}
			if _, err := s.Create(runB, evs[0]); err == nil {
				t.Fatalf("run id mismatch must fail")
			} else {
				storeErr(t, err, CodeAppendRejected)
			}
			if _, err := s.Create("bad/id", evs[0]); err == nil {
				t.Fatalf("invalid id must fail")
			} else {
				storeErr(t, err, CodeStorePath)
			}
			if _, err := s.Events(runB); err == nil {
				t.Fatalf("missing run must fail")
			} else {
				storeErr(t, err, CodeRunNotFound)
			}
			if _, err := s.Events("bad/id"); err == nil {
				t.Fatalf("invalid id on Events must fail")
			} else {
				storeErr(t, err, CodeStorePath)
			}
			if s.Root() == "" && sc.disk {
				t.Fatalf("Root must be set")
			}
		})
	}
}

func TestStoreAppendContract(t *testing.T) {
	for _, sc := range stores() {
		t.Run(sc.name, func(t *testing.T) {
			s := sc.mk(t)
			evs := happyLog().Events()
			st := seed(t, s, evs[:1])
			ev := evs[1]
			ev.Seq, ev.Prev = 0, ""
			// no lock
			if _, err := s.Append(runA, st, ev); err == nil {
				t.Fatalf("append without lock must fail")
			} else {
				storeErr(t, err, CodeRunLocked)
			}
			unlock, err := s.Lock(runA)
			if err != nil {
				t.Fatalf("lock: %v", err)
			}
			// second lock in the same process fails
			if _, err := s.Lock(runA); err == nil {
				t.Fatalf("second lock must fail")
			} else {
				storeErr(t, err, CodeRunLocked)
			}
			// invalid id
			if _, err := s.Append("bad/id", st, ev); err == nil {
				t.Fatalf("invalid id must fail")
			} else {
				storeErr(t, err, CodeStorePath)
			}
			// CAS: wrong head, wrong seq
			bad := st
			bad.ChainHead = "0000"
			if _, err := s.Append(runA, bad, ev); err == nil {
				t.Fatalf("stale head must fail")
			} else {
				storeErr(t, err, CodeAuditCAS)
			}
			bad = st
			bad.Seq = 7
			if _, err := s.Append(runA, bad, ev); err == nil {
				t.Fatalf("stale seq must fail")
			} else {
				storeErr(t, err, CodeAuditCAS)
			}
			// rejected by Apply: nothing written, state unchanged
			before, _ := s.Events(runA)
			badEv := ev
			badEv.Iter = 9
			if _, err := s.Append(runA, st, badEv); err == nil {
				t.Fatalf("invalid event must be rejected")
			} else if se := storeErrE(t, err, CodeAppendRejected); se.Cause.(*FoldError).Reason != ReasonStamp {
				t.Fatalf("cause: %v", se.Cause)
			}
			after, _ := s.Events(runA)
			if len(after.Events) != len(before.Events) || after.Head != before.Head {
				t.Fatalf("rejected append changed the log")
			}
			// good append: Seq/Prev assigned, Data canonical, head advanced
			ev.Data = json.RawMessage("{ \"head\" : \"head0000\" , \"tree_hash\" : \"t0\", \"status\": \"<\" }")
			next, err := s.Append(runA, st, ev)
			if err != nil {
				t.Fatalf("append: %v", err)
			}
			log, _ := s.Events(runA)
			got := log.Events[1]
			if got.Seq != 2 || got.Prev != st.ChainHead || string(got.Data) != `{"head":"head0000","tree_hash":"t0","status":"<"}` || next.Seq != 2 || next.ChainHead != log.Head || next.TreeStatus != "<" {
				t.Fatalf("append result: %+v next=%+v head=%s", got, next, log.Head)
			}
			// duplicate keys at depth 3 rejected before write
			dup := ev
			dup.Data = json.RawMessage(`{"head":"h","tree_hash":{"a":{"b":1,"b":2}}}`)
			if _, err := s.Append(runA, next, dup); err == nil {
				t.Fatalf("duplicate keys must be rejected")
			} else {
				storeErr(t, err, CodeAppendRejected)
			}
			// too large
			huge := Event{SchemaVersion: 1, At: Time{time.Now()}, Type: TypeRecord, State: "discover", Data: json.RawMessage(`{"name":"r","data":"` + strings.Repeat("x", MaxPayload) + `"}`)}
			if _, err := s.Append(runA, next, huge); err == nil {
				t.Fatalf("oversize must be rejected")
			} else {
				storeErr(t, err, CodeAppendRejected) // caught by Apply's payload cap before MaxLine
			}
			// MaxEvents (5): counted types only; exempt types still append
			cur := next
			for i := 0; i < 4; i++ { // tree + 4 records = 5 counted events
				rec := Event{SchemaVersion: 1, At: Time{time.Now()}, Type: TypeRecord, State: "discover", Data: json.RawMessage(`{"name":"r","data":{}}`)}
				cur, err = s.Append(runA, cur, rec)
				if err != nil {
					t.Fatalf("record %d: %v", i, err)
				}
			}
			rec := Event{SchemaVersion: 1, At: Time{time.Now()}, Type: TypeRecord, State: "discover", Data: json.RawMessage(`{"name":"r","data":{}}`)}
			if _, err := s.Append(runA, cur, rec); err == nil {
				t.Fatalf("MaxEvents must refuse a counted event")
			} else {
				storeErr(t, err, CodeAuditFull)
			}
			warn := Event{SchemaVersion: 1, At: Time{time.Now()}, Type: TypeWarn, State: "discover", Data: json.RawMessage(`{"code":"W"}`)}
			if cur, err = s.Append(runA, cur, warn); err != nil {
				t.Fatalf("exempt type must still append: %v", err)
			}
			unlock()
			// lock released: a fresh lock succeeds
			unlock2, err := s.Lock(runA)
			if err != nil {
				t.Fatalf("relock: %v", err)
			}
			unlock2()
			// missing run
			if _, err := s.Lock(runB); err == nil {
				t.Fatalf("lock on missing run must fail")
			} else {
				storeErr(t, err, CodeRunNotFound)
			}
			if _, err := s.Append(runB, cur, warn); err == nil {
				t.Fatalf("append to missing run must fail")
			} else {
				storeErr(t, err, CodeRunNotFound)
			}
			// EventsWithLines exposes raw lines matching LineHash
			log, lines, err := s.EventsWithLines(runA)
			if err != nil || len(lines) != len(log.Events) || LineHash(lines[len(lines)-1]) != log.Head {
				t.Fatalf("EventsWithLines: %v", err)
			}
		})
	}
}

func TestStoreFoldEquivalence(t *testing.T) {
	evs := happyLog().Events()
	var snaps [][]byte
	for _, sc := range stores() {
		// raise MaxEvents for the full log
		var s RunStore
		if sc.disk {
			s = NewJSONLStore(t.TempDir(), Options{})
		} else {
			s = NewMemStore(Options{})
		}
		st := seed(t, s, evs)
		log, err := s.Events(runA)
		if err != nil {
			t.Fatalf("%s events: %v", sc.name, err)
		}
		snap := mustFold(t, log.Events)
		if !SnapshotEqualIgnoringSeq(snap, st.Snapshot) || snap.Seq != st.Seq {
			t.Fatalf("%s: Append state disagrees with Fold of the stored log", sc.name)
		}
		snaps = append(snaps, marshalCanonical(snap))
	}
	if string(snaps[0]) != string(snaps[1]) {
		t.Fatalf("stores disagree")
	}
}

// ---- JSONL-only rows ----------------------------------------------------------------------------

func TestJSONLLayout(t *testing.T) {
	root := t.TempDir()
	s := NewJSONLStore(root, Options{})
	evs := happyLog().Events()
	seed(t, s, evs[:1])
	runs := filepath.Join(root, ".metareview", "runs")
	for _, p := range []struct {
		path string
		mode os.FileMode
	}{
		{runs, 0o700}, {filepath.Join(runs, runA), 0o700},
		{filepath.Join(runs, runA, "audit.jsonl"), 0o600}, {filepath.Join(runs, ".gitignore"), 0o600},
	} {
		fi, err := os.Stat(p.path)
		if err != nil || fi.Mode().Perm() != p.mode {
			t.Fatalf("%s: %v %v", p.path, fi, err)
		}
	}
	gi, _ := os.ReadFile(filepath.Join(runs, ".gitignore"))
	if string(gi) != "*\n" {
		t.Fatalf(".gitignore = %q", gi)
	}
	// re-ensured on the next Create even if edited
	_ = os.WriteFile(filepath.Join(runs, ".gitignore"), []byte("edited"), 0o600)
	b := NewBuilder(runB)
	d := baseInit()
	d.RunID = runB
	b.Init(d)
	seed(t, s, b.Events())
	gi, _ = os.ReadFile(filepath.Join(runs, ".gitignore"))
	if string(gi) != "*\n" {
		t.Fatalf(".gitignore not re-ensured: %q", gi)
	}
	// stored line is the canonical encoding and ends with a newline
	raw, _ := os.ReadFile(filepath.Join(runs, runA, "audit.jsonl"))
	if !strings.HasSuffix(string(raw), "\n") || strings.Contains(string(raw), "\\u003c") || strings.Count(string(raw), "\n") != 1 {
		t.Fatalf("audit bytes: %q", raw)
	}
	if s.Root() != root {
		t.Fatalf("root")
	}
	// symlinked component → ERR_STORE_PATH
	evil := t.TempDir()
	linkRoot := t.TempDir()
	_ = os.MkdirAll(filepath.Join(linkRoot, ".metareview"), 0o700)
	_ = os.Symlink(evil, filepath.Join(linkRoot, ".metareview", "runs"))
	s2 := NewJSONLStore(linkRoot, Options{})
	if _, err := s2.Create(runA, evs[0]); err == nil {
		t.Fatalf("symlinked runs/ must be refused")
	} else {
		storeErr(t, err, CodeStorePath)
	}
	// a symlinked run dir is refused on read too
	_ = os.Remove(filepath.Join(linkRoot, ".metareview", "runs"))
	_ = os.MkdirAll(filepath.Join(linkRoot, ".metareview", "runs"), 0o700)
	_ = os.Symlink(filepath.Join(root, ".metareview", "runs", runA), filepath.Join(linkRoot, ".metareview", "runs", runA))
	if _, err := s2.Events(runA); err == nil {
		t.Fatalf("symlinked run dir must be refused")
	} else {
		storeErr(t, err, CodeStorePath)
	}
	_ = os.Remove(filepath.Join(linkRoot, ".metareview", "runs", runA))
	_ = os.Remove(filepath.Join(linkRoot, ".metareview", "runs"))
	_ = os.Symlink(evil, filepath.Join(linkRoot, ".metareview", "runs"))
	if _, err := s2.List(); err == nil {
		t.Fatalf("symlinked runs/ must be refused by List")
	} else {
		storeErr(t, err, CodeStorePath)
	}
	// unreadable root: Create fails with ERR_STORE_PATH
	bad := NewJSONLStore(filepath.Join(root, "nope", "file.txt"), Options{})
	_ = os.WriteFile(filepath.Join(root, "nope"), []byte("x"), 0o600)
	if _, err := bad.Create(runA, evs[0]); err == nil {
		t.Fatalf("create under a file must fail")
	} else {
		storeErr(t, err, CodeStorePath)
	}
}

func TestJSONLChainVerification(t *testing.T) {
	root := t.TempDir()
	s := NewJSONLStore(root, Options{})
	evs := happyLog().Events()
	seed(t, s, evs[:4])
	path := filepath.Join(root, ".metareview", "runs", runA, "audit.jsonl")
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	// corrupt line 2 by one byte (inside the payload) → chain break at seq 3
	lines2 := append([]string{}, lines...)
	lines2[1] = strings.Replace(lines2[1], `"t0"`, `"t1"`, 1)
	_ = os.WriteFile(path, []byte(strings.Join(lines2, "\n")+"\n"), 0o600)
	if _, err := s.Events(runA); err == nil {
		t.Fatalf("corrupted line must break the chain")
	} else if se := storeErrE(t, err, CodeAuditChain); se.Seq != 3 {
		t.Fatalf("chain break seq = %d", se.Seq)
	}
	// undecodable complete line
	lines3 := append([]string{}, lines...)
	lines3[2] = "not json"
	_ = os.WriteFile(path, []byte(strings.Join(lines3, "\n")+"\n"), 0o600)
	if _, err := s.Events(runA); err == nil {
		t.Fatalf("undecodable line must fail")
	} else if se := storeErrE(t, err, CodeAuditChain); se.Seq != 3 || se.Detail != "undecodable" {
		t.Fatalf("undecodable: %+v", se)
	}
	// seq gap in stored lines
	lines4 := append([]string{}, lines...)
	lines4[2] = strings.Replace(lines4[2], `"seq":3`, `"seq":4`, 1)
	_ = os.WriteFile(path, []byte(strings.Join(lines4, "\n")+"\n"), 0o600)
	if _, err := s.Events(runA); err == nil {
		t.Fatalf("seq gap must fail")
	} else {
		storeErr(t, err, CodeAuditChain)
	}
	// restore and verify clean
	_ = os.WriteFile(path, raw, 0o600)
	if _, err := s.Events(runA); err != nil {
		t.Fatalf("restored log: %v", err)
	}
	// a chain-valid but fold-invalid log: Events returns it, Fold rejects it
	nb := NewBuilder(runA)
	nb.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}) // seq 1, prev "", not an init
	_ = os.WriteFile(path, append(marshalCanonical(nb.Events()[0]), '\n'), 0o600)
	log, err := s.Events(runA)
	if err != nil {
		t.Fatalf("events must not fold: %v", err)
	}
	if _, err := Fold(log.Events); err == nil {
		t.Fatalf("fold must reject")
	}
}

// ---- R8: torn tail ------------------------------------------------------------------------------

func TestJSONLTornTail(t *testing.T) {
	root := t.TempDir()
	s := NewJSONLStore(root, Options{})
	evs := happyLog().Events()
	st := seed(t, s, evs[:3])
	path := filepath.Join(root, ".metareview", "runs", runA, "audit.jsonl")
	tails := []struct {
		name string
		tail string
	}{
		{"mid-line", `{"schemaVersion":1,"seq":4,"pr`},
		{"complete-no-newline", `{"schemaVersion":1,"seq":4,"prev":"x","at":"2026-08-26T00:00:04Z","type":"warn","state":"discover","iter":0,"data":{"code":"W"}}`},
		{"valid-prefix", `{"schemaVersion":1,"seq":4}`},
	}
	for _, tc := range tails {
		name := tc.name
		clean, _ := os.ReadFile(path)
		nlines := strings.Count(string(clean), "\n")
		content := append(append([]byte{}, clean...), []byte(tc.tail)...)
		_ = os.WriteFile(path, content, 0o600)
		log, err := s.Events(runA)
		if err != nil || log.Torn == nil || len(log.Events) != nlines || log.Torn.Offset != int64(len(clean)) || string(log.Torn.Bytes) != string(content[len(clean):]) {
			t.Fatalf("%s: torn detection failed: %+v %v", name, log.Torn, err)
		}
		after, _ := os.ReadFile(path)
		if string(after) != string(content) {
			t.Fatalf("%s: Events must not modify the file", name)
		}
		unlock, _ := s.Lock(runA)
		warn := Event{SchemaVersion: 1, At: Time{time.Now()}, Type: TypeWarn, State: "discover", Data: json.RawMessage(`{"code":"W"}`)}
		if _, err := s.Append(runA, st, warn); err == nil {
			t.Fatalf("%s: append onto a torn log must fail", name)
		} else {
			storeErr(t, err, CodeAuditTorn)
		}
		// repair requires the lock
		unlock()
		if err := s.RepairTail(runA); err == nil {
			t.Fatalf("%s: repair without lock must fail", name)
		} else {
			storeErr(t, err, CodeRunLocked)
		}
		unlock, _ = s.Lock(runA)
		if err := s.RepairTail(runA); err != nil {
			t.Fatalf("%s: repair: %v", name, err)
		}
		repaired, _ := os.ReadFile(path)
		if string(repaired) != string(clean) {
			t.Fatalf("%s: repair must truncate to the clean prefix", name)
		}
		matches, _ := filepath.Glob(filepath.Join(root, ".metareview", "runs", runA, "audit.torn-*.bin"))
		if len(matches) == 0 {
			t.Fatalf("%s: sidecar missing", name)
		}
		sort.Strings(matches)
		var side []byte
		for _, m := range matches { // the newest sidecar carries this iteration's tail
			if b, _ := os.ReadFile(m); string(b) == tc.tail {
				side = b
			}
		}
		if string(side) != string(content[len(clean):]) {
			t.Fatalf("%s: sidecar bytes", name)
		}
		fi, _ := os.Stat(matches[0])
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("sidecar mode")
		}
		// not torn any more
		if err := s.RepairTail(runA); err == nil {
			t.Fatalf("%s: repair of a clean log must fail", name)
		} else {
			storeErr(t, err, CodeAuditNotTorn)
		}
		// the machine's warn now appends fine and the log stays foldable
		if next, err := s.Append(runA, st, warn); err != nil {
			t.Fatalf("%s: append after repair: %v", name, err)
		} else {
			st = next
		}
		log, _ = s.Events(runA)
		if _, err := Fold(log.Events); err != nil {
			t.Fatalf("%s: fold after repair: %v", name, err)
		}
		unlock()
	}
	// sidecars are counted and multiple repairs get distinct names
	sum, err := s.List()
	if err != nil || len(sum) != 1 || sum[0].Sidecars != 3 || sum[0].Torn {
		t.Fatalf("list after repairs: %+v %v", sum, err)
	}
	// offset 0: the run never existed → directory removed, bytes preserved under .torn/
	_ = os.WriteFile(path, []byte(`{"schemaVersion":1,"seq":1,"pr`), 0o600)
	unlock, _ := s.Lock(runA)
	if err := s.RepairTail(runA); err != nil {
		t.Fatalf("offset-0 repair: %v", err)
	}
	unlock()
	if _, err := os.Stat(filepath.Join(root, ".metareview", "runs", runA)); !os.IsNotExist(err) {
		t.Fatalf("run dir must be removed")
	}
	preserved, _ := filepath.Glob(filepath.Join(root, ".metareview", "runs", ".torn", runA+"-*.bin"))
	if len(preserved) != 1 {
		t.Fatalf("offset-0 bytes must be preserved: %v", preserved)
	}
	// RepairTail on a missing run / invalid id
	if err := s.RepairTail(runA); err == nil {
		t.Fatalf("repair of a removed run must fail")
	} else {
		storeErr(t, err, CodeRunNotFound)
	}
	if err := s.RepairTail("bad/id"); err == nil {
		t.Fatalf("invalid id")
	} else {
		storeErr(t, err, CodeStorePath)
	}
	// mem store: never torn
	m := NewMemStore(Options{})
	seed(t, m, evs[:1])
	unlock, _ = m.Lock(runA)
	if err := m.RepairTail(runA); err == nil {
		t.Fatalf("mem repair must report not torn")
	} else {
		storeErr(t, err, CodeAuditNotTorn)
	}
	unlock()
	if err := m.RepairTail(runA); err == nil {
		t.Fatalf("mem repair without lock")
	} else {
		storeErr(t, err, CodeRunLocked)
	}
}

// ---- R9: listing --------------------------------------------------------------------------------

func TestList(t *testing.T) {
	for _, sc := range stores() {
		t.Run(sc.name, func(t *testing.T) {
			var s RunStore
			if sc.disk {
				s = NewJSONLStore(t.TempDir(), Options{})
			} else {
				s = NewMemStore(Options{})
			}
			empty, err := s.List()
			if err != nil || len(empty) != 0 {
				t.Fatalf("empty list: %v %v", empty, err)
			}
			// two runs, created out of order; the later CreatedAt sorts first; ties by id
			b2 := NewBuilder(runB)
			d := baseInit()
			d.RunID = runB
			d.CreatedAt = Time{time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)}
			b2.Init(d)
			seed(t, s, b2.Events())
			seed(t, s, happyLog().Events())
			list, err := s.List()
			if err != nil || len(list) != 2 || list[0].RunID != runB || list[1].RunID != runA {
				t.Fatalf("order: %+v %v", list, err)
			}
			if list[1].State != "done" || list[1].Outcome != OutcomeFixed || list[1].Workflow != "sdlc-loop" || list[1].Mock != "" || list[1].MockTainted || list[1].Torn || list[1].Error != "" {
				t.Fatalf("summary: %+v", list[1])
			}
			if !sc.disk {
				return
			}
			root := s.Root()
			runs := filepath.Join(root, ".metareview", "runs")
			// non-conforming names skipped; corrupt run yields an Error row; torn run flagged
			_ = os.MkdirAll(filepath.Join(runs, "not-a-run"), 0o700)
			_ = os.WriteFile(filepath.Join(runs, runB, "audit.jsonl"), []byte("garbage\n"), 0o600)
			p := filepath.Join(runs, runA, "audit.jsonl")
			raw, _ := os.ReadFile(p)
			_ = os.WriteFile(p, append(raw, []byte(`{"torn`)...), 0o600)
			list, err = s.List()
			if err != nil || len(list) != 2 {
				t.Fatalf("list with corruption: %+v %v", list, err)
			}
			for _, r := range list {
				switch r.RunID {
				case runA:
					if !r.Torn || r.State != "done" {
						t.Fatalf("torn summary: %+v", r)
					}
				case runB:
					if r.Error == "" {
						t.Fatalf("corrupt run must carry Error: %+v", r)
					}
				}
			}
			// a foldable-invalid run (chain ok, fold rejects)
			nb := NewBuilder(runB)
			nb.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"})
			_ = os.WriteFile(filepath.Join(runs, runB, "audit.jsonl"), append(marshalCanonical(nb.Events()[0]), '\n'), 0o600)
			list, _ = s.List()
			for _, r := range list {
				if r.RunID == runB && !strings.Contains(r.Error, "first_not_init") {
					t.Fatalf("fold error must surface: %+v", r)
				}
			}
			// missing runs/ dir → empty
			_ = os.RemoveAll(runs)
			list, err = s.List()
			if err != nil || len(list) != 0 {
				t.Fatalf("missing runs dir: %v %v", list, err)
			}
		})
	}
}

func TestJSONLLockContentionAndPathErrors(t *testing.T) {
	root := t.TempDir()
	s1 := NewJSONLStore(root, Options{})
	s2 := NewJSONLStore(root, Options{}) // a second holder, as another process would be
	evs := happyLog().Events()
	seed(t, s1, evs[:1])
	unlock, err := s1.Lock(runA)
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	if _, err := s2.Lock(runA); err == nil {
		t.Fatalf("flock contention must fail")
	} else if se := storeErrE(t, err, CodeRunLocked); !strings.Contains(se.Detail, "another") {
		t.Fatalf("detail: %s", se.Detail)
	}
	unlock()
	unlock2, err := s2.Lock(runA)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	unlock2()
	// a symlinked audit file is refused (O_NOFOLLOW)
	dir := filepath.Join(root, ".metareview", "runs", runA)
	real := filepath.Join(dir, "audit.jsonl")
	_ = os.Rename(real, real+".real")
	_ = os.Symlink(real+".real", real)
	if _, err := s1.Events(runA); err == nil {
		t.Fatalf("symlinked audit must be refused")
	} else {
		storeErr(t, err, CodeStorePath)
	}
	_ = os.Remove(real)
	_ = os.Rename(real+".real", real)
	// permission failures surface as ERR_STORE_PATH (skipped as root, which ignores modes)
	runs := filepath.Join(root, ".metareview", "runs")
	if os.Geteuid() != 0 {
		_ = os.Chmod(runs, 0o000)
		if _, err := s1.List(); err == nil {
			t.Fatalf("unreadable runs dir must fail List")
		} else {
			storeErr(t, err, CodeStorePath)
		}
		b := NewBuilder(runB)
		d := baseInit()
		d.RunID = runB
		b.Init(d)
		// runs/ readable but not writable with a wrong .gitignore: re-ensuring it fails
		_ = os.Chmod(runs, 0o700)
		_ = os.WriteFile(filepath.Join(runs, ".gitignore"), []byte("edited"), 0o600)
		_ = os.Chmod(runs, 0o500)
		if _, err := s1.Create(runB, b.Events()[0]); err == nil {
			t.Fatalf("unwritable runs dir with a wrong .gitignore must fail Create")
		} else {
			storeErr(t, err, CodeStorePath)
		}
		_ = os.Chmod(runs, 0o700)
		_ = os.WriteFile(filepath.Join(runs, ".gitignore"), []byte("*\n"), 0o600)
		// runs/ readable but not writable: .gitignore is already right, so Mkdir of the run dir fails
		_ = os.Chmod(runs, 0o500)
		if _, err := s1.Create(runB, b.Events()[0]); err == nil {
			t.Fatalf("unwritable runs dir must fail Create")
		} else {
			storeErr(t, err, CodeStorePath)
		}
		// runs/ unreadable: the .gitignore cannot be re-ensured
		_ = os.Chmod(runs, 0o000)
		if _, err := s1.Create(runB, b.Events()[0]); err == nil {
			t.Fatalf("unreadable runs dir must fail Create")
		} else {
			storeErr(t, err, CodeStorePath)
		}
		_ = os.Chmod(runs, 0o700)
		// a run dir that cannot take a lock file
		seed(t, s1, b.Events())
		_ = os.Chmod(filepath.Join(runs, runB), 0o500)
		if _, err := s1.Lock(runB); err == nil {
			t.Fatalf("lock file creation must fail in a read-only run dir")
		} else {
			storeErr(t, err, CodeStorePath)
		}
		_ = os.Chmod(filepath.Join(runs, runB), 0o700)
	}
	if _, err := s1.Lock("bad/id"); err == nil {
		t.Fatalf("invalid id on Lock")
	} else {
		storeErr(t, err, CodeStorePath)
	}
	// mem store: repair/lock on a missing run
	m := NewMemStore(Options{})
	if err := m.RepairTail(runA); err == nil {
		t.Fatalf("mem repair on missing run")
	} else {
		storeErr(t, err, CodeRunNotFound)
	}
}

func TestStoreHelpers(t *testing.T) {
	e1, e2 := errors.New("one"), errors.New("two")
	if firstErr(nil, nil) != nil || firstErr(nil, e1, e2) != e1 || firstErr(e2) != e2 {
		t.Fatalf("firstErr")
	}
	st := FoldState{ChainHead: "h"}
	if zeroOnErr(st, nil).ChainHead != "h" || zeroOnErr(st, e1).ChainHead != "" {
		t.Fatalf("zeroOnErr")
	}
	if pathErr(0, nil) != nil {
		t.Fatalf("pathErr(nil)")
	}
	if (Options{}).maxEvents() != DefaultMaxEvents || (Options{MaxEvents: 3}).maxEvents() != 3 {
		t.Fatalf("maxEvents")
	}
}

func TestListTieOrder(t *testing.T) {
	m := NewMemStore(Options{})
	at := Time{time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)}
	for _, id := range []string{runB, runA} {
		b := NewBuilder(id)
		d := baseInit()
		d.RunID = id
		d.CreatedAt = at
		b.Init(d)
		seed(t, m, b.Events())
	}
	list, _ := m.List()
	if len(list) != 2 || list[0].RunID != runA || list[1].RunID != runB {
		t.Fatalf("tie order: %+v", list)
	}
}

func TestBuilderHelpers(t *testing.T) {
	b := NewBuilder(runA)
	b.Init(baseInit())
	b.Event(TypeTree, TreeData{Head: "h", TreeHash: "t"}, WithPrev("bogus"))
	// Fold never verifies Prev (that is the store's job); the log still folds
	mustFold(t, b.Events())
	lines := b.Lines()
	if len(lines) != 2 || LineHash(lines[0]) == "" || b.Events()[1].Prev != "bogus" {
		t.Fatalf("lines/prev: %d %q", len(lines), b.Events()[1].Prev)
	}
	// but the store's chain verification rejects it
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".metareview", "runs", runA), 0o700)
	var raw []byte
	for _, l := range lines {
		raw = append(append(raw, l...), '\n')
	}
	_ = os.WriteFile(filepath.Join(root, ".metareview", "runs", runA, "audit.jsonl"), raw, 0o600)
	if _, err := NewJSONLStore(root, Options{}).Events(runA); err == nil {
		t.Fatalf("bogus prev must break the chain")
	} else {
		storeErr(t, err, CodeAuditChain)
	}
}

// ---- R2b: external oracle -----------------------------------------------------------------------

func TestOracle(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".metareview", "runs", runA)
	_ = os.MkdirAll(dir, 0o700)
	oracle, err := os.ReadFile("testdata/oracle.jsonl")
	if err != nil {
		t.Fatalf("oracle fixture: %v", err)
	}
	_ = os.WriteFile(filepath.Join(dir, "audit.jsonl"), oracle, 0o600)
	s := NewJSONLStore(root, Options{})
	log, lines, err := s.EventsWithLines(runA)
	if err != nil {
		t.Fatalf("oracle must be accepted: %v", err)
	}
	hashes, _ := os.ReadFile("testdata/oracle.sha256")
	want := strings.Fields(string(hashes))
	if len(want) != len(lines) {
		t.Fatalf("oracle.sha256 has %d hashes for %d lines", len(want), len(lines))
	}
	for i, l := range lines {
		if LineHash(l) != want[i] {
			t.Fatalf("line %d: LineHash %s != oracle %s", i+1, LineHash(l), want[i])
		}
		if i > 0 && log.Events[i].Prev != want[i-1] {
			t.Fatalf("line %d prev mismatch", i+1)
		}
		canon, err := Canonical(l)
		if err != nil || string(canon) != string(l) {
			t.Fatalf("line %d is not canonical: %v", i+1, err)
		}
	}
	if _, err := Fold(log.Events); err != nil {
		t.Fatalf("oracle must fold: %v", err)
	}
	// the oracle carries <>& and U+2028 in a payload; Canonical of the non-canonical source equals the stored form
	src, _ := os.ReadFile("testdata/oracle.events.jsonl")
	srcLines := strings.Split(strings.TrimSuffix(string(src), "\n"), "\n")
	for i, sl := range srcLines {
		var ev Event
		if err := json.Unmarshal([]byte(sl), &ev); err != nil {
			t.Fatalf("source line %d: %v", i+1, err)
		}
		ev.Seq = int64(i + 1)
		if i > 0 {
			ev.Prev = want[i-1]
		}
		ev.Data, _ = Canonical(ev.Data)
		if string(marshalCanonical(ev)) != string(lines[i]) {
			t.Fatalf("re-encoding source line %d does not reproduce the oracle:\n%s\n%s", i+1, marshalCanonical(ev), lines[i])
		}
	}
	// one-byte edit breaks the chain at the right seq
	edited := strings.Replace(string(oracle), "tree_hash\":\"t0", "tree_hash\":\"t9", 1)
	_ = os.WriteFile(filepath.Join(dir, "audit.jsonl"), []byte(edited), 0o600)
	if _, err := s.Events(runA); err == nil {
		t.Fatalf("edited oracle must fail")
	} else if se := storeErrE(t, err, CodeAuditChain); se.Seq != 3 {
		t.Fatalf("edit seq: %+v", se)
	}
}

// ---- spec 3 r5 owned amendments -----------------------------------------------------------------

func TestCountedAndMaxEvents(t *testing.T) {
	for _, typ := range []string{TypeNeedsInput, TypeNodeOutput, TypeDeltaApplied, TypeLLMCall, TypeTokens, TypeRecord, TypeTree} {
		if !Counted(typ) {
			t.Fatalf("%s must be counted", typ)
		}
	}
	for _, typ := range []string{TypeInit, TypeTransition, TypeGate, TypeCmdCall, TypeConverge, TypeFork, TypeFixBaseline, TypeWarn, TypeOverflowHandler} {
		if Counted(typ) {
			t.Fatalf("%s must not be counted", typ)
		}
	}
	if NewMemStore(Options{MaxEvents: 5}).MaxEvents() != 5 || NewJSONLStore(t.TempDir(), Options{MaxEvents: 7}).MaxEvents() != 7 {
		t.Fatal("MaxEvents must echo Options")
	}
	if NewMemStore(Options{}).MaxEvents() != DefaultMaxEvents || NewJSONLStore(t.TempDir(), Options{}).MaxEvents() != DefaultMaxEvents {
		t.Fatal("zero → DefaultMaxEvents")
	}
}

func TestTornFiles(t *testing.T) {
	mem := NewMemStore(Options{})
	seed(t, mem, happyLog().Events())
	if files, err := mem.TornFiles(runA); err != nil || len(files) != 0 {
		t.Fatalf("mem store has no torn files: %v %v", files, err)
	}
	if _, err := mem.TornFiles("../x"); err == nil {
		t.Fatal("invalid id must be refused")
	}
	if _, err := mem.TornFiles(runB); err == nil {
		t.Fatal("unknown run must be refused")
	}
	s := NewJSONLStore(t.TempDir(), Options{})
	seed(t, s, happyLog().Events())
	dir := filepath.Join(s.Root(), ".metareview", "runs", runA)
	if files, err := s.TornFiles(runA); err != nil || len(files) != 0 {
		t.Fatalf("no torn files yet: %v %v", files, err)
	}
	_ = os.WriteFile(filepath.Join(dir, "audit.torn-9-2.bin"), []byte("zz"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "audit.torn-9-1.bin"), []byte("{\"torn"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600)
	files, err := s.TornFiles(runA)
	if err != nil || len(files) != 2 {
		t.Fatalf("torn files: %+v %v", files, err)
	}
	want := TornFile{Name: "audit.torn-9-1.bin", SHA256: LineHash([]byte("{\"torn")), Bytes: 6}
	if files[0] != want || files[1].Name != "audit.torn-9-2.bin" || files[1].Bytes != 2 {
		t.Fatalf("literal: %+v", files)
	}
	if _, err := s.TornFiles("../x"); err == nil {
		t.Fatal("invalid id must be refused")
	}
	if _, err := s.TornFiles(runB); err == nil {
		t.Fatal("unknown run must be refused")
	}
	_ = os.Chmod(filepath.Join(dir, "audit.torn-9-2.bin"), 0)
	if _, err := s.TornFiles(runA); err == nil && os.Getuid() != 0 {
		t.Fatal("unreadable torn file must error")
	}
}

func TestSummarizeIncompleteFork(t *testing.T) {
	parent := happyLog().Events()
	mk := func(to int64, kind Kind, extra ...string) Log {
		b := NewBuilder(runB)
		evs := b.Copy(parent, 2, to, runB, func(d *InitData) { d.ForkedAtSeq = to })
		_ = kind
		return Log{Events: evs}
	}
	// no rebaseline tree: Seq == ForkedAtSeq → incomplete
	if s := summarize(runB, mk(3, ""), nil, 0); !strings.Contains(s.Error, "incomplete fork") {
		t.Fatalf("expected incomplete fork, got %+v", s)
	}
	// a root run is never incomplete
	if s := summarize(runA, Log{Events: parent[:3]}, nil, 0); s.Error != "" {
		t.Fatalf("root must not be flagged: %+v", s)
	}
}
