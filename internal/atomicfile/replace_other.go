//go:build !windows

package atomicfile

import "os"

func replace(from, to string) error { return os.Rename(from, to) }
