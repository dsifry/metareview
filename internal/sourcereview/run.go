package sourcereview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dsifry/metareview/internal/lensoutput"
)

const usageText = `Usage: metareview source-review --model astra|opus|grok --output <dir>
                              [--jobs <n>] [--call-timeout <duration>] [--path <path>]... [<repo>]

Reviews the first-party source at HEAD. Prints the kept list and the dropped
list before any model call. Writes <dir>/findings.json and <dir>/review.html.
The output directory must be outside the repository.

--jobs runs up to n prompts at once (default 8). --call-timeout limits each
model call (default 30m); a call that runs past it fails the run. --path
reviews only the kept files at that path or under that directory; repeat it
to review several. A --path that matches no kept file fails the run.`

// DefaultJobs and DefaultCallTimeout apply when Options leaves them zero.
const (
	DefaultJobs        = 8
	DefaultCallTimeout = 30 * time.Minute
)

var errHelp = errors.New("help")

// Path seams. Production uses the stdlib functions. Tests replace them to
// cover the error returns filepath and os do not produce on this OS.
var (
	pathAbs  = filepath.Abs
	pathEval = filepath.EvalSymlinks
	pathRel  = filepath.Rel
	statPath = os.Stat
)

// Options is one source-review run. A nil Git, MkdirAll, WriteFile, Remove or
// Rename uses the real git or filesystem. CLI supplies the OSRunner. A nil Context uses
// context.Background. Budget 0 means MaxPromptBytes.
// Jobs 0 means DefaultJobs and CallTimeout 0 means DefaultCallTimeout.
type Options struct {
	Context     context.Context
	Repo        string
	Model       string
	OutputDir   string
	Stdout      io.Writer
	Stderr      io.Writer
	Budget      int
	Jobs        int
	CallTimeout time.Duration
	// Paths, when set, limits the review to kept files at or under these
	// repo-relative paths. Every path must match at least one kept file.
	Paths     []string
	Runner    Runner
	Git       GitFunc
	MkdirAll  func(string, os.FileMode) error
	WriteFile func(string, []byte, os.FileMode) error
	Remove    func(string) error
	Rename    func(string, string) error
}

// Run reviews one repository. It writes the findings file and the page only
// after every prompt succeeds. A failed run preserves any previously saved reports.
func Run(opts Options) error {
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	if err := opts.Context.Err(); err != nil {
		return err
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if strings.TrimSpace(opts.OutputDir) == "" {
		return errors.New("missing output directory")
	}
	modelID, err := ModelID(opts.Model)
	if err != nil {
		return err
	}
	git := opts.Git
	if git == nil {
		git = realGit
	}
	top, err := resolveTop(opts.Repo, git)
	if err != nil {
		return err
	}
	if err := refuseInside(top, opts.OutputDir); err != nil {
		return err
	}
	kept, dropped, commit, err := selectPaths(top, git, opts.Paths)
	if err != nil {
		return err
	}
	if err := opts.Context.Err(); err != nil {
		return err
	}
	writeLists(opts.Stdout, kept, dropped)
	files := make(map[string][]byte, len(kept))
	for _, f := range kept {
		files[f.Path] = f.Body
	}
	if len(kept) == 0 {
		return writeOutputs(opts, top, commit, modelID, nil, files)
	}
	budget := opts.Budget
	if budget == 0 {
		budget = MaxPromptBytes
	}
	prompts, err := Pack(kept, budget)
	if err != nil {
		return err
	}
	if opts.Runner == nil {
		return errors.New("no runner")
	}
	all, err := review(opts, prompts, files)
	if err != nil {
		return err
	}
	return writeOutputs(opts, top, commit, modelID, all, files)
}

// review runs the prompts up to opts.Jobs at a time and returns the accepted
// findings in prompt order. Each prompt block is printed whole, in prompt
// order, before its call starts. After the first failure no new call starts,
// running calls are cancelled, and that failure is returned.
func review(opts Options, prompts [][]byte, files map[string][]byte) ([]lensoutput.TypedFinding, error) {
	jobs := opts.Jobs
	if jobs == 0 {
		jobs = DefaultJobs
	}
	if jobs < 1 {
		return nil, fmt.Errorf("jobs must be at least 1, got %d", jobs)
	}
	timeout := opts.CallTimeout
	if timeout == 0 {
		timeout = DefaultCallTimeout
	}
	if timeout < 0 {
		return nil, fmt.Errorf("call timeout must be positive, got %s", timeout)
	}
	ctx, cancel := context.WithCancel(opts.Context)
	defer cancel()
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
	)
	fail := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
	}
	n := len(prompts)
	results := make([][]lensoutput.TypedFinding, n)
	sem := make(chan struct{}, jobs)
schedule:
	for i, p := range prompts {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break schedule
		}
		if ctx.Err() != nil {
			<-sem
			break
		}
		mu.Lock()
		writePrompt(opts.Stdout, p)
		mu.Unlock()
		wg.Add(1)
		go func(i int, p []byte) {
			defer wg.Done()
			defer func() { <-sem }()
			start := time.Now()
			callCtx, callCancel := context.WithTimeout(ctx, timeout)
			defer callCancel()
			text, err := Call(callCtx, opts.Runner, opts.Model, p)
			if err == nil {
				err = callCtx.Err()
			}
			if err != nil {
				if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
					err = fmt.Errorf("call timed out after %s: %w", timeout, err)
				}
				fail(fmt.Errorf("prompt %d/%d: %w", i+1, n, err))
				return
			}
			got, skipped, err := Parse(text, files)
			if err != nil {
				fail(fmt.Errorf("prompt %d/%d: %w", i+1, n, err))
				return
			}
			results[i] = got
			mu.Lock()
			defer mu.Unlock()
			for _, msg := range skipped {
				fmt.Fprintln(opts.Stderr, msg)
			}
			fmt.Fprintf(opts.Stderr, "prompt %d/%d done in %ds (%d findings)\n", i+1, n, int(time.Since(start).Round(time.Second)/time.Second), len(got))
		}(i, p)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var all []lensoutput.TypedFinding
	for _, r := range results {
		all = append(all, r...)
	}
	return all, nil
}

func writePrompt(w io.Writer, p []byte) {
	fmt.Fprintln(w, "prompt-begin")
	_, _ = w.Write(p)
	if len(p) == 0 || p[len(p)-1] != '\n' {
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "prompt-end")
}

// onlyPaths keeps the files at or under any of paths. No paths keeps every
// file. A path that matches no kept file is an error, so a typo or a file the
// source cut dropped is reported instead of reviewing nothing.
func onlyPaths(kept []File, paths []string) ([]File, error) {
	var err error
	paths, err = cleanPaths(paths)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return kept, nil
	}
	var out []File
	matched := make([]bool, len(paths))
	for _, f := range kept {
		hit := false
		for i, p := range paths {
			if pathSelected(f.Path, []string{p}) {
				matched[i] = true
				hit = true
			}
		}
		if hit {
			out = append(out, f)
		}
	}
	for i, ok := range matched {
		if !ok {
			return nil, fmt.Errorf("--path %s matches no kept file", paths[i])
		}
	}
	return out, nil
}

func cleanPaths(paths []string) ([]string, error) {
	cleaned := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			return nil, errors.New("--path must not be empty")
		}
		cleaned = append(cleaned, path.Clean(p))
	}
	return cleaned, nil
}

func pathSelected(file string, paths []string) bool {
	if len(paths) == 0 {
		return true
	}
	for _, p := range paths {
		if file == p || strings.HasPrefix(file, p+"/") || p == "." {
			return true
		}
	}
	return false
}

func writeLists(w io.Writer, kept []File, dropped []string) {
	fmt.Fprintln(w, "kept:")
	for _, f := range kept {
		fmt.Fprintln(w, f.Path)
	}
	fmt.Fprintln(w, "dropped:")
	for _, p := range dropped {
		fmt.Fprintln(w, p)
	}
}

func writeOutputs(opts Options, repo, commit, modelID string, findings []lensoutput.TypedFinding, files map[string][]byte) error {
	if opts.Context != nil {
		if err := opts.Context.Err(); err != nil {
			return err
		}
	}
	body := findingsJSON(findings)
	page := Render(Page{Repo: repo, Commit: commit, ModelID: modelID, Findings: findings, Files: files})
	mkdir := opts.MkdirAll
	if mkdir == nil {
		mkdir = os.MkdirAll
	}
	if err := mkdir(opts.OutputDir, 0o755); err != nil {
		return err
	}
	return publishReports(opts, body, page)
}

func findingsJSON(fs []lensoutput.TypedFinding) []byte {
	if fs == nil {
		fs = []lensoutput.TypedFinding{}
	}
	body, _ := json.Marshal(struct {
		Findings []lensoutput.TypedFinding `json:"findings"`
	}{Findings: fs})
	return append(body, '\n')
}

func refuseInside(repo, out string) error {
	repoAbs, outAbs, err := absBoth(repo, out)
	if err != nil {
		return err
	}
	repoReal, err := pathEval(repoAbs)
	if err != nil {
		return err
	}
	outReal, err := evalExisting(outAbs)
	if err != nil {
		return err
	}
	rel, err := pathRel(repoReal, outReal)
	if err != nil {
		return err
	}
	if insideRel(rel) {
		return fmt.Errorf("refusing output directory inside the tree under review: %s", out)
	}
	return nil
}

func absBoth(a, b string) (string, string, error) {
	aa, err := pathAbs(a)
	if err != nil {
		return "", "", err
	}
	bb, err := pathAbs(b)
	if err != nil {
		return "", "", err
	}
	return aa, bb, nil
}

func evalExisting(p string) (string, error) {
	cur := p
	var tail []string
	for {
		if _, err := statPath(cur); err == nil {
			real, err := pathEval(cur)
			if err != nil {
				return "", err
			}
			for i := len(tail) - 1; i >= 0; i-- {
				real = filepath.Join(real, tail[i])
			}
			return real, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("cannot resolve %s", p)
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

func insideRel(rel string) bool {
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// CLI is the source-review command. repo defaults to cwd.
func CLI(args []string, cwd string, stdout, stderr io.Writer) int {
	a, err := parseArgs(args)
	if err != nil {
		if errors.Is(err, errHelp) {
			fmt.Fprintln(stdout, usageText)
			return 0
		}
		fmt.Fprintln(stderr, err.Error())
		fmt.Fprintln(stderr, usageText)
		return 2
	}
	if a.repo == "" {
		a.repo = cwd
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = Run(Options{
		Context:     ctx,
		Repo:        a.repo,
		Model:       a.model,
		OutputDir:   a.out,
		Stdout:      stdout,
		Stderr:      stderr,
		Jobs:        a.jobs,
		Paths:       a.paths,
		CallTimeout: a.timeout,
		Runner:      OSRunner{},
	})
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	return 0
}

type cliArgs struct {
	model, out, repo string
	jobs             int
	timeout          time.Duration
	paths            []string
}

func parseArgs(args []string) (cliArgs, error) {
	a := cliArgs{jobs: DefaultJobs, timeout: DefaultCallTimeout}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			return cliArgs{}, errHelp
		case arg == "--model" || arg == "--output" || arg == "--jobs" || arg == "--call-timeout" || arg == "--path":
			if i+1 >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for %s", arg)
			}
			i++
			if err := a.set(arg, args[i]); err != nil {
				return cliArgs{}, err
			}
		case strings.HasPrefix(arg, "--"):
			return cliArgs{}, fmt.Errorf("unknown option: %s", arg)
		default:
			if a.repo != "" {
				return cliArgs{}, fmt.Errorf("unexpected argument: %s", arg)
			}
			a.repo = arg
		}
	}
	if a.model == "" || a.out == "" {
		return cliArgs{}, errors.New("missing --model or --output")
	}
	return a, nil
}

func (a *cliArgs) set(flag, v string) error {
	switch flag {
	case "--model":
		a.model = v
	case "--output":
		a.out = v
	case "--path":
		if v == "" {
			return errors.New("--path must not be empty")
		}
		a.paths = append(a.paths, v)
	case "--jobs":
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return fmt.Errorf("--jobs must be an integer of at least 1, got %q", v)
		}
		a.jobs = n
	default:
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return fmt.Errorf("--call-timeout must be a positive duration such as 30m, got %q", v)
		}
		a.timeout = d
	}
	return nil
}
