package mire

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mire/internal/testutil"
)

func TestRecordCreatesRelativePath(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	testutil.MustMkdirAll(t, testDir)
	testutil.WriteValidConfig(t, filepath.Join(root, "mire.toml"), "e2e")
	mustWriteRecordShell(t, testDir)
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	var got string
	testutil.WithWorkingDir(t, root, func() struct{} {
		testutil.CapturePromptedOutput(t, "exit\n", "Save recording?", "y\n", func() {
			path, err := Record(filepath.Join("a", "b", "c")+string(os.PathSeparator), RecordOptions{})
			if err != nil {
				t.Fatalf("Record() error = %v", err)
			}
			got = path
		})
		return struct{}{}
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

	gotIn, err := loadRecordedInput(filepath.Join(want, "in"))
	if err != nil {
		t.Fatalf("loadRecordedInput(saved in) error = %v", err)
	}
	if string(gotIn) != "exit\n" {
		t.Fatalf("saved in = %q, want replay input %q", string(gotIn), "exit\n")
	}
	if got := testutil.ReadFile(t, filepath.Join(want, "out")); got != "exit\r\nexit\r\n" {
		t.Fatalf("saved out = %q, want %q", got, "exit\\r\\nexit\\r\\n")
	}
}

func TestRecordReturnsDiscardedErrorWhenSaveDeclined(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	testutil.MustMkdirAll(t, testDir)
	mustWriteRecordShell(t, testDir)
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	err := testutil.WithWorkingDir(t, root, func() error {
		target := filepath.Join(testDir, "a", "b", "c")
		testutil.MustMkdirAll(t, target)
		return withRecordStreams(t, "exit\nn\n", func(rio recordIO) error {
			return recordScenario(target, recordShellPath(testDir), rio, defaultSandboxConfig(), nil, nil, nil, false)
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
	testutil.MustMkdirAll(t, target)
	mustWriteRecordShell(t, testDir)
	testutil.WriteFile(t, filepath.Join(target, "in"), "existing in\n")
	testutil.WriteFile(t, filepath.Join(target, "out"), "existing out\n")

	err := testutil.WithWorkingDir(t, root, func() error {
		return withRecordStreams(t, "n\n", func(rio recordIO) error {
			return recordScenario(target, recordShellPath(testDir), rio, defaultSandboxConfig(), nil, nil, nil, false)
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
		"host_home=${MIRE_HOST_HOME:?}",
		"host_tmp=${MIRE_HOST_TMP:?}",
		"path_env=${MIRE_PATH_ENV:?}",
		"visible_home=${MIRE_HOME:?}",
		"bootstrap_rc=\"$host_home/.mire-shell-rc\"",
		"setup_scripts_dir='/tmp/mire-setup-scripts'",
		"visible_bin_dir='/tmp/mire/bin'",
		"visible_bootstrap_rc=\"$visible_home/.mire-shell-rc\"",
		"for path in /tmp/mire-setup-scripts/*.sh; do",
		"source \"$path\"",
		`if [ -n "${MIRE_MOUNTS:-}" ]; then`,
		`while IFS= read -r mount || [ -n "$mount" ]; do`,
		`host_path=${mount%%:*}`,
		`sandbox_path=${mount#*:}`,
		`set -- "$@" --ro-bind "$host_path" "$sandbox_path"`,
		"${MIRE_MOUNTS-}",
		`if [ -n "${MIRE_PATHS:-}" ]; then`,
		`visible_path=$visible_bin_dir/${host_path##*/}`,
		`set -- "$@" --ro-bind "$host_path" "$visible_path"`,
		"${MIRE_PATHS-}",
		`if [ -n "${MIRE_SETUP_SCRIPTS:-}" ]; then`,
		"i=1",
		`while IFS= read -r host_path || [ -n "$host_path" ]; do`,
		`visible_path=$(printf '%s/%03d.sh' "$setup_scripts_dir" "$i")`,
		`set -- "$@" --ro-bind "$host_path" "$visible_path"`,
		"${MIRE_SETUP_SCRIPTS-}",
		`if [ "${MIRE_COMPARE_MARKER:-0}" = "1" ]; then`,
		"printf '__MIRE_PROMPT_READY__\\n'",
		"PROMPT_COMMAND=__mire_prompt_ready",
		"unset MIRE_COMPARE_MARKER",
		"--bind \"$host_home\" \"$visible_home\"",
		"--bind \"$host_tmp\" '/tmp'",
		"--setenv HOME \"$visible_home\"",
		"--setenv PATH \"$visible_bin_dir:$path_env\"",
		"--setenv PS1 '$ '",
		"--setenv TERM 'xterm-256color'",
		"--setenv TZ 'UTC'",
		"--chdir \"$visible_home\"",
		"--dir /tmp/mire",
		"--dir \"$visible_bin_dir\"",
		"exec bwrap \"$@\" bash --noprofile --rcfile \"$visible_bootstrap_rc\" -i",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("wrapper = %q, want substring %q", body, want)
		}
	}
	if strings.Contains(body, "printf '__MIRE_E2E_BEGIN__\\n'") {
		t.Fatalf("wrapper = %q, want old compare marker removed", body)
	}
}

func TestRecordSessionEnvIncludesConfiguredSandboxEnv(t *testing.T) {
	env := recordSessionEnv(recordSandbox{
		hostHome: "/tmp/host-home",
		hostTmp:  "/tmp/host-tmp",
		pathEnv:  "/tmp/bin",
	}, map[string]string{
		"home":     "/sandbox/home",
		"key_word": "value",
	}, []string{"/host/data:/sandbox/data", "/host/cache:/sandbox/cache"}, []string{"/host/bin/mend", "/host/bin/foo"}, []string{"/repo/e2e/setup.sh", "/repo/e2e/suite/setup.sh"})

	for _, want := range []string{
		"MIRE_HOST_HOME=/tmp/host-home",
		"MIRE_HOST_TMP=/tmp/host-tmp",
		"MIRE_PATH_ENV=/tmp/bin",
		"MIRE_MOUNTS=/host/data:/sandbox/data\n/host/cache:/sandbox/cache",
		"MIRE_PATHS=/host/bin/mend\n/host/bin/foo",
		"MIRE_KEY_WORD=value",
		"MIRE_HOME=/sandbox/home",
		"MIRE_SETUP_SCRIPTS=/repo/e2e/setup.sh\n/repo/e2e/suite/setup.sh",
	} {
		if !containsEnvEntry(env, want) {
			t.Fatalf("env = %#v, want entry %q", env, want)
		}
	}
	if containsEnvKey(env, "MIRE_SETUP_SCRIPT_BINDS") {
		t.Fatalf("env = %#v, want MIRE_SETUP_SCRIPT_BINDS omitted", env)
	}
}

func TestRecordSessionEnvWithExtraIncludesAdditionalEntries(t *testing.T) {
	env := recordSessionEnvWithExtra(recordSandbox{
		hostHome: "/tmp/host-home",
		hostTmp:  "/tmp/host-tmp",
		pathEnv:  "/tmp/bin",
	}, map[string]string{
		"home": "/sandbox/home",
	}, []string{"/host/data:/sandbox/data"}, []string{"/host/bin/mend"}, []string{"/repo/e2e/setup.sh"}, map[string]string{
		compareMarkerEnvName: compareMarkerEnabledValue,
	})

	for _, want := range []string{
		"MIRE_HOST_HOME=/tmp/host-home",
		"MIRE_HOST_TMP=/tmp/host-tmp",
		"MIRE_PATH_ENV=/tmp/bin",
		"MIRE_MOUNTS=/host/data:/sandbox/data",
		"MIRE_PATHS=/host/bin/mend",
		"MIRE_HOME=/sandbox/home",
		"MIRE_SETUP_SCRIPTS=/repo/e2e/setup.sh",
		"MIRE_COMPARE_MARKER=1",
	} {
		if !containsEnvEntry(env, want) {
			t.Fatalf("env = %#v, want entry %q", env, want)
		}
	}
	if containsEnvKey(env, "MIRE_SETUP_SCRIPT_BINDS") {
		t.Fatalf("env = %#v, want MIRE_SETUP_SCRIPT_BINDS omitted", env)
	}
}

func TestVerifyRecordedScenarioLeavesInputUnchangedWhenFastWins(t *testing.T) {
	target := t.TempDir()
	inPath := filepath.Join(target, "in")
	recordedIn := []byte("echo hi\n")
	testutil.WriteFile(t, inPath, string(recordedIn))

	err := verifyRecordedScenario(target, recordedIn, 100*time.Millisecond, func(opts replayOptions) error {
		if opts.runWithBreaks {
			time.Sleep(20 * time.Millisecond)
			return nil
		}

		time.Sleep(5 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("verifyRecordedScenario() error = %v", err)
	}

	if got := testutil.ReadFile(t, inPath); got != string(recordedIn) {
		t.Fatalf("saved in = %q, want %q", got, string(recordedIn))
	}
}

func TestVerifyRecordedScenarioWritesMarkerWhenBreakModeWinsAfterFastFailure(t *testing.T) {
	target := t.TempDir()
	inPath := filepath.Join(target, "in")
	recordedIn := []byte("echo hi\n")
	testutil.WriteFile(t, inPath, string(recordedIn))

	err := verifyRecordedScenario(target, recordedIn, 100*time.Millisecond, func(opts replayOptions) error {
		if opts.runWithBreaks {
			time.Sleep(20 * time.Millisecond)
			return nil
		}

		time.Sleep(5 * time.Millisecond)
		return errors.New("fast failed")
	})
	if err != nil {
		t.Fatalf("verifyRecordedScenario() error = %v", err)
	}

	if got := testutil.ReadFile(t, inPath); got != runWithBreaksMarker+"\n"+string(recordedIn) {
		t.Fatalf("saved in = %q, want slow marker prefix", got)
	}
}

func TestVerifyRecordedScenarioWritesMarkerWhenBreakModeSucceedsFirst(t *testing.T) {
	target := t.TempDir()
	inPath := filepath.Join(target, "in")
	recordedIn := []byte("echo hi\n")
	testutil.WriteFile(t, inPath, string(recordedIn))

	err := verifyRecordedScenario(target, recordedIn, 100*time.Millisecond, func(opts replayOptions) error {
		if opts.runWithBreaks {
			time.Sleep(5 * time.Millisecond)
			return nil
		}

		time.Sleep(40 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("verifyRecordedScenario() error = %v", err)
	}

	if got := testutil.ReadFile(t, inPath); got != runWithBreaksMarker+"\n"+string(recordedIn) {
		t.Fatalf("saved in = %q, want slow marker prefix", got)
	}
}

func TestVerifyRecordedScenarioFailsWhenBothModesFail(t *testing.T) {
	target := t.TempDir()
	inPath := filepath.Join(target, "in")
	recordedIn := []byte("echo hi\n")
	testutil.WriteFile(t, inPath, string(recordedIn))

	err := verifyRecordedScenario(target, recordedIn, 100*time.Millisecond, func(opts replayOptions) error {
		if opts.runWithBreaks {
			time.Sleep(10 * time.Millisecond)
			return errors.New("slow failed")
		}

		time.Sleep(5 * time.Millisecond)
		return errors.New("fast failed")
	})
	if !errors.Is(err, errInternalMireFailure) {
		t.Fatalf("verifyRecordedScenario() error = %v, want errInternalMireFailure", err)
	}

	if got := testutil.ReadFile(t, inPath); got != string(recordedIn) {
		t.Fatalf("saved in = %q, want unchanged input", got)
	}
}

func TestRunRecordSessionCapturesInputAndOutput(t *testing.T) {
	shellPath := filepath.Join(t.TempDir(), "shell.sh")
	testutil.WriteFile(t, shellPath, "#!/bin/sh\nprintf 'ready\\n'\nread line\nprintf 'seen:%s\\n' \"$line\"\n")
	if err := os.Chmod(shellPath, 0o755); err != nil {
		t.Fatalf("Chmod(%q) error = %v", shellPath, err)
	}

	sandbox, cleanup, err := newRecordSandboxForPathEnv(os.Getenv("PATH"))
	if err != nil {
		t.Fatalf("newRecordSandboxForPathEnv() error = %v", err)
	}
	defer cleanup()

	rawIn := filepath.Join(t.TempDir(), "raw.in")
	rawOut := filepath.Join(t.TempDir(), "raw.out")
	err = withRecordStreams(t, "hello\n", func(rio recordIO) error {
		return runRecordSession(t.TempDir(), rawIn, rawOut, shellPath, sandbox, rio, defaultSandboxConfig(), nil, nil, nil)
	})
	if err != nil {
		t.Fatalf("runRecordSession() error = %v", err)
	}

	if got := testutil.ReadFile(t, rawIn); got != "hello\n" {
		t.Fatalf("raw in = %q, want %q", got, "hello\n")
	}

	rawOutput := testutil.ReadFile(t, rawOut)
	for _, want := range []string{"ready", "seen:hello"} {
		if !strings.Contains(rawOutput, want) {
			t.Fatalf("raw out = %q, want substring %q", rawOutput, want)
		}
	}
}

func TestRecordScenarioUsesDeterministicSandbox(t *testing.T) {
	testutil.RequireCommands(t, "bwrap", "bash")

	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	target := filepath.Join(testDir, "suite", "spec")
	testutil.MustMkdirAll(t, target)
	mustWriteRecordShell(t, testDir)
	sandboxConfig := map[string]string{
		"home":     "/home/test",
		"key_word": "value",
	}
	visibleHome := sandboxConfig["home"]

	err := testutil.WithWorkingDir(t, root, func() error {
		return withPromptedRecordStreams(
			t,
			"pwd\necho \"$HOME\"\necho \"$MIRE_KEY_WORD\"\nif [ -e \"$HOME/repo\" ]; then echo FOUND; else echo MISSING; fi\npwd\nexit\n",
			"y\n",
			func(rio recordIO) error {
				rio.out = ioDiscard{}
				return recordScenario(target, recordShellPath(testDir), rio, sandboxConfig, nil, nil, nil, false)
			},
		)
	})
	if err != nil {
		t.Fatalf("recordScenario() error = %v", err)
	}

	recordedIn := testutil.ReadFile(t, filepath.Join(target, "in"))
	if strings.Contains(recordedIn, "Script started on ") {
		t.Fatalf("saved in = %q, want stripped script wrapper", recordedIn)
	}
	for _, want := range []string{"pwd\n", "echo \"$HOME\"\n", "echo \"$MIRE_KEY_WORD\"\n", "if [ -e \"$HOME/repo\" ]; then echo FOUND; else echo MISSING; fi\n", "exit\n"} {
		if !strings.Contains(recordedIn, want) {
			t.Fatalf("saved in = %q, want substring %q", recordedIn, want)
		}
	}

	recordedOut := testutil.ReadFile(t, filepath.Join(target, "out"))
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

func TestRecordScenarioStripsInterruptsFromSavedFixtures(t *testing.T) {
	testutil.RequireCommands(t, "bwrap", "bash")

	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	target := filepath.Join(testDir, "suite", "spec")
	testutil.MustMkdirAll(t, target)
	mustWriteRecordShell(t, testDir)

	err := testutil.WithWorkingDir(t, root, func() error {
		reader, writer, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatalf("os.Pipe() error = %v", pipeErr)
		}
		t.Cleanup(func() {
			_ = reader.Close()
			_ = writer.Close()
		})

		errWriter := &promptSignalBuffer{
			marker: "Save recording?",
			ready:  make(chan struct{}),
		}

		writeDone := make(chan error, 1)
		go func() {
			defer close(writeDone)
			defer writer.Close()

			for _, chunk := range []struct {
				data  string
				delay time.Duration
			}{
				{data: "echo a\n", delay: 100 * time.Millisecond},
				{data: "echo ", delay: 100 * time.Millisecond},
				{data: "\x03", delay: 100 * time.Millisecond},
				{data: "exit\n", delay: 0},
			} {
				if _, err := writer.Write([]byte(chunk.data)); err != nil {
					writeDone <- err
					return
				}
				if chunk.delay > 0 {
					time.Sleep(chunk.delay)
				}
			}

			<-errWriter.ready

			if _, err := writer.Write([]byte("y\n")); err != nil {
				writeDone <- err
				return
			}

			writeDone <- nil
		}()

		err := recordScenario(target, recordShellPath(testDir), recordIO{
			in:  reader,
			out: ioDiscard{},
			err: errWriter,
		}, defaultSandboxConfig(), nil, nil, nil, false)
		if writeErr := <-writeDone; writeErr != nil {
			t.Fatalf("write record input: %v", writeErr)
		}
		return err
	})
	if err != nil {
		t.Fatalf("recordScenario() error = %v", err)
	}

	recordedIn := testutil.ReadFile(t, filepath.Join(target, "in"))
	if recordedIn != "echo a\nexit\n" {
		t.Fatalf("saved in = %q, want %q", recordedIn, "echo a\nexit\n")
	}

	recordedOut := testutil.ReadFile(t, filepath.Join(target, "out"))
	if strings.Contains(recordedOut, "^C") {
		t.Fatalf("saved out = %q, want interrupt output removed", recordedOut)
	}
	if strings.Contains(recordedOut, "echo exit\r\n") {
		t.Fatalf("saved out = %q, want interrupted line removed before exit", recordedOut)
	}
	if !strings.Contains(recordedOut, "exit\r\n") {
		t.Fatalf("saved out = %q, want exit preserved", recordedOut)
	}
}

func TestRecordFailsWhenRecorderShellMissing(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	testutil.WriteValidConfig(t, filepath.Join(root, "mire.toml"), "e2e")
	testutil.MustMkdirAll(t, testDir)

	target := filepath.Join(testDir, "suite", "spec")
	err := testutil.WithWorkingDir(t, root, func() error {
		_, err := Record(filepath.Join("suite", "spec"), RecordOptions{})
		return err
	})
	if err == nil {
		t.Fatal("Record() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "rerun `mire init`") {
		t.Fatalf("Record() error = %q, want rerun init hint", err.Error())
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("Stat(%q) error = %v, want not exists", target, statErr)
	}
}
