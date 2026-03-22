package mire

import (
	"bytes"
	"fmt"
	"os"
)

var (
	scriptStartPrefix = []byte("Script started on ")
	scriptDonePrefix  = []byte("Script done on ")
	shellPromptPrefix = []byte("\x1b[?2004h$ ")
)

const (
	interruptByte = byte(0x03)
	eofByte       = byte(0x04)
	escByte       = byte(0x1b)
	bellByte      = byte(0x07)
)

var interruptSuffix = []byte("^C\x1b[?2004l\r\x1b[?2004h\x1b[?2004l\r\r\n")

func loadRecordedInput(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if hasScriptWrapper(data) {
		data, err = stripScriptWrapper(data)
		if err != nil {
			return nil, err
		}
	}

	return trimTrailingReplayNewline(data), nil
}

func loadRecordedOutput(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if !hasScriptWrapper(data) {
		return stripTerminalTitlePrefixes(data), nil
	}

	data, err = stripScriptWrapper(data)
	if err != nil {
		return nil, err
	}

	return stripTerminalTitlePrefixes(data), nil
}

func sanitizeInterrupts(input, output []byte) ([]byte, []byte) {
	return sanitizeInterruptInput(input), sanitizeInterruptOutput(output)
}

func sanitizeInterruptInput(data []byte) []byte {
	cleaned := make([]byte, 0, len(data))
	lineStart := 0

	for _, b := range data {
		if b == interruptByte {
			cleaned = cleaned[:lineStart]
			continue
		}

		cleaned = append(cleaned, b)
		if b == '\n' {
			lineStart = len(cleaned)
		}
	}

	return cleaned
}

func sanitizeInterruptOutput(data []byte) []byte {
	cleaned := make([]byte, 0, len(data))
	cursor := 0

	for {
		suffixStart := bytes.Index(data[cursor:], interruptSuffix)
		if suffixStart == -1 {
			cleaned = append(cleaned, data[cursor:]...)
			return cleaned
		}
		suffixStart += cursor

		promptStart := bytes.LastIndex(data[cursor:suffixStart], shellPromptPrefix)
		if promptStart == -1 {
			end := suffixStart + len(interruptSuffix)
			cleaned = append(cleaned, data[cursor:end]...)
			cursor = end
			continue
		}
		promptStart += cursor

		if bytes.IndexByte(data[promptStart+len(shellPromptPrefix):suffixStart], '\n') != -1 {
			end := suffixStart + len(interruptSuffix)
			cleaned = append(cleaned, data[cursor:end]...)
			cursor = end
			continue
		}

		cleaned = append(cleaned, data[cursor:promptStart]...)
		cursor = suffixStart + len(interruptSuffix)
	}
}

func hasScriptWrapper(data []byte) bool {
	if bytes.HasPrefix(data, scriptStartPrefix) {
		return true
	}

	_, state := findScriptFooter(data)
	return state != footerMissing
}

func stripScriptWrapper(data []byte) ([]byte, error) {
	hasHeader := bytes.HasPrefix(data, scriptStartPrefix)

	headerEnd := bytes.IndexByte(data, '\n')
	if hasHeader && headerEnd == -1 {
		return nil, fmt.Errorf("could not parse script wrapper from recorded output: incomplete header line")
	}

	body := data
	if hasHeader {
		body = data[headerEnd+1:]
	}

	footerStart, footerState := findScriptFooter(body)
	switch {
	case hasHeader && footerState == footerMissing:
		return nil, fmt.Errorf("could not parse script wrapper from recorded output: missing footer line")
	case !hasHeader && footerState == footerFound:
		return nil, fmt.Errorf("could not parse script wrapper from recorded output: missing header line")
	case !hasHeader && footerState == footerMissing:
		return nil, fmt.Errorf("could not parse script wrapper from recorded output: missing header and footer lines")
	case footerState == footerMalformed:
		return nil, fmt.Errorf("could not parse script wrapper from recorded output: incomplete footer line")
	}

	return body[:footerStart], nil
}

type footerMatchState int

const (
	footerMissing footerMatchState = iota
	footerMalformed
	footerFound
)

func findScriptFooter(data []byte) (int, footerMatchState) {
	candidate := -1
	switch {
	case bytes.HasPrefix(data, scriptDonePrefix):
		candidate = 0
	default:
		idx := bytes.LastIndex(data, append([]byte{'\n'}, scriptDonePrefix...))
		if idx != -1 {
			candidate = idx + 1
		}
	}

	if candidate == -1 {
		return 0, footerMissing
	}

	footer := data[candidate:]
	newline := bytes.IndexByte(footer, '\n')
	if newline == -1 || candidate+newline+1 != len(data) {
		return 0, footerMalformed
	}

	return candidate, footerFound
}

func trimTrailingReplayNewline(data []byte) []byte {
	// util-linux script records the final Enter as "\r\n", but replay feeds the
	// bytes from a file rather than a tty. If the shell exits after consuming the
	// trailing '\r', the leftover '\n' can trigger script's ~2s non-tty stdin
	// delay before it shuts down. Keep the '\r' that bash consumed, drop only the
	// synthetic final '\n'. The same cleanup is needed after a terminal EOF byte.
	if len(data) >= 2 && data[len(data)-1] == '\n' && (data[len(data)-2] == '\r' || data[len(data)-2] == eofByte) {
		return data[:len(data)-1]
	}

	return data
}

func stripTerminalTitlePrefixes(data []byte) []byte {
	var cleaned []byte

	for i := 0; i < len(data); {
		if !hasTerminalTitlePrefixAt(data, i) {
			if cleaned != nil {
				cleaned = append(cleaned, data[i])
			}
			i++
			continue
		}

		end := bytes.IndexByte(data[i+4:], bellByte)
		if end == -1 {
			if cleaned == nil {
				return data
			}
			cleaned = append(cleaned, data[i:]...)
			return cleaned
		}

		if cleaned == nil {
			cleaned = make([]byte, 0, len(data))
			cleaned = append(cleaned, data[:i]...)
		}

		i += 4 + end + 1
	}

	if cleaned == nil {
		return data
	}

	return cleaned
}

func hasTerminalTitlePrefixAt(data []byte, i int) bool {
	if i+4 > len(data) {
		return false
	}

	return data[i] == escByte &&
		data[i+1] == ']' &&
		data[i+2] == '0' &&
		data[i+3] == ';'
}
