package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"miro/internal/output"
)

func TestRunShowsHelpWhenNoArgs(t *testing.T) {
	addFakeRecordDependencies(t, "bwrap", "screen")

	stdout, stderr := captureOutput(t, func() {
		if got := Run(nil); got != 0 {
			t.Fatalf("Run() code = %d, want %d", got, 0)
		}
	})

	if !strings.Contains(stdout, "A lean CLI E2E testing framework.") {
		t.Fatalf("stdout = %q, want root help", stdout)
	}
	if !strings.Contains(stdout, "init") || !strings.Contains(stdout, "record") {
		t.Fatalf("stdout = %q, want listed subcommands", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestRunInit(t *testing.T) {
	addFakeRecordDependencies(t, "bwrap", "screen")

	stdout, stderr := captureOutput(t, func() {
		if got := Run([]string{"init"}); got != 0 {
			t.Fatalf("Run() code = %d, want %d", got, 0)
		}
	})

	if stdout != prefixed("Done initialising...\n") {
		t.Fatalf("stdout = %q, want %q", stdout, prefixed("Done initialising...\n"))
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestRunRecord(t *testing.T) {
	addFakeRecordDependencies(t, "bwrap", "screen")

	root := t.TempDir()
	wantDir := filepath.Join(root, "e2e")
	if err := os.MkdirAll(wantDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	withWorkingDir(t, root, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"record", "suite/spec"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		createdPath := filepath.Join(wantDir, "suite", "spec")
		if stdout != prefixed(createdPath+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed(createdPath+"\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}

		info, err := os.Stat(createdPath)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("created path %q is not a directory", createdPath)
		}
	})
}

func TestRunRecordWithExplicitTestDirPath(t *testing.T) {
	addFakeRecordDependencies(t, "bwrap", "screen")

	root := t.TempDir()
	wantDir := filepath.Join(root, "e2e")
	if err := os.MkdirAll(wantDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	withWorkingDir(t, root, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"record", filepath.Join("e2e", "suite", "spec")}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		createdPath := filepath.Join(wantDir, "suite", "spec")
		if stdout != prefixed(createdPath+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed(createdPath+"\n"))
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}

		if _, err := os.Stat(createdPath); err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
	})
}

func TestRunRecordRejectsAbsolutePathOutsideTestDir(t *testing.T) {
	addFakeRecordDependencies(t, "bwrap", "screen")

	root := t.TempDir()
	wantDir := filepath.Join(root, "e2e")
	if err := os.MkdirAll(wantDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	outside := filepath.Join(root, "outside", "spec")
	withWorkingDir(t, root, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"record", outside}); got != 1 {
				t.Fatalf("Run() code = %d, want %d", got, 1)
			}
		})

		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, "must be inside test directory") {
			t.Fatalf("stderr = %q, want test directory error", stderr)
		}
		if _, err := os.Stat(outside); !os.IsNotExist(err) {
			t.Fatalf("Stat(%q) error = %v, want not exists", outside, err)
		}
	})
}

func TestRunFailsWhenDependenciesMissing(t *testing.T) {
	root := t.TempDir()
	wantDir := filepath.Join(root, "e2e")
	if err := os.MkdirAll(wantDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	addFakeRecordDependencies(t, "screen")

	withWorkingDir(t, root, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"init"}); got != 1 {
				t.Fatalf("Run() code = %d, want %d", got, 1)
			}
		})

		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, `required command "bwrap" not found in PATH`) {
			t.Fatalf("stderr = %q, want missing dependency error", stderr)
		}
		if _, err := os.Stat(filepath.Join(wantDir, "suite", "spec")); !os.IsNotExist(err) {
			t.Fatalf("Stat() error = %v, want not exists", err)
		}
	})
}

func TestRunRecordMissingPath(t *testing.T) {
	addFakeRecordDependencies(t, "bwrap", "screen")

	stdout, stderr := captureOutput(t, func() {
		if got := Run([]string{"record"}); got != 1 {
			t.Fatalf("Run() code = %d, want %d", got, 1)
		}
	})

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "accepts 1 arg(s), received 0") {
		t.Fatalf("stderr = %q, want argument error", stderr)
	}
	if !strings.Contains(stderr, "Usage:") || !strings.Contains(stderr, "miro record <path>") {
		t.Fatalf("stderr = %q, want record usage", stderr)
	}
}

func TestRunInitExtraArgs(t *testing.T) {
	addFakeRecordDependencies(t, "bwrap", "screen")

	stdout, stderr := captureOutput(t, func() {
		if got := Run([]string{"init", "extra"}); got != 1 {
			t.Fatalf("Run() code = %d, want %d", got, 1)
		}
	})

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "unknown command \"extra\" for \"miro init\"") {
		t.Fatalf("stderr = %q, want extra-arg error", stderr)
	}
	if !strings.Contains(stderr, "Usage:") || !strings.Contains(stderr, "miro init") {
		t.Fatalf("stderr = %q, want init usage", stderr)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	addFakeRecordDependencies(t, "bwrap", "screen")

	stdout, stderr := captureOutput(t, func() {
		if got := Run([]string{"wat"}); got != 1 {
			t.Fatalf("Run() code = %d, want %d", got, 1)
		}
	})

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "unknown command \"wat\" for \"miro\"") {
		t.Fatalf("stderr = %q, want unknown command error", stderr)
	}
}

func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() stdout error = %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() stderr error = %v", err)
	}

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	t.Cleanup(func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	})

	fn()

	if err := stdoutWriter.Close(); err != nil {
		t.Fatalf("stdout close error = %v", err)
	}
	if err := stderrWriter.Close(); err != nil {
		t.Fatalf("stderr close error = %v", err)
	}

	var stdoutBuf bytes.Buffer
	if _, err := io.Copy(&stdoutBuf, stdoutReader); err != nil {
		t.Fatalf("stdout copy error = %v", err)
	}

	var stderrBuf bytes.Buffer
	if _, err := io.Copy(&stderrBuf, stderrReader); err != nil {
		t.Fatalf("stderr copy error = %v", err)
	}

	return stdoutBuf.String(), stderrBuf.String()
}

func prefixed(msg string) string {
	return output.Format(msg)
}

func withWorkingDir(t *testing.T, dir string, fn func()) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	fn()
}

func addFakeRecordDependencies(t *testing.T, names ...string) {
	t.Helper()

	binDir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}

	t.Setenv("PATH", binDir)
}
