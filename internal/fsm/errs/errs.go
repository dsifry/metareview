// Package errs is the one error type shared by every internal/fsm package.
//
// Every failure a caller can act on is an *Error carrying an ERR_* Code, a
// human Detail, and structured Fields (reason, name, path, expected, got, …).
package errs

import (
	"errors"
	"sort"
	"strings"
)

// Error is a coded error. Code is an ERR_* constant; Detail is human text;
// Fields carries structured context for callers and the CLI envelope.
type Error struct {
	Code   string
	Detail string
	Fields map[string]string
	cause  error
}

// E builds an *Error. kv must be an even-length list of key/value strings;
// an odd trailing key is stored with an empty value.
func E(code, detail string, kv ...string) *Error {
	e := &Error{Code: code, Detail: detail}
	if len(kv) > 0 {
		e.Fields = make(map[string]string, len(kv)/2+1)
		for i := 0; i < len(kv); i += 2 {
			v := ""
			if i+1 < len(kv) {
				v = kv[i+1]
			}
			e.Fields[kv[i]] = v
		}
	}
	return e
}

// Wrap returns a copy of e carrying cause as its Unwrap target. A nil cause
// returns e itself.
func Wrap(e *Error, cause error) *Error {
	if cause == nil {
		return e
	}
	c := *e
	c.Fields = copyFields(e.Fields)
	c.cause = cause
	return &c
}

// Error renders "CODE: detail" followed by sorted fields as " (k=v, k=v)".
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Code)
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(e.Detail)
	}
	if len(e.Fields) > 0 {
		keys := make([]string, 0, len(e.Fields))
		for k := range e.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(" (")
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(k)
			b.WriteString("=")
			b.WriteString(e.Fields[k])
		}
		b.WriteString(")")
	}
	return b.String()
}

// Unwrap exposes the wrapped cause for errors.Is/As.
func (e *Error) Unwrap() error { return e.cause }

// Field returns Fields[k] or "".
func (e *Error) Field(k string) string { return e.Fields[k] }

// Is reports whether err is (or wraps) an *Error with the given code.
func Is(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// Code returns the ERR_* code of err, or "" when err is not an *Error.
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// As returns the *Error inside err, or nil.
func As(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

func copyFields(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
