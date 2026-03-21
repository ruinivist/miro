package output

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	bold   = "\x1b[1m"
	italic = "\x1b[3m"
	reset  = "\x1b[0m"
)

var (
	palette = struct {
		miroPrefixGreen uint32
		miroChevronTeal uint32
	}{
		miroPrefixGreen: 0x70E000,
		miroChevronTeal: 0x1DD3B0,
	}

	chevron = ansiColor(palette.miroChevronTeal)
	prefix  = ansiColor(palette.miroPrefixGreen) + bold + italic + "miro" + reset + " " + chevron + bold + italic + "›" + reset + " "
)

func Prefix() string {
	return prefix
}

func ansiColor(rgb uint32) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", byte(rgb>>16), byte(rgb>>8), byte(rgb))
}

func Format(msg string) string {
	body := strings.TrimRight(msg, "\n")
	suffix := msg[len(body):]
	return Prefix() + body + suffix
}

func Println(msg string) {
	Fprintln(os.Stdout, msg)
}

func Printf(format string, args ...any) {
	Fprintf(os.Stdout, format, args...)
}

func Fprintln(w io.Writer, msg string) {
	fmt.Fprintln(w, Format(msg))
}

func Fprintf(w io.Writer, format string, args ...any) {
	fmt.Fprint(w, Format(fmt.Sprintf(format, args...)))
}
