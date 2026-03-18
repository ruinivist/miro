package miro

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesConfigAtProjectRoot(t *testing.T) {
	root := t.TempDir()

	withWorkingDir(t, root, func() struct{} {
		if err := Init(); err != nil {
			t.Fatalf("Init() error = %v", err)
		}
		return struct{}{}
	})

	got := readFile(t, filepath.Join(root, "miro.toml"))
	if got != "test_dir = \"e2e\"\n" {
		t.Fatalf("config = %q, want %q", got, "test_dir = \"e2e\"\n")
	}
}

func TestInitUsesGitRoot(t *testing.T) {
	root := t.TempDir()
	mustGitInit(t, root)
	subdir := filepath.Join(root, "nested", "dir")
	mustMkdirAll(t, subdir)

	withWorkingDir(t, subdir, func() struct{} {
		if err := Init(); err != nil {
			t.Fatalf("Init() error = %v", err)
		}
		return struct{}{}
	})

	if _, err := os.Stat(filepath.Join(root, "miro.toml")); err != nil {
		t.Fatalf("Stat(%q) error = %v", filepath.Join(root, "miro.toml"), err)
	}
}

func TestInitLeavesExistingValidConfigUntouched(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "miro.toml"), "test_dir = \"custom/suite\"\n")

	withWorkingDir(t, root, func() struct{} {
		if err := Init(); err != nil {
			t.Fatalf("Init() error = %v", err)
		}
		return struct{}{}
	})

	got := readFile(t, filepath.Join(root, "miro.toml"))
	if got != "test_dir = \"custom/suite\"\n" {
		t.Fatalf("config = %q, want %q", got, "test_dir = \"custom/suite\"\n")
	}
}

func TestInitFailsWhenExistingConfigInvalid(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "miro.toml"), "[miro]\ntest_dir = \"e2e\"\n")

	err := withWorkingDir(t, root, func() error {
		return Init()
	})
	if err == nil {
		t.Fatal("Init() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing required test_dir") {
		t.Fatalf("Init() error = %q, want missing test_dir error", err.Error())
	}
}

func TestResolveTestDirFromConfig(t *testing.T) {
	root := t.TempDir()
	configured := filepath.Join(root, "custom", "suite")
	mustMkdirAll(t, configured)
	writeFile(t, filepath.Join(root, "miro.toml"), "test_dir = \"custom/suite\"\n")

	got := withWorkingDir(t, root, func() string {
		path, err := ResolveTestDir()
		if err != nil {
			t.Fatalf("ResolveTestDir() error = %v", err)
		}
		return path
	})

	if got != configured {
		t.Fatalf("ResolveTestDir() = %q, want %q", got, configured)
	}
}

func TestResolveTestDirMissingConfigFails(t *testing.T) {
	root := t.TempDir()

	err := withWorkingDir(t, root, func() error {
		_, err := ResolveTestDir()
		return err
	})

	if err == nil {
		t.Fatal("ResolveTestDir() error = nil, want error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("ResolveTestDir() error = %v, want os.ErrNotExist", err)
	}
}

func TestResolveTestDirMissingTestDirFails(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "miro.toml"), "")

	err := withWorkingDir(t, root, func() error {
		_, err := ResolveTestDir()
		return err
	})

	if err == nil {
		t.Fatal("ResolveTestDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing required test_dir") {
		t.Fatalf("ResolveTestDir() error = %q, want missing required test_dir", err.Error())
	}
}

func TestResolveTestDirEmptyTestDirFails(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "miro.toml"), "test_dir = \"\"\n")

	err := withWorkingDir(t, root, func() error {
		_, err := ResolveTestDir()
		return err
	})

	if err == nil {
		t.Fatal("ResolveTestDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "empty test_dir") {
		t.Fatalf("ResolveTestDir() error = %q, want empty test_dir", err.Error())
	}
}

func TestResolveTestDirMalformedConfigFails(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "miro.toml"), "test_dir = [\n")

	err := withWorkingDir(t, root, func() error {
		_, err := ResolveTestDir()
		return err
	})

	if err == nil {
		t.Fatal("ResolveTestDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to read") {
		t.Fatalf("ResolveTestDir() error = %q, want read failure", err.Error())
	}
}

func TestResolveTestDirConfiguredMissingDirectoryReturnsConfiguredPath(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "missing")
	writeFile(t, filepath.Join(root, "miro.toml"), "test_dir = \"missing\"\n")

	got := withWorkingDir(t, root, func() string {
		path, err := ResolveTestDir()
		if err != nil {
			t.Fatalf("ResolveTestDir() error = %v", err)
		}
		return path
	})

	if got != want {
		t.Fatalf("ResolveTestDir() = %q, want %q", got, want)
	}
}

func TestResolveTestDirConfiguredFileFails(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "case.txt"), "hello\n")
	writeFile(t, filepath.Join(root, "miro.toml"), "test_dir = \"case.txt\"\n")

	err := withWorkingDir(t, root, func() error {
		_, err := ResolveTestDir()
		return err
	})

	if err == nil {
		t.Fatal("ResolveTestDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "configured test_dir is not a directory") {
		t.Fatalf("ResolveTestDir() error = %q, want file path error", err.Error())
	}
}

func TestResolveTestDirUsesGitRoot(t *testing.T) {
	root := t.TempDir()
	mustGitInit(t, root)
	want := filepath.Join(root, "e2e")
	writeFile(t, filepath.Join(root, "miro.toml"), "test_dir = \"e2e\"\n")
	subdir := filepath.Join(root, "nested", "dir")
	mustMkdirAll(t, subdir)

	got := withWorkingDir(t, subdir, func() string {
		path, err := ResolveTestDir()
		if err != nil {
			t.Fatalf("ResolveTestDir() error = %v", err)
		}
		return path
	})

	if got != want {
		t.Fatalf("ResolveTestDir() = %q, want %q", got, want)
	}
}

func withWorkingDir[T any](t *testing.T, dir string, fn func() T) T {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	return fn()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
}

func mustGitInit(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}
