package main

import (
	"fmt"
	"os"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
