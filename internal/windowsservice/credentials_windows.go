//go:build windows

package windowsservice

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

func readPassword() (string, error) {
	fmt.Fprint(os.Stderr, "Windows account password (for a Microsoft account, its online password; a PIN does not work): ")
	data, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	password := string(data)
	for i := range data {
		data[i] = 0
	}
	return password, nil
}

type lsaAttributes struct {
	Length                   uint32
	RootDirectory            uintptr
	ObjectName               uintptr
	Attributes               uint32
	SecurityDescriptor       uintptr
	SecurityQualityOfService uintptr
}
type lsaString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

func grantServiceLogon(owner string) error {
	sid, err := windows.StringToSid(owner)
	if err != nil {
		return err
	}
	dll := windows.NewLazySystemDLL("advapi32.dll")
	open := dll.NewProc("LsaOpenPolicy")
	add := dll.NewProc("LsaAddAccountRights")
	closePolicy := dll.NewProc("LsaClose")
	toError := dll.NewProc("LsaNtStatusToWinError")
	attributes := lsaAttributes{}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var policy uintptr
	check := func(status uintptr) error {
		if uint32(status) == 0 {
			return nil
		}
		code, _, _ := toError.Call(status)
		return syscall.Errno(code)
	}
	status, _, _ := open.Call(0, uintptr(unsafe.Pointer(&attributes)), 0x810, uintptr(unsafe.Pointer(&policy)))
	if err := check(status); err != nil {
		return err
	}
	defer closePolicy.Call(policy)
	name, _ := windows.UTF16FromString("SeServiceLogonRight")
	right := lsaString{Length: uint16((len(name) - 1) * 2), MaximumLength: uint16(len(name) * 2), Buffer: &name[0]}
	status, _, _ = add.Call(policy, uintptr(unsafe.Pointer(sid)), uintptr(unsafe.Pointer(&right)), 1)
	runtime.KeepAlive(sid)
	runtime.KeepAlive(name)
	return check(status)
}

var errBadPassword = errors.New("incorrect password")

// verifyLogon checks the password the way SCM will use it, so a PIN or a typo
// fails here instead of at the first service start. Needs SeServiceLogonRight.
func verifyLogon(account, password string) error {
	domain, name := ".", account
	if i := strings.LastIndex(account, `\`); i >= 0 {
		domain, name = account[:i], account[i+1:]
	}
	u, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	d, err := windows.UTF16PtrFromString(domain)
	if err != nil {
		return err
	}
	pw, err := windows.UTF16FromString(password)
	if err != nil {
		return err
	}
	defer func() {
		for i := range pw {
			pw[i] = 0
		}
	}()
	const logonService, providerDefault = 5, 0
	var token windows.Token
	ok, _, callErr := windows.NewLazySystemDLL("advapi32.dll").NewProc("LogonUserW").Call(
		uintptr(unsafe.Pointer(u)), uintptr(unsafe.Pointer(d)), uintptr(unsafe.Pointer(&pw[0])),
		logonService, providerDefault, uintptr(unsafe.Pointer(&token)))
	runtime.KeepAlive(pw)
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_LOGON_FAILURE) {
			return errBadPassword
		}
		return fmt.Errorf("verify service logon: %w", callErr)
	}
	return token.Close()
}
