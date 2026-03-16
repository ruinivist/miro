package miro

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveTestDirConfigOverride(t *testing.T) {
	root := t.TempDir()
	configured := filepath.Join(root, "custom", "suite")
	mustMkdirAll(t, configured)
	mustMkdirAll(t, filepath.Join(root, "e2e"))
	writeFile(t, filepath.Join(root, "miro.toml"), "[miro]\ntest_dir = \"custom/suite\"\n")

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

func TestResolveTestDirFallsBackWhenConfigOmitsTestDir(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "e2e")
	mustMkdirAll(t, want)
	writeFile(t, filepath.Join(root, "miro.toml"), "[miro]\n")

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

func TestResolveTestDirFallsBackWhenConfiguredPathEmpty(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "e2e")
	mustMkdirAll(t, want)
	writeFile(t, filepath.Join(root, "miro.toml"), "[miro]\ntest_dir = \"\"\n")

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

func TestResolveTestDirMalformedConfigFails(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "miro.toml"), "[miro]\ntest_dir = [\n")

	err := withWorkingDir(t, root, func() error {
		_, err := ResolveTestDir()
		return err
	})

	if err == nil {
		t.Fatal("ResolveTestDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to read") {
		t.Fatalf("ResolveTestDir() error = %q, want read failure", err)
	}
}

func TestResolveTestDirConfiguredMissingDirectoryReturnsConfiguredPath(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "missing")
	writeFile(t, filepath.Join(root, "miro.toml"), "[miro]\ntest_dir = \"missing\"\n")

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

func TestResolveTestDirConfiguredFileReturnsConfiguredPath(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "case.txt")
	writeFile(t, filepath.Join(root, "case.txt"), "hello\n")
	writeFile(t, filepath.Join(root, "miro.toml"), "[miro]\ntest_dir = \"case.txt\"\n")

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

func TestResolveTestDirFallbackOrder(t *testing.T) {
	root := t.TempDir()
	lower := filepath.Join(root, "tests", "miro")
	higher := filepath.Join(root, "tests", "e2e")
	mustMkdirAll(t, lower)
	mustMkdirAll(t, higher)

	got := withWorkingDir(t, root, func() string {
		path, err := ResolveTestDir()
		if err != nil {
			t.Fatalf("ResolveTestDir() error = %v", err)
		}
		return path
	})

	if got != higher {
		t.Fatalf("ResolveTestDir() = %q, want %q", got, higher)
	}
}

func TestResolveTestDirFallbackToMiroDirectories(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "test", "miro")
	mustMkdirAll(t, want)

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

func TestResolveTestDirUsesGitRoot(t *testing.T) {
	root := t.TempDir()
	mustGitInit(t, root)
	want := filepath.Join(root, "e2e")
	mustMkdirAll(t, want)
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

func TestResolveTestDirReturnsFirstFallbackWhenNothingDetected(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "e2e")

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
