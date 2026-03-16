package miro

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	miroconfig "miro/internal/config"
)

var fallbackTestDirs = []string{
	"e2e",
	filepath.Join("test", "e2e"),
	filepath.Join("tests", "e2e"),
	"miro",
	filepath.Join("test", "miro"),
	filepath.Join("tests", "miro"),
}

// ResolveTestDir resolves the project test directory from config or conventions.
func ResolveTestDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %v", err)
	}

	root := projectRoot(cwd)
	return resolveTestDirFromRoot(root)
}

func resolveTestDirFromRoot(root string) (string, error) {
	candidates, err := testDirCandidates(root)
	if err != nil {
		return "", err
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("failed to check test directory %s: %v", candidate, err)
		}
	}

	if len(candidates) == 0 {
		return "", errors.New("no test directory candidates available")
	}

	return candidates[0], nil
}

// returns the git root if in a git repo else current pwd
func projectRoot(cwd string) string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return cwd
	}

	root := strings.TrimSpace(string(out))
	if root == "" {
		return cwd
	}
	return filepath.Clean(root)
}

// list of e2e test directory candidates. uses from config if it exists
// else uses fallback paths
func testDirCandidates(root string) ([]string, error) {
	configPath := filepath.Join(root, "miro.toml")

	cfg, err := miroconfig.ReadConfig(configPath)
	if err == nil && cfg.TestDir != "" {
		return []string{filepath.Join(root, cfg.TestDir)}, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	candidates := make([]string, 0, len(fallbackTestDirs))
	for _, candidate := range fallbackTestDirs {
		candidates = append(candidates, filepath.Join(root, candidate))
	}

	return candidates, nil
}
