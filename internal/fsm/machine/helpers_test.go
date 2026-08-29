package machine

import (
	"io"
	"os"
	"testing"
	"time"
)

type timeDuration = time.Duration

func appendBytes(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

func writeBytes(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Mode().Perm()
}

func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func fileOwnerCanChmod() bool { return os.Getuid() != 0 }

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func joinLines(lines [][]byte) []byte {
	var out []byte
	for _, l := range lines {
		out = append(out, l...)
		out = append(out, '\n')
	}
	return out
}

// badFile fails the chosen operation.
type badFile struct{ rerr, werr, cerr error }

func (b badFile) Read(p []byte) (int, error) {
	if b.rerr != nil {
		return 0, b.rerr
	}
	return 0, io.EOF
}
func (b badFile) Write(p []byte) (int, error) {
	if b.werr != nil {
		return 0, b.werr
	}
	return len(p), nil
}
func (b badFile) Close() error { return b.cerr }
