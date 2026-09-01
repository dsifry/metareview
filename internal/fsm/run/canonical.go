package run

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"
)

// Canonical returns the canonical bytes of a JSON value (§2.3): valid JSON, no duplicate keys at any
// depth, compacted, with HTML characters left as literal UTF-8, U+2028/U+2029 and U+FFFD written as
// escapes, and no trailing newline. Canonical is idempotent and preserves key order.
//
// The escapes are this package's choice rather than encoding/json's: the standard library writes
// those three differently across Go releases, and these bytes are what the audit chain hashes over.
// Every pass only ever adds an escape, never removes one, so the transform cannot change what a
// payload says. See marshalCanonical and TestCanonicalIsToolchainIndependent.
func Canonical(raw []byte) ([]byte, error) {
	if !json.Valid(raw) {
		return nil, errors.New("canonical: invalid JSON")
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		return nil, err
	}
	// Encoding a valid RawMessage cannot fail, so the error is discarded.
	return marshalCanonical(json.RawMessage(raw)), nil
}

// rejectDuplicateKeys walks the token stream and fails on a repeated key within any object, at any
// depth. raw must already be valid JSON.
func rejectDuplicateKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	type frame struct {
		object    bool
		keys      map[string]struct{}
		expectKey bool
	}
	var stack []*frame
	valueDone := func() {
		if len(stack) > 0 && stack[len(stack)-1].object {
			stack[len(stack)-1].expectKey = true
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil { // only io.EOF is possible after json.Valid
			return nil
		}
		if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].expectKey {
			if d, ok := tok.(json.Delim); ok && d == '}' {
				stack = stack[:len(stack)-1]
				valueDone()
				continue
			}
			top := stack[len(stack)-1]
			key := tok.(string) // a key position always holds a string in valid JSON
			if _, dup := top.keys[key]; dup {
				return fmt.Errorf("canonical: duplicate key %q", key)
			}
			top.keys[key] = struct{}{}
			top.expectKey = false
			continue
		}
		switch d, ok := tok.(json.Delim); {
		case ok && d == '{':
			stack = append(stack, &frame{object: true, keys: map[string]struct{}{}, expectKey: true})
		case ok && d == '[':
			stack = append(stack, &frame{})
		case ok:
			stack = stack[:len(stack)-1] // ']' (a '}' in key position was handled above)
			valueDone()
		default:
			valueDone()
		}
	}
}

// OutputHash is the hex sha256 of Canonical(raw). Invalid JSON hashes the raw bytes' canonical failure
// deterministically as the empty-string hash so callers never see a zero value; Apply rejects such
// payloads before any hash is compared.
func OutputHash(raw []byte) string {
	canon, err := Canonical(raw)
	if err != nil {
		canon = nil
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:])
}

// LineHash is the hex sha256 of a stored line's exact bytes (no trailing newline).
func LineHash(line []byte) string {
	sum := sha256.Sum256(line)
	return hex.EncodeToString(sum[:])
}

// Key builds the node@iter key used by NodeOutputs/Applied.
func Key(node string, iter int) string { return node + "@" + strconv.Itoa(iter) }

// CapText truncates s so that the canonical JSON-string encoding of the result is at most max bytes,
// cutting at a UTF-8 boundary. The flag reports whether truncation happened. The encoded length is
// computed rune by rune with the same escaping rules as the canonical encoder (quotes, backslash,
// \n \r \t as two bytes; other control characters and U+2028/U+2029 as six; everything else verbatim).
func CapText(s string, max int) (string, bool) {
	if max < 2 {
		return "", true // even "" encodes to two bytes
	}
	total := 2 // opening and closing quotes
	cut := 0
	for i, r := range s {
		_, size := utf8.DecodeRuneInString(s[i:])
		n := encodedRuneLen(r)
		if r == utf8.RuneError {
			// marshalCanonical writes both an invalid byte and a genuine U+FFFD
			// as the six-character escape, so both cost six here.
			n = 6
		}
		if total+n > max {
			return s[:cut], true
		}
		total += n
		cut = i + size
	}
	return s, false
}

func encodedRuneLen(r rune) int {
	switch {
	case r == '"' || r == '\\' || r == '\n' || r == '\r' || r == '\t':
		return 2
	case r < 0x20 || r == '\u2028' || r == '\u2029':
		return 6
	default:
		return utf8.RuneLen(r)
	}
}

// CapDetail applies CapText with MaxDetail.
func CapDetail(s string) (string, bool) { return CapText(s, MaxDetail) }
