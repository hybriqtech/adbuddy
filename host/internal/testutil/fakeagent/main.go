// Command fakeagent stands in for a coding agent in tests. It never talks to
// a model: it puts its terminal in raw mode, turns on bracketed paste and
// echoes each pasted message (or typed line) as "got:<text>". A message
// "send <file>" runs `adbuddy send <file>` (found in the PATH the host gives
// the agent) and prints "sent:ok", or "sent:error:<what adbuddy said>".
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

func main() {
	st, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent: raw mode:", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), st)
	enableVTInput()
	fmt.Print("\x1b[?2004hready\r\n")

	var cur []byte
	inPaste := false
	b := make([]byte, 1)
	for {
		if n, err := os.Stdin.Read(b); n == 0 || err != nil {
			if err != nil {
				return
			}
			continue
		}
		cur = append(cur, b[0])
		s := string(cur)
		switch {
		case strings.HasSuffix(s, pasteStart):
			cur, inPaste = cur[:len(cur)-len(pasteStart)], true
		case strings.HasSuffix(s, pasteEnd):
			cur, inPaste = cur[:len(cur)-len(pasteEnd)], false
		case b[0] == '\r' && !inPaste:
			line := string(cur[:len(cur)-1])
			cur = nil
			handle(line)
		}
	}
}

func handle(text string) {
	if file, ok := strings.CutPrefix(text, "send "); ok {
		out, err := exec.Command("adbuddy", "send", file).CombinedOutput()
		if err != nil {
			fmt.Printf("sent:error:%s\r\n", strings.TrimSpace(string(out)))
			return
		}
		fmt.Print("sent:ok\r\n")
		return
	}
	fmt.Printf("got:%s\r\n", strings.ReplaceAll(text, "\n", `\n`))
}
