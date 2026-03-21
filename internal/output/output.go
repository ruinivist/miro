package output

import (
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	palette = struct {
		miroPrefixGreen uint32
		miroChevronTeal uint32
	}{
		miroPrefixGreen: 0x70E000,
		miroChevronTeal: 0x1DD3B0,
	}

	chevron = NewStyle().FG(palette.miroChevronTeal).Bold().Italic().Apply("›")
	prefix  = NewStyle().FG(palette.miroPrefixGreen).Bold().Italic().Apply("miro") + " " + chevron + " "
)

func Prefix() string {
	return prefix
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
