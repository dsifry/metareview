package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dsifry/metareview/internal/repo"
)

type RunOptions struct {
	Kind   string
	CWD    string
	Covers []string
	Env    []string
	Now    func() time.Time
}

// redactCWD turns an absolute working directory into a form safe to commit in a shareable receipt: the path
// relative to the repository root when the command ran inside one (usually "." — the repo root), otherwise
// the home directory collapsed to "~", otherwise just the leaf directory. The receipt's CWD is metadata
// only (never read back as a path), so this never affects where the command runs — it keeps the operator's
// username and filesystem layout out of committed artifacts (issue #80).
func redactCWD(cwd string) string {
	if cwd == "" {
		return ""
	}
	if root, err := repo.Root(cwd); err == nil {
		if rel, err := filepath.Rel(root, cwd); err == nil && !strings.HasPrefix(rel, "..") {
			return rel // "." at the repo root, "internal/foo" below it
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, cwd); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(filepath.Join("~", rel))
		}
	}
	// Last resort: the leaf directory only — never a full path. A filesystem root (`/`, or a Windows volume
	// root like `C:\`) has no meaningful leaf, so collapse it to neutral "." rather than leaking an absolute.
	clean := filepath.Clean(cwd)
	if clean == string(filepath.Separator) || clean == filepath.VolumeName(clean)+string(filepath.Separator) {
		return "."
	}
	return filepath.Base(clean)
}

func Run(ctx context.Context, command []string, options RunOptions) (Receipt, error) {
	if len(command) == 0 {
		return Receipt{}, errors.New("evidence command is required")
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	cwd := options.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return Receipt{}, err
		}
	}
	kind := options.Kind
	if kind == "" {
		kind = ReceiptKindValidation
	}
	started := now().UTC()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = cwd
	if len(options.Env) > 0 {
		cmd.Env = append(os.Environ(), options.Env...)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	finished := now().UTC()
	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
			err = nil
		}
	}
	receipt := Receipt{
		SchemaVersion: 1,
		Kind:          kind,
		Command:       command,
		CWD:           redactCWD(cwd),
		ExitCode:      exitCode,
		StartedAt:     started,
		FinishedAt:    finished,
		StdoutSHA256:  sha256Hex(stdout.Bytes()),
		StderrSHA256:  sha256Hex(stderr.Bytes()),
		Summary:       strings.Join(command, " ") + " exited " + strconv.Itoa(exitCode),
		Covers:        options.Covers,
	}
	return receipt, err
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
