package run

import (
	"encoding/json"
	"time"
)

// Override mutates a Builder-constructed event before it is recorded. Every field is overridable
// so tests can build invalid logs.
type Override func(*Event)

// Override constructors.
func WithSeq(n int64) Override        { return func(e *Event) { e.Seq = n } }
func WithPrev(p string) Override      { return func(e *Event) { e.Prev = p } }
func WithIter(i int) Override         { return func(e *Event) { e.Iter = i } }
func WithState(s State) Override      { return func(e *Event) { e.State = s } }
func WithAt(t time.Time) Override     { return func(e *Event) { e.At = Time{t} } }
func WithNode(n string) Override      { return func(e *Event) { e.Node = n } }
func WithMock(m bool) Override        { return func(e *Event) { e.Mock = m } }
func WithOrigin(o *Origin) Override   { return func(e *Event) { e.Origin = o } }
func WithVersion(v int) Override      { return func(e *Event) { e.SchemaVersion = v } }
func WithRawData(raw string) Override { return func(e *Event) { e.Data = json.RawMessage(raw) } }
func WithType(t string) Override      { return func(e *Event) { e.Type = t } }

// Builder constructs event logs with literal stamps for tests. It never calls Apply: Seq and Prev are
// derived by counters and by hashing the canonical line of the previous event, exactly as the store
// would, and Iter/State/Mock defaults track the transitions the builder has seen.
type Builder struct {
	runID  string
	events []Event
	lines  [][]byte
	at     time.Time
	iter   int
	state  State
	mock   bool
}

// NewBuilder returns a builder for runID with a deterministic clock.
func NewBuilder(runID string) *Builder {
	return &Builder{runID: runID, at: time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)}
}

// Init appends the seq-1 init event.
func (b *Builder) Init(d InitData, o ...Override) Event {
	if d.RunID == "" {
		d.RunID = b.runID
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = Time{b.at}
	}
	if d.Vars == nil {
		d.Vars = map[string]string{}
	}
	if d.AllowedCmds == nil {
		d.AllowedCmds = []AllowedCmd{}
	}
	if d.Goldens == nil {
		d.Goldens = []Golden{}
	}
	if d.Lineage == nil {
		d.Lineage = []string{}
	}
	b.state = d.InitialState
	b.mock = d.Mock != ""
	ev := b.stamp(TypeInit, d, o)
	ev.State = ""
	ev.Iter = 0
	ev.Mock = false
	for _, f := range o {
		f(&ev)
	}
	b.record(ev)
	return ev
}

// Event appends an event of the given type with a JSON-encodable payload.
func (b *Builder) Event(typ string, data any, o ...Override) Event {
	ev := b.stamp(typ, data, nil)
	if typ == TypeTransition {
		if td, ok := data.(TransitionData); ok {
			if td.Loop {
				ev.Iter = b.iter + 1
			}
		}
	}
	for _, f := range o {
		f(&ev)
	}
	b.record(ev)
	if typ == TypeTransition {
		if td, ok := data.(TransitionData); ok {
			b.state = td.To
			if td.Loop {
				b.iter++
			}
		}
	}
	return ev
}

func (b *Builder) stamp(typ string, data any, _ []Override) Event {
	canon := marshalCanonical(data) // escape-off encoder, like the store
	b.at = b.at.Add(time.Second)
	return Event{
		SchemaVersion: SchemaVersion,
		Seq:           int64(len(b.events)) + 1,
		Prev:          b.head(),
		At:            Time{b.at},
		Type:          typ,
		State:         b.state,
		Iter:          b.iter,
		Mock:          b.mock,
		Data:          canon,
	}
}

func (b *Builder) head() string {
	if len(b.lines) == 0 {
		return ""
	}
	return LineHash(b.lines[len(b.lines)-1])
}

func (b *Builder) record(ev Event) {
	b.events = append(b.events, ev)
	b.lines = append(b.lines, marshalCanonical(ev))
}

// Events returns the log built so far.
func (b *Builder) Events() []Event {
	out := make([]Event, len(b.events))
	copy(out, b.events)
	return out
}

// Lines returns the canonical stored-line bytes for the log built so far.
func (b *Builder) Lines() [][]byte {
	out := make([][]byte, len(b.lines))
	for i, l := range b.lines {
		out[i] = append([]byte(nil), l...)
	}
	return out
}

// Copy builds a child log per the fork rules (§7): a child init derived from the parent's init, then
// verbatim copies of parent events [from..to] with Origin set and fresh Seq/Prev.
func (b *Builder) Copy(parent []Event, from, to int64, childID string, edit func(*InitData)) []Event {
	pb := NewBuilder("")
	pb.events = parent
	for _, ev := range parent {
		pb.lines = append(pb.lines, marshalCanonical(ev))
	}
	var pd InitData
	_ = json.Unmarshal(parent[0].Data, &pd)
	cd := pd
	cd.RunID = childID
	cd.CreatedAt = Time{b.at.Add(time.Hour)}
	cd.ParentRunID = pd.RunID
	cd.Lineage = append(append([]string{}, pd.Lineage...), pd.RunID)
	cd.ForkedAtSeq = to
	if edit != nil {
		edit(&cd)
	}
	child := NewBuilder(childID)
	child.at = b.at.Add(time.Hour)
	child.Init(cd)
	for _, ev := range parent {
		if ev.Seq < from || ev.Seq > to {
			continue
		}
		c := ev
		c.Seq = int64(len(child.events)) + 1
		c.Prev = child.head()
		c.Origin = &Origin{RunID: pd.RunID, Seq: ev.Seq, Version: 1, Hash: LineHash(pb.lines[ev.Seq-1])}
		child.record(c)
		child.state = c.State
		child.iter = c.Iter
		if c.Type == TypeTransition {
			var td TransitionData
			_ = json.Unmarshal(c.Data, &td)
			child.state = td.To
		}
	}
	child.mock = cd.Mock != ""
	b.events, b.lines, b.at, b.iter, b.state, b.mock = child.events, child.lines, child.at, child.iter, child.state, child.mock
	b.runID = childID
	return child.Events()
}
