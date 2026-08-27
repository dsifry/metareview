package gate

import (
	"context"
	"fmt"
)

// Fake is a scripted Git for tests. Unset answers return zero values; Err
// (when non-nil) is returned by every method. Calls records method names.
type Fake struct {
	HeadSHA   string
	Refs      map[string]string // RevParse answers
	Ancestors map[string]bool   // "a b" → answer
	Counts    map[string]int    // "from..to" → count
	Clean     bool
	Porcelain string
	Diffs     map[string]string // "from..to" → diff; "HEAD" → working diff
	Tree      string            // WorkTree answer
	Err       error
	Calls     []string
}

func (f *Fake) call(name string, args ...string) {
	f.Calls = append(f.Calls, fmt.Sprint(append([]string{name}, args...)))
}

func (f *Fake) Head(context.Context) (string, error) {
	f.call("Head")
	return f.HeadSHA, f.Err
}

func (f *Fake) RevParse(_ context.Context, ref string) (string, error) {
	f.call("RevParse", ref)
	if f.Err != nil {
		return "", f.Err
	}
	if ref == "HEAD" {
		return f.HeadSHA, nil
	}
	if sha, ok := f.Refs[ref]; ok {
		return sha, nil
	}
	return "", fmt.Errorf("%s: unknown ref %q", CodeGit, ref)
}

func (f *Fake) IsAncestor(_ context.Context, a, b string) (bool, error) {
	f.call("IsAncestor", a, b)
	return f.Ancestors[a+" "+b], f.Err
}

func (f *Fake) CommitCount(_ context.Context, from, to string) (int, error) {
	f.call("CommitCount", from, to)
	return f.Counts[from+".."+to], f.Err
}

func (f *Fake) Status(context.Context) (bool, string, error) {
	f.call("Status")
	return f.Clean, f.Porcelain, f.Err
}

func (f *Fake) Diff(_ context.Context, from, to string, max int) (string, bool, error) {
	f.call("Diff", from, to)
	if f.Err != nil {
		return "", false, f.Err
	}
	d, t := Cut(f.Diffs[from+".."+to], max)
	return d, t, nil
}

func (f *Fake) WorkingDiff(_ context.Context, max int) (string, bool, error) {
	f.call("WorkingDiff")
	if f.Err != nil {
		return "", false, f.Err
	}
	d, t := Cut(f.Diffs["HEAD"], max)
	return d, t, nil
}

func (f *Fake) WorkTree(context.Context) (string, error) {
	f.call("WorkTree")
	return f.Tree, f.Err
}
