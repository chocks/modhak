// Command modhak is an advisor and dashboard for MariaDB and MySQL.
package main

import (
	"fmt"
	"os"
)

const usage = `modhak: advisor and dashboard for MariaDB and MySQL

Usage:
  modhak <command> [flags]

Commands (M0):
  version    print the version
`

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, "modhak", version)
		return 0
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "modhak: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}
