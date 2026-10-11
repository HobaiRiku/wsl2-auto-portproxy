//go:build windows

package windowsservice

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestTaskXMLIsWellFormedAndEscaped(t *testing.T) {
	d := Deployment{Home: `C:\Program Data\wslpp & co\data`, Listen: "127.0.0.1:47831", OwnerSID: "S-1-5-21-1-2-3-1001", Executable: `C:\Program Data\wslpp & co\bin\wslpp.exe`}
	var task struct {
		Principal struct {
			UserID    string `xml:"UserId"`
			LogonType string `xml:"LogonType"`
		} `xml:"Principals>Principal"`
		Exec struct {
			Command   string `xml:"Command"`
			Arguments string `xml:"Arguments"`
		} `xml:"Actions>Exec"`
	}
	// encoding/xml rejects the UTF-16 declaration; the body is what matters.
	body := strings.Replace(taskXML(d), `encoding="UTF-16"`, `encoding="UTF-8"`, 1)
	if err := xml.Unmarshal([]byte(body), &task); err != nil {
		t.Fatal(err)
	}
	if task.Principal.UserID != d.OwnerSID || task.Principal.LogonType != "S4U" {
		t.Fatalf("principal %+v", task.Principal)
	}
	if task.Exec.Command != d.Executable {
		t.Fatalf("command %q", task.Exec.Command)
	}
	want := `run --home "C:\Program Data\wslpp & co\data" --listen 127.0.0.1:47831`
	if task.Exec.Arguments != want {
		t.Fatalf("arguments %q, want %q", task.Exec.Arguments, want)
	}
}
