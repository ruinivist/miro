package miro

import (
	"path/filepath"
	"testing"
)

func TestLoadRecordedInputTrimsTrailingNewlineAfterEOF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "in")
	writeFile(t, path, "echo hi\r"+string([]byte{eofByte})+"\n")

	got, err := loadRecordedInput(path)
	if err != nil {
		t.Fatalf("loadRecordedInput() error = %v", err)
	}

	want := "echo hi\r" + string([]byte{eofByte})
	if string(got) != want {
		t.Fatalf("loadRecordedInput() = %q, want %q", string(got), want)
	}
}

func TestLoadRecordedInputLeavesNormalTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "in")
	writeFile(t, path, "echo hi\n")

	got, err := loadRecordedInput(path)
	if err != nil {
		t.Fatalf("loadRecordedInput() error = %v", err)
	}

	if string(got) != "echo hi\n" {
		t.Fatalf("loadRecordedInput() = %q, want %q", string(got), "echo hi\n")
	}
}
