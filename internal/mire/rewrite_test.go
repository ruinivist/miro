package mire

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"mire/internal/testutil"
)

func TestRewriteScopedRunOnlyUpdatesSelectedSubtree(t *testing.T) {
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	testutil.WriteValidConfig(t, filepath.Join(root, "mire.toml"), "e2e")
	mustWriteRecordShell(t, testDir)
	testutil.WriteScenarioFixtures(t, filepath.Join(testDir, "a"), "echo root\nexit\n", "stale root\n")
	testutil.WriteScenarioFixtures(t, filepath.Join(testDir, "nested", "b"), "echo nested\nexit\n", "stale nested\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := testutil.WithWorkingDir(t, root, func() error {
		return rewrite("nested", testIO{
			out: &stdout,
			err: &stderr,
		})
	})
	if err != nil {
		t.Fatalf("rewrite() error = %v", err)
	}

	if strings.Contains(stdout.String(), "RUN a") {
		t.Fatalf("stdout = %q, want scoped rewrite to skip root scenario", stdout.String())
	}
	if got := testutil.ReadFile(t, filepath.Join(testDir, "a", "out")); got != "stale root\n" {
		t.Fatalf("root scenario out = %q, want unchanged stale output", got)
	}
	if got := testutil.ReadFile(t, filepath.Join(testDir, "nested", "b", "out")); got == "stale nested\n" {
		t.Fatalf("nested scenario out = %q, want rewritten output", got)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRewriteContinuesAfterFailureAndReturnsError(t *testing.T) {
	testutil.RequireCommands(t, "bwrap", "bash")

	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	testutil.WriteValidConfig(t, filepath.Join(root, "mire.toml"), "e2e")
	mustWriteRecordShell(t, testDir)
	testutil.WriteScenarioFixtures(t, filepath.Join(testDir, "bad"), "echo bad\nexit\n", "stale bad\n")
	testutil.WriteFile(t, filepath.Join(testDir, "bad", setupScriptName), "exit 1\n")
	testutil.WriteScenarioFixtures(t, filepath.Join(testDir, "ok"), "echo ok\nexit\n", "stale ok\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := testutil.WithWorkingDir(t, root, func() error {
		return rewrite("", testIO{
			out: &stdout,
			err: &stderr,
		})
	})
	if err == nil {
		t.Fatal("rewrite() error = nil, want failure summary error")
	}
	if !strings.Contains(err.Error(), "1 scenario(s) failed to rewrite") {
		t.Fatalf("rewrite() error = %q, want failed rewrite summary", err.Error())
	}
	for _, want := range []string{
		"FAIL bad (",
		"PASS ok (",
		"Summary: total=2 passed=1 failed=1",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
	if got := testutil.ReadFile(t, filepath.Join(testDir, "bad", "out")); got != "stale bad\n" {
		t.Fatalf("bad scenario out = %q, want unchanged stale output", got)
	}
	if got := testutil.ReadFile(t, filepath.Join(testDir, "ok", "out")); got == "stale ok\n" {
		t.Fatalf("ok scenario out = %q, want rewritten output", got)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
