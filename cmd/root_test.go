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
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

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
	root := t.TempDir()

	withWorkingDir(t, root, func() {
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
	})

	if got := mustReadFile(t, filepath.Join(root, "miro.toml")); got != "test_dir = \"e2e\"\n" {
		t.Fatalf("config = %q, want %q", got, "test_dir = \"e2e\"\n")
	}
}

func TestRunRecord(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

	root := t.TempDir()
	wantDir := filepath.Join(root, "e2e")
	if err := os.MkdirAll(wantDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeFile(t, filepath.Join(root, "miro.toml"), "test_dir = \"e2e\"\n")

	withWorkingDir(t, root, func() {
		withStdin(t, "y\n", func() {})
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"record", "suite/spec"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		createdPath := filepath.Join(wantDir, "suite", "spec")
		if stdout != prefixed(createdPath+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed(createdPath+"\n"))
		}
		if !strings.Contains(stderr, "Save recording?") {
			t.Fatalf("stderr = %q, want save prompt", stderr)
		}

		for _, name := range []string{"in", "out"} {
			if _, err := os.Stat(filepath.Join(createdPath, name)); err != nil {
				t.Fatalf("Stat(%q) error = %v", filepath.Join(createdPath, name), err)
			}
		}
	})
}

func TestRunRecordWithExplicitTestDirPath(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

	root := t.TempDir()
	wantDir := filepath.Join(root, "e2e")
	if err := os.MkdirAll(wantDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeFile(t, filepath.Join(root, "miro.toml"), "test_dir = \"e2e\"\n")

	withWorkingDir(t, root, func() {
		withStdin(t, "y\n", func() {})
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"record", filepath.Join("e2e", "suite", "spec")}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		createdPath := filepath.Join(wantDir, "suite", "spec")
		if stdout != prefixed(createdPath+"\n") {
			t.Fatalf("stdout = %q, want %q", stdout, prefixed(createdPath+"\n"))
		}
		if !strings.Contains(stderr, "Save recording?") {
			t.Fatalf("stderr = %q, want save prompt", stderr)
		}

		for _, name := range []string{"in", "out"} {
			if _, err := os.Stat(filepath.Join(createdPath, name)); err != nil {
				t.Fatalf("Stat(%q) error = %v", filepath.Join(createdPath, name), err)
			}
		}
	})
}

func TestRunRecordRejectsAbsolutePathOutsideTestDir(t *testing.T) {
	root := t.TempDir()
	wantDir := filepath.Join(root, "e2e")
	if err := os.MkdirAll(wantDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeFile(t, filepath.Join(root, "miro.toml"), "test_dir = \"e2e\"\n")

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

func TestRunInitFailsWhenDependenciesMissing(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", "")

	withWorkingDir(t, root, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"init"}); got != 1 {
				t.Fatalf("Run() code = %d, want %d", got, 1)
			}
		})

		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, `required command "script" not found in PATH`) {
			t.Fatalf("stderr = %q, want missing dependency error", stderr)
		}
		if _, err := os.Stat(filepath.Join(root, "miro.toml")); !os.IsNotExist(err) {
			t.Fatalf("Stat(%q) error = %v, want not exists", filepath.Join(root, "miro.toml"), err)
		}
	})
}

func TestRunRecordFailsWhenDependencyMissing(t *testing.T) {
	root := t.TempDir()
	wantDir := filepath.Join(root, "e2e")
	if err := os.MkdirAll(wantDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeFile(t, filepath.Join(root, "miro.toml"), "test_dir = \"e2e\"\n")

	for _, tc := range []struct {
		name    string
		deps    []string
		missing string
	}{
		{name: "script", deps: []string{"bwrap", "bash"}, missing: "script"},
		{name: "bwrap", deps: []string{"script", "bash"}, missing: "bwrap"},
		{name: "bash", deps: []string{"script", "bwrap"}, missing: "bash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addFakeRecordDependencies(t, tc.deps...)

			withWorkingDir(t, root, func() {
				stdout, stderr := captureOutput(t, func() {
					if got := Run([]string{"record", "suite/spec"}); got != 1 {
						t.Fatalf("Run() code = %d, want %d", got, 1)
					}
				})

				if stdout != "" {
					t.Fatalf("stdout = %q, want empty", stdout)
				}
				want := `required command "` + tc.missing + `" not found in PATH`
				if !strings.Contains(stderr, want) {
					t.Fatalf("stderr = %q, want missing dependency error %q", stderr, want)
				}
				if _, err := os.Stat(filepath.Join(wantDir, "suite", "spec", "in")); !os.IsNotExist(err) {
					t.Fatalf("Stat() error = %v, want not exists", err)
				}
			})
		})
	}
}

func TestRunRecordMissingConfigFails(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

	root := t.TempDir()

	withWorkingDir(t, root, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"record", "suite/spec"}); got != 1 {
				t.Fatalf("Run() code = %d, want %d", got, 1)
			}
		})

		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, "failed to resolve test directory") || !strings.Contains(stderr, "miro.toml") {
			t.Fatalf("stderr = %q, want missing config error", stderr)
		}
	})
}

func TestRunRecordMissingPath(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

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
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

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
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

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

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
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
		body := "#!/bin/sh\nexit 0\n"
		if name == "script" {
			body = "#!/bin/sh\nif [ -n \"${FAKE_SCRIPT_ARGS_FILE:-}\" ]; then\n  : > \"$FAKE_SCRIPT_ARGS_FILE\"\n  for arg in \"$@\"; do\n    printf '%s\\n' \"$arg\" >> \"$FAKE_SCRIPT_ARGS_FILE\"\n  done\nfi\nin=''\nout=''\ncmd=''\nwhile [ \"$#\" -gt 0 ]; do\n  case \"$1\" in\n    -I)\n      in=\"$2\"\n      shift 2\n      ;;\n    -O)\n      out=\"$2\"\n      shift 2\n      ;;\n    -c)\n      cmd=\"$2\"\n      shift 2\n      ;;\n    *)\n      shift\n      ;;\n  esac\ndone\nif [ -n \"${FAKE_SCRIPT_COMMAND_BODY_FILE:-}\" ] && [ -n \"$cmd\" ]; then\n  : > \"$FAKE_SCRIPT_COMMAND_BODY_FILE\"\n  while IFS= read -r line || [ -n \"$line\" ]; do\n    printf '%s\\n' \"$line\" >> \"$FAKE_SCRIPT_COMMAND_BODY_FILE\"\n  done < \"$cmd\"\nfi\nprintf '%s' 'fake recorded input\n' > \"$in\"\nprintf '%s' 'Script started on 2026-03-18 11:13:38+00:00 [TERM=\"xterm-256color\"]\nfake recorded output\nScript done on 2026-03-18 11:13:44+00:00 [COMMAND_EXIT_CODE=\"0\"]\n' > \"$out\"\nexit 0\n"
		}
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}

	t.Setenv("PATH", binDir)
}

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stdin.txt")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}

	oldStdin := os.Stdin
	os.Stdin = file
	t.Cleanup(func() {
		os.Stdin = oldStdin
		if err := file.Close(); err != nil {
			t.Fatalf("close stdin file: %v", err)
		}
	})

	fn()
}
