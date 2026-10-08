package main

import (
	"fmt"
	"os"

	"github.com/HobaiRiku/wsl2-auto-portproxy/cmd"
)

var version = "dev"

func main() {
	if err := cmd.Execute(version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
