package mire

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const recordShellName = "shell.sh"

const (
	compareMarkerEnvName      = "MIRE_COMPARE_MARKER"
	compareMarkerEnabledValue = "1"
	compareOutputMarker       = "__MIRE_PROMPT_READY__"
)

//go:embed record_shell.sh
var recordShellFS embed.FS

func recordShellPath(testDir string) string {
	return filepath.Join(testDir, recordShellName)
}

func ensureRecordShell(testDir string) error {
	path := recordShellPath(testDir)
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("recorder shell %q is a directory; remove it and rerun `mire init`", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check recorder shell %q: %v", path, err)
	}

	return writeRecordShell(testDir)
}

func writeRecordShell(testDir string) error {
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		return fmt.Errorf("failed to create test directory %q: %v", testDir, err)
	}

	path := recordShellPath(testDir)
	if err := os.WriteFile(path, []byte(buildRecordShellScript()), 0o644); err != nil {
		return fmt.Errorf("failed to write recorder shell %q: %v", path, err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("failed to make recorder shell executable %q: %v", path, err)
	}

	return nil
}

func resolveRecordShell(testDir string) (string, error) {
	path := recordShellPath(testDir)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("missing recorder shell %q; rerun `mire init` or restore the file", path)
		}
		return "", fmt.Errorf("failed to check recorder shell %q: %v", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("recorder shell %q is a directory; rerun `mire init` or restore the file", path)
	}

	return path, nil
}

func buildRecordShellScript() string {
	body, err := recordShellFS.ReadFile("record_shell.sh")
	if err != nil {
		panic(fmt.Sprintf("read record shell: %v", err))
	}

	return string(body)
}

func recordSessionEnv(sandbox recordSandbox, sandboxConfig map[string]string, setupScripts []string) []string {
	return recordSessionEnvWithExtra(sandbox, sandboxConfig, setupScripts, nil)
}

func recordSessionEnvWithExtra(sandbox recordSandbox, sandboxConfig map[string]string, setupScripts []string, extraEnv map[string]string) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env,
		"MIRE_HOST_HOME="+sandbox.hostHome,
		"MIRE_HOST_TMP="+sandbox.hostTmp,
		"MIRE_PATH_ENV="+sandbox.pathEnv,
		setupScriptsEnvName+"="+strings.Join(setupScripts, "\n"),
	)
	for _, key := range sortedKeys(sandboxConfig) {
		env = append(env, sandboxEnvName(key)+"="+sandboxConfig[key])
	}
	for _, key := range sortedKeys(extraEnv) {
		env = append(env, key+"="+extraEnv[key])
	}

	return env
}

func sandboxEnvName(key string) string {
	return "MIRE_" + strings.ToUpper(key)
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
