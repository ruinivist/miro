package output

import (
	"os"
	"strings"
	"testing"
)

func TestFormatIncludesANSIByDefault(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	got := Format("hello\n")
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("Format() = %q, want ANSI styling", got)
	}
	if !strings.Contains(got, "hello\n") {
		t.Fatalf("Format() = %q, want message body", got)
	}
}

func TestFormatPlainWhenNoColorSet(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	if got := Format("hello\n"); got != "miro › hello\n" {
		t.Fatalf("Format() = %q, want %q", got, "miro › hello\n")
	}
}

func TestLabelsPlainWhenNoColorSet(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	if got := LabelInfo("RUN"); got != "RUN" {
		t.Fatalf("LabelInfo() = %q, want %q", got, "RUN")
	}
	if got := LabelPass("PASS"); got != "PASS" {
		t.Fatalf("LabelPass() = %q, want %q", got, "PASS")
	}
	if got := LabelFail("FAIL"); got != "FAIL" {
		t.Fatalf("LabelFail() = %q, want %q", got, "FAIL")
	}
}

func TestFormatReadsNoColorAtRuntime(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	styled := Format("hello\n")
	if err := os.Setenv("NO_COLOR", "1"); err != nil {
		t.Fatalf("Setenv() error = %v", err)
	}

	plain := Format("hello\n")
	if !strings.Contains(styled, "\x1b[") {
		t.Fatalf("styled Format() = %q, want ANSI styling", styled)
	}
	if plain != "miro › hello\n" {
		t.Fatalf("plain Format() = %q, want %q", plain, "miro › hello\n")
	}
}
