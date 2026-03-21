package miro

import (
	"bytes"
	"fmt"
	"os"
)

var (
	scriptStartPrefix = []byte("Script started on ")
	scriptDonePrefix  = []byte("Script done on ")
)

const eofByte = byte(0x04)

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

	return stripScriptWrapper(data)
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
