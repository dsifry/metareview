package run

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/dsifry/metareview/internal/jsonl"
)

// R4 goldens are regression fixtures only: the authority for fold semantics is R1/R2b/R13. They are
// (re)generated deliberately with FSM_RUN_UPDATE_GOLDEN=1 and reviewed in the diff, never on failure.
func TestGoldenPrefixes(t *testing.T) {
	evs := happyLog().Events()
	var logBuf, snapBuf bytes.Buffer
	for i := range evs {
		logBuf.Write(marshalCanonical(evs[i]))
		logBuf.WriteByte('\n')
		snapBuf.Write(marshalCanonical(mustFold(t, evs[:i+1])))
		snapBuf.WriteByte('\n')
	}
	if os.Getenv("FSM_RUN_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile("testdata/golden-log.jsonl", logBuf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("testdata/golden-snapshots.jsonl", snapBuf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wantLog, err := os.ReadFile("testdata/golden-log.jsonl")
	if err != nil {
		t.Fatalf("golden log: %v", err)
	}
	wantSnaps, err := os.ReadFile("testdata/golden-snapshots.jsonl")
	if err != nil {
		t.Fatalf("golden snapshots: %v", err)
	}
	if !bytes.Equal(wantLog, logBuf.Bytes()) {
		t.Fatalf("golden log drifted (Builder or canonical form changed); review and regenerate deliberately")
	}
	// fold the committed log line by line and compare each prefix snapshot
	var stored []Event
	sc := jsonl.NewScanner(bytes.NewReader(wantLog))
	sc.Buffer(make([]byte, MaxLine), MaxLine)
	for sc.Scan() {
		var ev Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, ev)
	}
	ws := jsonl.NewScanner(bytes.NewReader(wantSnaps))
	ws.Buffer(make([]byte, MaxLine), MaxLine)
	i := 0
	for ws.Scan() {
		got := marshalCanonical(mustFold(t, stored[:i+1]))
		if !bytes.Equal(got, ws.Bytes()) {
			t.Fatalf("prefix %d snapshot drifted:\n got %s\nwant %s", i+1, got, ws.Bytes())
		}
		i++
	}
	if i != len(stored) {
		t.Fatalf("golden has %d snapshots for %d events", i, len(stored))
	}
}
