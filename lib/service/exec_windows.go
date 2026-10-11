package service

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps wsl.exe from flashing a console when wslpp has none.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
