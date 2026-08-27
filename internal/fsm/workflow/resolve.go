package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dsifry/metareview/internal/fsm/errs"
	"github.com/dsifry/metareview/internal/fsm/run"
)

// Resolve substitutes $VAR tokens from vars (caller) over defaults and returns
// a resolved copy plus the effective vars. calibration pins JUDGE/JUDGE_EFFORT
// for declared vars and refuses caller values for them.
func (w *Workflow) Resolve(vars map[string]string, calibration bool) (*Workflow, map[string]string, error) {
	for name := range vars {
		if _, ok := w.Vars[name]; !ok {
			return nil, nil, errs.E(CodeVarUnknown, "unknown var "+name, "name", name)
		}
	}
	effective := map[string]string{}
	names := make([]string, 0, len(w.Vars))
	for n := range w.Vars {
		names = append(names, n)
	}
	sort.Strings(names)
	pins := map[string]string{"JUDGE": CalibrationJudge, "JUDGE_EFFORT": CalibrationEffort}
	for _, name := range names {
		spec := w.Vars[name]
		if pin, pinned := pins[name]; calibration && pinned {
			if _, given := vars[name]; given {
				return nil, nil, errs.E(CodeCalibrationPinned, "--calibration pins "+name, "name", name)
			}
			effective[name] = pin
			continue
		}
		if v, ok := vars[name]; ok {
			effective[name] = v
			continue
		}
		if spec.Required {
			return nil, nil, errs.E(CodeVarUnset, "required var "+name+" is unset", "name", name)
		}
		effective[name] = spec.Default
	}
	sub := func(s string) string {
		return varRef.ReplaceAllStringFunc(s, func(m string) string {
			if m == "$$" {
				return "$"
			}
			return effective[m[1:]]
		})
	}
	c := *w
	c.Nodes = make(map[run.State]*Node, len(w.Nodes))
	for s, n := range w.Nodes {
		nn := *n
		nn.Model, nn.Effort = sub(n.Model), sub(n.Effort)
		nn.Params = make(map[string]any, len(n.Params))
		for k, v := range n.Params {
			switch x := v.(type) {
			case string:
				nn.Params[k] = sub(x)
			case []any:
				l := make([]any, len(x))
				for i, e := range x {
					if s, ok := e.(string); ok {
						l[i] = sub(s)
					} else {
						l[i] = e
					}
				}
				nn.Params[k] = l
			default:
				nn.Params[k] = v
			}
		}
		c.Nodes[s] = &nn
	}
	c.Cmds = make(map[string]*CmdDecl, len(w.Cmds))
	for name, d := range w.Cmds {
		dd := *d
		dd.Argv = make([]string, len(d.Argv))
		for i, a := range d.Argv {
			dd.Argv[i] = sub(a)
		}
		c.Cmds[name] = &dd
	}
	return &c, effective, nil
}

// ResolveCmds pins every declared command: argv[0] to an absolute path, every
// argv element that names a regular file to its sha256. Returns the consent
// list (sorted by name) and cmds_sha256 = sha256(Canonical(json)).
func ResolveCmds(w *Workflow, workDir string, lookPath func(string) (string, error), hash func(string) (string, error)) ([]run.AllowedCmd, string, error) {
	var out []run.AllowedCmd
	for _, name := range w.cmdNames() {
		d := w.Cmds[name]
		argv := append([]string{}, d.Argv...)
		var abs string
		var err error
		if strings.Contains(argv[0], "/") {
			abs = argv[0]
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(workDir, abs)
			}
			abs, err = lookPath(abs)
		} else {
			abs, err = lookPath(argv[0])
		}
		if err != nil || !filepath.IsAbs(abs) {
			return nil, "", errs.E(CodeCmdNotFound, "cmd "+name+": "+argv[0]+" not found", "name", name)
		}
		argv[0] = abs
		fh := map[string]string{}
		for _, a := range argv {
			p := candidatePath(workDir, a)
			if h, err := hash(p); err == nil {
				fh[p] = h
			}
		}
		out = append(out, run.AllowedCmd{Name: name, Argv: argv, FileHashes: fh, TimeoutMS: d.Timeout.Milliseconds(), Env: append([]string(nil), d.Env...)})
	}
	if out == nil {
		return nil, "", nil
	}
	return out, CmdsSHA256(out), nil
}

// CmdsSHA256 is the consent digest of an allowed-command list.
func CmdsSHA256(cmds []run.AllowedCmd) string {
	sorted := append([]run.AllowedCmd{}, cmds...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	raw, _ := json.Marshal(sorted)
	canon, _ := run.Canonical(raw)
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:])
}

func candidatePath(workDir, elem string) string {
	if filepath.IsAbs(elem) {
		return elem
	}
	return filepath.Join(workDir, elem)
}

// VerifyCmds re-hashes every pinned file and refuses newly-appeared files.
func VerifyCmds(allowed []run.AllowedCmd, workDir string, hash func(string) (string, error)) error {
	for _, c := range allowed {
		for p, want := range c.FileHashes {
			got, err := hash(p)
			if err != nil {
				return errs.E(CodeCmdChanged, "pinned file missing: "+p, "path", p, "reason", "missing", "name", c.Name)
			}
			if got != want {
				return errs.E(CodeCmdChanged, "pinned file changed: "+p, "path", p, "reason", "mismatch", "name", c.Name)
			}
		}
		for _, a := range c.Argv {
			p := candidatePath(workDir, a)
			if _, pinned := c.FileHashes[p]; pinned {
				continue
			}
			if _, err := hash(p); err == nil {
				return errs.E(CodeCmdChanged, "unpinned argv element now names a file: "+p, "path", p, "reason", "appeared", "name", c.Name)
			}
		}
	}
	return nil
}

// FileSHA256 hashes a regular file; directories and missing paths error.
func FileSHA256(path string) (string, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", errs.E(CodeCmdChanged, "not a regular file: "+path, "path", path, "reason", "irregular")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
