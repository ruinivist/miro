/*
Intially mire dependend on screen(1)
But that had some quite a few issues
- kernel tty line discipline: script controlled a slave bash terminal so backspaces if any
were truncated due to output piping as the bash propmt wasn't ready. Initially we moves to
do disable that as a pty option entirely. That broke echo so now we have a marker based
approach which I guess would've worked in screen to.
- Second issue, screen runs took ~200 ms while with this custom one it's 8ms for the same
fixtures so this was kept.
*/

package script

import (
	"bytes"
	"context"
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
	replayReadySettleDelay = 5 * time.Millisecond
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
	DelayInput bool
	InputDelay time.Duration
	Timeout    time.Duration
}

type ReplayResult struct {
	ProcessErr error
	OutputErr  error
	InputErr   error
}

func (r ReplayResult) Err() error {
	return firstErr(r.ProcessErr, r.OutputErr, r.InputErr)
}

// Record runs a live PTY session so we can mirror interactive output while capturing stable input and output logs.
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

	// Keep the child PTY aligned with the real terminal so full-screen apps
	// redraw against the size the user is actually seeing.
	stopResize := watchResize(req.TTY, ptmx)
	defer stopResize()

	// Duplicate file-backed input so the stop path can close our copy without
	// accidentally tearing down the caller's stdin handle.
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

// Replay feeds recorded keystrokes back into a fresh PTY session to verify behavior against captured output.
func Replay(req ReplayRequest) ReplayResult {
	if req.Cmd == nil {
		return ReplayResult{ProcessErr: errors.New("replay session command is required")}
	}

	ptmx, err := pty.StartWithSize(req.Cmd, sessionSize(nil))
	if err != nil {
		return ReplayResult{ProcessErr: err}
	}
	defer ptmx.Close()

	outputDone := copyAsync(combineWriters(req.OutputLog), ptmx)
	processDone := make(chan struct{})
	inputDone := copyReplayInputWhenReady(ptmx, req.Input, req.InputReady, processDone, req.DelayInput, req.InputDelay)

	timedOut := make(chan struct{}, 1)
	var timeout *time.Timer
	if req.Timeout > 0 {
		timeout = time.AfterFunc(req.Timeout, func() {
			select {
			case timedOut <- struct{}{}:
			default:
			}
			if req.Cmd.Process != nil {
				_ = req.Cmd.Process.Kill()
			}
		})
	}

	waitErr := req.Cmd.Wait()
	if timeout != nil {
		timeout.Stop()
	}
	select {
	case <-timedOut:
		waitErr = context.DeadlineExceeded
	default:
	}
	close(processDone)
	ptmx.Close()

	outputErr := <-outputDone
	inputErr := <-inputDone

	return ReplayResult{
		ProcessErr: waitErr,
		OutputErr:  outputErr,
		InputErr:   inputErr,
	}
}

// combineWriters skips nil destinations so callers can fan out conditionally without repeated nil checks.
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

// copyAsync moves a stream in the background so PTY input and output can progress concurrently.
func copyAsync(dst io.Writer, src io.Reader) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(dst, src)
		done <- normalizeCopyError(err)
	}()
	return done
}

// copyReplayInputWhenReady delays replay input until the child shell is ready enough to receive it reliably.
func copyReplayInputWhenReady(dst io.Writer, input []byte, ready <-chan struct{}, stop <-chan struct{}, delayInput bool, inputDelay time.Duration) <-chan error {
	done := make(chan error, 1)
	go func() {
		if ready != nil {
			select {
			case <-ready:
				// Some programs signal readiness before they have finished their
				// first prompt, so we wait briefly to avoid replaying input too early.
				time.Sleep(replayReadySettleDelay)
			case <-stop:
				done <- nil
				return
			}
		}

		data, appendedEOF := replayInput(input)
		if !delayInput || inputDelay <= 0 {
			_, err := io.Copy(dst, bytes.NewReader(data))
			done <- normalizeCopyError(err)
			return
		}

		for i, b := range data {
			select {
			case <-stop:
				done <- nil
				return
			default:
			}

			if _, err := dst.Write([]byte{b}); err != nil {
				done <- normalizeCopyError(err)
				return
			}

			if appendedEOF && i == len(data)-1 {
				continue
			}
			if !shouldDelayReplayByte(b) {
				continue
			}

			timer := time.NewTimer(inputDelay)
			select {
			case <-timer.C:
			case <-stop:
				if !timer.Stop() {
					<-timer.C
				}
				done <- nil
				return
			}
		}

		done <- nil
	}()
	return done
}

// replayInput appends terminal EOF so non-interactive replays still tell the shell when scripted input is finished.
func replayInput(input []byte) ([]byte, bool) {
	if len(input) > 0 && input[len(input)-1] == terminalEOF {
		return input, false
	}

	data := make([]byte, 0, len(input)+1)
	data = append(data, input...)
	data = append(data, terminalEOF)
	return data, true
}

func shouldDelayReplayByte(b byte) bool {
	return b == '\n' || b == 0x03 || b == terminalEOF
}

// copyInputAsync gives file-backed input an interruptible read loop so shutdown does not hang on blocked stdin reads.
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
		// Poll lets us break out of a blocked terminal read immediately when the
		// session ends instead of waiting for the next keystroke.
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

// normalizeCopyError treats expected PTY shutdown conditions as success so callers only see actionable failures.
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

// firstErr preserves the earliest real failure because later shutdown errors are often just fallout.
func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// sessionSize prefers the caller's terminal geometry so interactive programs render as if they were attached directly.
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

// makeRaw disables local terminal processing so the child PTY sees the user's exact keystrokes and control bytes.
func makeRaw(tty *os.File) (func(), error) {
	if tty == nil || !term.IsTerminal(int(tty.Fd())) {
		return func() {}, nil
	}

	// Raw mode lets the child process receive keystrokes and control sequences
	// directly instead of having the local terminal preprocess them first.
	state, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return nil, err
	}

	return func() {
		_ = term.Restore(int(tty.Fd()), state)
	}, nil
}

// watchResize forwards host terminal resizes so curses-style apps redraw against the current viewport.
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

// duplicateInput gives the recorder ownership of file-backed input without mutating the caller's descriptor lifecycle.
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

	// The recorder may need to close its input to stop cleanly, but that should
	// only affect the duplicated descriptor it owns.
	dup := os.NewFile(uintptr(fd), file.Name())
	return dup, func() {
		_ = dup.Close()
	}, nil
}

type inputLogWriter struct {
	dst io.Writer
}

// newInputLogWriter hides optional logging behind a writer so the input path stays linear.
func newInputLogWriter(dst io.Writer) io.Writer {
	if dst == nil {
		return io.Discard
	}
	return inputLogWriter{dst: dst}
}

// Write normalizes terminal line endings so recorded input fixtures are easy to diff and reuse.
func (w inputLogWriter) Write(p []byte) (int, error) {
	normalized := make([]byte, len(p))
	for i, b := range p {
		if b == '\r' {
			// Logs are easier to diff and replay reasoning against when Enter is
			// stored as a regular newline instead of terminal carriage returns.
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
