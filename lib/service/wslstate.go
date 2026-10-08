package service

import (
	"bytes"
	"context"
	"strings"
	"unicode/utf16"
)

// IsWslRunning checks state without executing a Linux command.
func IsWslRunning() (bool, error) {
	state, err := (Scanner{}).State(context.Background())
	return state.State == "running", err
}

func containsFold(names []string, name string) bool {
	if name == "" {
		return false
	}
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// decodeWslOutput converts wsl.exe output, which is utf-16le on most
// versions, to a string.
func decodeWslOutput(b []byte) string {
	if len(b) >= 2 && b[0] == 0xff && b[1] == 0xfe {
		b = b[2:]
	} else if !bytes.Contains(b, []byte{0}) {
		// utf-8 text never contains NUL, while ascii chars in utf-16le always do
		return string(b)
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return string(utf16.Decode(u))
}

// parseDefaultDistro returns the distribution marked with '*' in the
// output of `wsl --list --verbose`. Only the marker and the name column
// are used, so localized headers and states don't matter.
func parseDefaultDistro(out string) string {
	for _, line := range parseLines(out) {
		if !strings.HasPrefix(line, "*") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "*"))
		if len(fields) >= 3 {
			return strings.Join(fields[:len(fields)-2], " ")
		}
	}
	return ""
}

// parseLines splits output into trimmed non-empty lines.
func parseLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
