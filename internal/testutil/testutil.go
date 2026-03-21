package testutil

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func CaptureOutput(t *testing.T, fn func()) (string, string) {
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

func CapturePromptedOutput(t *testing.T, sessionInput, promptMarker, promptInput string, fn func()) (string, string) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() stdin error = %v", err)
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() stdout error = %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() stderr error = %v", err)
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdin = stdinReader
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	t.Cleanup(func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	})

	var stdinOnce sync.Once
	closeStdin := func() {
		stdinOnce.Do(func() {
			_ = stdinWriter.Close()
		})
	}
	t.Cleanup(closeStdin)

	var stdoutBuf bytes.Buffer
	stdoutDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(&stdoutBuf, stdoutReader)
		stdoutDone <- err
	}()

	var stderrBuf bytes.Buffer
	var promptOnce sync.Once
	stderrDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 1024)
		for {
			n, readErr := stderrReader.Read(buf)
			if n > 0 {
				if _, err := stderrBuf.Write(buf[:n]); err != nil {
					stderrDone <- err
					return
				}
				if promptMarker != "" && strings.Contains(stderrBuf.String(), promptMarker) {
					promptOnce.Do(func() {
						if _, err := stdinWriter.Write([]byte(promptInput)); err != nil {
							stderrDone <- err
							return
						}
						closeStdin()
					})
				}
			}
			if readErr == nil {
				continue
			}
			if readErr == io.EOF {
				stderrDone <- nil
				return
			}
			stderrDone <- readErr
			return
		}
	}()

	go func() {
		if _, err := stdinWriter.Write([]byte(sessionInput)); err != nil {
			return
		}
		if promptMarker == "" {
			closeStdin()
		}
	}()

	fn()

	closeStdin()
	if err := stdoutWriter.Close(); err != nil {
		t.Fatalf("stdout close error = %v", err)
	}
	if err := stderrWriter.Close(); err != nil {
		t.Fatalf("stderr close error = %v", err)
	}

	if err := <-stdoutDone; err != nil {
		t.Fatalf("stdout copy error = %v", err)
	}
	if err := <-stderrDone; err != nil {
		t.Fatalf("stderr copy error = %v", err)
	}

	return stdoutBuf.String(), stderrBuf.String()
}

func WriteFile(t *testing.T, path, content string) {
	t.Helper()

	MustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func WriteScenarioFixtures(t *testing.T, dir, in, out string) {
	t.Helper()

	WriteFile(t, filepath.Join(dir, "in"), in)
	WriteFile(t, filepath.Join(dir, "out"), out)
}

func ReadFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}

func MustMkdirAll(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
}

func WithWorkingDir[T any](t *testing.T, dir string, fn func() T) T {
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

	return fn()
}

func WithStdin(t *testing.T, input string, fn func()) {
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

func AddFakeRecordDependencies(t *testing.T, names ...string) {
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
if [ -n "$cmd" ] && /bin/grep -q '__MIRE_E2E_BEGIN__' "$cmd" 2>/dev/null; then
  command_has_compare_marker=1
fi
stdin_file=''
if [ "${FAKE_SCRIPT_ECHO_STDIN:-}" = "1" ] || [ -n "${FAKE_SCRIPT_CAPTURE_STDIN_FILE:-}" ]; then
  stdin_file="${TMPDIR:-/tmp}/mire-fake-script-stdin-$$"
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
  if [ "${MIRE_COMPARE_MARKER:-0}" = "1" ] && [ "$command_has_compare_marker" = "1" ]; then
    printf '%s\n' '__MIRE_E2E_BEGIN__'
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
    if [ "${MIRE_COMPARE_MARKER:-0}" = "1" ] && [ "$command_has_compare_marker" = "1" ]; then
      printf '%s\n' '__MIRE_E2E_BEGIN__'
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
		if name == "bwrap" {
			body = `#!/bin/sh
while [ "$#" -gt 0 ]; do
  case "$1" in
    --ro-bind|--bind|--setenv)
      shift 3
      ;;
    --tmpfs|--chdir)
      shift 2
      ;;
    --dev|--proc)
      shift 2
      ;;
    --unshare-pid|--die-with-parent)
      shift
      ;;
    *)
      break
      ;;
  esac
done

if [ "$#" -eq 0 ]; then
  exit 0
fi

exec "$@"
`
		}
		if name == "bash" {
			body = `#!/bin/sh
while [ "$#" -gt 0 ]; do
  case "$1" in
    --noprofile|--norc|-i)
      shift
      ;;
    --rcfile)
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [ "${MIRE_COMPARE_MARKER:-0}" = "1" ]; then
  /bin/stty -echo 2>/dev/null || true
  printf '%s\n' '__MIRE_PROMPT_READY__'
else
  /bin/stty -echo 2>/dev/null || true
fi
while IFS= read -r line || [ -n "$line" ]; do
  printf '%s\n' "$line"
  if [ "$line" = "exit" ]; then
    break
  fi
done
exit 0
`
		}
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}

	oldPath := os.Getenv("PATH")
	if oldPath == "" {
		t.Setenv("PATH", binDir)
		return
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)
}

func RequireCommands(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("missing command %q: %v", name, err)
		}
	}
}

func MustGitInit(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

func DefaultWrittenConfig(testDir string) string {
	return "[mire]\n  test_dir = \"" + testDir + "\"\n\n[sandbox]\n  visible_home = \"/home/test\"\n"
}

func ValidConfigContent(testDir string) string {
	return "[mire]\ntest_dir = \"" + testDir + "\"\n\n[sandbox]\nvisible_home = \"/home/test\"\n"
}
