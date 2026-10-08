package paths

import (
	"os"
	"path/filepath"
)

func Resolve(home string) (string, error) {
	if home == "" {
		home = os.Getenv("WSLPP_HOME")
	}
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(userHome, ".wslpp")
	}
	return filepath.Abs(home)
}
func Ensure(home string) error {
	if err := os.MkdirAll(home, 0700); err != nil {
		return err
	}
	return protect(home)
}
