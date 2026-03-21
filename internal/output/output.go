package output

import (
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	palette = struct {
		miroGreen   uint32
		chevronTeal uint32
		info        uint32
		pass        uint32
		fail        uint32
	}{
		miroGreen:   0x70E000,
		chevronTeal: 0x1DD3B0,
		info:        0xf5f2e1,
		pass:        0x7bf1a8,
		fail:        0xff8fa3,
	}
)

func label(text string, color uint32) string {
	return NewStyle().FG(color).Apply(text)
}

func LabelPass(text string) string {
	return label(text, palette.pass)
}

func LabelFail(text string) string {
	return label(text, palette.fail)
}

func LabelInfo(text string) string {
	return label(text, palette.info)
}

func noColor() bool {
	return os.Getenv("NO_COLOR") != ""
}

func prefix() string {
	chevron := NewStyle().FG(palette.chevronTeal).Bold().Italic().Apply("›")
	return NewStyle().FG(palette.miroGreen).Bold().Italic().Apply("miro") + " " + chevron + " "
}

func Format(msg string) string {
	body := strings.TrimRight(msg, "\n")
	suffix := msg[len(body):]
	return prefix() + body + suffix
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
