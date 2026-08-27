package run

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"
)

// Canonical returns the canonical bytes of a JSON value (§2.3): valid JSON, no duplicate keys at any
// depth, compacted, with HTML characters and U+2028/U+2029 left as literal UTF-8, no trailing newline.
// Canonical is idempotent and preserves key order.
func Canonical(raw []byte) ([]byte, error) {
	if !json.Valid(raw) {
		return nil, errors.New("canonical: invalid JSON")
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(json.RawMessage(raw)); err != nil {
		return nil, fmt.Errorf("canonical: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// rejectDuplicateKeys walks the token stream and fails on a repeated key within any object.
func rejectDuplicateKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	type frame struct {
		object    bool
		keys      map[string]struct{}
		expectKey bool
	}
	var stack []*frame
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("canonical: %w", err)
		}
		if len(stack) > 0 {
			top := stack[len(stack)-1]
			if top.object && top.expectKey {
				if d, ok := tok.(json.Delim); ok && d == '}' {
					stack = stack[:len(stack)-1]
					continue
				}
				key := tok.(string)
				if _, dup := top.keys[key]; dup {
					return fmt.Errorf("canonical: duplicate key %q", key)
				}
				top.keys[key] = struct{}{}
				top.expectKey = false
				continue
			}
		}
		switch d, ok := tok.(json.Delim); {
		case ok && d == '{':
			stack = append(stack, &frame{object: true, keys: map[string]struct{}{}, expectKey: true})
		case ok && d == '[':
			stack = append(stack, &frame{})
		case ok && (d == '}' || d == ']'):
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 && stack[len(stack)-1].object {
			stack[len(stack)-1].expectKey = true
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

// BugID is the stable identity of a finding: hex(sha1(issueText))[:12].
func BugID(issueText string) string {
	sum := sha1.Sum([]byte(issueText))
	return hex.EncodeToString(sum[:])[:12]
}

// Key builds the node@iter key used by NodeOutputs/Applied.
func Key(node string, iter int) string { return node + "@" + strconv.Itoa(iter) }

// CapText truncates s so that the canonical JSON-string encoding of the result is at most max bytes,
// cutting at a UTF-8 boundary. The flag reports whether truncation happened. The encoded length is
// computed rune by rune with the same escaping rules as the canonical encoder (quotes, backslash,
// \n \r \t as two bytes; other control characters and U+2028/U+2029 as six; everything else verbatim).
func CapText(s string, max int) (string, bool) {
	if max < 2 {
		return "", s != "" || max < 2
	}
	total := 2 // opening and closing quotes
	cut := 0
	for i, r := range s {
		n := encodedRuneLen(r)
		if r == utf8.RuneError {
			if _, size := utf8.DecodeRuneInString(s[i:]); size == 1 {
				n = 6 // invalid byte: encoder emits \ufffd
			} else {
				n = 3 // a genuine U+FFFD is written verbatim
			}
		}
		if total+n > max {
			return s[:cut], true
		}
		total += n
		cut = i + utf8.RuneLen(r)
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
