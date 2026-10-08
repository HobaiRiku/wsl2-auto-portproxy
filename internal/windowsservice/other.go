//go:build !windows

package windowsservice

import "errors"

func unsupported() error             { return errors.New("SCM service management is available only on Windows") }
func Install(InstallOptions) error   { return unsupported() }
func Control(string) error           { return unsupported() }
func Elevate([]string) (bool, error) { return false, unsupported() }
func SystemHome() string             { return "" }
func OpenBrowser(string) error       { return unsupported() }
