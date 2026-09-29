// Command adbuddy is the ADBuddy host: CLI, daemon and — when invoked as
// `adb` — the shim that forwards adb commands to the daemon.
package main

import (
	"os"

	"github.com/hybriqtech/adbuddy/host/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
