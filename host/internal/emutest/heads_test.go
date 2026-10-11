package emutest

import (
	"context"
	"strings"
	"testing"
)

// The app's foreground reads each window's type and root package without the tree (P5): the equivalence of
// windowHeads() with windows() is a safety check of the guard, so it runs here on the real service, with
// -e adbuddyPerf true (the whole suite skips ForegroundHeadsOnEmulatorTest). The test enables the accessibility
// service and restores the settings it changes.
func TestHeadsMatchWindowsOnTheEmulator(t *testing.T) {
	e := newEmu(t)
	e.installApp(t)
	e.installTests(t)
	ctx, cancel := context.WithTimeout(e.ctx, instrumentTimeout)
	defer cancel()
	out, errOut, code, err := e.adb.Output(ctx, "shell", "am instrument -w -r -e class "+headsTest+" -e adbuddyPerf true "+instrumentRunner)
	out = strings.ReplaceAll(out, "\r", "")
	if err != nil || code != 0 {
		t.Fatalf("am instrument: exit %d, %v: %s\n%s", code, err, strings.TrimSpace(errOut), out)
	}
	if tests, _, skipped, failed := parseInstrument(out); failed || tests != 1 || len(skipped) != 0 {
		t.Fatalf("%s did not pass (%d tests, skipped %v):\n%s", headsTest, tests, skipped, out)
	}
}
