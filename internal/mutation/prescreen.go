package mutation

import (
	"go/scanner"
	"go/token"
	"strings"
)

// semanticallyNull reports whether orig and mutated are the SAME Go program ignoring comments and
// whitespace — i.e. the pin's mutation is a no-op the compiler cannot tell apart (spec §9.8 R7). It is
// the trivial-pin pre-screen: a comment- or whitespace-only mutation compiles and breaks no test, so
// without this it would surface as a phantom `survived`.
//
// It compares the two token streams (comments dropped by the scanner, whitespace irrelevant between
// tokens), which is position-independent and pure. If EITHER body fails to scan cleanly, it returns
// false (NOT null): a mutation that breaks scanning is the compile step's job, not this pre-screen's.
// It does not attempt dead-code / reachability detection — no pure syntactic method covers it, and the
// spec states no reachability contract, so that case falls through to the ordinary compile-then-break
// steps.
func semanticallyNull(orig, mutated string) bool {
	a, ok := goTokens(orig)
	if !ok {
		return false
	}
	b, ok := goTokens(mutated)
	if !ok {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// goTokens scans src into its Go token stream, returning one "tok lit" string per token (the literal
// distinguishes identifiers/literals whose token class is the same). Ordinary comments are dropped
// (they are semantically null), but a comment DIRECTIVE (//go:build, //go:embed, //line, …) is KEPT:
// it affects build selection, embedded data, or linker behaviour, so a change to one is NOT null.
// ok is false if the source did not scan cleanly.
func goTokens(src string) (toks []string, ok bool) {
	var fset token.FileSet
	file := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	ok = true
	s.Init(file, []byte(src), func(token.Position, string) { ok = false }, scanner.ScanComments)
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT {
			if d, isDir := directive(lit); isDir {
				toks = append(toks, "//dir "+d)
			}
			continue
		}
		if lit != "" {
			toks = append(toks, tok.String()+" "+lit)
		} else {
			toks = append(toks, tok.String())
		}
	}
	return toks, ok
}

// directive reports whether a line comment is a meaningful Go comment directive (returning its text
// without the leading //). It covers go/ast's isDirective form — a "//line " directive, or "//name:arg"
// with a lowercase/digit name and no space before the colon (e.g. //go:build, //go:embed, //go:noescape)
// — PLUS the other comments that change linkage or build selection: cgo "//export Name", gccgo
// "//extern name", and the legacy "// +build" constraint. A change to any of these is NOT semantically
// null, so it must not be classified a trivial pin. An ordinary comment (even one with a colon like
// "// note: x", or a block comment) is not a directive.
func directive(comment string) (string, bool) {
	if !strings.HasPrefix(comment, "//") {
		return "", false // block comments are never directives
	}
	c := comment[2:]
	// //line (line directive), //export (cgo), //extern (gccgo), and the legacy "// +build" constraint
	// (whose // is followed by a space, so c begins " +build") all change linkage or build selection.
	if strings.HasPrefix(c, "line ") || strings.HasPrefix(c, "export ") || strings.HasPrefix(c, "extern ") || strings.HasPrefix(c, " +build") {
		return c, true
	}
	// //name:arg — a lowercase/digit name, no space before the colon (e.g. //go:build).
	colon := strings.Index(c, ":")
	if colon <= 0 || colon+1 >= len(c) {
		return "", false
	}
	for i := 0; i < colon; i++ {
		b := c[i]
		lowerOrDigit := b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
		if !lowerOrDigit {
			return "", false
		}
	}
	return c, true
}
