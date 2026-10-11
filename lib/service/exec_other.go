//go:build !windows

package service

import "os/exec"

func hideWindow(*exec.Cmd) {}
