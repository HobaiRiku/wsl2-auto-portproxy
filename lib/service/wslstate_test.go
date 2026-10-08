package service

import (
	"testing"
	"unicode/utf16"
)

func utf16le(s string, bom bool) []byte {
	var b []byte
	if bom {
		b = append(b, 0xff, 0xfe)
	}
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

const listVerbose = "  NAME            STATE           VERSION\r\n" +
	"* Ubuntu-22.04    Running         2\r\n" +
	"  Debian          Stopped         2\r\n"

const listVerboseZh = "  名称            状态            版本\r\n" +
	"  Debian          已停止          2\r\n" +
	"* Ubuntu          正在运行        2\r\n"

func TestDecodeWslOutput(t *testing.T) {
	for _, s := range []string{listVerbose, listVerboseZh} {
		if got := decodeWslOutput(utf16le(s, false)); got != s {
			t.Errorf("utf-16 decode: got %q", got)
		}
		if got := decodeWslOutput(utf16le(s, true)); got != s {
			t.Errorf("utf-16 with bom decode: got %q", got)
		}
		if got := decodeWslOutput([]byte(s)); got != s {
			t.Errorf("utf-8 passthrough: got %q", got)
		}
	}
}

func TestParseDefaultDistro(t *testing.T) {
	if got := parseDefaultDistro(listVerbose); got != "Ubuntu-22.04" {
		t.Errorf("got %q", got)
	}
	if got := parseDefaultDistro(listVerboseZh); got != "Ubuntu" {
		t.Errorf("got %q", got)
	}
	if got := parseDefaultDistro(""); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestParseLines(t *testing.T) {
	got := parseLines("Ubuntu\r\n\r\nDebian\r\n")
	if len(got) != 2 || got[0] != "Ubuntu" || got[1] != "Debian" {
		t.Errorf("got %q", got)
	}
}

func TestContainsFold(t *testing.T) {
	names := []string{"Ubuntu-22.04", "Debian"}
	if !containsFold(names, "ubuntu-22.04") {
		t.Error("should match case-insensitively")
	}
	if containsFold(names, "") || containsFold(nil, "Debian") {
		t.Error("empty name or list should not match")
	}
}
