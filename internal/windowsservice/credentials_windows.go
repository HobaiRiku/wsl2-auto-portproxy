//go:build windows

package windowsservice

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

func readPassword() (string, error) {
	fmt.Fprint(os.Stderr, "Windows service account password (not Windows Hello PIN): ")
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
