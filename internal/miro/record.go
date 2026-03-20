package miro

import (
	"fmt"
	"os"
)

// Record creates the requested scenario path under the resolved test directory
// and records an interactive shell session into in/out fixtures when saved.
func Record(path string) (string, error) {
	root, err := currentProjectRoot()
	if err != nil {
		return "", err
	}

	cfg, err := readConfigFromRoot(root)
	if err != nil {
		return "", fmt.Errorf("failed to resolve test directory: %v", err)
	}

	testDir, err := resolveTestDirFromConfig(root, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to resolve test directory: %v", err)
	}

	target, err := resolveRecordTarget(testDir, path)
	if err != nil {
		return "", err
	}

	shellPath, err := resolveRecordShell(testDir)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", err
	}

	if err := recordScenario(target, shellPath, recordIO{
		in:  os.Stdin,
		out: os.Stdout,
		err: os.Stderr,
	}, cfg.Sandbox); err != nil {
		return "", err
	}

	return target, nil
}
