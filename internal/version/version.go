package version

import "fmt"

// Variables are injected at build time via -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func String() string {
	return fmt.Sprintf("version=%s commit=%s buildDate=%s", Version, Commit, BuildDate)
}
