package miro

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"miro/internal/output"
)

type testIO struct {
	out io.Writer
	err io.Writer
}

type testScenario struct {
	dir     string
	relPath string
	inPath  string
	outPath string
}

type testSummary struct {
	total  int
	passed int
	failed int
}

type testFixtureFiles struct {
	inPath  string
	outPath string
}

type TestSuiteFailedError struct {
	Failed int
}

func (e TestSuiteFailedError) Error() string {
	return fmt.Sprintf("%d scenario(s) failed", e.Failed)
}

// RunTests replays all scenarios under the configured test directory.
func RunTests() error {
	return runTests(testIO{
		out: os.Stdout,
		err: os.Stderr,
	})
}

func runTests(tio testIO) error {
	root, err := currentProjectRoot()
	if err != nil {
		return err
	}

	cfg, err := readConfigFromRoot(root)
	if err != nil {
		return fmt.Errorf("failed to resolve test directory: %v", err)
	}

	testDir, err := resolveTestDirFromConfig(root, cfg)
	if err != nil {
		return fmt.Errorf("failed to resolve test directory: %v", err)
	}

	shellPath, err := resolveRecordShell(testDir)
	if err != nil {
		return err
	}

	scenarios, err := discoverTestScenarios(testDir)
	if err != nil {
		return err
	}
	if len(scenarios) == 0 {
		return fmt.Errorf("no test scenarios found in %q", testDir)
	}

	summary := testSummary{total: len(scenarios)}
	for _, scenario := range scenarios {
		output.Fprintf(tio.out, "RUN %s\n", scenario.relPath)

		if err := replayScenario(scenario, shellPath, tio, cfg.Sandbox); err != nil {
			summary.failed++
			output.Fprintf(tio.out, "FAIL %s: %v\n", scenario.relPath, err)
			continue
		}

		summary.passed++
		output.Fprintf(tio.out, "PASS %s\n", scenario.relPath)
	}

	output.Fprintf(
		tio.out,
		"Summary: total=%d passed=%d failed=%d\n",
		summary.total,
		summary.passed,
		summary.failed,
	)

	if summary.failed > 0 {
		return TestSuiteFailedError{Failed: summary.failed}
	}

	return nil
}

func discoverTestScenarios(testDir string) ([]testScenario, error) {
	fixturesByDir := map[string]testFixtureFiles{}

	if err := filepath.WalkDir(testDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		base := filepath.Base(path)
		if base != "in" && base != "out" {
			return nil
		}

		dir := filepath.Dir(path)
		files := fixturesByDir[dir]
		if base == "in" {
			files.inPath = path
		} else {
			files.outPath = path
		}
		fixturesByDir[dir] = files

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to scan test directory %q: %v", testDir, err)
	}

	scenarios := make([]testScenario, 0, len(fixturesByDir))
	for dir, files := range fixturesByDir {
		relPath, err := filepath.Rel(testDir, dir)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve scenario path for %q: %v", dir, err)
		}

		switch {
		case files.inPath == "":
			return nil, fmt.Errorf("malformed scenario %q: missing in fixture", relPath)
		case files.outPath == "":
			return nil, fmt.Errorf("malformed scenario %q: missing out fixture", relPath)
		}

		scenarios = append(scenarios, testScenario{
			dir:     dir,
			relPath: relPath,
			inPath:  files.inPath,
			outPath: files.outPath,
		})
	}

	sort.Slice(scenarios, func(i, j int) bool {
		return scenarios[i].relPath < scenarios[j].relPath
	})

	return scenarios, nil
}

func replayScenario(scenario testScenario, shellPath string, tio testIO, sandboxConfig map[string]string) error {
	input, err := loadRecordedInput(scenario.inPath)
	if err != nil {
		return fmt.Errorf("failed to read recorded input: %v", err)
	}

	_, rawOut, cleanupFiles, err := newRecordFiles()
	if err != nil {
		return fmt.Errorf("failed to prepare replay files: %v", err)
	}
	defer cleanupFiles()

	sandbox, cleanupSandbox, err := newRecordSandbox()
	if err != nil {
		return fmt.Errorf("failed to prepare replay sandbox: %v", err)
	}
	defer cleanupSandbox()

	cmd := exec.Command("script", "-q", "-E", "always", "-O", rawOut, "-c", shellPath)
	cmd.Dir = scenario.dir
	cmd.Env = recordSessionEnvWithExtra(sandbox, sandboxConfig, map[string]string{
		compareMarkerEnvName: compareMarkerEnabledValue,
	})
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = tio.out
	cmd.Stderr = tio.err

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("replay failed: %v", err)
	}

	got, err := loadRecordedOutput(rawOut)
	if err != nil {
		return fmt.Errorf("failed to read replay output: %v", err)
	}
	got, err = trimReplayOutputToMarker(got, shellPath)
	if err != nil {
		return err
	}

	want, err := os.ReadFile(scenario.outPath)
	if err != nil {
		return fmt.Errorf("failed to read expected output: %v", err)
	}

	if !bytes.Equal(got, want) {
		return fmt.Errorf("output differed")
	}

	return nil
}

func trimReplayOutputToMarker(data []byte, shellPath string) ([]byte, error) {
	idx := bytes.Index(data, []byte(compareOutputMarker))
	if idx == -1 {
		return nil, fmt.Errorf(
			"missing compare marker %q in replay output; rerun `miro init` or refresh %q",
			compareOutputMarker,
			shellPath,
		)
	}

	data = data[idx+len(compareOutputMarker):]
	switch {
	case bytes.HasPrefix(data, []byte("\r\n")):
		return data[2:], nil
	case bytes.HasPrefix(data, []byte("\n")):
		return data[1:], nil
	default:
		return data, nil
	}
}
