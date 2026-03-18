package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	TestDir string
}

type tomlConfig struct {
	TestDir string `toml:"test_dir"`
}

// ReadConfig reads miro.toml.
func ReadConfig(path string) (Config, error) {
	var raw tomlConfig

	meta, err := toml.DecodeFile(path, &raw)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("failed to read %s: %v", path, err)
	}
	if !meta.IsDefined("test_dir") {
		return Config{}, fmt.Errorf("failed to read %s: missing required test_dir", path)
	}
	if raw.TestDir == "" {
		return Config{}, fmt.Errorf("failed to read %s: empty test_dir", path)
	}

	return Config{
		TestDir: raw.TestDir,
	}, nil
}

// WriteConfig writes miro.toml.
func WriteConfig(path string, cfg Config) error {
	if cfg.TestDir == "" {
		return errors.New("empty test_dir")
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(tomlConfig{
		TestDir: cfg.TestDir,
	}); err != nil {
		return fmt.Errorf("failed to encode %s: %v", path, err)
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}
