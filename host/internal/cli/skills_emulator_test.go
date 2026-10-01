package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestSkillsReadOnlyOnEmulator runs the examples marked green on an Android
// emulator and fails if the phone answers that a program or an option does
// not exist. It needs ADBUDDY_MKSH_SERIAL and ADBUDDY_MKSH_ADB (scripts/emulator.sh
// start prints them), the same as TestRecordOracle, and refuses anything that
// is not an emulator: it never runs on a real phone.
func TestSkillsReadOnlyOnEmulator(t *testing.T) {
	serial, adbPath := os.Getenv("ADBUDDY_MKSH_SERIAL"), os.Getenv("ADBUDDY_MKSH_ADB")
	if serial == "" || adbPath == "" {
		t.Skip("set ADBUDDY_MKSH_SERIAL and ADBUDDY_MKSH_ADB (scripts/emulator.sh start)")
	}
	if !strings.HasPrefix(serial, "emulator-") {
		t.Fatalf("ADBUDDY_MKSH_SERIAL=%q is not an emulator: the skills' examples never run on a real phone", serial)
	}
	missing := []string{": not found", "inaccessible or not found", "unknown option", "invalid option", "illegal option", "unknown command", "error: device"}
	ran := 0
	for _, ex := range skillExamples(t) {
		if ex.kind != "adb" || ex.level != "green" {
			continue
		}
		fields, err := splitExample(ex.text)
		if err != nil {
			t.Fatalf("%s:%d: %v", ex.file, ex.line, err)
		}
		if fields[0] != "shell" {
			continue // a pull would write into the current folder, and what it answers depends on the sample paths
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		out, err := exec.CommandContext(ctx, adbPath, append([]string{"-s", serial}, fields...)...).CombinedOutput()
		timedOut := ctx.Err() != nil
		cancel()
		if timedOut {
			t.Errorf("%s:%d: %q timed out", ex.file, ex.line, ex.text)
			continue
		}
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			t.Fatalf("%s:%d: adb did not start: %v", ex.file, ex.line, err) // a non-zero exit is normal (grep, command -v)
		}
		ran++
		low := strings.ToLower(string(out))
		for _, m := range missing {
			if strings.Contains(low, m) {
				t.Errorf("%s:%d: %q on the emulator: %s", ex.file, ex.line, ex.text, strings.TrimSpace(string(out)))
				break
			}
		}
	}
	if ran == 0 {
		t.Fatal("no green example was run")
	}
}
