package mire

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mire/internal/testutil"
)

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		name     string
		elapsed  time.Duration
		wantText string
	}{
		{name: "sub-second", elapsed: 123 * time.Millisecond, wantText: "123 ms"},
		{name: "boundary", elapsed: time.Second, wantText: "1.00 s"},
		{name: "multi-second", elapsed: 1534 * time.Millisecond, wantText: "1.53 s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatElapsed(tt.elapsed); got != tt.wantText {
				t.Fatalf("formatElapsed(%v) = %q, want %q", tt.elapsed, got, tt.wantText)
			}
		})
	}
}

func TestDiscoverTestScenariosFindsNestedFixturesAndSorts(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "e2e")
	testutil.WriteScenarioFixtures(t, filepath.Join(testDir, "nested", "b"), "echo two\n", "echo two\n")
	testutil.WriteScenarioFixtures(t, filepath.Join(testDir, "a"), "echo one\n", "echo one\n")
	testutil.WriteFile(t, filepath.Join(testDir, "shell.sh"), "#!/bin/sh\n")
	testutil.WriteFile(t, filepath.Join(testDir, "notes.txt"), "ignore me\n")

	got, err := discoverTestScenarios(testDir, testDir)
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
	testutil.WriteFile(t, filepath.Join(testDir, "broken", "in"), "echo broken\n")

	_, err := discoverTestScenarios(testDir, testDir)
	if err == nil {
		t.Fatal("discoverTestScenarios() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `malformed scenario "broken": missing out fixture`) {
		t.Fatalf("discoverTestScenarios() error = %q, want missing out fixture", err.Error())
	}
}

func TestDiscoverTestScenariosRejectsMissingInFixture(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "e2e")
	testutil.WriteFile(t, filepath.Join(testDir, "broken", "out"), "echo broken\n")

	_, err := discoverTestScenarios(testDir, testDir)
	if err == nil {
		t.Fatal("discoverTestScenarios() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `malformed scenario "broken": missing in fixture`) {
		t.Fatalf("discoverTestScenarios() error = %q, want missing in fixture", err.Error())
	}
}

func TestReplayScenarioUsesRecordedInputAndKeepsReplayOutputQuiet(t *testing.T) {
	testutil.AddFakeRecordDependencies(t, "bwrap", "bash")

	testDir := filepath.Join(t.TempDir(), "e2e")
	shellPath := filepath.Join(testDir, "shell.sh")
	mustWriteRecordShell(t, testDir)
	scenarioDir := filepath.Join(testDir, "suite", "spec")
	testutil.WriteScenarioFixtures(t, scenarioDir, "echo replay\nexit\n", "echo replay\r\nexit\r\n")

	err := replayScenario(testScenario{
		dir:          scenarioDir,
		relPath:      filepath.Join("suite", "spec"),
		inPath:       filepath.Join(scenarioDir, "in"),
		outPath:      filepath.Join(scenarioDir, "out"),
		setupScripts: nil,
	}, shellPath, defaultSandboxConfig(), nil, nil)
	if err != nil {
		t.Fatalf("replayScenario() error = %v", err)
	}
}

func TestReplayScenarioWaitsForPromptReadyMarkerBeforeSendingInput(t *testing.T) {
	testutil.RequireCommands(t, "bwrap", "bash")

	testDir := filepath.Join(t.TempDir(), "e2e")
	shellPath := filepath.Join(testDir, "shell.sh")
	mustWriteRecordShell(t, testDir)
	scenarioDir := filepath.Join(testDir, "suite", "spec")
	testutil.WriteScenarioFixtures(
		t,
		scenarioDir,
		"echo ab\x7fc\nexit\n",
		"\x1b[?2004h$ echo ab\b \bc\r\n\x1b[?2004l\rac\r\n\x1b[?2004h$ exit\r\n\x1b[?2004l\rexit\r\n",
	)

	err := replayScenario(testScenario{
		dir:          scenarioDir,
		relPath:      filepath.Join("suite", "spec"),
		inPath:       filepath.Join(scenarioDir, "in"),
		outPath:      filepath.Join(scenarioDir, "out"),
		setupScripts: nil,
	}, shellPath, defaultSandboxConfig(), nil, nil)
	if err != nil {
		t.Fatalf("replayScenario() error = %v", err)
	}
}

func TestReplayScenarioFailsWhenCompareMarkerMissing(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "e2e")
	shellPath := filepath.Join(testDir, "shell.sh")
	testutil.WriteFile(t, shellPath, "#!/bin/sh\n")
	if err := os.Chmod(shellPath, 0o755); err != nil {
		t.Fatalf("Chmod(%q) error = %v", shellPath, err)
	}
	scenarioDir := filepath.Join(testDir, "suite", "spec")
	testutil.WriteScenarioFixtures(t, scenarioDir, "echo replay\nexit\n", "echo replay\nexit\n")

	err := replayScenario(testScenario{
		dir:          scenarioDir,
		relPath:      filepath.Join("suite", "spec"),
		inPath:       filepath.Join(scenarioDir, "in"),
		outPath:      filepath.Join(scenarioDir, "out"),
		setupScripts: nil,
	}, shellPath, defaultSandboxConfig(), nil, nil)
	if err == nil {
		t.Fatal("replayScenario() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "__MIRE_PROMPT_READY__") || !strings.Contains(err.Error(), "rerun `mire init`") {
		t.Fatalf("replayScenario() error = %q, want prompt-ready marker refresh hint", err.Error())
	}
}

func TestDiscoverTestScenariosUsesDisplayRootForRelativePaths(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "e2e")
	scopedDir := filepath.Join(testDir, "nested")
	testutil.WriteFile(t, filepath.Join(testDir, setupScriptName), "export ROOT=1\n")
	testutil.WriteScenarioFixtures(t, filepath.Join(scopedDir, "b"), "echo two\n", "echo two\n")

	got, err := discoverTestScenarios(scopedDir, testDir)
	if err != nil {
		t.Fatalf("discoverTestScenarios() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(discoverTestScenarios()) = %d, want 1", len(got))
	}
	if got[0].relPath != filepath.Join("nested", "b") {
		t.Fatalf("scenario relPath = %q, want %q", got[0].relPath, filepath.Join("nested", "b"))
	}
	if len(got[0].setupScripts) != 1 || got[0].setupScripts[0] != filepath.Join(testDir, setupScriptName) {
		t.Fatalf("scenario setupScripts = %#v, want root setup", got[0].setupScripts)
	}
}

func TestResolveTestDiscoveryRootRejectsMissingPath(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "e2e")
	testutil.MustMkdirAll(t, testDir)

	_, err := resolveTestDiscoveryRoot(testDir, "missing")
	if err == nil {
		t.Fatal("resolveTestDiscoveryRoot() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `does not exist`) {
		t.Fatalf("resolveTestDiscoveryRoot() error = %q, want missing-path error", err.Error())
	}
}

func TestResolveTestDiscoveryRootRejectsFile(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	testutil.WriteFile(t, filepath.Join(testDir, "case.txt"), "hello\n")

	err := testutil.WithWorkingDir(t, root, func() error {
		_, err := resolveTestDiscoveryRoot(testDir, filepath.Join("e2e", "case.txt"))
		return err
	})
	if err == nil {
		t.Fatal("resolveTestDiscoveryRoot() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `is not a directory`) {
		t.Fatalf("resolveTestDiscoveryRoot() error = %q, want directory error", err.Error())
	}
}

func TestRunTestsScopedRunEmptyDirectoryFails(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	emptyDir := filepath.Join(testDir, "empty")
	testutil.WriteValidConfig(t, filepath.Join(root, "mire.toml"), "e2e")
	mustWriteRecordShell(t, testDir)
	testutil.MustMkdirAll(t, emptyDir)

	err := testutil.WithWorkingDir(t, root, func() error {
		return runTests("empty", testIO{
			out: &bytes.Buffer{},
			err: &bytes.Buffer{},
		})
	})
	if err == nil {
		t.Fatal("runTests() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `no test scenarios found in "`) || !strings.Contains(err.Error(), filepath.Join("e2e", "empty")) {
		t.Fatalf("runTests() error = %q, want empty-directory error", err.Error())
	}
}
