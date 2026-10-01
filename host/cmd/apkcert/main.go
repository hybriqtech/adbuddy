// Command apkcert prints the SHA-256 of the signing certificates of an APK,
// with the parser the host uses before it trusts the app (spec M6a §5),
// every check included. The release workflows use it; it is not shipped.
//
//	apkcert <file.apk>   each distinct signer certificate, one per line
//	apkcert -pinned      the certificate this host pins (fails if none)
package main

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: apkcert <file.apk> | apkcert -pinned")
		return 2
	}
	if args[0] == "-pinned" {
		if phoneapp.SigningCertSHA256 == "" {
			fmt.Fprintln(stderr, "apkcert: this host pins no signing certificate (phoneapp.SigningCertSHA256 is empty; see docs/releasing.md)")
			return 1
		}
		fmt.Fprintln(stdout, phoneapp.SigningCertSHA256)
		return 0
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "apkcert:", err)
		return 1
	}
	certs, err := phoneapp.Signers(data)
	if err != nil {
		fmt.Fprintf(stderr, "apkcert: %s: %v\n", args[0], err)
		return 1
	}
	slices.Sort(certs)
	for _, c := range slices.Compact(certs) {
		fmt.Fprintln(stdout, c)
	}
	return 0
}
