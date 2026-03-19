package miro

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverTestScenariosFindsNestedFixturesAndSorts(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "e2e")
	writeScenarioFixtures(t, filepath.Join(testDir, "nested", "b"), "echo two\n", "echo two\n")
	writeScenarioFixtures(t, filepath.Join(testDir, "a"), "echo one\n", "echo one\n")
	writeFile(t, filepath.Join(testDir, "shell.sh"), "#!/bin/sh\n")
	writeFile(t, filepath.Join(testDir, "notes.txt"), "ignore me\n")

	got, err := discoverTestScenarios(testDir)
	if err != nil {
		t.Fatalf("discoverTestScenarios() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(discoverTestScenarios()) = %d, want 2", len(got))
	}

	want := []struct {
		relPath string
		dir     string
	}{
		{relPath: "a", dir: filepath.Join(testDir, "a")},
		{relPath: filepath.Join("nested", "b"), dir: filepath.Join(testDir, "nested", "b")},
	}
	for i, tc := range want {
		if got[i].relPath != tc.relPath {
			t.Fatalf("scenario[%d].relPath = %q, want %q", i, got[i].relPath, tc.relPath)
		}
		if got[i].dir != tc.dir {
			t.Fatalf("scenario[%d].dir = %q, want %q", i, got[i].dir, tc.dir)
		}
	}
}

func TestDiscoverTestScenariosRejectsMissingOutFixture(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "e2e")
	writeFile(t, filepath.Join(testDir, "broken", "in"), "echo broken\n")

	_, err := discoverTestScenarios(testDir)
	if err == nil {
		t.Fatal("discoverTestScenarios() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `malformed scenario "broken": missing out fixture`) {
		t.Fatalf("discoverTestScenarios() error = %q, want missing out fixture", err.Error())
	}
}

func TestDiscoverTestScenariosRejectsMissingInFixture(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "e2e")
	writeFile(t, filepath.Join(testDir, "broken", "out"), "echo broken\n")

	_, err := discoverTestScenarios(testDir)
	if err == nil {
		t.Fatal("discoverTestScenarios() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `malformed scenario "broken": missing in fixture`) {
		t.Fatalf("discoverTestScenarios() error = %q, want missing in fixture", err.Error())
	}
}

func TestReplayScenarioUsesRecordedInputAndStripsScriptWrapper(t *testing.T) {
	addFakeRecordDependencies(t, "script")
	t.Setenv("FAKE_SCRIPT_ECHO_STDIN", "1")

	testDir := filepath.Join(t.TempDir(), "e2e")
	shellPath := filepath.Join(testDir, "shell.sh")
	mustWriteRecordShell(t, testDir)
	scenarioDir := filepath.Join(testDir, "suite", "spec")
	writeScenarioFixtures(t, scenarioDir, "echo replay\nexit\n", "echo replay\nexit\n")

	capturedInput := filepath.Join(t.TempDir(), "stdin.capture")
	t.Setenv("FAKE_SCRIPT_CAPTURE_STDIN_FILE", capturedInput)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := replayScenario(testScenario{
		dir:     scenarioDir,
		relPath: filepath.Join("suite", "spec"),
		inPath:  filepath.Join(scenarioDir, "in"),
		outPath: filepath.Join(scenarioDir, "out"),
	}, shellPath, testIO{
		out: &stdout,
		err: &stderr,
	}, defaultSandboxConfig())
	if err != nil {
		t.Fatalf("replayScenario() error = %v", err)
	}

	if got := readFile(t, capturedInput); got != "echo replay\nexit\n" {
		t.Fatalf("captured stdin = %q, want recorded input", got)
	}
	if got := stdout.String(); !strings.Contains(got, "echo replay\nexit\n") {
		t.Fatalf("stdout = %q, want streamed replay output", got)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestReplayScenarioFailsWhenCompareMarkerMissing(t *testing.T) {
	addFakeRecordDependencies(t, "script")
	t.Setenv("FAKE_SCRIPT_ECHO_STDIN", "1")

	testDir := filepath.Join(t.TempDir(), "e2e")
	shellPath := filepath.Join(testDir, "shell.sh")
	writeFile(t, shellPath, "#!/bin/sh\n")
	scenarioDir := filepath.Join(testDir, "suite", "spec")
	writeScenarioFixtures(t, scenarioDir, "echo replay\nexit\n", "echo replay\nexit\n")

	err := replayScenario(testScenario{
		dir:     scenarioDir,
		relPath: filepath.Join("suite", "spec"),
		inPath:  filepath.Join(scenarioDir, "in"),
		outPath: filepath.Join(scenarioDir, "out"),
	}, shellPath, testIO{
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
	}, defaultSandboxConfig())
	if err == nil {
		t.Fatal("replayScenario() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing compare marker") || !strings.Contains(err.Error(), "rerun `miro init`") {
		t.Fatalf("replayScenario() error = %q, want compare marker refresh hint", err.Error())
	}
}

func TestRunTestsRunsFullSuiteAndSummarizesFailures(t *testing.T) {
	addFakeRecordDependencies(t, "script")
	t.Setenv("FAKE_SCRIPT_ECHO_STDIN", "1")

	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	writeFile(t, filepath.Join(root, "miro.toml"), validConfigContent("e2e"))
	mustWriteRecordShell(t, testDir)
	writeScenarioFixtures(t, filepath.Join(testDir, "a"), "echo one\n", "echo one\n")
	writeScenarioFixtures(t, filepath.Join(testDir, "b"), "echo two\n", "different output\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := withWorkingDir(t, root, func() error {
		return runTests(testIO{
			out: &stdout,
			err: &stderr,
		})
	})

	var suiteErr TestSuiteFailedError
	if !errors.As(err, &suiteErr) {
		t.Fatalf("runTests() error = %v, want TestSuiteFailedError", err)
	}
	if suiteErr.Failed != 1 {
		t.Fatalf("TestSuiteFailedError.Failed = %d, want 1", suiteErr.Failed)
	}

	for _, want := range []string{
		"RUN a",
		"PASS a",
		"RUN b",
		"FAIL b: output differed",
		"Summary: total=2 passed=1 failed=1",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want substring %q", stdout.String(), want)
		}
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func writeScenarioFixtures(t *testing.T, dir, in, out string) {
	t.Helper()

	writeFile(t, filepath.Join(dir, "in"), in)
	writeFile(t, filepath.Join(dir, "out"), out)
}
