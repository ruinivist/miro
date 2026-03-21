package mire

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"mire/internal/output"
	"mire/internal/screen"
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
}

const (
	replayPromptReadyMarker  = compareOutputMarker
	replayPromptReadyTimeout = 2 * time.Second
)

func (e *testMismatchError) Error() string {
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

	discoveryRoot, err := resolveTestDiscoveryRoot(testDir, path)
	if err != nil {
		return err
	}

	shellPath, err := resolveRecordShell(testDir)
	if err != nil {
		return err
	}

	scenarios, err := discoverTestScenarios(discoveryRoot, testDir)
	if err != nil {
		return err
	}
	if len(scenarios) == 0 {
		return fmt.Errorf("no test scenarios found in %q", discoveryRoot)
	}

	summary := testSummary{total: len(scenarios)}
	for _, scenario := range scenarios {
		output.Fprintf(tio.out, "%s %s\n", output.Label("RUN", output.Info), scenario.relPath)

		start := time.Now()
		if err := replayScenario(scenario, shellPath, tio, cfg.Sandbox); err != nil {
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

	summaryColor := output.Pass
	if summary.failed > 0 {
		summaryColor = output.Fail
	}

	output.Fprintf(
		tio.out,
		"%s\n",
		output.Label(
			fmt.Sprintf("Summary: total=%d passed=%d failed=%d", summary.total, summary.passed, summary.failed),
			summaryColor,
		),
	)

	return nil
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

func replayScenario(scenario testScenario, shellPath string, _ testIO, sandboxConfig map[string]string) error {
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

	rawOutFile, err := os.Create(rawOut)
	if err != nil {
		return fmt.Errorf("failed to prepare replay output: %v", err)
	}

	cmd := exec.Command(shellPath)
	cmd.Dir = scenario.dir
	cmd.Env = recordSessionEnvWithExtra(sandbox, sandboxConfig, scenario.setupScripts, map[string]string{
		compareMarkerEnvName: compareMarkerEnabledValue,
	})

	ready := make(chan struct{})
	promptWriter := newReplayPromptWriter(rawOutFile, replayPromptReadyMarker, ready)
	promptTimeout := time.AfterFunc(replayPromptReadyTimeout, promptWriter.release)
	defer promptTimeout.Stop()

	if err := screen.Replay(screen.ReplayRequest{
		Cmd:        cmd,
		Input:      input,
		InputReady: ready,
		OutputLog:  promptWriter,
	}); err != nil {
		rawOutFile.Close()
		return fmt.Errorf("replay failed: %v", err)
	}
	if err := rawOutFile.Close(); err != nil {
		return fmt.Errorf("failed to close replay output: %v", err)
	}
	if !promptWriter.seen {
		return fmt.Errorf("replay shell never emitted %q; rerun `mire init` or refresh %q", compareOutputMarker, shellPath)
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
		return &testMismatchError{
			expected: want,
			actual:   got,
		}
	}

	return nil
}

func writeScenarioMismatch(w io.Writer, mismatch *testMismatchError) {
	output.Fprintf(w, "Expected:\n%s", string(mismatch.expected))
	if len(mismatch.expected) == 0 || mismatch.expected[len(mismatch.expected)-1] != '\n' {
		output.Fprintf(w, "\n")
	}
	output.Fprintf(w, "Actual:\n%s", string(mismatch.actual))
	if len(mismatch.actual) == 0 || mismatch.actual[len(mismatch.actual)-1] != '\n' {
		output.Fprintf(w, "\n")
	}
}

func trimReplayOutputToMarker(data []byte, shellPath string) ([]byte, error) {
	idx := bytes.Index(data, []byte(compareOutputMarker))
	if idx == -1 {
		return nil, fmt.Errorf(
			"missing replay start marker %q in replay output; rerun `mire init` or refresh %q",
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

type replayPromptWriter struct {
	dst    io.Writer
	marker []byte
	tail   []byte
	ready  chan struct{}
	once   sync.Once
	seen   bool
}

func newReplayPromptWriter(dst io.Writer, marker string, ready chan struct{}) *replayPromptWriter {
	return &replayPromptWriter{
		dst:    dst,
		marker: []byte(marker),
		ready:  ready,
	}
}

func (w *replayPromptWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	if n > 0 {
		w.observe(p[:n])
	}
	return n, err
}

func (w *replayPromptWriter) observe(p []byte) {
	if w.seen || len(w.marker) == 0 {
		return
	}

	data := make([]byte, 0, len(w.tail)+len(p))
	data = append(data, w.tail...)
	data = append(data, p...)

	if bytes.Contains(data, w.marker) {
		w.seen = true
		w.release()
	}

	keep := len(w.marker) - 1
	if keep <= 0 {
		return
	}
	if len(data) <= keep {
		w.tail = append(w.tail[:0], data...)
		return
	}

	w.tail = append(w.tail[:0], data[len(data)-keep:]...)
}

func (w *replayPromptWriter) release() {
	w.once.Do(func() {
		close(w.ready)
	})
}
