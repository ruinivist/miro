package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mire/internal/testutil"
)

func TestRunShowsHelpWhenNoArgs(t *testing.T) {
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	stdout, stderr := testutil.CaptureOutput(t, func() {
		if got := Run(nil); got != 0 {
			t.Fatalf("Run() code = %d, want %d", got, 0)
		}
	})

	if !strings.Contains(stdout, "A lean CLI E2E testing framework.") {
		t.Fatalf("stdout = %q, want root help", stdout)
	}
	for _, want := range []string{
		"init        Initialise mire in the current project",
		"record      Record a new CLI scenario",
		"rewrite     Refresh recorded CLI output fixtures",
		"test        Replay recorded CLI scenarios",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want listed subcommand %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "completion") {
		t.Fatalf("stdout = %q, want hidden completion command", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestRunCompletionStillWorksWhenHidden(t *testing.T) {
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	stdout, stderr := testutil.CaptureOutput(t, func() {
		if got := Run([]string{"completion", "bash"}); got != 0 {
			t.Fatalf("Run() code = %d, want %d", got, 0)
		}
	})

	if !strings.Contains(stdout, "# bash completion V2 for mire") {
		t.Fatalf("stdout = %q, want bash completion script", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestRunInit(t *testing.T) {
	root := t.TempDir()
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	testutil.WithWorkingDir(t, root, func() struct{} {
		stdout, stderr := testutil.CaptureOutput(t, func() {
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
		return struct{}{}
	})

	if _, err := os.Stat(filepath.Join(root, "mire.toml")); err != nil {
		t.Fatalf("Stat(%q) error = %v", filepath.Join(root, "mire.toml"), err)
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
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	root := t.TempDir()
	wantDir := filepath.Join(root, "e2e")

	testutil.WithWorkingDir(t, root, func() struct{} {
		initStdout, initStderr := testutil.CaptureOutput(t, func() {
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

		stdout, stderr := testutil.CapturePromptedOutput(t, "exit\n", "Save recording?", "y\n", func() {
			if got := Run([]string{"record", "suite/spec"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		createdPath := filepath.Join(wantDir, "suite", "spec")
		if !strings.Contains(stdout, createdPath) {
			t.Fatalf("stdout = %q, want created path", stdout)
		}
		if !strings.Contains(stderr, "Save recording?") {
			t.Fatalf("stderr = %q, want save prompt", stderr)
		}

		for _, name := range []string{"in", "out"} {
			if _, err := os.Stat(filepath.Join(createdPath, name)); err != nil {
				t.Fatalf("Stat(%q) error = %v", filepath.Join(createdPath, name), err)
			}
		}
		return struct{}{}
	})
}

func TestRunRewrite(t *testing.T) {
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	root := t.TempDir()
	testutil.WriteScenarioFixtures(t, filepath.Join(root, "e2e", "suite", "spec"), "echo rewrite\nexit\n", "stale output\n")

	testutil.WithWorkingDir(t, root, func() struct{} {
		initStdout, initStderr := testutil.CaptureOutput(t, func() {
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

		stdout, stderr := testutil.CaptureOutput(t, func() {
			if got := Run([]string{"rewrite"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		for _, want := range []string{
			"RUN suite/spec",
			"PASS suite/spec (",
			"Summary: total=1 passed=1 failed=0",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout = %q, want substring %q", stdout, want)
			}
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		return struct{}{}
	})

	if got := testutil.ReadFile(t, filepath.Join(root, "e2e", "suite", "spec", "out")); got == "stale output\n" {
		t.Fatalf("rewrite left stale output unchanged")
	}
}

func TestRunInitFailsWhenDependenciesMissing(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", "")

	testutil.WithWorkingDir(t, root, func() struct{} {
		stdout, stderr := testutil.CaptureOutput(t, func() {
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
		if _, err := os.Stat(filepath.Join(root, "mire.toml")); !os.IsNotExist(err) {
			t.Fatalf("Stat(%q) error = %v, want not exists", filepath.Join(root, "mire.toml"), err)
		}
		if _, err := os.Stat(filepath.Join(root, "e2e", "shell.sh")); !os.IsNotExist(err) {
			t.Fatalf("Stat(%q) error = %v, want not exists", filepath.Join(root, "e2e", "shell.sh"), err)
		}
		return struct{}{}
	})
}

func TestRunRecordMissingPath(t *testing.T) {
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	stdout, stderr := testutil.CaptureOutput(t, func() {
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
	if !strings.Contains(stderr, "Usage:") || !strings.Contains(stderr, "mire record <path>") {
		t.Fatalf("stderr = %q, want record usage", stderr)
	}
}

func TestRunTest(t *testing.T) {
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	root := t.TempDir()
	testutil.WriteScenarioFixtures(t, filepath.Join(root, "e2e", "a"), "echo one\n", "echo one\r\n")
	testutil.WriteScenarioFixtures(t, filepath.Join(root, "e2e", "nested", "b"), "echo two\n", "echo two\r\n")

	testutil.WithWorkingDir(t, root, func() struct{} {
		initStdout, initStderr := testutil.CaptureOutput(t, func() {
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

		stdout, stderr := testutil.CaptureOutput(t, func() {
			if got := Run([]string{"test"}); got != 0 {
				t.Fatalf("Run() code = %d, want %d", got, 0)
			}
		})

		for _, want := range []string{
			"RUN a",
			"PASS a (",
			" ms)",
			"RUN nested/b",
			"PASS nested/b (",
			"Summary: total=2 passed=2 failed=0",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("stdout = %q, want substring %q", stdout, want)
			}
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		return struct{}{}
	})
}

func TestRunTestExtraArgs(t *testing.T) {
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	stdout, stderr := testutil.CaptureOutput(t, func() {
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
	if !strings.Contains(stderr, "Usage:") || !strings.Contains(stderr, "mire test [path]") {
		t.Fatalf("stderr = %q, want test usage", stderr)
	}
}

func TestRunInitExtraArgs(t *testing.T) {
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	stdout, stderr := testutil.CaptureOutput(t, func() {
		if got := Run([]string{"init", "extra"}); got != 1 {
			t.Fatalf("Run() code = %d, want %d", got, 1)
		}
	})

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "unknown command \"extra\" for \"mire init\"") {
		t.Fatalf("stderr = %q, want extra-arg error", stderr)
	}
	if !strings.Contains(stderr, "Usage:") || !strings.Contains(stderr, "mire init") {
		t.Fatalf("stderr = %q, want init usage", stderr)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	stdout, stderr := testutil.CaptureOutput(t, func() {
		if got := Run([]string{"wat"}); got != 1 {
			t.Fatalf("Run() code = %d, want %d", got, 1)
		}
	})

	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "unknown command \"wat\" for \"mire\"") {
		t.Fatalf("stderr = %q, want unknown command error", stderr)
	}
}

func prefixed(msg string) string {
	return "mire › " + msg
}
