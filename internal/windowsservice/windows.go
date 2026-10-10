//go:build windows

package windowsservice

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/atomicfile"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/paths"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/config"
	kservice "github.com/kardianos/service"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func root() string       { return filepath.Join(os.Getenv("ProgramData"), "wslpp") }
func SystemHome() string { return filepath.Join(root(), "data") }
func Elevate(args []string) (bool, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false, err
	}
	defer token.Close()
	if token.IsElevated() {
		return false, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	// The elevated console closes on exit; keep it open so its result is seen.
	args = append(args[:len(args):len(args)], "--pause-on-exit")
	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = syscall.EscapeArg(arg)
	}
	params, _ := windows.UTF16PtrFromString(strings.Join(escaped, " "))
	err = windows.ShellExecute(0, verb, file, params, nil, 1)
	if err != nil {
		return false, fmt.Errorf("UAC: %w", err)
	}
	fmt.Fprintln(os.Stderr, "wslpp: continuing in an elevated window")
	return true, nil
}
func OpenBrowser(url string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	target, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, target, nil, nil, 1)
}
func serviceConfig(d Deployment, password string) *kservice.Config {
	args := []string{"run", "--home", d.Home, "--listen", d.Listen}
	return &kservice.Config{Name: Name, DisplayName: "WSL Port Proxy", Description: "Windows to WSL TCP/UDP forwarding", Executable: d.Executable, Arguments: args, UserName: d.Account, WorkingDirectory: d.Home, Option: kservice.KeyValue{"Password": password, "DelayedAutoStart": true, "OnFailure": "restart", "OnFailureDelayDuration": "5s", "OnFailureResetPeriod": 86400}}
}
func loadDeployment() (Deployment, error) {
	var d Deployment
	data, err := os.ReadFile(filepath.Join(root(), "deployment.json"))
	if err != nil {
		return d, err
	}
	err = json.Unmarshal(data, &d)
	return d, err
}
func dataAccess(path, sid string) error {
	return setAccess(path, sid, "FA")
}
func setAccess(path, sid, access string) error {
	descriptor, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;%s;;;%s)", access, sid))
	if err != nil {
		return err
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
func Install(o InstallOptions) (result error) {
	host, port, err := net.SplitHostPort(o.Listen)
	ip := net.ParseIP(host)
	number, parseErr := strconv.Atoi(port)
	if err != nil || parseErr != nil || ip == nil || !ip.IsLoopback() || number <= 0 || number > 65535 {
		return errors.New("installed management listen must use a loopback IP and fixed port")
	}
	if os.Getenv("ProgramData") == "" {
		return errors.New("ProgramData unavailable")
	}
	if o.Account == "" || o.OwnerSID == "" {
		return errors.New("explicit WSL owner account and SID required")
	}
	account, err := user.Lookup(o.Account)
	if err != nil {
		return err
	}
	if account.Uid != o.OwnerSID {
		return errors.New("service account does not match the WSL owner SID")
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if existing, err := m.OpenService(Name); err == nil {
		existing.Close()
		return errors.New("wslpp already installed; use update to replace its binary")
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if strings.Contains(strings.ToLower(exe), "go-build") {
		return errors.New("install from a stable release binary, not go run")
	}
	if err := grantServiceLogon(o.OwnerSID); err != nil {
		return err
	}
	// Capture the password in this elevated console, never in process arguments.
	var password string
	for attempt := 1; ; attempt++ {
		if password, err = readPassword(); err != nil {
			return err
		}
		err = verifyLogon(o.Account, password)
		if err == nil {
			break
		}
		password = ""
		if !errors.Is(err, errBadPassword) || attempt == 3 {
			return err
		}
		fmt.Fprintln(os.Stderr, "Incorrect password, try again.")
	}
	if err := os.MkdirAll(root(), 0700); err != nil {
		return err
	}
	release, err := paths.Lock(root())
	if err != nil {
		return err
	}
	defer release()
	if existing, err := m.OpenService(Name); err == nil {
		existing.Close()
		return errors.New("wslpp was installed concurrently")
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	if err := setAccess(root(), o.OwnerSID, "GRGX"); err != nil {
		return err
	}
	bin := filepath.Join(root(), "bin")
	if err := paths.Ensure(bin); err != nil {
		return err
	}
	if err := setAccess(bin, o.OwnerSID, "GRGX"); err != nil {
		return err
	}
	home := SystemHome()
	if err := os.MkdirAll(home, 0700); err != nil {
		return err
	}
	if err := dataAccess(home, o.OwnerSID); err != nil {
		return err
	}
	if o.ImportConfig != "" {
		raw, err := os.ReadFile(o.ImportConfig)
		if err != nil {
			return err
		}
		if _, err := config.Parse(raw); err != nil {
			return err
		}
		target := filepath.Join(home, "config.json")
		if _, err := os.Stat(target); os.IsNotExist(err) {
			if err := atomicfile.Write(target, raw, 0600); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	dest := filepath.Join(bin, "wslpp.exe")
	metadata := filepath.Join(root(), "deployment.json")
	if _, err := os.Stat(metadata); err == nil {
		return errors.New("unregistered deployment metadata exists; run uninstall to clean it up first")
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		return errors.New("unregistered installation binary exists; preserve or remove it before installing")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := atomicfile.Write(dest, data, 0700); err != nil {
		return err
	}
	d := Deployment{Home: home, Listen: o.Listen, Account: o.Account, OwnerSID: o.OwnerSID, Executable: dest}
	installAttempted := false
	success := false
	defer func() {
		if success {
			return
		}
		if installAttempted {
			existing, err := m.OpenService(Name)
			if err == nil {
				existing.Close()
				s, createErr := kservice.New(&program{}, serviceConfig(d, ""))
				if createErr == nil {
					createErr = s.Uninstall()
				}
				if createErr != nil {
					// Keep the binary and deployment record so a failed SCM
					// cleanup can be diagnosed and retried with uninstall.
					result = errors.Join(result, fmt.Errorf("installation rollback: %w", createErr))
					return
				}
			} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
				result = errors.Join(result, fmt.Errorf("installation rollback: %w", err))
				return
			}
		}
		result = errors.Join(result, removeIfExists(dest), removeIfExists(metadata))
	}()
	encoded, _ := json.MarshalIndent(d, "", "  ")
	if err := atomicfile.Write(metadata, encoded, 0600); err != nil {
		return err
	}
	service, err := kservice.New(&program{}, serviceConfig(d, password))
	password = ""
	if err != nil {
		return err
	}
	installAttempted = true // Install may create the service before returning an error.
	if err := service.Install(); err != nil {
		return err
	}
	success = true
	return nil
}
func Control(action string) error {
	d, err := loadDeployment()
	if os.IsNotExist(err) && action == "uninstall" {
		return nil
	}
	if err != nil {
		return err
	}
	release, err := paths.Lock(root())
	if err != nil {
		return err
	}
	defer release()
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(Name)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) && action == "uninstall" {
		return errors.Join(removeIfExists(d.Executable), removeIfExists(filepath.Join(root(), "deployment.json")))
	}
	if err != nil {
		return err
	}
	defer s.Close()
	wait := func(state svc.State) error {
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			status, err := s.Query()
			if err != nil {
				return err
			}
			if status.State == state {
				return nil
			}
			select {
			case <-timer.C:
				return errors.New("SCM transition timed out")
			case <-ticker.C:
			}
		}
	}
	stop := func() error {
		status, err := s.Query()
		if err != nil {
			return err
		}
		if status.State == svc.Stopped {
			return nil
		}
		if status.State != svc.StopPending {
			if _, err := s.Control(svc.Stop); err != nil {
				return err
			}
		}
		return wait(svc.Stopped)
	}
	start := func() error {
		status, err := s.Query()
		if err != nil {
			return err
		}
		if status.State == svc.Running {
			return nil
		}
		if status.State != svc.StartPending {
			if err := s.Start(); err != nil {
				return err
			}
		}
		return wait(svc.Running)
	}
	switch action {
	case "start":
		return start()
	case "stop":
		return stop()
	case "restart":
		if err := stop(); err != nil {
			return err
		}
		return start()
	case "uninstall":
		if err := stop(); err != nil {
			return err
		}
		service, err := kservice.New(&program{}, serviceConfig(d, ""))
		if err != nil {
			return err
		}
		if err := service.Uninstall(); err != nil {
			return err
		}
		if err := os.Remove(d.Executable); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Remove(filepath.Join(root(), "deployment.json"))
	case "update":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if strings.EqualFold(exe, d.Executable) {
			return errors.New("run update from the new release binary outside the installed bin directory")
		}
		next, err := os.ReadFile(exe)
		if err != nil {
			return err
		}
		old, err := os.ReadFile(d.Executable)
		if err != nil {
			return err
		}
		if bytes.Equal(next, old) {
			return nil
		}
		return updateBinary(updateOperations{
			stop: stop, start: start,
			backup:  func() error { return atomicfile.Write(d.Executable+".bak", old, 0700) },
			replace: func() error { return atomicfile.Write(d.Executable, next, 0700) },
			restore: func() error { return atomicfile.Write(d.Executable, old, 0700) },
			cleanup: func() { os.Remove(d.Executable + ".bak") },
			healthy: func() error {
				client := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
				deadline := time.Now().Add(10 * time.Second)
				for time.Now().Before(deadline) {
					response, err := client.Get("http://" + d.Listen + "/api/health")
					if err == nil {
						response.Body.Close()
						if response.StatusCode == 200 {
							return nil
						}
					}
					time.Sleep(100 * time.Millisecond)
				}
				return errors.New("new service failed health check")
			},
		})
	default:
		return fmt.Errorf("unknown service action %q", action)
	}
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
