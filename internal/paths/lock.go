package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// Lock holds an OS file lock for this data root. The persistent file is harmless;
// the kernel releases the lock on exit, including a crashed process.
func Lock(home string) (func(), error) {
	file, err := os.OpenFile(filepath.Join(home, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(file); err != nil {
		file.Close()
		return nil, fmt.Errorf("data root already in use or cannot be locked: %w", err)
	}
	return func() { file.Close() }, nil
}
