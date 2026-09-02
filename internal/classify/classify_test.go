package classify

import "testing"

func TestClassifyBySuffixAndBasename(t *testing.T) {
	cases := map[string]Class{
		// code (programming/markup) and the safe default for anything unrecognised
		"internal/foo.go":      Code,
		"src/app.ts":           Code,
		"a/b/c.py":             Code,
		"x.rs":                 Code,
		"page.html":            Code,
		"style.css":            Code,
		"Makefile":             Code, // extension-less, unknown basename -> safe default
		"weird.qqzz":           Code, // unknown extension -> Code, never skipped
		"nodots":               Code,
		"internal/foo_test.go": Code,
		// config / data / behavioral
		"workflows/sdlc-loop-clean.yaml": Config,
		"a.yml":                          Config,
		"tsconfig.json":                  Config,
		"Cargo.toml":                     Config,
		"go.mod":                         Config,
		"go.sum":                         Config,
		".gitignore":                     Config,
		"cmd/.gitignore":                 Config, // basename match regardless of dir
		"data.csv":                       Config,
		"config.xml":                     Config,
		// docs / prose
		"README.md":              Docs,
		"docs/guide.rst":         Docs,
		"NOTES.txt":              Docs,
		"design.adoc":            Docs,
		"a/b/CHANGELOG.markdown": Docs,
	}
	for p, want := range cases {
		if got := Classify(p); got != want {
			t.Errorf("Classify(%q) = %s, want %s", p, got, want)
		}
	}
}

func TestClassifyIsCaseInsensitiveOnExtension(t *testing.T) {
	// Assert the EXACT class, not merely "not code", so a mutation flipping (e.g.) README.MD from Docs to
	// Config would be caught.
	for p, want := range map[string]Class{"README.MD": Docs, "x.Md": Docs, "A.YAML": Config, "y.Json": Config} {
		if got := Classify(p); got != want {
			t.Errorf("Classify(%q) = %v, want %v; extension match must be case-insensitive and land the right class", p, got, want)
		}
	}
}

func TestTallyAndHasCodeOrConfig(t *testing.T) {
	paths := []string{"internal/foo.go", "workflows/w.yaml", "README.md", "docs/x.rst", "y.unknown"}
	c := Tally(paths)
	// foo.go + y.unknown = code(2); w.yaml = config(1); README.md + x.rst = docs(2)
	if c.Code != 2 || c.Config != 1 || c.Docs != 2 {
		t.Fatalf("Tally = %+v, want {Code:2 Config:1 Docs:2}", c)
	}
	if !c.HasCodeOrConfig() {
		t.Fatal("a set with code and config must report HasCodeOrConfig")
	}
	if (Counts{Docs: 3}).HasCodeOrConfig() {
		t.Fatal("a docs-only set must not report HasCodeOrConfig")
	}
	if !(Counts{Config: 1}).HasCodeOrConfig() {
		t.Fatal("config alone is behavioral and must report HasCodeOrConfig")
	}
}
