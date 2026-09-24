package mutationfresh

import (
	"os"
	"path/filepath"
	"testing"
)

// Spec §11.5: an edit that intersects no mutant of the file (by the §11.4.3 rule) may change what
// the file exports without touching a mutant, so it is a blanket cause, like a zero-mutant file.
func TestOutOfMutantHunkIsABlanketCause(t *testing.T) {
	for path, text := range map[string]string{
		// line 5 is the blank line between inc (1–4) and dec (6–9)
		"src/e.ts": "export function inc(n: number): number {\n  const r = n + 1;\n  return r;\n}\n// between\nexport function dec(n: number): number {\n  const r = n - 1;\n  return r;\n}\n",
		// a pure insertion after the block mutant [1,5] is outside it
		"src/a.ts": "export function grade(score: number, pass: number, top: number): string {\n  if (score < pass) return 'fail';\n  if (score > top) return 'over';\n  return 'ok';\n}\n// trailing note\n",
	} {
		report, root := realCase(t, "full")
		write(t, root, path, text)
		f := classifyAt(t, report, root)
		if f.Stale != 24 || causes(f)[path] != 24 {
			t.Errorf("%s: %+v", path, f)
		}
	}
}

// Beyond the edit distance the gate cannot tell which lines changed, so the file is a blanket cause.
func TestUndiffableMutateFileIsABlanketCause(t *testing.T) {
	report, root := realCase(t, "full")
	big := ""
	for i := 0; i <= maxEditDistance; i++ {
		big += "// filler\n"
	}
	write(t, root, "src/c.ts", big)
	if f := classifyAt(t, report, root); f.Stale != 24 || causes(f)["src/c.ts"] != 24 {
		t.Errorf("undiffable c.ts: %+v", f)
	}
}

// A deleted mutate file keeps 2a's rule: its own kills and those of the tests covering it.
// (src/e.ts has no static mutant.)
func TestDeletedMutateFileIsNotBlanket(t *testing.T) {
	report, root := realCase(t, "full")
	if err := os.Remove(filepath.Join(root, "src/e.ts")); err != nil {
		t.Fatal(err)
	}
	f := classifyAt(t, report, root)
	if f.Stale < 4 || f.Stale == 24 || causes(f)["src/e.ts"] != f.Stale {
		t.Errorf("deleted c.ts: %+v", f)
	}
}

// Spec §6.3 counts present paths: a tracked, unattested file removed with rm (still in the index)
// is not a change.
func TestDeletedUnattestedPathIsNotAChange(t *testing.T) {
	report, root := realCase(t, "full")
	write(t, root, "tests/helpers/old.ts", "export const old = 1;\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "old helper")
	if err := os.Remove(filepath.Join(root, "tests/helpers/old.ts")); err != nil {
		t.Fatal(err)
	}
	if f := classifyAt(t, report, root); f.Stale != 0 || f.Verified != 24 {
		t.Errorf("rm'd helper: %+v", f)
	}
}

// Stryker reports a static (module-level) mutant with no coveredBy: every test ran it, and the gate
// has no import graph to say which tests read it. A changed file with one is a blanket cause.
// src/c.ts's line-2 mutants include static mutant 17 in the real report.
func TestStaticMutantFileIsABlanketCause(t *testing.T) {
	report, root := realCase(t, "full")
	write(t, root, "src/c.ts", "import { LIMIT } from './limits';\nexport const twice = (n: number) => Math.min(n * 3, LIMIT);\n")
	if f := classifyAt(t, report, root); f.Stale != 24 || causes(f)["src/c.ts"] != 24 {
		t.Errorf("edited c.ts: %+v", f)
	}
}
