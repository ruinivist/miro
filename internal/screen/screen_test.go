package screen

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestRecordCopiesInputAndOutput(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	go func() {
		_, _ = writer.Write([]byte("hello\n"))
		_ = writer.Close()
	}()

	cmd := exec.Command("sh", "-c", `printf 'ready\n'; read line; printf 'line:%s\n' "$line"`)

	var live bytes.Buffer
	var inputLog bytes.Buffer
	var outputLog bytes.Buffer
	if err := Record(RecordRequest{
		Cmd:       cmd,
		Input:     reader,
		Output:    &live,
		InputLog:  &inputLog,
		OutputLog: &outputLog,
	}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	if got := inputLog.String(); got != "hello\n" {
		t.Fatalf("input log = %q, want %q", got, "hello\n")
	}
	for _, got := range []string{live.String(), outputLog.String()} {
		if !bytes.Contains([]byte(got), []byte("ready")) || !bytes.Contains([]byte(got), []byte("line:hello")) {
			t.Fatalf("output = %q, want ready + line:hello", got)
		}
	}
}

func TestReplayCapturesOutput(t *testing.T) {
	cmd := exec.Command("sh", "-c", `read line; printf '__MIRE_E2E_BEGIN__\nline:%s\n' "$line"`)

	var outputLog bytes.Buffer
	if err := Replay(ReplayRequest{
		Cmd:       cmd,
		Input:     []byte("hello\n"),
		OutputLog: &outputLog,
	}); err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	got := outputLog.String()
	if !bytes.Contains([]byte(got), []byte("__MIRE_E2E_BEGIN__")) || !bytes.Contains([]byte(got), []byte("line:hello")) {
		t.Fatalf("output log = %q, want marker + line:hello", got)
	}
}

func TestReplayWaitsForInputReadySignal(t *testing.T) {
	cmd := exec.Command("sh", "-c", `IFS= read -r line; printf 'line:%s\n' "$line"`)
	ready := make(chan struct{})

	var outputLog bytes.Buffer
	start := time.Now()
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(ready)
	}()

	if err := Replay(ReplayRequest{
		Cmd:        cmd,
		Input:      []byte("hello\n"),
		InputReady: ready,
		OutputLog:  &outputLog,
	}); err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("Replay() elapsed = %v, want >= %v", elapsed, 100*time.Millisecond)
	}
	if !bytes.Contains(outputLog.Bytes(), []byte("line:hello")) {
		t.Fatalf("output log = %q, want line:hello", outputLog.String())
	}
}
