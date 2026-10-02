package emutest

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// The app's instrumented tests run with `am instrument` through the
// emulator's own adb server, never through Gradle's connected* tasks (M2.5b
// spec §2.4): AGP's test engine reaches adb server 5037, where the user's
// phone may be.
const instrumentRunner = phoneapp.Package + ".test/androidx.test.runner.AndroidJUnitRunner"

// instrumentTimeout caps `am instrument`, on top of the context's own limit.
const instrumentTimeout = 5 * time.Minute

// requiredClasses must show up in the output: a suite that silently lost one
// of them is not green.
var requiredClasses = []string{"TokenProviderTest", "AdbWifiObserverTest", "LocalesTest"}

var (
	okRe       = regexp.MustCompile(`(?m)^OK \((\d+) tests?\)`)
	classRe    = regexp.MustCompile(`(?m)^INSTRUMENTATION_STATUS: class=(\S+)`)
	badCodeRe  = regexp.MustCompile(`(?m)^INSTRUMENTATION_STATUS_CODE: (-1|-2|-3|-4)\s*$`)
	failuresRe = regexp.MustCompile(`(?m)^(FAILURES!!!|Tests run:)`)
)

// parseInstrument reads the output of `am instrument -w -r`: how many tests
// the runner says passed ("OK (N tests)"), the short names of the classes
// that reported, and whether the run failed. It fails on a failure or an
// error (status codes -2, -1), on a test that did not run (-3 ignored, -4
// assumption failure: skipped counts as failed), on "FAILURES!!!", with no
// "OK (N tests)" line and with N = 0.
func parseInstrument(out string) (tests int, classes []string, failed bool) {
	seen := map[string]bool{}
	for _, m := range classRe.FindAllStringSubmatch(out, -1) {
		name := m[1]
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		if !seen[name] {
			seen[name] = true
			classes = append(classes, name)
		}
	}
	sort.Strings(classes)
	if m := okRe.FindStringSubmatch(out); m != nil {
		tests, _ = strconv.Atoi(m[1])
	}
	failed = tests == 0 || badCodeRe.MatchString(out) || failuresRe.MatchString(out)
	return tests, classes, failed
}

func TestParseInstrument(t *testing.T) {
	status := func(class string, code int) string {
		return "INSTRUMENTATION_STATUS: class=tech.hybriq.adbuddy." + class + "\n" +
			"INSTRUMENTATION_STATUS_CODE: 1\n" +
			"INSTRUMENTATION_STATUS: class=tech.hybriq.adbuddy." + class + "\n" +
			"INSTRUMENTATION_STATUS_CODE: " + strconv.Itoa(code) + "\n"
	}
	ok := status("TokenProviderTest", 0) + status("AdbWifiObserverTest", 0) + status("LocalesTest", 0) +
		"INSTRUMENTATION_RESULT: stream=\n\nTime: 1.999\n\nOK (15 tests)\n\n\nINSTRUMENTATION_CODE: -1\n"
	cases := []struct {
		name    string
		out     string
		tests   int
		classes []string
		failed  bool
	}{
		{"ok", ok, 15, []string{"AdbWifiObserverTest", "LocalesTest", "TokenProviderTest"}, false},
		{"failure", status("TokenProviderTest", -2) + "INSTRUMENTATION_RESULT: stream=\n\nTime: 2\n\nFAILURES!!!\nTests run: 7,  Failures: 1\n\n\nINSTRUMENTATION_CODE: -1\n",
			0, []string{"TokenProviderTest"}, true},
		{"error", status("LocalesTest", -1) + "OK (5 tests)\n", 5, []string{"LocalesTest"}, true},
		{"empty", "", 0, nil, true},
		{"skipped", status("LocalesTest", -4) + "INSTRUMENTATION_RESULT: stream=\n\nOK (5 tests)\n", 5, []string{"LocalesTest"}, true},
		{"ignored", status("LocalesTest", -3) + "OK (5 tests)\n", 5, []string{"LocalesTest"}, true},
		{"zero tests", "INSTRUMENTATION_RESULT: stream=\n\nOK (0 tests)\n", 0, nil, true},
		{"one test", status("LocalesTest", 0) + "OK (1 test)\n", 1, []string{"LocalesTest"}, false},
	}
	for _, c := range cases {
		tests, classes, failed := parseInstrument(c.out)
		if tests != c.tests || failed != c.failed || strings.Join(classes, ",") != strings.Join(c.classes, ",") {
			t.Errorf("%s: got (%d, %v, %v), want (%d, %v, %v)", c.name, tests, classes, failed, c.tests, c.classes, c.failed)
		}
	}
}

// TestInstrumentedSuite runs the app's instrumented tests on the emulator
// (TokenProviderTest, AdbWifiObserverTest, LocalesTest) through its own adb
// server, and fails on anything but a full green run.
func TestInstrumentedSuite(t *testing.T) {
	e := newEmu(t)
	e.installApp(t)
	e.installTests(t)
	ctx, cancel := context.WithTimeout(e.ctx, instrumentTimeout)
	defer cancel()
	out, errOut, code, err := e.adb.Output(ctx, "shell", "am", "instrument", "-w", "-r", instrumentRunner)
	out = strings.ReplaceAll(out, "\r", "")
	if err != nil || code != 0 {
		t.Fatalf("am instrument: exit %d, %v: %s\n%s", code, err, strings.TrimSpace(errOut), out)
	}
	tests, classes, failed := parseInstrument(out)
	if failed {
		t.Fatalf("the instrumented tests failed (%d tests passed):\n%s", tests, out)
	}
	have := map[string]bool{}
	for _, c := range classes {
		have[c] = true
	}
	for _, c := range requiredClasses {
		if !have[c] {
			t.Errorf("class %s did not run (ran: %v)", c, classes)
		}
	}
	t.Logf("instrumented tests: OK (%d tests) in %v", tests, classes)
}
