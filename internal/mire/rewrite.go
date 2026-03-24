package mire

import (
	"fmt"
	"os"
	"time"

	"mire/internal/output"
)

// Rewrite replays scenarios and refreshes their recorded output fixtures.
func Rewrite(path string) error {
	return rewrite(path, testIO{
		out: os.Stdout,
		err: os.Stderr,
	})
}

func rewrite(path string, tio testIO) error {
	suite, err := loadReplaySuite(path)
	if err != nil {
		return err
	}

	summary := testSummary{total: len(suite.scenarios)}
	for _, scenario := range suite.scenarios {
		output.Fprintf(tio.out, "%s %s\n", output.Label("RUN", output.Info), scenario.relPath)

		start := time.Now()
		got, err := replayScenarioOutput(scenario, suite.shellPath, suite.sandboxConfig, suite.mounts, suite.paths)
		if err != nil {
			elapsed := time.Since(start)
			summary.failed++
			output.Fprintf(tio.out, "%s %s (%s): %v\n", output.Label("FAIL", output.Fail), scenario.relPath, formatElapsed(elapsed), err)
			continue
		}

		if err := os.WriteFile(scenario.outPath, got, 0o644); err != nil {
			elapsed := time.Since(start)
			summary.failed++
			output.Fprintf(tio.out, "%s %s (%s): failed to write expected output: %v\n", output.Label("FAIL", output.Fail), scenario.relPath, formatElapsed(elapsed), err)
			continue
		}

		elapsed := time.Since(start)
		summary.passed++
		output.Fprintf(tio.out, "%s %s (%s)\n", output.Label("PASS", output.Pass), scenario.relPath, formatElapsed(elapsed))
	}

	writeScenarioSummary(tio.out, summary)

	if summary.failed > 0 {
		return fmt.Errorf("%d scenario(s) failed to rewrite", summary.failed)
	}

	return nil
}
