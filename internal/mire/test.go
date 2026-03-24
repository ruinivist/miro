package mire

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"mire/internal/output"
)

type testIO struct {
	out io.Writer
	err io.Writer
}

type testScenario struct {
	dir          string
	relPath      string
	inPath       string
	outPath      string
	setupScripts []string
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

type testMismatchError struct {
	expected []byte
	actual   []byte
	details  mismatchDetails
}

const (
	replayPromptReadyMarker  = compareOutputMarker
	replayPromptReadyTimeout = 2 * time.Second
)

func (e *testMismatchError) Error() string {
	if e.details.lineNumber > 0 {
		return fmt.Sprintf("output differed at line %d", e.details.lineNumber)
	}

	return "output differed"
}

// RunTests replays all scenarios under the configured test directory.
func RunTests(path string) error {
	return runTests(path, testIO{
		out: os.Stdout,
		err: os.Stderr,
	})
}

func runTests(path string, tio testIO) error {
	suite, err := loadReplaySuite(path)
	if err != nil {
		return err
	}

	summary := testSummary{total: len(suite.scenarios)}
	for _, scenario := range suite.scenarios {
		output.Fprintf(tio.out, "%s %s\n", output.Label("RUN", output.Info), scenario.relPath)

		start := time.Now()
		if err := replayScenario(scenario, suite.shellPath, suite.ignoreDiffs, suite.sandboxConfig, suite.mounts, suite.paths); err != nil {
			elapsed := time.Since(start)
			summary.failed++
			output.Fprintf(tio.out, "%s %s (%s): %v\n", output.Label("FAIL", output.Fail), scenario.relPath, formatElapsed(elapsed), err)
			var mismatchErr *testMismatchError
			if errors.As(err, &mismatchErr) {
				writeScenarioMismatch(tio.out, mismatchErr)
			}
			continue
		}

		elapsed := time.Since(start)
		summary.passed++
		output.Fprintf(tio.out, "%s %s (%s)\n", output.Label("PASS", output.Pass), scenario.relPath, formatElapsed(elapsed))
	}

	writeScenarioSummary(tio.out, summary)

	return nil
}

func writeScenarioSummary(w io.Writer, summary testSummary) {
	summaryColor := output.Pass
	if summary.failed > 0 {
		summaryColor = output.Fail
	}

	output.Fprintf(
		w,
		"%s\n",
		output.Label(
			fmt.Sprintf("Summary: total=%d passed=%d failed=%d", summary.total, summary.passed, summary.failed),
			summaryColor,
		),
	)
}

func formatElapsed(elapsed time.Duration) string {
	if elapsed < time.Second {
		return fmt.Sprintf("%d ms", elapsed/time.Millisecond)
	}

	return fmt.Sprintf("%.2f s", elapsed.Seconds())
}

func resolveTestDiscoveryRoot(testDir, path string) (string, error) {
	if path == "" {
		return testDir, nil
	}

	target, err := resolvePathWithinTestDir(testDir, path, "test")
	if err != nil {
		return "", err
	}

	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("test path %q does not exist", target)
		}
		return "", fmt.Errorf("failed to check test path %q: %v", target, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("test path %q is not a directory", target)
	}

	return target, nil
}

func discoverTestScenarios(discoveryRoot, displayRoot string) ([]testScenario, error) {
	fixturesByDir := map[string]testFixtureFiles{}

	if err := filepath.WalkDir(discoveryRoot, func(path string, d fs.DirEntry, walkErr error) error {
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
		return nil, fmt.Errorf("failed to scan test directory %q: %v", discoveryRoot, err)
	}

	scenarios := make([]testScenario, 0, len(fixturesByDir))
	for dir, files := range fixturesByDir {
		relPath, err := filepath.Rel(displayRoot, dir)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve scenario path for %q: %v", dir, err)
		}
		setupScripts, err := discoverSetupScripts(displayRoot, dir)
		if err != nil {
			return nil, err
		}

		switch {
		case files.inPath == "":
			return nil, fmt.Errorf("malformed scenario %q: missing in fixture", relPath)
		case files.outPath == "":
			return nil, fmt.Errorf("malformed scenario %q: missing out fixture", relPath)
		}

		scenarios = append(scenarios, testScenario{
			dir:          dir,
			relPath:      relPath,
			inPath:       files.inPath,
			outPath:      files.outPath,
			setupScripts: setupScripts,
		})
	}

	sort.Slice(scenarios, func(i, j int) bool {
		return scenarios[i].relPath < scenarios[j].relPath
	})

	return scenarios, nil
}

func replayScenario(scenario testScenario, shellPath string, ignoreDiffs []string, sandboxConfig map[string]string, mounts, paths []string) error {
	got, err := replayScenarioOutput(scenario, shellPath, sandboxConfig, mounts, paths)
	if err != nil {
		return err
	}

	return compareReplayedScenarioOutput(scenario, ignoreDiffs, got)
}

func compareReplayedScenarioOutput(scenario testScenario, ignoreDiffs []string, got []byte) error {
	want, err := os.ReadFile(scenario.outPath)
	if err != nil {
		return fmt.Errorf("failed to read expected output: %v", err)
	}

	matched, details := compareOutput(want, got, ignoreDiffs)
	if !matched {
		return &testMismatchError{
			expected: want,
			actual:   got,
			details:  details,
		}
	}

	return nil
}
