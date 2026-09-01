package run

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/dsifry/metareview/internal/state"
)

// SchemaVersion is the only version this binary reads or writes (§5.4).
const SchemaVersion = 1

// State, Outcome and Kind are opaque strings to run, except the constants below.
type State string
type Outcome string
type Kind string

const (
	OutcomeFixed    Outcome = "fixed"
	OutcomeClean    Outcome = "clean"
	OutcomeReviewed Outcome = "reviewed"
	OutcomeStalled  Outcome = "stalled"
	OutcomeOverflow Outcome = "overflow"
	OutcomeCustom   Outcome = "custom"
	OutcomeFailed   Outcome = "failed"
)

// Outcomes is the closed vocabulary a transition may carry.
var Outcomes = []Outcome{OutcomeFixed, OutcomeClean, OutcomeReviewed, OutcomeStalled, OutcomeOverflow, OutcomeCustom, OutcomeFailed}

// KindAgentEdit is the only node kind run interprets (fix-entry head bookkeeping).
const KindAgentEdit Kind = "agent-edit"

// Caps (§2.3), all measured on canonical bytes.
const (
	MaxShort   = 1 << 10
	MaxText    = 4 << 10
	MaxDesc    = 2 << 10
	MaxDetail  = 64 << 10
	MaxStderr  = 8 << 10
	MaxPayload = 256 << 10
	MaxLine    = 1 << 20

	MaxVars        = 64
	MaxGoldens     = 64
	MaxAllowedCmds = 16
	MaxArgv        = 32
	MaxFileHashes  = 64
	MaxEnv         = 16
	MaxDeltaList   = 256
	MaxWarnings    = 1024

	DefaultMaxEvents = 100_000
)

// Finding mirrors harnesseval's Finding.to_candidate_dict().
type Finding struct {
	IssueText string `json:"issue_text"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Category  string `json:"category,omitempty"`
	Source    string `json:"source,omitempty"`
}

// Golden is a human-authored expected bug.
type Golden struct {
	Comment  string `json:"comment"`
	Severity string `json:"severity,omitempty"`
	Category string `json:"category,omitempty"`
}

// Bug is a confirmed bug (the fix/verify unit).
// Bug.Verdict vocabulary (design §6).
const (
	VerdictMatched       = "matched"
	VerdictRealButUngold = "real_but_ungold"
	VerdictHallucination = "hallucination"
	// VerdictUnverifiedNoEvidence marks a candidate no judge was asked about, because the
	// diff carried no hunks for its file. It is deliberately NOT "checked but unverified":
	// nothing looked at it. The distinction matters to whoever triages the result - "no one
	// could look" is a different action than "a judge looked and was unsure" - and the
	// second case, if we ever record it, needs its own value.
	//
	// It is a confirmed-side value: a finding that could not be checked is kept for a human,
	// never denied. Asking anyway is worse than not asking, because the judge's schema is one
	// boolean, so "I cannot verify this" is returned as is_real:false and read here as
	// VerdictHallucination - which drops the finding. Measured on this repo: 52 of 100
	// verdicts said in their reasoning that the file was absent, every one at 0.99 confidence.
	VerdictUnverifiedNoEvidence = "unverified_no_evidence"
	// VerdictCheckedButUnverified marks a candidate a judge WAS asked about but returned no
	// usable answer for - today, a reply that did not parse. Distinct from
	// VerdictUnverifiedNoEvidence, where nothing looked at it at all: a human triaging the
	// two takes different actions, and collapsing either into hallucination drops a finding
	// on the strength of a transport failure rather than a judgment.
	VerdictCheckedButUnverified = "checked_but_unverified"
)

// Bug is a confirmed (or rejected) finding.
type Bug struct {
	ID         string  `json:"id"`
	Desc       string  `json:"desc"`
	File       string  `json:"file,omitempty"`
	Line       int     `json:"line,omitempty"`
	Verdict    string  `json:"verdict"`
	Confidence float64 `json:"confidence"`
	GoldenIdx  *int    `json:"golden_idx,omitempty"`
}

// BugStatus is one verify result for a bug in AllFound.
type BugStatus struct {
	ID           string  `json:"id"`
	StillPresent bool    `json:"still_present"`
	Confidence   float64 `json:"confidence"`
}

// TokenTotals accumulates token usage (the only sources are llm_call and tokens events).
type TokenTotals struct {
	Input       int64 `json:"input"`
	CacheRead   int64 `json:"cache_read"`
	CacheCreate int64 `json:"cache_create"`
	Output      int64 `json:"output"`
	Reasoning   int64 `json:"reasoning"`
}

// Add returns the field-wise sum.
// MaxTokenCounter bounds every counter in one record so sums can never wrap.
const MaxTokenCounter = int64(1) << 40

// Negative reports whether any counter is below zero (rejected by Apply so a
// driver cannot pay down a budget with negative records).
func (t TokenTotals) Negative() bool {
	return t.Input < 0 || t.CacheRead < 0 || t.CacheCreate < 0 || t.Output < 0 || t.Reasoning < 0
}

// TooLarge reports whether any counter exceeds MaxTokenCounter.
func (t TokenTotals) TooLarge() bool {
	return t.Input > MaxTokenCounter || t.CacheRead > MaxTokenCounter || t.CacheCreate > MaxTokenCounter || t.Output > MaxTokenCounter || t.Reasoning > MaxTokenCounter
}

func (t TokenTotals) Add(u TokenTotals) TokenTotals {
	return TokenTotals{Input: t.Input + u.Input, CacheRead: t.CacheRead + u.CacheRead, CacheCreate: t.CacheCreate + u.CacheCreate, Output: t.Output + u.Output, Reasoning: t.Reasoning + u.Reasoning}
}

// Total sums all five fields.
func (t TokenTotals) Total() int64 {
	return t.Input + t.CacheRead + t.CacheCreate + t.Output + t.Reasoning
}

// GateError is a failed gate evaluation.
type GateError struct {
	Code            string `json:"code"`
	Gate            string `json:"gate"`
	Detail          string `json:"detail,omitempty"`
	DetailTruncated bool   `json:"detail_truncated,omitempty"`
}

func (e *GateError) Error() string { return e.Gate + ": " + e.Code }

// AllowedCmd is a sanctioned shell command (argv[0] absolute; every file-resolving element hashed).
type AllowedCmd struct {
	Name       string            `json:"name"`
	Argv       []string          `json:"argv"`
	FileHashes map[string]string `json:"file_hashes"`
	TimeoutMS  int64             `json:"timeout_ms,omitempty"`
	Env        []string          `json:"env,omitempty"` // extra environment names passed through (consent-covered)
}

// Delta is what a node's Reduce produced; Fold applies it (§4.3). It carries no tokens.
type Delta struct {
	Findings  []Finding   `json:"findings,omitempty"`
	Confirmed []Bug       `json:"confirmed,omitempty"`
	Status    []BugStatus `json:"status,omitempty"`
	Commit    string      `json:"commit,omitempty"`
	// Pins/PinResults keep their §2.4 names but carry the §9.1 generalization: the elements are
	// DifferentialProof/ProofResult, of which a mutate-a-line pin is the Kind=="pin" case. The
	// field names are retained because §2.4 keeps them ("§9.1 GENERALIZES these") — a reproduction
	// or deletion proof rides the same carrier, so no proof kind is omitted.
	Pins       []DifferentialProof `json:"pins,omitempty"`        // fix node → prove node: the claims to check
	PinResults []ProofResult       `json:"pin_results,omitempty"` // prove node → gate: what was learned
}

// MaxPins caps how many mutations one fix node may ask to have verified. Each pin is a full
// build+test cycle in an isolated copy, so this bounds the cost of one fix round.
const MaxPins = 32

// Finding provenance for pin-derived findings. A gate selects on these, never on issue text.
const (
	SourceMutationVerify = "mutation-verify"
	CategoryUnprovenFix  = "unproven-fix"  // a fix whose pin the tests did not catch: blocks
	CategoryMalformedPin = "malformed-pin" // a pin that could not be evaluated: reported, does not block
	CategoryUnverifiable = "unverifiable"  // the tree could not answer at all: blocks
)

// ProofCategoryBlocks reports whether a mutation-verify finding of this Category is a BLOCKING one —
// an unproven fix or an unverifiable tree, both of which pins_proven fails on. A malformed pin is
// advisory (the claim is bad, not the fix), so it never blocks. Shared by the gate and the prove node
// so the two can never disagree on what blocks.
func ProofCategoryBlocks(category string) bool {
	return category == CategoryUnprovenFix || category == CategoryUnverifiable
}

// Pin is a fix agent's claim that one specific test holds one specific line of production code.
//
// {commit, summary} is not evidence; a pin is checkable: apply From→To at File and the tests must
// FAIL, restore and they must PASS. The agent declares what to break, never what to run — the test
// command comes from the workflow's consent-hashed cmds block, not from here.
//
// What a proven pin HONESTLY establishes: "this added line is exercised by a test that fails when
// the line is broken." Not that the fix is correct, nor complete — stronger than the agent's word,
// weaker than proof.
type Pin struct {
	// ID is an idempotent content hash of {Finding,File,From,To} — the reference/override key. Same
	// pin content always yields the same id; a redone fix that rewords From/To mints a new one.
	ID string `json:"id"`
	// Finding is the confirmed-finding id this pin proves a fix for: the chain link, and the key
	// Snapshot.Unproven clears by (stable across a reworded From/To, unlike ID).
	Finding string `json:"finding"`
	// File is the production file the fix touched, repo-relative.
	File string `json:"file"`
	// From is the exact text to replace; it must appear in File exactly once, and (enforced in the
	// prove wiring, not here) be a line the commit ADDED.
	From string `json:"from"`
	// To is what From becomes: a compiling change that breaks the behaviour the fix introduced.
	To string `json:"to"`
	// Test names the test the fix claims pins this line — for the report and a by-hand re-run. It
	// selects, it does not execute.
	Test string `json:"test"`
}

// PinID derives a pin's idempotent id from its defining content. Pure — no timestamp, no
// randomness — so it is stable across machines and replays.
func PinID(finding, file, from, to string) string {
	h := sha256.Sum256([]byte(finding + "\x00" + file + "\x00" + from + "\x00" + to))
	return fmt.Sprintf("%x", h[:16])
}

// PinOutcome is the schema's vocabulary for what checking a pin found. Typed and persisted here
// because it is folded into snapshots: the durable shape belongs to the durable package.
type PinOutcome string

const (
	PinProven   PinOutcome = "proven"   // break failed the tests, restore passed them
	PinSurvived PinOutcome = "survived" // mutation compiled, tests still passed → a test gap
	// PinMalformed: the claim could not be evaluated → says nothing about the fix. Widened per
	// §2.2/§9.8 R7 to cover a compiles-but-semantically-null mutation (a comment/whitespace/
	// dead-code pin, caught by the §9.8 AST pre-screen) as well as an absent/ambiguous anchor and a
	// mutation that won't compile — all "bad pin, rewrite it," never a verdict on the code.
	PinMalformed    PinOutcome = "malformed"
	PinUnverifiable PinOutcome = "unverifiable" // the tree itself could not answer → nothing learned
)

// Valid reports whether o is one of the four outcomes. An unrecognised value is never a success.
func (o PinOutcome) Valid() bool {
	switch o {
	case PinProven, PinSurvived, PinMalformed, PinUnverifiable:
		return true
	}
	return false
}

// PinResult is superseded by ProofResult (§9.1): the outcome of checking one Pin generalizes to the
// outcome of checking a DifferentialProof of any kind. See proof.go. The mutate-a-line engine keeps
// its own pin-only result type in internal/mutation.

// Time marshals as UTC RFC3339Nano and unmarshals only the Z form (§2.2).
type Time struct{ time.Time }

// MarshalJSON renders the instant in UTC with nanosecond precision and a Z suffix.
func (t Time) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.UTC().Format(time.RFC3339Nano))
}

// UnmarshalJSON accepts only RFC3339(Nano) strings with a Z suffix.
func (t *Time) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if len(s) == 0 || s[len(s)-1] != 'Z' {
		return errors.New("time: UTC Z form required")
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return err
	}
	t.Time = parsed.UTC()
	return nil
}

// Snapshot is the state derived by folding a run's events (§2.1). Never persisted as authority.
type Snapshot struct {
	SchemaVersion  int                 `json:"schemaVersion"`
	RunID          string              `json:"run_id"`
	ParentRunID    string              `json:"parent_run_id,omitempty"`
	Lineage        []string            `json:"lineage"`
	ForkedAtSeq    int64               `json:"forked_at_seq,omitempty"`
	CreatedAt      Time                `json:"created_at"`
	Seq            int64               `json:"seq"`
	Workflow       string              `json:"workflow"`
	WorkflowHash   string              `json:"workflow_hash"`
	WorkflowSource string              `json:"workflow_source,omitempty"`
	Vars           map[string]string   `json:"vars"`
	Calibration    bool                `json:"calibration"`
	Mock           string              `json:"mock,omitempty"`
	MockTainted    bool                `json:"mock_tainted"`
	RepoMode       string              `json:"repo_mode"`
	AllowedCmds    []AllowedCmd        `json:"allowed_cmds"`
	CmdsSHA256     string              `json:"cmds_sha256,omitempty"`
	RepoRoot       string              `json:"repo_root"`
	WorkDir        string              `json:"work_dir"`
	State          State               `json:"state"`
	StateKind      Kind                `json:"state_kind,omitempty"`
	Outcome        Outcome             `json:"outcome,omitempty"`
	Iteration      int                 `json:"iteration"`
	BaseSHA        string              `json:"base_sha"`
	Head           string              `json:"head"`
	FixEntryHead   string              `json:"fix_entry_head,omitempty"`
	TreeHash       string              `json:"tree_hash,omitempty"`
	TreeStatus     string              `json:"tree_status,omitempty"`
	Goldens        []Golden            `json:"goldens"`
	Findings       []Finding           `json:"findings"`
	Confirmed      []Bug               `json:"confirmed"`
	Unproven       []DifferentialProof `json:"unproven,omitempty"` // proofs no round has proven; drives re-discover. Derived, never persisted.
	AllFound       []Bug               `json:"all_found"`
	Status         []BugStatus         `json:"status"`
	Unfixed        int                 `json:"unfixed"`
	// PrevUnfixed is retained for the wire schema and operator diagnostics ONLY. No predicate
	// reads it: measuring progress by comparing unfixed totals is the defect UnfixedAtEntry
	// exists to replace, and a consumer reaching for this field would reproduce it.
	PrevUnfixed *int `json:"prev_unfixed"`
	// UnfixedAtEntry is the ID of every bug that was unfixed when the current iteration began.
	// Progress means fixing one of THESE — a set, not a count.
	//
	// Two counts were tried and both were wrong, in opposite directions. Comparing unfixed totals
	// stalls a productive loop, because the total grows whenever discovery outpaces fixing.
	// Comparing fixed totals never stalls a stuck one, because a bug discovered and fixed in the
	// same round raises the count while the backlog is untouched — and it can be inflated with no
	// work at all, since adjudicate confirming N bugs and verify calling those same N absent moves
	// it by N with nothing edited. Only the entering set distinguishes "this round fixed something
	// it was handed" from "this round found something easy".
	UnfixedAtEntry  []string                   `json:"unfixed_at_entry,omitempty"`
	Tokens          TokenTotals                `json:"tokens"`
	NodeOutputs     map[string]json.RawMessage `json:"node_outputs"`
	Applied         map[string]bool            `json:"applied"`
	NodesRun        []string                   `json:"nodes_run"`
	LastError       *GateError                 `json:"last_error,omitempty"`
	StopReason      string                     `json:"stop_reason,omitempty"`
	OverflowHandled bool                       `json:"overflow_handled"`
	Warnings        []string                   `json:"warnings"`
}

// Clone returns a deep copy: every slice element, map value, pointer target and RawMessage is fresh.
func (s Snapshot) Clone() Snapshot {
	c := s
	c.Unproven = cloneProofs(s.Unproven)
	c.Lineage = cloneStrings(s.Lineage)
	c.Vars = cloneStringMap(s.Vars)
	if s.AllowedCmds != nil {
		c.AllowedCmds = make([]AllowedCmd, len(s.AllowedCmds))
		for i, a := range s.AllowedCmds {
			c.AllowedCmds[i] = AllowedCmd{Name: a.Name, Argv: cloneStrings(a.Argv), FileHashes: cloneStringMap(a.FileHashes), TimeoutMS: a.TimeoutMS, Env: cloneStrings(a.Env)}
		}
	}
	if s.Goldens != nil {
		c.Goldens = make([]Golden, len(s.Goldens))
		copy(c.Goldens, s.Goldens)
	}
	if s.Findings != nil {
		c.Findings = make([]Finding, len(s.Findings))
		copy(c.Findings, s.Findings)
	}
	c.Confirmed = cloneBugs(s.Confirmed)
	c.AllFound = cloneBugs(s.AllFound)
	if s.Status != nil {
		c.Status = make([]BugStatus, len(s.Status))
		copy(c.Status, s.Status)
	}
	c.PrevUnfixed = cloneInt(s.PrevUnfixed)
	c.UnfixedAtEntry = append([]string(nil), s.UnfixedAtEntry...)
	if s.NodeOutputs != nil {
		c.NodeOutputs = make(map[string]json.RawMessage, len(s.NodeOutputs))
		for k, v := range s.NodeOutputs {
			c.NodeOutputs[k] = append(make(json.RawMessage, 0, len(v)), v...)
		}
	}
	if s.Applied != nil {
		c.Applied = make(map[string]bool, len(s.Applied))
		for k, v := range s.Applied {
			c.Applied[k] = v
		}
	}
	c.NodesRun = cloneStrings(s.NodesRun)
	if s.LastError != nil {
		e := *s.LastError
		c.LastError = &e
	}
	c.Warnings = cloneStrings(s.Warnings)
	return c
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneBugs(in []Bug) []Bug {
	if in == nil {
		return nil
	}
	out := make([]Bug, len(in))
	for i, b := range in {
		out[i] = b
		out[i].GoldenIdx = cloneInt(b.GoldenIdx)
	}
	return out
}

// cloneProofs deep-copies a proof slice INCLUDING each element's Pin/Deletes pointer targets, so a
// mutation through the clone can never reach the original's payload. A shallow copy would share
// those pointers and quietly break the Clone contract for a pin or deletion proof.
func cloneProofs(in []DifferentialProof) []DifferentialProof {
	if in == nil {
		return nil
	}
	out := make([]DifferentialProof, len(in))
	for i, p := range in {
		out[i] = p
		if p.Pin != nil {
			pin := *p.Pin
			out[i].Pin = &pin
		}
		if p.Deletes != nil {
			del := *p.Deletes
			out[i].Deletes = &del
		}
	}
	return out
}

func cloneInt(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// SnapshotEqualIgnoringSeq compares two snapshots by canonical encoding with Seq zeroed.
func SnapshotEqualIgnoringSeq(a, b Snapshot) bool {
	a.Seq, b.Seq = 0, 0
	return bytes.Equal(marshalCanonical(a), marshalCanonical(b))
}

// MarshalCanonical encodes v with HTML escaping disabled and no trailing
// newline — the encoding every struct→JSON path in internal/fsm must use.
func MarshalCanonical(v any) []byte { return marshalCanonical(v) }

// marshalCanonical encodes v with HTML escaping disabled and no trailing newline.
//
// The U+2028/U+2029 pass is what makes "canonical" true across toolchains rather
// than only within one. encoding/json changed its treatment of those two runes
// between Go 1.26 and 1.27 — 1.26.7 writes them raw, 1.27 escapes them — and this
// encoder produces the bytes the audit chain hashes over. Inheriting the
// standard library's evolving default would mean a run recorded under one Go
// release failing chain verification under another. Escaping them ourselves,
// always, pins the output to this repository rather than to the toolchain.
//
// Replacing the raw bytes wholesale is safe: in valid JSON these sequences can
// only ever occur inside a string literal, never as syntax.
func marshalCanonical(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // the types in this package never fail to encode
	out := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	out = bytes.ReplaceAll(out, []byte("\u2028"), []byte(`\u2028`))
	out = bytes.ReplaceAll(out, []byte("\u2029"), []byte(`\u2029`))
	// Invalid UTF-8 becomes U+FFFD either way, but the two releases disagree on
	// its written form: 1.26.7 emits the six-byte escape, 1.27 the raw rune.
	//
	// Settled on the escaped form, like the two above, because the direction
	// matters more than the choice. Every pass here only ever ADDS an escape,
	// which cannot change what the payload says. Going the other way — turning
	// an escape into a raw rune — rewrites text the payload itself contained:
	// a value holding the six literal characters of the escape came out as a
	// lone backslash before a raw rune, which is not valid JSON, and left the
	// run unrecordable. CapText budgets six to match.
	return bytes.ReplaceAll(out, []byte("\ufffd"), []byte(`\ufffd`))
}

var runIDPattern = regexp.MustCompile(`^mrv-[A-Za-z0-9-]{8,200}$`)

// ValidateRunID accepts only ids of the shape ^mrv-[A-Za-z0-9-]{8,200}$ (no path separators).
func ValidateRunID(id string) error {
	if !runIDPattern.MatchString(id) {
		return fmt.Errorf("invalid run id %q", strconv.Quote(id))
	}
	return nil
}

// RunID derives a run id for a workflow in the existing mrv-<stamp>-<scope>-<target>-<hash> shape.
func RunID(workflow string, at time.Time) string {
	return state.RunID("fsm-"+workflow, workflow, at)
}
