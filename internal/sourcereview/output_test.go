package sourcereview

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportPublicationCancellationPreservesPreviousPair(t *testing.T) {
	for cancelAfter := 0; cancelAfter <= 6; cancelAfter++ {
		t.Run(fmt.Sprintf("after-operation-%d", cancelAfter), func(t *testing.T) {
			dir := t.TempDir()
			names := []string{"findings.json", "review.html"}
			seedReports(t, dir, names)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			n := 0
			completed := func() {
				n++
				if n == cancelAfter {
					cancel()
				}
			}
			if cancelAfter == 0 {
				cancel()
			}
			err := publishReports(Options{Context: ctx, OutputDir: dir,
				WriteFile: func(p string, body []byte, mode os.FileMode) error {
					if err := os.WriteFile(p, body, mode); err != nil {
						return err
					}
					completed()
					return nil
				}, Rename: func(from, to string) error {
					if err := os.Rename(from, to); err != nil {
						return err
					}
					completed()
					return nil
				}}, []byte("new findings"), []byte("new page"))
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			checkOldReports(t, dir, names)
		})
	}
}

func TestRunCancellationDuringReportSetup(t *testing.T) {
	root, _ := newRepoBytes(t, droppedOnlyFiles())
	dir := t.TempDir()
	names := []string{"findings.json", "review.html"}
	seedReports(t, dir, names)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := Run(Options{Context: ctx, Repo: root, Model: "opus", OutputDir: dir,
		MkdirAll: func(p string, mode os.FileMode) error { cancel(); return os.MkdirAll(p, mode) }})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	checkOldReports(t, dir, names)
}

var oldReports = map[string]string{"findings.json": "old findings", "review.html": "old page"}

func seedReports(t *testing.T, dir string, names []string) {
	t.Helper()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(oldReports[name]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func checkOldReports(t *testing.T, dir string, names []string) {
	t.Helper()
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(body) != oldReports[name] {
			t.Fatalf("lost previous %s: %q %v", name, body, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != len(names) {
		t.Fatalf("extra or missing files after failure: %v %v", entries, err)
	}
}

func TestReportPublicationFailuresPreservePreviousResults(t *testing.T) {
	for _, names := range [][]string{{"findings.json", "review.html"}, {"findings.json"}, {"review.html"}, nil} {
		for failWrite := 1; failWrite <= 2; failWrite++ {
			t.Run(fmt.Sprintf("partial-write-%d-existing-%d-%v", failWrite, len(names), names), func(t *testing.T) {
				dir := t.TempDir()
				seedReports(t, dir, names)
				n := 0
				failure := errors.New("no space left on device")
				err := publishReports(Options{OutputDir: dir, WriteFile: func(p string, body []byte, mode os.FileMode) error {
					n++
					if n == failWrite {
						_ = os.WriteFile(p, []byte("partial write"), mode)
						return failure
					}
					return os.WriteFile(p, body, mode)
				}}, []byte("new findings"), []byte("new page"))
				if !errors.Is(err, failure) {
					t.Fatal(err)
				}
				checkOldReports(t, dir, names)
			})
		}
		for failRename := 1; failRename <= len(names)+2; failRename++ {
			t.Run(fmt.Sprintf("rename-%d-existing-%d-%v", failRename, len(names), names), func(t *testing.T) {
				dir := t.TempDir()
				seedReports(t, dir, names)
				n := 0
				failure := errors.New("rename failed")
				err := publishReports(Options{OutputDir: dir, Rename: func(from, to string) error {
					n++
					if n == failRename {
						return failure
					}
					return os.Rename(from, to)
				}, Remove: os.Remove}, []byte("new findings"), []byte("new page"))
				if !errors.Is(err, failure) {
					t.Fatal(err)
				}
				checkOldReports(t, dir, names)
			})
		}
	}
}

func TestReportPublicationReplacesPreviousPair(t *testing.T) {
	dir := t.TempDir()
	seedReports(t, dir, []string{"findings.json", "review.html"})
	if err := publishReports(Options{OutputDir: dir}, []byte("new findings"), []byte("new page")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"findings.json": "new findings", "review.html": "new page"} {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(body) != want {
			t.Fatalf("publication failed: %q %v", body, err)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Fatalf("left staging files: %v", entries)
	}
}

func TestReportPublicationKeepsBackupsWhenRestoreFails(t *testing.T) {
	dir := t.TempDir()
	seedReports(t, dir, []string{"findings.json", "review.html"})
	err := publishReports(Options{OutputDir: dir, Rename: func(from, to string) error {
		if strings.HasSuffix(from, ".previous") {
			return errors.New("restore failed")
		}
		if filepath.Dir(from) != dir && filepath.Base(from) == "review.html" {
			return errors.New("publish failed")
		}
		return os.Rename(from, to)
	}}, []byte("new findings"), []byte("new page"))
	if err == nil || !strings.Contains(err.Error(), "backups retained") {
		t.Fatal(err)
	}
	stages, _ := filepath.Glob(filepath.Join(dir, ".metareview-publish-*"))
	if len(stages) != 1 {
		t.Fatalf("lost backups: %v", stages)
	}
	for name, want := range oldReports {
		body, err := os.ReadFile(filepath.Join(stages[0], name+".previous"))
		if err != nil || string(body) != want {
			t.Fatalf("backup %s lost: %q %v", name, body, err)
		}
	}
}

func TestReportPublicationReportsCleanupFailure(t *testing.T) {
	dir := t.TempDir()
	err := publishReports(Options{OutputDir: dir, Rename: func(from, to string) error {
		if filepath.Base(from) == "review.html" {
			return errors.New("publish failed")
		}
		return os.Rename(from, to)
	}, Remove: func(string) error { return errors.New("remove failed") }}, []byte("new findings"), []byte("new page"))
	if err == nil || !strings.Contains(err.Error(), "remove failed") {
		t.Fatal(err)
	}
}

func TestReportPublicationFilesystemErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishReports(Options{OutputDir: file}, nil, nil); err == nil {
		t.Fatal("staging under a file accepted")
	}
	seedReports(t, dir, []string{"findings.json"})
	if err := os.Mkdir(filepath.Join(dir, "review.html"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := publishReports(Options{OutputDir: dir}, nil, nil); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "findings.json"))
	if err != nil || string(body) != oldReports["findings.json"] {
		t.Fatal("changed findings despite invalid page destination")
	}
	prev := reportLstat
	reportLstat = func(string) (os.FileInfo, error) { return nil, errors.New("metadata failed") }
	t.Cleanup(func() { reportLstat = prev })
	if err := publishReports(Options{OutputDir: dir}, nil, nil); err == nil || !strings.Contains(err.Error(), "metadata failed") {
		t.Fatal(err)
	}
}
