package miro

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePathWithinTestDirAcceptsRelativePath(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")

	got := withWorkingDir(t, root, func() string {
		path, err := resolvePathWithinTestDir(testDir, filepath.Join("suite", "spec"), "record")
		if err != nil {
			t.Fatalf("resolvePathWithinTestDir() error = %v", err)
		}
		return path
	})

	want := filepath.Join(testDir, "suite", "spec")
	if got != want {
		t.Fatalf("resolvePathWithinTestDir() = %q, want %q", got, want)
	}
}

func TestResolvePathWithinTestDirAcceptsExplicitTestDirPrefix(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")

	got := withWorkingDir(t, root, func() string {
		path, err := resolvePathWithinTestDir(testDir, filepath.Join("e2e", "suite", "spec"), "record")
		if err != nil {
			t.Fatalf("resolvePathWithinTestDir() error = %v", err)
		}
		return path
	})

	want := filepath.Join(testDir, "suite", "spec")
	if got != want {
		t.Fatalf("resolvePathWithinTestDir() = %q, want %q", got, want)
	}
}

func TestResolvePathWithinTestDirAcceptsAbsolutePathInsideTestDir(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "e2e")
	want := filepath.Join(testDir, "suite", "spec")

	got, err := resolvePathWithinTestDir(testDir, want, "record")
	if err != nil {
		t.Fatalf("resolvePathWithinTestDir() error = %v", err)
	}

	if got != want {
		t.Fatalf("resolvePathWithinTestDir() = %q, want %q", got, want)
	}
}

func TestResolvePathWithinTestDirRejectsAbsolutePathOutsideTestDir(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	outside := filepath.Join(root, "outside", "suite", "spec")

	_, err := resolvePathWithinTestDir(testDir, outside, "record")
	if err == nil {
		t.Fatal("resolvePathWithinTestDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "must be inside test directory") {
		t.Fatalf("resolvePathWithinTestDir() error = %q, want inside test directory error", err.Error())
	}
}

func TestResolvePathWithinTestDirAllowsPathCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "e2e")
	mustMkdirAll(t, testDir)

	got := withWorkingDir(t, testDir, func() string {
		path, err := resolvePathWithinTestDir(testDir, ".", "record")
		if err != nil {
			t.Fatalf("resolvePathWithinTestDir() error = %v", err)
		}
		return path
	})

	want, err := filepath.Abs(testDir)
	if err != nil {
		t.Fatalf("Abs(%q) error = %v", testDir, err)
	}
	if got != want {
		t.Fatalf("resolvePathWithinTestDir() = %q, want %q", got, want)
	}
}

func TestIsWithinBase(t *testing.T) {
	tests := []struct {
		name string
		rel  string
		want bool
	}{
		{name: "dot", rel: ".", want: true},
		{name: "nested", rel: filepath.Join("a", "b"), want: true},
		{name: "parent", rel: "..", want: false},
		{name: "outside", rel: filepath.Join("..", "a"), want: false},
		{name: "same-prefix", rel: ".." + string(os.PathSeparator) + "a", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWithinBase(tt.rel); got != tt.want {
				t.Fatalf("isWithinBase(%q) = %v, want %v", tt.rel, got, tt.want)
			}
		})
	}
}
