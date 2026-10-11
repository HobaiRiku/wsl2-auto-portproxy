package service

import (
	"bytes"
	"strings"
	"unicode/utf16"
)

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

// parseDistros reads `wsl --list --verbose`. Only the default marker, the
// last column (version) and the name are used, so localized headers and
// states don't matter. Names may contain spaces; states are one word.
func parseDistros(out string) []Distro {
	var distros []Distro
	for i, line := range parseLines(out) {
		if i == 0 && !strings.HasPrefix(line, "*") {
			continue // header
		}
		isDefault := strings.HasPrefix(line, "*")
		fields := strings.Fields(strings.TrimPrefix(line, "*"))
		if len(fields) < 3 {
			continue
		}
		distros = append(distros, Distro{Name: strings.Join(fields[:len(fields)-2], " "), Default: isDefault, Version: fields[len(fields)-1]})
	}
	return distros
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
