package output

import "fmt"

const (
	ansiBold   = "\x1b[1m"
	ansiItalic = "\x1b[3m"
	ansiReset  = "\x1b[0m"
)

type Style struct {
	bold   bool
	italic bool
	fg     *uint32
	bg     *uint32
}

func NewStyle() Style {
	return Style{}
}

func (s Style) Bold() Style {
	s.bold = true
	return s
}

func (s Style) Italic() Style {
	s.italic = true
	return s
}

func (s Style) FG(rgb uint32) Style {
	s.fg = &rgb
	return s
}

func (s Style) BG(rgb uint32) Style {
	s.bg = &rgb
	return s
}

func (s Style) Apply(text string) string {
	if !s.bold && !s.italic && s.fg == nil && s.bg == nil {
		return text
	}

	style := ""
	if s.bold {
		style += ansiBold
	}
	if s.italic {
		style += ansiItalic
	}
	if s.fg != nil {
		style += ansiFG(*s.fg)
	}
	if s.bg != nil {
		style += ansiBG(*s.bg)
	}

	return style + text + ansiReset
}

func ansiFG(rgb uint32) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", byte(rgb>>16), byte(rgb>>8), byte(rgb))
}

func ansiBG(rgb uint32) string {
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", byte(rgb>>16), byte(rgb>>8), byte(rgb))
}
