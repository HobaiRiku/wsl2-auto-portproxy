//go:build !windows

package paths

import "os"

func protect(path string) error { return os.Chmod(path, 0700) }
