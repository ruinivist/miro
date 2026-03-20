package miro

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveRecordTarget(testDir, path string) (string, error) {
	return resolvePathWithinTestDir(testDir, path, "record")
}

func resolvePathWithinTestDir(testDir, path, pathType string) (string, error) {
	absTestDir, err := filepath.Abs(testDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve test directory path: %v", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s path: %v", pathType, err)
	}

	relToTestDir, err := filepath.Rel(absTestDir, absPath)
	if err == nil && isWithinBase(relToTestDir) {
		return filepath.Join(absTestDir, relToTestDir), nil
	}

	if filepath.IsAbs(path) {
		return "", fmt.Errorf("%s path %q must be inside test directory %q", pathType, path, absTestDir)
	}

	return filepath.Join(absTestDir, path), nil
}

func isWithinBase(rel string) bool {
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
