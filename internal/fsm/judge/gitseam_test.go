package judge

import (
	"context"
	"testing"
)

// The ONE implementation of the pinned-rev git-seam contract (see gitseam.go): grep's
// exit-1-is-no-matches, -z NUL parsing with the per-entry rev: prefix strip, ls-tree
// absence distinct from failure, and byte-exact blob reads — tested here once, with the
// cli and eval call sites injecting their own runners and their integration fixtures.

func TestGrepSeamContract(t *testing.T) {
	paths, err := GrepSeam(context.Background(), func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		if len(args) < 6 || args[0] != "grep" || args[4] != "-z" {
			t.Errorf("grep invocation = %v; want -l -i -E -z and a rev", args)
		}
		return []byte("\x00rev:spec with space.rb\x00rev:spec/plain.rb\x00"), 0, nil
	}, "d", "rev")("pat")
	if err != nil || len(paths) != 2 || paths[0] != "spec with space.rb" || paths[1] != "spec/plain.rb" {
		t.Errorf("paths = %v, %v; want the NUL entries with the rev: prefix stripped", paths, err)
	}
	if paths, err := GrepSeam(context.Background(), func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return nil, 1, nil
	}, "d", "rev")("pat"); err != nil || len(paths) != 0 {
		t.Errorf("exit 1 = %v, %v; want no matches, no error", paths, err)
	}
	if _, err := GrepSeam(context.Background(), func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return nil, 128, nil
	}, "d", "rev")("pat"); err == nil {
		t.Error("exit 128 must be an error, not an empty result")
	}
	if _, err := GrepSeam(context.Background(), func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return nil, 0, context.Canceled
	}, "d", "rev")("pat"); err == nil {
		t.Error("a transport error must surface")
	}
}

func TestShowSeamContract(t *testing.T) {
	// byte-exact blob read: leading/trailing blank lines survive
	body, ok, err := ShowSeam(context.Background(), func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		if args[0] == "ls-tree" {
			return []byte("p\x00"), 0, nil
		}
		return []byte("\n\nbody bytes\n"), 0, nil
	}, "d", "rev")("p")
	if err != nil || !ok || string(body) != "\n\nbody bytes\n" {
		t.Errorf("blob = %q, %v, %v; want the raw body", body, ok, err)
	}
	// absence: ls-tree lists nothing
	if b, ok, err := ShowSeam(context.Background(), func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
		return nil, 0, nil
	}, "d", "rev")("p"); err != nil || ok || b != nil {
		t.Errorf("absent = %v, %v, %v; want nil, false, nil", b, ok, err)
	}
	// every failure mode is an error, never "absent"
	cases := map[string]RawGitRunner{
		"ls-tree failure": func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
			if args[0] == "ls-tree" {
				return nil, 128, nil
			}
			return []byte("p\x00"), 0, nil
		},
		"ls-tree transport": func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
			return nil, 0, context.Canceled
		},
		"cat-file failure": func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
			if args[0] == "cat-file" {
				return nil, 1, nil
			}
			return []byte("p\x00"), 0, nil
		},
		"cat-file transport": func(ctx context.Context, dir string, args ...string) ([]byte, int, error) {
			if args[0] == "cat-file" {
				return nil, 0, context.Canceled
			}
			return []byte("p\x00"), 0, nil
		},
	}
	for name, run := range cases {
		if _, _, err := ShowSeam(context.Background(), run, "d", "rev")("p"); err == nil {
			t.Errorf("%s must surface as an error", name)
		}
	}
}
