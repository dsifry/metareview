package mutation

import (
	"go/scanner"
	"go/token"
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

// goTokens scans src into its Go token stream with comments OFF, returning one "tok lit" string per
// token (the literal distinguishes identifiers/literals whose token class is the same). ok is false if
// the source did not scan cleanly.
func goTokens(src string) (toks []string, ok bool) {
	var fset token.FileSet
	file := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	ok = true
	s.Init(file, []byte(src), func(token.Position, string) { ok = false }, 0) // mode 0 → comments dropped
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if lit != "" {
			toks = append(toks, tok.String()+" "+lit)
		} else {
			toks = append(toks, tok.String())
		}
	}
	return toks, ok
}
