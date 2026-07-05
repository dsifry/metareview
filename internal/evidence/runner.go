package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type RunOptions struct {
	Kind   string
	CWD    string
	Covers []string
	Env    []string
	Now    func() time.Time
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
		CWD:           cwd,
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
