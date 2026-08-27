package errs

import (
	"errors"
	"fmt"
	"io"
	"testing"
)

func TestErrorFormat(t *testing.T) {
	cases := []struct {
		e    *Error
		want string
	}{
		{E("ERR_X", ""), "ERR_X"},
		{E("ERR_X", "boom"), "ERR_X: boom"},
		{E("ERR_X", "boom", "b", "2", "a", "1"), "ERR_X: boom (a=1, b=2)"},
		{E("ERR_X", "", "k"), "ERR_X (k=)"},
	}
	for _, c := range cases {
		if got := c.e.Error(); got != c.want {
			t.Errorf("%q != %q", got, c.want)
		}
	}
	if E("ERR_X", "").Fields != nil {
		t.Fatal("no kv → nil Fields")
	}
}

func TestIsCodeAsWrap(t *testing.T) {
	base := E("ERR_A", "a", "name", "n")
	wrapped := Wrap(base, io.EOF)
	outer := fmt.Errorf("ctx: %w", wrapped)
	if !Is(outer, "ERR_A") || Is(outer, "ERR_B") || Is(io.EOF, "ERR_A") || Is(nil, "ERR_A") {
		t.Fatal("Is")
	}
	if Code(outer) != "ERR_A" || Code(io.EOF) != "" {
		t.Fatal("Code")
	}
	if As(outer) != wrapped || As(io.EOF) != nil {
		t.Fatal("As")
	}
	if !errors.Is(outer, io.EOF) {
		t.Fatal("Unwrap chain")
	}
	if wrapped.Field("name") != "n" || wrapped.Field("zz") != "" {
		t.Fatal("Field")
	}
	// Wrap copies fields; the base must be untouched and nil cause returns base.
	wrapped.Fields["name"] = "changed"
	if base.Fields["name"] != "n" || base.Unwrap() != nil {
		t.Fatal("Wrap must copy")
	}
	if Wrap(base, nil) != base {
		t.Fatal("Wrap(nil) returns e")
	}
	if Wrap(E("ERR_N", ""), io.EOF).Fields != nil {
		t.Fatal("copyFields(nil) stays nil")
	}
}
