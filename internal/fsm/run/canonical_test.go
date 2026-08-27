package run

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// R2b (canonical form): Canonical strips whitespace, keeps <>& literal, escapes U+2028/9 (as Go's
// encoder always does — the fixed point is what matters), keeps existing escapes, preserves key order, rejects invalid JSON and duplicate keys at every depth, and is idempotent.
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
