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
// emulator's own adb server, never through Gradle's connected* tasks (plan
// M2.5b, rulings de la Tarea 2): AGP's test engine reaches adb server 5037, where the user's
// phone may be.
const instrumentRunner = phoneapp.Package + ".test/androidx.test.runner.AndroidJUnitRunner"

// instrumentTimeout caps `am instrument`, on top of the context's own limit.
const instrumentTimeout = 5 * time.Minute

// requiredClasses must show up in the output: a suite that silently lost one
// of them is not green.
// NotificationListenerOnEmulatorTest only reports its two skips here: it is
// required so that a build that lost it does not go unseen.
var requiredClasses = []string{"TokenProviderTest", "AdbWifiObserverTest", "LocalesTest", "NotificationListenerOnEmulatorTest"}

// allowedSkips are the only tests the suite may see skipped, as
// class#method: CutAccessOnEmulatorTest cuts the emulator's adb, so it runs
// only from TestCutAccessAndReactivate, with -e adbuddyCut 1, and
// TickerOnEmulatorTest holds a notification until the host releases it, so it
// runs only from TestNotificationReadsAndTheCodes, with -e adbuddyTicker 1;
// GuardOnEmulatorTest holds a heads-up or a bubble for the guard's tests
// (guard_test.go), with -e adbuddyGuard; NotificationListenerOnEmulatorTest
// reads the listener and the gate for listener_test.go, with -e
// adbuddyListener gate or content: this suite passes none of them.
var allowedSkips = map[string]bool{cutTest: true, tickerTest: true, guardTest: true, listenerGateTest: true, listenerContentTest: true}

// tickerTest is the app's instrumented half of TestNotificationReadsAndTheCodes
// (android/app/src/androidTest/…/TickerOnEmulatorTest.kt).
const tickerTest = phoneapp.Package + ".TickerOnEmulatorTest#postAndHold"

// guardTest is the app's instrumented half of the guard's tests with a
// heads-up or the bubble (android/app/src/androidTest/…/GuardOnEmulatorTest.kt).
const guardTest = phoneapp.Package + ".GuardOnEmulatorTest#postAndHold"

var (
	okRe       = regexp.MustCompile(`(?m)^OK \((\d+) tests?\)`)
	classRe    = regexp.MustCompile(`(?m)^INSTRUMENTATION_STATUS: class=(\S+)`)
	failuresRe = regexp.MustCompile(`(?m)^(FAILURES!!!|Tests run:)`)
)

// shortName drops the package of a class (and of class#method).
func shortName(name string) string {
	cls, _, _ := strings.Cut(name, "#")
	if i := strings.LastIndex(cls, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

// parseInstrument reads the output of `am instrument -w -r`: how many tests
// the runner says ran ("OK (N tests)"), the short names of the classes that
// reported, the tests in allowedSkips that were skipped (short class#method),
// and whether the run failed. It fails on a failure or an error (status
// codes -2, -1), on a test that did not run (-3 ignored, -4 assumption
// failure) unless it is a skip (-4) in allowedSkips, on "FAILURES!!!", with
// no "OK (N tests)" line and with N = 0.
func parseInstrument(out string) (tests int, classes, skipped []string, failed bool) {
	seen := map[string]bool{}
	for _, m := range classRe.FindAllStringSubmatch(out, -1) {
		name := shortName(m[1])
		if !seen[name] {
			seen[name] = true
			classes = append(classes, name)
		}
	}
	sort.Strings(classes)
	if m := okRe.FindStringSubmatch(out); m != nil {
		tests, _ = strconv.Atoi(m[1])
	}
	// Each status block names its class and test before its code.
	var class, test string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "INSTRUMENTATION_STATUS: class="); ok {
			class = v
		} else if v, ok := strings.CutPrefix(line, "INSTRUMENTATION_STATUS: test="); ok {
			test = v
		} else if v, ok := strings.CutPrefix(line, "INSTRUMENTATION_STATUS_CODE: "); ok {
			switch v {
			case "-1", "-2", "-3":
				failed = true
			case "-4":
				if name := class + "#" + test; allowedSkips[name] {
					skipped = append(skipped, shortName(name))
				} else {
					failed = true
				}
			}
			class, test = "", ""
		}
	}
	failed = failed || tests == 0 || failuresRe.MatchString(out)
	return tests, classes, skipped, failed
}

// minInstrumentedTests is how many tests the app's APK has (the cut, the
// ticker, the guard's and the listener's two, skipped here, included): fewer
// means a class was dropped from the run.
const minInstrumentedTests = 20

func enoughInstrumentedTests(n int) bool { return n >= minInstrumentedTests }

func TestEnoughInstrumentedTests(t *testing.T) {
	for n, want := range map[int]bool{0: false, 19: false, 20: true, 21: true} {
		if got := enoughInstrumentedTests(n); got != want {
			t.Errorf("%d tests: enough = %v, want %v", n, got, want)
		}
	}
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
	// test reports one method, as the runner does: test= in each block, and
	// the assumption's stack trace (several lines) in the last one.
	test := func(class, method string, code int) string {
		head := "INSTRUMENTATION_STATUS: class=tech.hybriq.adbuddy." + class + "\n" +
			"INSTRUMENTATION_STATUS: current=1\nINSTRUMENTATION_STATUS: id=AndroidJUnitRunner\n" +
			"INSTRUMENTATION_STATUS: numtests=16\nINSTRUMENTATION_STATUS: stream=\n" +
			"INSTRUMENTATION_STATUS: test=" + method + "\n"
		end := head
		if code == -4 {
			end = "INSTRUMENTATION_STATUS: class=tech.hybriq.adbuddy." + class + "\n" +
				"INSTRUMENTATION_STATUS: stack=org.junit.AssumptionViolatedException: only host/internal/emutest runs it\n" +
				"\tat org.junit.Assume.assumeTrue(Assume.java:68)\n" +
				"INSTRUMENTATION_STATUS: stream=\nINSTRUMENTATION_STATUS: test=" + method + "\n"
		}
		return head + "INSTRUMENTATION_STATUS_CODE: 1\n" + end + "INSTRUMENTATION_STATUS_CODE: " + strconv.Itoa(code) + "\n"
	}
	cut := test("CutAccessOnEmulatorTest", "cutThenReactivate", -4)
	result16 := "INSTRUMENTATION_RESULT: stream=\n\nTime: 2.1\n\nOK (16 tests)\n\n\nINSTRUMENTATION_CODE: -1\n"
	cases := []struct {
		name    string
		out     string
		tests   int
		classes []string
		skipped []string
		failed  bool
	}{
		{"ok", ok, 15, []string{"AdbWifiObserverTest", "LocalesTest", "TokenProviderTest"}, nil, false},
		{"failure", status("TokenProviderTest", -2) + "INSTRUMENTATION_RESULT: stream=\n\nTime: 2\n\nFAILURES!!!\nTests run: 7,  Failures: 1\n\n\nINSTRUMENTATION_CODE: -1\n",
			0, []string{"TokenProviderTest"}, nil, true},
		{"error", status("LocalesTest", -1) + "OK (5 tests)\n", 5, []string{"LocalesTest"}, nil, true},
		{"empty", "", 0, nil, nil, true},
		{"skipped", status("LocalesTest", -4) + "INSTRUMENTATION_RESULT: stream=\n\nOK (5 tests)\n", 5, []string{"LocalesTest"}, nil, true},
		{"ignored", status("LocalesTest", -3) + "OK (5 tests)\n", 5, []string{"LocalesTest"}, nil, true},
		{"zero tests", "INSTRUMENTATION_RESULT: stream=\n\nOK (0 tests)\n", 0, nil, nil, true},
		{"one test", status("LocalesTest", 0) + "OK (1 test)\n", 1, []string{"LocalesTest"}, nil, false},
		// The one skip allowed: the cut, which only TestCutAccessAndReactivate runs.
		{"the cut skipped", ok[:strings.Index(ok, "INSTRUMENTATION_RESULT")] + cut + result16, 16,
			[]string{"AdbWifiObserverTest", "CutAccessOnEmulatorTest", "LocalesTest", "TokenProviderTest"},
			[]string{"CutAccessOnEmulatorTest#cutThenReactivate"}, false},
		{"the cut skipped first", cut + test("LocalesTest", "spanishIsSpanish", 0) + "OK (2 tests)\n", 2,
			[]string{"CutAccessOnEmulatorTest", "LocalesTest"}, []string{"CutAccessOnEmulatorTest#cutThenReactivate"}, false},
		// Any other skip still fails: another method of that class, the same
		// method of another class, a method that only starts the same way.
		{"another method skipped", test("CutAccessOnEmulatorTest", "somethingElse", -4) + "OK (1 test)\n", 1,
			[]string{"CutAccessOnEmulatorTest"}, nil, true},
		{"another class skipped", test("LocalesTest", "cutThenReactivate", -4) + "OK (1 test)\n", 1,
			[]string{"LocalesTest"}, nil, true},
		{"a longer name skipped", test("CutAccessOnEmulatorTest", "cutThenReactivateAgain", -4) + "OK (1 test)\n", 1,
			[]string{"CutAccessOnEmulatorTest"}, nil, true},
		{"the cut skipped and another", cut + test("LocalesTest", "spanishIsSpanish", -4) + "OK (2 tests)\n", 2,
			[]string{"CutAccessOnEmulatorTest", "LocalesTest"}, []string{"CutAccessOnEmulatorTest#cutThenReactivate"}, true},
		// Only skipped (-4): ignored (-3) or failed (-2) is not allowed even for the cut.
		{"the cut ignored", test("CutAccessOnEmulatorTest", "cutThenReactivate", -3) + "OK (1 test)\n", 1,
			[]string{"CutAccessOnEmulatorTest"}, nil, true},
		{"the cut failed", test("CutAccessOnEmulatorTest", "cutThenReactivate", -2) + "OK (1 test)\n", 1,
			[]string{"CutAccessOnEmulatorTest"}, nil, true},
	}
	for _, c := range cases {
		tests, classes, skipped, failed := parseInstrument(c.out)
		if tests != c.tests || failed != c.failed || strings.Join(classes, ",") != strings.Join(c.classes, ",") ||
			strings.Join(skipped, ",") != strings.Join(c.skipped, ",") {
			t.Errorf("%s: got (%d, %v, %v, %v), want (%d, %v, %v, %v)", c.name, tests, classes, skipped, failed,
				c.tests, c.classes, c.skipped, c.failed)
		}
	}
}

// TestInstrumentedSuite runs the app's instrumented tests on the emulator
// (TokenProviderTest, AdbWifiObserverTest, LocalesTest) through its own adb
// server, and fails on anything but a full green run with only the cut
// (allowedSkips) skipped.
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
	tests, classes, skipped, failed := parseInstrument(out)
	if failed {
		t.Fatalf("the instrumented tests failed (%d tests ran):\n%s", tests, out)
	}
	if !enoughInstrumentedTests(tests) {
		t.Errorf("%d tests ran, want at least %d (the skipped cut included)", tests, minInstrumentedTests)
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
	// The cut, the ticker, the guard's and the listener's must have been
	// skipped here, not run: they are in the APK and this suite never passes
	// -e adbuddyCut 1, -e adbuddyTicker 1, -e adbuddyGuard nor -e adbuddyListener.
	if len(skipped) != len(allowedSkips) {
		t.Errorf("skipped %v, want each of %v skipped once", skipped, allowedSkips)
	}
	t.Logf("instrumented tests: OK (%d tests) in %v, skipped as expected %v", tests, classes, skipped)
}
