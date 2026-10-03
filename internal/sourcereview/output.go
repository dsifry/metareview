package sourcereview

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// reportLstat is a seam for filesystem metadata failures.
var reportLstat = os.Lstat

// publishReports stages both files on the destination filesystem before changing
// either saved report. Publication errors restore the previous pair. Renaming
// two files is not an atomic transaction for concurrent readers or process crashes.
func publishReports(opts Options, findings, page []byte) error {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	write, rename, remove := opts.WriteFile, opts.Rename, opts.Remove
	if write == nil {
		write = os.WriteFile
	}
	if rename == nil {
		rename = os.Rename
	}
	if remove == nil {
		remove = os.Remove
	}
	stage, err := os.MkdirTemp(opts.OutputDir, ".metareview-publish-*")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stage)
		}
	}()
	type report struct {
		name      string
		body      []byte
		exists    bool
		backedUp  bool
		published bool
	}
	reports := []report{{name: "findings.json", body: findings}, {name: "review.html", body: page}}
	for i := range reports {
		if err := ctx.Err(); err != nil {
			return err
		}
		r := &reports[i]
		if err := write(filepath.Join(stage, r.name), r.body, 0o644); err != nil {
			return err
		}
		info, err := reportLstat(filepath.Join(opts.OutputDir, r.name))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		r.exists = err == nil
		if r.exists && info.IsDir() {
			return fmt.Errorf("report destination is a directory: %s", r.name)
		}
	}
	rollback := func(cause error) error {
		var restoreErrors []error
		for i := len(reports) - 1; i >= 0; i-- {
			r := &reports[i]
			dest := filepath.Join(opts.OutputDir, r.name)
			if r.backedUp {
				if err := rename(filepath.Join(stage, r.name+".previous"), dest); err != nil {
					restoreErrors = append(restoreErrors, err)
				}
			} else if r.published {
				if err := remove(dest); err != nil {
					restoreErrors = append(restoreErrors, err)
				}
			}
		}
		if len(restoreErrors) > 0 {
			// Never delete the only surviving copy if restoration itself fails.
			cleanup = false
			return errors.Join(cause, fmt.Errorf("restore reports (backups retained in %s): %w", stage, errors.Join(restoreErrors...)))
		}
		return cause
	}
	for i := range reports {
		if err := ctx.Err(); err != nil {
			return rollback(err)
		}
		r := &reports[i]
		if r.exists {
			if err := rename(filepath.Join(opts.OutputDir, r.name), filepath.Join(stage, r.name+".previous")); err != nil {
				return rollback(err)
			}
			r.backedUp = true
		}
	}
	for i := range reports {
		if err := ctx.Err(); err != nil {
			return rollback(err)
		}
		r := &reports[i]
		if err := rename(filepath.Join(stage, r.name), filepath.Join(opts.OutputDir, r.name)); err != nil {
			return rollback(err)
		}
		r.published = true
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	return nil
}
