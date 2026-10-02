//go:build !unix

package cli

import "errors"

// fifosExist: no FIFOs in this file system (on Windows, named pipes live in
// \\.\pipe\, out of any environment).
const fifosExist = false

func makeFIFO(string) error { return errors.New("no FIFOs on this system") }
