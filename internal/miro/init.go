package miro

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	miroconfig "miro/internal/config"
)

const defaultTestDir = "e2e"

// Init creates the default config when missing and leaves valid config untouched.
func Init() error {
	root, err := currentProjectRoot()
	if err != nil {
		return err
	}

	configPath := filepath.Join(root, "miro.toml")
	if _, err := os.Stat(configPath); err == nil {
		if _, err := miroconfig.ReadConfig(configPath); err != nil {
			return err
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check %s: %v", configPath, err)
	}

	if err := miroconfig.WriteConfig(configPath, miroconfig.Config{
		TestDir: defaultTestDir,
	}); err != nil {
		return fmt.Errorf("failed to write %s: %v", configPath, err)
	}

	return nil
}
