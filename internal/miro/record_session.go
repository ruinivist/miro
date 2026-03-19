package miro

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"miro/internal/output"
)

const (
	recordVisibleHome = "/home/test"
	recordGitDate     = "2024-01-01T00:00:00Z"
)

type recordIO struct {
	in  io.Reader
	out io.Writer
	err io.Writer
}

func recordScenario(target, shellPath string, rio recordIO) error {
	rawIn, rawOut, cleanup, err := newRecordFiles()
	if err != nil {
		return err
	}
	defer cleanup()

	sandbox, cleanupSandbox, err := newRecordSandbox()
	if err != nil {
		return err
	}
	defer cleanupSandbox()

	overwrite, err := confirmRecordOverwrite(target, rio)
	if err != nil {
		return err
	}
	if !overwrite {
		return ErrRecordingDiscarded
	}

	output.Fprintln(rio.err, "Run commands in the recorder shell, then type exit to finish.")

	if err := runRecordSession(target, rawIn, rawOut, shellPath, sandbox, rio); err != nil {
		return err
	}

	save, err := confirmRecordSave(rio)
	if err != nil {
		return err
	}
	if !save {
		return ErrRecordingDiscarded
	}

	recordedIn, recordedOut, err := loadRecordedFixtures(rawIn, rawOut)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(target, "in"), recordedIn, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(target, "out"), recordedOut, 0o644); err != nil {
		return err
	}

	return nil
}

func newRecordFiles() (string, string, func(), error) {
	dir, err := os.MkdirTemp("", "miro-record-")
	if err != nil {
		return "", "", nil, err
	}

	cleanup := func() {
		_ = os.RemoveAll(dir)
	}

	return filepath.Join(dir, "in"), filepath.Join(dir, "out"), cleanup, nil
}

type recordSandbox struct {
	hostHome string
	hostTmp  string
	pathEnv  string
}

func newRecordSandbox() (recordSandbox, func(), error) {
	return newRecordSandboxForPathEnv(os.Getenv("PATH"))
}

func newRecordSandboxForPathEnv(pathEnv string) (recordSandbox, func(), error) {
	dir, err := os.MkdirTemp("", "miro-record-sandbox-")
	if err != nil {
		return recordSandbox{}, nil, err
	}

	cleanup := func() {
		_ = os.RemoveAll(dir)
	}

	sandbox := recordSandbox{
		hostHome: filepath.Join(dir, "home"),
		hostTmp:  filepath.Join(dir, "tmp"),
		pathEnv:  pathEnv,
	}

	for _, path := range []string{
		sandbox.hostHome,
		sandbox.hostTmp,
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			cleanup()
			return recordSandbox{}, nil, err
		}
	}

	return sandbox, cleanup, nil
}
func runRecordSession(dir, rawIn, rawOut, shellPath string, sandbox recordSandbox, rio recordIO) error {
	cmd := exec.Command("script", "-q", "-E", "always", "-I", rawIn, "-O", rawOut, "-c", shellPath)
	cmd.Dir = dir
	cmd.Env = recordSessionEnv(sandbox)
	cmd.Stdin = rio.in
	cmd.Stdout = rio.out
	cmd.Stderr = rio.err
	return cmd.Run()
}

func confirmRecordOverwrite(target string, rio recordIO) (bool, error) {
	exists, err := recordFixturesExist(target)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil
	}

	output.Fprintf(rio.err, "Overwrite existing recording? [y/N] ")
	return readRecordConfirmation(rio)
}

func confirmRecordSave(rio recordIO) (bool, error) {
	output.Fprintf(rio.err, "Save recording? [y/N] ")

	return readRecordConfirmation(rio)
}

func readRecordConfirmation(rio recordIO) (bool, error) {
	reply, err := bufio.NewReader(rio.in).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}

	reply = strings.TrimSpace(reply)
	switch strings.ToLower(reply) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func recordFixturesExist(target string) (bool, error) {
	for _, path := range []string{
		filepath.Join(target, "in"),
		filepath.Join(target, "out"),
	} {
		_, err := os.Stat(path)
		if err == nil {
			return true, nil
		}
		if !os.IsNotExist(err) {
			return false, err
		}
	}

	return false, nil
}

func loadRecordedFixtures(rawIn, rawOut string) ([]byte, []byte, error) {
	recordedIn, err := loadRecordedInput(rawIn)
	if err != nil {
		return nil, nil, err
	}

	recordedOut, err := loadRecordedOutput(rawOut)
	if err != nil {
		return nil, nil, err
	}

	return recordedIn, recordedOut, nil
}
