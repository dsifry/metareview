package covergate

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Run is the covergate CLI, factored out of main so it is unit-testable to 100% (main is a one-line
// delegate). It reads the profile, floor, and require-100 list files named by flags, enforces the gate,
// and (with --update-floor) rewrites the floor. It returns the process exit code: 0 pass, 1 gate failure
// or refused update, 2 usage/flag error. It never calls os.Exit and never runs a subprocess.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("covergate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profilePath := fs.String("profile", "", "path to the `go tool covdata textfmt` profile")
	floorPath := fs.String("floor", "", "path to tests/coverage-floor.txt")
	module := fs.String("module", "", "module path (go list -m), stripped from profile/require paths")
	requirePath := fs.String("require-100", "", "file listing packages that must be exactly 100%")
	updateFloor := fs.Bool("update-floor", false, "rewrite the floor from measured packages")
	allowDecrease := fs.Bool("allow-floor-decrease", false, "permit --update-floor to lower a floor")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *profilePath == "" || *floorPath == "" || *module == "" || *requirePath == "" {
		_, _ = fmt.Fprintln(stderr, "covergate: --profile, --floor, --module and --require-100 are all required")
		return 2
	}

	profile, err := readProfile(*profilePath, *module)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	floor, err := readFloor(*floorPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	require100, err := readRequireList(*requirePath, *module)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	if *updateFloor {
		nf, err := UpdateFloor(profile, floor, require100, *allowDecrease)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		if err := os.WriteFile(*floorPath, []byte(FormatFloor(nf)), 0o644); err != nil {
			_, _ = fmt.Fprintf(stderr, "covergate: writing floor: %v\n", err)
			return 1
		}
		floor = nf // re-gate against the freshly written floor (rule 15: missing floors are now remedied)
		_, _ = fmt.Fprintf(stdout, "floor updated: %s\n", *floorPath)
	}

	rows, failures := Gate(GateInput{Profile: profile, Floor: floor, Require100: require100})
	for _, r := range rows {
		_, _ = fmt.Fprintf(stdout, "%-48s %8s  %s\n", r.Pkg, r.Pct, r.Status)
	}
	if failures > 0 {
		_, _ = fmt.Fprintf(stderr, "coverage gate FAILED (%d package(s))\n", failures)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "coverage gate passed")
	return 0
}

func readProfile(path, module string) (map[string]PkgCov, error) {
	f, err := os.Open(path) // #nosec G304 -- path is a build-tool argument, not user input
	if err != nil {
		return nil, fmt.Errorf("covergate: opening profile: %w", err)
	}
	defer func() { _ = f.Close() }()
	return ParseProfile(f, module)
}

func readFloor(path string) (map[string]float64, error) {
	f, err := os.Open(path) // #nosec G304 -- path is a build-tool argument, not user input
	if err != nil {
		return nil, fmt.Errorf("covergate: opening floor: %w", err)
	}
	defer func() { _ = f.Close() }()
	return ParseFloor(f)
}

// readRequireList reads one package path per line (as `go list` prints them, module-qualified) and
// strips the module prefix so the keys match the profile's package identities.
func readRequireList(path, module string) ([]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is a build-tool argument, not user input
	if err != nil {
		return nil, fmt.Errorf("covergate: opening require-100 list: %w", err)
	}
	prefix := strings.TrimSuffix(module, "/") + "/"
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, strings.TrimPrefix(line, prefix))
	}
	return out, nil
}
