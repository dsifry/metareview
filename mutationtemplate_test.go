package metareview

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestMutationIncrementalTemplate runs the Node test suite of templates/mutation-incremental with
// 100% coverage thresholds. It fails in CI when Node is missing (a skipped suite is a gate that
// passed because it looked at less) and skips only on a developer machine without Node.
func TestMutationIncrementalTemplate(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("node >= 22.8 is required in CI to test templates/mutation-incremental")
		}
		t.Skip("node not installed: templates/mutation-incremental tests were NOT run")
	}
	version, err := exec.Command(node, "--version").Output()
	if err != nil {
		t.Fatalf("node --version: %v", err)
	}
	if !nodeAtLeast(strings.TrimSpace(string(version)), 22, 8) {
		t.Fatalf("node >= 22.8 required, found %s", version)
	}
	cmd := exec.Command(node, "--test", "--experimental-test-coverage",
		"--test-coverage-include=templates/mutation-incremental/**/*.mjs",
		"--test-coverage-exclude=templates/mutation-incremental/test/**",
		"--test-coverage-lines=100", "--test-coverage-branches=100", "--test-coverage-functions=100",
		"templates/mutation-incremental/test/*.test.mjs")
	cmd.Env = append(os.Environ(), "MUTATION_ALLOW_TMP_STATE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node test suite failed: %v\n%s", err, out)
	}
	report := string(out)
	sources, err := filepath.Glob("templates/mutation-incremental/lib/*.mjs")
	if err != nil {
		t.Fatal(err)
	}
	sources = append(sources, "templates/mutation-incremental/cli.mjs")
	for _, src := range sources {
		name := filepath.Base(src)
		line := regexp.MustCompile(`(?m)^.*\b` + regexp.QuoteMeta(name) + `\s*\|.*$`).FindString(report)
		if line == "" {
			t.Errorf("%s does not appear in the coverage report (never loaded by a test)", src)
			continue
		}
		if strings.Count(line, "100.00") < 3 {
			t.Errorf("%s is below 100%% coverage: %s", src, line)
		}
	}
}

func nodeAtLeast(v string, major, minor int) bool {
	parts := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3)
	if len(parts) < 2 {
		return false
	}
	ma, _ := strconv.Atoi(parts[0])
	mi, _ := strconv.Atoi(parts[1])
	return ma > major || (ma == major && mi >= minor)
}
