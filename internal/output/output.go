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
		info:        0xD7FFFF,
		pass:        0x87D75F,
		fail:        0xFF8787,
	}
)

type Color int

const (
	Info Color = iota
	Pass
	Fail
)

func label(text string, color Color) string {
	var fg uint32

	switch color {
	case Info:
		fg = palette.info
	case Pass:
		fg = palette.pass
	case Fail:
		fg = palette.fail
	default:
		fg = palette.info
	}

	return NewStyle().FG(fg).Apply(text)
}

func Label(text string, color Color) string {
	return label(text, color)
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
