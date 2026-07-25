package main

import (
	"fmt"
	"os"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/cli"
)

func main() {
	if os.Getenv("AWG_VDS_ASKPASS") == "1" {
		// OpenSSH invokes the same binary as an in-memory askpass helper.
		_, _ = fmt.Fprint(os.Stdout, os.Getenv("AWG_VDS_SSH_PASSWORD"))
		return
	}
	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
