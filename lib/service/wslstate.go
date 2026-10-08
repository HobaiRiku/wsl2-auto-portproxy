package service

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"unicode/utf16"
)

// IsWslRunning reports whether the default wsl distribution is running.
// Only `wsl --list` commands are used here, they never boot the wsl vm,
// unlike `wsl -- <cmd>`, so polling this keeps a stopped wsl stopped.
func IsWslRunning() (bool, error) {
	all, err := wslList("--list", "--verbose")
	if err != nil {
		return false, err
	}
	defaultDistro := parseDefaultDistro(all)
	if defaultDistro == "" {
		return false, nil
	}
	// exits with non-zero status when there are no running distributions
	running, err := wslList("--list", "--running", "--quiet")
	if err != nil {
		return false, nil
	}
	for _, name := range parseLines(running) {
		if strings.EqualFold(name, defaultDistro) {
			return true, nil
		}
	}
	return false, nil
}

func wslList(args ...string) (string, error) {
	cmd := exec.Command("wsl", args...)
	// newer wsl prints utf-8 with this set, older ones always print utf-16
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return decodeWslOutput(output), nil
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
		if len(fields) > 0 {
			return fields[0]
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
