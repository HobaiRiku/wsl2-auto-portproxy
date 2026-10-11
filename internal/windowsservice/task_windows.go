//go:build windows

package windowsservice

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/atomicfile"
	"golang.org/x/sys/windows"
)

// The default deployment is a Task Scheduler task with an S4U principal: it
// runs as the WSL owner without a stored password, in the background from
// boot, which a Windows service can only do with the account password.
const (
	modeService = "service"
	modeTask    = "task"
)

func escapeXML(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// taskXML describes the task. The boot trigger repeats every five minutes and
// IgnoreNew skips runs while one is active, so a crashed instance comes back;
// stop therefore disables the task as well as ending it.
func taskXML(d Deployment) string {
	args := []string{"run", "--home", d.Home, "--listen", d.Listen}
	for i, arg := range args {
		args[i] = syscall.EscapeArg(arg)
	}
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>WSL Port Proxy: Windows to WSL TCP/UDP forwarding</Description>
  </RegistrationInfo>
  <Triggers>
    <BootTrigger>
      <Enabled>true</Enabled>
      <Repetition>
        <Interval>PT5M</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
    </BootTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>` + escapeXML(d.OwnerSID) + `</UserId>
      <LogonType>S4U</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>5</Priority>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>999</Count>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + escapeXML(d.Executable) + `</Command>
      <Arguments>` + escapeXML(strings.Join(args, " ")) + `</Arguments>
      <WorkingDirectory>` + escapeXML(d.Home) + `</WorkingDirectory>
    </Exec>
  </Actions>
</Task>
`
}

func schtasks(args ...string) error {
	out, err := exec.Command("schtasks.exe", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
func taskExists() bool { return exec.Command("schtasks.exe", "/Query", "/TN", Name).Run() == nil }

// registerTask passes the definition as a UTF-16 file, the encoding schtasks
// expects for /XML.
func registerTask(d Deployment) error {
	text := utf16.Encode([]rune(taskXML(d)))
	data := make([]byte, 2, 2+2*len(text))
	binary.LittleEndian.PutUint16(data, 0xFEFF)
	for _, c := range text {
		data = binary.LittleEndian.AppendUint16(data, c)
	}
	file := filepath.Join(root(), "task.xml")
	if err := atomicfile.Write(file, data, 0600); err != nil {
		return err
	}
	defer os.Remove(file)
	return schtasks("/Create", "/TN", Name, "/XML", file)
}

// The rule follows the installed binary: without it, inbound connections from
// other devices are dropped because background processes get no firewall prompt.
func addFirewallRule(program string) error {
	removeFirewallRule(program)
	out, err := exec.Command("netsh.exe", "advfirewall", "firewall", "add", "rule", "name="+Name, "dir=in", "action=allow", "enable=yes", "profile=any", "program="+program).CombinedOutput()
	if err != nil {
		return fmt.Errorf("firewall rule: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
func removeFirewallRule(program string) {
	exec.Command("netsh.exe", "advfirewall", "firewall", "delete", "rule", "name="+Name, "program="+program).Run()
}

// running reports whether any process runs the given image, which is how a
// task's state is checked without parsing localized schtasks output.
func running(image string) (bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), filepath.Base(image)) {
			continue
		}
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		size := uint32(len(buf))
		err = windows.QueryFullProcessImageName(process, 0, &buf[0], &size)
		windows.CloseHandle(process)
		if err == nil && strings.EqualFold(windows.UTF16ToString(buf[:size]), image) {
			return true, nil
		}
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return false, nil
	}
	return false, err
}
func waitFor(what string, timeout time.Duration, done func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for {
		ok, err := done()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", what)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
func healthy(listen string, timeout time.Duration) error {
	client := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return waitFor("the management API", timeout, func() (bool, error) {
		response, err := client.Get("http://" + listen + "/api/health")
		if err != nil {
			return false, nil
		}
		response.Body.Close()
		return response.StatusCode == 200, nil
	})
}

// startTask and stopTask also enable and disable it, so the watchdog
// repetition neither revives a stopped instance nor skips a started one.
func startTask(d Deployment) error {
	if err := schtasks("/Change", "/TN", Name, "/ENABLE"); err != nil {
		return err
	}
	if ok, err := running(d.Executable); err != nil || ok {
		return err
	}
	if err := schtasks("/Run", "/TN", Name); err != nil {
		return err
	}
	return healthy(d.Listen, 20*time.Second)
}
func stopTask(d Deployment) error {
	if err := schtasks("/Change", "/TN", Name, "/DISABLE"); err != nil {
		return err
	}
	schtasks("/End", "/TN", Name) // fails harmlessly when not running
	return waitFor("wslpp to exit", 20*time.Second, func() (bool, error) {
		ok, err := running(d.Executable)
		return !ok, err
	})
}
