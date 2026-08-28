package run

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// R2b (canonical form): Canonical strips whitespace, keeps <>& literal, escapes U+2028/9, keeps
// existing escapes, preserves key order, rejects invalid JSON and duplicate keys at every depth, and
// is idempotent.
//
// The U+2028/9 case used to be justified as "what Go's encoder always does". That stopped being
// true: encoding/json writes them raw on Go 1.26.7 and escapes them on 1.27, so marshalCanonical
// now escapes them itself. See TestCanonicalIsToolchainIndependent.
func TestCanonicalNormalForm(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"whitespace", "{ \"a\" : 1 ,\n \"b\" : [ 1 , 2 ] }", `{"a":1,"b":[1,2]}`},
		{"html", `{"s":"a < b && c > d"}`, `{"s":"a < b && c > d"}`},
		{"unicode-line-seps-escaped", "{\"s\":\"x\u2028y\u2029z\"}", `{"s":"x\u2028y\u2029z"}`},
		{"existing-escapes-kept", `{"s":"\u003c"}`, `{"s":"\u003c"}`},
		{"key-order-preserved", `{"z":1,"a":2}`, `{"z":1,"a":2}`},
		{"scalar", ` "str" `, `"str"`},
		{"nested-array", `[ {"k":[ 1, { "m": null } ] } ]`, `[{"k":[1,{"m":null}]}]`},
		{"nested-object-then-scalar", `{"a":{"b":1},"c":2}`, `{"a":{"b":1},"c":2}`},
		{"mixed", `{"a":[1],"b":true,"c":{"d":false},"e":null,"f":"s"}`, `{"a":[1],"b":true,"c":{"d":false},"e":null,"f":"s"}`},
		{"empty-containers", `{"a":{},"b":[]}`, `{"a":{},"b":[]}`},
	}
	for _, c := range cases {
		got, err := Canonical([]byte(c.in))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if string(got) != c.want {
			t.Fatalf("%s: got %q want %q", c.name, got, c.want)
		}
		again, err := Canonical(got)
		if err != nil || !bytes.Equal(again, got) {
			t.Fatalf("%s: not idempotent: %q -> %q (%v)", c.name, got, again, err)
		}
		if bytes.HasSuffix(got, []byte("\n")) {
			t.Fatalf("%s: trailing newline retained", c.name)
		}
	}
}

func TestCanonicalRejects(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"invalid", `{"a":`},
		{"empty", ``},
		{"dup-top", `{"a":1,"a":2}`},
		{"dup-depth3", `{"a":{"b":[{"c":1,"c":2}]}}`},
		{"dup-after-nested", `{"a":{"b":1},"a":2}`},
		{"dup-in-array-object", `[{"x":1},{"y":1,"y":2}]`},
		{"trailing-garbage", `{"a":1} x`},
	}
	for _, c := range cases {
		if _, err := Canonical([]byte(c.in)); err == nil {
			t.Fatalf("%s: expected error", c.name)
		}
	}
}

func TestOutputHashAndLineHash(t *testing.T) {
	raw := []byte("{ \"a\" : \"<\" }")
	canon, _ := Canonical(raw)
	sum := sha256.Sum256(canon)
	if OutputHash(raw) != hex.EncodeToString(sum[:]) {
		t.Fatalf("OutputHash must hash the canonical bytes")
	}
	if OutputHash(raw) != OutputHash(canon) {
		t.Fatalf("OutputHash must be stable under canonicalization")
	}
	if OutputHash([]byte("not json")) != OutputHash([]byte("{")) || OutputHash([]byte("{")) == OutputHash([]byte("{}")) {
		t.Fatalf("invalid JSON must hash deterministically as the empty canonical form")
	}
	line := []byte(`{"seq":1}`)
	lsum := sha256.Sum256(line)
	if LineHash(line) != hex.EncodeToString(lsum[:]) {
		t.Fatalf("LineHash must hash the raw line bytes")
	}
	if LineHash(line) == LineHash([]byte(`{"seq":1} `)) {
		t.Fatalf("LineHash must be over exact bytes")
	}
}

// R13: value pins the statement-coverage gate cannot see.
func TestValuePins(t *testing.T) {
	if got := BugID("x"); got != "11f6ad8ec52a" {
		t.Fatalf("BugID(x) = %q", got)
	}
	if got := Key("n", 2); got != "n@2" {
		t.Fatalf("Key = %q", got)
	}
	a := TokenTotals{Input: 1, CacheRead: 2, CacheCreate: 3, Output: 4, Reasoning: 5}
	b := TokenTotals{Input: 10, CacheRead: 20, CacheCreate: 30, Output: 40, Reasoning: 50}
	sum := a.Add(b)
	if sum != (TokenTotals{Input: 11, CacheRead: 22, CacheCreate: 33, Output: 44, Reasoning: 55}) {
		t.Fatalf("Add = %+v", sum)
	}
	if sum.Total() != 165 {
		t.Fatalf("Total = %d", sum.Total())
	}
	if len(Outcomes) != 7 || Outcomes[0] != OutcomeFixed || Outcomes[6] != OutcomeFailed {
		t.Fatalf("Outcomes = %v", Outcomes)
	}
	if KindAgentEdit != "agent-edit" {
		t.Fatalf("KindAgentEdit = %q", KindAgentEdit)
	}
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d", SchemaVersion)
	}
}

// R12 (CapText): caps are measured on the canonical JSON-string encoding, cut at a UTF-8 boundary,
// exactly-max is unchanged with flag false, one-over is truncated with flag true.
func canonLen(s string) int {
	b, _ := json.Marshal(s)
	c, _ := Canonical(b)
	return len(c)
}

func TestCapText(t *testing.T) {
	for _, s := range []string{"", "a", "aé", "ab\ncd", "q\"b\\", "x\u2028y", "\x01ctl", "é", "a\xffb", "\ufffd!"} {
		for max := 0; max <= 12; max++ {
			got, trunc := CapText(s, max)
			if l := canonLen(got); max >= 2 && l > max {
				t.Fatalf("CapText(%q,%d) = %q encodes to %d bytes", s, max, got, l)
			}
			if !trunc && got != s {
				t.Fatalf("untruncated result must be the input")
			}
			if trunc && canonLen(s) <= max {
				t.Fatalf("spurious truncation of %q at %d", s, max)
			}
		}
	}
	// A plain ASCII string of n bytes encodes to n+2 canonical bytes (quotes).
	s := strings.Repeat("a", 10)
	got, trunc := CapText(s, 12)
	if got != s || trunc {
		t.Fatalf("exactly-max: got %q trunc=%v", got, trunc)
	}
	got, trunc = CapText(s, 11)
	if !trunc || got != strings.Repeat("a", 9) {
		t.Fatalf("one-over: got %q trunc=%v", got, trunc)
	}
	// Escape-expanding content: a newline is 2 canonical bytes.
	nl := "ab\ncd"
	got, trunc = CapText(nl, 8) // "ab\ncd" canonical = "\"ab\\ncd\"" = 8 bytes
	if got != nl || trunc {
		t.Fatalf("escape exact: got %q trunc=%v", got, trunc)
	}
	got, trunc = CapText(nl, 7)
	if !trunc || got != "ab\nc" {
		t.Fatalf("escape one-over: got %q trunc=%v", got, trunc)
	}
	// UTF-8 boundary: "é" is 2 bytes; a cut mid-rune must retreat.
	got, trunc = CapText("aé", 4) // canonical "\"aé\"" is 5 bytes
	if !trunc || got != "a" {
		t.Fatalf("utf8 boundary: got %q trunc=%v", got, trunc)
	}
	got, trunc = CapDetail(strings.Repeat("x", MaxDetail))
	if !trunc || len(got) != MaxDetail-2 {
		t.Fatalf("CapDetail: len=%d trunc=%v", len(got), trunc)
	}
	if got, trunc := CapText("", 2); got != "" || trunc {
		t.Fatalf("empty: %q %v", got, trunc)
	}
}

func TestGateErrorError(t *testing.T) {
	e := &GateError{Code: "ERR_NO_COMMIT", Gate: "commit_exists", Detail: "d"}
	if e.Error() != "commit_exists: ERR_NO_COMMIT" {
		t.Fatalf("Error() = %q", e.Error())
	}
}

// The canonical encoder is what the audit chain hashes over, so its bytes must not
// depend on the toolchain: a run recorded under one Go release has to verify under
// another. encoding/json changed its treatment of U+2028/U+2029 between Go 1.26 and
// 1.27, which CI caught and no local run could, because the two differ only across
// versions. marshalCanonical now escapes them itself rather than inheriting the
// standard library's current default.
func TestCanonicalIsToolchainIndependent(t *testing.T) {
	const ls, ps = "\u2028", "\u2029"
	cases := []struct{ name, in, want string }{
		{"line separator", "x" + ls + "y", `{"s":"x\u2028y"}`},
		{"paragraph separator", "x" + ps + "y", `{"s":"x\u2029y"}`},
		{"repeated and adjacent", ls + "a" + ps + ps + "b" + ls, `{"s":"\u2028a\u2029\u2029b\u2028"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(marshalCanonical(map[string]string{"s": c.in}))
			if got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
			if strings.ContainsAny(got, ls+ps) {
				t.Fatalf("a raw separator survived into the canonical bytes: %q", got)
			}
		})
	}
	// The other half of the contract: HTML escaping stays off.
	if got := string(marshalCanonical(map[string]string{"s": "a < b && c > d"})); got != `{"s":"a < b && c > d"}` {
		t.Fatalf("HTML escaping leaked back in: %s", got)
	}
	// And the hash the chain links on is stable for a value carrying one.
	sum := sha256.Sum256(marshalCanonical(map[string]string{"s": "x" + ls + "y"}))
	if hex.EncodeToString(sum[:]) != hex.EncodeToString(func() []byte {
		s := sha256.Sum256([]byte(`{"s":"x\u2028y"}`))
		return s[:]
	}()) {
		t.Fatal("the chain hash does not match the pinned canonical bytes")
	}
}

// The normalisation must only ever ADD escapes. Removing one is unsafe: an
// unescape rewrites text the payload itself contained, and a payload holding
// the six literal characters of a \ufffd escape — which this repository's own
// types.go does — then encodes to a lone backslash before a raw rune, which is
// not a valid JSON escape. Three reviewers reproduced that independently; the
// consequences ranged from an unparseable run-log line to ERR_AUDIT_CHAIN
// rejecting a whole run with no recovery, since RepairTail only mends a tail.
func TestCanonicalNeverUnescapesThePayloadsOwnText(t *testing.T) {
	literal := "\\ufffd" // backslash u f f f d, six characters
	if len(literal) != 6 {
		t.Fatalf("fixture wrong: %q is %d bytes", literal, len(literal))
	}
	for _, payload := range []string{
		literal,
		"the escape " + literal + " appears mid-sentence",
		literal + literal,
		"raw \ufffd and literal " + literal + " together",
	} {
		out := marshalCanonical(map[string]string{"s": payload})
		if !json.Valid(out) {
			t.Fatalf("canonical output is not valid JSON for %q: %s", payload, out)
		}
		var back map[string]string
		if err := json.Unmarshal(out, &back); err != nil {
			t.Fatalf("decode %q: %v (%s)", payload, err, out)
		}
		if back["s"] != payload {
			t.Fatalf("round trip changed the value: %q -> %q", payload, back["s"])
		}
		// Idempotent, as Canonical's contract promises.
		again, err := Canonical(out)
		if err != nil || string(again) != string(out) {
			t.Fatalf("not idempotent for %q: %s -> %s (%v)", payload, out, again, err)
		}
	}
}
