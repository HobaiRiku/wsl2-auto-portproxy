package main

import (
	"fmt"
	"os"

	"github.com/HobaiRiku/wsl2-auto-portproxy/cmd"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/version"
)

func main() {
	if err := cmd.Execute(version.Version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
