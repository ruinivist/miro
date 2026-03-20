package miro

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecordCreatesRelativePath(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	mustMkdirAll(t, testDir)
	writeFile(t, filepath.Join(root, "miro.toml"), validConfigContent("e2e"))
	mustWriteRecordShell(t, testDir)
	addFakeRecordDependencies(t, "script")

	got := withWorkingDir(t, root, func() string {
		withStdin(t, "y\n", func() {})
		path, err := Record(filepath.Join("a", "b", "c") + string(os.PathSeparator))
		if err != nil {
			t.Fatalf("Record() error = %v", err)
		}
		return path
	})

	want := filepath.Join(testDir, "a", "b", "c")
	if got != want {
		t.Fatalf("Record() = %q, want %q", got, want)
	}

	for _, name := range []string{"in", "out"} {
		if _, err := os.Stat(filepath.Join(want, name)); err != nil {
			t.Fatalf("Stat(%q) error = %v", filepath.Join(want, name), err)
		}
	}

	if got := readFile(t, filepath.Join(want, "in")); got != "fake recorded input\n" {
		t.Fatalf("saved in = %q, want %q", got, "fake recorded input\n")
	}
	if got := readFile(t, filepath.Join(want, "out")); got != "fake recorded output\n" {
		t.Fatalf("saved out = %q, want %q", got, "fake recorded output\n")
	}
}

func TestRecordReturnsDiscardedErrorWhenSaveDeclined(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	mustMkdirAll(t, testDir)
	mustWriteRecordShell(t, testDir)
	addFakeRecordDependencies(t, "script")

	err := withWorkingDir(t, root, func() error {
		target := filepath.Join(testDir, "a", "b", "c")
		mustMkdirAll(t, target)
		return withRecordStreams(t, "n\n", func(rio recordIO) error {
			return recordScenario(target, recordShellPath(testDir), rio, defaultSandboxConfig())
		})
	})

	if !errors.Is(err, ErrRecordingDiscarded) {
		t.Fatalf("Record() error = %v, want ErrRecordingDiscarded", err)
	}

	target := filepath.Join(testDir, "a", "b", "c")
	for _, name := range []string{"in", "out"} {
		if _, err := os.Stat(filepath.Join(target, name)); !os.IsNotExist(err) {
			t.Fatalf("Stat(%q) error = %v, want not exists", filepath.Join(target, name), err)
		}
	}
}

func TestRecordReturnsDiscardedErrorWhenOverwriteDeclined(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	target := filepath.Join(testDir, "a", "b", "c")
	mustMkdirAll(t, target)
	mustWriteRecordShell(t, testDir)
	addFakeRecordDependencies(t, "script")
	writeFile(t, filepath.Join(target, "in"), "existing in\n")
	writeFile(t, filepath.Join(target, "out"), "existing out\n")

	err := withWorkingDir(t, root, func() error {
		return withRecordStreams(t, "n\n", func(rio recordIO) error {
			return recordScenario(target, recordShellPath(testDir), rio, defaultSandboxConfig())
		})
	})

	if !errors.Is(err, ErrRecordingDiscarded) {
		t.Fatalf("recordScenario() error = %v, want ErrRecordingDiscarded", err)
	}

	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "in", want: "existing in\n"},
		{name: "out", want: "existing out\n"},
	} {
		got, readErr := os.ReadFile(filepath.Join(target, tc.name))
		if readErr != nil {
			t.Fatalf("ReadFile(%q) error = %v", filepath.Join(target, tc.name), readErr)
		}
		if string(got) != tc.want {
			t.Fatalf("%s = %q, want %q", tc.name, string(got), tc.want)
		}
	}
}

func TestBuildRecordShellScriptUsesExpectedCommands(t *testing.T) {
	body := buildRecordShellScript()

	for _, want := range []string{
		"host_home=${MIRO_HOST_HOME:?}",
		"host_tmp=${MIRO_HOST_TMP:?}",
		"path_env=${MIRO_PATH_ENV:?}",
		"visible_home=${MIRO_VISIBLE_HOME:?}",
		`if [ "${MIRO_COMPARE_MARKER:-0}" = "1" ]; then`,
		"printf '__MIRO_E2E_BEGIN__\\n'",
		"--bind \"$host_home\" \"$visible_home\"",
		"--bind \"$host_tmp\" '/tmp'",
		"--setenv HOME \"$visible_home\"",
		"--setenv PATH \"$path_env\"",
		"--setenv PS1 '$ '",
		"--setenv TERM 'xterm-256color'",
		"--setenv TZ 'UTC'",
		"--chdir \"$visible_home\"",
		"exec bwrap \"$@\" bash --noprofile --norc -i",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("wrapper = %q, want substring %q", body, want)
		}
	}
}

func TestRecordSessionEnvIncludesConfiguredSandboxEnv(t *testing.T) {
	env := recordSessionEnv(recordSandbox{
		hostHome: "/tmp/host-home",
		hostTmp:  "/tmp/host-tmp",
		pathEnv:  "/tmp/bin",
	}, map[string]string{
		"visible_home": "/sandbox/home",
		"key_word":     "value",
	})

	for _, want := range []string{
		"MIRO_HOST_HOME=/tmp/host-home",
		"MIRO_HOST_TMP=/tmp/host-tmp",
		"MIRO_PATH_ENV=/tmp/bin",
		"MIRO_KEY_WORD=value",
		"MIRO_VISIBLE_HOME=/sandbox/home",
	} {
		if !containsEnvEntry(env, want) {
			t.Fatalf("env = %#v, want entry %q", env, want)
		}
	}
}

func TestRecordSessionEnvWithExtraIncludesAdditionalEntries(t *testing.T) {
	env := recordSessionEnvWithExtra(recordSandbox{
		hostHome: "/tmp/host-home",
		hostTmp:  "/tmp/host-tmp",
		pathEnv:  "/tmp/bin",
	}, map[string]string{
		"visible_home": "/sandbox/home",
	}, map[string]string{
		compareMarkerEnvName: compareMarkerEnabledValue,
	})

	for _, want := range []string{
		"MIRO_HOST_HOME=/tmp/host-home",
		"MIRO_HOST_TMP=/tmp/host-tmp",
		"MIRO_PATH_ENV=/tmp/bin",
		"MIRO_VISIBLE_HOME=/sandbox/home",
		"MIRO_COMPARE_MARKER=1",
	} {
		if !containsEnvEntry(env, want) {
			t.Fatalf("env = %#v, want entry %q", env, want)
		}
	}
}

func TestRunRecordSessionUsesSandboxedScriptCommand(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "e2e")
	mustWriteRecordShell(t, testDir)
	addFakeRecordDependencies(t, "script")

	argsPath := filepath.Join(t.TempDir(), "script.args")
	commandBodyPath := filepath.Join(t.TempDir(), "script.command")
	t.Setenv("FAKE_SCRIPT_ARGS_FILE", argsPath)
	t.Setenv("FAKE_SCRIPT_COMMAND_BODY_FILE", commandBodyPath)

	sandbox, cleanup, err := newRecordSandboxForPathEnv(os.Getenv("PATH"))
	if err != nil {
		t.Fatalf("newRecordSandboxForPathEnv() error = %v", err)
	}
	defer cleanup()

	shellPath := recordShellPath(testDir)
	err = withRecordStreams(t, "", func(rio recordIO) error {
		return runRecordSession(t.TempDir(), filepath.Join(t.TempDir(), "raw.in"), filepath.Join(t.TempDir(), "raw.out"), shellPath, sandbox, rio, defaultSandboxConfig())
	})
	if err != nil {
		t.Fatalf("runRecordSession() error = %v", err)
	}

	args := strings.Split(strings.TrimSpace(readFile(t, argsPath)), "\n")
	if len(args) != 9 {
		t.Fatalf("script args = %q, want 9 args", args)
	}
	if got := args[:4]; strings.Join(got, "\n") != strings.Join([]string{"-q", "-E", "always", "-I"}, "\n") {
		t.Fatalf("script args prefix = %q, want %q", got, []string{"-q", "-E", "always", "-I"})
	}
	if args[5] != "-O" {
		t.Fatalf("script args[5] = %q, want %q", args[5], "-O")
	}
	if args[7] != "-c" {
		t.Fatalf("script args[7] = %q, want %q", args[7], "-c")
	}
	if args[8] != shellPath {
		t.Fatalf("script args[8] = %q, want %q", args[8], shellPath)
	}

	body := readFile(t, commandBodyPath)
	for _, want := range []string{
		"host_home=${MIRO_HOST_HOME:?}",
		"visible_home=${MIRO_VISIBLE_HOME:?}",
		`if [ "${MIRO_COMPARE_MARKER:-0}" = "1" ]; then`,
		"printf '__MIRO_E2E_BEGIN__\\n'",
		"--ro-bind / /",
		"--tmpfs /home",
		"--setenv HOME \"$visible_home\"",
		"--setenv TMPDIR '/tmp'",
		"exec bwrap \"$@\" bash --noprofile --norc -i",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("wrapper = %q, want substring %q", body, want)
		}
	}
}

func TestRecordScenarioUsesDeterministicSandbox(t *testing.T) {
	requireCommands(t, "script", "bwrap", "bash")

	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	target := filepath.Join(testDir, "suite", "spec")
	mustMkdirAll(t, target)
	mustWriteRecordShell(t, testDir)
	sandboxConfig := map[string]string{
		"visible_home": "/home/test",
		"key_word":     "value",
	}
	visibleHome := sandboxConfig["visible_home"]

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Fatalf("close pipe reader: %v", err)
		}
	})

	writeDone := make(chan error, 1)
	go func() {
		defer close(writeDone)
		defer writer.Close()

		if _, err := writer.Write([]byte("pwd\necho \"$HOME\"\necho \"$MIRO_KEY_WORD\"\nif [ -e \"$HOME/repo\" ]; then echo FOUND; else echo MISSING; fi\npwd\nexit\n")); err != nil {
			writeDone <- err
			return
		}

		time.Sleep(300 * time.Millisecond)

		if _, err := writer.Write([]byte("y\n")); err != nil {
			writeDone <- err
			return
		}

		writeDone <- nil
	}()

	err = withWorkingDir(t, root, func() error {
		return recordScenario(target, recordShellPath(testDir), recordIO{
			in:  reader,
			out: ioDiscard{},
			err: &bytes.Buffer{},
		}, sandboxConfig)
	})
	if err != nil {
		t.Fatalf("recordScenario() error = %v", err)
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("write session input: %v", err)
	}

	recordedIn := readFile(t, filepath.Join(target, "in"))
	if strings.Contains(recordedIn, "Script started on ") {
		t.Fatalf("saved in = %q, want stripped script wrapper", recordedIn)
	}
	for _, want := range []string{"pwd\n", "echo \"$HOME\"\n", "echo \"$MIRO_KEY_WORD\"\n", "if [ -e \"$HOME/repo\" ]; then echo FOUND; else echo MISSING; fi\n", "exit\n"} {
		if !strings.Contains(recordedIn, want) {
			t.Fatalf("saved in = %q, want substring %q", recordedIn, want)
		}
	}

	recordedOut := readFile(t, filepath.Join(target, "out"))
	if strings.Contains(recordedOut, "Script started on ") {
		t.Fatalf("saved out = %q, want stripped script wrapper", recordedOut)
	}
	for _, want := range []string{visibleHome, "value"} {
		if !strings.Contains(recordedOut, want) {
			t.Fatalf("saved out = %q, want substring %q", recordedOut, want)
		}
	}
	if !strings.Contains(recordedOut, "MISSING") {
		t.Fatalf("saved out = %q, want missing repo confirmation", recordedOut)
	}
	if strings.Contains(recordedOut, "\r\nFOUND\r\n") {
		t.Fatalf("saved out = %q, want repo to stay unavailable", recordedOut)
	}
}

func TestRecordFailsWhenRecorderShellMissing(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	writeFile(t, filepath.Join(root, "miro.toml"), validConfigContent("e2e"))
	mustMkdirAll(t, testDir)
	addFakeRecordDependencies(t, "script")

	target := filepath.Join(testDir, "suite", "spec")
	err := withWorkingDir(t, root, func() error {
		_, err := Record(filepath.Join("suite", "spec"))
		return err
	})
	if err == nil {
		t.Fatal("Record() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "rerun `miro init`") {
		t.Fatalf("Record() error = %q, want rerun init hint", err.Error())
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("Stat(%q) error = %v, want not exists", target, statErr)
	}
}

func withRecordStreams[T any](t *testing.T, input string, fn func(recordIO) T) T {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stdin.txt")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}

	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Fatalf("close record input: %v", err)
		}
	})

	return fn(recordIO{
		in:  file,
		out: ioDiscard{},
		err: &bytes.Buffer{},
	})
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

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	return string(data)
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
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

func requireCommands(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("missing command %q: %v", name, err)
		}
	}
}

func mustWriteRecordShell(t *testing.T, testDir string) {
	t.Helper()

	if err := writeRecordShell(testDir); err != nil {
		t.Fatalf("writeRecordShell(%q) error = %v", testDir, err)
	}
}

func defaultSandboxConfig() map[string]string {
	return map[string]string{
		"visible_home": "/home/test",
	}
}

func containsEnvEntry(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}

	return false
}
