package mire

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"mire/internal/script"
)

type replaySuite struct {
	shellPath     string
	scenarios     []testScenario
	sandboxConfig map[string]string
	mounts        []string
	paths         []string
}

func loadReplaySuite(path string) (replaySuite, error) {
	root, err := currentProjectRoot()
	if err != nil {
		return replaySuite{}, err
	}

	cfg, err := readConfigFromRoot(root)
	if err != nil {
		return replaySuite{}, fmt.Errorf("failed to resolve test directory: %v", err)
	}

	testDir, err := resolveTestDirFromConfig(root, cfg)
	if err != nil {
		return replaySuite{}, fmt.Errorf("failed to resolve test directory: %v", err)
	}

	discoveryRoot, err := resolveTestDiscoveryRoot(testDir, path)
	if err != nil {
		return replaySuite{}, err
	}

	shellPath, err := resolveRecordShell(testDir)
	if err != nil {
		return replaySuite{}, err
	}

	scenarios, err := discoverTestScenarios(discoveryRoot, testDir)
	if err != nil {
		return replaySuite{}, err
	}
	if len(scenarios) == 0 {
		return replaySuite{}, fmt.Errorf("no test scenarios found in %q", discoveryRoot)
	}

	return replaySuite{
		shellPath:     shellPath,
		scenarios:     scenarios,
		sandboxConfig: cfg.Sandbox,
		mounts:        cfg.Mounts,
		paths:         cfg.Paths,
	}, nil
}

func replayScenarioOutput(scenario testScenario, shellPath string, sandboxConfig map[string]string, mounts, paths []string) ([]byte, error) {
	input, err := loadRecordedInput(scenario.inPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read recorded input: %v", err)
	}

	_, rawOut, cleanupFiles, err := newRecordFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to prepare replay files: %v", err)
	}
	defer cleanupFiles()

	sandbox, cleanupSandbox, err := newRecordSandbox()
	if err != nil {
		return nil, fmt.Errorf("failed to prepare replay sandbox: %v", err)
	}
	defer cleanupSandbox()

	rawOutFile, err := os.Create(rawOut)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare replay output: %v", err)
	}

	cmd := exec.Command(shellPath)
	cmd.Dir = scenario.dir
	cmd.Env = recordSessionEnvWithExtra(sandbox, sandboxConfig, mounts, paths, scenario.setupScripts, map[string]string{
		compareMarkerEnvName: compareMarkerEnabledValue,
	})

	ready := make(chan struct{})
	promptWriter := newReplayPromptWriter(rawOutFile, replayPromptReadyMarker, ready)
	promptTimeout := time.AfterFunc(replayPromptReadyTimeout, promptWriter.release)
	defer promptTimeout.Stop()

	replayResult := script.Replay(script.ReplayRequest{
		Cmd:        cmd,
		Input:      input,
		InputReady: ready,
		OutputLog:  promptWriter,
	})
	if err := rawOutFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close replay output: %v", err)
	}
	if !promptWriter.seen {
		if err := replayResult.Err(); err != nil {
			return nil, fmt.Errorf("replay failed: %v", err)
		}
		return nil, fmt.Errorf("replay shell never emitted %q; rerun `mire init` or refresh %q", compareOutputMarker, shellPath)
	}
	if replayResult.OutputErr != nil {
		return nil, fmt.Errorf("replay failed: %v", replayResult.OutputErr)
	}
	if replayResult.InputErr != nil {
		return nil, fmt.Errorf("replay failed: %v", replayResult.InputErr)
	}

	got, err := loadRecordedOutput(rawOut)
	if err != nil {
		return nil, fmt.Errorf("failed to read replay output: %v", err)
	}

	got, err = trimReplayOutputToMarker(got, shellPath)
	if err != nil {
		return nil, err
	}

	return got, nil
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
