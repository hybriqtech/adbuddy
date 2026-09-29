// Command adbuddy is the ADBuddy host: CLI, daemon and — when invoked as
// `adb` — the shim that forwards adb commands to the daemon.
package main

import (
	"os"

	"github.com/hybriqtech/adbuddy/host/internal/cli"
	"github.com/hybriqtech/adbuddy/host/internal/shim"
)

func main() {
	if shim.IsShim(os.Args[0]) {
		os.Exit(shim.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
	}
	os.Exit(cli.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
