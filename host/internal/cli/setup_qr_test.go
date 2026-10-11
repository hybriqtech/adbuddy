package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestQRStyle(t *testing.T) {
	cases := []struct {
		name     string
		term     bool
		noColor  string
		termEnv  string
		vt       bool
		expected bool
	}{
		{"terminal with vt", true, "", "xterm", true, true},
		{"NO_COLOR does not turn the functional QR off", true, "1", "xterm", true, true},
		{"TERM=dumb", true, "", "dumb", true, false},
		{"not a terminal", false, "", "xterm", true, false},
		{"not a terminal, dumb, no vt", false, "1", "dumb", false, false},
		{"terminal without virtual mode", true, "", "xterm", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", c.noColor)
			t.Setenv("TERM", c.termEnv)
			setVar(t, &isTerminal, func(any) bool { return c.term })
			app := &App{Stderr: &bytes.Buffer{}, Getenv: func(k string) string {
				return map[string]string{"NO_COLOR": c.noColor, "TERM": c.termEnv}[k]
			}}
			if got := qrStyle(app, c.vt); got != c.expected {
				t.Fatalf("qrStyle = %v, want %v", got, c.expected)
			}
		})
	}
}

func TestSetupQRHasNoEscapesWhenRedirected(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")
	e := newSetupEnv(t, phoneRules())
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d stderr %s", code, errOut)
	}
	if strings.Contains(errOut, "\x1b") {
		t.Fatalf("escapes in redirected output: %q", errOut)
	}
	if !strings.Contains(errOut, HelpURL) {
		t.Fatalf("no HelpURL in %q", errOut)
	}
	if strings.Contains(errOut, "WIFI:T:ADB") {
		t.Fatalf("pairing payload printed in text: %q", errOut)
	}
}

func TestSetupQRColorOnATerminalIgnoresNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm")
	e := newSetupEnv(t, phoneRules())
	setVar(t, &isTerminal, func(any) bool { return true })
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d stderr %s", code, errOut)
	}
	if !strings.Contains(errOut, "\x1b[30;47m") || !strings.Contains(errOut, "\x1b[0m") {
		t.Fatalf("QR without colors on a terminal: %q", errOut)
	}
	if strings.Contains(errOut, "WIFI:T:ADB") {
		t.Fatalf("pairing payload printed in text")
	}
	// Outside the QR the output adds no colors: every escape is a QR line's.
	for _, line := range strings.Split(errOut, "\n") {
		if strings.Contains(line, "\x1b") && !strings.HasPrefix(line, "\x1b[30;47m") {
			t.Fatalf("escape outside the QR: %q", line)
		}
	}
}
