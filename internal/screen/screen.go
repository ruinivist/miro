package screen

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const (
	defaultRows            = 24
	defaultCols            = 80
	terminalEOF            = byte(0x04)
	replayReadySettleDelay = 10 * time.Millisecond
)

type RecordRequest struct {
	Cmd       *exec.Cmd
	Input     io.Reader
	Output    io.Writer
	TTY       *os.File
	InputLog  io.Writer
	OutputLog io.Writer
}

type ReplayRequest struct {
	Cmd        *exec.Cmd
	Input      []byte
	InputReady <-chan struct{}
	OutputLog  io.Writer
}

func Record(req RecordRequest) error {
	if req.Cmd == nil {
		return errors.New("record session command is required")
	}

	ptmx, err := pty.StartWithSize(req.Cmd, sessionSize(req.TTY))
	if err != nil {
		return err
	}
	defer ptmx.Close()

	restoreTTY, err := makeRaw(req.TTY)
	if err != nil {
		return err
	}
	defer restoreTTY()

	stopResize := watchResize(req.TTY, ptmx)
	defer stopResize()

	input, closeInput, err := duplicateInput(req.Input)
	if err != nil {
		return err
	}
	defer closeInput()

	outputDone := copyAsync(combineWriters(req.Output, req.OutputLog), ptmx)
	inputDone, stopInput, err := copyInputAsync(combineWriters(ptmx, newInputLogWriter(req.InputLog)), input)
	if err != nil {
		return err
	}
	defer stopInput()

	waitErr := req.Cmd.Wait()
	stopInput()
	ptmx.Close()

	outputErr := <-outputDone
	inputErr := <-inputDone

	return firstErr(waitErr, outputErr, inputErr)
}

func Replay(req ReplayRequest) error {
	if req.Cmd == nil {
		return errors.New("replay session command is required")
	}

	ptmx, err := pty.StartWithSize(req.Cmd, sessionSize(nil))
	if err != nil {
		return err
	}
	defer ptmx.Close()

	outputDone := copyAsync(combineWriters(req.OutputLog), ptmx)
	processDone := make(chan struct{})
	inputDone := copyAsyncWhenReady(ptmx, bytes.NewReader(replayInput(req.Input)), req.InputReady, processDone)

	waitErr := req.Cmd.Wait()
	close(processDone)
	ptmx.Close()

	outputErr := <-outputDone
	inputErr := <-inputDone

	return firstErr(waitErr, outputErr, inputErr)
}

func combineWriters(writers ...io.Writer) io.Writer {
	active := make([]io.Writer, 0, len(writers))
	for _, writer := range writers {
		if writer != nil {
			active = append(active, writer)
		}
	}

	switch len(active) {
	case 0:
		return io.Discard
	case 1:
		return active[0]
	default:
		return io.MultiWriter(active...)
	}
}

func copyAsync(dst io.Writer, src io.Reader) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(dst, src)
		done <- normalizeCopyError(err)
	}()
	return done
}

func copyAsyncWhenReady(dst io.Writer, src io.Reader, ready <-chan struct{}, stop <-chan struct{}) <-chan error {
	done := make(chan error, 1)
	go func() {
		if ready != nil {
			select {
			case <-ready:
				time.Sleep(replayReadySettleDelay)
			case <-stop:
				done <- nil
				return
			}
		}
		_, err := io.Copy(dst, src)
		done <- normalizeCopyError(err)
	}()
	return done
}

func replayInput(input []byte) []byte {
	if len(input) > 0 && input[len(input)-1] == terminalEOF {
		return input
	}

	data := make([]byte, 0, len(input)+1)
	data = append(data, input...)
	data = append(data, terminalEOF)
	return data
}

func copyInputAsync(dst io.Writer, src io.Reader) (<-chan error, func(), error) {
	file, ok := src.(*os.File)
	if !ok {
		done := copyAsync(dst, src)
		return done, func() {}, nil
	}

	stopReader, stopWriter, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}

	done := make(chan error, 1)
	go func() {
		defer stopReader.Close()

		buf := make([]byte, 4096)
		fds := []unix.PollFd{
			{Fd: int32(file.Fd()), Events: unix.POLLIN | unix.POLLHUP},
			{Fd: int32(stopReader.Fd()), Events: unix.POLLIN | unix.POLLHUP},
		}

		for {
			_, err := unix.Poll(fds, -1)
			if err != nil {
				if err == unix.EINTR {
					continue
				}
				done <- err
				return
			}
			if fds[1].Revents != 0 {
				done <- nil
				return
			}
			if fds[0].Revents&(unix.POLLIN|unix.POLLHUP) == 0 {
				continue
			}

			n, readErr := file.Read(buf)
			if n > 0 {
				if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
					done <- normalizeCopyError(writeErr)
					return
				}
			}
			if readErr != nil {
				done <- normalizeCopyError(readErr)
				return
			}
		}
	}()

	stop := func() {
		_ = stopWriter.Close()
		_ = file.Close()
	}

	return done, stop, nil
}

func normalizeCopyError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, io.EOF):
		return nil
	case errors.Is(err, os.ErrClosed):
		return nil
	case errors.Is(err, syscall.EIO):
		return nil
	default:
		return err
	}
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func sessionSize(tty *os.File) *pty.Winsize {
	if tty != nil && term.IsTerminal(int(tty.Fd())) {
		if cols, rows, err := term.GetSize(int(tty.Fd())); err == nil {
			return &pty.Winsize{
				Rows: uint16(rows),
				Cols: uint16(cols),
			}
		}
	}

	return &pty.Winsize{
		Rows: defaultRows,
		Cols: defaultCols,
	}
}

func makeRaw(tty *os.File) (func(), error) {
	if tty == nil || !term.IsTerminal(int(tty.Fd())) {
		return func() {}, nil
	}

	state, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return nil, err
	}

	return func() {
		_ = term.Restore(int(tty.Fd()), state)
	}, nil
}

func watchResize(tty *os.File, ptmx *os.File) func() {
	if tty == nil || !term.IsTerminal(int(tty.Fd())) {
		return func() {}
	}

	applySize := func() {
		_ = pty.Setsize(ptmx, sessionSize(tty))
	}
	applySize()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-signals:
				applySize()
			case <-done:
				return
			}
		}
	}()

	return func() {
		signal.Stop(signals)
		close(done)
	}
}

func duplicateInput(input io.Reader) (io.Reader, func(), error) {
	if input == nil {
		return bytes.NewReader(nil), func() {}, nil
	}

	file, ok := input.(*os.File)
	if !ok {
		return input, func() {}, nil
	}

	fd, err := syscall.Dup(int(file.Fd()))
	if err != nil {
		return nil, nil, err
	}

	dup := os.NewFile(uintptr(fd), file.Name())
	return dup, func() {
		_ = dup.Close()
	}, nil
}

type inputLogWriter struct {
	dst io.Writer
}

func newInputLogWriter(dst io.Writer) io.Writer {
	if dst == nil {
		return io.Discard
	}
	return inputLogWriter{dst: dst}
}

func (w inputLogWriter) Write(p []byte) (int, error) {
	normalized := make([]byte, len(p))
	for i, b := range p {
		if b == '\r' {
			normalized[i] = '\n'
			continue
		}
		normalized[i] = b
	}

	if _, err := w.dst.Write(normalized); err != nil {
		return 0, err
	}

	return len(p), nil
}
