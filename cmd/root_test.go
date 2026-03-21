package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if !strings.Contains(stdout, "init") || !strings.Contains(stdout, "record") || !strings.Contains(stdout, "test") {
		t.Fatalf("stdout = %q, want listed subcommands", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestRunInit(t *testing.T) {
	root := t.TempDir()
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

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

	if got := mustReadFile(t, filepath.Join(root, "miro.toml")); got != defaultWrittenConfig("e2e") {
		t.Fatalf("config = %q, want %q", got, defaultWrittenConfig("e2e"))
	}
	info, err := os.Stat(filepath.Join(root, "e2e", "shell.sh"))
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", filepath.Join(root, "e2e", "shell.sh"), err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("shell.sh mode = %o, want executable", info.Mode().Perm())
	}
}

func TestRunRecord(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

	root := t.TempDir()
	wantDir := filepath.Join(root, "e2e")

	withWorkingDir(t, root, func() {
		initStdout, initStderr := captureOutput(t, func() {
			if got := Run([]string{"init"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})
		if initStdout != prefixed("Done initialising...\n") {
			t.Fatalf("stdout = %q, want %q", initStdout, prefixed("Done initialising...\n"))
		}
		if initStderr != "" {
			t.Fatalf("stderr = %q, want empty", initStderr)
		}

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
		if _, err := os.Stat(filepath.Join(root, "e2e", "shell.sh")); !os.IsNotExist(err) {
			t.Fatalf("Stat(%q) error = %v, want not exists", filepath.Join(root, "e2e", "shell.sh"), err)
		}
	})
}

func TestRunRecordFailsWhenDependencyMissing(t *testing.T) {
	root := t.TempDir()
	wantDir := filepath.Join(root, "e2e")
	if err := os.MkdirAll(wantDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeFile(t, filepath.Join(root, "miro.toml"), validConfigContent("e2e"))

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

func TestRunTest(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")
	t.Setenv("FAKE_SCRIPT_ECHO_STDIN", "1")

	root := t.TempDir()
	writeScenarioFixtures(t, filepath.Join(root, "e2e", "a"), "echo one\n", "echo one\n")
	writeScenarioFixtures(t, filepath.Join(root, "e2e", "nested", "b"), "echo two\n", "echo two\n")

	withWorkingDir(t, root, func() {
		initStdout, initStderr := captureOutput(t, func() {
			if got := Run([]string{"init"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})
		if initStdout != prefixed("Done initialising...\n") {
			t.Fatalf("stdout = %q, want %q", initStdout, prefixed("Done initialising...\n"))
		}
		if initStderr != "" {
			t.Fatalf("stderr = %q, want empty", initStderr)
		}

		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"test"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		for _, want := range []string{
			"RUN a",
			"PASS a",
			"RUN nested/b",
			"PASS nested/b",
			"Summary: total=2 passed=2 failed=0",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout = %q, want substring %q", stdout, want)
			}
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})
}

func TestRunTestScopedDirectory(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")
	t.Setenv("FAKE_SCRIPT_ECHO_STDIN", "1")

	root := t.TempDir()
	writeScenarioFixtures(t, filepath.Join(root, "e2e", "a"), "echo one\n", "echo one\n")
	writeScenarioFixtures(t, filepath.Join(root, "e2e", "nested", "b"), "echo two\n", "echo two\n")
	writeScenarioFixtures(t, filepath.Join(root, "e2e", "nested", "c"), "echo three\n", "echo three\n")

	withWorkingDir(t, root, func() {
		initStdout, initStderr := captureOutput(t, func() {
			if got := Run([]string{"init"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})
		if initStdout != prefixed("Done initialising...\n") {
			t.Fatalf("stdout = %q, want %q", initStdout, prefixed("Done initialising...\n"))
		}
		if initStderr != "" {
			t.Fatalf("stderr = %q, want empty", initStderr)
		}

		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"test", "nested"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		for _, want := range []string{
			"RUN nested/b",
			"PASS nested/b",
			"RUN nested/c",
			"PASS nested/c",
			"Summary: total=2 passed=2 failed=0",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout = %q, want substring %q", stdout, want)
			}
		}
		if strings.Contains(stdout, "RUN a") {
			t.Fatalf("stdout = %q, want scoped run to exclude scenario outside nested", stdout)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})
}

func TestRunTestScopedLeafDirectory(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")
	t.Setenv("FAKE_SCRIPT_ECHO_STDIN", "1")

	root := t.TempDir()
	writeScenarioFixtures(t, filepath.Join(root, "e2e", "a"), "echo one\n", "echo one\n")
	writeScenarioFixtures(t, filepath.Join(root, "e2e", "nested", "b"), "echo two\n", "echo two\n")

	withWorkingDir(t, root, func() {
		initStdout, initStderr := captureOutput(t, func() {
			if got := Run([]string{"init"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})
		if initStdout != prefixed("Done initialising...\n") {
			t.Fatalf("stdout = %q, want %q", initStdout, prefixed("Done initialising...\n"))
		}
		if initStderr != "" {
			t.Fatalf("stderr = %q, want empty", initStderr)
		}

		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"test", filepath.Join("nested", "b")}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		for _, want := range []string{
			"RUN nested/b",
			"PASS nested/b",
			"Summary: total=1 passed=1 failed=0",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout = %q, want substring %q", stdout, want)
			}
		}
		if strings.Contains(stdout, "RUN a") {
			t.Fatalf("stdout = %q, want scoped run to exclude scenario outside nested/b", stdout)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})
}

func TestRunTestReturnsOneWhenScenarioMismatches(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")
	t.Setenv("FAKE_SCRIPT_ECHO_STDIN", "1")

	root := t.TempDir()
	writeScenarioFixtures(t, filepath.Join(root, "e2e", "a"), "echo one\n", "echo one\n")
	writeScenarioFixtures(t, filepath.Join(root, "e2e", "b"), "echo two\n", "different output\n")

	withWorkingDir(t, root, func() {
		initStdout, initStderr := captureOutput(t, func() {
			if got := Run([]string{"init"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})
		if initStdout != prefixed("Done initialising...\n") {
			t.Fatalf("stdout = %q, want %q", initStdout, prefixed("Done initialising...\n"))
		}
		if initStderr != "" {
			t.Fatalf("stderr = %q, want empty", initStderr)
		}

		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"test"}); got != 1 {
				t.Fatalf("Run() code = %d, want %d", got, 1)
			}
		})

		for _, want := range []string{
			"RUN a",
			"PASS a",
			"RUN b",
			"FAIL b: output differed",
			"Summary: total=2 passed=1 failed=1",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout = %q, want substring %q", stdout, want)
			}
		}
		if !strings.Contains(stderr, "1 scenario(s) failed") {
			t.Fatalf("stderr = %q, want suite failure", stderr)
		}
	})
}

func TestRunTestMissingConfigFails(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

	root := t.TempDir()

	withWorkingDir(t, root, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"test"}); got != 1 {
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

func TestRunTestFailsWhenRecorderShellMissing(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "miro.toml"), validConfigContent("e2e"))

	withWorkingDir(t, root, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"test"}); got != 1 {
				t.Fatalf("Run() code = %d, want %d", got, 1)
			}
		})

		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, "rerun `miro init`") {
			t.Fatalf("stderr = %q, want rerun init hint", stderr)
		}
	})
}

func TestRunTestFailsWhenFixtureMalformed(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "miro.toml"), validConfigContent("e2e"))
	writeFile(t, filepath.Join(root, "e2e", "shell.sh"), "#!/bin/sh\n")
	writeFile(t, filepath.Join(root, "e2e", "broken", "in"), "echo broken\n")

	withWorkingDir(t, root, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"test"}); got != 1 {
				t.Fatalf("Run() code = %d, want %d", got, 1)
			}
		})

		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, `malformed scenario "broken": missing out fixture`) {
			t.Fatalf("stderr = %q, want malformed fixture error", stderr)
		}
	})
}

func TestRunTestFailsWhenCompareMarkerMissing(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")
	t.Setenv("FAKE_SCRIPT_ECHO_STDIN", "1")

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "miro.toml"), validConfigContent("e2e"))
	writeFile(t, filepath.Join(root, "e2e", "shell.sh"), "#!/bin/sh\n")
	writeScenarioFixtures(t, filepath.Join(root, "e2e", "some"), "echo one\n", "echo one\n")

	withWorkingDir(t, root, func() {
		stdout, stderr := captureOutput(t, func() {
			if got := Run([]string{"test"}); got != 1 {
				t.Fatalf("Run() code = %d, want %d", got, 1)
			}
		})

		for _, want := range []string{
			"RUN some",
			"FAIL some: missing compare marker",
			"Summary: total=1 passed=0 failed=1",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout = %q, want substring %q", stdout, want)
			}
		}
		if !strings.Contains(stderr, "1 scenario(s) failed") {
			t.Fatalf("stderr = %q, want suite failure", stderr)
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

func TestRunTestExtraArgs(t *testing.T) {
	addFakeRecordDependencies(t, "script", "bwrap", "bash")

	stdout, stderr := captureOutput(t, func() {
		if got := Run([]string{"test", "a", "b"}); got != 1 {
			t.Fatalf("Run() code = %d, want %d", got, 1)
		}
	})

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "accepts at most 1 arg(s), received 2") {
		t.Fatalf("stderr = %q, want extra-arg error", stderr)
	}
	if !strings.Contains(stderr, "Usage:") || !strings.Contains(stderr, "miro test [path]") {
		t.Fatalf("stderr = %q, want test usage", stderr)
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
	t.Setenv("NO_COLOR", "1")

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
	return "miro › " + msg
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func writeScenarioFixtures(t *testing.T, dir, in, out string) {
	t.Helper()

	writeFile(t, filepath.Join(dir, "in"), in)
	writeFile(t, filepath.Join(dir, "out"), out)
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
			body = `#!/bin/sh
if [ -n "${FAKE_SCRIPT_ARGS_FILE:-}" ]; then
  : > "$FAKE_SCRIPT_ARGS_FILE"
  for arg in "$@"; do
    printf '%s\n' "$arg" >> "$FAKE_SCRIPT_ARGS_FILE"
  done
fi
in=''
out=''
cmd=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    -I)
      in="$2"
      shift 2
      ;;
    -O)
      out="$2"
      shift 2
      ;;
    -c)
      cmd="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [ -n "${FAKE_SCRIPT_COMMAND_BODY_FILE:-}" ] && [ -n "$cmd" ]; then
  : > "$FAKE_SCRIPT_COMMAND_BODY_FILE"
  while IFS= read -r line || [ -n "$line" ]; do
    printf '%s\n' "$line" >> "$FAKE_SCRIPT_COMMAND_BODY_FILE"
  done < "$cmd"
fi
command_has_compare_marker=0
if [ -n "$cmd" ] && /bin/grep -q '__MIRO_E2E_BEGIN__' "$cmd" 2>/dev/null; then
  command_has_compare_marker=1
fi
stdin_file=''
if [ "${FAKE_SCRIPT_ECHO_STDIN:-}" = "1" ] || [ -n "${FAKE_SCRIPT_CAPTURE_STDIN_FILE:-}" ]; then
  stdin_file="${TMPDIR:-/tmp}/miro-fake-script-stdin-$$"
  /bin/cat > "$stdin_file"
fi
if [ -n "${FAKE_SCRIPT_CAPTURE_STDIN_FILE:-}" ] && [ -n "$stdin_file" ]; then
  /bin/cp "$stdin_file" "$FAKE_SCRIPT_CAPTURE_STDIN_FILE"
fi
if [ -n "$in" ] && [ -n "${FAKE_SCRIPT_LOG_IN+x}" ]; then
  printf '%s' "$FAKE_SCRIPT_LOG_IN" > "$in"
elif [ -n "$in" ] && [ -n "$stdin_file" ]; then
  /bin/cp "$stdin_file" "$in"
elif [ -n "$in" ]; then
  printf '%s' 'fake recorded input
' > "$in"
fi
if [ -n "${FAKE_SCRIPT_STREAM_OUT+x}" ]; then
  printf '%s' "$FAKE_SCRIPT_STREAM_OUT"
elif [ "${FAKE_SCRIPT_ECHO_STDIN:-}" = "1" ] && [ -n "$stdin_file" ]; then
  /bin/cat "$stdin_file"
  if [ "${MIRO_COMPARE_MARKER:-0}" = "1" ] && [ "$command_has_compare_marker" = "1" ]; then
    printf '%s\n' '__MIRO_E2E_BEGIN__'
    /bin/cat "$stdin_file"
  fi
fi
if [ -n "${FAKE_SCRIPT_STREAM_ERR+x}" ]; then
  printf '%s' "$FAKE_SCRIPT_STREAM_ERR" >&2
fi
if [ -n "$out" ] && [ -n "${FAKE_SCRIPT_LOG_OUT+x}" ]; then
  printf '%s' "$FAKE_SCRIPT_LOG_OUT" > "$out"
elif [ -n "$out" ] && [ "${FAKE_SCRIPT_ECHO_STDIN:-}" = "1" ] && [ -n "$stdin_file" ]; then
  {
    printf '%s' 'Script started on 2026-03-18 11:13:38+00:00 [TERM="xterm-256color"]
'
    /bin/cat "$stdin_file"
    if [ "${MIRO_COMPARE_MARKER:-0}" = "1" ] && [ "$command_has_compare_marker" = "1" ]; then
      printf '%s\n' '__MIRO_E2E_BEGIN__'
      /bin/cat "$stdin_file"
    fi
    printf '%s' 'Script done on 2026-03-18 11:13:44+00:00 [COMMAND_EXIT_CODE="0"]
'
  } > "$out"
elif [ -n "$out" ]; then
  printf '%s' 'Script started on 2026-03-18 11:13:38+00:00 [TERM="xterm-256color"]
fake recorded output
Script done on 2026-03-18 11:13:44+00:00 [COMMAND_EXIT_CODE="0"]
' > "$out"
fi
if [ -n "$stdin_file" ]; then
  /bin/rm -f "$stdin_file"
fi
if [ -n "${FAKE_SCRIPT_EXIT_CODE:-}" ]; then
  exit "$FAKE_SCRIPT_EXIT_CODE"
fi
exit 0
`
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

func defaultWrittenConfig(testDir string) string {
	return "[miro]\n  test_dir = \"" + testDir + "\"\n\n[sandbox]\n  visible_home = \"/home/test\"\n"
}

func validConfigContent(testDir string) string {
	return "[miro]\ntest_dir = \"" + testDir + "\"\n\n[sandbox]\nvisible_home = \"/home/test\"\n"
}
